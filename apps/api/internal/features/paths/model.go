// Package paths owns the center's learning paths (lộ trình học): an ordered
// list of stages, each recommending courses from the catalog, that tells
// staff which course a student should take next. A path is descriptive
// only — it never enrols anyone — and is center-wide: every holder of
// paths.read sees all of it.
package paths

import (
	"time"

	"github.com/google/uuid"
)

// Learning path statuses. draft → active → archived.
const (
	StatusDraft    = "draft"
	StatusActive   = "active"
	StatusArchived = "archived"
)

// LearningPath is one learning_paths row. DeletedAt is a plain timestamp,
// not gorm.DeletedAt: the repository filters live rows explicitly so the
// soft delete stays visible in every query it touches.
type LearningPath struct {
	ID          uuid.UUID
	CenterID    uuid.UUID
	Code        string
	Name        string
	Description *string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

// TableName pins the table so the model name does not have to match it.
func (LearningPath) TableName() string { return "learning_paths" }

// Stage is one path_stages row: positions are dense 1..n per path and
// unique through a deferred constraint, so a reorder swaps them inside one
// transaction.
type Stage struct {
	ID       uuid.UUID
	PathID   uuid.UUID
	CenterID uuid.UUID
	Position int
	Name     string
	Goal     *string
}

// TableName pins the table.
func (Stage) TableName() string { return "path_stages" }

// StageCourse links a course into a stage at a position.
type StageCourse struct {
	StageID  uuid.UUID `gorm:"primaryKey"`
	CourseID uuid.UUID `gorm:"primaryKey"`
	CenterID uuid.UUID
	Position int
}

// TableName pins the table.
func (StageCourse) TableName() string { return "path_stage_courses" }

// StageCourseRow is a stage link joined with the live course it points at.
type StageCourseRow struct {
	StageID  uuid.UUID
	Position int
	CourseID uuid.UUID
	Code     string
	Name     string
	Status   string
}
