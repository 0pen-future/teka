package attendance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
)

// --- fake RosterSource ---

type fakeRosterSource struct {
	// rosters is keyed by classID; every entry is treated as active on any
	// date the tests query, which is enough to exercise the service without
	// re-testing enrollments.ActiveOn's own boundary logic (covered in that
	// package's tests).
	rosters map[uuid.UUID][]enrollments.Enrollment
}

func newFakeRosterSource() *fakeRosterSource {
	return &fakeRosterSource{rosters: map[uuid.UUID][]enrollments.Enrollment{}}
}

func (f *fakeRosterSource) addEnrollment(classID, studentID uuid.UUID) enrollments.Enrollment {
	e := enrollments.Enrollment{ID: id.New(), ClassID: classID, StudentID: studentID}
	f.rosters[classID] = append(f.rosters[classID], e)
	return e
}

func (f *fakeRosterSource) ActiveOn(_ context.Context, _ authctx.Scope, classID uuid.UUID, _ time.Time) ([]enrollments.Enrollment, error) {
	return f.rosters[classID], nil
}

// --- fake SessionStore ---

type fakeSession struct {
	sessions.Session
}

type fakeSessionStore struct {
	rows map[uuid.UUID]*fakeSession
}

func newFakeSessionStore() *fakeSessionStore {
	return &fakeSessionStore{rows: map[uuid.UUID]*fakeSession{}}
}

func (f *fakeSessionStore) addSession(teacherID, classID uuid.UUID, on time.Time, status string) uuid.UUID {
	sessionID := id.New()
	f.rows[sessionID] = &fakeSession{Session: sessions.Session{
		ID: sessionID, TeacherID: teacherID, ClassID: classID, SessionDate: on, Status: status,
	}}
	return sessionID
}

func (f *fakeSessionStore) GetByID(_ context.Context, teacherID, sessionID uuid.UUID) (*sessions.Session, error) {
	r, ok := f.rows[sessionID]
	if !ok || r.TeacherID != teacherID {
		return nil, sessions.ErrNotFound
	}
	cp := r.Session
	return &cp, nil
}

func (f *fakeSessionStore) MarkHeldAndConfirmed(_ context.Context, teacherID, sessionID uuid.UUID, at time.Time) error {
	r, ok := f.rows[sessionID]
	if !ok || r.TeacherID != teacherID {
		return sessions.ErrNotFound
	}
	r.Status = sessions.StatusHeld
	r.AttendanceConfirmedAt = &at
	return nil
}

// --- fake Repository ---

type fakeRecordRow struct {
	Record
	deleted bool
}

type fakeRepository struct {
	rows  map[uuid.UUID]*fakeRecordRow // keyed by record id
	names map[uuid.UUID]StudentName
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{rows: map[uuid.UUID]*fakeRecordRow{}, names: map[uuid.UUID]StudentName{}}
}

func (f *fakeRepository) UpsertMany(_ context.Context, records []Record) error {
	for _, rec := range records {
		var existing *fakeRecordRow
		for _, r := range f.rows {
			if !r.deleted && r.SessionID == rec.SessionID && r.StudentID == rec.StudentID {
				existing = r
				break
			}
		}
		if existing != nil {
			existing.Status = rec.Status
			existing.EnrollmentID = rec.EnrollmentID
			existing.Billable = rec.Billable
			existing.Note = rec.Note
			existing.UpdatedAt = time.Now()
			continue
		}
		cp := rec
		f.rows[cp.ID] = &fakeRecordRow{Record: cp}
	}
	return nil
}

func (f *fakeRepository) ListBySession(_ context.Context, teacherID, sessionID uuid.UUID) ([]Record, error) {
	var out []Record
	for _, r := range f.rows {
		if r.deleted || r.TeacherID != teacherID || r.SessionID != sessionID {
			continue
		}
		out = append(out, r.Record)
	}
	return out, nil
}

func (f *fakeRepository) SoftDeleteMissing(_ context.Context, teacherID, sessionID uuid.UUID, keepStudentIDs []uuid.UUID) error {
	keep := make(map[uuid.UUID]bool, len(keepStudentIDs))
	for _, sid := range keepStudentIDs {
		keep[sid] = true
	}
	for _, r := range f.rows {
		if r.deleted || r.TeacherID != teacherID || r.SessionID != sessionID {
			continue
		}
		if !keep[r.StudentID] {
			r.deleted = true
		}
	}
	return nil
}

func (f *fakeRepository) StudentNames(_ context.Context, _ uuid.UUID, studentIDs []uuid.UUID) (map[uuid.UUID]StudentName, error) {
	out := make(map[uuid.UUID]StudentName, len(studentIDs))
	for _, sid := range studentIDs {
		out[sid] = f.names[sid]
	}
	return out, nil
}

func (f *fakeRepository) TallyByEnrollment(_ context.Context, teacherID uuid.UUID, _, _ time.Time) ([]EnrollmentTally, error) {
	byEnrollment := map[uuid.UUID]*EnrollmentTally{}
	for _, r := range f.rows {
		if r.deleted || r.TeacherID != teacherID {
			continue
		}
		t, ok := byEnrollment[r.EnrollmentID]
		if !ok {
			t = &EnrollmentTally{EnrollmentID: r.EnrollmentID}
			byEnrollment[r.EnrollmentID] = t
		}
		if r.Billable {
			t.BillableCount++
		}
		switch r.Status {
		case StatusAbsent:
			t.AbsentCount++
		case StatusPresent:
			t.PresentCount++
		}
	}
	out := make([]EnrollmentTally, 0, len(byEnrollment))
	for _, t := range byEnrollment {
		out = append(out, *t)
	}
	return out, nil
}

