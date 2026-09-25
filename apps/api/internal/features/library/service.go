package library

import (
	"context"
	"errors"
	neturl "net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/classcode"
	"teka/apps/api/internal/shared/dbtypes"
	"teka/apps/api/internal/shared/idset"
	"teka/apps/api/internal/shared/pagination"
)

// Service implements the library business rules: one draft per template,
// published versions are immutable, a new draft starts from the latest
// published lessons, and every lesson list stays contiguously numbered.
// Authorization is checked here as well as by the route policy so callers
// that bypass HTTP get the same 403.
type Service struct {
	repo Repository
	tx   database.TxManager
}

// NewService builds the service.
func NewService(repo Repository, tx database.TxManager) *Service {
	return &Service{repo: repo, tx: tx}
}

// CreateTemplate inserts a template and opens its first draft (v1) so the
// author can add lessons right away.
func (s *Service) CreateTemplate(ctx context.Context, sc authctx.Scope, req TemplateRequest) (*TemplateResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return nil, err
	}
	code, err := normalizeCode(req.Code)
	if err != nil {
		return nil, err
	}
	tpl := &Template{
		Code: code, Name: strings.TrimSpace(req.Name),
		Subject: optionalText(req.Subject), Level: optionalText(req.Level), Description: optionalText(req.Description),
		CreatedBy: &sc.TeacherID,
	}
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.repo.CreateTemplate(ctx, sc, tpl); err != nil {
			return mapCodeClash(err)
		}
		draft := &Version{TemplateID: tpl.ID, VersionNo: 1, Status: StatusDraft, CreatedBy: &sc.TeacherID}
		if err := s.repo.CreateVersion(ctx, sc, draft); err != nil {
			return err
		}
		if req.LessonCount == nil || *req.LessonCount <= 0 {
			return nil
		}
		// Seed placeholder lessons so the draft has cards before anyone
		// writes the content.
		rows := make([]*Lesson, 0, *req.LessonCount)
		for i := 1; i <= *req.LessonCount; i++ {
			rows = append(rows, &Lesson{VersionID: draft.ID, Position: i, Title: "Buổi " + strconv.Itoa(i)})
		}
		return s.repo.CreateLessons(ctx, sc, rows)
	})
	if err != nil {
		return nil, err
	}
	return s.GetTemplate(ctx, sc, tpl.ID)
}

// GetTemplate returns one template with its version summary.
func (s *Service) GetTemplate(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*TemplateResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryRead); err != nil {
		return nil, err
	}
	row, err := s.repo.GetTemplate(ctx, sc, id)
	if err != nil {
		return nil, notFound(err, "program template")
	}
	versions, err := s.repo.ListVersions(ctx, sc, id)
	if err != nil {
		return nil, err
	}
	row.Versions = versionRefs(versions)
	out := templateResponse(row)
	return &out, nil
}

// ListTemplates pages the center's live templates.
func (s *Service) ListTemplates(ctx context.Context, sc authctx.Scope, f ListFilter, p pagination.Params) ([]TemplateResponse, int64, error) {
	if err := authctx.Require(sc, authctx.PermLibraryRead); err != nil {
		return nil, 0, err
	}
	rows, total, err := s.repo.ListTemplates(ctx, sc, f, p)
	if err != nil {
		return nil, 0, err
	}
	ids := make([]uuid.UUID, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
	}
	versionsByTemplate, err := s.versionsByTemplate(ctx, sc, ids)
	if err != nil {
		return nil, 0, err
	}
	out := make([]TemplateResponse, 0, len(rows))
	for i := range rows {
		rows[i].Versions = versionRefs(versionsByTemplate[rows[i].ID])
		out = append(out, templateResponse(&rows[i]))
	}
	return out, total, nil
}

// versionsByTemplate batches ListVersions across every given template id in
// one query, grouping the rows in Go, so a caller looping over templates
// never issues one ListVersions call per row.
func (s *Service) versionsByTemplate(ctx context.Context, sc authctx.Scope, templateIDs []uuid.UUID) (map[uuid.UUID][]VersionRow, error) {
	rows, err := s.repo.ListVersionsFor(ctx, sc, templateIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID][]VersionRow, len(templateIDs))
	for _, row := range rows {
		out[row.TemplateID] = append(out[row.TemplateID], row)
	}
	return out, nil
}

// UpdateTemplate replaces the template's own fields.
func (s *Service) UpdateTemplate(ctx context.Context, sc authctx.Scope, id uuid.UUID, req TemplateRequest) (*TemplateResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return nil, err
	}
	code, err := normalizeCode(req.Code)
	if err != nil {
		return nil, err
	}
	tpl := &Template{
		ID: id, Code: code, Name: strings.TrimSpace(req.Name),
		Subject: optionalText(req.Subject), Level: optionalText(req.Level), Description: optionalText(req.Description),
	}
	if err := s.repo.UpdateTemplate(ctx, sc, tpl); err != nil {
		return nil, notFound(mapCodeClash(err), "program template")
	}
	return s.GetTemplate(ctx, sc, id)
}

// DeleteTemplate soft-deletes the template; its versions and lessons stay
// in place for anything still bound to them. The row is locked for the
// in-use check and the stamp: a class applying one of its versions
// concurrently holds the row FOR SHARE, so one side waits and sees the
// other's outcome instead of racing a pre-lock snapshot.
func (s *Service) DeleteTemplate(ctx context.Context, sc authctx.Scope, id uuid.UUID) error {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return err
	}
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := s.repo.LockTemplate(ctx, sc, id); err != nil {
			return notFound(err, "program template")
		}
		inUse, err := s.repo.TemplateInUse(ctx, sc, id)
		if err != nil {
			return err
		}
		if inUse {
			return errTemplateInUse()
		}
		return notFound(s.repo.SoftDeleteTemplate(ctx, sc, id), "program template")
	})
}

// LockTemplateForVersion locks the version's template row for the rest of
// the caller's transaction: classprogram's Apply calls this before reading
// the version, so a concurrent DeleteTemplate of the same template either
// waits for the apply to finish or has already removed the template by the
// time the apply reaches the lock. ErrNotFound (mapped to 404) covers both a
// version outside the center and a template already deleted.
func (s *Service) LockTemplateForVersion(ctx context.Context, sc authctx.Scope, versionID uuid.UUID) error {
	return notFound(s.repo.LockTemplateForVersion(ctx, sc, versionID), "program template")
}

// PublishedVersion returns a version with its lessons for the class-program
// feature: the class page reads through the class's own read gate, so a
// class's program must not need catalog access. Only a published version
// can be applied, so a draft or archived one is a 409.
func (s *Service) PublishedVersion(ctx context.Context, sc authctx.Scope, versionID uuid.UUID) (*VersionResponse, []LessonDetailResponse, error) {
	return s.versionForClass(ctx, sc, versionID, func(status string) bool { return status == StatusPublished })
}

// ReleasedVersion is PublishedVersion for reading back a version a class
// already applies: a released (published or archived) version is immutable,
// so retiring it in the library must not blank the class's lessons. Only a
// draft is refused.
func (s *Service) ReleasedVersion(ctx context.Context, sc authctx.Scope, versionID uuid.UUID) (*VersionResponse, []LessonDetailResponse, error) {
	return s.versionForClass(ctx, sc, versionID, func(status string) bool { return status != StatusDraft })
}

