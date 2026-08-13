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
	// SetStatus flips a live account's status; ErrNotFound when no live
	// account matches (already soft-deleted, or unknown id).
	SetStatus(ctx context.Context, id uuid.UUID, status string) error
	// ReactivateAccount rewrites the password hash and flips status to active
	// in one statement, guarded to only ever act on a currently-disabled
	// account; ErrNotFound when the account is not disabled (a concurrent
	// reactivation, or it was never disabled — the race guard for Reactivate).
	ReactivateAccount(ctx context.Context, id uuid.UUID, passwordHash string, at time.Time) error
	// SetPasswordHash rewrites the password hash of a currently-active
	// account; ErrNotFound when the account is not active (unknown id,
	// soft-deleted, or disabled) — the guard for password reset, which must
	// never resurrect a disabled account.
	SetPasswordHash(ctx context.Context, id uuid.UUID, passwordHash string, at time.Time) error
	// SetPasswordHashForRecovery rewrites the password hash of a live
	// (not soft-deleted) account regardless of active/disabled status,
	// without touching status — the operator `reset-password` CLI's write
	// path, which must be able to recover a disabled account (e.g. a
	// locked-out owner) without silently reactivating it. ErrNotFound when no
	// live account matches.
	SetPasswordHashForRecovery(ctx context.Context, id uuid.UUID, passwordHash string, at time.Time) error
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

func (r *gormRepository) SetStatus(ctx context.Context, id uuid.UUID, status string) error {
	res := database.FromContext(ctx, r.db).
		Model(&Account{}).
		Where("id = ?", id).
		Update("status", status)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) ReactivateAccount(ctx context.Context, id uuid.UUID, passwordHash string, at time.Time) error {
	res := database.FromContext(ctx, r.db).
		Model(&Account{}).
		Where("id = ? AND status = ?", id, StatusDisabled).
		Updates(map[string]any{
			"password_hash": passwordHash,
			"status":        StatusActive,
			"updated_at":    at,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) SetPasswordHash(ctx context.Context, id uuid.UUID, passwordHash string, at time.Time) error {
	res := database.FromContext(ctx, r.db).
		Model(&Account{}).
		Where("id = ? AND status = ?", id, StatusActive).
		Updates(map[string]any{
			"password_hash": passwordHash,
			"updated_at":    at,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *gormRepository) SetPasswordHashForRecovery(ctx context.Context, id uuid.UUID, passwordHash string, at time.Time) error {
	// No status filter (GORM's soft-delete scope still excludes a
	// soft-deleted row) — unlike SetPasswordHash, this must reach a disabled
	// account too, without changing its status.
	res := database.FromContext(ctx, r.db).
		Model(&Account{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"password_hash": passwordHash,
			"updated_at":    at,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// translateError maps the partial unique phone index violation onto
// ErrDuplicatePhone so callers stay driver-agnostic.
func translateError(err error) error {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrDuplicatePhone
	}
	return err
}
