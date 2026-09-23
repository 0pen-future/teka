package courses

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/library"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/classcode"
	"teka/apps/api/internal/shared/pagination"
)

// maxTuitionPacks bounds one course's pack list.
const maxTuitionPacks = 20

// Service implements the catalog rules: codes are unique among live
// courses, the default template must be a published version of the
// center's library, a course with live classes can be archived but not
// deleted. Authorization is checked here as well as by the route policy so
// callers that bypass HTTP get the same 403.
type Service struct {
	repo Repository
	tx   database.TxManager
}

// NewService builds the service.
func NewService(repo Repository, tx database.TxManager) *Service {
	return &Service{repo: repo, tx: tx}
}

// Create inserts a course.
func (s *Service) Create(ctx context.Context, sc authctx.Scope, req CourseRequest) (*CourseResponse, error) {
	if err := authctx.Require(sc, authctx.PermCoursesEdit); err != nil {
		return nil, err
	}
	course, err := s.fromRequest(ctx, sc, req, nil)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, sc, course); err != nil {
		return nil, mapCodeClash(err)
	}
	return s.Get(ctx, sc, course.ID)
}

// Get returns one course with its counters and tuition packs.
func (s *Service) Get(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*CourseResponse, error) {
	if err := authctx.Require(sc, authctx.PermCoursesRead); err != nil {
		return nil, err
	}
	row, err := s.repo.Get(ctx, sc, id)
	if err != nil {
		return nil, notFound(err)
	}
	packs, err := s.repo.ListPacks(ctx, sc, []uuid.UUID{id})
	if err != nil {
		return nil, err
	}
	out := courseResponse(row, packs)
	return &out, nil
}

// List pages the center's live courses.
func (s *Service) List(ctx context.Context, sc authctx.Scope, f ListFilter, p pagination.Params) ([]CourseResponse, int64, error) {
	if err := authctx.Require(sc, authctx.PermCoursesRead); err != nil {
		return nil, 0, err
	}
	rows, total, err := s.repo.List(ctx, sc, f, p)
	if err != nil {
		return nil, 0, err
	}
	ids := make([]uuid.UUID, len(rows))
	for i := range rows {
		ids[i] = rows[i].ID
	}
	packs, err := s.repo.ListPacks(ctx, sc, ids)
	if err != nil {
		return nil, 0, err
	}
	byCourse := make(map[uuid.UUID][]TuitionPack, len(rows))
	for _, p := range packs {
		byCourse[p.CourseID] = append(byCourse[p.CourseID], p)
	}
	out := make([]CourseResponse, 0, len(rows))
	for i := range rows {
		out = append(out, courseResponse(&rows[i], byCourse[rows[i].ID]))
	}
	return out, total, nil
}

// Update replaces the course's own fields. The stored row is read first so
// a blank status and an unchanged default version are treated as "keep",
// not as a fresh choice that must pass the published check again.
func (s *Service) Update(ctx context.Context, sc authctx.Scope, id uuid.UUID, req CourseRequest) (*CourseResponse, error) {
	if err := authctx.Require(sc, authctx.PermCoursesEdit); err != nil {
		return nil, err
	}
	current, err := s.repo.Get(ctx, sc, id)
	if err != nil {
		return nil, notFound(err)
	}
	course, err := s.fromRequest(ctx, sc, req, &current.Course)
	if err != nil {
		return nil, err
	}
	course.ID = id
	if err := s.repo.Update(ctx, sc, course); err != nil {
		return nil, notFound(mapCodeClash(err))
	}
	return s.Get(ctx, sc, id)
}

// Archive moves the course to archived. Classes attached to it are left
// untouched; 409 COURSE_ARCHIVED when it already is.
func (s *Service) Archive(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*CourseResponse, error) {
	if err := authctx.Require(sc, authctx.PermCoursesEdit); err != nil {
		return nil, err
	}
	row, err := s.repo.Get(ctx, sc, id)
	if err != nil {
		return nil, notFound(err)
	}
	if row.Status == StatusArchived {
		return nil, errCourseArchived()
	}
	if err := s.repo.SetStatus(ctx, sc, id, StatusArchived); err != nil {
		return nil, notFound(err)
	}
	return s.Get(ctx, sc, id)
}

