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
	// CanSendReports is the delegated report-sender permission; always false
	// for the owner (member-only flag).
	CanSendReports bool `json:"can_send_reports"`
}

// MeResponse is the body of GET /centers/me for the owner: the center plus
// its full member roster.
type MeResponse struct {
	Center  CenterResponse   `json:"center"`
	Members []MemberResponse `json:"members"`
	// Permissions is the caller's effective permission key list; the owner
	// always holds the full catalog (implicit superuser).
	Permissions []string `json:"permissions"`
}

// MemberMeResponse is the body of GET /centers/me for a non-owner member —
// the roster is owner-only data, so a member sees only the center's name
// plus their own delegated-send permission.
type MemberMeResponse struct {
	CenterName     string `json:"center_name"`
	CanSendReports bool   `json:"can_send_reports"`
	// Permissions is the caller's effective permission key list — the client's
	// source for gating navigation and pages.
	Permissions []string `json:"permissions"`
}

// PermissionInfo is one catalog entry: a stable key plus its Vietnamese
// display label and the structured fields the permission UI groups, sorts,
// and warns on. The catalog is code-owned; clients render what they receive
// and keep no copy. Older clients that only read key/label keep parsing —
// the structured fields are additive.
type PermissionInfo struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Resource    string `json:"resource"`
	Action      string `json:"action"`
	Kind        string `json:"kind"`
	Risk        string `json:"risk"`
	Description string `json:"description"`
}

// RoleResponse is one center role with its current permission set.
type RoleResponse struct {
	ID          uuid.UUID `json:"id"`
	Key         string    `json:"key"`
	Name        string    `json:"name"`
	Permissions []string  `json:"permissions"`
	// AssignmentVersion is the CAS token for this role's permission set:
	// echo it back on a replacement write; a mismatch means someone else
	// saved in between and the write returns 409 without mutating.
	AssignmentVersion int64 `json:"assignment_version"`
}

// MemberPermissionsResponse is one non-owner member's RBAC state: assigned
// role plus per-member overrides. RoleKey is "" when the stint holds no role.
type MemberPermissionsResponse struct {
	TeacherID uuid.UUID  `json:"teacher_id"`
	FullName  string     `json:"full_name"`
	RoleID    *uuid.UUID `json:"role_id"`
	RoleKey   string     `json:"role_key"`
	Grants    []string   `json:"grants"`
	Denies    []string   `json:"denies"`
	// AssignmentVersion is the CAS token for this member's override set —
	// same contract as RoleResponse.AssignmentVersion.
	AssignmentVersion int64 `json:"assignment_version"`
}

// PermissionsResponse is the body of GET /centers/me/permissions — the
// owner's full permission-management read model.
type PermissionsResponse struct {
	Catalog []PermissionInfo            `json:"catalog"`
	Roles   []RoleResponse              `json:"roles"`
	Members []MemberPermissionsResponse `json:"members"`
	// CatalogVersion identifies the catalog generation this read model was
	// rendered under; writes echo it so a client holding a stale catalog
	// gets 409 instead of silently assigning keys it never displayed.
	CatalogVersion int `json:"catalog_version"`
}

// RolePermissionsRequest replaces a role's permission set. An empty list is
// valid — it strips the role of every permission. CatalogVersion and
// AssignmentVersion echo the read model for compare-and-set; zero (or
// omitted) means a pre-CAS client and skips the check.
type RolePermissionsRequest struct {
	Permissions       []string `json:"permissions"`
	CatalogVersion    int      `json:"catalog_version"`
	AssignmentVersion int64    `json:"assignment_version"`
}

// MemberRoleRequest assigns a member's role.
type MemberRoleRequest struct {
	RoleID uuid.UUID `json:"role_id" binding:"required"`
}

// MemberOverridesRequest replaces a member's grant/deny override lists.
// CatalogVersion and AssignmentVersion follow the same CAS contract as
// RolePermissionsRequest.
type MemberOverridesRequest struct {
	Grants            []string `json:"grants"`
	Denies            []string `json:"denies"`
	CatalogVersion    int      `json:"catalog_version"`
	AssignmentVersion int64    `json:"assignment_version"`
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
