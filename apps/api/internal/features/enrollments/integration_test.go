//go:build integration

package enrollments_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/students"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/testutil"
)

func newIntegrationService(t *testing.T) (*enrollments.Service, *gorm.DB) {
	t.Helper()
	db := testutil.StartPostgres(t)
	return enrollments.NewService(enrollments.NewRepository(db), nil), db
}

// listParams builds pagination params the way a handler would.
func listParams(t *testing.T) pagination.Params {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	return pagination.Parse(c, "started_on", map[string]string{"started_on": "enrollments.started_on"})
}

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestActiveOnBoundariesAreInclusive(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID)

	// Three students around the queried date D = 2026-03-14: one starting
	// exactly on D, one whose last day is D, and one starting the day after.
	startsOnD := testutil.Student(t, db, teacher.ID, contact.ID, testutil.WithStudentFullName("Starts On D"))
	endsOnD := testutil.Student(t, db, teacher.ID, contact.ID, testutil.WithStudentFullName("Ends On D"))
	startsAfter := testutil.Student(t, db, teacher.ID, contact.ID, testutil.WithStudentFullName("Starts After"))

	_, err := svc.Create(ctx, sc, enrollments.CreateRequest{
		StudentID: startsOnD.ID, ClassID: class.ID, StartedOn: "2026-03-14",
	})
	require.NoError(t, err)
	ending, err := svc.Create(ctx, sc, enrollments.CreateRequest{
		StudentID: endsOnD.ID, ClassID: class.ID, StartedOn: "2026-01-05",
	})
	require.NoError(t, err)
	_, err = svc.End(ctx, sc, ending.ID, enrollments.EndRequest{EndedOn: "2026-03-14"})
	require.NoError(t, err)
	_, err = svc.Create(ctx, sc, enrollments.CreateRequest{
		StudentID: startsAfter.ID, ClassID: class.ID, StartedOn: "2026-03-15",
	})
	require.NoError(t, err)

	active, err := svc.ActiveOn(ctx, sc, class.ID, date("2026-03-14"))
	require.NoError(t, err)
	require.Len(t, active, 2,
		"a student starting on D attends that session and a student ending on D attends their last one")
	got := map[string]bool{}
	for _, e := range active {
		got[e.StudentID.String()] = true
	}
	require.True(t, got[startsOnD.ID.String()], "started_on == D is inclusive")
	require.True(t, got[endsOnD.ID.String()], "ended_on == D is inclusive")
	require.False(t, got[startsAfter.ID.String()], "started_on == D+1 is excluded")
}

func TestDuplicateOpenEnrollmentRefusedByIndexThenReenrollAllowed(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID)
	student := testutil.Student(t, db, teacher.ID, contact.ID)

	first, err := svc.Create(ctx, sc, enrollments.CreateRequest{
		StudentID: student.ID, ClassID: class.ID, StartedOn: "2026-01-05",
	})
	require.NoError(t, err)

	// uq_enrollments_active — not a pre-check — refuses the duplicate.
	_, err = svc.Create(ctx, sc, enrollments.CreateRequest{
		StudentID: student.ID, ClassID: class.ID, StartedOn: "2026-02-01",
	})
	require.True(t, errors.Is(err, enrollments.ErrAlreadyEnrolled), "want 409 cause, got %v", err)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)

	_, err = svc.End(ctx, sc, first.ID, enrollments.EndRequest{EndedOn: "2026-03-31"})
	require.NoError(t, err)

	second, err := svc.Create(ctx, sc, enrollments.CreateRequest{
		StudentID: student.ID, ClassID: class.ID, StartedOn: "2026-05-01",
	})
	require.NoError(t, err, "re-enrolling after leaving must succeed")

	// The closed row survives with its history.
	old, err := svc.Get(ctx, sc, first.ID)
	require.NoError(t, err)
	require.NotNil(t, old.EndedOn)
	require.Equal(t, "2026-03-31", old.EndedOn.Format("2006-01-02"))
	require.NotEqual(t, first.ID, second.ID)
}

