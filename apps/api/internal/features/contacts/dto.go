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

// ContactResponse is the public contact shape.
type ContactResponse struct {
	ID           uuid.UUID `json:"id"`
	FullName     string    `json:"full_name"`
	Phone        string    `json:"phone"`
	StudentCount int64     `json:"student_count"`
	CreatedAt    time.Time `json:"created_at"`
}

// FromModel maps a repository row onto the response DTO.
func FromModel(row *Row) ContactResponse {
	return ContactResponse{
		ID:           row.ID,
		FullName:     row.FullName,
		Phone:        row.Phone,
		StudentCount: row.StudentCount,
		CreatedAt:    row.CreatedAt,
	}
}