func (s *Service) versionForClass(ctx context.Context, sc authctx.Scope, versionID uuid.UUID, accept func(status string) bool) (*VersionResponse, []LessonDetailResponse, error) {
	row, err := s.repo.GetVersion(ctx, sc, versionID)
	if err != nil {
		return nil, nil, notFound(err, "template version")
	}
	if !accept(row.Status) {
		return nil, nil, errVersionNotPublishedForClass()
	}
	lessons, err := s.repo.ListLessons(ctx, sc, versionID)
	if err != nil {
		return nil, nil, err
	}
	details, err := s.lessonDetails(ctx, sc, lessons)
	if err != nil {
		return nil, nil, err
	}
	version := versionResponse(row)
	return &version, details, nil
}

// CreateVersion opens a new draft numbered after the highest existing
// version, seeded with a copy of the lessons of the latest released
// version (published, or archived when nothing is published any more).
// 409 DRAFT_EXISTS while another draft is open.
func (s *Service) CreateVersion(ctx context.Context, sc authctx.Scope, templateID uuid.UUID, req CreateVersionRequest) (*VersionResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return nil, err
	}
	tpl, err := s.repo.GetTemplate(ctx, sc, templateID)
	if err != nil {
		return nil, notFound(err, "program template")
	}
	if tpl.DraftVersionNo != nil {
		return nil, errDraftExists()
	}
	var draft Version
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		no, err := s.repo.NextVersionNo(ctx, sc, templateID)
		if err != nil {
			return err
		}
		draft = Version{
			TemplateID: templateID, VersionNo: no, Status: StatusDraft,
			Changelog: req.Changelog, CreatedBy: &sc.TeacherID,
		}
		if err := s.repo.CreateVersion(ctx, sc, &draft); err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) {
				return errDraftExists()
			}
			return err
		}
		source, err := s.repo.LatestReleasedVersion(ctx, sc, templateID)
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return s.copyVersionContent(ctx, sc, source, &draft)
	})
	if err != nil {
		return nil, err
	}
	return s.version(ctx, sc, draft.ID)
}

// copyVersionContent seeds the new draft with everything the source
// version carries: exercise groups (so lesson-exercise links can point at
// their copy), lessons with their material and exercise links, the log
// fields and the score set. Items are shared by reference (the same
// center-wide material row), everything else is new rows.
func (s *Service) copyVersionContent(ctx context.Context, sc authctx.Scope, source, draft *Version) error {
	groups, err := s.repo.ListExerciseGroups(ctx, sc, source.ID)
	if err != nil {
		return err
	}
	groupOf := make(map[uuid.UUID]uuid.UUID, len(groups))
	if len(groups) > 0 {
		groupCopies := make([]*ExerciseGroup, len(groups))
		for i := range groups {
			groupCopies[i] = &ExerciseGroup{VersionID: draft.ID, Name: groups[i].Name, Position: groups[i].Position}
		}
		if err := s.repo.CreateExerciseGroups(ctx, sc, groupCopies); err != nil {
			return err
		}
		for i := range groups {
			groupOf[groups[i].ID] = groupCopies[i].ID
		}
	}
	lessons, err := s.repo.ListLessons(ctx, sc, source.ID)
	if err != nil {
		return err
	}
	copies := make([]*Lesson, 0, len(lessons))
	for i := range lessons {
		l := lessons[i]
		copies = append(copies, &Lesson{
			VersionID: draft.ID, Position: l.Position, Title: l.Title, Mode: l.Mode, Unit: l.Unit,
			Objectives: l.Objectives, DurationMin: l.DurationMin, HomeworkNote: l.HomeworkNote,
		})
	}
	if err := s.repo.CreateLessons(ctx, sc, copies); err != nil {
		return err
	}
	if len(lessons) > 0 {
		sourceIDs := lessonIDs(lessons)
		copyOf := make(map[uuid.UUID]uuid.UUID, len(lessons))
		for i := range lessons {
			copyOf[lessons[i].ID] = copies[i].ID
		}
		materials, err := s.repo.ListLessonMaterials(ctx, sc, sourceIDs)
		if err != nil {
			return err
		}
		mLinks := make([]LessonMaterial, 0, len(materials))
		for _, row := range materials {
			mLinks = append(mLinks, LessonMaterial{
				LessonID: copyOf[row.LessonID], MaterialID: row.ID,
				SharedWithStudents: row.SharedWithStudents, Position: row.Position,
			})
		}
		if err := s.repo.CreateLessonMaterials(ctx, sc, mLinks); err != nil {
			return err
		}
		exercises, err := s.repo.ListLessonExercises(ctx, sc, sourceIDs)
		if err != nil {
			return err
		}
		eLinks := make([]LessonExercise, 0, len(exercises))
		for _, row := range exercises {
			var newGroup *uuid.UUID
			if row.GroupID != nil {
				if g, ok := groupOf[*row.GroupID]; ok {
					newGroup = &g
				}
			}
			eLinks = append(eLinks, LessonExercise{
				LessonID: copyOf[row.LessonID], ExerciseID: row.ID, GroupID: newGroup, Position: row.Position,
			})
		}
		if err := s.repo.CreateLessonExercises(ctx, sc, eLinks); err != nil {
			return err
		}
	}
	fields, err := s.repo.ListLogFields(ctx, sc, source.ID)
	if err != nil {
		return err
	}
	fieldCopies := make([]*LogField, 0, len(fields))
	for i := range fields {
		f := fields[i]
		fieldCopies = append(fieldCopies, &LogField{Label: f.Label, Kind: f.Kind, Options: f.Options, Required: f.Required})
	}
	if len(fieldCopies) > 0 {
		if err := s.repo.ReplaceLogFields(ctx, sc, draft.ID, fieldCopies); err != nil {
			return err
		}
	}
	if len(source.ScoreSet) > 0 {
		return s.repo.SetScoreSet(ctx, sc, draft.ID, source.ScoreSet)
	}
	return nil
}

// ListVersions returns the template's versions, newest first.
func (s *Service) ListVersions(ctx context.Context, sc authctx.Scope, templateID uuid.UUID) ([]VersionResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryRead); err != nil {
		return nil, err
	}
	if _, err := s.repo.GetTemplate(ctx, sc, templateID); err != nil {
		return nil, notFound(err, "program template")
	}
	rows, err := s.repo.ListVersions(ctx, sc, templateID)
	if err != nil {
		return nil, err
	}
	versionIDs := make([]uuid.UUID, len(rows))
	for i := range rows {
		versionIDs[i] = rows[i].ID
	}
	classesByVersion, err := s.classesByVersion(ctx, sc, versionIDs)
	if err != nil {
		return nil, err
	}
	out := make([]VersionResponse, 0, len(rows))
	for i := range rows {
		out = append(out, versionResponseWithClasses(&rows[i], classesByVersion[rows[i].ID]))
	}
	return out, nil
}

// classesByVersion batches ListVersionClasses across every given version id
// in one query, grouping the rows in Go.
func (s *Service) classesByVersion(ctx context.Context, sc authctx.Scope, versionIDs []uuid.UUID) (map[uuid.UUID][]VersionClassRef, error) {
	links, err := s.repo.ListVersionClassesFor(ctx, sc, versionIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID][]VersionClassRef, len(versionIDs))
	for _, link := range links {
		out[link.VersionID] = append(out[link.VersionID], VersionClassRef{ID: link.ID, Name: link.Name})
	}
	return out, nil
}

