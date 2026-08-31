package sessions

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
)

// maxRangeDays caps a generation request so an unbounded range cannot become
// an accidental denial of service against the database.
const maxRangeDays = 400

// validateRange guards every range-listing entry point with the same order
// and cap rules, so the two read ports cannot drift apart.
func validateRange(from, to time.Time) error {
	if to.Before(from) {
		return apperror.Invalid("validation failed", map[string]string{"to": "must not be before from"})
	}
	if days := int(to.Sub(from).Hours() / 24); days > maxRangeDays {
		return apperror.Invalid("validation failed",
			map[string]string{"to": fmt.Sprintf("range must not exceed %d days", maxRangeDays)})
	}
	return nil
}

// ClassSource is the slice of the classes feature session generation needs:
// reading a class for its [start_date, end_date] clamp, and listing schedule
// rows effective within a window. *classes.Service satisfies this — declared
// here (a consumer-defined interface, the same pattern students uses for
// students.EnrollmentEnder) so sessions depends on classes' public service
// contract, never its repository type.
type ClassSource interface {
	// GetWritable is the shared WRITE gate: the owner, or an ACTIVE stint in
	// a role the capability admits. Every session-mutating flow (generation,
	// ad-hoc create) resolves through it, and it answers 403 (readable but
	// wrong role) vs 404 (no relationship) itself.
	GetWritable(ctx context.Context, sc authctx.Scope, classID uuid.UUID, capability authctx.ClassCapability) (*classes.Class, error)
	// GetReadableWithRoles is the read port: classes the caller holds a
	// class_staff stint on (ended included) or sees center-wide, plus the
	// caller's ACTIVE role keys — the classbook branches on them to decide
	// whether a read may trigger session generation.
	GetReadableWithRoles(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*classes.Class, []string, error)
	ListEffectiveSchedules(ctx context.Context, sc authctx.Scope, classID uuid.UUID, from, to time.Time) ([]classes.Schedule, error)
}

// TeacherSource is the slice of the teachers feature session generation
// needs: the teacher's IANA timezone, so dates are generated in the
// teacher's calendar day, not the server's. *teachers.Service satisfies this.
type TeacherSource interface {
	GetByID(ctx context.Context, id uuid.UUID) (*teachers.Profile, error)
}

// EnrollmentSource is the slice of the enrollments feature session responses
// need: the roster active on a given date, so a session preview can show how
// many students attendance confirmation will cover. *enrollments.Service
// satisfies this.
type EnrollmentSource interface {
	ActiveOn(ctx context.Context, sc authctx.Scope, classID uuid.UUID, on time.Time) ([]enrollments.Enrollment, error)
}

// Detail is a session enriched with its class name and the size of the
// roster attendance confirmation would cover — the read model dto.go maps
// onto the wire response.
type Detail struct {
	Row
	StudentCount int
}

// Service owns session generation and lifecycle: on-demand materialisation
// from a class's schedules, and the planned/held/cancelled transitions.
type Service struct {
	repo        Repository
	classes     ClassSource
	teachers    TeacherSource
	enrollments EnrollmentSource
	// now is overridden in tests so "today" for ListPending's cutoff is
	// deterministic; production always gets time.Now.
	now func() time.Time
}

// NewService builds the sessions service.
func NewService(repo Repository, classes ClassSource, teachers TeacherSource, enrollments EnrollmentSource) *Service {
	return &Service{repo: repo, classes: classes, teachers: teachers, enrollments: enrollments, now: time.Now}
}

