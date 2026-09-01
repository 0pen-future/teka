package sessions

import (
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
)

// dateLayout is the wire form of session_date.
const dateLayout = "2006-01-02"

// CancelRequest cancels a session; reason is required and becomes the line
// parents see on their statement.
type CancelRequest struct {
	Reason string `json:"reason" binding:"required,min=1,max=500"`
}

// CreateSessionRequest adds a single ad-hoc session — a make-up class placed
// by hand, outside any schedule.
type CreateSessionRequest struct {
	SessionDate string `json:"session_date" binding:"required,datetime=2006-01-02"`
	StartTime   string `json:"start_time" binding:"omitempty,hhmm"`
}

// AttendanceSummary tallies a confirmed session's attendance records by
// status. All four statuses bill identically; these counts are display data
// for calendar badges, never a billing input.
type AttendanceSummary struct {
	Present int `json:"present"`
	Late    int `json:"late"`
	Absent  int `json:"absent"`
	Excused int `json:"excused"`
}

// SessionResponse is the public session shape. StudentCount previews the
// roster size attendance confirmation would cover — every student enrolled
// in the class on session_date. AttendanceSummary is null until the session's
// attendance is confirmed, so "not recorded yet" stays distinguishable from
// "recorded with zero students".
type SessionResponse struct {
	ID                    uuid.UUID          `json:"id"`
	ClassID               uuid.UUID          `json:"class_id"`
	ClassName             string             `json:"class_name"`
	SessionDate           string             `json:"session_date"`
	StartTime             *string            `json:"start_time"`
	Status                string             `json:"status"`
	CancelReason          *string            `json:"cancel_reason"`
	AttendanceConfirmedAt *time.Time         `json:"attendance_confirmed_at"`
	StudentCount          int                `json:"student_count"`
	AttendanceSummary     *AttendanceSummary `json:"attendance_summary"`
	CreatedAt             time.Time          `json:"created_at"`
}

// FromDetail maps a session enriched with its class name and roster size
// onto the wire response.
func FromDetail(d *Detail) SessionResponse {
	var startTime *string
	if d.StartTime != nil {
		s := string(*d.StartTime)
		startTime = &s
	}
	// The confirmation stamp, not the counts, decides null: an all-zero
	// tally on a confirmed session is real data (nobody on the roster).
	var summary *AttendanceSummary
	if d.AttendanceConfirmedAt != nil {
		summary = &AttendanceSummary{
			Present: d.PresentCount,
			Late:    d.LateCount,
			Absent:  d.AbsentCount,
			Excused: d.ExcusedCount,
		}
	}
	return SessionResponse{
		ID:                    d.ID,
		ClassID:               d.ClassID,
		ClassName:             d.ClassName,
		SessionDate:           d.SessionDate.Format(dateLayout),
		StartTime:             startTime,
		Status:                d.Status,
		CancelReason:          d.CancelReason,
		AttendanceConfirmedAt: d.AttendanceConfirmedAt,
		StudentCount:          d.StudentCount,
		AttendanceSummary:     summary,
		CreatedAt:             d.CreatedAt,
	}
}

// parseDate converts a binding-validated YYYY-MM-DD string; a parse failure
// can only mean the value bypassed binding, so it surfaces as a validation
// error rather than a 500.
func parseDate(field, value string) (time.Time, error) {
	t, err := time.Parse(dateLayout, value)
	if err != nil {
		return time.Time{}, apperror.Invalid("validation failed",
			map[string]string{field: "must be a date in YYYY-MM-DD form"})
	}
	return t, nil
}
