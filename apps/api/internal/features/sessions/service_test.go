package sessions

import (
	"context"
	"errors"
	"sort"
	"testing"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
)

// d parses a YYYY-MM-DD literal into a UTC date. Local to this package (as
// opposed to generator_test.go's identical helper in package sessions_test)
// because this file exercises unexported Service internals directly.
func d(s string) time.Time {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		panic(err)
	}
	return t
}

// --- fake ClassSource ---

type fakeClass struct {
	teacherID uuid.UUID
	class     *classes.Class
	schedules []classes.Schedule
}

type fakeClassSource struct {
	rows map[uuid.UUID]*fakeClass
}

func newFakeClassSource() *fakeClassSource {
	return &fakeClassSource{rows: map[uuid.UUID]*fakeClass{}}
}

// addClass stamps CenterID the same as teacherID: these unit tests exercise
// a single teacher acting as the sole owner of their own center, so the two
// ids coincide by construction (see the sc := authctx.Scope{...} literals
// below). Multi-tenant center semantics are covered by the real-DB tests in
// integration_test.go.
func (f *fakeClassSource) addClass(teacherID uuid.UUID, startDate time.Time, endDate *time.Time) uuid.UUID {
	classID := id.New()
	f.rows[classID] = &fakeClass{
		teacherID: teacherID,
		class: &classes.Class{
			ID: classID, TeacherID: teacherID, CenterID: teacherID, Name: "Fixture Class",
			StartDate: startDate, EndDate: endDate,
		},
	}
	return classID
}

func (f *fakeClassSource) addSchedule(classID uuid.UUID, weekday int16, startTime string, effectiveFrom time.Time, effectiveTo *time.Time) {
	f.rows[classID].schedules = append(f.rows[classID].schedules, classes.Schedule{
		ID: id.New(), ClassID: classID, Weekday: weekday, StartTime: classes.TimeOfDay(startTime),
		EffectiveFrom: effectiveFrom, EffectiveTo: effectiveTo,
	})
}

func (f *fakeClassSource) Get(_ context.Context, sc authctx.Scope, classID uuid.UUID) (*classes.Class, error) {
	c, ok := f.rows[classID]
	if !ok || c.teacherID != sc.TeacherID {
		return nil, apperror.NotFound("class")
	}
	return c.class, nil
}

