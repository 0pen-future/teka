package attendance

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// dateLayout is the wire form of session_date.
const dateLayout = "2006-01-02"

// ConfirmMark is one student's exception from the all-present default:
// late, absent, or excused, optionally with a per-student note ("mẹ báo ốm").
// present is never sent — an unlisted roster student defaults to it — which
// preserves PRD R2's interaction budget: a normal session is still one tap.
type ConfirmMark struct {
	StudentID uuid.UUID `json:"student_id" binding:"required"`
	Status    string    `json:"status" binding:"required"`
	Note      string    `json:"note" binding:"omitempty,max=500"`
}

// ConfirmRequest carries the exceptions from the all-present default. The
// server resolves the roster and writes present for every unlisted student.
//
// Marks and AbsentStudentIDs are mutually exclusive; sending both is a 400.
// Neither has a `required` tag: an empty body is a legitimate, common request
// meaning "everyone was present" and must bind the same as an explicit []
// the client sent — not be confused with a missing field.
//
// Deprecated contract: AbsentStudentIDs is the pre-4-status body (absent ids
// only) kept for one release while the web client migrates to Marks; it maps
// onto Marks with status=absent.
type ConfirmRequest struct {
	Marks            []ConfirmMark `json:"marks"`
	AbsentStudentIDs []uuid.UUID   `json:"absent_student_ids"`
	Note             string        `json:"note" binding:"omitempty,max=500"`
}

// RowResponse is one roster row: a student's current attendance status for
// the session, or a null status when the session has never been confirmed.
// DisplayNote disambiguates same-named siblings on the tick screen (PRD edge
// case).
type RowResponse struct {
	StudentID    uuid.UUID `json:"student_id"`
	StudentName  string    `json:"student_name"`
	DisplayNote  *string   `json:"display_note"`
	EnrollmentID uuid.UUID `json:"enrollment_id"`
	Status       *string   `json:"status"`
	Billable     bool      `json:"billable"`
	Note         *string   `json:"note"`
}

// Response is the full attendance-sheet read model for one session: one row
// per student enrolled as of the session date, present students included, so
// the screen renders in a single call.
type Response struct {
	SessionID             uuid.UUID     `json:"session_id"`
	SessionDate           string        `json:"session_date"`
	Status                string        `json:"status"`
	AttendanceConfirmedAt *time.Time    `json:"attendance_confirmed_at"`
	Rows                  []RowResponse `json:"rows"`
	// Warning is set only by Confirm, only when the session's date falls
	// inside an already-closed billing period and the automatic
	// reconciliation attempt (plan 04) failed. A successful confirm with no
	// closed-period implication, or one whose reconciliation succeeded
	// (including posting nothing because nothing changed), leaves this nil.
	Warning *string `json:"warning,omitempty"`
}

// trimmedNotePtr converts a possibly-blank request note into the pointer
// form Record.Note carries, collapsing whitespace-only input to NULL.
func trimmedNotePtr(note string) *string {
	trimmed := strings.TrimSpace(note)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
