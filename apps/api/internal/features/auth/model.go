// Package auth owns authentication: register/login/refresh/logout, access
// JWTs, and rotating refresh tokens with family revocation on reuse.
package auth

import (
	"time"

	"github.com/google/uuid"
)

// RefreshToken mirrors the refresh_tokens table (000002). Only the sha256
// hash of the opaque token is stored; rotation-issued tokens share FamilyID.
type RefreshToken struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID    uuid.UUID
	TokenHash string
	FamilyID  uuid.UUID
	ExpiresAt time.Time
	RevokedAt *time.Time
	CreatedAt time.Time
}

// Revoked reports whether the token has been invalidated.
func (t *RefreshToken) Revoked() bool { return t.RevokedAt != nil }

// Expired reports whether the token lifetime has passed at now.
func (t *RefreshToken) Expired(now time.Time) bool { return now.After(t.ExpiresAt) }
