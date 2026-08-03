// Package students manages the teacher's student roster over PRD R1's closed
// field list — full name, contact, display note — and the anonymising delete
// that scrubs PII while financial foreign keys survive.
package students

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AnonymizedName replaces full_name on delete. It must stay a constant,
// non-identifying value: initials, hashes, or truncations of the real name are
// correlatable and therefore not erasure.
const AnonymizedName = "Đã xoá"

// Student mirrors the students table. UserID is never written in V1 — students
// have no accounts by design. DisplayNote is the attendance-screen
// disambiguator ("An lớp 9A"), the only free-text field the closed list allows.
type Student struct {
	ID           uuid.UUID `gorm:"primaryKey"`
	TeacherID    uuid.UUID
	ContactID    uuid.UUID
	UserID       *uuid.UUID
	FullName     string
	DisplayNote  *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    gorm.DeletedAt
	AnonymizedAt *time.Time
}

// TableName pins the table explicitly.
func (Student) TableName() string { return "students" }
