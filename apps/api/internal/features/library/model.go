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
	DraftVersionID     *uuid.UUID
	VersionCount       int
	// ClassCount is how many classes are bound to any version of this
	// template. LessonCount is the lesson count of the released (published)
	// version if one exists, else the open draft's. Versions is every
	// version of the template, newest first.
	ClassCount  int
	LessonCount int
	// Versions is populated by the service layer (versionRefs), never by
	// GORM scanning: without gorm:"-" it tries to treat the slice as an
	// association and logs a schema error on every query.
	Versions []VersionRef `gorm:"-"`
}

// VersionRef is a compact reference to one version of a template, used to
// list every version a template has without loading each version in full.
type VersionRef struct {
	ID        uuid.UUID
	VersionNo int
	Status    string
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

// VersionRow is a version with its lesson count and how many classes are
// bound to it.
type VersionRow struct {
	Version
	LessonCount int
	ClassCount  int
}

// VersionClassRef is a class bound to a version, projected for the version
// detail response's class list.
type VersionClassRef struct {
	ID   uuid.UUID
	Name string
}

// VersionClassLink is a VersionClassRef tagged with the version it belongs
// to, for a batched query across several versions at once.
type VersionClassLink struct {
	VersionID uuid.UUID
	ID        uuid.UUID
	Name      string
}

// Lesson modes: whether a lesson happens in class at a fixed time
// (scheduled) or the student works through it on their own (self_study).
const (
	LessonModeScheduled = "scheduled"
	LessonModeSelfStudy = "self_study"
)

// Lesson is one template_lessons row. Position is 1-based and contiguous
// within a version; the service renumbers on delete, reorder and duplicate.
type Lesson struct {
	ID           uuid.UUID
	VersionID    uuid.UUID
	CenterID     uuid.UUID
	Position     int
	Title        string
	Mode         string
	Unit         *string
	Objectives   *string
	DurationMin  *int
	HomeworkNote *string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// LessonRow is a lesson with how many materials and exercises it links.
type LessonRow struct {
	Lesson
	MaterialCount int
	ExerciseCount int
}

// TableName maps the model onto template_lessons.
func (Lesson) TableName() string { return "template_lessons" }

// Material kinds: what the link points at. MaterialKindOther stays valid for
// legacy rows but is no longer offered on new input (see MaterialRequest).
const (
	MaterialKindLink  = "link"
	MaterialKindDoc   = "doc"
	MaterialKindVideo = "video"
	MaterialKindAudio = "audio"
	MaterialKindImage = "image"
	MaterialKindNote  = "note"
	MaterialKindLive  = "live"
	MaterialKindOther = "other"
)

// Log-field kinds: the input a teacher fills in a session log.
const (
	LogFieldText     = "text"
	LogFieldLongText = "long_text"
	LogFieldNumber   = "number"
	LogFieldSelect   = "select"
	LogFieldCheckbox = "checkbox"
	LogFieldStudent  = "student"
)

// Material is one library_materials row: a center-wide link (or reference)
// with a description, never an uploaded file. Soft-deleted like templates.
// Active toggles visibility in the bank and the lesson pickers without
// deleting the row.
type Material struct {
	ID          uuid.UUID
	CenterID    uuid.UUID
	Title       string
	Kind        string
	URL         *string
	Description *string
	Tags        dbtypes.StringList
	Active      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

// TableName maps the model onto library_materials.
func (Material) TableName() string { return "library_materials" }

// MaterialRow is a material with the counts the bank table shows: how many
// live template lessons link it, and across how many distinct templates.
type MaterialRow struct {
	Material
	LessonCount   int
	TemplateCount int
}

// Exercise is one library_exercises row: a center-wide exercise with an
// optional 1..5 difficulty, a center-unique display code, free-text skill
// and level, and an Active flag. Soft-deleted like templates.
type Exercise struct {
	ID          uuid.UUID
	CenterID    uuid.UUID
	Title       string
	Description *string
	Difficulty  *int
	Code        string
	Skill       *string
	Level       *string
	Tags        dbtypes.StringList
	Active      bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

// TableName maps the model onto library_exercises.
func (Exercise) TableName() string { return "library_exercises" }

// ExerciseRow is an exercise with the counts the bank table shows.
type ExerciseRow struct {
	Exercise
	LessonCount   int
	TemplateCount int
}

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

// LessonExercise links one exercise to one template lesson. GroupID is
// optional: it buckets the link under one of the version's exercise groups
// and is nulled automatically (not cascaded) when that group is deleted.
type LessonExercise struct {
	LessonID   uuid.UUID
	ExerciseID uuid.UUID
	CenterID   uuid.UUID
	GroupID    *uuid.UUID
	Position   int
}

// TableName maps the model onto template_lesson_exercises.
func (LessonExercise) TableName() string { return "template_lesson_exercises" }

// LessonExerciseRow is a link joined with its live exercise.
type LessonExerciseRow struct {
	Exercise
	LessonID uuid.UUID
	GroupID  *uuid.UUID
	Position int
}

// ExerciseGroup is one template_exercise_groups row: a named bucket of
// exercises within a version (e.g. "Khởi động", "Bài tập về nhà"). Position
// is 1-based and contiguous within a version, like Lesson.Position.
type ExerciseGroup struct {
	ID        uuid.UUID
	VersionID uuid.UUID
	CenterID  uuid.UUID
	Name      string
	Position  int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName maps the model onto template_exercise_groups.
func (ExerciseGroup) TableName() string { return "template_exercise_groups" }

// ExerciseGroupRow is a group with how many live lesson-exercise links (of
// any lesson in its version) point at it.
type ExerciseGroupRow struct {
	ExerciseGroup
	ExerciseCount int
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

// ScoreComponent is one entry of a score set group: a stable key the class
// grading can refer to, a label, the maximum score and a weight.
type ScoreComponent struct {
	Key    string  `json:"key"`
	Label  string  `json:"label"`
	Max    float64 `json:"max"`
	Weight float64 `json:"weight"`
}

// ScoreSetGroup is one named bucket of score components within a version
// (e.g. "Giữa kỳ", "Cuối kỳ"): a stable key, a display title and its
// components.
type ScoreSetGroup struct {
	Key        string           `json:"key"`
	Title      string           `json:"title"`
	Components []ScoreComponent `json:"components"`
}

// ScoreSet maps the score_set JSONB column: an ordered list of score
// groups. A nil set writes as [] so the NOT NULL DEFAULT '[]' column never
// sees a SQL NULL.
type ScoreSet []ScoreSetGroup

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
