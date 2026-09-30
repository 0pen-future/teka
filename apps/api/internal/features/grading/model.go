// Package grading owns the component-score feature: a per-class snapshot of
// the score components defined by the program template version applied to the
// class, and the per-student × component × session scores teachers and the
// owner enter in the classbook.
//
// The snapshot is deliberate: the template version's score set is the source;
// class_score_components is a copy taken when the version is applied, so the
// scores recorded against a class never depend on the template afterwards.
package grading

import (
	"time"

	"github.com/google/uuid"
)

// ClassComponent is a snapshot row: one component copied into a class when a
// program template version was applied. It carries its own name/position so
// it is self-sufficient.
type ClassComponent struct {
	ID        uuid.UUID `gorm:"primaryKey"`
	ClassID   uuid.UUID
	CenterID  uuid.UUID
	Name      string
	Position  int16
	CreatedAt time.Time
}

// TableName pins the table explicitly.
func (ClassComponent) TableName() string { return "class_score_components" }

// StudentScore is one student's 0–10 score for one component in one session.
// TeacherID/CenterID anchor the row in the session's own teacher and center at
// write time (guard FK to center_members), exactly like session_marks — even
// when the owner is the one entering the score (attribution of who entered it
// lives in the audit log, not on this row).
type StudentScore struct {
	ID          uuid.UUID `gorm:"primaryKey"`
	ClassID     uuid.UUID
	SessionID   uuid.UUID
	ComponentID uuid.UUID
	StudentID   uuid.UUID
	TeacherID   uuid.UUID
	CenterID    uuid.UUID
	Score       float64
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TableName pins the table explicitly.
func (StudentScore) TableName() string { return "student_scores" }
