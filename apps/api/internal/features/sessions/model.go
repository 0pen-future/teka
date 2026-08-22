// Package sessions generates and manages class_sessions: the calendar rows a
// class's schedules materialise into, plus their planned/held/cancelled
// lifecycle. Generation is on-demand (no cron): a session row only exists once
// something asks for its date range.
package sessions

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/features/classes"
)

// Session lifecycle states — mirrors the class_sessions.status CHECK
// constraint in the baseline schema.
const (
	StatusPlanned   = "planned"
	StatusHeld      = "held"
	StatusCancelled = "cancelled"
)

// Session is one calendar occurrence of a class, generated from its
// schedules or added ad-hoc. StartTime reuses classes.TimeOfDay rather than a
// parallel "HH:MM" type since both wrap the same Postgres TIME column shape.
type Session struct {
	ID        uuid.UUID `gorm:"primaryKey"`
	TeacherID uuid.UUID
	// CenterID anchors the row in the center it was created in; it never
	// changes, even when the creating teacher later moves centers.
	CenterID              uuid.UUID
	ClassID               uuid.UUID
	SessionDate           time.Time
	StartTime             *classes.TimeOfDay
	Status                string
	CancelReason          *string
	AttendanceConfirmedAt *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
	DeletedAt             gorm.DeletedAt
}

// TableName pins the table explicitly so a later model rename cannot silently
// break the mapping.
func (Session) TableName() string { return "class_sessions" }
