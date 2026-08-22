package imports

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/contacts"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/students"
	"teka/apps/api/internal/shared/authctx"
)

// fakeRoster stands in for the four roster services this feature drives. It
// keeps rows in slices and filters every read by the scope it was handed, so a
// test that leaks the wrong anchor gets a miss rather than a silent match —
// which is exactly the bug the anchoring rule exists to prevent.
type fakeRoster struct {
	classRows    []*classes.Class
	scheduleRows []fakeSchedule
	contactRows  []*contacts.Contact
	studentRows  []*students.Student
	enrollRows   []*enrollments.Enrollment

	// calls records write calls in order, so a test can assert that a failure
	// stopped the sequence instead of merely changing its result.
	calls []string
	// failOn maps a call name ("students.Create") to the error it returns.
	failOn map[string]error
}

type fakeSchedule struct {
	classID       uuid.UUID
	teacherID     uuid.UUID
	centerID      uuid.UUID
	weekday       int16
	startTime     classes.TimeOfDay
	effectiveFrom time.Time
}

func newFakeRoster() *fakeRoster {
	return &fakeRoster{failOn: map[string]error{}}
}

// record logs a write and returns the configured failure for it, if any.
func (f *fakeRoster) record(name string) error {
	f.calls = append(f.calls, name)
	return f.failOn[name]
}

// snapshot/restore give rollbackTxManager the same all-or-nothing guarantee a
// real transaction gives: rows only ever append, so lengths are enough.
type rosterSnapshot struct{ classes, schedules, contacts, students, enrolls int }

func (f *fakeRoster) snapshot() rosterSnapshot {
	return rosterSnapshot{
		classes:   len(f.classRows),
		schedules: len(f.scheduleRows),
		contacts:  len(f.contactRows),
		students:  len(f.studentRows),
		enrolls:   len(f.enrollRows),
	}
}

func (f *fakeRoster) restore(s rosterSnapshot) {
	f.classRows = f.classRows[:s.classes]
	f.scheduleRows = f.scheduleRows[:s.schedules]
	f.contactRows = f.contactRows[:s.contacts]
	f.studentRows = f.studentRows[:s.students]
	f.enrollRows = f.enrollRows[:s.enrolls]
}

// --- ClassWriter ---

func (f *fakeRoster) FindActiveByName(_ context.Context, sc authctx.Scope, name string) (*classes.Class, bool, error) {
	for _, c := range f.classRows {
		if c.TeacherID == sc.TeacherID && c.CenterID == sc.CenterID &&
			c.Name == name && c.Status == classes.StatusActive {
			return c, true, nil
		}
	}
	return nil, false, nil
}

func (f *fakeRoster) ScheduleExists(_ context.Context, sc authctx.Scope, classID uuid.UUID,
	weekday int16, startTime classes.TimeOfDay, effectiveFrom time.Time) (bool, error) {
	for _, s := range f.scheduleRows {
		if s.classID == classID && s.teacherID == sc.TeacherID && s.centerID == sc.CenterID &&
			s.weekday == weekday && s.startTime == startTime && s.effectiveFrom.Equal(effectiveFrom) {
			return true, nil
		}
	}
	return false, nil
}

func (f *fakeRoster) Create(_ context.Context, sc authctx.Scope, req classes.CreateClassRequest) (*classes.Class, error) {
	if err := f.record("classes.Create"); err != nil {
		return nil, err
	}
	start, err := time.Parse(dateWireLayout, req.StartDate)
	if err != nil {
		return nil, err
	}
	var end *time.Time
	if req.EndDate != "" {
		parsed, err := time.Parse(dateWireLayout, req.EndDate)
		if err != nil {
			return nil, err
		}
		end = &parsed
	}
	row := &classes.Class{
		ID:               uuid.New(),
		TeacherID:        sc.TeacherID,
		CenterID:         sc.CenterID,
		Name:             req.Name,
		StartDate:        start,
		EndDate:          end,
		DefaultUnitPrice: *req.DefaultUnitPrice,
		Status:           classes.StatusActive,
	}
	f.classRows = append(f.classRows, row)
	for _, s := range req.Schedules {
		if _, err := f.AddSchedule(context.Background(), sc, row.ID, s); err != nil {
			return nil, err
		}
	}
	return row, nil
}

func (f *fakeRoster) AddSchedule(_ context.Context, sc authctx.Scope, classID uuid.UUID,
	req classes.ScheduleRequest) (*classes.Schedule, error) {
	if err := f.record("classes.AddSchedule"); err != nil {
		return nil, err
	}
	from, err := time.Parse(dateWireLayout, req.EffectiveFrom)
	if err != nil {
		return nil, err
	}
	f.scheduleRows = append(f.scheduleRows, fakeSchedule{
		classID:       classID,
		teacherID:     sc.TeacherID,
		centerID:      sc.CenterID,
		weekday:       *req.Weekday,
		startTime:     classes.TimeOfDay(req.StartTime),
		effectiveFrom: from,
	})
	return &classes.Schedule{ID: uuid.New(), ClassID: classID}, nil
}

// --- ContactWriter ---

func (f *fakeRoster) FindIDByPhone(_ context.Context, sc authctx.Scope, phone string) (uuid.UUID, bool, error) {
	for _, c := range f.contactRows {
		if c.TeacherID == sc.TeacherID && c.CenterID == sc.CenterID && c.Phone == phone {
			return c.ID, true, nil
		}
	}
	return uuid.Nil, false, nil
}

