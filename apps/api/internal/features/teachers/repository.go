package teachers

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
)

// Repository is the persistence contract for teacher identity; the service
// depends on this interface, tests supply a fake.
type Repository interface {
	// GetByPhone loads the account and profile for a live (not soft-deleted)
	// account by its E.164 phone.
	GetByPhone(ctx context.Context, phone string) (*Profile, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Profile, error)
	// CreateAccountWithProfile inserts both rows on the context's transaction.
	// The caller supplies both ids already set to the same value.
	CreateAccountWithProfile(ctx context.Context, acct *Account, t *Teacher) error
	Update(ctx context.Context, t *Teacher) error
	TouchLastLogin(ctx context.Context, id uuid.UUID, at time.Time) error
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) GetByPhone(ctx context.Context, phone string) (*Profile, error) {
	var acct Account
	err := database.FromContext(ctx, r.db).First(&acct, "phone = ?", phone).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return r.attachTeacher(ctx, acct)
}

// scoped returns a query bound to one tenant. Every read is scoped to one
// teacher and skips soft-deleted rows (via gorm.DeletedAt on model queries —
// raw SQL and Table() queries must add deleted_at IS NULL by hand). Composite
// FKs stop cross-teacher writes; only this filter stops cross-teacher reads.
// On the identity tables the tenant key IS the primary key; repositories over
// domain tables (classes, students, ...) copy this helper with
// .Where("teacher_id = ?", teacherID) instead.
func (r *gormRepository) scoped(ctx context.Context, teacherID uuid.UUID) *gorm.DB {
	return database.FromContext(ctx, r.db).Where("id = ?", teacherID)
}

func (r *gormRepository) GetByID(ctx context.Context, id uuid.UUID) (*Profile, error) {
	var acct Account
	err := r.scoped(ctx, id).First(&acct).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return r.attachTeacher(ctx, acct)
}

func (r *gormRepository) attachTeacher(ctx context.Context, acct Account) (*Profile, error) {
	var t Teacher
	err := database.FromContext(ctx, r.db).First(&t, "id = ?", acct.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		// An account without its profile row is a broken invariant, not a
		// lookup miss; surface it as not-found to the caller regardless.
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &Profile{Account: acct, Teacher: t}, nil
}

func (r *gormRepository) CreateAccountWithProfile(ctx context.Context, acct *Account, t *Teacher) error {
	db := database.FromContext(ctx, r.db)
	if err := db.Create(acct).Error; err != nil {
		return translateError(err)
	}
	if err := db.Create(t).Error; err != nil {
		return translateError(err)
	}
	return nil
}

func (r *gormRepository) Update(ctx context.Context, t *Teacher) error {
	return database.FromContext(ctx, r.db).Save(t).Error
}

func (r *gormRepository) TouchLastLogin(ctx context.Context, id uuid.UUID, at time.Time) error {
	return database.FromContext(ctx, r.db).
		Model(&Account{}).
		Where("id = ?", id).
		Update("last_login_at", at).Error
}

// translateError maps the partial unique phone index violation onto
// ErrDuplicatePhone so callers stay driver-agnostic.
func translateError(err error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrDuplicatePhone
	}
	return err
}
