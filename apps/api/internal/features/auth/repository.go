package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"teka/apps/api/internal/database"
)

// ErrTokenNotFound is returned when no refresh token matches a hash.
var ErrTokenNotFound = errors.New("refresh token not found")

// ErrTokenAlreadyRevoked is returned by Revoke when the token was revoked in
// the meantime — a concurrent rotation of the same token, i.e. reuse.
var ErrTokenAlreadyRevoked = errors.New("refresh token already revoked")

// ErrResetTokenNotFound is returned when no password reset token matches a
// hash.
var ErrResetTokenNotFound = errors.New("password reset token not found")

// ErrActiveResetTokenExists is returned by CreateResetToken when the insert
// loses a race to a concurrent request that already holds the one live token
// the partial unique index uq_password_reset_active permits per user. It lets
// the caller treat a benign forgot-password race as a no-op rather than an
// internal error, driver-agnostically.
var ErrActiveResetTokenExists = errors.New("an active password reset token already exists")

// Repository is the persistence contract for refresh tokens.
type Repository interface {
	Create(ctx context.Context, t *RefreshToken) error
	GetByHash(ctx context.Context, hash string) (*RefreshToken, error)
	// Revoke invalidates one live token; ErrTokenAlreadyRevoked when it lost
	// a race with another revocation.
	Revoke(ctx context.Context, id uuid.UUID, at time.Time) error
	RevokeFamily(ctx context.Context, familyID uuid.UUID, at time.Time) error
	// RevokeAllForUser invalidates every live refresh token across every
	// family for one account — account-wide, unlike RevokeFamily. Used when
	// an account is disabled or its password is reset, so every outstanding
	// session dies at once regardless of which device issued it.
	RevokeAllForUser(ctx context.Context, userID uuid.UUID, at time.Time) error

	// CreateResetToken inserts a new password-reset token row on the ambient
	// transaction. The caller must have superseded any existing live token for
	// the same user in the same transaction first, or the partial unique index
	// uq_password_reset_active rejects the insert. A concurrent request that
	// wins the race to that single live slot yields ErrActiveResetTokenExists,
	// which the caller can treat as a benign no-op.
	CreateResetToken(ctx context.Context, t *PasswordResetToken) error
	// LatestResetToken returns the most recently created reset token for a
	// user, live or not — ForgotPassword's cooldown check needs the newest
	// row's CreatedAt regardless of its live state. ErrResetTokenNotFound when
	// the user has never requested one.
	LatestResetToken(ctx context.Context, userID uuid.UUID) (*PasswordResetToken, error)
	// SupersedeResetTokens marks every live reset token for a user as
	// superseded at the given time, freeing the partial unique index slot for
	// a new insert in the same transaction. A no-op, not an error, when none
	// is live.
	SupersedeResetTokens(ctx context.Context, userID uuid.UUID, at time.Time) error
	// ConsumeResetTokenForUpdate loads a reset token by hash under
	// SELECT ... FOR UPDATE, so two concurrent redemptions of the same token
	// serialize inside their WithinTx and only one can flip it to used.
	// ErrResetTokenNotFound covers an unknown hash.
	ConsumeResetTokenForUpdate(ctx context.Context, hash string) (*PasswordResetToken, error)
	// MarkResetTokenUsed flips a reset token to used in the ambient tx.
	MarkResetTokenUsed(ctx context.Context, id uuid.UUID, at time.Time) error
}

type gormRepository struct {
	db *gorm.DB
}

// NewRepository returns the GORM-backed refresh-token Repository.
func NewRepository(db *gorm.DB) Repository {
	return &gormRepository{db: db}
}

func (r *gormRepository) Create(ctx context.Context, t *RefreshToken) error {
	return database.FromContext(ctx, r.db).Create(t).Error
}

func (r *gormRepository) GetByHash(ctx context.Context, hash string) (*RefreshToken, error) {
	var t RefreshToken
	err := database.FromContext(ctx, r.db).First(&t, "token_hash = ?", hash).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrTokenNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *gormRepository) Revoke(ctx context.Context, id uuid.UUID, at time.Time) error {
	res := database.FromContext(ctx, r.db).
		Model(&RefreshToken{}).
		Where("id = ? AND revoked_at IS NULL", id).
		Update("revoked_at", at)
	if res.Error != nil {
		return res.Error
	}
	// Zero rows means a concurrent request revoked it first: the row lock
	// serializes the two UPDATEs and the loser re-reads the committed row.
	if res.RowsAffected == 0 {
		return ErrTokenAlreadyRevoked
	}
	return nil
}

func (r *gormRepository) RevokeFamily(ctx context.Context, familyID uuid.UUID, at time.Time) error {
	return database.FromContext(ctx, r.db).
		Model(&RefreshToken{}).
		Where("family_id = ? AND revoked_at IS NULL", familyID).
		Update("revoked_at", at).Error
}

func (r *gormRepository) RevokeAllForUser(ctx context.Context, userID uuid.UUID, at time.Time) error {
	return database.FromContext(ctx, r.db).
		Model(&RefreshToken{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Update("revoked_at", at).Error
}

func (r *gormRepository) CreateResetToken(ctx context.Context, t *PasswordResetToken) error {
	err := database.FromContext(ctx, r.db).Create(t).Error
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return ErrActiveResetTokenExists
	}
	return err
}

func (r *gormRepository) LatestResetToken(ctx context.Context, userID uuid.UUID) (*PasswordResetToken, error) {
	var t PasswordResetToken
	err := database.FromContext(ctx, r.db).
		Where("user_id = ?", userID).
		Order("created_at DESC").
		First(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrResetTokenNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *gormRepository) SupersedeResetTokens(ctx context.Context, userID uuid.UUID, at time.Time) error {
	return database.FromContext(ctx, r.db).
		Model(&PasswordResetToken{}).
		Where("user_id = ? AND used_at IS NULL AND superseded_at IS NULL", userID).
		Update("superseded_at", at).Error
}

func (r *gormRepository) ConsumeResetTokenForUpdate(ctx context.Context, hash string) (*PasswordResetToken, error) {
	var t PasswordResetToken
	err := database.FromContext(ctx, r.db).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("token_hash = ?", hash).
		Take(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrResetTokenNotFound
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *gormRepository) MarkResetTokenUsed(ctx context.Context, id uuid.UUID, at time.Time) error {
	res := database.FromContext(ctx, r.db).
		Model(&PasswordResetToken{}).
		Where("id = ?", id).
		Update("used_at", at)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrResetTokenNotFound
	}
	return nil
}
