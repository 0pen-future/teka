package paths

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/classcode"
	"teka/apps/api/internal/shared/idset"
	"teka/apps/api/internal/shared/pagination"
)

// maxStageCourses caps the courses one stage may recommend; the request
// binding enforces the same number at the edge.
const maxStageCourses = 20

// Service holds the learning path rules: code normalisation, dense stage
// positions and course links limited to the center's live catalog.
type Service struct {
	repo Repository
	tx   database.TxManager
}

// NewService wires the service.
func NewService(repo Repository, tx database.TxManager) *Service {
	return &Service{repo: repo, tx: tx}
}

// Create adds a path in draft unless a status is given.
func (s *Service) Create(ctx context.Context, sc authctx.Scope, req PathRequest) (*PathResponse, error) {
	if err := authctx.Require(sc, authctx.PermPathsEdit); err != nil {
		return nil, err
	}
	p, err := fromRequest(req, nil)
	if err != nil {
		return nil, err
	}
	if err := s.repo.Create(ctx, sc, p); err != nil {
		return nil, mapCodeClash(err)
	}
	return s.load(ctx, sc, p.ID)
}

// Get returns one path with its stages.
func (s *Service) Get(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*PathResponse, error) {
	if err := authctx.Require(sc, authctx.PermPathsRead); err != nil {
		return nil, err
	}
	return s.load(ctx, sc, id)
}

// List pages the center's paths, each with its stages.
func (s *Service) List(ctx context.Context, sc authctx.Scope, f ListFilter, p pagination.Params) ([]PathResponse, int64, error) {
	if err := authctx.Require(sc, authctx.PermPathsRead); err != nil {
		return nil, 0, err
	}
	rows, total, err := s.repo.List(ctx, sc, f, p)
	if err != nil {
		return nil, 0, err
	}
	out, err := s.assemble(ctx, sc, rows)
	if err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// Update replaces the path's own fields; a blank status keeps the stored
// one.
func (s *Service) Update(ctx context.Context, sc authctx.Scope, id uuid.UUID, req PathRequest) (*PathResponse, error) {
	if err := authctx.Require(sc, authctx.PermPathsEdit); err != nil {
		return nil, err
	}
	prev, err := s.repo.Get(ctx, sc, id)
	if err != nil {
		return nil, notFound(err)
	}
	p, err := fromRequest(req, prev)
	if err != nil {
		return nil, err
	}
	p.ID = id
	if err := s.repo.Update(ctx, sc, p); err != nil {
		return nil, notFound(mapCodeClash(err))
	}
	return s.load(ctx, sc, id)
}

// Delete soft-deletes the path; its stages stay attached to the hidden row
// and the code becomes reusable.
func (s *Service) Delete(ctx context.Context, sc authctx.Scope, id uuid.UUID) error {
	if err := authctx.Require(sc, authctx.PermPathsEdit); err != nil {
		return err
	}
	return notFound(s.repo.SoftDelete(ctx, sc, id))
}

// CreateStage appends a stage after the last one.
func (s *Service) CreateStage(ctx context.Context, sc authctx.Scope, pathID uuid.UUID, req StageRequest) (*PathResponse, error) {
	if err := authctx.Require(sc, authctx.PermPathsEdit); err != nil {
		return nil, err
	}
	st, err := stageFromRequest(req)
	if err != nil {
		return nil, err
	}
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := s.repo.Lock(ctx, sc, pathID); err != nil {
			return err
		}
		current, err := s.repo.ListStages(ctx, sc, []uuid.UUID{pathID})
		if err != nil {
			return err
		}
		st.PathID, st.Position = pathID, len(current)+1
		return s.repo.CreateStage(ctx, sc, st)
	})
	if err != nil {
		return nil, notFound(err)
	}
	return s.load(ctx, sc, pathID)
}

// UpdateStage replaces a stage's name and goal.
func (s *Service) UpdateStage(ctx context.Context, sc authctx.Scope, pathID, stageID uuid.UUID, req StageRequest) (*PathResponse, error) {
	if err := authctx.Require(sc, authctx.PermPathsEdit); err != nil {
		return nil, err
	}
	st, err := stageFromRequest(req)
	if err != nil {
		return nil, err
	}
	if _, err := s.repo.Get(ctx, sc, pathID); err != nil {
		return nil, notFound(err)
	}
	st.ID, st.PathID = stageID, pathID
	if err := s.repo.UpdateStage(ctx, sc, st); err != nil {
		return nil, notFound(err)
	}
	return s.load(ctx, sc, pathID)
}

// DeleteStage removes a stage and closes the gap so positions stay 1..n.
func (s *Service) DeleteStage(ctx context.Context, sc authctx.Scope, pathID, stageID uuid.UUID) (*PathResponse, error) {
	if err := authctx.Require(sc, authctx.PermPathsEdit); err != nil {
		return nil, err
	}
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := s.repo.Lock(ctx, sc, pathID); err != nil {
			return err
		}
		if err := s.repo.DeleteStage(ctx, sc, pathID, stageID); err != nil {
			return err
		}
		remaining, err := s.repo.ListStages(ctx, sc, []uuid.UUID{pathID})
		if err != nil {
			return err
		}
		return s.repo.SetStagePositions(ctx, sc, pathID, stageIDsOf(remaining))
	})
	if err != nil {
		return nil, notFound(err)
	}
	return s.load(ctx, sc, pathID)
}

