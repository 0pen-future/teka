// Package classstaff manages per-class staff assignments (class_staff): who
// works in a class and in what role. An assignment is a stint — ended_at NULL
// means active; a soft-closed stint keeps granting READ access to the class's
// history, which is why revoking a mistaken grant needs the separate void
// (hard-delete) mode.
//
// The giao_vien role is special during the dual-write window: it mirrors
// classes.teacher_id and changes only through the handoff feature, never
// through this package's assign/remove API.
package classstaff

import (
	"time"

	"github.com/google/uuid"
)

// Assignment is one staff stint in a class.
type Assignment struct {
	ID        uuid.UUID
	ClassID   uuid.UUID
	CenterID  uuid.UUID
	TeacherID uuid.UUID
	RoleKey   string
	StartedAt time.Time
	EndedAt   *time.Time
}

// TableName maps the model onto class_staff.
func (Assignment) TableName() string { return "class_staff" }

// StaffRow is an assignment joined with the teacher's display name — the
// listing shape the API returns.
type StaffRow struct {
	Assignment
	TeacherName string
}