// Delete soft-deletes a course nothing points at any more; 409
// COURSE_IN_USE while live classes still reference it. The row is locked
// for the count and the stamp: a class attaching concurrently holds the
// row FOR SHARE, so one side waits and sees the other's outcome.
func (s *Service) Delete(ctx context.Context, sc authctx.Scope, id uuid.UUID) error {
	if err := authctx.Require(sc, authctx.PermCoursesEdit); err != nil {
		return err
	}
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := s.repo.Lock(ctx, sc, id); err != nil {
			return notFound(err)
		}
		n, err := s.repo.LiveClassCount(ctx, sc, id)
		if err != nil {
			return err
		}
		if n > 0 {
			return errCourseInUse()
		}
		return notFound(s.repo.SoftDelete(ctx, sc, id))
	})
}

// SetTuitionPacks replaces the course's whole pack list; [] clears it.
func (s *Service) SetTuitionPacks(ctx context.Context, sc authctx.Scope, id uuid.UUID, items []TuitionPackInput) ([]TuitionPackResponse, error) {
	if err := authctx.Require(sc, authctx.PermCoursesEdit); err != nil {
		return nil, err
	}
	if len(items) > maxTuitionPacks {
		return nil, apperror.Invalid("Tối đa 20 gói học phí cho một khóa học",
			map[string]string{"tuition_packs": "tối đa 20 gói"})
	}
	rows := make([]*TuitionPack, 0, len(items))
	for i, it := range items {
		name := strings.TrimSpace(it.Name)
		if name == "" {
			return nil, apperror.Invalid("Tên gói học phí không được để trống",
				map[string]string{fmt.Sprintf("%d.name", i): "bắt buộc"})
		}
		rows = append(rows, &TuitionPack{Name: name, Sessions: it.Sessions, Price: it.Price})
	}
	var saved []TuitionPack
	// Locking the course serialises two concurrent rewrites (the deferred
	// unique on position would otherwise fail one of them at commit) and
	// keeps a pack list from landing on a course deleted meanwhile.
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := s.repo.Lock(ctx, sc, id); err != nil {
			return notFound(err)
		}
		if err := s.repo.ReplacePacks(ctx, sc, id, rows); err != nil {
			return err
		}
		var err error
		saved, err = s.repo.ListPacks(ctx, sc, []uuid.UUID{id})
		return err
	})
	if err != nil {
		return nil, err
	}
	return packResponses(saved), nil
}

// fromRequest normalises a request into a Course, validating the code and
// the default template version. A newly chosen version must belong to the
// center and be published: a draft may still change and an archived one is
// retired. prev is the stored row on update (nil on create): resending its
// version id is not a new choice, so a version archived or a template
// deleted since then does not block unrelated edits, and a blank status
// keeps the stored one.
func (s *Service) fromRequest(ctx context.Context, sc authctx.Scope, req CourseRequest, prev *Course) (*Course, error) {
	code, err := normalizeCode(req.Code)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, apperror.Invalid("Tên khóa học không được để trống",
			map[string]string{"name": "bắt buộc"})
	}
	status := req.Status
	if status == "" {
		status = StatusDraft
		if prev != nil {
			status = prev.Status
		}
	}
	unchangedVersion := prev != nil && prev.DefaultTemplateVersionID != nil &&
		req.DefaultTemplateVersionID != nil && *req.DefaultTemplateVersionID == *prev.DefaultTemplateVersionID
	if req.DefaultTemplateVersionID != nil && !unchangedVersion {
		ref, err := s.repo.FindTemplateVersion(ctx, sc, *req.DefaultTemplateVersionID)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, err
		}
		if err != nil || ref.Status != library.StatusPublished {
			return nil, apperror.Invalid("Chương trình mẫu mặc định phải là phiên bản đã phát hành của trung tâm",
				map[string]string{"default_template_version_id": "phải là phiên bản đã phát hành"})
		}
	}
	return &Course{
		Code: code, Name: name,
		Subject: optionalText(req.Subject), Level: optionalText(req.Level), Description: optionalText(req.Description),
		Status:                   status,
		DefaultTemplateVersionID: req.DefaultTemplateVersionID,
		DefaultUnitPrice:         req.DefaultUnitPrice,
		TotalSessions:            req.TotalSessions,
		DurationMin:              req.DurationMin,
	}, nil
}

func normalizeCode(raw string) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(raw))
	if !classcode.Valid(code) {
		return "", apperror.Invalid("Mã khóa học không hợp lệ",
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

// notFound maps the repository sentinel onto a 404 and passes every other
// error through.
func notFound(err error) error {
	if errors.Is(err, ErrNotFound) {
		return apperror.NotFound("course")
	}
	return err
}
