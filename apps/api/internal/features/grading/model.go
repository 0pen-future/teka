// Package grading owns the component-score feature: center-level "score sets"
// (named bundles of component names, e.g. IELTS → Listening/Speaking/Reading/
// Writing) that the owner curates, a per-class snapshot of a set taken at
// assignment time, and the per-student × component × session scores teachers
// and the owner enter in the classbook.
//
// The two-tier snapshot is deliberate: score_sets/score_set_components are the
// editable template; class_score_components is a copy taken when a set is
// assigned to a class, so editing or deleting the source set never disturbs a
// class already using it.
package grading

import (
	"time"

	"github.com/google/uuid"
)

// ScoreSet is a center-level template: a named bundle of component names. Soft
// deleted (deleted_at) so an assigned snapshot can still trace source_set_id.
type ScoreSet struct {
	ID        uuid.UUID `gorm:"primaryKey"`
	CenterID  uuid.UUID
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt *time.Time
}

// TableName pins the table explicitly so a later model rename cannot silently
// break the mapping.
func (ScoreSet) TableName() string { return "score_sets" }

// SetComponent is one component of a template set: just a name and an order.
// Editing a set replaces its whole component list (hard delete + insert); the
// per-class snapshots copied earlier are unaffected.
type SetComponent struct {
	ID       uuid.UUID `gorm:"primaryKey"`
	SetID    uuid.UUID
	Name     string
	Position int16
}

// TableName pins the table explicitly.
func (SetComponent) TableName() string { return "score_set_components" }

// ClassComponent is a snapshot row: one component copied into a class when a
// score set was assigned. SourceSetID is trace-only (SET NULL if the source
// set is hard-deleted); the snapshot carries its own name/position so it is
// self-sufficient.
type ClassComponent struct {
	ID          uuid.UUID `gorm:"primaryKey"`
	ClassID     uuid.UUID
	CenterID    uuid.UUID
	Name        string
	Position    int16
	SourceSetID *uuid.UUID
	CreatedAt   time.Time
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