func (f *fakeClassSource) ListEffectiveSchedules(_ context.Context, sc authctx.Scope, classID uuid.UUID, from, to time.Time) ([]classes.Schedule, error) {
	c, ok := f.rows[classID]
	if !ok || c.teacherID != sc.TeacherID {
		return nil, apperror.NotFound("class")
	}
	var out []classes.Schedule
	for _, s := range c.schedules {
		if s.EffectiveFrom.After(to) {
			continue
		}
		if s.EffectiveTo != nil && s.EffectiveTo.Before(from) {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// --- fake TeacherSource ---

type fakeTeacherSource struct {
	profiles map[uuid.UUID]*teachers.Profile
}

func newFakeTeacherSource() *fakeTeacherSource {
	return &fakeTeacherSource{profiles: map[uuid.UUID]*teachers.Profile{}}
}

func (f *fakeTeacherSource) addTeacher(teacherID uuid.UUID, timezone string) {
	f.profiles[teacherID] = &teachers.Profile{Teacher: teachers.Teacher{ID: teacherID, Timezone: timezone}}
}

func (f *fakeTeacherSource) GetByID(_ context.Context, id uuid.UUID) (*teachers.Profile, error) {
	p, ok := f.profiles[id]
	if !ok {
		return nil, apperror.NotFound("teacher")
	}
	return p, nil
}

// --- fake EnrollmentSource ---

type fakeEnrollmentSource struct {
	counts map[uuid.UUID]int // classID -> active student count
}

func newFakeEnrollmentSource() *fakeEnrollmentSource {
	return &fakeEnrollmentSource{counts: map[uuid.UUID]int{}}
}

func (f *fakeEnrollmentSource) ActiveOn(_ context.Context, _ authctx.Scope, classID uuid.UUID, _ time.Time) ([]enrollments.Enrollment, error) {
	return make([]enrollments.Enrollment, f.counts[classID]), nil
}

// --- fake Repository ---

type fakeSessionRow struct {
	Session
	deleted bool
}

type fakeRepository struct {
	rows    map[uuid.UUID]*fakeSessionRow
	classes *fakeClassSource // resolves ClassName, mirroring the repository's SQL join
	// gotNotBefore records the boundary the service resolved and passed to
	// ReassignPlanned, so a test can assert it is teacher-local today.
	gotNotBefore time.Time
}

func newFakeRepository(classes *fakeClassSource) *fakeRepository {
	return &fakeRepository{rows: map[uuid.UUID]*fakeSessionRow{}, classes: classes}
}

func (f *fakeRepository) row(r *fakeSessionRow) Row {
	name := ""
	if c, ok := f.classes.rows[r.ClassID]; ok {
		name = c.class.Name
	}
	return Row{Session: r.Session, ClassName: name}
}

// visible mirrors gormRepository.scoped: an owner sees every row in their
// center, a member sees only the ones they teach themselves.
func visible(sc authctx.Scope, r *fakeSessionRow) bool {
	if r.CenterID != sc.CenterID {
		return false
	}
	return sc.IsOwner || r.TeacherID == sc.TeacherID
}

func sameCalendarDate(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return ay == by && am == bm && ad == bd
}

func (f *fakeRepository) BulkInsertIgnoreConflicts(_ context.Context, rows []Session) error {
	for _, r := range rows {
		conflict := false
		for _, existing := range f.rows {
			if !existing.deleted && existing.ClassID == r.ClassID && sameCalendarDate(existing.SessionDate, r.SessionDate) {
				conflict = true
				break
			}
		}
		if conflict {
			continue
		}
		cp := r
		f.rows[cp.ID] = &fakeSessionRow{Session: cp}
	}
	return nil
}

func (f *fakeRepository) Create(_ context.Context, s *Session) error {
	for _, existing := range f.rows {
		if !existing.deleted && existing.ClassID == s.ClassID && sameCalendarDate(existing.SessionDate, s.SessionDate) {
			return ErrSessionExists
		}
	}
	f.rows[s.ID] = &fakeSessionRow{Session: *s}
	return nil
}

func (f *fakeRepository) ListByClassAndRange(_ context.Context, sc authctx.Scope, classID uuid.UUID, from, to time.Time) ([]Row, error) {
	var out []Row
	for _, r := range f.rows {
		if r.deleted || !visible(sc, r) || r.ClassID != classID {
			continue
		}
		if r.SessionDate.Before(from) || r.SessionDate.After(to) {
			continue
		}
		out = append(out, f.row(r))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SessionDate.Before(out[j].SessionDate) })
	return out, nil
}

func (f *fakeRepository) GetByID(_ context.Context, sc authctx.Scope, id uuid.UUID) (*Row, error) {
	r, ok := f.rows[id]
	if !ok || r.deleted || !visible(sc, r) {
		return nil, ErrNotFound
	}
	row := f.row(r)
	return &row, nil
}

func (f *fakeRepository) UpdateStatus(_ context.Context, sc authctx.Scope, id uuid.UUID, status string, cancelReason *string) error {
	r, ok := f.rows[id]
	if !ok || r.deleted || !visible(sc, r) {
		return ErrNotFound
	}
	r.Status = status
	r.CancelReason = cancelReason
	return nil
}

func (f *fakeRepository) SoftDelete(_ context.Context, sc authctx.Scope, id uuid.UUID) error {
	r, ok := f.rows[id]
	if !ok || r.deleted || !visible(sc, r) {
		return ErrNotFound
	}
	r.deleted = true
	return nil
}

func (f *fakeRepository) ReassignPlanned(_ context.Context, sc authctx.Scope, classID, newTeacherID uuid.UUID, notBefore time.Time) (int64, error) {
	f.gotNotBefore = notBefore
	var moved int64
	for _, r := range f.rows {
		if r.deleted || !visible(sc, r) {
			continue
		}
		if r.ClassID == classID && r.Status == StatusPlanned && !r.SessionDate.Before(notBefore) {
			r.TeacherID = newTeacherID
			moved++
		}
	}
	return moved, nil
}

func (f *fakeRepository) MarkHeldAndConfirmed(_ context.Context, sc authctx.Scope, id uuid.UUID, at time.Time) error {
	r, ok := f.rows[id]
	if !ok || r.deleted || !visible(sc, r) {
		return ErrNotFound
	}
	r.Status = StatusHeld
	r.AttendanceConfirmedAt = &at
	return nil
}

// --- test wiring ---

type testDeps struct {
	repo     *fakeRepository
	classes  *fakeClassSource
	teachers *fakeTeacherSource
	enrolls  *fakeEnrollmentSource
}

func newTestService() (*Service, *testDeps) {
	classSrc := newFakeClassSource()
	deps := &testDeps{
		repo:     newFakeRepository(classSrc),
		classes:  classSrc,
		teachers: newFakeTeacherSource(),
		enrolls:  newFakeEnrollmentSource(),
	}
	return NewService(deps.repo, deps.classes, deps.teachers, deps.enrolls), deps
}

func TestListRangeGeneratesAndIsIdempotent(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID := id.New()
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	deps.teachers.addTeacher(teacherID, "Asia/Ho_Chi_Minh")
	classID := deps.classes.addClass(teacherID, d("2026-01-01"), nil)
	deps.classes.addSchedule(classID, 2, "18:00", d("2026-01-01"), nil) // Tuesday

	first, err := svc.ListRange(ctx, sc, classID, d("2026-01-01"), d("2026-01-31"))
	if err != nil {
		t.Fatalf("first generation: %v", err)
	}
	if len(first) != 4 {
		t.Fatalf("want 4 Tuesdays in January 2026, got %d", len(first))
	}
	if first[0].StartTime == nil || string(*first[0].StartTime) != "18:00" {
		t.Fatalf("start_time must come from the matching schedule, got %v", first[0].StartTime)
	}
	if first[0].ClassName != "Fixture Class" {
		t.Fatalf("row must carry the class name, got %q", first[0].ClassName)
	}

	// Regenerating the same range must not duplicate rows.
	second, err := svc.ListRange(ctx, sc, classID, d("2026-01-01"), d("2026-01-31"))
	if err != nil {
		t.Fatalf("second generation: %v", err)
	}
	if len(second) != len(first) {
		t.Fatalf("regenerating an overlapping range must be idempotent: got %d rows, want %d", len(second), len(first))
	}
}

func TestListRangeCancelledSessionIsNotRegenerated(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID := id.New()
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	deps.teachers.addTeacher(teacherID, "Asia/Ho_Chi_Minh")
	classID := deps.classes.addClass(teacherID, d("2026-01-01"), nil)
	deps.classes.addSchedule(classID, 2, "18:00", d("2026-01-01"), nil)

	rows, err := svc.ListRange(ctx, sc, classID, d("2026-01-01"), d("2026-01-31"))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	target := rows[0]
	if _, err := svc.Cancel(ctx, sc, target.ID, "nghỉ lễ"); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	regenerated, err := svc.ListRange(ctx, sc, classID, d("2026-01-01"), d("2026-01-31"))
	if err != nil {
		t.Fatalf("regenerate: %v", err)
	}
	if len(regenerated) != len(rows) {
		t.Fatalf("regeneration must not add a row for an already-occupied date: got %d, want %d", len(regenerated), len(rows))
	}
	for _, r := range regenerated {
		if r.ID == target.ID {
			if r.Status != StatusCancelled {
				t.Fatalf("cancelled session must keep its status across regeneration, got %s", r.Status)
			}
			if !r.SessionDate.Equal(target.SessionDate) {
				t.Fatalf("cancelled session must keep its date, got %v want %v", r.SessionDate, target.SessionDate)
			}
		}
	}
}

func TestListRangeRejectsToBeforeFrom(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID := id.New()
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	deps.teachers.addTeacher(teacherID, "Asia/Ho_Chi_Minh")
	classID := deps.classes.addClass(teacherID, d("2026-01-01"), nil)

	_, err := svc.ListRange(ctx, sc, classID, d("2026-01-31"), d("2026-01-01"))
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeValidation {
		t.Fatalf("to before from must be 422, got %v", err)
	}
}

func TestListRangeRejectsRangeOver400Days(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID := id.New()
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	deps.teachers.addTeacher(teacherID, "Asia/Ho_Chi_Minh")
	classID := deps.classes.addClass(teacherID, d("2020-01-01"), nil)

	_, err := svc.ListRange(ctx, sc, classID, d("2026-01-01"), d("2027-06-01"))
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeValidation {
		t.Fatalf("range over 400 days must be 422, got %v", err)
	}
}

func TestListRangeMissingClassIs404(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID := id.New()
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	deps.teachers.addTeacher(teacherID, "Asia/Ho_Chi_Minh")

	_, err := svc.ListRange(ctx, sc, id.New(), d("2026-01-01"), d("2026-01-31"))
	if apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("missing class must be 404, got %v", err)
	}
}

func TestListRangeInvalidTimezoneFallsBackToUTC(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID := id.New()
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	deps.teachers.addTeacher(teacherID, "not-a-real-zone")
	classID := deps.classes.addClass(teacherID, d("2026-01-01"), nil)
	deps.classes.addSchedule(classID, 2, "18:00", d("2026-01-01"), nil)

	rows, err := svc.ListRange(ctx, sc, classID, d("2026-01-01"), d("2026-01-31"))
	if err != nil {
		t.Fatalf("an invalid stored timezone must not fail the request, got %v", err)
	}
	if len(rows) != 4 {
		t.Fatalf("want 4 Tuesdays, got %d", len(rows))
	}
}

func TestStudentCountReflectsActiveEnrollments(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID := id.New()
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	deps.teachers.addTeacher(teacherID, "Asia/Ho_Chi_Minh")
	classID := deps.classes.addClass(teacherID, d("2026-01-01"), nil)
	deps.classes.addSchedule(classID, 2, "18:00", d("2026-01-01"), nil)
	deps.enrolls.counts[classID] = 7

	rows, err := svc.ListRange(ctx, sc, classID, d("2026-01-01"), d("2026-01-31"))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	for _, r := range rows {
		if r.StudentCount != 7 {
			t.Fatalf("student_count must reflect ActiveOn's roster size, got %d", r.StudentCount)
		}
	}
}

func TestCancelRequiresANonEmptyReason(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID := id.New()
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	deps.teachers.addTeacher(teacherID, "Asia/Ho_Chi_Minh")
	classID := deps.classes.addClass(teacherID, d("2026-01-01"), nil)
	deps.classes.addSchedule(classID, 2, "18:00", d("2026-01-01"), nil)
	rows, err := svc.ListRange(ctx, sc, classID, d("2026-01-01"), d("2026-01-31"))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	for _, reason := range []string{"", "   "} {
		_, err := svc.Cancel(ctx, sc, rows[0].ID, reason)
		var appErr *apperror.AppError
		if !errors.As(err, &appErr) || appErr.Code != apperror.CodeValidation {
			t.Fatalf("empty/whitespace reason %q must be 422, got %v", reason, err)
		}
		if !errors.Is(err, ErrReasonRequired) {
			t.Fatalf("want ErrReasonRequired cause, got %v", err)
		}
	}
}

func TestCancelAndUncancelRoundTrip(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID := id.New()
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	deps.teachers.addTeacher(teacherID, "Asia/Ho_Chi_Minh")
	classID := deps.classes.addClass(teacherID, d("2026-01-01"), nil)
	deps.classes.addSchedule(classID, 2, "18:00", d("2026-01-01"), nil)
	rows, err := svc.ListRange(ctx, sc, classID, d("2026-01-01"), d("2026-01-31"))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	cancelled, err := svc.Cancel(ctx, sc, rows[0].ID, "nghỉ lễ")
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if cancelled.Status != StatusCancelled || cancelled.CancelReason == nil || *cancelled.CancelReason != "nghỉ lễ" {
		t.Fatalf("cancel must set status and store the reason, got %+v", cancelled.Session)
	}

	uncancelled, err := svc.Uncancel(ctx, sc, rows[0].ID)
	if err != nil {
		t.Fatalf("uncancel: %v", err)
	}
	if uncancelled.Status != StatusPlanned {
		t.Fatalf("uncancel must return to planned, got %s", uncancelled.Status)
	}
	if uncancelled.CancelReason != nil {
		t.Fatalf("uncancel must clear cancel_reason, got %q", *uncancelled.CancelReason)
	}
}

func TestCancelRefusesWhenAttendanceConfirmed(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID := id.New()
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	deps.teachers.addTeacher(teacherID, "Asia/Ho_Chi_Minh")
	classID := deps.classes.addClass(teacherID, d("2026-01-01"), nil)
	deps.classes.addSchedule(classID, 2, "18:00", d("2026-01-01"), nil)
	rows, err := svc.ListRange(ctx, sc, classID, d("2026-01-01"), d("2026-01-31"))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	confirmedAt := time.Now()
	deps.repo.rows[rows[0].ID].AttendanceConfirmedAt = &confirmedAt

	_, err = svc.Cancel(ctx, sc, rows[0].ID, "nghỉ lễ")
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeConflict {
		t.Fatalf("cancelling a confirmed session must be 409, got %v", err)
	}
	if !errors.Is(err, ErrAttendanceConfirmed) {
		t.Fatalf("want ErrAttendanceConfirmed cause, got %v", err)
	}
}

func TestDeleteRefusesWhenAttendanceConfirmed(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID := id.New()
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	deps.teachers.addTeacher(teacherID, "Asia/Ho_Chi_Minh")
	classID := deps.classes.addClass(teacherID, d("2026-01-01"), nil)
	deps.classes.addSchedule(classID, 2, "18:00", d("2026-01-01"), nil)
	rows, err := svc.ListRange(ctx, sc, classID, d("2026-01-01"), d("2026-01-31"))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	confirmedAt := time.Now()
	deps.repo.rows[rows[0].ID].AttendanceConfirmedAt = &confirmedAt

	err = svc.Delete(ctx, sc, rows[0].ID)
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeConflict {
		t.Fatalf("deleting a confirmed session must be 409, got %v", err)
	}

	// A session without confirmed attendance deletes cleanly.
	if err := svc.Delete(ctx, sc, rows[1].ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := svc.Get(ctx, sc, rows[1].ID); apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("deleted session must read as 404, got %v", err)
	}
}

func TestHoldMarksSessionHeld(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID := id.New()
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	deps.teachers.addTeacher(teacherID, "Asia/Ho_Chi_Minh")
	classID := deps.classes.addClass(teacherID, d("2026-01-01"), nil)
	deps.classes.addSchedule(classID, 2, "18:00", d("2026-01-01"), nil)
	rows, err := svc.ListRange(ctx, sc, classID, d("2026-01-01"), d("2026-01-31"))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	held, err := svc.Hold(ctx, sc, rows[0].ID)
	if err != nil {
		t.Fatalf("hold: %v", err)
	}
	if held.Status != StatusHeld {
		t.Fatalf("hold must set status to held, got %s", held.Status)
	}
}

func TestUncancelRefusesASessionThatIsNotCancelled(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID := id.New()
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	deps.teachers.addTeacher(teacherID, "Asia/Ho_Chi_Minh")
	classID := deps.classes.addClass(teacherID, d("2026-01-01"), nil)
	deps.classes.addSchedule(classID, 2, "18:00", d("2026-01-01"), nil)
	rows, err := svc.ListRange(ctx, sc, classID, d("2026-01-01"), d("2026-01-31"))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	// A confirmed (held) session must not be reopenable to planned: doing so
	// would leave attendance_confirmed_at set while status drops to planned,
	// and billing counts only held sessions — the money would vanish.
	confirmedAt := time.Now()
	deps.repo.rows[rows[0].ID].AttendanceConfirmedAt = &confirmedAt
	deps.repo.rows[rows[0].ID].Status = StatusHeld

	_, err = svc.Uncancel(ctx, sc, rows[0].ID)
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeConflict {
		t.Fatalf("un-cancelling a non-cancelled session must be 409, got %v", err)
	}
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("want ErrInvalidTransition cause, got %v", err)
	}
	if deps.repo.rows[rows[0].ID].Status != StatusHeld {
		t.Fatalf("a refused un-cancel must leave the session held, got %s", deps.repo.rows[rows[0].ID].Status)
	}
}

