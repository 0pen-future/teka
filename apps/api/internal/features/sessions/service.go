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
	"teka/apps/api/internal/shared/id"
)

// maxRangeDays caps a generation request so an unbounded range cannot become
// an accidental denial of service against the database.
const maxRangeDays = 400

// ClassSource is the slice of the classes feature session generation needs:
// reading a class for its [start_date, end_date] clamp, and listing schedule
// rows effective within a window. *classes.Service satisfies this — declared
// here (a consumer-defined interface, the same pattern students uses for
// students.EnrollmentEnder) so sessions depends on classes' public service
// contract, never its repository type.
type ClassSource interface {
	Get(ctx context.Context, teacherID, classID uuid.UUID) (*classes.Class, error)
	ListEffectiveSchedules(ctx context.Context, teacherID, classID uuid.UUID, from, to time.Time) ([]classes.Schedule, error)
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
	ActiveOn(ctx context.Context, teacherID, classID uuid.UUID, on time.Time) ([]enrollments.Enrollment, error)
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
func (s *Service) ListRange(ctx context.Context, teacherID, classID uuid.UUID, from, to time.Time) ([]Detail, error) {
	if to.Before(from) {
		return nil, apperror.Invalid("validation failed", map[string]string{"to": "must not be before from"})
	}
	if days := int(to.Sub(from).Hours() / 24); days > maxRangeDays {
		return nil, apperror.Invalid("validation failed",
			map[string]string{"to": fmt.Sprintf("range must not exceed %d days", maxRangeDays)})
	}

	class, err := s.classes.Get(ctx, teacherID, classID)
	if err != nil {
		return nil, err
	}
	schedules, err := s.classes.ListEffectiveSchedules(ctx, teacherID, classID, from, to)
	if err != nil {
		return nil, err
	}
	loc, err := s.teacherLocation(ctx, teacherID)
	if err != nil {
		return nil, err
	}

	windows := make([]ScheduleWindow, len(schedules))
	for i, sc := range schedules {
		windows[i] = ScheduleWindow{
			Weekday:       sc.Weekday,
			StartTime:     string(sc.StartTime),
			EffectiveFrom: sc.EffectiveFrom,
			EffectiveTo:   sc.EffectiveTo,
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
			ID:          id.New(),
			TeacherID:   teacherID,
			ClassID:     classID,
			SessionDate: sessionDate,
			StartTime:   startTime,
			Status:      StatusPlanned,
		})
	}
	if err := s.repo.BulkInsertIgnoreConflicts(ctx, candidates); err != nil {
		return nil, apperror.Internal(err)
	}

	listed, err := s.repo.ListByClassAndRange(ctx, teacherID, classID, from, to)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	details := make([]Detail, 0, len(listed))
	for i := range listed {
		detail, err := s.toDetail(ctx, teacherID, &listed[i])
		if err != nil {
			return nil, err
		}
		details = append(details, *detail)
	}
	return details, nil
}

// ListPending returns the teacher's unconfirmed past sessions — the feed the
// dashboard's pending-attendance warning renders from, and the exact
// predicate plan 04's period-closing gate reuses with from/to bound to the
// billing period. "Past" is evaluated against today in the teacher's
// timezone (teachers.Timezone), not the server's: a session dated today is
// never pending until its own day is over. limit defaults to
// defaultPendingLimit when unset (zero or negative) and is capped at
// maxPendingLimit; total always reflects the unlimited count.
func (s *Service) ListPending(ctx context.Context, teacherID uuid.UUID, from, to *time.Time, limit int) (*PendingResponse, error) {
	loc, err := s.teacherLocation(ctx, teacherID)
	if err != nil {
		return nil, err
	}
	// now().In(loc) converts the instant into the teacher's calendar day
	// first; dateOnly then re-expresses that Y/M/D at UTC midnight, matching
	// how session_date is stored (see generator.go's dateOnly doc).
	today := dateOnly(s.now().In(loc), time.UTC)

	switch {
	case limit <= 0:
		limit = defaultPendingLimit
	case limit > maxPendingLimit:
		limit = maxPendingLimit
	}

	rows, total, err := s.repo.ListPending(ctx, teacherID, today, from, to, limit)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	items := make([]PendingSessionResponse, 0, len(rows))
	for i := range rows {
		items = append(items, fromPendingRow(&rows[i], today))
	}
	return &PendingResponse{Total: total, Items: items}, nil
}

// CreateAdHoc adds a single session outside any schedule — a make-up class
// the teacher places by hand. A date already occupied by another (non-deleted)
// session for the class returns 409, not a silent no-op.
func (s *Service) CreateAdHoc(ctx context.Context, teacherID, classID uuid.UUID, req CreateSessionRequest) (*Detail, error) {
	sessionDate, err := parseDate("session_date", req.SessionDate)
	if err != nil {
		return nil, err
	}

	// The composite FK would refuse a foreign class_id anyway; checking first
	// turns that refusal into a clean 404 instead of a constraint-violation 500.
	if _, err := s.classes.Get(ctx, teacherID, classID); err != nil {
		return nil, err
	}

	var startTime *classes.TimeOfDay
	if req.StartTime != "" {
		t := classes.TimeOfDay(req.StartTime)
		startTime = &t
	}
	session := &Session{
		ID:          id.New(),
		TeacherID:   teacherID,
		ClassID:     classID,
		SessionDate: sessionDate,
		StartTime:   startTime,
		Status:      StatusPlanned,
	}
	if err := s.repo.Create(ctx, session); err != nil {
		return nil, translate(err)
	}
	row, err := s.repo.GetByID(ctx, teacherID, session.ID)
	if err != nil {
		return nil, translate(err)
	}
	return s.toDetail(ctx, teacherID, row)
}

