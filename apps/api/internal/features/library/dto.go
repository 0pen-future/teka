package library

import (
	"time"

	"github.com/google/uuid"
)

// TemplateRequest creates or fully replaces a template's own fields. Code
// follows the class-code shape (uppercase letters, digits, dashes) and is
// unique among the center's live templates.
type TemplateRequest struct {
	Code        string  `json:"code" binding:"required,min=2,max=20"`
	Name        string  `json:"name" binding:"required,min=1,max=200"`
	Subject     *string `json:"subject" binding:"omitempty,max=100"`
	Level       *string `json:"level" binding:"omitempty,max=100"`
	Description *string `json:"description" binding:"omitempty,max=2000"`
}

// TemplateResponse is the wire form of a template with its version summary.
type TemplateResponse struct {
	ID                 uuid.UUID  `json:"id"`
	Code               string     `json:"code"`
	Name               string     `json:"name"`
	Subject            *string    `json:"subject"`
	Level              *string    `json:"level"`
	Description        *string    `json:"description"`
	CreatedBy          *uuid.UUID `json:"created_by"`
	PublishedVersionNo *int       `json:"published_version_no"`
	DraftVersionNo     *int       `json:"draft_version_no"`
	VersionCount       int        `json:"version_count"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

func templateResponse(row *TemplateRow) TemplateResponse {
	return TemplateResponse{
		ID: row.ID, Code: row.Code, Name: row.Name,
		Subject: row.Subject, Level: row.Level, Description: row.Description,
		CreatedBy:          row.CreatedBy,
		PublishedVersionNo: row.PublishedVersionNo,
		DraftVersionNo:     row.DraftVersionNo,
		VersionCount:       row.VersionCount,
		CreatedAt:          row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

// CreateVersionRequest opens a new draft; the changelog is what changed
// against the previous published version.
type CreateVersionRequest struct {
	Changelog *string `json:"changelog" binding:"omitempty,max=2000"`
}

// VersionResponse is the wire form of a version.
type VersionResponse struct {
	ID          uuid.UUID  `json:"id"`
	TemplateID  uuid.UUID  `json:"template_id"`
	VersionNo   int        `json:"version_no"`
	Status      string     `json:"status"`
	Changelog   *string    `json:"changelog"`
	PublishedAt *time.Time `json:"published_at"`
	CreatedBy   *uuid.UUID `json:"created_by"`
	LessonCount int        `json:"lesson_count"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func versionResponse(row *VersionRow) VersionResponse {
	return VersionResponse{
		ID: row.ID, TemplateID: row.TemplateID, VersionNo: row.VersionNo,
		Status: row.Status, Changelog: row.Changelog, PublishedAt: row.PublishedAt,
		CreatedBy: row.CreatedBy, LessonCount: row.LessonCount,
		CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
}

// LessonRequest creates or fully replaces a template lesson's content.
// Position is never in the body: create appends, order changes go through
// the reorder endpoint.
type LessonRequest struct {
	Title        string  `json:"title" binding:"required,min=1,max=200"`
	Objectives   *string `json:"objectives" binding:"omitempty,max=4000"`
	DurationMin  *int    `json:"duration_min" binding:"omitempty,min=1,max=1440"`
	HomeworkNote *string `json:"homework_note" binding:"omitempty,max=4000"`
}

// LessonResponse is the wire form of a template lesson.
type LessonResponse struct {
	ID           uuid.UUID `json:"id"`
	VersionID    uuid.UUID `json:"version_id"`
	Position     int       `json:"position"`
	Title        string    `json:"title"`
	Objectives   *string   `json:"objectives"`
	DurationMin  *int      `json:"duration_min"`
	HomeworkNote *string   `json:"homework_note"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func lessonResponse(l *Lesson) LessonResponse {
	return LessonResponse{
		ID: l.ID, VersionID: l.VersionID, Position: l.Position, Title: l.Title,
		Objectives: l.Objectives, DurationMin: l.DurationMin, HomeworkNote: l.HomeworkNote,
		CreatedAt: l.CreatedAt, UpdatedAt: l.UpdatedAt,
	}
}

func lessonResponses(rows []Lesson) []LessonResponse {
	out := make([]LessonResponse, 0, len(rows))
	for i := range rows {
		out = append(out, lessonResponse(&rows[i]))
	}
	return out
}

// ReorderRequest is the complete new order of a version's lessons: every
// lesson id exactly once.
type ReorderRequest struct {
	LessonIDs []uuid.UUID `json:"lesson_ids" binding:"required,min=1"`
}

// MaterialRequest creates or fully replaces a library material. A material
// is a link (or a named reference when url is empty), never an upload.
type MaterialRequest struct {
	Title       string   `json:"title" binding:"required,min=1,max=200"`
	Kind        string   `json:"kind" binding:"required,oneof=link doc video other"`
	URL         *string  `json:"url" binding:"omitempty,max=2000"`
	Description *string  `json:"description" binding:"omitempty,max=4000"`
	Tags        []string `json:"tags" binding:"omitempty,max=20,dive,max=50"`
}

// MaterialResponse is the wire form of a material.
type MaterialResponse struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Kind        string    `json:"kind"`
	URL         *string   `json:"url"`
	Description *string   `json:"description"`
	Tags        []string  `json:"tags"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func materialResponse(m *Material) MaterialResponse {
	return MaterialResponse{
		ID: m.ID, Title: m.Title, Kind: m.Kind, URL: m.URL, Description: m.Description,
		Tags: tags(m.Tags), CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
	}
}

// ExerciseRequest creates or fully replaces a library exercise.
type ExerciseRequest struct {
	Title       string   `json:"title" binding:"required,min=1,max=200"`
	Description *string  `json:"description" binding:"omitempty,max=4000"`
	Difficulty  *int     `json:"difficulty" binding:"omitempty,min=1,max=5"`
	Tags        []string `json:"tags" binding:"omitempty,max=20,dive,max=50"`
}

// ExerciseResponse is the wire form of an exercise.
type ExerciseResponse struct {
	ID          uuid.UUID `json:"id"`
	Title       string    `json:"title"`
	Description *string   `json:"description"`
	Difficulty  *int      `json:"difficulty"`
	Tags        []string  `json:"tags"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func exerciseResponse(e *Exercise) ExerciseResponse {
	return ExerciseResponse{
		ID: e.ID, Title: e.Title, Description: e.Description, Difficulty: e.Difficulty,
		Tags: tags(e.Tags), CreatedAt: e.CreatedAt, UpdatedAt: e.UpdatedAt,
	}
}

// tags returns a non-nil slice so the wire form is always a JSON array.
func tags(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

// LessonMaterialInput is one entry of the wholesale-replace body of
// PUT /library/lessons/{lid}/materials; the body order is the display order.
type LessonMaterialInput struct {
	MaterialID         uuid.UUID `json:"material_id" binding:"required"`
	SharedWithStudents bool      `json:"shared_with_students"`
}

// LessonMaterialResponse is a material as attached to one lesson.
type LessonMaterialResponse struct {
	MaterialResponse
	SharedWithStudents bool `json:"shared_with_students"`
	Position           int  `json:"position"`
}

func lessonMaterialResponses(rows []LessonMaterialRow) []LessonMaterialResponse {
	out := make([]LessonMaterialResponse, 0, len(rows))
	for i := range rows {
		out = append(out, LessonMaterialResponse{
			MaterialResponse:   materialResponse(&rows[i].Material),
			SharedWithStudents: rows[i].SharedWithStudents,
			Position:           rows[i].Position,
		})
	}
	return out
}

// LessonExerciseInput is one entry of the wholesale-replace body of
// PUT /library/lessons/{lid}/exercises.
type LessonExerciseInput struct {
	ExerciseID uuid.UUID `json:"exercise_id" binding:"required"`
}

// LessonExerciseResponse is an exercise as attached to one lesson.
type LessonExerciseResponse struct {
	ExerciseResponse
	Position int `json:"position"`
}

func lessonExerciseResponses(rows []LessonExerciseRow) []LessonExerciseResponse {
	out := make([]LessonExerciseResponse, 0, len(rows))
	for i := range rows {
		out = append(out, LessonExerciseResponse{
			ExerciseResponse: exerciseResponse(&rows[i].Exercise),
			Position:         rows[i].Position,
		})
	}
	return out
}

// LessonDetailResponse is a lesson with everything attached to it.
type LessonDetailResponse struct {
	LessonResponse
	Materials []LessonMaterialResponse `json:"materials"`
	Exercises []LessonExerciseResponse `json:"exercises"`
}

// LogFieldInput is one entry of the wholesale-replace body of
// PUT /library/versions/{vid}/log-fields; the body order is the position.
// Options are only meaningful for kind select, where at least one is
// required; other kinds store an empty list.
type LogFieldInput struct {
	Label    string   `json:"label" binding:"required,min=1,max=100"`
	Kind     string   `json:"kind" binding:"required,oneof=text number select checkbox"`
	Options  []string `json:"options" binding:"omitempty,max=20,dive,max=100"`
	Required bool     `json:"required"`
}

// LogFieldResponse is the wire form of a log field.
type LogFieldResponse struct {
	ID       uuid.UUID `json:"id"`
	Position int       `json:"position"`
	Label    string    `json:"label"`
	Kind     string    `json:"kind"`
	Options  []string  `json:"options"`
	Required bool      `json:"required"`
}

func logFieldResponses(rows []LogField) []LogFieldResponse {
	out := make([]LogFieldResponse, 0, len(rows))
	for i := range rows {
		f := &rows[i]
		out = append(out, LogFieldResponse{
			ID: f.ID, Position: f.Position, Label: f.Label, Kind: f.Kind,
			Options: tags(f.Options), Required: f.Required,
		})
	}
	return out
}

// ScoreComponentInput is one entry of the wholesale-replace body of
// PUT /library/versions/{vid}/score-set. Key is the stable identifier a
// class's grading refers to: lowercase letters, digits and underscores,
// unique within the set.
type ScoreComponentInput struct {
	Key    string  `json:"key" binding:"required,min=1,max=30"`
	Label  string  `json:"label" binding:"required,min=1,max=100"`
	Max    float64 `json:"max" binding:"required,gt=0"`
	Weight float64 `json:"weight" binding:"gte=0"`
}

// VersionDetailResponse is a version with its score set, log fields and
// every lesson including attachments: what a class binding to the version
// will inherit.
type VersionDetailResponse struct {
	VersionResponse
	ScoreSet  []ScoreComponent       `json:"score_set"`
	LogFields []LogFieldResponse     `json:"log_fields"`
	Lessons   []LessonDetailResponse `json:"lessons"`
}