// ReorderStages applies a full permutation of the path's stages in one
// transaction, which the deferred position unique relies on.
func (s *Service) ReorderStages(ctx context.Context, sc authctx.Scope, pathID uuid.UUID, req ReorderRequest) (*PathResponse, error) {
	if err := authctx.Require(sc, authctx.PermPathsEdit); err != nil {
		return nil, err
	}
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := s.repo.Lock(ctx, sc, pathID); err != nil {
			return err
		}
		current, err := s.repo.ListStages(ctx, sc, []uuid.UUID{pathID})
		if err != nil {
			return err
		}
		if !idset.Same(stageIDsOf(current), req.StageIDs) {
			return apperror.Invalid("Thứ tự phải liệt kê đúng mỗi giai đoạn của lộ trình một lần",
				map[string]string{"stage_ids": "phải chứa đúng các giai đoạn của lộ trình, mỗi giai đoạn một lần"})
		}
		return s.repo.SetStagePositions(ctx, sc, pathID, req.StageIDs)
	})
	if err != nil {
		return nil, notFound(err)
	}
	return s.load(ctx, sc, pathID)
}

// SetStageCourses replaces a stage's course list. Every id must be a live
// course of the center (archived is fine: the status is shown); the
// courses are share-locked so a concurrent delete waits for the link.
func (s *Service) SetStageCourses(ctx context.Context, sc authctx.Scope, pathID, stageID uuid.UUID, req StageCoursesRequest) (*PathResponse, error) {
	if err := authctx.Require(sc, authctx.PermPathsEdit); err != nil {
		return nil, err
	}
	if len(req.CourseIDs) > maxStageCourses {
		return nil, apperror.Invalid("Tối đa 20 khóa học cho một giai đoạn",
			map[string]string{"course_ids": "tối đa 20 khóa học"})
	}
	seen := make(map[uuid.UUID]struct{}, len(req.CourseIDs))
	for _, courseID := range req.CourseIDs {
		if _, dup := seen[courseID]; dup {
			return nil, apperror.Invalid("Mỗi khóa học chỉ xuất hiện một lần trong giai đoạn",
				map[string]string{"course_ids": "không được lặp lại khóa học"})
		}
		seen[courseID] = struct{}{}
	}
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if _, err := s.repo.Lock(ctx, sc, pathID); err != nil {
			return err
		}
		if _, err := s.repo.GetStage(ctx, sc, pathID, stageID); err != nil {
			return err
		}
		found, err := s.repo.LockCourses(ctx, sc, req.CourseIDs)
		if err != nil {
			return err
		}
		if len(found) != len(req.CourseIDs) {
			return apperror.Invalid("Khóa học phải thuộc trung tâm và chưa bị xoá",
				map[string]string{"course_ids": "phải là khóa học còn hoạt động của trung tâm"})
		}
		return s.repo.ReplaceStageCourses(ctx, sc, stageID, req.CourseIDs)
	})
	if err != nil {
		return nil, notFound(err)
	}
	return s.load(ctx, sc, pathID)
}

// load reads one path and assembles it; callers have already checked the
// permission or just wrote the row.
func (s *Service) load(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*PathResponse, error) {
	p, err := s.repo.Get(ctx, sc, id)
	if err != nil {
		return nil, notFound(err)
	}
	out, err := s.assemble(ctx, sc, []LearningPath{*p})
	if err != nil {
		return nil, err
	}
	return &out[0], nil
}

// assemble loads the stages and course links of paths in two batched
// queries and folds them into responses in the order given.
func (s *Service) assemble(ctx context.Context, sc authctx.Scope, rows []LearningPath) ([]PathResponse, error) {
	pathIDs := make([]uuid.UUID, len(rows))
	for i := range rows {
		pathIDs[i] = rows[i].ID
	}
	stages, err := s.repo.ListStages(ctx, sc, pathIDs)
	if err != nil {
		return nil, err
	}
	courses, err := s.repo.ListStageCourses(ctx, sc, stageIDsOf(stages))
	if err != nil {
		return nil, err
	}
	byStage := map[uuid.UUID][]StageCourseRow{}
	for _, c := range courses {
		byStage[c.StageID] = append(byStage[c.StageID], c)
	}
	byPath := map[uuid.UUID][]Stage{}
	for _, st := range stages {
		byPath[st.PathID] = append(byPath[st.PathID], st)
	}
	out := make([]PathResponse, 0, len(rows))
	for i := range rows {
		out = append(out, pathResponse(&rows[i], byPath[rows[i].ID], byStage))
	}
	return out, nil
}

func stageIDsOf(stages []Stage) []uuid.UUID {
	ids := make([]uuid.UUID, len(stages))
	for i := range stages {
		ids[i] = stages[i].ID
	}
	return ids
}

// fromRequest normalises a request into a LearningPath. prev is the stored
// row on update (nil on create): a blank status keeps it.
func fromRequest(req PathRequest, prev *LearningPath) (*LearningPath, error) {
	code, err := normalizeCode(req.Code)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, apperror.Invalid("Tên lộ trình không được để trống",
			map[string]string{"name": "bắt buộc"})
	}
	status := req.Status
	if status == "" {
		status = StatusDraft
		if prev != nil {
			status = prev.Status
		}
	}
	return &LearningPath{Code: code, Name: name, Description: optionalText(req.Description), Status: status}, nil
}

func stageFromRequest(req StageRequest) (*Stage, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, apperror.Invalid("Tên giai đoạn không được để trống",
			map[string]string{"name": "bắt buộc"})
	}
	return &Stage{Name: name, Goal: optionalText(req.Goal)}, nil
}

func normalizeCode(raw string) (string, error) {
	code := strings.ToUpper(strings.TrimSpace(raw))
	if !classcode.Valid(code) {
		return "", apperror.Invalid("Mã lộ trình không hợp lệ",
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
