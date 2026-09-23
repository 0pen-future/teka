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
