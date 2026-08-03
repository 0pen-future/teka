// Package users owns the user resource: model, repository, service, DTOs,
// handlers, and routes. Schema lives in migrations 000001; the model mirrors
// it and is never auto-migrated.
package users

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/shared/authctx"
)

// Roles a user can hold; canonical values live in authctx (shared with the
// auth middleware), mirrored by the CHECK constraint in 000001.
const (
	RoleAdmin = authctx.RoleAdmin
	RoleUser  = authctx.RoleUser
)

// User mirrors the users table (000001_create_users). Email is citext with a
// partial unique index WHERE deleted_at IS NULL.
type User struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	Email        string
	PasswordHash string
	Name         string
	Role         string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt
}

// IsAdmin reports whether the user holds the admin role.
func (u *User) IsAdmin() bool { return u.Role == RoleAdmin }
