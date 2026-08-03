package attendance

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// dateLayout is the wire form of session_date.
const dateLayout = "2006-01-02"

// ConfirmRequest carries the interaction budget PRD R2 sets: only the ids of
// absent students. The server resolves the rest of the roster and writes
// present for everyone else.
//
// AbsentStudentIDs deliberately has no `required` tag: an empty array is a
// legitimate, common request meaning "everyone was present" and must bind
// the same as an explicit [] the client sent — not be confused with a
// missing field.
type ConfirmRequest struct {
	AbsentStudentIDs []uuid.UUID `json:"absent_student_ids"`
	Note             string      `json:"note" binding:"omitempty,max=500"`
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
