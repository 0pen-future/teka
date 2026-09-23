package courses

import (
	"time"

	"github.com/google/uuid"
)

// CourseRequest creates or fully replaces a course's own fields. Code
// follows the class-code shape (uppercase letters, digits, dashes) and is
// unique among the center's live courses. default_template_version_id must
// be a published version of the center's library; tuition packs have their
// own endpoint.
type CourseRequest struct {
	Code        string  `json:"code" binding:"required,min=2,max=20"`
	Name        string  `json:"name" binding:"required,min=1,max=200"`
	Subject     *string `json:"subject" binding:"omitempty,max=100"`
	Level       *string `json:"level" binding:"omitempty,max=100"`
	Description *string `json:"description" binding:"omitempty,max=2000"`
	// Status left blank means draft on create and "keep as stored" on update.
	Status                   string     `json:"status" binding:"omitempty,oneof=draft active archived"`
	DefaultTemplateVersionID *uuid.UUID `json:"default_template_version_id"`
	DefaultUnitPrice         int64      `json:"default_unit_price" binding:"min=0"`
	TotalSessions            *int       `json:"total_sessions" binding:"omitempty,min=1,max=1000"`
	DurationMin              *int       `json:"duration_min" binding:"omitempty,min=1,max=1440"`
}

// TuitionPackInput is one pack of the wholesale-replaced list.
type TuitionPackInput struct {
	Name     string `json:"name" binding:"required,min=1,max=100"`
	Sessions int    `json:"sessions" binding:"required,min=1,max=1000"`
	Price    int64  `json:"price" binding:"min=0"`
}

// TuitionPackResponse is the wire form of a tuition pack.
type TuitionPackResponse struct {
	ID       uuid.UUID `json:"id"`
	Name     string    `json:"name"`
	Sessions int       `json:"sessions"`
	Price    int64     `json:"price"`
	Position int       `json:"position"`
}

// DefaultTemplateResponse names the template behind the course's default
// version so the list can show it without a second request. Status is the
// version's current status: it was published when chosen but may have been
// archived since, and the client says so instead of assuming.
type DefaultTemplateResponse struct {
	VersionID  uuid.UUID `json:"version_id"`
	VersionNo  int       `json:"version_no"`
	Status     string    `json:"status"`
	TemplateID uuid.UUID `json:"template_id"`
	Code       string    `json:"code"`
	Name       string    `json:"name"`
}

// CourseResponse is the wire form of a course with its derived counters
// and tuition packs.
type CourseResponse struct {
	ID                       uuid.UUID                `json:"id"`
	Code                     string                   `json:"code"`
	Name                     string                   `json:"name"`
	Subject                  *string                  `json:"subject"`
	Level                    *string                  `json:"level"`
	Description              *string                  `json:"description"`
	Status                   string                   `json:"status"`
	DefaultTemplateVersionID *uuid.UUID               `json:"default_template_version_id"`
	DefaultTemplate          *DefaultTemplateResponse `json:"default_template"`
	DefaultUnitPrice         int64                    `json:"default_unit_price"`
	TotalSessions            *int                     `json:"total_sessions"`
	DurationMin              *int                     `json:"duration_min"`
	ClassesRunning           int                      `json:"classes_running"`
	ClassesUpcoming          int                      `json:"classes_upcoming"`
	TuitionPacks             []TuitionPackResponse    `json:"tuition_packs"`
	CreatedAt                time.Time                `json:"created_at"`
	UpdatedAt                time.Time                `json:"updated_at"`
}

func courseResponse(row *CourseRow, packs []TuitionPack) CourseResponse {
	out := CourseResponse{
		ID: row.ID, Code: row.Code, Name: row.Name,
		Subject: row.Subject, Level: row.Level, Description: row.Description,
		Status:                   row.Status,
		DefaultTemplateVersionID: row.DefaultTemplateVersionID,
		DefaultUnitPrice:         row.DefaultUnitPrice,
		TotalSessions:            row.TotalSessions,
		DurationMin:              row.DurationMin,
		ClassesRunning:           row.ClassesRunning,
		ClassesUpcoming:          row.ClassesUpcoming,
		TuitionPacks:             packResponses(packs),
		CreatedAt:                row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if row.DefaultTemplateVersionID != nil && row.TemplateID != nil {
		out.DefaultTemplate = &DefaultTemplateResponse{
			VersionID:  *row.DefaultTemplateVersionID,
			VersionNo:  derefInt(row.TemplateVersionNo),
			Status:     derefStr(row.TemplateVersionStatus),
			TemplateID: *row.TemplateID,
			Code:       derefStr(row.TemplateCode),
			Name:       derefStr(row.TemplateName),
		}
	}
	return out
}

// packResponses returns a non-nil slice so the wire form is always an array.
func packResponses(rows []TuitionPack) []TuitionPackResponse {
	out := make([]TuitionPackResponse, 0, len(rows))
	for _, p := range rows {
		out = append(out, TuitionPackResponse{ID: p.ID, Name: p.Name, Sessions: p.Sessions, Price: p.Price, Position: p.Position})
	}
	return out
}

func derefInt(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func derefStr(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