// Publish turns the draft into the published version and locks its lessons.
func (s *Service) Publish(ctx context.Context, sc authctx.Scope, versionID uuid.UUID) (*VersionResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryPublish); err != nil {
		return nil, err
	}
	now := nowUTC()
	err := s.transition(ctx, sc, versionID, StatusDraft, StatusPublished, &now, errVersionNotDraft)
	if err != nil {
		return nil, err
	}
	return s.version(ctx, sc, versionID)
}

// Archive retires a published version without deleting anything.
func (s *Service) Archive(ctx context.Context, sc authctx.Scope, versionID uuid.UUID) (*VersionResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return nil, err
	}
	err := s.transition(ctx, sc, versionID, StatusPublished, StatusArchived, nil, errVersionNotPublished)
	if err != nil {
		return nil, err
	}
	return s.version(ctx, sc, versionID)
}

// transition moves a version from one status to the next under the row
// lock, so it waits for any in-flight lesson write of that version and
// answers wrongStatus once the row is no longer in `from`.
func (s *Service) transition(ctx context.Context, sc authctx.Scope, versionID uuid.UUID, from, to string, publishedAt *time.Time, wrongStatus func() *apperror.AppError) error {
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		v, err := s.repo.LockVersion(ctx, sc, versionID)
		if err != nil {
			return notFound(err, "template version")
		}
		if v.Status != from {
			return wrongStatus()
		}
		return s.repo.SetVersionStatus(ctx, sc, versionID, from, to, publishedAt)
	})
}

// ListLessons returns the version's lessons in position order.
func (s *Service) ListLessons(ctx context.Context, sc authctx.Scope, versionID uuid.UUID) ([]LessonResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryRead); err != nil {
		return nil, err
	}
	if _, err := s.repo.GetVersion(ctx, sc, versionID); err != nil {
		return nil, notFound(err, "template version")
	}
	rows, err := s.repo.ListLessons(ctx, sc, versionID)
	if err != nil {
		return nil, err
	}
	return lessonResponses(rows), nil
}

// GetLesson returns one lesson with its attached materials and exercises.
func (s *Service) GetLesson(ctx context.Context, sc authctx.Scope, lessonID uuid.UUID) (*LessonDetailResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryRead); err != nil {
		return nil, err
	}
	l, err := s.repo.GetLesson(ctx, sc, lessonID)
	if err != nil {
		return nil, notFound(err, "template lesson")
	}
	details, err := s.lessonDetails(ctx, sc, []LessonRow{*l})
	if err != nil {
		return nil, err
	}
	return &details[0], nil
}

// lessonDetails decorates lessons with their attachments in two bulk
// reads, so a version detail costs the same number of queries as one lesson.
func (s *Service) lessonDetails(ctx context.Context, sc authctx.Scope, lessons []LessonRow) ([]LessonDetailResponse, error) {
	ids := lessonIDs(lessons)
	materials, err := s.repo.ListLessonMaterials(ctx, sc, ids)
	if err != nil {
		return nil, err
	}
	exercises, err := s.repo.ListLessonExercises(ctx, sc, ids)
	if err != nil {
		return nil, err
	}
	byLessonM := make(map[uuid.UUID][]LessonMaterialRow, len(lessons))
	for _, row := range materials {
		byLessonM[row.LessonID] = append(byLessonM[row.LessonID], row)
	}
	byLessonE := make(map[uuid.UUID][]LessonExerciseRow, len(lessons))
	for _, row := range exercises {
		byLessonE[row.LessonID] = append(byLessonE[row.LessonID], row)
	}
	out := make([]LessonDetailResponse, 0, len(lessons))
	for i := range lessons {
		l := &lessons[i]
		out = append(out, LessonDetailResponse{
			LessonResponse: lessonRowResponse(l),
			Materials:      lessonMaterialResponses(byLessonM[l.ID]),
			Exercises:      lessonExerciseResponses(byLessonE[l.ID]),
		})
	}
	return out, nil
}

// CreateLesson appends a lesson to a draft version.
func (s *Service) CreateLesson(ctx context.Context, sc authctx.Scope, versionID uuid.UUID, req LessonRequest) (*LessonResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return nil, err
	}
	var created *Lesson
	err := s.lessonWrite(ctx, sc, versionID, func(ctx context.Context) error {
		pos, err := s.repo.NextPosition(ctx, sc, versionID)
		if err != nil {
			return err
		}
		created = &Lesson{
			VersionID: versionID, Position: pos, Title: strings.TrimSpace(req.Title),
			Mode: req.mode(), Unit: optionalText(req.Unit),
			Objectives: req.Objectives, DurationMin: req.DurationMin, HomeworkNote: req.HomeworkNote,
		}
		return s.repo.CreateLessons(ctx, sc, []*Lesson{created})
	})
	if err != nil {
		return nil, err
	}
	l, err := s.repo.GetLesson(ctx, sc, created.ID)
	if err != nil {
		return nil, notFound(err, "template lesson")
	}
	out := lessonRowResponse(l)
	return &out, nil
}

// UpdateLesson replaces a lesson's content; its version must be a draft.
func (s *Service) UpdateLesson(ctx context.Context, sc authctx.Scope, lessonID uuid.UUID, req LessonRequest) (*LessonDetailResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return nil, err
	}
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		l, err := s.repo.GetLesson(ctx, sc, lessonID)
		if err != nil {
			return notFound(err, "template lesson")
		}
		return s.lessonWrite(ctx, sc, l.VersionID, func(ctx context.Context) error {
			l.Title = strings.TrimSpace(req.Title)
			l.Mode, l.Unit = req.mode(), optionalText(req.Unit)
			l.Objectives, l.DurationMin, l.HomeworkNote = req.Objectives, req.DurationMin, req.HomeworkNote
			return notFound(s.repo.UpdateLesson(ctx, sc, &l.Lesson), "template lesson")
		})
	})
	if err != nil {
		return nil, err
	}
	return s.GetLesson(ctx, sc, lessonID)
}

// DeleteLesson removes a lesson from a draft and closes the position gap.
func (s *Service) DeleteLesson(ctx context.Context, sc authctx.Scope, lessonID uuid.UUID) error {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return err
	}
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		l, err := s.repo.GetLesson(ctx, sc, lessonID)
		if err != nil {
			return notFound(err, "template lesson")
		}
		return s.lessonWrite(ctx, sc, l.VersionID, func(ctx context.Context) error {
			if err := s.repo.DeleteLesson(ctx, sc, lessonID); err != nil {
				return notFound(err, "template lesson")
			}
			rest, err := s.repo.ListLessons(ctx, sc, l.VersionID)
			if err != nil {
				return err
			}
			return s.repo.SetPositions(ctx, sc, l.VersionID, lessonIDs(rest))
		})
	})
}