// --- noopTx ---

type noopTx struct{}

func (noopTx) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

// --- test wiring ---

type testDeps struct {
	repo     *fakeRepository
	roster   *fakeRosterSource
	sessions *fakeSessionStore
}

func newTestService() (*Service, *testDeps) {
	deps := &testDeps{repo: newFakeRepository(), roster: newFakeRosterSource(), sessions: newFakeSessionStore()}
	return NewService(deps.repo, deps.roster, deps.sessions, noopTx{}), deps
}

func TestConfirmEmptyAbsentListMarksEveryonePresent(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID, classID := id.New(), id.New()
	a := deps.roster.addEnrollment(classID, id.New())
	b := deps.roster.addEnrollment(classID, id.New())
	sessionID := deps.sessions.addSession(teacherID, classID, time.Now(), sessions.StatusPlanned)

	out, err := svc.Confirm(ctx, teacherID, sessionID, ConfirmRequest{})
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if len(out.Rows) != 2 {
		t.Fatalf("want 2 roster rows, got %d", len(out.Rows))
	}
	for _, row := range out.Rows {
		if row.Status == nil || *row.Status != StatusPresent {
			t.Fatalf("empty absent list must mark everyone present, got %+v", row)
		}
	}
	_ = a
	_ = b
}

func TestConfirmDedupsAbsentIDs(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID, classID := id.New(), id.New()
	studentID := id.New()
	deps.roster.addEnrollment(classID, studentID)
	sessionID := deps.sessions.addSession(teacherID, classID, time.Now(), sessions.StatusPlanned)

	out, err := svc.Confirm(ctx, teacherID, sessionID,
		ConfirmRequest{AbsentStudentIDs: []uuid.UUID{studentID, studentID}})
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if len(out.Rows) != 1 {
		t.Fatalf("duplicate absent ids must collapse to one record, got %d rows", len(out.Rows))
	}
	if out.Rows[0].Status == nil || *out.Rows[0].Status != StatusAbsent {
		t.Fatalf("want status absent, got %+v", out.Rows[0])
	}
}

func TestConfirmRejectsAbsentIDOutsideRoster(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID, classID := id.New(), id.New()
	deps.roster.addEnrollment(classID, id.New())
	sessionID := deps.sessions.addSession(teacherID, classID, time.Now(), sessions.StatusPlanned)

	stranger := id.New()
	_, err := svc.Confirm(ctx, teacherID, sessionID, ConfirmRequest{AbsentStudentIDs: []uuid.UUID{stranger}})
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeValidation {
		t.Fatalf("absent id outside roster must be 422, got %v", err)
	}
	if appErr.Fields["absent_student_ids"] == "" {
		t.Fatalf("422 must name the offending id in fields, got %+v", appErr.Fields)
	}
	if !errors.Is(err, ErrStudentNotEnrolled) {
		t.Fatalf("want ErrStudentNotEnrolled cause, got %v", err)
	}
}

func TestConfirmCancelledSessionIs409(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID, classID := id.New(), id.New()
	deps.roster.addEnrollment(classID, id.New())
	sessionID := deps.sessions.addSession(teacherID, classID, time.Now(), sessions.StatusCancelled)

	_, err := svc.Confirm(ctx, teacherID, sessionID, ConfirmRequest{})
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeConflict {
		t.Fatalf("confirming a cancelled session must be 409, got %v", err)
	}
	if !errors.Is(err, ErrSessionCancelled) {
		t.Fatalf("want ErrSessionCancelled cause, got %v", err)
	}
}

func TestConfirmMissingSessionIs404(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID := id.New()
	_ = deps

	_, err := svc.Confirm(ctx, teacherID, id.New(), ConfirmRequest{})
	if apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("missing session must be 404, got %v", err)
	}
}

func TestConfirmTransitionsSessionToHeld(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID, classID := id.New(), id.New()
	deps.roster.addEnrollment(classID, id.New())
	sessionID := deps.sessions.addSession(teacherID, classID, time.Now(), sessions.StatusPlanned)

	out, err := svc.Confirm(ctx, teacherID, sessionID, ConfirmRequest{})
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if out.Status != sessions.StatusHeld {
		t.Fatalf("confirm must transition the session to held, got %s", out.Status)
	}
	if out.AttendanceConfirmedAt == nil {
		t.Fatalf("confirm must stamp attendance_confirmed_at")
	}
}

func TestGetReturnsNullStatusBeforeFirstConfirm(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID, classID := id.New(), id.New()
	deps.roster.addEnrollment(classID, id.New())
	sessionID := deps.sessions.addSession(teacherID, classID, time.Now(), sessions.StatusPlanned)

	out, err := svc.Get(ctx, teacherID, sessionID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(out.Rows) != 1 || out.Rows[0].Status != nil {
		t.Fatalf("an unconfirmed session must have a null status, got %+v", out.Rows)
	}
}

func TestConfirmCrossTenantIs404(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	owner, stranger, classID := id.New(), id.New(), id.New()
	deps.roster.addEnrollment(classID, id.New())
	sessionID := deps.sessions.addSession(owner, classID, time.Now(), sessions.StatusPlanned)

	if _, err := svc.Get(ctx, stranger, sessionID); apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant get must be 404, got %v", err)
	}
	if _, err := svc.Confirm(ctx, stranger, sessionID, ConfirmRequest{}); apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant confirm must be 404, got %v", err)
	}
}
