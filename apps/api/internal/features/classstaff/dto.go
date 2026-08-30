package classstaff

import (
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/authctx"
)

// AssignRequest names the member and the role the owner assigns. giao_vien is
// deliberately not assignable here — the handoff flow owns it.
type AssignRequest struct {
	TeacherID uuid.UUID `json:"teacher_id" binding:"required"`
	RoleKey   string    `json:"role_key" binding:"required,max=32"`
}

// StaffResponse is one staff stint of the class. EndedAt non-null marks a
// soft-closed stint that still grants history reads.
type StaffResponse struct {
	ID          uuid.UUID  `json:"id"`
	TeacherID   uuid.UUID  `json:"teacher_id"`
	TeacherName string     `json:"teacher_name"`
	RoleKey     string     `json:"role_key"`
	RoleLabel   string     `json:"role_label"`
	StartedAt   time.Time  `json:"started_at"`
	EndedAt     *time.Time `json:"ended_at"`
}

func toResponse(row StaffRow) StaffResponse {
	return StaffResponse{
		ID:          row.ID,
		TeacherID:   row.TeacherID,
		TeacherName: row.TeacherName,
		RoleKey:     row.RoleKey,
		RoleLabel:   authctx.StaffRoleLabel(row.RoleKey),
		StartedAt:   row.StartedAt,
		EndedAt:     row.EndedAt,
	}
}
