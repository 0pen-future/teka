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

// addSession stamps CenterID the same as teacherID: these unit tests exercise
// a single teacher acting as the sole owner of their own center, so the two
// ids coincide by construction (see the sc := authctx.Scope{...} literals
// below). Multi-tenant center semantics are covered by the real-DB tests in
// integration_test.go.
func (f *fakeSessionStore) addSession(teacherID, classID uuid.UUID, on time.Time, status string) uuid.UUID {
	sessionID := id.New()
	f.rows[sessionID] = &fakeSession{Session: sessions.Session{
		ID: sessionID, TeacherID: teacherID, CenterID: teacherID, ClassID: classID, SessionDate: on, Status: status,
	}}
	return sessionID
}

// visibleSession mirrors gormRepository.scoped: an owner sees every row in
// their center, a member sees only the ones they teach themselves.
func visibleSession(sc authctx.Scope, r *fakeSession) bool {
	if r.CenterID != sc.CenterID {
		return false
	}
	return sc.IsOwner || r.TeacherID == sc.TeacherID
}

func (f *fakeSessionStore) GetWritable(_ context.Context, sc authctx.Scope, sessionID uuid.UUID, _ authctx.ClassCapability) (*sessions.Session, error) {
	r, ok := f.rows[sessionID]
	if !ok || !visibleSession(sc, r) {
		return nil, sessions.ErrNotFound
	}
	cp := r.Session
	return &cp, nil
}

// GetReadableByID mirrors GetWritable: the unit fakes carry no class_staff
// table, so the readable port collapses onto the own-rows one.
func (f *fakeSessionStore) GetReadableByID(ctx context.Context, sc authctx.Scope, sessionID uuid.UUID) (*sessions.Session, error) {
	return f.GetWritable(ctx, sc, sessionID, authctx.CapAttendanceWrite)
}

