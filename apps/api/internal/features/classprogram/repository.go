package classprogram

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/authctx"
)

// Repository owns the class_programs table. Every read is scoped by the
// caller's center; the class itself is resolved by the service through the
// classes read port before any call lands here.
type Repository interface {
	// Get returns the class's applied program joined with its template and
	// version, or nil when the class applies none.
	Get(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*ProgramRow, error)
	// Upsert inserts or replaces the class's single program row.
	Upsert(ctx context.Context, p *Program) error
	// Delete removes the class's program row and reports whether one existed.
	Delete(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (bool, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository builds a GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Get(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*ProgramRow, error) {
	var row ProgramRow
	err := database.FromContext(ctx, r.db).
		Table("class_programs AS p").
		Select(`p.class_id, p.center_id, p.template_version_id, p.applied_at, p.applied_by,
			v.template_id, v.version_no, v.status AS version_status, t.name AS template_name,
			(SELECT count(*) FROM template_lessons l WHERE l.version_id = p.template_version_id) AS lesson_count`).
		Joins("JOIN program_template_versions v ON v.id = p.template_version_id").
		Joins("JOIN program_templates t ON t.id = v.template_id").
		Where("p.class_id = ? AND p.center_id = ?", classID, sc.CenterID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *gormRepository) Upsert(ctx context.Context, p *Program) error {
	return database.FromContext(ctx, r.db).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "class_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"template_version_id": gorm.Expr("excluded.template_version_id"),
				"applied_at":          gorm.Expr("now()"),
				"applied_by":          gorm.Expr("excluded.applied_by"),
			}),
		}).
		Create(p).Error
}

func (r *gormRepository) Delete(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (bool, error) {
	res := database.FromContext(ctx, r.db).
		Where("class_id = ? AND center_id = ?", classID, sc.CenterID).
		Delete(&Program{})
	return res.RowsAffected > 0, res.Error
}
