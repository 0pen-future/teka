//go:build integration

package enrollments_test

import (
	"context"
	"errors"
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
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/testutil"
)

func newIntegrationService(t *testing.T) (*enrollments.Service, *gorm.DB) {
	t.Helper()
	db := testutil.StartPostgres(t)
	return enrollments.NewService(enrollments.NewRepository(db)), db
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
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID)

	// Three students around the queried date D = 2026-03-14: one starting
	// exactly on D, one whose last day is D, and one starting the day after.
	startsOnD := testutil.Student(t, db, teacher.ID, contact.ID, testutil.WithStudentFullName("Starts On D"))
	endsOnD := testutil.Student(t, db, teacher.ID, contact.ID, testutil.WithStudentFullName("Ends On D"))
	startsAfter := testutil.Student(t, db, teacher.ID, contact.ID, testutil.WithStudentFullName("Starts After"))

	_, err := svc.Create(ctx, teacher.ID, enrollments.CreateRequest{
		StudentID: startsOnD.ID, ClassID: class.ID, StartedOn: "2026-03-14",
	})
	require.NoError(t, err)
	ending, err := svc.Create(ctx, teacher.ID, enrollments.CreateRequest{
		StudentID: endsOnD.ID, ClassID: class.ID, StartedOn: "2026-01-05",
	})
	require.NoError(t, err)
	_, err = svc.End(ctx, teacher.ID, ending.ID, enrollments.EndRequest{EndedOn: "2026-03-14"})
	require.NoError(t, err)
	_, err = svc.Create(ctx, teacher.ID, enrollments.CreateRequest{
		StudentID: startsAfter.ID, ClassID: class.ID, StartedOn: "2026-03-15",
	})
	require.NoError(t, err)

	active, err := svc.ActiveOn(ctx, teacher.ID, class.ID, date("2026-03-14"))
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
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID)
	student := testutil.Student(t, db, teacher.ID, contact.ID)

	first, err := svc.Create(ctx, teacher.ID, enrollments.CreateRequest{
		StudentID: student.ID, ClassID: class.ID, StartedOn: "2026-01-05",
	})
	require.NoError(t, err)

	// uq_enrollments_active — not a pre-check — refuses the duplicate.
	_, err = svc.Create(ctx, teacher.ID, enrollments.CreateRequest{
		StudentID: student.ID, ClassID: class.ID, StartedOn: "2026-02-01",
	})
	require.True(t, errors.Is(err, enrollments.ErrAlreadyEnrolled), "want 409 cause, got %v", err)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)

	_, err = svc.End(ctx, teacher.ID, first.ID, enrollments.EndRequest{EndedOn: "2026-03-31"})
	require.NoError(t, err)

	second, err := svc.Create(ctx, teacher.ID, enrollments.CreateRequest{
		StudentID: student.ID, ClassID: class.ID, StartedOn: "2026-05-01",
	})
	require.NoError(t, err, "re-enrolling after leaving must succeed")

	// The closed row survives with its history.
	old, err := svc.Get(ctx, teacher.ID, first.ID)
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
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID)
	student := testutil.Student(t, db, teacher.ID, contact.ID)

	row, err := svc.Create(ctx, teacher.ID, enrollments.CreateRequest{
		StudentID: student.ID, ClassID: class.ID, StartedOn: "2026-01-05",
	})
	require.NoError(t, err)

	_, err = svc.End(ctx, teacher.ID, row.ID, enrollments.EndRequest{EndedOn: "2026-03-31"})
	require.NoError(t, err)

	// Call the repository directly to exercise the concurrent-loser path the
	// service's pre-check would otherwise hide: the row exists but is closed.
	repo := enrollments.NewRepository(db)
	err = repo.End(ctx, teacher.ID, row.ID, date("2026-04-30"))
	require.ErrorIs(t, err, enrollments.ErrAlreadyEnded,
		"ending an already-closed row is a 409, not a 404")

	// A genuinely absent id still reports not-found.
	err = repo.End(ctx, teacher.ID, testutil.Student(t, db, teacher.ID, contact.ID).ID, date("2026-04-30"))
	require.ErrorIs(t, err, enrollments.ErrNotFound)
}

func TestRaisingClassPriceLeavesEnrollmentsUntouched(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassUnitPrice(150_000))
	student := testutil.Student(t, db, teacher.ID, contact.ID)

	row, err := svc.Create(ctx, teacher.ID, enrollments.CreateRequest{
		StudentID: student.ID, ClassID: class.ID, StartedOn: "2026-01-05",
	})
	require.NoError(t, err)
	require.EqualValues(t, 150_000, row.UnitPrice)

	require.NoError(t, db.Exec(
		"UPDATE classes SET default_unit_price = 999999 WHERE id = ?", class.ID).Error)

	after, err := svc.Get(ctx, teacher.ID, row.ID)
	require.NoError(t, err)
	require.EqualValues(t, 150_000, after.UnitPrice,
		"a price change must never rewrite what an enrolled student owes")

	// New enrollments do pick up the new default.
	other := testutil.Student(t, db, teacher.ID, contact.ID)
	fresh, err := svc.Create(ctx, teacher.ID, enrollments.CreateRequest{
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
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID)
	student := testutil.Student(t, db, teacher.ID, contact.ID)

	row, err := enrollSvc.Create(ctx, teacher.ID, enrollments.CreateRequest{
		StudentID: student.ID, ClassID: class.ID, StartedOn: "2026-01-05",
	})
	require.NoError(t, err)

	// The exact wiring router.go uses: the students service consumes the
	// enrollments service as its EnrollmentEnder.
	studentsSvc := students.NewService(students.NewRepository(db), enrollSvc, database.NewTxManager(db))
	require.NoError(t, studentsSvc.Delete(ctx, testutil.ScopeFor(t, db, teacher.ID), student.ID))

	var endedOn *time.Time
	require.NoError(t, db.Raw("SELECT ended_on FROM enrollments WHERE id = ?", row.ID).Scan(&endedOn).Error)
	require.NotNil(t, endedOn, "deleting a student must close their open enrollments")
}

func TestCrossTenantReadsAreNotFound(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacherA, _ := testutil.Teacher(t, db)
	teacherB, _ := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacherA.ID)
	class := testutil.Class(t, db, teacherA.ID)
	student := testutil.Student(t, db, teacherA.ID, contact.ID)

	row, err := svc.Create(ctx, teacherA.ID, enrollments.CreateRequest{
		StudentID: student.ID, ClassID: class.ID, StartedOn: "2026-01-05",
	})
	require.NoError(t, err)

	_, err = svc.Get(ctx, teacherB.ID, row.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	// Teacher B cannot enroll their student into A's class either.
	contactB := testutil.Contact(t, db, teacherB.ID)
	studentB := testutil.Student(t, db, teacherB.ID, contactB.ID)
	_, err = svc.Create(ctx, teacherB.ID, enrollments.CreateRequest{
		StudentID: studentB.ID, ClassID: class.ID,
	})
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code)

	rows, total, err := svc.List(ctx, teacherB.ID, enrollments.ListFilter{}, listParams(t))
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, rows)
}