func (f *fakeSessionStore) MarkHeldAndConfirmed(_ context.Context, sc authctx.Scope, sessionID uuid.UUID, at time.Time) error {
	r, ok := f.rows[sessionID]
	if !ok || !visibleSession(sc, r) {
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

// visibleRecord mirrors gormRepository.scoped: an owner sees every row in
// their center, a member sees only the ones they teach themselves.
func visibleRecord(sc authctx.Scope, r *fakeRecordRow) bool {
	if r.CenterID != sc.CenterID {
		return false
	}
	return sc.IsOwner || r.TeacherID == sc.TeacherID
}

func (f *fakeRepository) ListBySession(_ context.Context, sc authctx.Scope, sessionID uuid.UUID) ([]Record, error) {
	var out []Record
	for _, r := range f.rows {
		if r.deleted || !visibleRecord(sc, r) || r.SessionID != sessionID {
			continue
		}
		out = append(out, r.Record)
	}
	return out, nil
}

func (f *fakeRepository) SoftDeleteMissing(_ context.Context, sc authctx.Scope, sessionID uuid.UUID, keepStudentIDs []uuid.UUID) error {
	keep := make(map[uuid.UUID]bool, len(keepStudentIDs))
	for _, sid := range keepStudentIDs {
		keep[sid] = true
	}
	for _, r := range f.rows {
		if r.deleted || !visibleRecord(sc, r) || r.SessionID != sessionID {
			continue
		}
		if !keep[r.StudentID] {
			r.deleted = true
		}
	}
	return nil
}

func (f *fakeRepository) StudentNames(_ context.Context, _ authctx.Scope, studentIDs []uuid.UUID) (map[uuid.UUID]StudentName, error) {
	out := make(map[uuid.UUID]StudentName, len(studentIDs))
	for _, sid := range studentIDs {
		out[sid] = f.names[sid]
	}
	return out, nil
}

func (f *fakeRepository) TallyByEnrollment(_ context.Context, sc authctx.Scope, _, _ time.Time) ([]EnrollmentTally, error) {
	byEnrollment := map[uuid.UUID]*EnrollmentTally{}
	for _, r := range f.rows {
		if r.deleted || !visibleRecord(sc, r) {
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
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	a := deps.roster.addEnrollment(classID, id.New())
	b := deps.roster.addEnrollment(classID, id.New())
	sessionID := deps.sessions.addSession(teacherID, classID, time.Now(), sessions.StatusPlanned)

	out, err := svc.Confirm(ctx, sc, sessionID, ConfirmRequest{})
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
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	studentID := id.New()
	deps.roster.addEnrollment(classID, studentID)
	sessionID := deps.sessions.addSession(teacherID, classID, time.Now(), sessions.StatusPlanned)

	out, err := svc.Confirm(ctx, sc, sessionID,
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
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	deps.roster.addEnrollment(classID, id.New())
	sessionID := deps.sessions.addSession(teacherID, classID, time.Now(), sessions.StatusPlanned)

	stranger := id.New()
	_, err := svc.Confirm(ctx, sc, sessionID, ConfirmRequest{AbsentStudentIDs: []uuid.UUID{stranger}})
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
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	deps.roster.addEnrollment(classID, id.New())
	sessionID := deps.sessions.addSession(teacherID, classID, time.Now(), sessions.StatusCancelled)

	_, err := svc.Confirm(ctx, sc, sessionID, ConfirmRequest{})
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
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	_ = deps

	_, err := svc.Confirm(ctx, sc, id.New(), ConfirmRequest{})
	if apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("missing session must be 404, got %v", err)
	}
}

func TestConfirmTransitionsSessionToHeld(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID, classID := id.New(), id.New()
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	deps.roster.addEnrollment(classID, id.New())
	sessionID := deps.sessions.addSession(teacherID, classID, time.Now(), sessions.StatusPlanned)

	out, err := svc.Confirm(ctx, sc, sessionID, ConfirmRequest{})
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
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	deps.roster.addEnrollment(classID, id.New())
	sessionID := deps.sessions.addSession(teacherID, classID, time.Now(), sessions.StatusPlanned)

	out, err := svc.Get(ctx, sc, sessionID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(out.Rows) != 1 || out.Rows[0].Status != nil {
		t.Fatalf("an unconfirmed session must have a null status, got %+v", out.Rows)
	}
}

func TestConfirmMarksMixedStatuses(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID, classID := id.New(), id.New()
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	unlisted := deps.roster.addEnrollment(classID, id.New())
	late := deps.roster.addEnrollment(classID, id.New())
	absent := deps.roster.addEnrollment(classID, id.New())
	excused := deps.roster.addEnrollment(classID, id.New())
	sessionID := deps.sessions.addSession(teacherID, classID, time.Now(), sessions.StatusPlanned)

	out, err := svc.Confirm(ctx, sc, sessionID, ConfirmRequest{
		Marks: []ConfirmMark{
			{StudentID: late.StudentID, Status: StatusLate},
			{StudentID: absent.StudentID, Status: StatusAbsent},
			{StudentID: excused.StudentID, Status: StatusExcused, Note: "mẹ báo ốm"},
		},
		Note: "lớp học bù",
	})
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	byStudent := map[uuid.UUID]RowResponse{}
	for _, row := range out.Rows {
		byStudent[row.StudentID] = row
	}
	assertRow := func(studentID uuid.UUID, wantStatus string, wantNote string) {
		t.Helper()
		row, ok := byStudent[studentID]
		if !ok || row.Status == nil || *row.Status != wantStatus {
			t.Fatalf("want student %s status %s, got %+v", studentID, wantStatus, row)
		}
		if !row.Billable {
			t.Fatalf("every status must stay billable=true, got %+v", row)
		}
		if wantNote == "" {
			return
		}
		if row.Note == nil || *row.Note != wantNote {
			t.Fatalf("want note %q, got %+v", wantNote, row.Note)
		}
	}
	assertRow(unlisted.StudentID, StatusPresent, "lớp học bù")
	assertRow(late.StudentID, StatusLate, "lớp học bù")
	assertRow(absent.StudentID, StatusAbsent, "lớp học bù")
	// A per-student note wins over the session-level one for that student.
	assertRow(excused.StudentID, StatusExcused, "mẹ báo ốm")
}

func TestConfirmMarksReplacePreviousStatuses(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID, classID := id.New(), id.New()
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	a := deps.roster.addEnrollment(classID, id.New())
	b := deps.roster.addEnrollment(classID, id.New())
	sessionID := deps.sessions.addSession(teacherID, classID, time.Now(), sessions.StatusPlanned)

	if _, err := svc.Confirm(ctx, sc, sessionID, ConfirmRequest{
		Marks: []ConfirmMark{{StudentID: a.StudentID, Status: StatusAbsent}},
	}); err != nil {
		t.Fatalf("first confirm: %v", err)
	}
	out, err := svc.Confirm(ctx, sc, sessionID, ConfirmRequest{
		Marks: []ConfirmMark{{StudentID: b.StudentID, Status: StatusLate}},
	})
	if err != nil {
		t.Fatalf("re-confirm: %v", err)
	}
	for _, row := range out.Rows {
		switch row.StudentID {
		case a.StudentID:
			if row.Status == nil || *row.Status != StatusPresent {
				t.Fatalf("re-confirm must reset an unlisted student to present, got %+v", row)
			}
		case b.StudentID:
			if row.Status == nil || *row.Status != StatusLate {
				t.Fatalf("re-confirm must apply the new mark, got %+v", row)
			}
		}
	}
}

func TestConfirmRejectsMarksCombinedWithLegacyBody(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID, classID := id.New(), id.New()
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	e := deps.roster.addEnrollment(classID, id.New())
	sessionID := deps.sessions.addSession(teacherID, classID, time.Now(), sessions.StatusPlanned)

	_, err := svc.Confirm(ctx, sc, sessionID, ConfirmRequest{
		Marks:            []ConfirmMark{{StudentID: e.StudentID, Status: StatusLate}},
		AbsentStudentIDs: []uuid.UUID{e.StudentID},
	})
	if apperror.From(err).Code != apperror.CodeBadRequest {
		t.Fatalf("sending both marks and absent_student_ids must be 400, got %v", err)
	}
}

func TestConfirmRejectsDuplicateStudentInMarks(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID, classID := id.New(), id.New()
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	e := deps.roster.addEnrollment(classID, id.New())
	sessionID := deps.sessions.addSession(teacherID, classID, time.Now(), sessions.StatusPlanned)

	_, err := svc.Confirm(ctx, sc, sessionID, ConfirmRequest{
		Marks: []ConfirmMark{
			{StudentID: e.StudentID, Status: StatusLate},
			{StudentID: e.StudentID, Status: StatusAbsent},
		},
	})
	if apperror.From(err).Code != apperror.CodeBadRequest {
		t.Fatalf("duplicate student_id in marks must be 400, got %v", err)
	}
}

func TestConfirmRejectsMarkOutsideRoster(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID, classID := id.New(), id.New()
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	deps.roster.addEnrollment(classID, id.New())
	sessionID := deps.sessions.addSession(teacherID, classID, time.Now(), sessions.StatusPlanned)

	stranger := id.New()
	_, err := svc.Confirm(ctx, sc, sessionID, ConfirmRequest{
		Marks: []ConfirmMark{{StudentID: stranger, Status: StatusAbsent}},
	})
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeValidation {
		t.Fatalf("mark outside roster must be 422, got %v", err)
	}
	if appErr.Fields["marks"] == "" {
		t.Fatalf("422 must name the offending id under the marks field, got %+v", appErr.Fields)
	}
	if !errors.Is(err, ErrStudentNotEnrolled) {
		t.Fatalf("want ErrStudentNotEnrolled cause, got %v", err)
	}
}

func TestConfirmRejectsInvalidMarkStatus(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	teacherID, classID := id.New(), id.New()
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	e := deps.roster.addEnrollment(classID, id.New())
	sessionID := deps.sessions.addSession(teacherID, classID, time.Now(), sessions.StatusPlanned)

	// present is the implicit default and everything else is not a status:
	// the service enforces the vocabulary itself so non-HTTP callers get the
	// same contract as the gin binding.
	for _, status := range []string{StatusPresent, "tardy", ""} {
		_, err := svc.Confirm(ctx, sc, sessionID, ConfirmRequest{
			Marks: []ConfirmMark{{StudentID: e.StudentID, Status: status}},
		})
		if apperror.From(err).Code != apperror.CodeValidation {
			t.Fatalf("mark status %q must be 422, got %v", status, err)
		}
	}
}

func TestConfirmCrossTenantIs404(t *testing.T) {
	svc, deps := newTestService()
	ctx := context.Background()
	owner, stranger, classID := id.New(), id.New(), id.New()
	strangerScope := authctx.Scope{TeacherID: stranger, CenterID: stranger, IsOwner: true}
	deps.roster.addEnrollment(classID, id.New())
	sessionID := deps.sessions.addSession(owner, classID, time.Now(), sessions.StatusPlanned)

	if _, err := svc.Get(ctx, strangerScope, sessionID); apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant get must be 404, got %v", err)
	}
	if _, err := svc.Confirm(ctx, strangerScope, sessionID, ConfirmRequest{}); apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant confirm must be 404, got %v", err)
	}
}