// Get returns one session with its class name and roster preview.
func (s *Service) Get(ctx context.Context, teacherID, sessionID uuid.UUID) (*Detail, error) {
	row, err := s.repo.GetByID(ctx, teacherID, sessionID)
	if err != nil {
		return nil, translate(err)
	}
	return s.toDetail(ctx, teacherID, row)
}

// GetByID returns one bare session, without the class name and roster
// preview Get enriches with. This is the shape other features consume
// through a narrow consumer interface (e.g. attendance's SessionStore),
// which resolve their own related data instead of paying for
// toDetail's roster query on every call.
func (s *Service) GetByID(ctx context.Context, teacherID, sessionID uuid.UUID) (*Session, error) {
	row, err := s.repo.GetByID(ctx, teacherID, sessionID)
	if err != nil {
		return nil, translate(err)
	}
	return &row.Session, nil
}

// MarkHeldAndConfirmed transitions a session planned->held and stamps
// attendance_confirmed_at. Exposed for the attendance feature's SessionStore
// contract; runs against database.FromContext(ctx, ...) so a caller invoking
// this from inside its own WithinTx block shares that same transaction,
// committing session status and attendance records atomically.
func (s *Service) MarkHeldAndConfirmed(ctx context.Context, teacherID, sessionID uuid.UUID, at time.Time) error {
	return translate(s.repo.MarkHeldAndConfirmed(ctx, teacherID, sessionID, at))
}

// Cancel marks a session cancelled with a reason — the line parents see on
// their statement. Refuses (409) a session whose attendance is already
// confirmed: the schema's CHECK constraint backs this up, but the service
// check exists to produce a clear message rather than a constraint-violation
// 500.
func (s *Service) Cancel(ctx context.Context, teacherID, sessionID uuid.UUID, reason string) (*Detail, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, reasonRequired()
	}
	row, err := s.repo.GetByID(ctx, teacherID, sessionID)
	if err != nil {
		return nil, translate(err)
	}
	if row.AttendanceConfirmedAt != nil {
		return nil, attendanceConfirmedConflict("cancel")
	}
	if err := s.repo.UpdateStatus(ctx, teacherID, sessionID, StatusCancelled, &reason); err != nil {
		return nil, translate(err)
	}
	return s.Get(ctx, teacherID, sessionID)
}

// Uncancel returns a cancelled session to planned and clears its reason. It
// only acts on a cancelled session: reopening one already in another state
// would strip status back to planned while leaving any confirmed attendance in
// place, producing a "planned but confirmed" row that billing (which counts
// only held sessions) would silently drop.
func (s *Service) Uncancel(ctx context.Context, teacherID, sessionID uuid.UUID) (*Detail, error) {
	row, err := s.repo.GetByID(ctx, teacherID, sessionID)
	if err != nil {
		return nil, translate(err)
	}
	if row.Status != StatusCancelled {
		return nil, invalidTransition("un-cancel a session that is not cancelled")
	}
	if err := s.repo.UpdateStatus(ctx, teacherID, sessionID, StatusPlanned, nil); err != nil {
		return nil, translate(err)
	}
	return s.Get(ctx, teacherID, sessionID)
}

// Hold marks a session held — the explicit action a teacher can take ahead of
// attendance confirmation (phase 2) implicitly doing the same. A cancelled
// session must be un-cancelled first: holding it directly would resurrect it
// with its stale cancel reason still attached.
func (s *Service) Hold(ctx context.Context, teacherID, sessionID uuid.UUID) (*Detail, error) {
	row, err := s.repo.GetByID(ctx, teacherID, sessionID)
	if err != nil {
		return nil, translate(err)
	}
	if row.Status == StatusCancelled {
		return nil, invalidTransition("hold a cancelled session; un-cancel it first")
	}
	if err := s.repo.UpdateStatus(ctx, teacherID, sessionID, StatusHeld, nil); err != nil {
		return nil, translate(err)
	}
	return s.Get(ctx, teacherID, sessionID)
}

// Delete soft-deletes a session. Refuses (409) a session whose attendance is
// already confirmed, for the same reason Cancel does.
func (s *Service) Delete(ctx context.Context, teacherID, sessionID uuid.UUID) error {
	row, err := s.repo.GetByID(ctx, teacherID, sessionID)
	if err != nil {
		return translate(err)
	}
	if row.AttendanceConfirmedAt != nil {
		return attendanceConfirmedConflict("delete")
	}
	return translate(s.repo.SoftDelete(ctx, teacherID, sessionID))
}

// toDetail enriches a joined row with the roster size active on its date.
func (s *Service) toDetail(ctx context.Context, teacherID uuid.UUID, row *Row) (*Detail, error) {
	active, err := s.enrollments.ActiveOn(ctx, teacherID, row.ClassID, row.SessionDate)
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