// ListRange returns a class's sessions in [from, to], generating any missing
// rows first. Generation is idempotent — rerunning the same range only ever
// fills gaps, never duplicates or moves an existing row (cancelled sessions
// keep occupying their date and are never regenerated over).
func (s *Service) ListRange(ctx context.Context, sc authctx.Scope, classID uuid.UUID, from, to time.Time) ([]Detail, error) {
	if err := validateRange(from, to); err != nil {
		return nil, err
	}

	class, err := s.classes.GetWritable(ctx, sc, classID, authctx.CapSessionsWrite)
	if err != nil {
		return nil, err
	}
	schedules, err := s.classes.ListEffectiveSchedules(ctx, sc, classID, from, to)
	if err != nil {
		return nil, err
	}
	// Generation must be viewer-independent — an owner generating a member's
	// class sees the same dates the member would — so the calendar day comes
	// from the class's own teacher, never the caller.
	loc, err := s.teacherLocation(ctx, class.TeacherID)
	if err != nil {
		return nil, err
	}

	windows := make([]ScheduleWindow, len(schedules))
	for i, sched := range schedules {
		windows[i] = ScheduleWindow{
			Weekday:       sched.Weekday,
			StartTime:     string(sched.StartTime),
			EffectiveFrom: sched.EffectiveFrom,
			EffectiveTo:   sched.EffectiveTo,
		}
	}

	classWindow := ClassWindow{StartDate: class.StartDate, EndDate: class.EndDate}
	generatedDates := Expand(classWindow, windows, from, to, loc)

	candidates := make([]Session, 0, len(generatedDates))
	for _, sessionDate := range generatedDates {
		var startTime *classes.TimeOfDay
		if sw, ok := ScheduleFor(windows, sessionDate); ok && sw.StartTime != "" {
			t := classes.TimeOfDay(sw.StartTime)
			startTime = &t
		}
		candidates = append(candidates, Session{
			ID: id.New(),
			// Generated rows inherit the class's own anchors, not the
			// caller's — an owner generating a member's class sessions must
			// not silently reassign them to the owner, matching
			// classes.AddSchedule's rule for a schedule added to a member's
			// class.
			TeacherID:   class.TeacherID,
			CenterID:    class.CenterID,
			ClassID:     classID,
			SessionDate: sessionDate,
			StartTime:   startTime,
			Status:      StatusPlanned,
		})
	}
	if err := s.repo.BulkInsertIgnoreConflicts(ctx, candidates); err != nil {
		return nil, apperror.Internal(err)
	}

	// The write gate already admitted the caller; the listing itself is a
	// read, and it must include rows anchored to a previous teacher (a
	// handed-off class's history), which the own-rows port would drop.
	listed, err := s.repo.ListByClassAndRangeReadable(ctx, sc, classID, from, to)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	details := make([]Detail, 0, len(listed))
	for i := range listed {
		detail, err := s.toDetail(ctx, sc, &listed[i])
		if err != nil {
			return nil, err
		}
		details = append(details, *detail)
	}
	return details, nil
}

// ListRangeReadable is the classbook GET port. A caller who may write the
// class's sessions (the owner, or an ACTIVE stint in a sessions.write role)
// gets the full ListRange behavior, generation included. A caller who merely
// holds any other stint — a different role, or an ended one — gets the
// already-materialised sessions read-only: staff reads must never insert rows.
func (s *Service) ListRangeReadable(ctx context.Context, sc authctx.Scope, classID uuid.UUID, from, to time.Time) ([]Detail, error) {
	_, roles, err := s.classes.GetReadableWithRoles(ctx, sc, classID)
	if err != nil {
		return nil, err
	}
	canGenerate := sc.CenterWideFor(authctx.PermSessionsViewAll)
	for _, role := range roles {
		if authctx.StaffRoleCan(role, authctx.CapSessionsWrite) {
			canGenerate = true
		}
	}
	if canGenerate {
		return s.ListRange(ctx, sc, classID, from, to)
	}
	if err := validateRange(from, to); err != nil {
		return nil, err
	}
	listed, err := s.repo.ListByClassAndRangeReadable(ctx, sc, classID, from, to)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	details := make([]Detail, 0, len(listed))
	for i := range listed {
		detail, err := s.toDetail(ctx, sc, &listed[i])
		if err != nil {
			return nil, err
		}
		details = append(details, *detail)
	}
	return details, nil
}

// ListRangeReadOnly returns a class's already-materialised sessions in
// [from, to] without generating missing ones — the listing path for viewers
// (an owner browsing a member's calendar) whose GET must never write. It
// deliberately skips toDetail's roster lookup: callers that need per-session
// student counts batch them on their side instead of paying one query per row.
func (s *Service) ListRangeReadOnly(ctx context.Context, sc authctx.Scope, classID uuid.UUID, from, to time.Time) ([]Row, error) {
	if to.Before(from) {
		return nil, apperror.Invalid("validation failed", map[string]string{"to": "must not be before from"})
	}
	rows, err := s.repo.ListByClassAndRange(ctx, sc, classID, from, to)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	return rows, nil
}

