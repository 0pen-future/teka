package tasks

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/pkg/kanban"
)

// maxColumnsPerTenant mirrors kanban's default Config.MaxColumns (see
// pkg/kanban/options.go, defaultMaxColumns). This package never overrides
// that default via kanban.WithMaxColumns (see service.go), so the two values
// must be kept in sync by hand; a second, independent cap check here is what
// closes the CreateColumn check-then-act race the core's own README
// documents (CountByTenant and Create are not atomic from the core's point
// of view) — see columnRepository.Create.
const maxColumnsPerTenant = 8

// columnRepository implements kanban.ColumnRepository against task_columns.
type columnRepository struct {
	db *gorm.DB
}

// newColumnRepository builds the GORM-backed kanban.ColumnRepository.
func newColumnRepository(db *gorm.DB) *columnRepository {
	return &columnRepository{db: db}
}

var _ kanban.ColumnRepository = (*columnRepository)(nil)

// List returns every column of tenant, unordered — Service.Board sorts by
// Position itself.
func (r *columnRepository) List(ctx context.Context, tenant kanban.TenantID) ([]kanban.Column, error) {
	var rows []columnModel
	err := database.FromContext(ctx, r.db).
		Where("center_id = ?", uuid.UUID(tenant)).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	cols := make([]kanban.Column, len(rows))
	for i, row := range rows {
		cols[i] = columnToCore(row)
	}
	return cols, nil
}

// Get returns the column identified by id within tenant, or
// kanban.ErrColumnNotFound if it does not exist or belongs to another
// tenant — a cross-tenant id is indistinguishable from a missing one.
func (r *columnRepository) Get(ctx context.Context, tenant kanban.TenantID, id kanban.ColumnID) (kanban.Column, error) {
	var row columnModel
	err := database.FromContext(ctx, r.db).
		Where("id = ? AND center_id = ?", uuid.UUID(id), uuid.UUID(tenant)).
		First(&row).Error
	if err != nil {
		if errIsRecordNotFound(err) {
			return kanban.Column{}, kanban.ErrColumnNotFound
		}
		return kanban.Column{}, err
	}
	return columnToCore(row), nil
}

// Create inserts col. It re-validates tenant's column cap inside its own
// transaction, guarded by a per-tenant advisory lock, so two concurrent
// CreateColumn calls cannot both pass the Service's own (non-atomic)
// pre-check and jointly exceed maxColumnsPerTenant. The lock is transaction-
// scoped (pg_advisory_xact_lock) and releases automatically on commit or
// rollback.
func (r *columnRepository) Create(ctx context.Context, tenant kanban.TenantID, col kanban.Column) (kanban.Column, error) {
	row := columnFromCore(col)
	err := database.FromContext(ctx, r.db).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtext(?::text))`, uuid.UUID(tenant).String()).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&columnModel{}).Where("center_id = ?", uuid.UUID(tenant)).Count(&count).Error; err != nil {
			return err
		}
		if int(count) >= maxColumnsPerTenant {
			return kanban.ErrColumnLimit
		}
		return translateDBError(tx.Create(&row).Error)
	})
	if err != nil {
		return kanban.Column{}, err
	}
	return columnToCore(row), nil
}

// Update persists col's current field values (Name, IsDone, Color — the only
// fields Service.UpdateColumn mutates) over the row matching its id and
// tenant.
func (r *columnRepository) Update(ctx context.Context, tenant kanban.TenantID, col kanban.Column) (kanban.Column, error) {
	res := database.FromContext(ctx, r.db).
		Model(&columnModel{}).
		Where("id = ? AND center_id = ?", uuid.UUID(col.ID), uuid.UUID(tenant)).
		Updates(map[string]any{
			"name":       col.Name,
			"is_done":    col.IsDone,
			"color":      col.Color,
			"updated_at": gorm.Expr("now()"),
		})
	if res.Error != nil {
		return kanban.Column{}, translateDBError(res.Error)
	}
	if res.RowsAffected == 0 {
		return kanban.Column{}, kanban.ErrColumnNotFound
	}
	return r.Get(ctx, tenant, col.ID)
}

// Delete removes the column identified by id within tenant. A RESTRICT
// foreign key on tasks.column_id turns a delete of a still-referenced column
// into a 23503, translated by translateDBError to
// kanban.ErrColumnNotEmpty — a defense-in-depth backstop behind
// Service.DeleteColumn's own CountInColumn check.
func (r *columnRepository) Delete(ctx context.Context, tenant kanban.TenantID, id kanban.ColumnID) error {
	res := database.FromContext(ctx, r.db).
		Where("id = ? AND center_id = ?", uuid.UUID(id), uuid.UUID(tenant)).
		Delete(&columnModel{})
	if res.Error != nil {
		return translateDBError(res.Error)
	}
	if res.RowsAffected == 0 {
		return kanban.ErrColumnNotFound
	}
	return nil
}

// UpdatePositions writes each column's new Position per its index in order.
// Service.ReorderColumns has already validated order is a permutation of
// tenant's current column ids.
func (r *columnRepository) UpdatePositions(ctx context.Context, tenant kanban.TenantID, order []kanban.ColumnID) error {
	db := database.FromContext(ctx, r.db)
	for i, id := range order {
		res := db.Model(&columnModel{}).
			Where("id = ? AND center_id = ?", uuid.UUID(id), uuid.UUID(tenant)).
			Update("position", i)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return kanban.ErrColumnNotFound
		}
	}
	return nil
}

// CountByTenant returns how many columns tenant currently has.
func (r *columnRepository) CountByTenant(ctx context.Context, tenant kanban.TenantID) (int, error) {
	var count int64
	err := database.FromContext(ctx, r.db).
		Model(&columnModel{}).
		Where("center_id = ?", uuid.UUID(tenant)).
		Count(&count).Error
	return int(count), err
}

// ExistsName reports whether tenant already has a column named name,
// case-insensitively — mirroring the uq_task_columns_name index
// (center_id, lower(name)).
func (r *columnRepository) ExistsName(ctx context.Context, tenant kanban.TenantID, name string) (bool, error) {
	var exists bool
	err := database.FromContext(ctx, r.db).
		Raw(`SELECT EXISTS (SELECT 1 FROM task_columns WHERE center_id = ? AND lower(name) = lower(?))`,
			uuid.UUID(tenant), strings.TrimSpace(name)).
		Scan(&exists).Error
	return exists, err
}
