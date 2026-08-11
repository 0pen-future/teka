package enrollments

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
)

type fakeClass struct {
	teacherID uuid.UUID
	name      string
	price     int64
}

type fakeStudent struct {
	teacherID uuid.UUID
	name      string
}

type fakeEnrollment struct {
	Enrollment
	deleted bool
}

// fakeRepository is an in-memory Repository enforcing the same invariants the
// SQL layer does: tenant-scoped reads, soft-delete filtering, and the
// uq_enrollments_active refusal of a second open enrollment.
type fakeRepository struct {
	rows     map[uuid.UUID]*fakeEnrollment
	classes  map[uuid.UUID]fakeClass
	students map[uuid.UUID]fakeStudent
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		rows:     map[uuid.UUID]*fakeEnrollment{},
		classes:  map[uuid.UUID]fakeClass{},
		students: map[uuid.UUID]fakeStudent{},
	}
}

func (f *fakeRepository) addClass(teacherID uuid.UUID, price int64) uuid.UUID {
	classID := id.New()
	f.classes[classID] = fakeClass{teacherID: teacherID, name: "Toán 8", price: price}
	return classID
}

func (f *fakeRepository) addStudent(teacherID uuid.UUID) uuid.UUID {
	studentID := id.New()
	f.students[studentID] = fakeStudent{teacherID: teacherID, name: "Bé An"}
	return studentID
}

func (f *fakeRepository) row(e *fakeEnrollment) Row {
	return Row{
		Enrollment:  e.Enrollment,
		StudentName: f.students[e.StudentID].name,
		ClassName:   f.classes[e.ClassID].name,
	}
}

func (f *fakeRepository) Create(_ context.Context, e *Enrollment) error {
	for _, existing := range f.rows {
		if !existing.deleted && existing.StudentID == e.StudentID &&
			existing.ClassID == e.ClassID && existing.EndedOn == nil {
			return ErrAlreadyEnrolled
		}
	}
	f.rows[e.ID] = &fakeEnrollment{Enrollment: *e}
	return nil
}

func (f *fakeRepository) GetByID(_ context.Context, teacherID, id uuid.UUID) (*Row, error) {
	e, ok := f.rows[id]
	if !ok || e.deleted || e.TeacherID != teacherID {
		return nil, ErrNotFound
	}
	row := f.row(e)
	return &row, nil
}

func (f *fakeRepository) List(_ context.Context, teacherID uuid.UUID, filter ListFilter, _ pagination.Params) ([]Row, int64, error) {
	var out []Row
	for _, e := range f.rows {
		if e.deleted || e.TeacherID != teacherID {
			continue
		}
		if filter.StudentID != uuid.Nil && e.StudentID != filter.StudentID {
			continue
		}
		if filter.ClassID != uuid.Nil && e.ClassID != filter.ClassID {
			continue
		}
		if filter.Active != nil && *filter.Active != (e.EndedOn == nil) {
			continue
		}
		out = append(out, f.row(e))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedOn.Before(out[j].StartedOn) })
	return out, int64(len(out)), nil
}

func (f *fakeRepository) End(_ context.Context, teacherID, id uuid.UUID, endedOn time.Time) error {
	e, ok := f.rows[id]
	if !ok || e.deleted || e.TeacherID != teacherID {
		return ErrNotFound
	}
	if e.EndedOn != nil {
		// Mirror the real repo: an already-closed row is a 409, not a 404, so a
		// concurrent double-end loser does not retry against a set date.
		return ErrAlreadyEnded
	}
	e.EndedOn = &endedOn
	return nil
}

func (f *fakeRepository) SoftDelete(_ context.Context, teacherID, id uuid.UUID) error {
	e, ok := f.rows[id]
	if !ok || e.deleted || e.TeacherID != teacherID {
		return ErrNotFound
	}
	e.deleted = true
	return nil
}

func (f *fakeRepository) ActiveOn(_ context.Context, teacherID, classID uuid.UUID, on time.Time) ([]Enrollment, error) {
	var out []Enrollment
	for _, e := range f.rows {
		if e.deleted || e.TeacherID != teacherID || e.ClassID != classID {
			continue
		}
		if e.StartedOn.After(on) {
			continue
		}
		if e.EndedOn != nil && e.EndedOn.Before(on) {
			continue
		}
		out = append(out, e.Enrollment)
	}
	return out, nil
}

func (f *fakeRepository) EndOpenEnrollments(_ context.Context, sc authctx.Scope, studentID uuid.UUID, on time.Time) error {
	for _, e := range f.rows {
		if !e.deleted && e.TeacherID == sc.TeacherID && e.StudentID == studentID && e.EndedOn == nil {
			ended := on
			e.EndedOn = &ended
		}
	}
	return nil
}

