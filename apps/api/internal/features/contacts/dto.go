package contacts

import (
	"time"

	"github.com/google/uuid"
)

// CreateRequest creates a contact. Length caps mirror VARCHAR(100)/VARCHAR(20).
type CreateRequest struct {
	FullName string `json:"full_name" binding:"required,min=1,max=100"`
	Phone    string `json:"phone" binding:"required,vnphone"`
}

// UpdateRequest replaces both editable fields; partial updates are not
// supported on a two-field resource.
type UpdateRequest struct {
	FullName string `json:"full_name" binding:"required,min=1,max=100"`
	Phone    string `json:"phone" binding:"required,vnphone"`
}

// ZaloMappingRequest binds a contact to one Zalo friend. The picker UI is the
// source of both values; the backend deliberately does not re-validate them
// against the live friend list (that would make every contact edit depend on
// Zalo being up). Length caps mirror VARCHAR(32)/VARCHAR(100).
type ZaloMappingRequest struct {
	ZaloUserID string `json:"zalo_user_id" binding:"required,min=1,max=32"`
	ZaloName   string `json:"zalo_name" binding:"required,min=1,max=100"`
}

// ContactResponse is the public contact shape.
type ContactResponse struct {
	ID           uuid.UUID `json:"id"`
	FullName     string    `json:"full_name"`
	Phone        string    `json:"phone"`
	StudentCount int64     `json:"student_count"`
	ZaloUserID   string    `json:"zalo_user_id,omitempty"`
	ZaloName     string    `json:"zalo_name,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// FromModel maps a repository row onto the response DTO.
func FromModel(row *Row) ContactResponse {
	out := ContactResponse{
		ID:           row.ID,
		FullName:     row.FullName,
		Phone:        row.Phone,
		StudentCount: row.StudentCount,
		CreatedAt:    row.CreatedAt,
	}
	if row.ZaloUserID != nil {
		out.ZaloUserID = *row.ZaloUserID
	}
	if row.ZaloName != nil {
		out.ZaloName = *row.ZaloName
	}
	return out
}
