package centers

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/features/attendance"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/pagination"
)

// ClassReader is the slice of the classes feature the dashboard consumes
// (consumer-defined interface; implemented by *classes.Service).
type ClassReader interface {
	Get(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*classes.Class, error)
	List(ctx context.Context, sc authctx.Scope, filter classes.ListFilter, p pagination.Params) ([]classes.Class, int64, error)
}

// SessionReader is the slice of the sessions feature the dashboard consumes.
// ListRangeReadOnly — never ListRange — because a dashboard GET must not
// materialise sessions on the viewed teacher's behalf.
type SessionReader interface {
	ListRangeReadOnly(ctx context.Context, sc authctx.Scope, classID uuid.UUID, from, to time.Time) ([]sessions.Row, error)
	Get(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) (*sessions.Detail, error)
}

// AttendanceReader is the slice of the attendance feature the dashboard
// consumes: the full roster sheet of one session.
type AttendanceReader interface {
	Get(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) (*attendance.Response, error)
}

// Dashboard serves the owner's cross-teacher read models. It is a separate
// service from Service because it depends on features (classes, sessions,
// attendance) that are constructed after the membership endpoints are.
type Dashboard struct {
	repo       Repository
	classes    ClassReader
	sessions   SessionReader
	attendance AttendanceReader
}

// NewDashboard builds the dashboard service.
func NewDashboard(repo Repository, classes ClassReader, sessions SessionReader, attendance AttendanceReader) *Dashboard {
	return &Dashboard{repo: repo, classes: classes, sessions: sessions, attendance: attendance}
}

// requireDashboardView gates every dashboard read: members manage their own
// data through the regular feature endpoints and only see the center-wide
// screens when granted dashboard.view.
func requireDashboardView(sc authctx.Scope) error {
	if !sc.Has(authctx.PermDashboardView) {
		return apperror.Forbidden("you are not allowed to view the center dashboard")
	}
	return nil
}

// requireCenterTeacher validates a drill-down target. A teacher who ever had
// a membership stint stays addressable after removal — the data they created
// belongs to the center — while ids from other centers get the same generic
// denial, leaking nothing about their existence.
func (d *Dashboard) requireCenterTeacher(ctx context.Context, sc authctx.Scope, teacherID uuid.UUID) error {
	ok, err := d.repo.WasEverMember(ctx, sc.CenterID, teacherID)
	if err != nil {
		return apperror.Internal(err)
	}
	if !ok {
		return apperror.Forbidden("teacher is not part of this center")
	}
	return nil
}

// targetScope is the strict per-teacher scope drill-down reads run under:
// exactly the viewed teacher's rows in the owner's center, never owner-wide,
// so the consumed features scope precisely like the teacher themselves would.
func targetScope(sc authctx.Scope, teacherID uuid.UUID) authctx.Scope {
	return authctx.Scope{TeacherID: teacherID, CenterID: sc.CenterID}
}

// forbiddenOnNotFound collapses a scoped lookup miss into the dashboard's
// uniform denial: whether an id is cross-center, cross-teacher, or nonexistent
// must be indistinguishable to the caller.
func forbiddenOnNotFound(err error) error {
	if apperror.From(err).Code == apperror.CodeNotFound {
		return apperror.Forbidden("not accessible from this center's dashboard")
	}
	return err
}

// monthRange resolves an optional "YYYY-MM" into the month's [first, last]
// day, defaulting to the current month.
func monthRange(month string) (time.Time, time.Time, error) {
	if month == "" {
		month = time.Now().Format("2006-01")
	}
	from, err := time.Parse("2006-01", month)
	if err != nil {
		return time.Time{}, time.Time{}, apperror.Invalid("validation failed",
			map[string]string{"month": "must be YYYY-MM"})
	}
	return from, from.AddDate(0, 1, -1), nil
}

// Teachers returns the roster with per-teacher activity counts.
func (d *Dashboard) Teachers(ctx context.Context, sc authctx.Scope) ([]TeacherStatsResponse, error) {
	if err := requireDashboardView(sc); err != nil {
		return nil, err
	}
	rows, err := d.repo.DashboardTeacherStats(ctx, sc.CenterID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	out := make([]TeacherStatsResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, TeacherStatsResponse{
			Teacher:        MemberResponse{ID: r.ID, FullName: r.FullName, Phone: r.Phone, IsOwner: r.IsOwner},
			ActiveClasses:  r.ActiveClasses,
			ActiveStudents: r.ActiveStudents,
		})
	}
	return out, nil
}

