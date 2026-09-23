// Package library owns the center's shared teaching library: program
// templates (chương trình mẫu), their versions and the template lessons
// (buổi học mẫu) each version carries. A template is a center-wide asset —
// every holder of library.read sees all of them — with at most one draft
// version at a time; a published version is immutable and an archived one
// keeps its lessons so classes bound to it can still read them.
package library

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/dbtypes"
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

// Version is one program_template_versions row. ScoreSet is the version's
// score components, read and written as one block.
type Version struct {
	ID          uuid.UUID
	TemplateID  uuid.UUID
	CenterID    uuid.UUID
	VersionNo   int
	Status      string
	Changelog   *string
	PublishedAt *time.Time
	ScoreSet    ScoreSet
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

// Material kinds: what the link points at.
const (
	MaterialKindLink  = "link"
	MaterialKindDoc   = "doc"
	MaterialKindVideo = "video"
	MaterialKindOther = "other"
)

// Log-field kinds: the input a teacher fills in a session log.
const (
	LogFieldText     = "text"
	LogFieldNumber   = "number"
	LogFieldSelect   = "select"
	LogFieldCheckbox = "checkbox"
)

// Material is one library_materials row: a center-wide link (or reference)
// with a description, never an uploaded file. Soft-deleted like templates.
type Material struct {
	ID          uuid.UUID
	CenterID    uuid.UUID
	Title       string
	Kind        string
	URL         *string
	Description *string
	Tags        dbtypes.StringList
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

// TableName maps the model onto library_materials.
func (Material) TableName() string { return "library_materials" }

// Exercise is one library_exercises row: a center-wide exercise with an
// optional 1..5 difficulty. Soft-deleted like templates.
type Exercise struct {
	ID          uuid.UUID
	CenterID    uuid.UUID
	Title       string
	Description *string
	Difficulty  *int
	Tags        dbtypes.StringList
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

// TableName maps the model onto library_exercises.
func (Exercise) TableName() string { return "library_exercises" }

// LessonMaterial links one material to one template lesson; the whole
// list of a lesson is replaced at once, ordered by Position.
type LessonMaterial struct {
	LessonID           uuid.UUID
	MaterialID         uuid.UUID
	CenterID           uuid.UUID
	SharedWithStudents bool
	Position           int
}

// TableName maps the model onto template_lesson_materials.
func (LessonMaterial) TableName() string { return "template_lesson_materials" }

// LessonMaterialRow is a link joined with its live material.
type LessonMaterialRow struct {
	Material
	LessonID           uuid.UUID
	SharedWithStudents bool
	Position           int
}

// LessonExercise links one exercise to one template lesson.
type LessonExercise struct {
	LessonID   uuid.UUID
	ExerciseID uuid.UUID
	CenterID   uuid.UUID
	Position   int
}

// TableName maps the model onto template_lesson_exercises.
func (LessonExercise) TableName() string { return "template_lesson_exercises" }

// LessonExerciseRow is a link joined with its live exercise.
type LessonExerciseRow struct {
	Exercise
	LessonID uuid.UUID
	Position int
}

// LogField is one template_log_fields row: an input the session log of a
// class bound to this version asks the teacher for. Position is 1-based
// and contiguous within a version; the list is replaced as a whole.
type LogField struct {
	ID        uuid.UUID
	VersionID uuid.UUID
	CenterID  uuid.UUID
	Position  int
	Label     string
	Kind      string
	Options   dbtypes.StringList
	Required  bool
}

// TableName maps the model onto template_log_fields.
func (LogField) TableName() string { return "template_log_fields" }

// ScoreComponent is one entry of a version's score set: a stable key the
// class grading can refer to, a label, the maximum score and a weight.
type ScoreComponent struct {
	Key    string  `json:"key"`
	Label  string  `json:"label"`
	Max    float64 `json:"max"`
	Weight float64 `json:"weight"`
}

// ScoreSet maps the score_set JSONB column. A nil set writes as [] so the
// NOT NULL DEFAULT '[]' column never sees a SQL NULL.
type ScoreSet []ScoreComponent

// Value marshals the set.
func (s ScoreSet) Value() (driver.Value, error) {
	if s == nil {
		s = ScoreSet{}
	}
	return json.Marshal(s)
}

// Scan accepts the []byte/string forms the pgx/gorm stack hands over.
func (s *ScoreSet) Scan(value any) error {
	switch v := value.(type) {
	case nil:
		*s = nil
		return nil
	case []byte:
		return json.Unmarshal(v, s)
	case string:
		return json.Unmarshal([]byte(v), s)
	default:
		return fmt.Errorf("cannot scan %T into ScoreSet", value)
	}
}

// nowUTC is the single clock for response stamps that do not come from the
// database.
func nowUTC() time.Time { return time.Now().UTC() }
