// Package classes manages classes and their weekly schedules: the lớp with
// its opening date (ngày khai giảng), default per-session price in đồng, and
// the fixed timetable that session generation turns into concrete
// class_sessions. Schedule changes are modelled as closing the current row's
// effective range and inserting a replacement — never as an in-place edit —
// so past sessions stay explicable by the row that was effective back then.
package classes

import (
	"database/sql/driver"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Class status values, mirroring the CHECK constraint on classes.status.
// Archiving is the normal end-of-term action and keeps the class in history;
// soft delete is reserved for classes created by mistake.
const (
	StatusActive   = "active"
	StatusArchived = "archived"
)

// Class is one lớp học. Money is BIGINT đồng — never a float anywhere.
type Class struct {
	// ID is a UUIDv7 generated in Go via id.New(); the column has no default.
	ID        uuid.UUID `gorm:"primaryKey"`
	TeacherID uuid.UUID
	Name      string
	// StartDate/EndDate are Postgres DATE columns; only the date part is
	// meaningful.
	StartDate        time.Time
	EndDate          *time.Time
	DefaultUnitPrice int64
	Status           string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	DeletedAt        gorm.DeletedAt
	// Schedules holds the class's live schedule rows when preloaded.
	Schedules []Schedule `gorm:"foreignKey:ClassID"`
}

// TableName pins the table explicitly so a later model rename cannot silently
// break the mapping.
func (Class) TableName() string { return "classes" }

// Schedule is one weekly timetable row, effective over
// [effective_from, effective_to] with NULL effective_to meaning open-ended.
type Schedule struct {
	ID        uuid.UUID `gorm:"primaryKey"`
	TeacherID uuid.UUID
	ClassID   uuid.UUID
	// Weekday uses 0 = Chủ nhật (Sunday), deliberately matching Go's
	// time.Weekday where int(time.Sunday) == 0 — convert with a direct cast,
	// never an offset.
	Weekday       int16
	StartTime     TimeOfDay
	DurationMin   int16
	EffectiveFrom time.Time
	EffectiveTo   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     gorm.DeletedAt
}

// TableName pins the table explicitly.
func (Schedule) TableName() string { return "class_schedules" }

// TimeOfDay is a wall-clock "HH:MM" string backed by a Postgres TIME column.
// A time.Time would carry a zero date and invite timezone conversion; the
// string form has neither problem and is validated with the hhmm rule at the
// API boundary.
type TimeOfDay string

// Value writes the "HH:MM" form, which Postgres accepts for TIME directly.
func (t TimeOfDay) Value() (driver.Value, error) { return string(t), nil }

// Scan accepts the driver's representation of TIME — string, []byte, or
// time.Time depending on driver mode — and normalises to "HH:MM".
func (t *TimeOfDay) Scan(src any) error {
	switch v := src.(type) {
	case string:
		return t.fromString(v)
	case []byte:
		return t.fromString(string(v))
	case time.Time:
		*t = TimeOfDay(v.Format("15:04"))
		return nil
	default:
		return fmt.Errorf("cannot scan %T into TimeOfDay", src)
	}
}

// fromString truncates "HH:MM:SS" to "HH:MM"; seconds are never used.
func (t *TimeOfDay) fromString(s string) error {
	if len(s) < 5 {
		return fmt.Errorf("cannot scan %q into TimeOfDay", s)
	}
	*t = TimeOfDay(s[:5])
	return nil
}