// Overview returns one month of per-class KPIs grouped by teacher. It covers
// every live class of the center — including classes whose teacher has left —
// so nothing the center paid for can disappear from sight.
func (d *Dashboard) Overview(ctx context.Context, sc authctx.Scope, month string) ([]OverviewTeacherResponse, error) {
	if err := requireDashboardView(sc); err != nil {
		return nil, err
	}
	from, to, err := monthRange(month)
	if err != nil {
		return nil, err
	}
	stats, err := d.repo.OverviewClassStats(ctx, sc.CenterID, from, to)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	invoiced, err := d.repo.InvoicedByClass(ctx, sc.CenterID, from.Year(), int(from.Month()))
	if err != nil {
		return nil, apperror.Internal(err)
	}
	closed, err := d.repo.ClosedPeriodTeachers(ctx, sc.CenterID, from.Year(), int(from.Month()))
	if err != nil {
		return nil, apperror.Internal(err)
	}

	out := make([]OverviewTeacherResponse, 0)
	for _, s := range stats {
		cls := OverviewClassResponse{
			ClassID:          s.ClassID,
			ClassName:        s.ClassName,
			SessionsHeld:     s.HeldSessions,
			EstimatedRevenue: s.EstimatedRevenue,
		}
		if s.HeldSessions > 0 {
			avg := float64(s.AttendanceTotal) / float64(s.HeldSessions)
			cls.AvgAttendance = &avg
		}
		if s.AttendanceTotal > 0 {
			rate := float64(s.PresentCount) / float64(s.AttendanceTotal)
			cls.PresentRate = &rate
		}
		if s.FirstDayCount > 0 {
			retention := float64(s.RetainedCount) / float64(s.FirstDayCount)
			cls.RetentionRate = &retention
		}
		// Invoiced revenue exists only once the teacher's period for the
		// month is closed; before that the number would still move, so the
		// API says "not final yet" with null rather than a misleading 0.
		if _, ok := closed[s.TeacherID]; ok {
			v := invoiced[s.ClassID]
			cls.InvoicedRevenue = &v
		}
		if n := len(out); n > 0 && out[n-1].TeacherID == s.TeacherID {
			out[n-1].Classes = append(out[n-1].Classes, cls)
			continue
		}
		out = append(out, OverviewTeacherResponse{
			TeacherID:   s.TeacherID,
			TeacherName: s.TeacherName,
			Classes:     []OverviewClassResponse{cls},
		})
	}
	return out, nil
}

// TeacherClasses lists one teacher's classes for drill-down, reusing the
// classes feature's own scoped listing.
func (d *Dashboard) TeacherClasses(ctx context.Context, sc authctx.Scope, teacherID uuid.UUID, filter classes.ListFilter, p pagination.Params) ([]classes.Class, int64, error) {
	if err := requireDashboardView(sc); err != nil {
		return nil, 0, err
	}
	if err := d.requireCenterTeacher(ctx, sc, teacherID); err != nil {
		return nil, 0, err
	}
	return d.classes.List(ctx, targetScope(sc, teacherID), filter, p)
}

// ClassSessions lists one class's materialised sessions in [from, to] with
// per-session attendance stats, without generating anything.
func (d *Dashboard) ClassSessions(ctx context.Context, sc authctx.Scope, teacherID, classID uuid.UUID, from, to time.Time) ([]SessionRowResponse, error) {
	if err := requireDashboardView(sc); err != nil {
		return nil, err
	}
	if err := d.requireCenterTeacher(ctx, sc, teacherID); err != nil {
		return nil, err
	}
	target := targetScope(sc, teacherID)
	if _, err := d.classes.Get(ctx, target, classID); err != nil {
		return nil, forbiddenOnNotFound(err)
	}
	rows, err := d.sessions.ListRangeReadOnly(ctx, target, classID, from, to)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].ID)
	}
	stats, err := d.repo.SessionStats(ctx, sc.CenterID, ids)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	out := make([]SessionRowResponse, 0, len(rows))
	for i := range rows {
		st := stats[rows[i].ID]
		out = append(out, SessionRowResponse{
			SessionID:        rows[i].ID,
			SessionDate:      rows[i].SessionDate.Format("2006-01-02"),
			Status:           rows[i].Status,
			AttendanceTotal:  st.Total,
			PresentCount:     st.Present,
			EstimatedRevenue: st.Estimated,
		})
	}
	return out, nil
}

// Session returns one session with its full attendance sheet and both revenue
// numbers.
func (d *Dashboard) Session(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) (*SessionDetailResponse, error) {
	if err := requireDashboardView(sc); err != nil {
		return nil, err
	}
	det, err := d.sessions.Get(ctx, sc, sessionID)
	if err != nil {
		return nil, forbiddenOnNotFound(err)
	}
	// The sheet resolves the roster through the session teacher's own strict
	// scope: their enrollments are theirs, not the owner's.
	sheet, err := d.attendance.Get(ctx, targetScope(sc, det.TeacherID), sessionID)
	if err != nil {
		return nil, err
	}
	stats, err := d.repo.SessionStats(ctx, sc.CenterID, []uuid.UUID{sessionID})
	if err != nil {
		return nil, apperror.Internal(err)
	}
	// The session was just fetched, so a miss here means it vanished
	// mid-request — same uniform denial as any other inaccessible id.
	invoiced, err := d.repo.SessionInvoiced(ctx, sc.CenterID, sessionID)
	if errors.Is(err, ErrNotFound) {
		return nil, apperror.Forbidden("not accessible from this center's dashboard")
	}
	if err != nil {
		return nil, apperror.Internal(err)
	}
	attRows := make([]SessionAttendanceRow, 0, len(sheet.Rows))
	for _, r := range sheet.Rows {
		attRows = append(attRows, SessionAttendanceRow{
			StudentID:   r.StudentID,
			StudentName: r.StudentName,
			DisplayNote: r.DisplayNote,
			Status:      r.Status,
			Billable:    r.Billable,
			Note:        r.Note,
		})
	}
	return &SessionDetailResponse{
		Session: SessionSummaryResponse{
			ID:                    det.ID,
			ClassID:               det.ClassID,
			ClassName:             det.ClassName,
			TeacherID:             det.TeacherID,
			SessionDate:           det.SessionDate.Format("2006-01-02"),
			Status:                det.Status,
			AttendanceConfirmedAt: det.AttendanceConfirmedAt,
		},
		Attendance:       attRows,
		EstimatedRevenue: stats[sessionID].Estimated,
		InvoicedRevenue:  invoiced,
	}, nil
}
