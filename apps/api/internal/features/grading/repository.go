package grading

import (
	"context"

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
// enforced by the scopelint analyzer under tools/).
type Repository interface {
	// GetClassComponents returns a class's snapshot components, position order.
	GetClassComponents(ctx context.Context, sc authctx.Scope, classID uuid.UUID) ([]ClassComponent, error)
	// ReplaceClassComponents removes a class's current snapshot and inserts the
	// new copies. Deleting a snapshot row cascade-deletes its student_scores,
	// so callers guard with ClassHasScores first.
	ReplaceClassComponents(ctx context.Context, classID uuid.UUID, components []ClassComponent) error
	// ClassHasScores reports whether the class carries ≥1 student score (any
	// session, including past ones) — the guard before a snapshot replace.
	ClassHasScores(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (bool, error)
	// LockClassForScoring takes a transaction-scoped advisory lock keyed on the
	// class. Both the component swap (template apply) and the score write take it,
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
	// rare (an owner applying a program while a teacher grades the same class),
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
