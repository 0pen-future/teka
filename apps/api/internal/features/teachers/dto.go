package teachers

import (
	"time"

	"github.com/google/uuid"
)

// TeacherResponse is the public teacher representation; the password hash
// never leaves the service layer.
type TeacherResponse struct {
	ID        uuid.UUID `json:"id"`
	Phone     string    `json:"phone"`
	FullName  string    `json:"full_name"`
	Timezone  string    `json:"timezone"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

// UpdateProfileRequest is the payload for PUT /me. It deliberately carries
// only these two fields — phone moves the login identifier (out of scope) and
// binding role/status from the client would be privilege escalation.
type UpdateProfileRequest struct {
	FullName string `json:"full_name" binding:"required,min=1,max=100"`
	Timezone string `json:"timezone" binding:"required,max=64"`
}

// FromModel maps an account + teacher pair onto the public representation.
func FromModel(acct *Account, t *Teacher) TeacherResponse {
	return TeacherResponse{
		ID:        acct.ID,
		Phone:     acct.Phone,
		FullName:  t.FullName,
		Timezone:  t.Timezone,
		Status:    acct.Status,
		CreatedAt: acct.CreatedAt,
	}
}