func TestHoldRefusesACancelledSession(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID := id.New()
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	deps.teachers.addTeacher(teacherID, "Asia/Ho_Chi_Minh")
	classID := deps.classes.addClass(teacherID, d("2026-01-01"), nil)
	deps.classes.addSchedule(classID, 2, "18:00", d("2026-01-01"), nil)
	rows, err := svc.ListRange(ctx, sc, classID, d("2026-01-01"), d("2026-01-31"))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := svc.Cancel(ctx, sc, rows[0].ID, "nghỉ lễ"); err != nil {
		t.Fatalf("cancel: %v", err)
	}

	_, err = svc.Hold(ctx, sc, rows[0].ID)
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeConflict {
		t.Fatalf("holding a cancelled session must be 409, got %v", err)
	}
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("want ErrInvalidTransition cause, got %v", err)
	}
	// Un-cancelling first, then holding, is the sanctioned path and clears the
	// stale reason.
	if _, err := svc.Uncancel(ctx, sc, rows[0].ID); err != nil {
		t.Fatalf("uncancel: %v", err)
	}
	held, err := svc.Hold(ctx, sc, rows[0].ID)
	if err != nil {
		t.Fatalf("hold after uncancel: %v", err)
	}
	if held.Status != StatusHeld || held.CancelReason != nil {
		t.Fatalf("hold must set held and carry no stale reason, got %+v", held.Session)
	}
}