// ListPending returns the teacher's unconfirmed past sessions — the feed the
// dashboard's pending-attendance warning renders from. "Past" is evaluated
// against today in the teacher's timezone (teachers.Timezone), not the
// server's: a session dated today is never pending until its own day is
// over. It delegates to ListUnconfirmedInWindow with before=today, so it and
// plan 04's period-closing gate (which calls ListUnconfirmedInWindow
// directly with its own before cutoff) share one predicate by construction.
func (s *Service) ListPending(ctx context.Context, sc authctx.Scope, from, to *time.Time, limit int) (*PendingResponse, error) {
	loc, err := s.teacherLocation(ctx, sc.TeacherID)
	if err != nil {
		return nil, err
	}
	// now().In(loc) converts the instant into the teacher's calendar day
	// first; dateOnly then re-expresses that Y/M/D at UTC midnight, matching
	// how session_date is stored (see generator.go's dateOnly doc).
	today := dateOnly(s.now().In(loc), time.UTC)
	return s.ListUnconfirmedInWindow(ctx, sc, from, to, today, limit)
}

// ListUnconfirmedInWindow is ListPending's predicate with an explicit
// `before` cutoff instead of an implicit "today": teacher-scoped,
// session_date < before, attendance_confirmed_at IS NULL, status IN
// ('held','planned'), deleted_at IS NULL, further narrowed by from/to when
// set. It exists so a consumer whose cutoff is not "today" — plan 04's
// billing close, which needs a future-dated window
// (from=today+1, to=period_end, before=period_end+1) for its unconfirmed-
// sessions warning — can reuse this exact predicate instead of billing
// writing its own session query. limit defaults to defaultPendingLimit when
// unset (zero or negative) and is capped at maxPendingLimit; total always
// reflects the unlimited count.
func (s *Service) ListUnconfirmedInWindow(ctx context.Context, sc authctx.Scope, from, to *time.Time, before time.Time, limit int) (*PendingResponse, error) {
	switch {
	case limit <= 0:
		limit = defaultPendingLimit
	case limit > maxPendingLimit:
		limit = maxPendingLimit
	}

	rows, total, err := s.repo.ListPending(ctx, sc, before, from, to, limit)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	items := make([]PendingSessionResponse, 0, len(rows))
	for i := range rows {
		items = append(items, fromPendingRow(&rows[i], before))
	}
	return &PendingResponse{Total: total, Items: items}, nil
}

// CreateAdHoc adds a single session outside any schedule — a make-up class
// the teacher places by hand. A date already occupied by another (non-deleted)
// session for the class returns 409, not a silent no-op.
func (s *Service) CreateAdHoc(ctx context.Context, sc authctx.Scope, classID uuid.UUID, req CreateSessionRequest) (*Detail, error) {
	sessionDate, err := parseDate("session_date", req.SessionDate)
	if err != nil {
		return nil, err
	}

	// The composite FK would refuse a foreign class_id anyway; checking first
	// turns that refusal into a clean 404/403 instead of a constraint-violation
	// 500.
	class, err := s.classes.GetWritable(ctx, sc, classID, authctx.CapSessionsWrite)
	if err != nil {
		return nil, err
	}

	var startTime *classes.TimeOfDay
	if req.StartTime != "" {
		t := classes.TimeOfDay(req.StartTime)
		startTime = &t
	}
	session := &Session{
		ID: id.New(),
		// An ad-hoc session inherits the class's own anchors, not the
		// caller's — an owner adding a make-up session to a member's class
		// must not silently reassign it to the owner. Same rule ListRange's
		// generation follows.
		TeacherID:   class.TeacherID,
		CenterID:    class.CenterID,
		ClassID:     classID,
		SessionDate: sessionDate,
		StartTime:   startTime,
		Status:      StatusPlanned,
	}
	if err := s.repo.Create(ctx, session); err != nil {
		return nil, translate(err)
	}
	// Read back through the read port: the row inherits the class's anchors,
	// so a writer who is not the anchored teacher would miss it on own-rows.
	row, err := s.repo.GetReadableByID(ctx, sc, session.ID)
	if err != nil {
		return nil, translate(err)
	}
	return s.toDetail(ctx, sc, row)
}

// GetReadable is the GET port: a class_staff stint (active or ended) on the
// session's class grants the read, so lifecycle writers can also read back
// what they changed even when the row is anchored to another teacher.
func (s *Service) GetReadable(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) (*Detail, error) {
	row, err := s.repo.GetReadableByID(ctx, sc, sessionID)
	if err != nil {
		return nil, translate(err)
	}
	return s.toDetail(ctx, sc, row)
}

// GetReadableByID is GetByID on the read port — the shape read-only consumers
// (attendance's sheet GET) resolve a session through.
func (s *Service) GetReadableByID(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) (*Session, error) {
	row, err := s.repo.GetReadableByID(ctx, sc, sessionID)
	if err != nil {
		return nil, translate(err)
	}
	return &row.Session, nil
}

