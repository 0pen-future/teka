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

// PasswordResetToken mirrors the password_reset_tokens table (000008). Only
// the sha256 hash of the opaque token is stored; the plaintext lives in the
// delivered link alone. used_at marks a successful redemption; superseded_at
// marks a token replaced by a newer request (cooldown/supersede). The partial
// unique index uq_password_reset_active enforces at most one live
// (used_at IS NULL AND superseded_at IS NULL) row per user — ForgotPassword
// must supersede any existing live token in the same transaction as the
// insert, or the index rejects the legitimate re-request.
type PasswordResetToken struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	UserID       uuid.UUID
	TokenHash    string
	ExpiresAt    time.Time
	UsedAt       *time.Time
	SupersededAt *time.Time
	CreatedAt    time.Time
}

// TableName pins the table explicitly so a later model rename cannot silently
// break the mapping.
func (PasswordResetToken) TableName() string { return "password_reset_tokens" }

// Live reports whether the token is still usable: unused, unsuperseded, and
// unexpired at now.
func (t *PasswordResetToken) Live(now time.Time) bool {
	return t.UsedAt == nil && t.SupersededAt == nil && now.Before(t.ExpiresAt)
}
