// Package classinvites lets a center owner invite a member to take a role in
// a class (giáo viên chính, trợ giảng, học vụ). An invitation is a proposal:
// the invitee accepts or declines in the app, but no class_staff stint is
// written until the owner confirms — for giao_vien through the handoff
// feature, for the other roles through classstaff. The feature owns the
// class_invitations table only; every stint write goes through the feature
// that owns class_staff.
package classinvites

import (
	"time"

	"github.com/google/uuid"
)

// Invitation statuses. pending → accepted | declined | cancelled;
// pending | accepted → assigned (owner confirm); accepted → cancelled.
const (
	StatusPending   = "pending"
	StatusAccepted  = "accepted"
	StatusDeclined  = "declined"
	StatusCancelled = "cancelled"
	StatusAssigned  = "assigned"
)

// ValidStatus reports whether s is a known invitation status.
func ValidStatus(s string) bool {
	switch s {
	case StatusPending, StatusAccepted, StatusDeclined, StatusCancelled, StatusAssigned:
		return true
	}
	return false
}

// Invitation is one class_invitations row.
type Invitation struct {
	ID          uuid.UUID
	CenterID    uuid.UUID
	ClassID     uuid.UUID
	TeacherID   uuid.UUID
	RoleKey     string
	Status      string
	InvitedBy   uuid.UUID
	Message     *string
	SentAt      time.Time
	RemindedAt  *time.Time
	RespondedAt *time.Time
	AssignedAt  *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TableName maps the model onto class_invitations.
func (Invitation) TableName() string { return "class_invitations" }

// Row is an invitation joined with the display names the API returns.
type Row struct {
	Invitation
	ClassName     string
	TeacherName   string
	InvitedByName string
}

// nowUTC is the single clock for test fakes and response stamps that do not
// come from the database.
func nowUTC() time.Time { return time.Now().UTC() }
