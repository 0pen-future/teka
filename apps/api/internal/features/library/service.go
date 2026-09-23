package library

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/classcode"
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
		return s.repo.CreateVersion(ctx, sc, &Version{
			TemplateID: tpl.ID, VersionNo: 1, Status: StatusDraft, CreatedBy: &sc.TeacherID,
		})
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
	return notFound(s.repo.SoftDeleteTemplate(ctx, sc, id), "program template")
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
		return s.repo.CreateLessons(ctx, sc, copies)
	})
	if err != nil {
		return nil, err
	}
	return s.version(ctx, sc, draft.ID)
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

// GetLesson returns one lesson.
func (s *Service) GetLesson(ctx context.Context, sc authctx.Scope, lessonID uuid.UUID) (*LessonResponse, error) {
	if err := authctx.Require(sc, authctx.PermLibraryRead); err != nil {
		return nil, err
	}
	l, err := s.repo.GetLesson(ctx, sc, lessonID)
	if err != nil {
		return nil, notFound(err, "template lesson")
	}
	out := lessonResponse(l)
	return &out, nil
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
	return s.GetLesson(ctx, sc, created.ID)
}

// UpdateLesson replaces a lesson's content; its version must be a draft.
func (s *Service) UpdateLesson(ctx context.Context, sc authctx.Scope, lessonID uuid.UUID, req LessonRequest) (*LessonResponse, error) {
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
		if !sameIDSet(lessonIDs(current), req.LessonIDs) {
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

// sameIDSet reports whether want and got hold the same ids, each exactly
// once.
func sameIDSet(want, got []uuid.UUID) bool {
	if len(want) != len(got) {
		return false
	}
	seen := make(map[uuid.UUID]bool, len(want))
	for _, id := range want {
		seen[id] = true
	}
	for _, id := range got {
		if !seen[id] {
			return false
		}
		delete(seen, id)
	}
	return len(seen) == 0
}
