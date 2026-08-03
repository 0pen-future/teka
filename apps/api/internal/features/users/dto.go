package users

import (
	"time"

	"github.com/google/uuid"
)

// CreateRequest is the admin user-creation payload.
type CreateRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8,max=72"`
	Name     string `json:"name" binding:"required,min=1,max=100"`
	Role     string `json:"role" binding:"omitempty,oneof=admin user"`
}

// UpdateRequest is a partial update; nil fields are left unchanged.
type UpdateRequest struct {
	Name *string `json:"name" binding:"omitempty,min=1,max=100"`
	// Role changes are admin-only; the service enforces it.
	Role *string `json:"role" binding:"omitempty,oneof=admin user"`
}

// Response is the public user representation; the password hash never leaves
// the service layer.
type Response struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FromModel maps a User onto its public representation.
func FromModel(u *User) Response {
	return Response{
		ID:        u.ID,
		Email:     u.Email,
		Name:      u.Name,
		Role:      u.Role,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}

// FromModels maps a slice of users; always returns a non-nil slice so list
// responses serialize as [] instead of null.
func FromModels(us []User) []Response {
	out := make([]Response, 0, len(us))
	for i := range us {
		out = append(out, FromModel(&us[i]))
	}
	return out
}
