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
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/events"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
)

type fakeClass struct {
	centerID  uuid.UUID
	teacherID uuid.UUID
	name      string
	price     int64
}

type fakeStudent struct {
	centerID  uuid.UUID
	teacherID uuid.UUID
	name      string
}

type fakeEnrollment struct {
	Enrollment
	deleted bool
}

// fakeRepository is an in-memory Repository enforcing the same invariants the
// SQL layer does: center-scoped reads (owner sees the whole center, a member
// only their own rows), soft-delete filtering, and the uq_enrollments_active
// refusal of a second open enrollment.
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

// selfScope returns a scope for a teacher who owns their own center — the
// fake repository's convention (mirrored from handler_test.go's
// fakeScopeResolver) is that a self-owned teacher's center id equals their
// own id.
func selfScope(teacherID uuid.UUID) authctx.Scope {
	return authctx.Scope{TeacherID: teacherID, CenterID: teacherID, IsOwner: true}
}

// addClass inserts a fixture class self-owned by teacherID (their own
// center). addClassFor exists for tests that need a class owned by a
// specific teacher within a shared, independently-identified center.
func (f *fakeRepository) addClass(teacherID uuid.UUID, price int64) uuid.UUID {
	return f.addClassFor(selfScope(teacherID), price)
}

func (f *fakeRepository) addClassFor(sc authctx.Scope, price int64) uuid.UUID {
	classID := id.New()
	f.classes[classID] = fakeClass{centerID: sc.CenterID, teacherID: sc.TeacherID, name: "Toán 8", price: price}
	return classID
}

func (f *fakeRepository) addStudent(teacherID uuid.UUID) uuid.UUID {
	return f.addStudentFor(selfScope(teacherID))
}

func (f *fakeRepository) addStudentFor(sc authctx.Scope) uuid.UUID {
	studentID := id.New()
	f.students[studentID] = fakeStudent{centerID: sc.CenterID, teacherID: sc.TeacherID, name: "Bé An"}
	return studentID
}

func (f *fakeRepository) row(e *fakeEnrollment) Row {
	return Row{
		Enrollment:  e.Enrollment,
		StudentName: f.students[e.StudentID].name,
		ClassName:   f.classes[e.ClassID].name,
	}
}

// visibleEnrollment mirrors the real scoped() predicate: always the center,
// plus the teacher when the caller is not an owner.
func visibleEnrollment(e *fakeEnrollment, sc authctx.Scope) bool {
	if e.deleted || e.CenterID != sc.CenterID {
		return false
	}
	return sc.IsOwner || e.TeacherID == sc.TeacherID
}

// visibleClass is visibleEnrollment's counterpart for the class lookups
// ClassDefaultPrice performs.
func visibleClass(c fakeClass, sc authctx.Scope) bool {
	if c.centerID != sc.CenterID {
		return false
	}
	return sc.IsOwner || c.teacherID == sc.TeacherID
}

