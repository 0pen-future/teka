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
		// Seed placeholder lessons so the preparation board has cards
		// before anyone writes the content.
		rows := make([]*Lesson, 0, *req.LessonCount)
		for i := 1; i <= *req.LessonCount; i++ {
			rows = append(rows, &Lesson{VersionID: draft.ID, Position: i, Title: "Buổi " + strconv.Itoa(i), PrepStatus: PrepTodo})
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
	out := make([]TemplateResponse, 0, len(rows))
	for i := range rows {
		out = append(out, templateResponse(&rows[i]))
	}
	return out, total, nil
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
// in place for anything still bound to them.
func (s *Service) DeleteTemplate(ctx context.Context, sc authctx.Scope, id uuid.UUID) error {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return err
	}
	inUse, err := s.repo.TemplateInUse(ctx, sc, id)
	if err != nil {
		return err
	}
	if inUse {
		return errTemplateInUse()
	}
	return notFound(s.repo.SoftDeleteTemplate(ctx, sc, id), "program template")
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
// version carries: lessons with their material and exercise links, the
// log fields and the score set. Items are shared by reference (the same
// center-wide material row), lessons and log fields are new rows.
func (s *Service) copyVersionContent(ctx context.Context, sc authctx.Scope, source, draft *Version) error {
	lessons, err := s.repo.ListLessons(ctx, sc, source.ID)
	if err != nil {
		return err
	}
	copies := make([]*Lesson, 0, len(lessons))
	for i := range lessons {
		l := lessons[i]
		copies = append(copies, &Lesson{
			VersionID: draft.ID, Position: l.Position, Title: l.Title,
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
			eLinks = append(eLinks, LessonExercise{LessonID: copyOf[row.LessonID], ExerciseID: row.ID, Position: row.Position})
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
	out := make([]VersionResponse, 0, len(rows))
	for i := range rows {
		out = append(out, versionResponse(&rows[i]))
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
	details, err := s.lessonDetails(ctx, sc, []Lesson{*l})
	if err != nil {
		return nil, err
	}
	return &details[0], nil
}

// lessonDetails decorates lessons with their attachments in two bulk
// reads, so a version detail costs the same number of queries as one lesson.
func (s *Service) lessonDetails(ctx context.Context, sc authctx.Scope, lessons []Lesson) ([]LessonDetailResponse, error) {
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
			LessonResponse: lessonResponse(l),
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
	out := lessonResponse(l)
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
			l.Objectives, l.DurationMin, l.HomeworkNote = req.Objectives, req.DurationMin, req.HomeworkNote
			return notFound(s.repo.UpdateLesson(ctx, sc, l), "template lesson")
		})
	})
	if err != nil {
		return nil, err
	}
	return s.GetLesson(ctx, sc, lessonID)
}

// UpdateLessonPrep changes the preparation status and/or checklist of a
// draft lesson; both are optional and an omitted one keeps its value.
func (s *Service) UpdateLessonPrep(ctx context.Context, sc authctx.Scope, lessonID uuid.UUID, req PrepRequest) (*LessonResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return nil, err
	}
	if req.PrepStatus != nil && !validPrepStatus(*req.PrepStatus) {
		return nil, apperror.Invalid("Trạng thái chuẩn bị không hợp lệ",
			map[string]string{"prep_status": "phải là todo, doing, review hoặc done"})
	}
	return s.prepWrite(ctx, sc, lessonID, func(l *Lesson) error {
		if req.PrepStatus != nil {
			l.PrepStatus = *req.PrepStatus
		}
		if req.Checklist != nil {
			l.Checklist = Checklist(*req.Checklist)
		}
		if l.Checklist == nil {
			l.Checklist = Checklist{}
		}
		return s.repo.UpdateLessonPrep(ctx, sc, l)
	})
}

