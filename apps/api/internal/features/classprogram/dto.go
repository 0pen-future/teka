package classprogram

import (
	"time"

	"github.com/google/uuid"
)

// ApplyRequest applies a published template version to the class. Confirm
// acknowledges that the class's current curriculum differs from the
// template's lesson list and may be overwritten.
type ApplyRequest struct {
	TemplateVersionID uuid.UUID `json:"template_version_id" binding:"required"`
	Confirm           bool      `json:"confirm"`
}

// ProgramResponse is the program a class applies: the version, its template,
// and who applied it when. lesson_count is the version's lesson count so the
// class page can show "n bài" without fetching the lessons. version_status
// tells the page when the version has since been archived in the library,
// so the owner knows to move the class to a newer one.
type ProgramResponse struct {
	TemplateVersionID uuid.UUID `json:"template_version_id"`
	TemplateID        uuid.UUID `json:"template_id"`
	TemplateName      string    `json:"template_name"`
	VersionNo         int       `json:"version_no"`
	VersionStatus     string    `json:"version_status"`
	LessonCount       int       `json:"lesson_count"`
	AppliedAt         time.Time `json:"applied_at"`
	AppliedBy         uuid.UUID `json:"applied_by"`
}

func programResponse(row *ProgramRow) *ProgramResponse {
	return &ProgramResponse{
		TemplateVersionID: row.TemplateVersionID,
		TemplateID:        row.TemplateID,
		TemplateName:      row.TemplateName,
		VersionNo:         row.VersionNo,
		VersionStatus:     row.VersionStatus,
		LessonCount:       row.LessonCount,
		AppliedAt:         row.AppliedAt,
		AppliedBy:         row.AppliedBy,
	}
}
