package grading

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/authctx"
)

// Repository is the persistence contract for grading data; the service depends
// on this interface, tests supply a fake.
//
// Every method is center-scoped through the caller's center id only — never
// the owner flag or a permission check (the owner gate lives in the service,
// enforced by scoping_guard_test).
type Repository interface {
	// ListSets returns the center's live (non-deleted) score sets, name order.
	ListSets(ctx context.Context, sc authctx.Scope) ([]ScoreSet, error)
	// GetSet returns one live set of the center, or nil when missing / soft
	// deleted / another center's — the service maps nil to 404.
	GetSet(ctx context.Context, sc authctx.Scope, setID uuid.UUID) (*ScoreSet, error)
	// ListComponentsForSets returns the components of the given sets, ordered
	// by (set, position) — one query behind the list read.
	//
	// INVARIANT: it filters only by set_id, not center_id, so every caller MUST
	// pass set ids already resolved under the caller's scope (GetSet / ListSets).
	// Passing an unscoped set id would read another center's component names.
	ListComponentsForSets(ctx context.Context, setIDs []uuid.UUID) ([]SetComponent, error)
	// CreateSet inserts a set row. A duplicate live name in the center loses to
	// score_sets_center_name_live and surfaces as gorm.ErrDuplicatedKey.
	CreateSet(ctx context.Context, set *ScoreSet) error
	// UpdateSet writes name + updated_at back for a live set of the center.
	UpdateSet(ctx context.Context, sc authctx.Scope, set *ScoreSet) error
	// SoftDeleteSet stamps deleted_at on a live set of the center.
	SoftDeleteSet(ctx context.Context, sc authctx.Scope, setID uuid.UUID) error
	// ReplaceSetComponents hard-deletes a set's components and inserts the new
	// list — safe because per-class snapshots already copied the values.
	ReplaceSetComponents(ctx context.Context, setID uuid.UUID, components []SetComponent) error

	// GetClassComponents returns a class's snapshot components, position order.
	GetClassComponents(ctx context.Context, sc authctx.Scope, classID uuid.UUID) ([]ClassComponent, error)
	// ReplaceClassComponents removes a class's current snapshot and inserts the
	// new copies (empty list = clear). Deleting a snapshot row cascade-deletes
	// its student_scores, so callers guard with ClassHasScores first.
	ReplaceClassComponents(ctx context.Context, classID uuid.UUID, components []ClassComponent) error
	// ClassHasScores reports whether the class carries ≥1 student score (any
	// session, including past ones) — the re-apply / clear guard.
	ClassHasScores(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (bool, error)
	// LockClassForScoring takes a transaction-scoped advisory lock keyed on the
	// class. Both the component swap (assign/clear) and the score write take it,
	// so a swap — which cascade-deletes student_scores — cannot interleave with
	// a concurrent score write and silently drop a just-recorded grade. The lock
	// releases on commit/rollback; call it as the first statement of the tx.
	LockClassForScoring(ctx context.Context, classID uuid.UUID) error

	// ListScoresBySession returns the session's current score rows — the base
	// the batch write merges into.
	ListScoresBySession(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) ([]StudentScore, error)
	// UpsertScores batch-writes score rows in one statement, keyed on
	// uq_student_scores_session_component_student.
	UpsertScores(ctx context.Context, scores []StudentScore) error
	// DeleteScores removes the given rows by id — the null-cell outcome of the
	// merge. Center-scoped as defence in depth.
	DeleteScores(ctx context.Context, sc authctx.Scope, ids []uuid.UUID) error
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) ListSets(ctx context.Context, sc authctx.Scope) ([]ScoreSet, error) {
	var sets []ScoreSet
	err := database.FromContext(ctx, r.db).
		Where("center_id = ? AND deleted_at IS NULL", sc.CenterID).
		Order("lower(name)").
		Find(&sets).Error
	return sets, err
}

func (r *gormRepository) GetSet(ctx context.Context, sc authctx.Scope, setID uuid.UUID) (*ScoreSet, error) {
	var set ScoreSet
	err := database.FromContext(ctx, r.db).
		Where("id = ? AND center_id = ? AND deleted_at IS NULL", setID, sc.CenterID).
		Take(&set).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &set, nil
}

func (r *gormRepository) ListComponentsForSets(ctx context.Context, setIDs []uuid.UUID) ([]SetComponent, error) {
	if len(setIDs) == 0 {
		return nil, nil
	}
	var components []SetComponent
	err := database.FromContext(ctx, r.db).
		Where("set_id IN ?", setIDs).
		Order("set_id, position").
		Find(&components).Error
	return components, err
}

func (r *gormRepository) CreateSet(ctx context.Context, set *ScoreSet) error {
	return database.FromContext(ctx, r.db).Create(set).Error
}

func (r *gormRepository) UpdateSet(ctx context.Context, sc authctx.Scope, set *ScoreSet) error {
	return database.FromContext(ctx, r.db).
		Model(&ScoreSet{}).
		Where("id = ? AND center_id = ? AND deleted_at IS NULL", set.ID, sc.CenterID).
		Updates(map[string]any{
			"name":       set.Name,
			"updated_at": gorm.Expr("now()"),
		}).Error
}

func (r *gormRepository) SoftDeleteSet(ctx context.Context, sc authctx.Scope, setID uuid.UUID) error {
	return database.FromContext(ctx, r.db).
		Model(&ScoreSet{}).
		Where("id = ? AND center_id = ? AND deleted_at IS NULL", setID, sc.CenterID).
		Updates(map[string]any{
			"deleted_at": gorm.Expr("now()"),
			"updated_at": gorm.Expr("now()"),
		}).Error
}

func (r *gormRepository) ReplaceSetComponents(ctx context.Context, setID uuid.UUID, components []SetComponent) error {
	db := database.FromContext(ctx, r.db)
	if err := db.Where("set_id = ?", setID).Delete(&SetComponent{}).Error; err != nil {
		return err
	}
	if len(components) == 0 {
		return nil
	}
	return db.Create(components).Error
}

func (r *gormRepository) GetClassComponents(ctx context.Context, sc authctx.Scope, classID uuid.UUID) ([]ClassComponent, error) {
	var components []ClassComponent
	err := database.FromContext(ctx, r.db).
		Where("center_id = ? AND class_id = ?", sc.CenterID, classID).
		Order("position").
		Find(&components).Error
	return components, err
}

func (r *gormRepository) ReplaceClassComponents(ctx context.Context, classID uuid.UUID, components []ClassComponent) error {
	db := database.FromContext(ctx, r.db)
	if err := db.Where("class_id = ?", classID).Delete(&ClassComponent{}).Error; err != nil {
		return err
	}
	if len(components) == 0 {
		return nil
	}
	return db.Create(components).Error
}

func (r *gormRepository) ClassHasScores(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (bool, error) {
	var exists bool
	err := database.FromContext(ctx, r.db).
		Raw("SELECT EXISTS (SELECT 1 FROM student_scores WHERE center_id = ? AND class_id = ?)", sc.CenterID, classID).
		Scan(&exists).Error
	return exists, err
}

func (r *gormRepository) LockClassForScoring(ctx context.Context, classID uuid.UUID) error {
	// Blocking, not the TRY variant imports uses: contention is per-class and
	// rare (an owner reconfiguring a set while a teacher grades the same class),
	// the held work is a handful of small statements, and the caller's context
	// deadline bounds the wait — so waiting the few ms is preferable to failing
	// the write with a 409 the client would just retry. hashtext takes text;
	// class_id is a uuid, hence the cast.
	return database.FromContext(ctx, r.db).
		Exec(`SELECT pg_advisory_xact_lock(hashtext(?::text))`, classID.String()).Error
}

func (r *gormRepository) ListScoresBySession(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) ([]StudentScore, error) {
	var scores []StudentScore
	err := database.FromContext(ctx, r.db).
		Where("center_id = ? AND session_id = ?", sc.CenterID, sessionID).
		Find(&scores).Error
	return scores, err
}

func (r *gormRepository) UpsertScores(ctx context.Context, scores []StudentScore) error {
	if len(scores) == 0 {
		return nil
	}
	return database.FromContext(ctx, r.db).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "session_id"}, {Name: "component_id"}, {Name: "student_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"score":      gorm.Expr("excluded.score"),
				"updated_at": gorm.Expr("now()"),
			}),
		}).
		Create(scores).Error
}

func (r *gormRepository) DeleteScores(ctx context.Context, sc authctx.Scope, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	return database.FromContext(ctx, r.db).
		Where("center_id = ? AND id IN ?", sc.CenterID, ids).
		Delete(&StudentScore{}).Error
}