// UpdateLessonAssignment replaces the assignee and due date of a draft
// lesson. The assignee must currently be a member of the center.
func (s *Service) UpdateLessonAssignment(ctx context.Context, sc authctx.Scope, lessonID uuid.UUID, req AssignmentRequest) (*LessonResponse, error) {
	if err := authctx.Require(sc, authctx.PermPrepAssign); err != nil {
		return nil, err
	}
	var due *time.Time
	if req.DueDate != nil {
		d, err := time.Parse(dueDateLayout, *req.DueDate)
		if err != nil {
			return nil, apperror.Invalid("Hạn chuẩn bị không hợp lệ",
				map[string]string{"due_date": "phải có dạng YYYY-MM-DD"})
		}
		due = &d
	}
	if req.AssigneeID != nil {
		ok, err := s.repo.IsLiveMember(ctx, sc, *req.AssigneeID)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, apperror.Invalid("Người được phân công không còn là thành viên của trung tâm",
				map[string]string{"assignee_id": "phải là thành viên hiện tại của trung tâm"})
		}
	}
	return s.prepWrite(ctx, sc, lessonID, func(l *Lesson) error {
		l.AssigneeID, l.DueDate = req.AssigneeID, due
		return s.repo.UpdateLessonAssignment(ctx, sc, l)
	})
}

