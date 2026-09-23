package classinvites

import (
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/authctx"
)

// SendRequest names the member, the role proposed, and an optional note.
type SendRequest struct {
	TeacherID uuid.UUID `json:"teacher_id" binding:"required"`
	RoleKey   string    `json:"role_key" binding:"required,max=32"`
	Message   *string   `json:"message" binding:"omitempty,max=500"`
}

// ListQuery filters the invitation list. Both fields are optional; class_id
// is parsed by the service so a malformed value is a 422 with a field
// message rather than gin's opaque 400.
type ListQuery struct {
	Status  string `form:"status" binding:"omitempty,max=12"`
	ClassID string `form:"class_id" binding:"omitempty,max=36"`
}

// InvitationResponse is one invitation as the API returns it.
type InvitationResponse struct {
	ID            uuid.UUID  `json:"id"`
	ClassID       uuid.UUID  `json:"class_id"`
	ClassName     string     `json:"class_name"`
	TeacherID     uuid.UUID  `json:"teacher_id"`
	TeacherName   string     `json:"teacher_name"`
	RoleKey       string     `json:"role_key"`
	RoleLabel     string     `json:"role_label"`
	Status        string     `json:"status"`
	InvitedBy     uuid.UUID  `json:"invited_by"`
	InvitedByName string     `json:"invited_by_name"`
	Message       *string    `json:"message"`
	SentAt        time.Time  `json:"sent_at"`
	RemindedAt    *time.Time `json:"reminded_at"`
	RespondedAt   *time.Time `json:"responded_at"`
	AssignedAt    *time.Time `json:"assigned_at"`
}

// ConfirmResponse is the assigned invitation plus what the stint write moved:
// a giao_vien confirm is a handoff and carries its future planned sessions.
type ConfirmResponse struct {
	InvitationResponse
	MovedPlannedSessions int64 `json:"moved_planned_sessions"`
}

func toResponse(row Row) InvitationResponse {
	return InvitationResponse{
		ID:            row.ID,
		ClassID:       row.ClassID,
		ClassName:     row.ClassName,
		TeacherID:     row.TeacherID,
		TeacherName:   row.TeacherName,
		RoleKey:       row.RoleKey,
		RoleLabel:     authctx.StaffRoleLabel(row.RoleKey),
		Status:        row.Status,
		InvitedBy:     row.InvitedBy,
		InvitedByName: row.InvitedByName,
		Message:       row.Message,
		SentAt:        row.SentAt,
		RemindedAt:    row.RemindedAt,
		RespondedAt:   row.RespondedAt,
		AssignedAt:    row.AssignedAt,
	}
}