func TestRepoEndOnAlreadyEndedIsConflictNotNotFound(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID)
	student := testutil.Student(t, db, teacher.ID, contact.ID)

	row, err := svc.Create(ctx, sc, enrollments.CreateRequest{
		StudentID: student.ID, ClassID: class.ID, StartedOn: "2026-01-05",
	})
	require.NoError(t, err)

	_, err = svc.End(ctx, sc, row.ID, enrollments.EndRequest{EndedOn: "2026-03-31"})
	require.NoError(t, err)

	// Call the repository directly to exercise the concurrent-loser path the
	// service's pre-check would otherwise hide: the row exists but is closed.
	repo := enrollments.NewRepository(db)
	roles := authctx.StaffRolesFor(authctx.CapEnrollmentWrite)
	err = repo.End(ctx, sc, roles, row.ID, date("2026-04-30"))
	require.ErrorIs(t, err, enrollments.ErrAlreadyEnded,
		"ending an already-closed row is a 409, not a 404")

	// A genuinely absent id still reports not-found.
	err = repo.End(ctx, sc, roles, testutil.Student(t, db, teacher.ID, contact.ID).ID, date("2026-04-30"))
	require.ErrorIs(t, err, enrollments.ErrNotFound)
}

func TestRaisingClassPriceLeavesEnrollmentsUntouched(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassUnitPrice(150_000))
	student := testutil.Student(t, db, teacher.ID, contact.ID)

	row, err := svc.Create(ctx, sc, enrollments.CreateRequest{
		StudentID: student.ID, ClassID: class.ID, StartedOn: "2026-01-05",
	})
	require.NoError(t, err)
	require.EqualValues(t, 150_000, row.UnitPrice)

	require.NoError(t, db.Exec(
		"UPDATE classes SET default_unit_price = 999999 WHERE id = ?", class.ID).Error)

	after, err := svc.Get(ctx, sc, row.ID)
	require.NoError(t, err)
	require.EqualValues(t, 150_000, after.UnitPrice,
		"a price change must never rewrite what an enrolled student owes")

	// New enrollments do pick up the new default.
	other := testutil.Student(t, db, teacher.ID, contact.ID)
	fresh, err := svc.Create(ctx, sc, enrollments.CreateRequest{
		StudentID: other.ID, ClassID: class.ID, StartedOn: "2026-06-01",
	})
	require.NoError(t, err)
	require.EqualValues(t, 999_999, fresh.UnitPrice)
}

func TestDeletingStudentEndsOpenEnrollments(t *testing.T) {
	t.Parallel()
	enrollSvc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID)
	student := testutil.Student(t, db, teacher.ID, contact.ID)

	row, err := enrollSvc.Create(ctx, sc, enrollments.CreateRequest{
		StudentID: student.ID, ClassID: class.ID, StartedOn: "2026-01-05",
	})
	require.NoError(t, err)

	// The exact wiring router.go uses: the students service consumes the
	// enrollments service as its EnrollmentEnder.
	studentsSvc := students.NewService(students.NewRepository(db), enrollSvc, database.NewTxManager(db))
	require.NoError(t, studentsSvc.Delete(ctx, sc, student.ID))

	var endedOn *time.Time
	require.NoError(t, db.Raw("SELECT ended_on FROM enrollments WHERE id = ?", row.ID).Scan(&endedOn).Error)
	require.NotNil(t, endedOn, "deleting a student must close their open enrollments")
}