// prepWrite loads the lesson, locks its version like any content write and
// hands the row to fn, which persists its change; the fresh row is returned.
func (s *Service) prepWrite(ctx context.Context, sc authctx.Scope, lessonID uuid.UUID, fn func(l *Lesson) error) (*LessonResponse, error) {
	var out *LessonResponse
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		l, err := s.repo.GetLesson(ctx, sc, lessonID)
		if err != nil {
			return notFound(err, "template lesson")
		}
		return s.lessonWrite(ctx, sc, l.VersionID, func(ctx context.Context) error {
			if err := notFound(fn(l), "template lesson"); err != nil {
				return err
			}
			fresh, err := s.repo.GetLesson(ctx, sc, lessonID)
			if err != nil {
				return notFound(err, "template lesson")
			}
			resp := lessonResponse(fresh)
			out = &resp
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func validPrepStatus(status string) bool {
	for _, s := range PrepStatuses {
		if s == status {
			return true
		}
	}
	return false
}

// GetBoard returns the preparation board of a version: its lessons grouped
// into the four fixed status columns. Locked versions stay readable.
func (s *Service) GetBoard(ctx context.Context, sc authctx.Scope, versionID uuid.UUID) (*BoardResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryRead); err != nil {
		return nil, err
	}
	version, err := s.version(ctx, sc, versionID)
	if err != nil {
		return nil, err
	}
	tpl, err := s.repo.GetTemplate(ctx, sc, version.TemplateID)
	if err != nil {
		return nil, notFound(err, "program template")
	}
	cards, err := s.repo.ListBoardCards(ctx, sc, versionID)
	if err != nil {
		return nil, err
	}
	return boardResponse(templateResponse(tpl), *version, cards), nil
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
	out := versionResponse(row)
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

// notFound maps the repository sentinel onto a 404 for the named resource
// and passes every other error through.
func notFound(err error, resource string) error {
	if errors.Is(err, ErrNotFound) {
		return apperror.NotFound(resource)
	}
	return err
}

func lessonIDs(rows []Lesson) []uuid.UUID {
	ids := make([]uuid.UUID, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
	}
	return ids
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
		VersionResponse: versionResponse(row),
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
	out := materialResponse(m)
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
		out = append(out, materialResponse(&rows[i]))
	}
	return out, total, nil
}

// UpdateMaterial replaces every field of a material. Lessons linking it
// see the change at once: the link is by reference.
func (s *Service) UpdateMaterial(ctx context.Context, sc authctx.Scope, id uuid.UUID, req MaterialRequest) (*MaterialResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return nil, err
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

// CreateExercise adds a center-wide exercise.
func (s *Service) CreateExercise(ctx context.Context, sc authctx.Scope, req ExerciseRequest) (*ExerciseResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return nil, err
	}
	e := exerciseFrom(req)
	if err := s.repo.CreateExercise(ctx, sc, e); err != nil {
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
	out := exerciseResponse(e)
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
		out = append(out, exerciseResponse(&rows[i]))
	}
	return out, total, nil
}

// UpdateExercise replaces every field of an exercise.
func (s *Service) UpdateExercise(ctx context.Context, sc authctx.Scope, id uuid.UUID, req ExerciseRequest) (*ExerciseResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return nil, err
	}
	e := exerciseFrom(req)
	e.ID = id
	if err := s.repo.UpdateExercise(ctx, sc, e); err != nil {
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
	err := s.lessonLinkWrite(ctx, sc, lessonID, func(ctx context.Context) error {
		found, err := s.repo.FindMaterials(ctx, sc, ids)
		if err != nil {
			return err
		}
		if len(found) != len(ids) {
			return apperror.Invalid("Có học liệu không tồn tại trong kho của trung tâm",
				map[string]string{"material_id": "phải là học liệu còn hiệu lực của trung tâm"})
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
	for _, it := range items {
		ids = append(ids, it.ExerciseID)
		links = append(links, LessonExercise{ExerciseID: it.ExerciseID})
	}
	if err := uniqueIDs(ids, "exercise_id", "Mỗi bài tập chỉ gắn vào buổi một lần"); err != nil {
		return nil, err
	}
	var out []LessonExerciseResponse
	err := s.lessonLinkWrite(ctx, sc, lessonID, func(ctx context.Context) error {
		found, err := s.repo.FindExercises(ctx, sc, ids)
		if err != nil {
			return err
		}
		if len(found) != len(ids) {
			return apperror.Invalid("Có bài tập không tồn tại trong kho của trung tâm",
				map[string]string{"exercise_id": "phải là bài tập còn hiệu lực của trung tâm"})
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
// surface as a foreign-key failure on the link insert instead of a 404.
func (s *Service) lessonLinkWrite(ctx context.Context, sc authctx.Scope, lessonID uuid.UUID, fn func(ctx context.Context) error) error {
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		l, err := s.repo.GetLesson(ctx, sc, lessonID)
		if err != nil {
			return notFound(err, "template lesson")
		}
		return s.lessonWrite(ctx, sc, l.VersionID, func(ctx context.Context) error {
			if _, err := s.repo.GetLesson(ctx, sc, lessonID); err != nil {
				return notFound(err, "template lesson")
			}
			return fn(ctx)
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

// scoreKey is the shape of a score component key: a stable identifier the
// class grading can refer to.
var scoreKey = regexp.MustCompile(`^[a-z0-9_]{1,30}$`)

// SetScoreSet replaces the version's score components with items, in body
// order. Keys must match scoreKey and be unique; max must be positive and
// weight non-negative. The version must be a draft.
func (s *Service) SetScoreSet(ctx context.Context, sc authctx.Scope, versionID uuid.UUID, items []ScoreComponentInput) ([]ScoreComponent, error) {
	if err := authctx.Require(sc, authctx.PermLibraryEdit); err != nil {
		return nil, err
	}
	if len(items) > maxScoreComponents {
		return nil, apperror.Invalid("Tối đa 20 thành phần điểm cho một phiên bản",
			map[string]string{"score_set": "tối đa 20 thành phần"})
	}
	set := make(ScoreSet, 0, len(items))
	seen := make(map[string]bool, len(items))
	for i, it := range items {
		key := strings.TrimSpace(it.Key)
		switch {
		case !scoreKey.MatchString(key):
			return nil, apperror.Invalid("Khoá thành phần điểm không hợp lệ",
				map[string]string{fieldPath(i, "key"): "chỉ gồm chữ thường, số và dấu gạch dưới, tối đa 30 ký tự"})
		case seen[key]:
			return nil, apperror.Invalid("Khoá thành phần điểm bị trùng",
				map[string]string{fieldPath(i, "key"): "mỗi khoá chỉ dùng một lần"})
		case it.Max <= 0:
			return nil, apperror.Invalid("Điểm tối đa phải lớn hơn 0",
				map[string]string{fieldPath(i, "max"): "phải lớn hơn 0"})
		case it.Weight < 0:
			return nil, apperror.Invalid("Trọng số không được âm",
				map[string]string{fieldPath(i, "weight"): "phải từ 0 trở lên"})
		}
		label := strings.TrimSpace(it.Label)
		if label == "" {
			return nil, apperror.Invalid("Thành phần điểm phải có tên",
				map[string]string{fieldPath(i, "label"): "không được để trống"})
		}
		seen[key] = true
		set = append(set, ScoreComponent{Key: key, Label: label, Max: it.Max, Weight: it.Weight})
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
		Tags: cleanStrings(req.Tags),
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

func exerciseFrom(req ExerciseRequest) *Exercise {
	return &Exercise{
		Title: strings.TrimSpace(req.Title), Description: optionalText(req.Description),
		Difficulty: req.Difficulty, Tags: cleanStrings(req.Tags),
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
func scoreSet(set ScoreSet) []ScoreComponent {
	if set == nil {
		return []ScoreComponent{}
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