// visibleStudent mirrors the real StudentExists: center-only. Students anchor
// on the center owner, so a per-teacher filter would refuse every legitimate
// reference; the class check is what stays per-teacher.
func visibleStudent(s fakeStudent, sc authctx.Scope) bool {
	return s.centerID == sc.CenterID
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

func (f *fakeRepository) GetByID(_ context.Context, sc authctx.Scope, id uuid.UUID) (*Row, error) {
	e, ok := f.rows[id]
	if !ok || !visibleEnrollment(e, sc) {
		return nil, ErrNotFound
	}
	row := f.row(e)
	return &row, nil
}

// GetWritableByID collapses onto GetByID: the unit fakes carry no
// class_staff table; the capability gate is integration-tested.
func (f *fakeRepository) GetWritableByID(ctx context.Context, sc authctx.Scope, id uuid.UUID, _ []string) (*Row, error) {
	return f.GetByID(ctx, sc, id)
}

func (f *fakeRepository) List(_ context.Context, sc authctx.Scope, filter ListFilter, _ pagination.Params) ([]Row, int64, error) {
	var out []Row
	for _, e := range f.rows {
		if !visibleEnrollment(e, sc) {
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

func (f *fakeRepository) End(_ context.Context, sc authctx.Scope, _ []string, id uuid.UUID, endedOn time.Time) error {
	e, ok := f.rows[id]
	if !ok || !visibleEnrollment(e, sc) {
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

func (f *fakeRepository) SoftDelete(_ context.Context, sc authctx.Scope, _ []string, id uuid.UUID) error {
	e, ok := f.rows[id]
	if !ok || !visibleEnrollment(e, sc) {
		return ErrNotFound
	}
	e.deleted = true
	return nil
}

func (f *fakeRepository) ActiveOn(_ context.Context, sc authctx.Scope, classID uuid.UUID, on time.Time) ([]Enrollment, error) {
	var out []Enrollment
	for _, e := range f.rows {
		if !visibleEnrollment(e, sc) || e.ClassID != classID {
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
		if visibleEnrollment(e, sc) && e.StudentID == studentID && e.EndedOn == nil {
			ended := on
			e.EndedOn = &ended
		}
	}
	return nil
}

func (f *fakeRepository) ClassDefaultPrice(_ context.Context, sc authctx.Scope, classID uuid.UUID) (int64, error) {
	c, ok := f.classes[classID]
	if !ok || !visibleClass(c, sc) {
		return 0, ErrClassNotFound
	}
	return c.price, nil
}

func (f *fakeRepository) StudentExists(_ context.Context, sc authctx.Scope, studentID uuid.UUID) (bool, error) {
	s, ok := f.students[studentID]
	return ok && visibleStudent(s, sc), nil
}

func (f *fakeRepository) ClassInCenter(_ context.Context, sc authctx.Scope, classID uuid.UUID) (bool, error) {
	c, ok := f.classes[classID]
	return ok && c.centerID == sc.CenterID, nil
}

// CallerClassStanding models the invariant the fake lives under: the class's
// anchor teacher is its active giao_vien, and no other stints exist.
func (f *fakeRepository) CallerClassStanding(_ context.Context, sc authctx.Scope, classID uuid.UUID) (bool, bool, error) {
	c, ok := f.classes[classID]
	teaching := ok && c.centerID == sc.CenterID && c.teacherID == sc.TeacherID
	return teaching, teaching, nil
}

func (f *fakeRepository) SearchEnrollableStudents(_ context.Context, sc authctx.Scope, classID uuid.UUID, q string, limit int) ([]PickerStudent, error) {
	var rows []PickerStudent
	for studentID, s := range f.students {
		if s.centerID != sc.CenterID || !strings.Contains(strings.ToLower(s.name), strings.ToLower(q)) {
			continue
		}
		enrolled := false
		for _, e := range f.rows {
			if !e.deleted && e.StudentID == studentID && e.ClassID == classID && e.EndedOn == nil {
				enrolled = true
				break
			}
		}
		if !enrolled {
			rows = append(rows, PickerStudent{ID: studentID, FullName: s.name})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].FullName < rows[j].FullName })
	if len(rows) > limit {
		rows = rows[:limit]
	}
	return rows, nil
}

func newTestService() (*Service, *fakeRepository) {
	repo := newFakeRepository()
	return NewService(repo, nil), repo
}

func enroll(t *testing.T, svc *Service, sc authctx.Scope, studentID, classID uuid.UUID, startedOn string) *Row {
	t.Helper()
	row, err := svc.Create(context.Background(), sc, CreateRequest{
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
	sc := selfScope(teacherID)
	classID := repo.addClass(teacherID, 150_000)
	studentID := repo.addStudent(teacherID)

	row := enroll(t, svc, sc, studentID, classID, "2026-01-15")
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
	sc := selfScope(teacherID)
	classID := repo.addClass(teacherID, 150_000)
	studentID := repo.addStudent(teacherID)

	row := enroll(t, svc, sc, studentID, classID, "")
	if got, want := row.StartedOn.Format(dateLayout), today().Format(dateLayout); got != want {
		t.Fatalf("started_on must default to today (%s), got %s", want, got)
	}
}

func TestCreateRejectsForeignReferences(t *testing.T) {
	svc, repo := newTestService()
	teacherID := id.New()
	sc := selfScope(teacherID)
	stranger := id.New()
	ownClass := repo.addClass(teacherID, 150_000)
	ownStudent := repo.addStudent(teacherID)
	foreignClass := repo.addClass(stranger, 150_000)
	foreignStudent := repo.addStudent(stranger)

	_, err := svc.Create(context.Background(), sc, CreateRequest{
		StudentID: ownStudent, ClassID: foreignClass,
	})
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeValidation || appErr.Fields["class_id"] == "" {
		t.Fatalf("foreign class must be 422 naming class_id, got %v", err)
	}
	if !errors.Is(err, ErrClassNotFound) {
		t.Fatalf("want ErrClassNotFound cause, got %v", err)
	}

	_, err = svc.Create(context.Background(), sc, CreateRequest{
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
	sc := selfScope(teacherID)
	classID := repo.addClass(teacherID, 150_000)
	studentID := repo.addStudent(teacherID)

	enroll(t, svc, sc, studentID, classID, "2026-01-05")
	_, err := svc.Create(context.Background(), sc, CreateRequest{
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
	sc := selfScope(teacherID)
	classID := repo.addClass(teacherID, 150_000)
	studentID := repo.addStudent(teacherID)

	first := enroll(t, svc, sc, studentID, classID, "2026-01-05")
	ended, err := svc.End(context.Background(), sc, first.ID, EndRequest{EndedOn: "2026-03-31"})
	if err != nil {
		t.Fatalf("end: %v", err)
	}
	if ended.EndedOn == nil || ended.EndedOn.Format(dateLayout) != "2026-03-31" {
		t.Fatalf("ended_on must be stamped, got %v", ended.EndedOn)
	}

	// Ending twice: 409, and the date does not move.
	_, err = svc.End(context.Background(), sc, first.ID, EndRequest{EndedOn: "2026-04-15"})
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeConflict || !errors.Is(err, ErrAlreadyEnded) {
		t.Fatalf("double end must be 409 ErrAlreadyEnded, got %v", err)
	}
	unchanged, err := svc.Get(context.Background(), sc, first.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if unchanged.EndedOn.Format(dateLayout) != "2026-03-31" {
		t.Fatalf("a refused double end must not move the date, got %v", unchanged.EndedOn)
	}

	// Re-enrolling after leaving succeeds; the old row survives.
	second := enroll(t, svc, sc, studentID, classID, "2026-05-01")
	if second.ID == first.ID {
		t.Fatal("re-enrollment must be a new row")
	}
	if _, err := svc.Get(context.Background(), sc, first.ID); err != nil {
		t.Fatalf("the ended row must stay readable, got %v", err)
	}
}

func TestEndBeforeStartIsValidationError(t *testing.T) {
	svc, repo := newTestService()
	teacherID := id.New()
	sc := selfScope(teacherID)
	classID := repo.addClass(teacherID, 150_000)
	studentID := repo.addStudent(teacherID)

	row := enroll(t, svc, sc, studentID, classID, "2026-02-10")
	_, err := svc.End(context.Background(), sc, row.ID, EndRequest{EndedOn: "2026-02-09"})
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) || appErr.Code != apperror.CodeValidation || appErr.Fields["ended_on"] == "" {
		t.Fatalf("ended_on before started_on must be 422 naming ended_on, got %v", err)
	}

	// The boundary itself is allowed: ending on the start date is one paid
	// session, not an error.
	if _, err := svc.End(context.Background(), sc, row.ID, EndRequest{EndedOn: "2026-02-10"}); err != nil {
		t.Fatalf("ended_on == started_on must be allowed, got %v", err)
	}
}

func TestEndOpenEnrollmentsClosesOnlyThatStudent(t *testing.T) {
	svc, repo := newTestService()
	teacherID := id.New()
	sc := selfScope(teacherID)
	classID := repo.addClass(teacherID, 150_000)
	leaver := repo.addStudent(teacherID)
	stayer := repo.addStudent(teacherID)

	leaverRow := enroll(t, svc, sc, leaver, classID, "2026-01-05")
	stayerRow := enroll(t, svc, sc, stayer, classID, "2026-01-05")

	on := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if err := svc.EndOpenEnrollments(context.Background(), sc, leaver, on); err != nil {
		t.Fatalf("end open enrollments: %v", err)
	}
	if row, _ := svc.Get(context.Background(), sc, leaverRow.ID); row.EndedOn == nil {
		t.Fatal("the leaver's enrollment must be closed")
	}
	if row, _ := svc.Get(context.Background(), sc, stayerRow.ID); row.EndedOn != nil {
		t.Fatal("the other student's enrollment must stay open")
	}
}

func TestCrossTenantReadsAsNotFound(t *testing.T) {
	svc, repo := newTestService()
	owner := id.New()
	ownerScope := selfScope(owner)
	classID := repo.addClass(owner, 150_000)
	studentID := repo.addStudent(owner)
	row := enroll(t, svc, ownerScope, studentID, classID, "2026-01-05")

	stranger := selfScope(id.New())
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

// An owner reads and manages a member's existing enrollment — center
// oversight. Creating stamps the row as the caller's own and requires the
// caller's own class; the student may be any center student, since students
// anchor on the owner.
func TestOwnerScopeSeesAndManagesMembersEnrollment(t *testing.T) {
	svc, repo := newTestService()
	center := id.New()
	member := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: false}
	owner := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: true}

	classID := repo.addClassFor(member, 150_000)
	studentID := repo.addStudentFor(member)
	memberRow := enroll(t, svc, member, studentID, classID, "2026-01-05")

	if _, err := svc.Get(context.Background(), owner, memberRow.ID); err != nil {
		t.Fatalf("owner must read a member's enrollment, got %v", err)
	}
	if _, err := svc.End(context.Background(), owner, memberRow.ID, EndRequest{EndedOn: "2026-03-31"}); err != nil {
		t.Fatalf("owner must end a member's enrollment, got %v", err)
	}

	otherStudent := repo.addStudentFor(member)
	if _, err := svc.Create(context.Background(), owner, CreateRequest{
		StudentID: otherStudent, ClassID: classID, StartedOn: "2026-04-01",
	}); apperror.From(err).Code != apperror.CodeValidation {
		t.Fatalf("owner enrolling into a member's class must be a 422, got %v", err)
	}

	ownClass := repo.addClassFor(owner, 150_000)
	ownStudent := repo.addStudentFor(owner)
	created, err := svc.Create(context.Background(), owner, CreateRequest{
		StudentID: ownStudent, ClassID: ownClass, StartedOn: "2026-04-01",
	})
	if err != nil {
		t.Fatalf("owner must still enroll their own student into their own class, got %v", err)
	}
	if created.TeacherID != owner.TeacherID {
		t.Fatalf("an enrollment created by the owner must be stamped as the owner's own, got %s", created.TeacherID)
	}
	mixed, err := svc.Create(context.Background(), owner, CreateRequest{
		StudentID: otherStudent, ClassID: ownClass,
	})
	if err != nil {
		t.Fatalf("a center student must be enrollable into the owner's own class, got %v", err)
	}
	if mixed.TeacherID != owner.TeacherID {
		t.Fatalf("the enrollment must be stamped as the owner's own, got %s", mixed.TeacherID)
	}
}

// A peer in the same center but not the creator, and not the owner, must not
// see the enrollment — center scope alone is not enough, isolation still
// holds between non-owning members.
func TestPeerScopeCannotSeeAnotherMembersEnrollment(t *testing.T) {
	svc, repo := newTestService()
	center := id.New()
	author := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: false}
	peer := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: false}

	classID := repo.addClassFor(author, 150_000)
	studentID := repo.addStudentFor(author)
	row := enroll(t, svc, author, studentID, classID, "2026-01-05")

	if _, err := svc.Get(context.Background(), peer, row.ID); apperror.From(err).Code != apperror.CodeNotFound {
		t.Fatalf("peer must not read another member's enrollment, got %v", err)
	}
}

// FindByStudentAndClass mirrors the SQL predicate: scope-visible, not
// soft-deleted, and open OR ended — the ended case is the whole point, since
// uq_enrollments_active cannot see it.
func (f *fakeRepository) FindByStudentAndClass(_ context.Context, sc authctx.Scope, studentID, classID uuid.UUID) (*Enrollment, error) {
	var newest *fakeEnrollment
	for _, e := range f.rows {
		if !visibleEnrollment(e, sc) || e.StudentID != studentID || e.ClassID != classID {
			continue
		}
		if newest == nil || e.StartedOn.After(newest.StartedOn) {
			newest = e
		}
	}
	if newest == nil {
		return nil, ErrNotFound
	}
	out := newest.Enrollment
	return &out, nil
}

// FindByStudentAndClass is the bulk-import lookup. It deliberately returns
// ended rows too: uq_enrollments_active is partial on ended_on IS NULL, so a
// caller that only saw open rows would re-enrol a student who left, backdating
// started_on and invoicing every session since.

func TestFindByStudentAndClassReturnsAnEndedEnrollment(t *testing.T) {
	svc, repo := newTestService()
	teacherID := id.New()
	sc := selfScope(teacherID)
	classID := repo.addClass(teacherID, 150_000)
	studentID := repo.addStudent(teacherID)

	row := enroll(t, svc, sc, studentID, classID, "2026-01-05")
	if _, err := svc.End(context.Background(), sc, row.ID, EndRequest{EndedOn: "2026-03-31"}); err != nil {
		t.Fatalf("end: %v", err)
	}

	got, found, err := svc.FindByStudentAndClass(context.Background(), sc, studentID, classID)
	if err != nil || !found {
		t.Fatalf("an ended enrollment must still be found, got found=%v err=%v", found, err)
	}
	if got.EndedOn == nil {
		t.Fatal("the caller decides what to do about a departure, so ended_on must survive the lookup")
	}
}

func TestFindByStudentAndClassMissesWhenThereIsNone(t *testing.T) {
	svc, repo := newTestService()
	teacherID := id.New()
	sc := selfScope(teacherID)

	_, found, err := svc.FindByStudentAndClass(context.Background(), sc,
		repo.addStudent(teacherID), repo.addClass(teacherID, 150_000))
	if err != nil || found {
		t.Fatalf("an unenrolled pair must miss, got found=%v err=%v", found, err)
	}
}

func TestFindByStudentAndClassStaysWithinTheAnchorTeacher(t *testing.T) {
	svc, repo := newTestService()
	center := id.New()
	author := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: false}
	peer := authctx.Scope{TeacherID: id.New(), CenterID: center, IsOwner: false}

	classID := repo.addClassFor(author, 150_000)
	studentID := repo.addStudentFor(author)
	enroll(t, svc, author, studentID, classID, "2026-01-05")

	if _, found, err := svc.FindByStudentAndClass(context.Background(), peer, studentID, classID); err != nil || found {
		t.Fatalf("another teacher's enrollment must not be found, got found=%v err=%v", found, err)
	}
}

// Enrolling a student widens what the enrolling teacher can read and feeds the
// next billing close, so every successful create must leave an event for the
// audit trail — carrying the actor and the exact enrollment it produced.
func TestCreatePublishesStudentEnrolledEvent(t *testing.T) {
	repo := newFakeRepository()
	bus := events.NewSync()
	var got []events.Event
	bus.Subscribe("test", 0, func(_ context.Context, e events.Event) { got = append(got, e) })
	svc := NewService(repo, bus)

	teacherID := uuid.New()
	sc := selfScope(teacherID)
	classID := repo.addClass(teacherID, 100_000)
	studentID := repo.addStudent(teacherID)

	row, err := svc.Create(context.Background(), sc, CreateRequest{
		StudentID: studentID, ClassID: classID, StartedOn: "2026-01-05",
	})
	require.NoError(t, err)
	require.Len(t, got, 1)
	ev, ok := got[0].(StudentEnrolled)
	require.True(t, ok, "want a StudentEnrolled event, got %T", got[0])
	require.Equal(t, row.ID, ev.EnrollmentID)
	require.Equal(t, teacherID, ev.ActorID)
	require.Equal(t, sc.CenterID, ev.CenterID)
	require.Equal(t, classID, ev.ClassID)
	require.Equal(t, studentID, ev.StudentID)
	require.False(t, ev.OccurredAt.IsZero())

	// A refused create publishes nothing — the trail records what happened,
	// not what was attempted and rejected.
	_, err = svc.Create(context.Background(), sc, CreateRequest{
		StudentID: studentID, ClassID: classID,
	})
	require.Error(t, err)
	require.Len(t, got, 1)
}