// A teacher from a different center is refused on every operation with 404,
// never 403 — a 403 would confirm the id exists in another center. Creating
// against a foreign student or class is a 422 naming the reference field, and
// the stranger's list stays empty.
func TestCrossCenterReadsAreNotFound(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacherA, _ := testutil.Teacher(t, db)
	teacherB, _ := testutil.Teacher(t, db)
	scopeA := testutil.ScopeFor(t, db, teacherA.ID)
	scopeB := testutil.ScopeFor(t, db, teacherB.ID)
	contact := testutil.Contact(t, db, teacherA.ID)
	class := testutil.Class(t, db, teacherA.ID)
	student := testutil.Student(t, db, teacherA.ID, contact.ID)

	row, err := svc.Create(ctx, scopeA, enrollments.CreateRequest{
		StudentID: student.ID, ClassID: class.ID, StartedOn: "2026-01-05",
	})
	require.NoError(t, err)

	_, err = svc.Get(ctx, scopeB, row.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	_, err = svc.End(ctx, scopeB, row.ID, enrollments.EndRequest{})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	err = svc.Delete(ctx, scopeB, row.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	// Teacher B cannot enroll their student into A's class either.
	contactB := testutil.Contact(t, db, teacherB.ID)
	studentB := testutil.Student(t, db, teacherB.ID, contactB.ID)
	_, err = svc.Create(ctx, scopeB, enrollments.CreateRequest{
		StudentID: studentB.ID, ClassID: class.ID,
	})
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code)

	// Nor A's student into a class of B's.
	classB := testutil.Class(t, db, teacherB.ID)
	_, err = svc.Create(ctx, scopeB, enrollments.CreateRequest{
		StudentID: student.ID, ClassID: classB.ID,
	})
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code)

	rows, total, err := svc.List(ctx, scopeB, enrollments.ListFilter{}, listParams(t))
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, rows)

	// The owner still sees everything intact.
	got, err := svc.Get(ctx, scopeA, row.ID)
	require.NoError(t, err)
	require.Equal(t, row.ID, got.ID)
}

// An owner reads, ends, and deletes a member's enrollments — center-wide
// oversight over existing rows. Creating is stricter: an enrollment is always
// stamped as the caller's own, so a row created against a member's student or
// class would carry the owner's anchor while living in the member's roster —
// invisible to the member's own attendance and billing. The owner therefore
// enrolls only into their own classes; a member's rows are view-only for
// creation and get the same 422 a stranger's would.
func TestOwnerHasFullOversightOfMembersEnrollments(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, db)
	member, _ := testutil.Teacher(t, db)
	ownerCenter := testutil.ScopeFor(t, db, owner.ID).CenterID

	testutil.JoinCenter(t, db, member.ID, ownerCenter)
	ownerScope := testutil.ScopeFor(t, db, owner.ID)
	memberScope := testutil.ScopeFor(t, db, member.ID)
	require.Equal(t, ownerScope.CenterID, memberScope.CenterID, "member must have joined the owner's center")

	contact := testutil.Contact(t, db, member.ID)
	class := testutil.Class(t, db, member.ID)
	student := testutil.Student(t, db, member.ID, contact.ID)
	memberRow := testutil.Enrollment(t, db, member.ID, student.ID, class.ID, date("2026-01-05"))

	got, err := svc.Get(ctx, ownerScope, memberRow.ID)
	require.NoError(t, err, "owner must read a member's enrollment")
	require.Equal(t, memberRow.ID, got.ID)

	rows, total, err := svc.List(ctx, ownerScope, enrollments.ListFilter{}, listParams(t))
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, memberRow.ID, rows[0].ID, "owner's list must include the member's enrollment")

	ended, err := svc.End(ctx, ownerScope, memberRow.ID, enrollments.EndRequest{EndedOn: "2026-03-31"})
	require.NoError(t, err, "owner must end a member's enrollment")
	require.NotNil(t, ended.EndedOn)

	active, err := svc.ActiveOn(ctx, ownerScope, class.ID, date("2026-01-10"))
	require.NoError(t, err)
	require.Len(t, active, 1, "owner's ActiveOn must include the member's class roster")

	// Creating against a member's student or class is refused with the same
	// 422 a foreign reference gets — never silently stamped as the owner's.
	otherStudent := testutil.Student(t, db, member.ID, contact.ID)
	_, err = svc.Create(ctx, ownerScope, enrollments.CreateRequest{
		StudentID: otherStudent.ID, ClassID: class.ID, StartedOn: "2026-04-01",
	})
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code,
		"owner must not enroll into a member's class")

	// The owner still enrolls into their own classes like any teacher.
	ownContact := testutil.Contact(t, db, owner.ID)
	ownClass := testutil.Class(t, db, owner.ID)
	ownStudent := testutil.Student(t, db, owner.ID, ownContact.ID)
	created, err := svc.Create(ctx, ownerScope, enrollments.CreateRequest{
		StudentID: ownStudent.ID, ClassID: ownClass.ID, StartedOn: "2026-04-01",
	})
	require.NoError(t, err, "owner must still enroll their own student into their own class")
	require.Equal(t, owner.ID, created.TeacherID)

	// Students are center data anchored on the owner, so a member-anchored
	// student (a pre-migration legacy row) is a legitimate reference in the
	// owner's own class — the class is what stays per-teacher, not the child.
	mixed, err := svc.Create(ctx, ownerScope, enrollments.CreateRequest{
		StudentID: otherStudent.ID, ClassID: ownClass.ID,
	})
	require.NoError(t, err, "owner must enroll any center student into their own class")
	require.Equal(t, owner.ID, mixed.TeacherID)

	require.NoError(t, svc.Delete(ctx, ownerScope, memberRow.ID), "owner must delete a member's enrollment")
	_, err = svc.Get(ctx, ownerScope, memberRow.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
}

