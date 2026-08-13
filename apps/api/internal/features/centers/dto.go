package centers

import (
	"time"

	"github.com/google/uuid"
)

// RenameRequest is the payload for PATCH /centers/me. Max mirrors the
// VARCHAR(255) column.
type RenameRequest struct {
	Name string `json:"name" binding:"required,min=1,max=255"`
}

// CenterResponse describes the caller's center from their point of view.
type CenterResponse struct {
	ID      uuid.UUID `json:"id"`
	Name    string    `json:"name"`
	IsOwner bool      `json:"is_owner"`
}

// MemberResponse is one member of the caller's center.
type MemberResponse struct {
	ID       uuid.UUID `json:"id"`
	FullName string    `json:"full_name"`
	Phone    string    `json:"phone"`
	IsOwner  bool      `json:"is_owner"`
}

// MeResponse is the body of GET /centers/me for the owner: the center plus
// its full member roster.
type MeResponse struct {
	Center  CenterResponse   `json:"center"`
	Members []MemberResponse `json:"members"`
}

// MemberMeResponse is the body of GET /centers/me for a non-owner member —
// the roster is owner-only data, so a member sees only the center's name.
type MemberMeResponse struct {
	CenterName string `json:"center_name"`
}

// TeacherStatsResponse is one roster row of GET /centers/dashboard/teachers:
// the member plus how much of the center's current activity is theirs.
type TeacherStatsResponse struct {
	Teacher MemberResponse `json:"teacher"`
	// ActiveClasses counts their live classes with status active.
	ActiveClasses int `json:"active_classes"`
	// ActiveStudents counts distinct students with an enrollment of theirs
	// active today.
	ActiveStudents int `json:"active_students"`
}

// OverviewTeacherResponse groups one teacher's per-class monthly KPIs on
// GET /centers/dashboard/overview.
type OverviewTeacherResponse struct {
	TeacherID   uuid.UUID               `json:"teacher_id"`
	TeacherName string                  `json:"teacher_name"`
	Classes     []OverviewClassResponse `json:"classes"`
}

// OverviewClassResponse is one class's KPIs for the requested month. Rates
// are null when their denominator is zero — "no data" is not "zero percent".
type OverviewClassResponse struct {
	ClassID      uuid.UUID `json:"class_id"`
	ClassName    string    `json:"class_name"`
	SessionsHeld int       `json:"sessions_held"`
	// AvgAttendance is attendance records per held session.
	AvgAttendance *float64 `json:"avg_attendance"`
	// PresentRate is the share of attendance records marked present.
	PresentRate *float64 `json:"present_rate"`
	// RetentionRate is the share of enrollments active on the month's first
	// day still active on its last day.
	RetentionRate *float64 `json:"retention_rate"`
	// EstimatedRevenue sums unit prices of billable records on confirmed
	// held sessions — the number that moves as attendance is taken.
	EstimatedRevenue int64 `json:"estimated_revenue"`
	// InvoicedRevenue is the closed-books number: non-void invoice lines
	// plus session-attributed adjustments of the month's closed period.
	// Null until the teacher's period for the month is closed. Adjustments
	// without a source session belong to no class and are excluded here.
	InvoicedRevenue *int64 `json:"invoiced_revenue"`
}

// SessionRowResponse is one session row of the dashboard's class drill-down,
// with per-session attendance stats.
type SessionRowResponse struct {
	SessionID        uuid.UUID `json:"session_id"`
	SessionDate      string    `json:"session_date"`
	Status           string    `json:"status"`
	AttendanceTotal  int       `json:"attendance_total"`
	PresentCount     int       `json:"present_count"`
	EstimatedRevenue int64     `json:"estimated_revenue"`
}

// SessionSummaryResponse identifies the session a dashboard detail is about.
type SessionSummaryResponse struct {
	ID                    uuid.UUID  `json:"id"`
	ClassID               uuid.UUID  `json:"class_id"`
	ClassName             string     `json:"class_name"`
	TeacherID             uuid.UUID  `json:"teacher_id"`
	SessionDate           string     `json:"session_date"`
	Status                string     `json:"status"`
	AttendanceConfirmedAt *time.Time `json:"attendance_confirmed_at"`
}

// SessionAttendanceRow is one roster row in the dashboard's session detail;
// a null status means the student was on the roster but never recorded.
type SessionAttendanceRow struct {
	StudentID   uuid.UUID `json:"student_id"`
	StudentName string    `json:"student_name"`
	DisplayNote *string   `json:"display_note"`
	Status      *string   `json:"status"`
	Billable    bool      `json:"billable"`
	Note        *string   `json:"note"`
}

// SessionDetailResponse is the body of GET /centers/dashboard/sessions/:id.
type SessionDetailResponse struct {
	Session          SessionSummaryResponse `json:"session"`
	Attendance       []SessionAttendanceRow `json:"attendance"`
	EstimatedRevenue int64                  `json:"estimated_revenue"`
	// InvoicedRevenue is null while no closed billing period covers the
	// session's date.
	InvoicedRevenue *int64 `json:"invoiced_revenue"`
}