func (f *fakeRoster) CreateContact(_ context.Context, sc authctx.Scope, req contacts.CreateRequest) (*contacts.Row, error) {
	if err := f.record("contacts.Create"); err != nil {
		return nil, err
	}
	row := &contacts.Contact{
		ID:        uuid.New(),
		TeacherID: sc.TeacherID,
		CenterID:  sc.CenterID,
		FullName:  req.FullName,
		Phone:     req.Phone,
	}
	f.contactRows = append(f.contactRows, row)
	return &contacts.Row{Contact: *row}, nil
}

// --- StudentWriter ---

func (f *fakeRoster) FindIDByName(_ context.Context, sc authctx.Scope, contactID uuid.UUID,
	fullName string, note *string) (uuid.UUID, bool, error) {
	for _, s := range f.studentRows {
		if s.TeacherID == sc.TeacherID && s.CenterID == sc.CenterID &&
			s.ContactID == contactID && s.FullName == fullName && samePtr(s.DisplayNote, note) {
			return s.ID, true, nil
		}
	}
	return uuid.Nil, false, nil
}

func (f *fakeRoster) CreateStudent(_ context.Context, sc authctx.Scope, req students.CreateRequest) (*students.Row, error) {
	if err := f.record("students.Create"); err != nil {
		return nil, err
	}
	var note *string
	if req.DisplayNote != "" {
		note = &req.DisplayNote
	}
	row := &students.Student{
		ID:          uuid.New(),
		TeacherID:   sc.TeacherID,
		CenterID:    sc.CenterID,
		ContactID:   req.ContactID,
		FullName:    req.FullName,
		DisplayNote: note,
	}
	f.studentRows = append(f.studentRows, row)
	return &students.Row{Student: *row}, nil
}

// --- EnrollmentWriter ---

func (f *fakeRoster) FindByStudentAndClass(_ context.Context, sc authctx.Scope,
	studentID, classID uuid.UUID) (*enrollments.Enrollment, bool, error) {
	for _, e := range f.enrollRows {
		if e.TeacherID == sc.TeacherID && e.CenterID == sc.CenterID &&
			e.StudentID == studentID && e.ClassID == classID {
			return e, true, nil
		}
	}
	return nil, false, nil
}

func (f *fakeRoster) CreateEnrollment(_ context.Context, sc authctx.Scope,
	req enrollments.CreateRequest) (*enrollments.Row, error) {
	if err := f.record("enrollments.Create"); err != nil {
		return nil, err
	}
	started, err := time.Parse(dateWireLayout, req.StartedOn)
	if err != nil {
		return nil, err
	}
	row := &enrollments.Enrollment{
		ID:        uuid.New(),
		TeacherID: sc.TeacherID,
		CenterID:  sc.CenterID,
		StudentID: req.StudentID,
		ClassID:   req.ClassID,
		StartedOn: started,
	}
	f.enrollRows = append(f.enrollRows, row)
	return &enrollments.Row{Enrollment: *row}, nil
}

// samePtr compares two optional notes the way display_note IS NOT DISTINCT
// FROM compares them.
func samePtr(a, b *string) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return *a == *b
	}
}

// The four writer interfaces all name their creator Create, so one struct
// cannot implement them directly. These adapters give each interface its own
// receiver over the same store.
type contactWriter struct{ *fakeRoster }

func (w contactWriter) Create(ctx context.Context, sc authctx.Scope, req contacts.CreateRequest) (*contacts.Row, error) {
	return w.CreateContact(ctx, sc, req)
}

type studentWriter struct{ *fakeRoster }

func (w studentWriter) Create(ctx context.Context, sc authctx.Scope, req students.CreateRequest) (*students.Row, error) {
	return w.CreateStudent(ctx, sc, req)
}

type enrollmentWriter struct{ *fakeRoster }

func (w enrollmentWriter) Create(ctx context.Context, sc authctx.Scope, req enrollments.CreateRequest) (*enrollments.Row, error) {
	return w.CreateEnrollment(ctx, sc, req)
}

// rollbackTxManager reproduces the one transaction property these tests depend
// on: an error unwinds every write fn made.
type rollbackTxManager struct{ store *fakeRoster }

func (m rollbackTxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	saved := m.store.snapshot()
	if err := fn(ctx); err != nil {
		m.store.restore(saved)
		return err
	}
	return nil
}

// fakeLocker answers TryLockCenter with whatever the test set.
type fakeLocker struct {
	locked   bool
	lockErr  error
	timeouts int
	centers  []uuid.UUID
}

func (l *fakeLocker) TryLockCenter(_ context.Context, centerID uuid.UUID) (bool, error) {
	l.centers = append(l.centers, centerID)
	return l.locked, l.lockErr
}

func (l *fakeLocker) SetStatementTimeout(_ context.Context) error {
	l.timeouts++
	return nil
}

// newTestService wires the imports service over the in-memory store.
func newTestService(dir MemberDirectory, store *fakeRoster) (*Service, *fakeLocker) {
	locker := &fakeLocker{locked: true}
	return NewService(dir, store, contactWriter{store}, studentWriter{store},
		enrollmentWriter{store}, locker, rollbackTxManager{store: store}), locker
}

// errWriter is the failure a test injects into a writer.
var errWriter = errors.New("database exploded")
