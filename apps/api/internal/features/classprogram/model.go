package classprogram

import (
	"time"

	"github.com/google/uuid"
)

// Program links a class to the one published template version it applies.
// class_id is the primary key: a class carries at most one program, and
// re-applying replaces the row in place.
type Program struct {
	ClassID           uuid.UUID `gorm:"primaryKey"`
	CenterID          uuid.UUID
	TemplateVersionID uuid.UUID
	AppliedAt         time.Time
	AppliedBy         uuid.UUID
}

// TableName maps the model onto class_programs.
func (Program) TableName() string { return "class_programs" }

// ProgramRow is a Program joined with the applied version's number, its
// template's identity, and the version's lesson count for display.
type ProgramRow struct {
	Program
	TemplateID   uuid.UUID
	TemplateName string
	VersionNo    int
	LessonCount  int
}