// Two non-owning teachers in the same center are still isolated from each
// other: center scope grants the owner oversight, not peer-to-peer access.
func TestPeersInSameCenterCannotSeeEachOthersEnrollments(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, db)
	memberB, _ := testutil.Teacher(t, db)
	memberC, _ := testutil.Teacher(t, db)
	ownerCenter := testutil.ScopeFor(t, db, owner.ID).CenterID

	testutil.JoinCenter(t, db, memberB.ID, ownerCenter)
	testutil.JoinCenter(t, db, memberC.ID, ownerCenter)
	scopeB := testutil.ScopeFor(t, db, memberB.ID)
	scopeC := testutil.ScopeFor(t, db, memberC.ID)

	contactB := testutil.Contact(t, db, memberB.ID)
	classB := testutil.Class(t, db, memberB.ID)
	studentB := testutil.Student(t, db, memberB.ID, contactB.ID)

	created, err := svc.Create(ctx, scopeB, enrollments.CreateRequest{
		StudentID: studentB.ID, ClassID: classB.ID, StartedOn: "2026-01-05",
	})
	require.NoError(t, err)

	_, err = svc.Get(ctx, scopeC, created.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code, "a peer must not read another member's enrollment")
	_, err = svc.End(ctx, scopeC, created.ID, enrollments.EndRequest{})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code, "a peer must not end another member's enrollment")

	rows, total, err := svc.List(ctx, scopeC, enrollments.ListFilter{}, listParams(t))
	require.NoError(t, err)
	require.EqualValues(t, 0, total)
	for _, r := range rows {
		require.NotEqual(t, created.ID, r.ID, "a peer's list must not include another member's enrollment")
	}

	// A peer cannot enroll their own student into another member's class...
	contactC := testutil.Contact(t, db, memberC.ID)
	studentC := testutil.Student(t, db, memberC.ID, contactC.ID)
	_, err = svc.Create(ctx, scopeC, enrollments.CreateRequest{
		StudentID: studentC.ID, ClassID: classB.ID,
	})
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code, "a peer must not reference another member's class")

	// ...but a student is center data, not the enrolling teacher's: a
	// member-anchored student (pre-migration legacy row) enrolls into the
	// peer's own class fine, stamped as the peer's enrollment.
	classC := testutil.Class(t, db, memberC.ID)
	crossed, err := svc.Create(ctx, scopeC, enrollments.CreateRequest{
		StudentID: studentB.ID, ClassID: classC.ID,
	})
	require.NoError(t, err, "a center student must be enrollable into the caller's own class")
	require.Equal(t, memberC.ID, crossed.TeacherID)
}