// ReorderLessons applies a complete new order to a draft's lessons. The
// request must name every lesson of the version exactly once.
func (s *Service) ReorderLessons(ctx context.Context, sc authctx.Scope, versionID uuid.UUID, req ReorderRequest) ([]LessonResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return nil, err
	}
	err := s.lessonWrite(ctx, sc, versionID, func(ctx context.Context) error {
		current, err := s.repo.ListLessons(ctx, sc, versionID)
		if err != nil {
			return err
		}
		if !idset.Same(lessonIDs(current), req.LessonIDs) {
			return apperror.Invalid("Thứ tự phải liệt kê đúng mỗi buổi học mẫu của phiên bản một lần",
				map[string]string{"lesson_ids": "phải chứa đúng mọi buổi học của phiên bản, mỗi buổi một lần"})
		}
		return notFound(s.repo.SetPositions(ctx, sc, versionID, req.LessonIDs), "template lesson")
	})
	if err != nil {
		return nil, err
	}
	return s.ListLessons(ctx, sc, versionID)
}

// DuplicateLesson inserts a copy of a lesson right after it, shifting every
// later lesson's position up by one. The copy carries over content — mode,
// unit, objectives, duration, homework note, and its materials and exercise
// links including group_id — the same distinction copyVersionContent draws.
// Its version must be a draft.
func (s *Service) DuplicateLesson(ctx context.Context, sc authctx.Scope, lessonID uuid.UUID) (*DuplicateLessonResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return nil, err
	}
	var dupID uuid.UUID
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		src, err := s.repo.GetLesson(ctx, sc, lessonID)
		if err != nil {
			return notFound(err, "template lesson")
		}
		return s.lessonWrite(ctx, sc, src.VersionID, func(ctx context.Context) error {
			current, err := s.repo.ListLessons(ctx, sc, src.VersionID)
			if err != nil {
				return err
			}
			dup := &Lesson{
				VersionID: src.VersionID, Position: src.Position + 1, Title: duplicateTitle(src.Title),
				Mode: src.Mode, Unit: src.Unit, Objectives: src.Objectives,
				DurationMin: src.DurationMin, HomeworkNote: src.HomeworkNote,
			}
			if err := s.repo.CreateLessons(ctx, sc, []*Lesson{dup}); err != nil {
				return err
			}
			dupID = dup.ID
			order := make([]uuid.UUID, 0, len(current)+1)
			for _, l := range current {
				order = append(order, l.ID)
				if l.ID == lessonID {
					order = append(order, dup.ID)
				}
			}
			if err := s.repo.SetPositions(ctx, sc, src.VersionID, order); err != nil {
				return err
			}
			materials, err := s.repo.ListLessonMaterials(ctx, sc, []uuid.UUID{lessonID})
			if err != nil {
				return err
			}
			if len(materials) > 0 {
				links := make([]LessonMaterial, 0, len(materials))
				for _, m := range materials {
					links = append(links, LessonMaterial{
						LessonID: dup.ID, MaterialID: m.ID,
						SharedWithStudents: m.SharedWithStudents, Position: m.Position,
					})
				}
				if err := s.repo.CreateLessonMaterials(ctx, sc, links); err != nil {
					return err
				}
			}
			exercises, err := s.repo.ListLessonExercises(ctx, sc, []uuid.UUID{lessonID})
			if err != nil {
				return err
			}
			if len(exercises) > 0 {
				links := make([]LessonExercise, 0, len(exercises))
				for _, e := range exercises {
					links = append(links, LessonExercise{LessonID: dup.ID, ExerciseID: e.ID, GroupID: e.GroupID, Position: e.Position})
				}
				if err := s.repo.CreateLessonExercises(ctx, sc, links); err != nil {
					return err
				}
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	fresh, err := s.repo.GetLesson(ctx, sc, dupID)
	if err != nil {
		return nil, notFound(err, "template lesson")
	}
	out := lessonRowResponse(fresh)
	return &out, nil
}

// lessonTitleMaxLen matches LessonRequest.Title's max=200 validator tag
// (and the template_lessons.title column), counted in runes the same way
// go-playground's max tag counts a string.
const lessonTitleMaxLen = 200

// duplicateTitleSuffix is appended to a duplicated lesson's title.
const duplicateTitleSuffix = " (bản sao)"

// duplicateTitle builds a duplicated lesson's title, truncating the source
// title by runes so title+suffix never exceeds lessonTitleMaxLen — a title
// already near the column limit must not overflow it and 500 the request.
func duplicateTitle(title string) string {
	base := []rune(title)
	limit := lessonTitleMaxLen - len([]rune(duplicateTitleSuffix))
	if limit < 0 {
		limit = 0
	}
	if len(base) > limit {
		base = base[:limit]
	}
	return string(base) + duplicateTitleSuffix
}

// ClearLessons deletes every lesson of a draft version in one call — the
// join tables cascade. Published and archived versions refuse (409
// VERSION_LOCKED) the same way any other content write does.
func (s *Service) ClearLessons(ctx context.Context, sc authctx.Scope, versionID uuid.UUID) error {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return err
	}
	return s.lessonWrite(ctx, sc, versionID, func(ctx context.Context) error {
		return s.repo.ClearLessons(ctx, sc, versionID)
	})
}

// maxExerciseGroups bounds a version's exercise group list.
const maxExerciseGroups = 30

// ListExerciseGroups lists a version's exercise groups by position, each
// with how many lesson-exercise links point at it.
func (s *Service) ListExerciseGroups(ctx context.Context, sc authctx.Scope, versionID uuid.UUID) ([]ExerciseGroupResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryRead); err != nil {
		return nil, err
	}
	if _, err := s.repo.GetVersion(ctx, sc, versionID); err != nil {
		return nil, notFound(err, "template version")
	}
	rows, err := s.repo.ListExerciseGroups(ctx, sc, versionID)
	if err != nil {
		return nil, err
	}
	return exerciseGroupResponses(rows), nil
}

// CreateExerciseGroup adds a group at the end of the version's group list.
// The version must be a draft.
func (s *Service) CreateExerciseGroup(ctx context.Context, sc authctx.Scope, versionID uuid.UUID, req ExerciseGroupRequest) (*ExerciseGroupResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, apperror.Invalid("Nhóm bài tập phải có tên",
			map[string]string{"name": "không được để trống"})
	}
	var created *ExerciseGroup
	err := s.lessonWrite(ctx, sc, versionID, func(ctx context.Context) error {
		groups, err := s.repo.ListExerciseGroups(ctx, sc, versionID)
		if err != nil {
			return err
		}
		if len(groups) >= maxExerciseGroups {
			return apperror.Invalid("Tối đa 30 nhóm bài tập cho một phiên bản",
				map[string]string{"name": "đã đạt số nhóm tối đa"})
		}
		pos, err := s.repo.NextGroupPosition(ctx, sc, versionID)
		if err != nil {
			return err
		}
		created = &ExerciseGroup{VersionID: versionID, Name: name, Position: pos}
		return s.repo.CreateExerciseGroup(ctx, sc, created)
	})
	if err != nil {
		return nil, err
	}
	out := exerciseGroupResponse(&ExerciseGroupRow{ExerciseGroup: *created, ExerciseCount: 0})
	return &out, nil
}

// DeleteExerciseGroup removes a group from a draft version. The lesson-
// exercise links that pointed at it lose their group_id (set to NULL by the
// foreign key) instead of being deleted, so no exercise attachment is lost.
func (s *Service) DeleteExerciseGroup(ctx context.Context, sc authctx.Scope, versionID, groupID uuid.UUID) error {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return err
	}
	return s.lessonWrite(ctx, sc, versionID, func(ctx context.Context) error {
		return notFound(s.repo.DeleteExerciseGroup(ctx, sc, versionID, groupID), "exercise group")
	})
}

