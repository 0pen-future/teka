package classchat

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/authctx"
)

// Repository owns the class_messages table. Every read is scoped by the
// caller's center; the class itself is resolved by the service before any
// call lands here.
type Repository interface {
	// List returns up to limit live messages of the class, newest first,
	// older than the before cursor when one is given. A cursor that is not
	// one of the class's messages yields no rows.
	List(ctx context.Context, sc authctx.Scope, classID uuid.UUID, before *uuid.UUID, limit int) ([]MessageRow, error)
	// Get returns a live message of the class, or ErrNotFound.
	Get(ctx context.Context, sc authctx.Scope, classID, messageID uuid.UUID) (*MessageRow, error)
	// Create inserts the message.
	Create(ctx context.Context, m *Message) error
	// SoftDelete retracts a live message and reports whether it was live.
	SoftDelete(ctx context.Context, sc authctx.Scope, classID, messageID uuid.UUID) (bool, error)
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository builds a GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

const selectMessage = `m.id, m.center_id, m.class_id, m.author_id, m.body, m.created_at, m.deleted_at, t.full_name AS author_name`

func (r *gormRepository) List(ctx context.Context, sc authctx.Scope, classID uuid.UUID, before *uuid.UUID, limit int) ([]MessageRow, error) {
	q := database.FromContext(ctx, r.db).
		Table("class_messages AS m").
		Select(selectMessage).
		Joins("JOIN teachers t ON t.id = m.author_id").
		Where("m.center_id = ? AND m.class_id = ? AND m.deleted_at IS NULL", sc.CenterID, classID)
	if before != nil {
		// Keyset on the same (created_at, id) order the index serves; the
		// cursor row is looked up under the class so a foreign id matches
		// nothing instead of anchoring the page elsewhere.
		q = q.Where("(m.created_at, m.id) < (SELECT c.created_at, c.id FROM class_messages c WHERE c.id = ? AND c.class_id = m.class_id)", *before)
	}
	var rows []MessageRow
	err := q.Order("m.created_at DESC, m.id DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

func (r *gormRepository) Get(ctx context.Context, sc authctx.Scope, classID, messageID uuid.UUID) (*MessageRow, error) {
	var row MessageRow
	err := database.FromContext(ctx, r.db).
		Table("class_messages AS m").
		Select(selectMessage).
		Joins("JOIN teachers t ON t.id = m.author_id").
		Where("m.id = ? AND m.center_id = ? AND m.class_id = ? AND m.deleted_at IS NULL", messageID, sc.CenterID, classID).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *gormRepository) Create(ctx context.Context, m *Message) error {
	return database.FromContext(ctx, r.db).Create(m).Error
}

func (r *gormRepository) SoftDelete(ctx context.Context, sc authctx.Scope, classID, messageID uuid.UUID) (bool, error) {
	res := database.FromContext(ctx, r.db).
		Model(&Message{}).
		Where("id = ? AND center_id = ? AND class_id = ? AND deleted_at IS NULL", messageID, sc.CenterID, classID).
		Update("deleted_at", gorm.Expr("now()"))
	return res.RowsAffected > 0, res.Error
}