func TestCreateAdHocConflictsWithExistingDate(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID := id.New()
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	deps.teachers.addTeacher(teacherID, "Asia/Ho_Chi_Minh")
	classID := deps.classes.addClass(teacherID, d("2026-01-01"), nil)
	deps.classes.addSchedule(classID, 2, "18:00", d("2026-01-01"), nil)
	rows, err := svc.ListRange(ctx, sc, classID, d("2026-01-01"), d("2026-01-31"))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	occupiedDate := rows[0].SessionDate.Format(dateLayout)

	_, err = svc.CreateAdHoc(ctx, sc, classID, CreateSessionRequest{SessionDate: occupiedDate})
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeConflict {
		t.Fatalf("ad-hoc session on an occupied date must be 409, got %v", err)
	}
	if !errors.Is(err, ErrSessionExists) {
		t.Fatalf("want ErrSessionExists cause, got %v", err)
	}

	// A free date succeeds.
	created, err := svc.CreateAdHoc(ctx, sc, classID, CreateSessionRequest{SessionDate: "2026-01-15", StartTime: "10:00"})
	if err != nil {
		t.Fatalf("ad-hoc on a free date: %v", err)
	}
	if created.StartTime == nil || string(*created.StartTime) != "10:00" {
		t.Fatalf("ad-hoc start_time must round-trip, got %v", created.StartTime)
	}
}