// lessonWrite runs fn inside a transaction that holds the version's row
// lock. The lock is what makes "published is immutable" hold under
// concurrency: a publish that lands first leaves fn unreached (409
// VERSION_LOCKED), one that lands second waits for this commit and then
// locks the content fn just wrote. It also serialises position
// assignment, so two authors adding lessons at once get consecutive
// numbers instead of a duplicate. A missing version, or one whose
// template was deleted, answers 404.
func (s *Service) lessonWrite(ctx context.Context, sc authctx.Scope, versionID uuid.UUID, fn func(ctx context.Context) error) error {
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		v, err := s.repo.LockVersion(ctx, sc, versionID)
		if err != nil {
			return notFound(err, "template version")
		}
		if v.Locked() {
			return errVersionLocked()
		}
		return fn(ctx)
	})
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		// The deferred unique on (version_id, position) fires at commit;
		// with the row lock this should not happen, but a 409 the client
		// can retry beats a 500.
		return apperror.Conflict("Thứ tự buổi học mẫu vừa thay đổi, hãy tải lại rồi thử lại")
	}
	return err
}

func (s *Service) version(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*VersionResponse, error) {
	row, err := s.repo.GetVersion(ctx, sc, id)
	if err != nil {
		return nil, notFound(err, "template version")
	}
	classes, err := s.repo.ListVersionClasses(ctx, sc, id)
	if err != nil {
		return nil, err
	}
	out := versionResponseWithClasses(row, classes)
	return &out, nil
}

// normalizeCode upper-cases and validates a template code against the
// shared class-code shape.
func normalizeCode(raw string) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(raw))
	if !classcode.Valid(code) {
		return "", apperror.Invalid("Mã chương trình mẫu không hợp lệ",
			map[string]string{"code": "chỉ gồm chữ in hoa, số và dấu gạch ngang, 2–20 ký tự"})
	}
	return code, nil
}

// optionalText trims a free-text field and stores blank as NULL, so a
// client that sends "" and one that omits the field write the same row.
func optionalText(v *string) *string {
	if v == nil {
		return nil
	}
	t := strings.TrimSpace(*v)
	if t == "" {
		return nil
	}
	return &t
}

func mapCodeClash(err error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return errCodeTaken()
	}
	return err
}

// exerciseCodePattern is the shape of a caller-supplied exercise code:
// letters, digits and dashes, no length minimum (an auto-generated code is
// always BT-0001-style, but a caller may label an exercise with anything
// short, e.g. a single letter).
var exerciseCodePattern = regexp.MustCompile(`^[A-Za-z0-9-]+$`)

// normalizeExerciseCode upper-cases and validates a caller-supplied exercise
// code. A blank or omitted one returns (nil, nil): CreateExercise then
// assigns the next generated code, and UpdateExercise keeps the exercise's
// current one, since the column is NOT NULL and must never be blanked.
func normalizeExerciseCode(raw *string) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	code := strings.ToUpper(strings.TrimSpace(*raw))
	if code == "" {
		return nil, nil
	}
	if !exerciseCodePattern.MatchString(code) {
		return nil, apperror.Invalid("Mã bài tập không hợp lệ",
			map[string]string{"code": "chỉ gồm chữ, số và dấu gạch ngang"})
	}
	return &code, nil
}

// mapExerciseCodeClash turns the partial unique index's violation into the
// 409 the web client branches on.
func mapExerciseCodeClash(err error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return errExerciseCodeTaken()
	}
	return err
}

// notFound maps the repository sentinel onto a 404 for the named resource
// and passes every other error through.
func notFound(err error, resource string) error {
	if errors.Is(err, ErrNotFound) {
		return apperror.NotFound(resource)
	}
	return err
}

func lessonIDs(rows []LessonRow) []uuid.UUID {
	ids := make([]uuid.UUID, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
	}
	return ids
}

// versionRefs projects a version listing down to the compact reference
// TemplateResponse.Versions carries.
func versionRefs(rows []VersionRow) []VersionRef {
	out := make([]VersionRef, len(rows))
	for i := range rows {
		out[i] = VersionRef{ID: rows[i].ID, VersionNo: rows[i].VersionNo, Status: rows[i].Status}
	}
	return out
}

// GetVersion returns a version with its score set, log fields and lessons
// including attachments.
func (s *Service) GetVersion(ctx context.Context, sc authctx.Scope, versionID uuid.UUID) (*VersionDetailResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryRead); err != nil {
		return nil, err
	}
	row, err := s.repo.GetVersion(ctx, sc, versionID)
	if err != nil {
		return nil, notFound(err, "template version")
	}
	classes, err := s.repo.ListVersionClasses(ctx, sc, versionID)
	if err != nil {
		return nil, err
	}
	fields, err := s.repo.ListLogFields(ctx, sc, versionID)
	if err != nil {
		return nil, err
	}
	lessons, err := s.repo.ListLessons(ctx, sc, versionID)
	if err != nil {
		return nil, err
	}
	details, err := s.lessonDetails(ctx, sc, lessons)
	if err != nil {
		return nil, err
	}
	return &VersionDetailResponse{
		VersionResponse: versionResponseWithClasses(row, classes),
		ScoreSet:        scoreSet(row.ScoreSet),
		LogFields:       logFieldResponses(fields),
		Lessons:         details,
	}, nil
}

// CreateMaterial adds a center-wide material.
func (s *Service) CreateMaterial(ctx context.Context, sc authctx.Scope, req MaterialRequest) (*MaterialResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return nil, err
	}
	if req.Kind == MaterialKindOther {
		return nil, apperror.Invalid("Không thể tạo học liệu mới với loại này",
			map[string]string{"kind": "loại này chỉ còn dùng cho học liệu cũ"})
	}
	m, err := materialFrom(req)
	if err != nil {
		return nil, err
	}
	if err := s.repo.CreateMaterial(ctx, sc, m); err != nil {
		return nil, err
	}
	return s.GetMaterial(ctx, sc, m.ID)
}

// GetMaterial returns one material.
func (s *Service) GetMaterial(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*MaterialResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryRead); err != nil {
		return nil, err
	}
	m, err := s.repo.GetMaterial(ctx, sc, id)
	if err != nil {
		return nil, notFound(err, "library material")
	}
	out := materialBankResponse(m)
	return &out, nil
}

// ListMaterials pages the center's live materials.
func (s *Service) ListMaterials(ctx context.Context, sc authctx.Scope, f ListFilter, p pagination.Params) ([]MaterialResponse, int64, error) {
	if err := authctx.Require(sc, authctx.PermLibraryRead); err != nil {
		return nil, 0, err
	}
	rows, total, err := s.repo.ListMaterials(ctx, sc, f, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]MaterialResponse, 0, len(rows))
	for i := range rows {
		out = append(out, materialBankResponse(&rows[i]))
	}
	return out, total, nil
}

