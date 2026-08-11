package centers

import (
	"time"

	"github.com/google/uuid"
)

// JoinRequest is the payload for POST /centers/join. The phone is the join
// handshake: the owner hands their number to the teacher, the teacher
// initiates. Accepts local (0xxxxxxxxx) or E.164 (+84xxxxxxxxx) form.
type JoinRequest struct {
	OwnerPhone string `json:"owner_phone" binding:"required,vnphone"`
}

// JoinResponse confirms the move; nothing about the center beyond its id —
// the new member reads the rest through GET /centers/me.
type JoinResponse struct {
	CenterID uuid.UUID `json:"center_id"`
	JoinedAt time.Time `json:"joined_at"`
}

// RenameRequest is the payload for PATCH /centers/me. Max mirrors the
// VARCHAR(255) column.
type RenameRequest struct {
	Name string `json:"name" binding:"required,min=1,max=255"`
}

// CenterResponse describes the caller's center from their point of view.
type CenterResponse struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	IsOwner bool      `json:"is_owner"`
}

// MemberResponse is one member of the caller's center.
type MemberResponse struct {
	ID       uuid.UUID `json:"id"`
	FullName string    `json:"full_name"`
	Phone    string    `json:"phone"`
	IsOwner  bool      `json:"is_owner"`
}

// MeResponse is the body of GET /centers/me.
type MeResponse struct {
	Center  CenterResponse   `json:"center"`
	Members []MemberResponse `json:"members"`
}