// A class handoff moves the class to a new teacher but leaves the enrollment
// rows with their creator. The new teacher must still read AND manage the
// roster of the class now assigned to them — the enrollment write capability
// follows the active giao_vien stint, not the rows' creator — while an
// unassigned peer keeps seeing nothing.
func TestHandedOffClassRosterIsReadableByNewTeacher(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, db)
	newTeacher, _ := testutil.Teacher(t, db)
	peer, _ := testutil.Teacher(t, db)
	ownerCenter := testutil.ScopeFor(t, db, owner.ID).CenterID

	testutil.JoinCenter(t, db, newTeacher.ID, ownerCenter)
	testutil.JoinCenter(t, db, peer.ID, ownerCenter)
	ownerScope := testutil.ScopeFor(t, db, owner.ID)
	newTeacherScope := testutil.ScopeFor(t, db, newTeacher.ID)
	peerScope := testutil.ScopeFor(t, db, peer.ID)

	// The owner built the class and its roster, then handed the class over.
	contact := testutil.Contact(t, db, owner.ID)
	class := testutil.Class(t, db, owner.ID)
	student := testutil.Student(t, db, owner.ID, contact.ID)
	row := testutil.Enrollment(t, db, owner.ID, student.ID, class.ID, date("2026-01-05"))
	// Simulate the handoff's dual write: classes.teacher_id moves AND the
	// giao_vien stint follows (the previous one closes, the new teacher opens
	// theirs) — the invariant assignment-scoped reads key on.
	require.NoError(t, db.Exec(
		"UPDATE classes SET teacher_id = ? WHERE id = ?", newTeacher.ID, class.ID).Error)
	require.NoError(t, db.Exec(
		"UPDATE class_staff SET ended_at = now() WHERE class_id = ? AND role_key = 'giao_vien' AND ended_at IS NULL",
		class.ID).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO class_staff (class_id, center_id, teacher_id, role_key) VALUES (?, ?, ?, 'giao_vien')",
		class.ID, ownerCenter, newTeacher.ID).Error)

	rows, total, err := svc.List(ctx, newTeacherScope, enrollments.ListFilter{ClassID: class.ID}, listParams(t))
	require.NoError(t, err)
	require.EqualValues(t, 1, total, "the new teacher must see the handed-off class's roster")
	require.Equal(t, row.ID, rows[0].ID)
	require.NotEmpty(t, rows[0].StudentName, "roster rows must resolve the student's name")

	got, err := svc.Get(ctx, newTeacherScope, row.ID)
	require.NoError(t, err, "the new teacher must read a roster enrollment")
	require.Equal(t, row.ID, got.ID)

	active, err := svc.ActiveOn(ctx, newTeacherScope, class.ID, date("2026-01-10"))
	require.NoError(t, err)
	require.Len(t, active, 1, "the attendance roster must include the owner-created enrollment")

	// Writes follow the stint: the new teacher manages the roster they now
	// teach, even though the row's creator anchor still names the owner.
	ended, err := svc.End(ctx, newTeacherScope, row.ID, enrollments.EndRequest{EndedOn: "2026-02-28"})
	require.NoError(t, err, "the new teacher ends an enrollment on their handed-off class")
	require.NotNil(t, ended.EndedOn)

	// The widening is keyed on class assignment, not center membership: an
	// unassigned peer still sees nothing.
	_, total, err = svc.List(ctx, peerScope, enrollments.ListFilter{ClassID: class.ID}, listParams(t))
	require.NoError(t, err)
	require.EqualValues(t, 0, total, "an unassigned peer must not see the roster")
	_, err = svc.Get(ctx, peerScope, row.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	// The owner keeps full oversight after the handoff.
	_, total, err = svc.List(ctx, ownerScope, enrollments.ListFilter{ClassID: class.ID}, listParams(t))
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
}