func TestGetByIDReturnsBareSession(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID := id.New()
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	deps.teachers.addTeacher(teacherID, "Asia/Ho_Chi_Minh")
	classID := deps.classes.addClass(teacherID, d("2026-01-01"), nil)
	deps.classes.addSchedule(classID, 2, "18:00", d("2026-01-01"), nil)
	rows, err := svc.ListRange(ctx, sc, classID, d("2026-01-01"), d("2026-01-31"))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	session, err := svc.GetByID(ctx, sc, rows[0].ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if session.ID != rows[0].ID || session.ClassID != classID {
		t.Fatalf("GetByID must return the requested session, got %+v", session)
	}

	if _, err := svc.GetByID(ctx, sc, id.New()); apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("missing session must be 404, got %v", err)
	}
}

func TestMarkHeldAndConfirmedTransitionsStatus(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID := id.New()
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	deps.teachers.addTeacher(teacherID, "Asia/Ho_Chi_Minh")
	classID := deps.classes.addClass(teacherID, d("2026-01-01"), nil)
	deps.classes.addSchedule(classID, 2, "18:00", d("2026-01-01"), nil)
	rows, err := svc.ListRange(ctx, sc, classID, d("2026-01-01"), d("2026-01-31"))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	at := time.Now()
	if err := svc.MarkHeldAndConfirmed(ctx, sc, rows[0].ID, at); err != nil {
		t.Fatalf("MarkHeldAndConfirmed: %v", err)
	}
	session, err := svc.GetByID(ctx, sc, rows[0].ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if session.Status != StatusHeld {
		t.Fatalf("want status held, got %s", session.Status)
	}
	if session.AttendanceConfirmedAt == nil || !session.AttendanceConfirmedAt.Equal(at) {
		t.Fatalf("want attendance_confirmed_at set to %v, got %v", at, session.AttendanceConfirmedAt)
	}

	if err := svc.MarkHeldAndConfirmed(ctx, sc, id.New(), at); apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("missing session must be 404, got %v", err)
	}
}