func (f *fakeRepository) ClassDefaultPrice(_ context.Context, teacherID, classID uuid.UUID) (int64, error) {
	c, ok := f.classes[classID]
	if !ok || c.teacherID != teacherID {
		return 0, ErrClassNotFound
	}
	return c.price, nil
}

func (f *fakeRepository) StudentExists(_ context.Context, teacherID, studentID uuid.UUID) (bool, error) {
	s, ok := f.students[studentID]
	return ok && s.teacherID == teacherID, nil
}

func newTestService() (*Service, *fakeRepository) {
	repo := newFakeRepository()
	return NewService(repo), repo
}

func enroll(t *testing.T, svc *Service, teacherID, studentID, classID uuid.UUID, startedOn string) *Row {
	t.Helper()
	row, err := svc.Create(context.Background(), teacherID, CreateRequest{
		StudentID: studentID, ClassID: classID, StartedOn: startedOn,
	})
	if err != nil {
		t.Fatalf("enroll: %v", err)
	}
	return row
}

func TestCreateCopiesUnitPriceFromClass(t *testing.T) {
	svc, repo := newTestService()
	teacherID := id.New()
	classID := repo.addClass(teacherID, 150_000)
	studentID := repo.addStudent(teacherID)

	row := enroll(t, svc, teacherID, studentID, classID, "2026-01-15")
	if row.UnitPrice != 150_000 {
		t.Fatalf("unit_price must be copied from the class, got %d", row.UnitPrice)
	}
	if got := row.StartedOn.Format(dateLayout); got != "2026-01-15" {
		t.Fatalf("started_on must be stored verbatim (mid-cycle join), got %s", got)
	}
	if row.StudentName != "Bé An" || row.ClassName != "Toán 8" {
		t.Fatalf("row must carry display names, got %+v", row)
	}
}

// The DTOs are the enforcement of "V1 không cho sửa đơn giá": unit_price must
// have no path from a request into the database. If this test went red after
// adding the field, answer PRD section 4 first.
func TestWritableDTOsNeverExposeUnitPrice(t *testing.T) {
	for name, typ := range map[string]reflect.Type{
		"CreateRequest": reflect.TypeOf(CreateRequest{}),
		"EndRequest":    reflect.TypeOf(EndRequest{}),
	} {
		for i := range typ.NumField() {
			tag := strings.SplitN(typ.Field(i).Tag.Get("json"), ",", 2)[0]
			if tag == "unit_price" {
				t.Fatalf("%s must not expose unit_price", name)
			}
		}
	}
}

func TestCreateDefaultsStartedOnToToday(t *testing.T) {
	svc, repo := newTestService()
	teacherID := id.New()
	classID := repo.addClass(teacherID, 150_000)
	studentID := repo.addStudent(teacherID)

	row := enroll(t, svc, teacherID, studentID, classID, "")
	if got, want := row.StartedOn.Format(dateLayout), today().Format(dateLayout); got != want {
		t.Fatalf("started_on must default to today (%s), got %s", want, got)
	}
}

func TestCreateRejectsForeignReferences(t *testing.T) {
	svc, repo := newTestService()
	teacherID := id.New()
	stranger := id.New()
	ownClass := repo.addClass(teacherID, 150_000)
	ownStudent := repo.addStudent(teacherID)
	foreignClass := repo.addClass(stranger, 150_000)
	foreignStudent := repo.addStudent(stranger)

	_, err := svc.Create(context.Background(), teacherID, CreateRequest{
		StudentID: ownStudent, ClassID: foreignClass,
	})
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeValidation || appErr.Fields["class_id"] == "" {
		t.Fatalf("foreign class must be 422 naming class_id, got %v", err)
	}
	if !errors.Is(err, ErrClassNotFound) {
		t.Fatalf("want ErrClassNotFound cause, got %v", err)
	}

	_, err = svc.Create(context.Background(), teacherID, CreateRequest{
		StudentID: foreignStudent, ClassID: ownClass,
	})
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeValidation || appErr.Fields["student_id"] == "" {
		t.Fatalf("foreign student must be 422 naming student_id, got %v", err)
	}
	if !errors.Is(err, ErrStudentNotFound) {
		t.Fatalf("want ErrStudentNotFound cause, got %v", err)
	}
}

func TestEnrollingTwiceConflicts(t *testing.T) {
	svc, repo := newTestService()
	teacherID := id.New()
	classID := repo.addClass(teacherID, 150_000)
	studentID := repo.addStudent(teacherID)

	enroll(t, svc, teacherID, studentID, classID, "2026-01-05")
	_, err := svc.Create(context.Background(), teacherID, CreateRequest{
		StudentID: studentID, ClassID: classID, StartedOn: "2026-02-01",
	})
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeConflict {
		t.Fatalf("second open enrollment must be 409, got %v", err)
	}
	if !errors.Is(err, ErrAlreadyEnrolled) {
		t.Fatalf("want ErrAlreadyEnrolled cause, got %v", err)
	}
}