// UpdateMaterial replaces every field of a material. Lessons linking it
// see the change at once: the link is by reference.
func (s *Service) UpdateMaterial(ctx context.Context, sc authctx.Scope, id uuid.UUID, req MaterialRequest) (*MaterialResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return nil, err
	}
	if req.Kind == MaterialKindOther {
		current, err := s.repo.GetMaterial(ctx, sc, id)
		if err != nil {
			return nil, notFound(err, "library material")
		}
		if current.Kind != MaterialKindOther {
			return nil, apperror.Invalid("Không thể đổi học liệu sang loại này",
				map[string]string{"kind": "loại này chỉ giữ lại cho học liệu cũ, không dùng để đổi sang"})
		}
	}
	m, err := materialFrom(req)
	if err != nil {
		return nil, err
	}
	m.ID = id
	if err := s.repo.UpdateMaterial(ctx, sc, m); err != nil {
		return nil, notFound(err, "library material")
	}
	return s.GetMaterial(ctx, sc, id)
}

// SetMaterialStatus toggles a material's active flag. An inactive material
// stays visible to a lesson that already links it (the link is untouched)
// and stays in the bank list by default (ListMaterials only narrows by
// active when the caller passes that filter); SetLessonMaterials is the
// only place that refuses it, for a new attachment.
func (s *Service) SetMaterialStatus(ctx context.Context, sc authctx.Scope, id uuid.UUID, active bool) (*MaterialResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return nil, err
	}
	if err := s.repo.SetMaterialActive(ctx, sc, id, active); err != nil {
		return nil, notFound(err, "library material")
	}
	return s.GetMaterial(ctx, sc, id)
}

// DeleteMaterial soft-deletes a material that no lesson of a live
// template links any more; 409 MATERIAL_IN_USE otherwise, whatever the
// version's status. Links kept by a deleted template do not count: they
// are unreachable, and the template cannot be restored.
func (s *Service) DeleteMaterial(ctx context.Context, sc authctx.Scope, id uuid.UUID) error {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return err
	}
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		// The row lock orders this delete against an attach in flight,
		// which holds the material shared until its link is committed.
		if _, err := s.repo.LockMaterial(ctx, sc, id); err != nil {
			return notFound(err, "library material")
		}
		u, err := s.repo.MaterialUsage(ctx, sc, id)
		if err != nil {
			return err
		}
		if u.Released > 0 {
			return errMaterialInReleased()
		}
		if u.Draft > 0 {
			return errMaterialInUse()
		}
		return notFound(s.repo.SoftDeleteMaterial(ctx, sc, id), "library material")
	})
}

// CreateExercise adds a center-wide exercise. A caller-supplied code is
// normalized and must be unique per center (409 EXERCISE_CODE_TAKEN); an
// omitted code is auto-generated as the next BT-0001-style code of the
// center. Auto-generation takes a transaction-scoped advisory lock on the
// center before reading NextExerciseCode, so two concurrent requests never
// read the same max and race for the same generated code — the second
// waits for the first's transaction to commit (or roll back) and then sees
// its inserted code in its own max.
func (s *Service) CreateExercise(ctx context.Context, sc authctx.Scope, req ExerciseRequest) (*ExerciseResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return nil, err
	}
	code, err := normalizeExerciseCode(req.Code)
	if err != nil {
		return nil, err
	}
	e := exerciseFrom(req)
	if code != nil {
		e.Code = *code
		if err := mapExerciseCodeClash(s.repo.CreateExercise(ctx, sc, e)); err != nil {
			return nil, err
		}
		return s.GetExercise(ctx, sc, e.ID)
	}
	createErr := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.repo.LockCenterForExerciseCode(ctx, sc); err != nil {
			return err
		}
		next, err := s.repo.NextExerciseCode(ctx, sc)
		if err != nil {
			return err
		}
		e.Code = next
		return s.repo.CreateExercise(ctx, sc, e)
	})
	if err := mapExerciseCodeClash(createErr); err != nil {
		return nil, err
	}
	return s.GetExercise(ctx, sc, e.ID)
}

// GetExercise returns one exercise.
func (s *Service) GetExercise(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*ExerciseResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryRead); err != nil {
		return nil, err
	}
	e, err := s.repo.GetExercise(ctx, sc, id)
	if err != nil {
		return nil, notFound(err, "library exercise")
	}
	out := exerciseBankResponse(e)
	return &out, nil
}

// ListExercises pages the center's live exercises.
func (s *Service) ListExercises(ctx context.Context, sc authctx.Scope, f ListFilter, p pagination.Params) ([]ExerciseResponse, int64, error) {
	if err := authctx.Require(sc, authctx.PermLibraryRead); err != nil {
		return nil, 0, err
	}
	rows, total, err := s.repo.ListExercises(ctx, sc, f, p)
	if err != nil {
		return nil, 0, err
	}
	out := make([]ExerciseResponse, 0, len(rows))
	for i := range rows {
		out = append(out, exerciseBankResponse(&rows[i]))
	}
	return out, total, nil
}

// UpdateExercise replaces every field of an exercise. Code is optional like
// the rest of the body: given, it is normalized and validated as on create
// (409 EXERCISE_CODE_TAKEN on a clash); omitted, the exercise keeps its
// current code, since the column is NOT NULL and a full-replace body must
// not blank it.
func (s *Service) UpdateExercise(ctx context.Context, sc authctx.Scope, id uuid.UUID, req ExerciseRequest) (*ExerciseResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return nil, err
	}
	code, err := normalizeExerciseCode(req.Code)
	if err != nil {
		return nil, err
	}
	e := exerciseFrom(req)
	e.ID = id
	if code != nil {
		e.Code = *code
	} else {
		current, err := s.repo.GetExercise(ctx, sc, id)
		if err != nil {
			return nil, notFound(err, "library exercise")
		}
		e.Code = current.Code
	}
	if err := notFound(mapExerciseCodeClash(s.repo.UpdateExercise(ctx, sc, e)), "library exercise"); err != nil {
		return nil, err
	}
	return s.GetExercise(ctx, sc, id)
}

// SetExerciseStatus toggles an exercise's active flag, under the same rules
// as SetMaterialStatus.
func (s *Service) SetExerciseStatus(ctx context.Context, sc authctx.Scope, id uuid.UUID, active bool) (*ExerciseResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return nil, err
	}
	if err := s.repo.SetExerciseActive(ctx, sc, id, active); err != nil {
		return nil, notFound(err, "library exercise")
	}
	return s.GetExercise(ctx, sc, id)
}

// DeleteExercise soft-deletes an exercise no lesson of a live template
// links any more; 409 EXERCISE_IN_USE otherwise, like DeleteMaterial.
func (s *Service) DeleteExercise(ctx context.Context, sc authctx.Scope, id uuid.UUID) error {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return err
	}
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := s.repo.LockExercise(ctx, sc, id); err != nil {
			return notFound(err, "library exercise")
		}
		u, err := s.repo.ExerciseUsage(ctx, sc, id)
		if err != nil {
			return err
		}
		if u.Released > 0 {
			return errExerciseInReleased()
		}
		if u.Draft > 0 {
			return errExerciseInUse()
		}
		return notFound(s.repo.SoftDeleteExercise(ctx, sc, id), "library exercise")
	})
}

// maxLessonItems bounds the material and exercise lists of one lesson.
const maxLessonItems = 100