// GetWritable returns one bare session through the capability write gate —
// the shape write-path consumers (attendance's SessionStore, grading's and
// teaching's SessionSource) resolve a session through, each naming the
// capability it is about to exercise. The bare Session skips toDetail's
// roster query; consumers resolve their own related data.
func (s *Service) GetWritable(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID, capability authctx.ClassCapability) (*Session, error) {
	row, err := s.getWritableRow(ctx, sc, sessionID, capability)
	if err != nil {
		return nil, err
	}
	return &row.Session, nil
}

// getWritableRow fetches through the write port, then disambiguates a miss
// through the read port: readable means the caller has SOME stint on the
// class (ended, or a different role) but not this capability → honest 403;
// unreadable → 404, so outsiders cannot probe session ids.
func (s *Service) getWritableRow(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID, capability authctx.ClassCapability) (*Row, error) {
	roles := authctx.StaffRolesFor(capability)
	row, err := s.repo.GetWritableByID(ctx, sc, sessionID, roles)
	if err == nil {
		return row, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, err
	}
	if _, rerr := s.repo.GetReadableByID(ctx, sc, sessionID); rerr == nil {
		return nil, apperror.Forbidden("your role on this class does not allow this action")
	} else if !errors.Is(rerr, ErrNotFound) {
		return nil, rerr
	}
	return nil, apperror.NotFound("session")
}

// MarkHeldAndConfirmed transitions a session planned->held and stamps
// attendance_confirmed_at. Exposed for the attendance feature's SessionStore
// contract; runs against database.FromContext(ctx, ...) so a caller invoking
// this from inside its own WithinTx block shares that same transaction,
// committing session status and attendance records atomically. The write
// scope binds the attendance.write roles: whoever may confirm attendance may
// also perform its implicit held transition.
func (s *Service) MarkHeldAndConfirmed(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID, at time.Time) error {
	roles := authctx.StaffRolesFor(authctx.CapAttendanceWrite)
	return translate(s.repo.MarkHeldAndConfirmed(ctx, sc, roles, sessionID, at))
}

// Cancel marks a session cancelled with a reason — the line parents see on
// their statement. Refuses (409) a session whose attendance is already
// confirmed: the schema's CHECK constraint backs this up, but the service
// check exists to produce a clear message rather than a constraint-violation
// 500.
func (s *Service) Cancel(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID, reason string) (*Detail, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, reasonRequired()
	}
	row, err := s.getWritableRow(ctx, sc, sessionID, authctx.CapSessionsWrite)
	if err != nil {
		return nil, err
	}
	if row.AttendanceConfirmedAt != nil {
		return nil, attendanceConfirmedConflict("cancel")
	}
	roles := authctx.StaffRolesFor(authctx.CapSessionsWrite)
	if err := s.repo.UpdateStatus(ctx, sc, roles, sessionID, StatusCancelled, &reason); err != nil {
		return nil, translate(err)
	}
	return s.GetReadable(ctx, sc, sessionID)
}

// Uncancel returns a cancelled session to planned and clears its reason. It
// only acts on a cancelled session: reopening one already in another state
// would strip status back to planned while leaving any confirmed attendance in
// place, producing a "planned but confirmed" row that billing (which counts
// only held sessions) would silently drop.
func (s *Service) Uncancel(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) (*Detail, error) {
	row, err := s.getWritableRow(ctx, sc, sessionID, authctx.CapSessionsWrite)
	if err != nil {
		return nil, err
	}
	if row.Status != StatusCancelled {
		return nil, invalidTransition("un-cancel a session that is not cancelled")
	}
	roles := authctx.StaffRolesFor(authctx.CapSessionsWrite)
	if err := s.repo.UpdateStatus(ctx, sc, roles, sessionID, StatusPlanned, nil); err != nil {
		return nil, translate(err)
	}
	return s.GetReadable(ctx, sc, sessionID)
}

// Hold marks a session held — the explicit action a teacher can take ahead of
// attendance confirmation (phase 2) implicitly doing the same. A cancelled
// session must be un-cancelled first: holding it directly would resurrect it
// with its stale cancel reason still attached.
func (s *Service) Hold(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) (*Detail, error) {
	row, err := s.getWritableRow(ctx, sc, sessionID, authctx.CapSessionsWrite)
	if err != nil {
		return nil, err
	}
	if row.Status == StatusCancelled {
		return nil, invalidTransition("hold a cancelled session; un-cancel it first")
	}
	roles := authctx.StaffRolesFor(authctx.CapSessionsWrite)
	if err := s.repo.UpdateStatus(ctx, sc, roles, sessionID, StatusHeld, nil); err != nil {
		return nil, translate(err)
	}
	return s.GetReadable(ctx, sc, sessionID)
}