func TestEndAndReenroll(t *testing.T) {
	svc, repo := newTestService()
	teacherID := id.New()
	classID := repo.addClass(teacherID, 150_000)
	studentID := repo.addStudent(teacherID)

	first := enroll(t, svc, teacherID, studentID, classID, "2026-01-05")
	ended, err := svc.End(context.Background(), teacherID, first.ID, EndRequest{EndedOn: "2026-03-31"})
	if err != nil {
		t.Fatalf("end: %v", err)
	}
	if ended.EndedOn == nil || ended.EndedOn.Format(dateLayout) != "2026-03-31" {
		t.Fatalf("ended_on must be stamped, got %v", ended.EndedOn)
	}

	// Ending twice: 409, and the date does not move.
	_, err = svc.End(context.Background(), teacherID, first.ID, EndRequest{EndedOn: "2026-04-15"})
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeConflict || !errors.Is(err, ErrAlreadyEnded) {
		t.Fatalf("double end must be 409 ErrAlreadyEnded, got %v", err)
	}
	unchanged, err := svc.Get(context.Background(), teacherID, first.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if unchanged.EndedOn.Format(dateLayout) != "2026-03-31" {
		t.Fatalf("a refused double end must not move the date, got %v", unchanged.EndedOn)
	}

	// Re-enrolling after leaving succeeds; the old row survives.
	second := enroll(t, svc, teacherID, studentID, classID, "2026-05-01")
	if second.ID == first.ID {
		t.Fatal("re-enrollment must be a new row")
	}
	if _, err := svc.Get(context.Background(), teacherID, first.ID); err != nil {
		t.Fatalf("the ended row must stay readable, got %v", err)
	}
}

func TestEndBeforeStartIsValidationError(t *testing.T) {
	svc, repo := newTestService()
	teacherID := id.New()
	classID := repo.addClass(teacherID, 150_000)
	studentID := repo.addStudent(teacherID)

	row := enroll(t, svc, teacherID, studentID, classID, "2026-02-10")
	_, err := svc.End(context.Background(), teacherID, row.ID, EndRequest{EndedOn: "2026-02-09"})
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeValidation || appErr.Fields["ended_on"] == "" {
		t.Fatalf("ended_on before started_on must be 422 naming ended_on, got %v", err)
	}

	// The boundary itself is allowed: ending on the start date is one paid
	// session, not an error.
	if _, err := svc.End(context.Background(), teacherID, row.ID, EndRequest{EndedOn: "2026-02-10"}); err != nil {
		t.Fatalf("ended_on == started_on must be allowed, got %v", err)
	}
}

func TestEndOpenEnrollmentsClosesOnlyThatStudent(t *testing.T) {
	svc, repo := newTestService()
	teacherID := id.New()
	classID := repo.addClass(teacherID, 150_000)
	leaver := repo.addStudent(teacherID)
	stayer := repo.addStudent(teacherID)

	leaverRow := enroll(t, svc, teacherID, leaver, classID, "2026-01-05")
	stayerRow := enroll(t, svc, teacherID, stayer, classID, "2026-01-05")

	on := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	sc := authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
	if err := svc.EndOpenEnrollments(context.Background(), sc, leaver, on); err != nil {
		t.Fatalf("end open enrollments: %v", err)
	}
	if row, _ := svc.Get(context.Background(), teacherID, leaverRow.ID); row.EndedOn == nil {
		t.Fatal("the leaver's enrollment must be closed")
	}
	if row, _ := svc.Get(context.Background(), teacherID, stayerRow.ID); row.EndedOn != nil {
		t.Fatal("the other student's enrollment must stay open")
	}
}

func TestCrossTenantReadsAsNotFound(t *testing.T) {
	svc, repo := newTestService()
	owner := id.New()
	classID := repo.addClass(owner, 150_000)
	studentID := repo.addStudent(owner)
	row := enroll(t, svc, owner, studentID, classID, "2026-01-05")

	stranger := id.New()
	var appErr *apperror.AppError
	if _, err := svc.Get(context.Background(), stranger, row.ID); !errors.As(err, &appErr) || appErr.Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant get must be NOT_FOUND, got %v", err)
	}
	if _, err := svc.End(context.Background(), stranger, row.ID, EndRequest{}); !errors.As(err, &appErr) || appErr.Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant end must be NOT_FOUND, got %v", err)
	}
	if err := svc.Delete(context.Background(), stranger, row.ID); !errors.As(err, &appErr) || appErr.Code != apperror.CodeNotFound {
		t.Fatalf("cross-tenant delete must be NOT_FOUND, got %v", err)
	}
}