// SetLessonMaterials replaces the lesson's material list with items, in
// body order. Every id must be a live material of the center, each at
// most once (422 otherwise); the lesson's version must be a draft.
func (s *Service) SetLessonMaterials(ctx context.Context, sc authctx.Scope, lessonID uuid.UUID, items []LessonMaterialInput) ([]LessonMaterialResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return nil, err
	}
	if len(items) > maxLessonItems {
		return nil, apperror.Invalid("Tối đa 100 học liệu cho một buổi học mẫu",
			map[string]string{"materials": "tối đa 100 học liệu"})
	}
	ids := make([]uuid.UUID, 0, len(items))
	links := make([]LessonMaterial, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.MaterialID)
		links = append(links, LessonMaterial{MaterialID: it.MaterialID, SharedWithStudents: it.SharedWithStudents})
	}
	if err := uniqueIDs(ids, "material_id", "Mỗi học liệu chỉ gắn vào buổi một lần"); err != nil {
		return nil, err
	}
	var out []LessonMaterialResponse
	err := s.lessonLinkWrite(ctx, sc, lessonID, func(ctx context.Context, _ uuid.UUID) error {
		found, err := s.repo.FindMaterials(ctx, sc, ids)
		if err != nil {
			return err
		}
		if len(found) != len(ids) {
			return apperror.Invalid("Có học liệu không tồn tại trong kho của trung tâm",
				map[string]string{"material_id": "phải là học liệu còn hiệu lực của trung tâm"})
		}
		// An inactive material may stay attached to a lesson that already
		// carries it, but a new attachment must not pick one up: compare
		// against what the lesson already links before overwriting the list.
		already, err := s.repo.ListLessonMaterials(ctx, sc, []uuid.UUID{lessonID})
		if err != nil {
			return err
		}
		attached := make(map[uuid.UUID]bool, len(already))
		for _, row := range already {
			attached[row.ID] = true
		}
		for _, m := range found {
			if !m.Active && !attached[m.ID] {
				return apperror.Invalid("Nội dung đã ngừng hoạt động",
					map[string]string{"material_id": "đã ngừng hoạt động"})
			}
		}
		if err := s.repo.ReplaceLessonMaterials(ctx, sc, lessonID, links); err != nil {
			return err
		}
		rows, err := s.repo.ListLessonMaterials(ctx, sc, []uuid.UUID{lessonID})
		if err != nil {
			return err
		}
		out = lessonMaterialResponses(rows)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// SetLessonExercises replaces the lesson's exercise list with items, in
// body order, under the same rules as SetLessonMaterials.
func (s *Service) SetLessonExercises(ctx context.Context, sc authctx.Scope, lessonID uuid.UUID, items []LessonExerciseInput) ([]LessonExerciseResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return nil, err
	}
	if len(items) > maxLessonItems {
		return nil, apperror.Invalid("Tối đa 100 bài tập cho một buổi học mẫu",
			map[string]string{"exercises": "tối đa 100 bài tập"})
	}
	ids := make([]uuid.UUID, 0, len(items))
	links := make([]LessonExercise, 0, len(items))
	groupIDSet := make(map[uuid.UUID]bool)
	for _, it := range items {
		ids = append(ids, it.ExerciseID)
		links = append(links, LessonExercise{ExerciseID: it.ExerciseID, GroupID: it.GroupID})
		if it.GroupID != nil {
			groupIDSet[*it.GroupID] = true
		}
	}
	if err := uniqueIDs(ids, "exercise_id", "Mỗi bài tập chỉ gắn vào buổi một lần"); err != nil {
		return nil, err
	}
	var out []LessonExerciseResponse
	err := s.lessonLinkWrite(ctx, sc, lessonID, func(ctx context.Context, versionID uuid.UUID) error {
		found, err := s.repo.FindExercises(ctx, sc, ids)
		if err != nil {
			return err
		}
		if len(found) != len(ids) {
			return apperror.Invalid("Có bài tập không tồn tại trong kho của trung tâm",
				map[string]string{"exercise_id": "phải là bài tập còn hiệu lực của trung tâm"})
		}
		if len(groupIDSet) > 0 {
			groupIDs := make([]uuid.UUID, 0, len(groupIDSet))
			for id := range groupIDSet {
				groupIDs = append(groupIDs, id)
			}
			foundGroups, err := s.repo.FilterExistingGroupIDs(ctx, sc, versionID, groupIDs)
			if err != nil {
				return err
			}
			if len(foundGroups) != len(groupIDs) {
				return apperror.Invalid("Có nhóm bài tập không thuộc phiên bản này",
					map[string]string{"group_id": "phải là nhóm bài tập của phiên bản"})
			}
		}
		// Same inactive-attachment rule as SetLessonMaterials.
		already, err := s.repo.ListLessonExercises(ctx, sc, []uuid.UUID{lessonID})
		if err != nil {
			return err
		}
		attached := make(map[uuid.UUID]bool, len(already))
		for _, row := range already {
			attached[row.ID] = true
		}
		for _, e := range found {
			if !e.Active && !attached[e.ID] {
				return apperror.Invalid("Nội dung đã ngừng hoạt động",
					map[string]string{"exercise_id": "đã ngừng hoạt động"})
			}
		}
		if err := s.repo.ReplaceLessonExercises(ctx, sc, lessonID, links); err != nil {
			return err
		}
		rows, err := s.repo.ListLessonExercises(ctx, sc, []uuid.UUID{lessonID})
		if err != nil {
			return err
		}
		out = lessonExerciseResponses(rows)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// lessonLinkWrite resolves the lesson, then runs fn under its version's
// draft lock like every other content write. The lesson is read again
// once the lock is held: a delete committed in between would otherwise
// surface as a foreign-key failure on the link insert instead of a 404. fn
// receives the resolved version id, so a caller that must validate a
// version-scoped reference (an exercise group, say) does not need a second
// lookup.
func (s *Service) lessonLinkWrite(ctx context.Context, sc authctx.Scope, lessonID uuid.UUID, fn func(ctx context.Context, versionID uuid.UUID) error) error {
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		l, err := s.repo.GetLesson(ctx, sc, lessonID)
		if err != nil {
			return notFound(err, "template lesson")
		}
		return s.lessonWrite(ctx, sc, l.VersionID, func(ctx context.Context) error {
			if _, err := s.repo.GetLesson(ctx, sc, lessonID); err != nil {
				return notFound(err, "template lesson")
			}
			return fn(ctx, l.VersionID)
		})
	})
}

// maxLogFields and maxScoreComponents bound the wholesale-replace bodies.
const (
	maxLogFields       = 30
	maxScoreComponents = 20
)

// SetLogFields replaces the version's session-log fields with items, in
// body order. A select field needs at least one option; other kinds
// store none. The version must be a draft.
func (s *Service) SetLogFields(ctx context.Context, sc authctx.Scope, versionID uuid.UUID, items []LogFieldInput) ([]LogFieldResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return nil, err
	}
	if len(items) > maxLogFields {
		return nil, apperror.Invalid("Tối đa 30 trường nhật ký cho một phiên bản",
			map[string]string{"log_fields": "tối đa 30 trường"})
	}
	rows := make([]*LogField, 0, len(items))
	for i, it := range items {
		label := strings.TrimSpace(it.Label)
		if label == "" {
			return nil, apperror.Invalid("Trường nhật ký phải có nhãn",
				map[string]string{fieldPath(i, "label"): "không được để trống"})
		}
		options := dbtypes.StringList{}
		if it.Kind == LogFieldSelect {
			options = cleanStrings(it.Options)
			if len(options) == 0 {
				return nil, apperror.Invalid("Trường dạng chọn phải có ít nhất một lựa chọn",
					map[string]string{fieldPath(i, "options"): "cần ít nhất một lựa chọn"})
			}
		}
		rows = append(rows, &LogField{Label: label, Kind: it.Kind, Options: options, Required: it.Required})
	}
	var out []LogFieldResponse
	err := s.lessonWrite(ctx, sc, versionID, func(ctx context.Context) error {
		if err := s.repo.ReplaceLogFields(ctx, sc, versionID, rows); err != nil {
			return err
		}
		saved, err := s.repo.ListLogFields(ctx, sc, versionID)
		if err != nil {
			return err
		}
		out = logFieldResponses(saved)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// scoreKey is the shape of a score set or score component key: a stable
// identifier the class grading can refer to.
var scoreKey = regexp.MustCompile(`^[a-z0-9_]{1,30}$`)

// maxScoreGroups bounds the score set's own group list; maxScoreComponents
// (declared above with the log fields' bound) applies to each group's
// component list.
const maxScoreGroups = 10

// SetScoreSet replaces the version's score set with items, in body order.
// A group key must match scoreKey and be unique across the set; a
// component key must match scoreKey and be unique within its own group
// only, so two different groups may each declare the same component key.
// max must be positive and weight non-negative. The version must be a
// draft.
func (s *Service) SetScoreSet(ctx context.Context, sc authctx.Scope, versionID uuid.UUID, items []ScoreSetGroupInput) ([]ScoreSetGroup, error) {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return nil, err
	}
	if len(items) > maxScoreGroups {
		return nil, apperror.Invalid("Tối đa 10 nhóm điểm cho một phiên bản",
			map[string]string{"score_set": "tối đa 10 nhóm"})
	}
	set := make(ScoreSet, 0, len(items))
	seenGroups := make(map[string]bool, len(items))
	for gi, g := range items {
		key := strings.TrimSpace(g.Key)
		switch {
		case !scoreKey.MatchString(key):
			return nil, apperror.Invalid("Khoá nhóm điểm không hợp lệ",
				map[string]string{fieldPath(gi, "key"): "chỉ gồm chữ thường, số và dấu gạch dưới, tối đa 30 ký tự"})
		case seenGroups[key]:
			return nil, apperror.Invalid("Khoá nhóm điểm bị trùng",
				map[string]string{fieldPath(gi, "key"): "mỗi khoá chỉ dùng một lần"})
		}
		title := strings.TrimSpace(g.Title)
		if title == "" {
			return nil, apperror.Invalid("Nhóm điểm phải có tên",
				map[string]string{fieldPath(gi, "title"): "không được để trống"})
		}
		if len(g.Components) > maxScoreComponents {
			return nil, apperror.Invalid("Tối đa 20 thành phần điểm cho một nhóm",
				map[string]string{fieldPath(gi, "components"): "tối đa 20 thành phần"})
		}
		seenGroups[key] = true
		components := make([]ScoreComponent, 0, len(g.Components))
		seenComponents := make(map[string]bool, len(g.Components))
		for ci, it := range g.Components {
			path := fieldPath(gi, "components."+strconv.Itoa(ci))
			ckey := strings.TrimSpace(it.Key)
			switch {
			case !scoreKey.MatchString(ckey):
				return nil, apperror.Invalid("Khoá thành phần điểm không hợp lệ",
					map[string]string{path + ".key": "chỉ gồm chữ thường, số và dấu gạch dưới, tối đa 30 ký tự"})
			case seenComponents[ckey]:
				return nil, apperror.Invalid("Khoá thành phần điểm bị trùng trong nhóm",
					map[string]string{path + ".key": "mỗi khoá chỉ dùng một lần trong nhóm"})
			case it.Max <= 0:
				return nil, apperror.Invalid("Điểm tối đa phải lớn hơn 0",
					map[string]string{path + ".max": "phải lớn hơn 0"})
			case it.Weight < 0:
				return nil, apperror.Invalid("Trọng số không được âm",
					map[string]string{path + ".weight": "phải từ 0 trở lên"})
			}
			label := strings.TrimSpace(it.Label)
			if label == "" {
				return nil, apperror.Invalid("Thành phần điểm phải có tên",
					map[string]string{path + ".label": "không được để trống"})
			}
			seenComponents[ckey] = true
			components = append(components, ScoreComponent{Key: ckey, Label: label, Max: it.Max, Weight: it.Weight})
		}
		set = append(set, ScoreSetGroup{Key: key, Title: title, Components: components})
	}
	err := s.lessonWrite(ctx, sc, versionID, func(ctx context.Context) error {
		return notFound(s.repo.SetScoreSet(ctx, sc, versionID, set), "template version")
	})
	if err != nil {
		return nil, err
	}
	return scoreSet(set), nil
}

func materialFrom(req MaterialRequest) (*Material, error) {
	url, err := materialURL(req.URL)
	if err != nil {
		return nil, err
	}
	return &Material{
		Title: strings.TrimSpace(req.Title), Kind: req.Kind,
		URL: url, Description: optionalText(req.Description),
		Tags: cleanStrings(req.Tags), Active: true,
	}, nil
}

// materialURL trims the optional link and accepts only an absolute
// http(s) one: the value is rendered as an href for teachers and, when
// shared, for students, so a javascript: or data: scheme must not get in.
func materialURL(raw *string) (*string, error) {
	v := optionalText(raw)
	if v == nil {
		return nil, nil
	}
	u, err := neturl.Parse(*v)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, apperror.Invalid("Đường dẫn học liệu phải là liên kết http:// hoặc https://",
			map[string]string{"url": "phải bắt đầu bằng http:// hoặc https://"})
	}
	return v, nil
}

// exerciseFrom builds the row from the request; the caller assigns Code
// separately (it depends on normalization and, on update, the existing row).
func exerciseFrom(req ExerciseRequest) *Exercise {
	return &Exercise{
		Title: strings.TrimSpace(req.Title), Description: optionalText(req.Description),
		Difficulty: req.Difficulty, Skill: optionalText(req.Skill), Level: optionalText(req.Level),
		Tags: cleanStrings(req.Tags), Active: true,
	}
}

// cleanStrings trims each entry, drops blanks and duplicates, and keeps
// the first occurrence's order. The result is never nil.
func cleanStrings(in []string) dbtypes.StringList {
	out := make(dbtypes.StringList, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, raw := range in {
		v := strings.TrimSpace(raw)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// scoreSet returns a non-nil slice so the wire form is always an array.
func scoreSet(set ScoreSet) []ScoreSetGroup {
	if set == nil {
		return []ScoreSetGroup{}
	}
	return set
}

// uniqueIDs answers 422 on the named field when an id repeats.
func uniqueIDs(ids []uuid.UUID, field, msg string) error {
	seen := make(map[uuid.UUID]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			return apperror.Invalid(msg, map[string]string{field: "bị trùng"})
		}
		seen[id] = true
	}
	return nil
}

// fieldPath names one field of the i-th body entry, the way clients
// address list errors.
func fieldPath(i int, field string) string {
	return strconv.Itoa(i) + "." + field
}
