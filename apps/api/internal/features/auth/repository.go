package auth

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
)

// ErrTokenNotFound is returned when no refresh token matches a hash.
var ErrTokenNotFound = errors.New("refresh token not found")

// ErrTokenAlreadyRevoked is returned by Revoke when the token was revoked in
// the meantime — a concurrent rotation of the same token, i.e. reuse.
var ErrTokenAlreadyRevoked = errors.New("refresh token already revoked")

// Repository is the persistence contract for refresh tokens.
type Repository interface {
	Create(ctx context.Context, t *RefreshToken) error
	GetByHash(ctx context.Context, hash string) (*RefreshToken, error)
	// Revoke invalidates one live token; ErrTokenAlreadyRevoked when it lost
	// a race with another revocation.
	Revoke(ctx context.Context, id uuid.UUID, at time.Time) error
	RevokeFamily(ctx context.Context, familyID uuid.UUID, at time.Time) error
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