// The enrollable-student picker lets the class's active giao_vien — or the
// owner — find center students by name so an existing student can be enrolled
// without creating a duplicate. Names only: the picker carries no phone and no
// contact, and it answers nothing to queries shorter than two runes.
func TestEnrollableStudentPicker(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, db)
	gv, _ := testutil.Teacher(t, db)
	troGiang, _ := testutil.Teacher(t, db)
	peer, _ := testutil.Teacher(t, db)
	ownerScope := testutil.ScopeFor(t, db, owner.ID)
	center := ownerScope.CenterID
	testutil.JoinCenter(t, db, gv.ID, center)
	testutil.JoinCenter(t, db, troGiang.ID, center)
	testutil.JoinCenter(t, db, peer.ID, center)
	gvScope := testutil.ScopeFor(t, db, gv.ID)
	troGiangScope := testutil.ScopeFor(t, db, troGiang.ID)
	peerScope := testutil.ScopeFor(t, db, peer.ID)

	class := testutil.Class(t, db, gv.ID)
	testutil.StaffAssignment(t, db, class, troGiang.ID, "tro_giang")

	// Students are center data anchored on the owner.
	contact := testutil.Contact(t, db, owner.ID)
	anKhang := testutil.Student(t, db, owner.ID, contact.ID, testutil.WithStudentFullName("An Khang"))
	testutil.Student(t, db, owner.ID, contact.ID, testutil.WithStudentFullName("Binh An"))
	testutil.Student(t, db, owner.ID, contact.ID, testutil.WithStudentFullName("Chau Ngoc"))
	gone := testutil.Student(t, db, owner.ID, contact.ID, testutil.WithStudentFullName("An Da Xoa"))
	require.NoError(t, db.Exec(
		"UPDATE students SET deleted_at = now() WHERE id = ?", gone.ID).Error)

	rows, err := svc.EnrollableStudents(ctx, gvScope, class.ID, "an", 20)
	require.NoError(t, err)
	require.Len(t, rows, 2, "name matches, minus the deleted student")
	require.Equal(t, "An Khang", rows[0].FullName)
	require.Equal(t, "Binh An", rows[1].FullName)

	// A student already actively enrolled in THIS class disappears from its
	// picker (picking them could only 409); ending the enrollment brings them
	// back, and other classes are unaffected throughout.
	active := testutil.Enrollment(t, db, gv.ID, anKhang.ID, class.ID, date("2026-01-10"))
	rows, err = svc.EnrollableStudents(ctx, gvScope, class.ID, "an", 20)
	require.NoError(t, err)
	require.Len(t, rows, 1, "an actively enrolled student is not enrollable again")
	require.Equal(t, "Binh An", rows[0].FullName)

	otherClass := testutil.Class(t, db, gv.ID)
	rows, err = svc.EnrollableStudents(ctx, gvScope, otherClass.ID, "an", 20)
	require.NoError(t, err)
	require.Len(t, rows, 2, "enrollment elsewhere must not hide the student from another class's picker")

	require.NoError(t, db.Exec(
		"UPDATE enrollments SET ended_on = '2026-02-01' WHERE id = ?", active.ID).Error)
	rows, err = svc.EnrollableStudents(ctx, gvScope, class.ID, "an", 20)
	require.NoError(t, err)
	require.Len(t, rows, 2, "a student whose enrollment ended is enrollable again")

	rows, err = svc.EnrollableStudents(ctx, gvScope, class.ID, "a", 20)
	require.NoError(t, err)
	require.Empty(t, rows, "a query under two runes answers nothing")

	rows, err = svc.EnrollableStudents(ctx, ownerScope, class.ID, "chau", 20)
	require.NoError(t, err)
	require.Len(t, rows, 1, "the owner may use any class's picker")

	_, err = svc.EnrollableStudents(ctx, troGiangScope, class.ID, "an", 20)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code,
		"a tro_giang sees the class but must not pick students to enroll")
	_, err = svc.EnrollableStudents(ctx, peerScope, class.ID, "an", 20)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"an unassigned member must not even learn the class exists")

	// The 20-row cap holds even when the caller asks for more.
	for i := 0; i < 21; i++ {
		testutil.Student(t, db, owner.ID, contact.ID,
			testutil.WithStudentFullName(fmt.Sprintf("Hoc Sinh %02d", i)))
	}
	rows, err = svc.EnrollableStudents(ctx, gvScope, class.ID, "hoc sinh", 50)
	require.NoError(t, err)
	require.Len(t, rows, 20)
}
