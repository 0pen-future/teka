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

// SessionResponse is the public session shape. StudentCount previews the
// roster size attendance confirmation would cover — every student enrolled
// in the class on session_date.
type SessionResponse struct {
	ID                    uuid.UUID  `json:"id"`
	ClassID               uuid.UUID  `json:"class_id"`
	ClassName             string     `json:"class_name"`
	SessionDate           string     `json:"session_date"`
	StartTime             *string    `json:"start_time"`
	Status                string     `json:"status"`
	CancelReason          *string    `json:"cancel_reason"`
	AttendanceConfirmedAt *time.Time `json:"attendance_confirmed_at"`
	StudentCount          int        `json:"student_count"`
	CreatedAt             time.Time  `json:"created_at"`
}

// FromDetail maps a session enriched with its class name and roster size
// onto the wire response.
func FromDetail(d *Detail) SessionResponse {
	var startTime *string
	if d.StartTime != nil {
		s := string(*d.StartTime)
		startTime = &s
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
