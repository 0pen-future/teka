// Package courses owns the center's course catalog (danh mục khóa học): the
// products a center sells. A course carries a code, a default unit price, an
// optional default program template version and a set of tuition packs;
// classes attach to a course and inherit its unit price when opened without
// one. The catalog is center-wide — every holder of courses.read sees all of
// it — and archiving a course never touches the classes already running.
package courses

import (
	"time"

	"github.com/google/uuid"
)

// Course statuses. draft → active → archived; an archived course stops
// appearing in the class dialog but its classes keep running.
const (
	StatusDraft    = "draft"
	StatusActive   = "active"
	StatusArchived = "archived"
)

// Course is one courses row. DeletedAt is a plain timestamp, not
// gorm.DeletedAt: the repository filters live rows explicitly so the soft
// delete stays visible in every query it touches.
type Course struct {
	ID                       uuid.UUID
	CenterID                 uuid.UUID
	Code                     string
	Name                     string
	Subject                  *string
	Level                    *string
	Description              *string
	Status                   string
	DefaultTemplateVersionID *uuid.UUID
	DefaultUnitPrice         int64
	TotalSessions            *int
	DurationMin              *int
	CreatedAt                time.Time
	UpdatedAt                time.Time
	DeletedAt                *time.Time
}

// TableName maps the model onto courses.
func (Course) TableName() string { return "courses" }

// CourseRow is a course with the derived columns the list and detail
// responses carry: how many of its classes are running or upcoming today,
// and the template behind its default version. The template columns are
// nil when the template was deleted, while DefaultTemplateVersionID keeps
// the stored id.
type CourseRow struct {
	Course
	ClassesRunning        int
	ClassesUpcoming       int
	TemplateID            *uuid.UUID
	TemplateCode          *string
	TemplateName          *string
	TemplateVersionNo     *int
	TemplateVersionStatus *string
}

// TuitionPack is one course_tuition_packs row: "n sessions for x đồng",
// shown on the course's settings tab. The list is replaced wholesale, so
// positions are always 1..n.
type TuitionPack struct {
	ID       uuid.UUID
	CourseID uuid.UUID
	CenterID uuid.UUID
	Name     string
	Sessions int
	Price    int64
	Position int
}

// TableName maps the model onto course_tuition_packs.
func (TuitionPack) TableName() string { return "course_tuition_packs" }

// TemplateVersionRef is the slice of a program template version the course
// needs to validate and display its default template.
type TemplateVersionRef struct {
	ID           uuid.UUID
	TemplateID   uuid.UUID
	VersionNo    int
	Status       string
	TemplateCode string
	TemplateName string
}