func TestCrossTenantReadsAreNotFound(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	owner := id.New()
	stranger := id.New()
	ownerScope := authctx.Scope{TeacherID: owner, CenterID: owner, IsOwner: true}
	strangerScope := authctx.Scope{TeacherID: stranger, CenterID: stranger, IsOwner: true}
	deps.teachers.addTeacher(owner, "Asia/Ho_Chi_Minh")
	deps.teachers.addTeacher(stranger, "Asia/Ho_Chi_Minh")
	classID := deps.classes.addClass(owner, d("2026-01-01"), nil)
	deps.classes.addSchedule(classID, 2, "18:00", d("2026-01-01"), nil)
	rows, err := svc.ListRange(ctx, ownerScope, classID, d("2026-01-01"), d("2026-01-31"))
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if _, err := svc.Get(ctx, strangerScope, rows[0].ID); apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant get must be 404, got %v", err)
	}
	if _, err := svc.ListRange(ctx, strangerScope, classID, d("2026-01-01"), d("2026-01-31")); apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant list must be 404 (foreign class), got %v", err)
	}
}

// TestReassignPlannedBoundaryUsesTeacherTimezone pins the future-session cutoff
// to the old teacher's calendar day, not the DB/UTC clock: a handoff run in the
// small hours (UTC) must not sweep a session dated "yesterday-UTC / today-local"
// — and, symmetrically here, must resolve the boundary in the teacher's zone.
func TestReassignPlannedBoundaryUsesTeacherTimezone(t *testing.T) {
	oldTeacher := id.New()
	newTeacher := id.New()
	classID := id.New()
	sc := authctx.Scope{TeacherID: oldTeacher, CenterID: oldTeacher, IsOwner: true}

	// 2026-03-10 02:00 UTC is still 2026-03-09 in America/Los_Angeles (UTC-8):
	// the boundary must be the teacher's calendar day, not UTC's 03-10.
	fixedNow := time.Date(2026, 3, 10, 2, 0, 0, 0, time.UTC)
	classSrc := newFakeClassSource()
	repo := newFakeRepository(classSrc)
	teachersSrc := newFakeTeacherSource()
	teachersSrc.addTeacher(oldTeacher, "America/Los_Angeles")
	svc := &Service{
		repo:     repo,
		classes:  classSrc,
		teachers: teachersSrc,
		now:      func() time.Time { return fixedNow },
	}

	// Two planned sessions straddling the teacher-local boundary (03-09):
	// the one on the boundary moves (inclusive), the one before stays.
	onBoundary := &Session{
		ID: id.New(), TeacherID: oldTeacher, CenterID: sc.CenterID, ClassID: classID,
		SessionDate: d("2026-03-09"), Status: StatusPlanned,
	}
	before := &Session{
		ID: id.New(), TeacherID: oldTeacher, CenterID: sc.CenterID, ClassID: classID,
		SessionDate: d("2026-03-08"), Status: StatusPlanned,
	}
	repo.rows[onBoundary.ID] = &fakeSessionRow{Session: *onBoundary}
	repo.rows[before.ID] = &fakeSessionRow{Session: *before}

	moved, err := svc.ReassignPlanned(context.Background(), sc, classID, oldTeacher, newTeacher)
	if err != nil {
		t.Fatalf("ReassignPlanned: %v", err)
	}

	want := time.Date(2026, 3, 9, 0, 0, 0, 0, time.UTC)
	if !repo.gotNotBefore.Equal(want) {
		t.Fatalf("boundary must be today in the teacher's timezone, got %v want %v", repo.gotNotBefore, want)
	}
	if moved != 1 {
		t.Fatalf("only the on-boundary session moves, got %d", moved)
	}
	if repo.rows[onBoundary.ID].TeacherID != newTeacher {
		t.Fatal("the session dated on the local boundary must move to the new teacher")
	}
	if repo.rows[before.ID].TeacherID != oldTeacher {
		t.Fatal("the session dated before the local boundary must keep the old teacher")
	}
}
