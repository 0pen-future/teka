package paths

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/likeq"
	"teka/apps/api/internal/shared/pagination"
)

// ListFilter narrows List. Status keeps one status; Q matches the code or
// name.
type ListFilter struct {
	Status string
	Q      string
}

// Repository is the persistence contract for learning paths. Every method
// is bound to the caller's center: a row of another center reads as
// ErrNotFound / ErrStageNotFound.
type Repository interface {
	// Create inserts a path; gorm.ErrDuplicatedKey when a live path of the
	// center already uses the code.
	Create(ctx context.Context, sc authctx.Scope, p *LearningPath) error
	// Get loads one live path.
	Get(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*LearningPath, error)
	// Lock loads one live path FOR UPDATE inside the ambient transaction so
	// stage writes on the same path serialise (the position unique is
	// deferred, so two concurrent appends would otherwise collide at
	// commit); ErrNotFound when missing.
	Lock(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*LearningPath, error)
	// List pages the center's live paths.
	List(ctx context.Context, sc authctx.Scope, f ListFilter, p pagination.Params) ([]LearningPath, int64, error)
	// Update replaces the path's own fields; gorm.ErrDuplicatedKey on a
	// code clash, ErrNotFound when missing.
	Update(ctx context.Context, sc authctx.Scope, p *LearningPath) error
	// SoftDelete stamps deleted_at; ErrNotFound when missing.
	SoftDelete(ctx context.Context, sc authctx.Scope, id uuid.UUID) error

	// ListStages returns the stages of the given paths ordered by path then
	// position.
	ListStages(ctx context.Context, sc authctx.Scope, pathIDs []uuid.UUID) ([]Stage, error)
	// GetStage loads one stage of the path; ErrStageNotFound when missing.
	GetStage(ctx context.Context, sc authctx.Scope, pathID, stageID uuid.UUID) (*Stage, error)
	// CreateStage inserts a stage at the position already set on it.
	CreateStage(ctx context.Context, sc authctx.Scope, st *Stage) error
	// UpdateStage replaces the stage's name and goal; ErrStageNotFound when
	// missing.
	UpdateStage(ctx context.Context, sc authctx.Scope, st *Stage) error
	// DeleteStage removes the stage and its course links; ErrStageNotFound
	// when missing.
	DeleteStage(ctx context.Context, sc authctx.Scope, pathID, stageID uuid.UUID) error
	// SetStagePositions assigns positions 1..n to ids in order. Run it
	// inside a transaction holding the path lock: the unique on (path_id,
	// position) is deferred. ErrStageNotFound when an id is not a stage of
	// the path.
	SetStagePositions(ctx context.Context, sc authctx.Scope, pathID uuid.UUID, ids []uuid.UUID) error

	// ListStageCourses returns the live courses linked into the given
	// stages ordered by stage then position; a deleted course drops out.
	ListStageCourses(ctx context.Context, sc authctx.Scope, stageIDs []uuid.UUID) ([]StageCourseRow, error)
	// LockCourses takes a share lock on the live courses of the center among
	// ids and returns the ids it found, so a course cannot be deleted while
	// a stage is being pointed at it.
	LockCourses(ctx context.Context, sc authctx.Scope, ids []uuid.UUID) ([]uuid.UUID, error)
	// ReplaceStageCourses swaps the stage's whole course list for courseIDs
	// (positions assigned 1..n in order).
	ReplaceStageCourses(ctx context.Context, sc authctx.Scope, stageID uuid.UUID, courseIDs []uuid.UUID) error
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

// live scopes a query to the caller's center's non-deleted paths.
func (r *gormRepository) live(ctx context.Context, sc authctx.Scope) *gorm.DB {
	return database.FromContext(ctx, r.db).
		Model(&LearningPath{}).
		Where("learning_paths.center_id = ? AND learning_paths.deleted_at IS NULL", sc.CenterID)
}

// stages scopes a query to the caller's center's stages.
func (r *gormRepository) stages(ctx context.Context, sc authctx.Scope) *gorm.DB {
	return database.FromContext(ctx, r.db).
		Model(&Stage{}).
		Where("path_stages.center_id = ?", sc.CenterID)
}

// links scopes a query to the caller's center's stage-course links.
func (r *gormRepository) links(ctx context.Context, sc authctx.Scope) *gorm.DB {
	return database.FromContext(ctx, r.db).
		Model(&StageCourse{}).
		Where("path_stage_courses.center_id = ?", sc.CenterID)
}

func (r *gormRepository) Create(ctx context.Context, sc authctx.Scope, p *LearningPath) error {
	if p.ID == uuid.Nil {
		p.ID = id.New()
	}
	p.CenterID = sc.CenterID
	return database.FromContext(ctx, r.db).Create(p).Error
}

func (r *gormRepository) Get(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*LearningPath, error) {
	var p LearningPath
	err := r.live(ctx, sc).Where("learning_paths.id = ?", id).Take(&p).Error
	if err != nil {
		return nil, notFoundOr(err)
	}
	return &p, nil
}

func (r *gormRepository) Lock(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*LearningPath, error) {
	var p LearningPath
	err := r.live(ctx, sc).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("learning_paths.id = ?", id).
		Take(&p).Error
	if err != nil {
		return nil, notFoundOr(err)
	}
	return &p, nil
}

func (r *gormRepository) List(ctx context.Context, sc authctx.Scope, f ListFilter, p pagination.Params) ([]LearningPath, int64, error) {
	q := r.live(ctx, sc)
	if f.Status != "" {
		q = q.Where("learning_paths.status = ?", f.Status)
	}
	if f.Q != "" {
		needle := likeq.Contains(f.Q)
		q = q.Where(`(learning_paths.name ILIKE ? ESCAPE '\' OR learning_paths.code ILIKE ? ESCAPE '\')`, needle, needle)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []LearningPath
	// The id tie-breaker keeps pages stable when several paths share the
	// sort value.
	err := q.Scopes(p.Scope).Order("learning_paths.id ASC").Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (r *gormRepository) Update(ctx context.Context, sc authctx.Scope, p *LearningPath) error {
	res := r.live(ctx, sc).
		Where("learning_paths.id = ?", p.ID).
		Updates(map[string]any{
			"code": p.Code, "name": p.Name, "description": p.Description, "status": p.Status,
			"updated_at": gorm.Expr("now()"),
		})
	return affected(res, ErrNotFound)
}

func (r *gormRepository) SoftDelete(ctx context.Context, sc authctx.Scope, id uuid.UUID) error {
	res := r.live(ctx, sc).
		Where("learning_paths.id = ?", id).
		Updates(map[string]any{"deleted_at": gorm.Expr("now()"), "updated_at": gorm.Expr("now()")})
	return affected(res, ErrNotFound)
}

func (r *gormRepository) ListStages(ctx context.Context, sc authctx.Scope, pathIDs []uuid.UUID) ([]Stage, error) {
	if len(pathIDs) == 0 {
		return nil, nil
	}
	var rows []Stage
	err := r.stages(ctx, sc).
		Where("path_stages.path_id IN ?", pathIDs).
		Order("path_stages.path_id, path_stages.position, path_stages.id").
		Find(&rows).Error
	return rows, err
}

func (r *gormRepository) GetStage(ctx context.Context, sc authctx.Scope, pathID, stageID uuid.UUID) (*Stage, error) {
	var st Stage
	err := r.stages(ctx, sc).
		Where("path_stages.path_id = ? AND path_stages.id = ?", pathID, stageID).
		Take(&st).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStageNotFound
		}
		return nil, err
	}
	return &st, nil
}

func (r *gormRepository) CreateStage(ctx context.Context, sc authctx.Scope, st *Stage) error {
	if st.ID == uuid.Nil {
		st.ID = id.New()
	}
	st.CenterID = sc.CenterID
	return database.FromContext(ctx, r.db).Create(st).Error
}

func (r *gormRepository) UpdateStage(ctx context.Context, sc authctx.Scope, st *Stage) error {
	res := r.stages(ctx, sc).
		Where("path_stages.path_id = ? AND path_stages.id = ?", st.PathID, st.ID).
		Updates(map[string]any{"name": st.Name, "goal": st.Goal})
	return affected(res, ErrStageNotFound)
}

func (r *gormRepository) DeleteStage(ctx context.Context, sc authctx.Scope, pathID, stageID uuid.UUID) error {
	res := r.stages(ctx, sc).
		Where("path_stages.path_id = ? AND path_stages.id = ?", pathID, stageID).
		Delete(&Stage{})
	return affected(res, ErrStageNotFound)
}

func (r *gormRepository) SetStagePositions(ctx context.Context, sc authctx.Scope, pathID uuid.UUID, ids []uuid.UUID) error {
	for i, stageID := range ids {
		res := r.stages(ctx, sc).
			Where("path_stages.path_id = ? AND path_stages.id = ?", pathID, stageID).
			Update("position", i+1)
		if err := affected(res, ErrStageNotFound); err != nil {
			return err
		}
	}
	return nil
}

func (r *gormRepository) ListStageCourses(ctx context.Context, sc authctx.Scope, stageIDs []uuid.UUID) ([]StageCourseRow, error) {
	if len(stageIDs) == 0 {
		return nil, nil
	}
	var rows []StageCourseRow
	err := r.links(ctx, sc).
		Select("path_stage_courses.stage_id, path_stage_courses.position, path_stage_courses.course_id, c.code, c.name, c.status").
		Joins("JOIN courses c ON c.id = path_stage_courses.course_id AND c.center_id = path_stage_courses.center_id AND c.deleted_at IS NULL").
		Where("path_stage_courses.stage_id IN ?", stageIDs).
		Order("path_stage_courses.stage_id, path_stage_courses.position").
		Scan(&rows).Error
	return rows, err
}

func (r *gormRepository) LockCourses(ctx context.Context, sc authctx.Scope, ids []uuid.UUID) ([]uuid.UUID, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var found []uuid.UUID
	err := database.FromContext(ctx, r.db).
		Table("courses").
		Clauses(clause.Locking{Strength: "SHARE"}).
		Where("courses.center_id = ? AND courses.deleted_at IS NULL AND courses.id IN ?", sc.CenterID, ids).
		Pluck("courses.id", &found).Error
	return found, err
}

func (r *gormRepository) ReplaceStageCourses(ctx context.Context, sc authctx.Scope, stageID uuid.UUID, courseIDs []uuid.UUID) error {
	if err := r.links(ctx, sc).Where("path_stage_courses.stage_id = ?", stageID).Delete(&StageCourse{}).Error; err != nil {
		return err
	}
	if len(courseIDs) == 0 {
		return nil
	}
	rows := make([]StageCourse, 0, len(courseIDs))
	for i, courseID := range courseIDs {
		rows = append(rows, StageCourse{StageID: stageID, CourseID: courseID, CenterID: sc.CenterID, Position: i + 1})
	}
	return database.FromContext(ctx, r.db).Create(&rows).Error
}

// affected turns a zero-row write into missing.
func affected(res *gorm.DB, missing error) error {
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return missing
	}
	return nil
}

func notFoundOr(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return err
}