// Delete soft-deletes a session. Refuses (409) a session whose attendance is
// already confirmed, for the same reason Cancel does.
func (s *Service) Delete(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) error {
	row, err := s.getWritableRow(ctx, sc, sessionID, authctx.CapSessionsWrite)
	if err != nil {
		return err
	}
	if row.AttendanceConfirmedAt != nil {
		return attendanceConfirmedConflict("delete")
	}
	roles := authctx.StaffRolesFor(authctx.CapSessionsWrite)
	return translate(s.repo.SoftDelete(ctx, sc, roles, sessionID))
}

// ReassignPlanned moves the class's future planned sessions to newTeacherID and
// returns the count moved. It is the sessions-owned half of the owner-only
// class handoff (see the handoff feature): changing a class's teacher must
// carry its upcoming un-run sessions so the new teacher owns the work, while
// held/cancelled and past planned sessions keep the old teacher and leave
// attendance and billing history untouched.
//
// The "future" boundary is today in oldTeacherID's timezone — the calendar the
// class's sessions were dated in — not the DB clock, matching ListPending's
// cutoff discipline. oldTeacherID is the class's current teacher, resolved by
// the caller before the class row is reassigned.
func (s *Service) ReassignPlanned(ctx context.Context, sc authctx.Scope, classID, oldTeacherID, newTeacherID uuid.UUID) (int64, error) {
	loc, err := s.teacherLocation(ctx, oldTeacherID)
	if err != nil {
		return 0, err
	}
	// now().In(loc) picks the teacher's calendar day; dateOnly re-expresses it
	// at UTC midnight, matching how session_date is stored.
	today := dateOnly(s.now().In(loc), time.UTC)
	n, err := s.repo.ReassignPlanned(ctx, sc, classID, newTeacherID, today)
	if err != nil {
		return 0, apperror.Internal(err)
	}
	return n, nil
}

// toDetail enriches a joined row with the roster size active on its date.
func (s *Service) toDetail(ctx context.Context, sc authctx.Scope, row *Row) (*Detail, error) {
	active, err := s.enrollments.ActiveOn(ctx, sc, row.ClassID, row.SessionDate)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	return &Detail{Row: *row, StudentCount: len(active)}, nil
}

// teacherLocation resolves the teacher's IANA timezone for date generation,
// falling back to UTC rather than failing the whole request.
func (s *Service) teacherLocation(ctx context.Context, teacherID uuid.UUID) (*time.Location, error) {
	profile, err := s.teachers.GetByID(ctx, teacherID)
	if err != nil {
		return nil, err
	}
	loc, err := time.LoadLocation(profile.Teacher.Timezone)
	if err != nil {
		// teachers.UpdateProfile validates the timezone as a live IANA zone
		// whenever it is set, so a stored value that no longer loads should
		// never happen in practice. Fall back rather than fail a generation
		// request over a corrupted setting.
		return time.UTC, nil
	}
	return loc, nil
}

// reasonRequired is the 422 for an empty cancel reason.
func reasonRequired() error {
	appErr := apperror.Invalid("validation failed", map[string]string{"reason": "is required"})
	appErr.Err = ErrReasonRequired
	return appErr
}

// attendanceConfirmedConflict is the 409 for cancelling or deleting a
// session whose attendance is already confirmed.
func attendanceConfirmedConflict(action string) error {
	appErr := apperror.Conflict(fmt.Sprintf(
		"session has confirmed attendance; clear attendance before you can %s it", action))
	appErr.Err = ErrAttendanceConfirmed
	return appErr
}

// invalidTransition is the 409 for a lifecycle move that would break the
// held-implies-confirmed invariant.
func invalidTransition(action string) error {
	appErr := apperror.Conflict("cannot " + action)
	appErr.Err = ErrInvalidTransition
	return appErr
}

// translate maps domain errors onto the API error contract, keeping the
// domain error as the cause so errors.Is still works.
func translate(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrNotFound):
		return apperror.NotFound("session")
	case errors.Is(err, ErrSessionExists):
		appErr := apperror.Conflict("a session already exists on this date")
		appErr.Err = ErrSessionExists
		return appErr
	default:
		return err
	}
}
