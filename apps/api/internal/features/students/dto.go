package students

import (
	"time"

	"github.com/google/uuid"
)

// CreateRequest is the closed field list from PRD R1: full name, owning
// contact, and the attendance-screen disambiguator. Any proposed addition must
// answer "how does this field serve fee calculation" — if it cannot, it does
// not go in. dto_fields_test.go pins this set.
type CreateRequest struct {
	FullName    string    `json:"full_name" binding:"required,min=1,max=100"`
	ContactID   uuid.UUID `json:"contact_id" binding:"required"`
	DisplayNote string    `json:"display_note" binding:"omitempty,max=50"`
}

// UpdateRequest carries the same closed field list as CreateRequest.
type UpdateRequest struct {
	FullName    string    `json:"full_name" binding:"required,min=1,max=100"`
	ContactID   uuid.UUID `json:"contact_id" binding:"required"`
	DisplayNote string    `json:"display_note" binding:"omitempty,max=50"`
}

// StudentResponse is the public student shape, carrying the contact's name and
// phone so the roster screen needs no second call.
type StudentResponse struct {
	ID           uuid.UUID `json:"id"`
	FullName     string    `json:"full_name"`
	DisplayNote  string    `json:"display_note"`
	ContactID    uuid.UUID `json:"contact_id"`
	ContactName  string    `json:"contact_name"`
	ContactPhone string    `json:"contact_phone"`
	CreatedAt    time.Time `json:"created_at"`
}

// FromRow maps a joined row onto the response DTO.
func FromRow(row *Row) StudentResponse {
	note := ""
	if row.DisplayNote != nil {
		note = *row.DisplayNote
	}
	return StudentResponse{
		ID:           row.ID,
		FullName:     row.FullName,
		DisplayNote:  note,
		ContactID:    row.ContactID,
		ContactName:  row.ContactName,
		ContactPhone: row.ContactPhone,
		CreatedAt:    row.CreatedAt,
	}
}

// notePtr converts the wire form ("" = unset) to the nullable column.
func notePtr(note string) *string {
	if note == "" {
		return nil
	}
	return &note
}
