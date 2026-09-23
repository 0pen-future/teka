// Package library owns the center's shared teaching library: program
// templates (chương trình mẫu), their versions and the template lessons
// (buổi học mẫu) each version carries. A template is a center-wide asset —
// every holder of library.read sees all of them — with at most one draft
// version at a time; a published version is immutable and an archived one
// keeps its lessons so classes bound to it can still read them.
package library

import (
	"time"

	"github.com/google/uuid"
)

// Version statuses. draft → published → archived; a published or archived
// version never changes its lessons again.
const (
	StatusDraft     = "draft"
	StatusPublished = "published"
	StatusArchived  = "archived"
)

// Template is one program_templates row. DeletedAt is a plain timestamp, not
// gorm.DeletedAt: the repository filters live rows explicitly so the soft
// delete stays visible in every query it touches.
type Template struct {
	ID          uuid.UUID
	CenterID    uuid.UUID
	Code        string
	Name        string
	Subject     *string
	Level       *string
	Description *string
	CreatedBy   *uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

// TableName maps the model onto program_templates.
func (Template) TableName() string { return "program_templates" }

// TemplateRow is a template with the version summary the list and detail
// responses carry: which version is published (the highest published
// version_no) and whether a draft is open.
type TemplateRow struct {
	Template
	PublishedVersionNo *int
	DraftVersionNo     *int
	VersionCount       int
}

// Version is one program_template_versions row.
type Version struct {
	ID          uuid.UUID
	TemplateID  uuid.UUID
	CenterID    uuid.UUID
	VersionNo   int
	Status      string
	Changelog   *string
	PublishedAt *time.Time
	CreatedBy   *uuid.UUID
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TableName maps the model onto program_template_versions.
func (Version) TableName() string { return "program_template_versions" }

// Locked reports whether the version's lessons may no longer change.
func (v Version) Locked() bool { return v.Status != StatusDraft }

// VersionRow is a version with its lesson count.
type VersionRow struct {
	Version
	LessonCount int
}

// Lesson is one template_lessons row. Position is 1-based and contiguous
// within a version; the service renumbers on delete and reorder.
type Lesson struct {
	ID           uuid.UUID
	VersionID    uuid.UUID
	CenterID     uuid.UUID
	Position     int
	Title        string
	Objectives   *string
	DurationMin  *int
	HomeworkNote *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// TableName maps the model onto template_lessons.
func (Lesson) TableName() string { return "template_lessons" }

// nowUTC is the single clock for response stamps that do not come from the
// database.
func nowUTC() time.Time { return time.Now().UTC() }
