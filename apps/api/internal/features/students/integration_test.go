//go:build integration

package students_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/students"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/testutil"
)

// sqlEnder closes open enrollments with the same UPDATE the enrollments
// feature issues. Running through database.FromContext proves the closure
// joins the delete transaction.
type sqlEnder struct{ db *gorm.DB }

func (e sqlEnder) EndOpenEnrollments(ctx context.Context, sc authctx.Scope, studentID uuid.UUID, on time.Time) error {
	db := database.FromContext(ctx, e.db)
	if sc.IsOwner {
		return db.Exec(
			"UPDATE enrollments SET ended_on = ? WHERE center_id = ? AND student_id = ? AND ended_on IS NULL AND deleted_at IS NULL",
			on, sc.CenterID, studentID,
		).Error
	}
	return db.Exec(
		"UPDATE enrollments SET ended_on = ? WHERE center_id = ? AND teacher_id = ? AND student_id = ? AND ended_on IS NULL AND deleted_at IS NULL",
		on, sc.CenterID, sc.TeacherID, studentID,
	).Error
}

func newIntegrationService(t *testing.T) (*students.Service, *gorm.DB) {
	t.Helper()
	db := testutil.StartPostgres(t)
	svc := students.NewService(students.NewRepository(db), sqlEnder{db: db}, database.NewTxManager(db))
	return svc, db
}

// listParams builds pagination params the way a handler would.
func listParams(t *testing.T) pagination.Params {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	return pagination.Parse(c, "full_name", map[string]string{"full_name": "students.full_name"})
}

// insertEnrollment writes an enrollments row directly, stamped with the
// student's own center so the composite FKs hold.
func insertEnrollment(t *testing.T, db *gorm.DB, centerID, teacherID, studentID, classID uuid.UUID) uuid.UUID {
	t.Helper()
	eid := id.New()
	require.NoError(t, db.Exec(
		"INSERT INTO enrollments (id, teacher_id, center_id, student_id, class_id, unit_price, started_on) VALUES (?, ?, ?, ?, ?, 150000, '2026-01-05')",
		eid, teacherID, centerID, studentID, classID,
	).Error)
	return eid
}

func TestDeleteAnonymisesButPreservesFinancialRecords(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID)

	created, err := svc.Create(ctx, sc, students.CreateRequest{
		FullName: "Bé An", ContactID: contact.ID, DisplayNote: "An lớp 9A",
	})
	require.NoError(t, err)

	enrollmentID := insertEnrollment(t, db, sc.CenterID, teacher.ID, created.ID, class.ID)

	// A held session with a billable attendance record — the history that must
	// survive the delete because it backs money already reported to a parent.
	sessionID := id.New()
	require.NoError(t, db.Exec(
		"INSERT INTO class_sessions (id, teacher_id, center_id, class_id, session_date, status) VALUES (?, ?, ?, ?, '2026-01-06', 'held')",
		sessionID, teacher.ID, sc.CenterID, class.ID,
	).Error)
	attendanceID := id.New()
	require.NoError(t, db.Exec(
		"INSERT INTO attendance_records (id, teacher_id, center_id, session_id, student_id, enrollment_id, status) VALUES (?, ?, ?, ?, ?, ?, 'present')",
		attendanceID, teacher.ID, sc.CenterID, sessionID, created.ID, enrollmentID,
	).Error)

	// A closed-period invoice holding the name snapshot.
	periodID := id.New()
	require.NoError(t, db.Exec(
		"INSERT INTO billing_periods (id, teacher_id, center_id, year, month, period_start, period_end, status, closed_at) VALUES (?, ?, ?, 2026, 1, '2026-01-01', '2026-01-31', 'closed', now())",
		periodID, teacher.ID, sc.CenterID,
	).Error)
	invoiceID := id.New()
	require.NoError(t, db.Exec(
		"INSERT INTO invoices (id, teacher_id, center_id, period_id, student_id, contact_id, student_name, contact_name, current_charge, total_due) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 150000, 150000)",
		invoiceID, teacher.ID, sc.CenterID, periodID, created.ID, contact.ID, "Bé An", contact.FullName,
	).Error)

	require.NoError(t, svc.Delete(ctx, sc, created.ID))

	// The student row survives, scrubbed and stamped.
	var scrubbed struct {
		FullName     string
		DisplayNote  *string
		AnonymizedAt *time.Time
		DeletedAt    *time.Time
	}
	require.NoError(t, db.Raw(
		"SELECT full_name, display_note, anonymized_at, deleted_at FROM students WHERE id = ?", created.ID,
	).Scan(&scrubbed).Error)
	require.Equal(t, students.AnonymizedName, scrubbed.FullName,
		"the placeholder must be constant and non-identifying")
	require.Nil(t, scrubbed.DisplayNote, "display_note must be scrubbed")
	require.NotNil(t, scrubbed.AnonymizedAt)
	require.NotNil(t, scrubbed.DeletedAt)

	// The invoice keeps its snapshot of the original name.
	var invoiceName string
	require.NoError(t, db.Raw("SELECT student_name FROM invoices WHERE id = ?", invoiceID).Scan(&invoiceName).Error)
	require.Equal(t, "Bé An", invoiceName, "invoices must stay readable after erasure")

	// The attendance record is untouched — deleting it would change the
	// billable count behind that invoice.
	var attendance struct {
		Status    string
		DeletedAt *time.Time
	}
	require.NoError(t, db.Raw(
		"SELECT status, deleted_at FROM attendance_records WHERE id = ?", attendanceID,
	).Scan(&attendance).Error)
	require.Equal(t, "present", attendance.Status)
	require.Nil(t, attendance.DeletedAt)

	// The open enrollment was closed in the same transaction.
	var endedOn *time.Time
	require.NoError(t, db.Raw("SELECT ended_on FROM enrollments WHERE id = ?", enrollmentID).Scan(&endedOn).Error)
	require.NotNil(t, endedOn, "a deleted student must stop appearing on future attendance sheets")

	// A hard DELETE on the billed student is refused by the RESTRICT FK.
	err = db.Exec("DELETE FROM students WHERE id = ?", created.ID).Error
	require.Error(t, err, "hard delete of a billed student must be refused by the database")
}

func TestCreateRejectsForeignContact(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacherA, _ := testutil.Teacher(t, db)
	teacherB, _ := testutil.Teacher(t, db)
	scopeA := testutil.ScopeFor(t, db, teacherA.ID)
	foreignContact := testutil.Contact(t, db, teacherB.ID)

	_, err := svc.Create(ctx, scopeA, students.CreateRequest{
		FullName: "Bé An", ContactID: foreignContact.ID,
	})
	appErr := apperror.From(err)
	require.Equal(t, apperror.CodeValidation, appErr.Code, "a foreign contact must be 422, never 500")
	require.NotEmpty(t, appErr.Fields["contact_id"])
}

func TestCrossCenterReadsAreNotFound(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacherA, _ := testutil.Teacher(t, db)
	teacherB, _ := testutil.Teacher(t, db)
	scopeA := testutil.ScopeFor(t, db, teacherA.ID)
	scopeB := testutil.ScopeFor(t, db, teacherB.ID)
	contact := testutil.Contact(t, db, teacherA.ID)

	created, err := svc.Create(ctx, scopeA, students.CreateRequest{FullName: "Bé An", ContactID: contact.ID})
	require.NoError(t, err)

	// 404, never 403: a 403 would confirm the id exists in another center.
	_, err = svc.Get(ctx, scopeB, created.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	_, err = svc.Update(ctx, scopeB, created.ID, students.UpdateRequest{FullName: "X", ContactID: contact.ID})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	err = svc.Delete(ctx, scopeB, created.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	rows, total, err := svc.List(ctx, scopeB, students.ListFilter{}, listParams(t))
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, rows)
}

// An owner sees, updates, and deletes a student created by a teacher who
// joined their center — center-wide oversight, not per-teacher isolation.
// Creating is stricter: a student is always stamped as the caller's own, so
// the owner may only reference their own contacts; a member's contacts are
// view-only for creation and refused with the same 422 a stranger's would be.
func TestOwnerHasFullOversightOfMembersStudents(t *testing.T) {
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

	memberContact := testutil.Contact(t, db, member.ID)
	row, err := svc.Create(ctx, memberScope, students.CreateRequest{FullName: "Bé An", ContactID: memberContact.ID})
	require.NoError(t, err)

	got, err := svc.Get(ctx, ownerScope, row.ID)
	require.NoError(t, err, "owner must read a member's student")
	require.Equal(t, row.ID, got.ID)

	rows, total, err := svc.List(ctx, ownerScope, students.ListFilter{}, listParams(t))
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, row.ID, rows[0].ID, "owner's list must include the member's student")

	updated, err := svc.Update(ctx, ownerScope, row.ID, students.UpdateRequest{FullName: "Bé An (updated)", ContactID: memberContact.ID})
	require.NoError(t, err, "owner must update a member's student")
	require.Equal(t, "Bé An (updated)", updated.FullName)

	require.NoError(t, svc.Delete(ctx, ownerScope, row.ID), "owner must delete a member's student")
	_, err = svc.Get(ctx, ownerScope, row.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	// Creating against a member's contact is refused: the row would carry the
	// owner's anchor while the contact stays the member's.
	_, err = svc.Create(ctx, ownerScope, students.CreateRequest{FullName: "Bé Bình", ContactID: memberContact.ID})
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code,
		"owner must not create a student under a member's contact")

	// An owner still creates rows against their own contacts, as themselves.
	ownerContact := testutil.Contact(t, db, owner.ID)
	ownerRow, err := svc.Create(ctx, ownerScope, students.CreateRequest{FullName: "Bé Bình", ContactID: ownerContact.ID})
	require.NoError(t, err)
	require.Equal(t, owner.ID, ownerRow.TeacherID, "owner-created student must be stamped as the owner's own")
}

// Two non-owning teachers in the same center are still isolated from each
// other: center scope grants the owner oversight, not peer-to-peer access.
func TestPeersInSameCenterCannotSeeEachOthersStudents(t *testing.T) {
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
	row, err := svc.Create(ctx, scopeB, students.CreateRequest{FullName: "Bé An", ContactID: contactB.ID})
	require.NoError(t, err)

	_, err = svc.Get(ctx, scopeC, row.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code, "a peer must not read another member's student")

	rows, total, err := svc.List(ctx, scopeC, students.ListFilter{}, listParams(t))
	require.NoError(t, err)
	require.EqualValues(t, 0, total)
	for _, r := range rows {
		require.NotEqual(t, row.ID, r.ID, "a peer's list must not include another member's student")
	}
}

func TestListByClassFiltersThroughOpenEnrollmentsBounded(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID)

	enrolled, err := svc.Create(ctx, sc, students.CreateRequest{FullName: "Bé An", ContactID: contact.ID})
	require.NoError(t, err)
	departed, err := svc.Create(ctx, sc, students.CreateRequest{FullName: "Bé Bình", ContactID: contact.ID})
	require.NoError(t, err)
	_, err = svc.Create(ctx, sc, students.CreateRequest{FullName: "Bé Cường", ContactID: contact.ID})
	require.NoError(t, err)

	insertEnrollment(t, db, sc.CenterID, teacher.ID, enrolled.ID, class.ID)
	endedID := insertEnrollment(t, db, sc.CenterID, teacher.ID, departed.ID, class.ID)
	require.NoError(t, db.Exec("UPDATE enrollments SET ended_on = '2026-02-01' WHERE id = ?", endedID).Error)

	counter := &sqlCounter{Interface: gormlogger.Discard}
	counted := students.NewService(
		students.NewRepository(db.Session(&gorm.Session{Logger: counter})),
		sqlEnder{db: db}, database.NewTxManager(db))

	rows, total, err := counted.List(ctx, sc, students.ListFilter{ClassID: class.ID}, listParams(t))
	require.NoError(t, err)
	require.EqualValues(t, 1, total, "only the open enrollment counts")
	require.Len(t, rows, 1)
	require.Equal(t, enrolled.ID, rows[0].ID)
	require.Equal(t, contact.FullName, rows[0].ContactName, "rows carry the contact join")
	require.LessOrEqual(t, counter.n.Load(), int64(2),
		"class filtering must stay at count+select — join, do not loop")
}

func TestListUnenrolledExcludesOpenEnrollments(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID)

	enrolled, err := svc.Create(ctx, sc, students.CreateRequest{FullName: "Bé An", ContactID: contact.ID})
	require.NoError(t, err)
	departed, err := svc.Create(ctx, sc, students.CreateRequest{FullName: "Bé Bình", ContactID: contact.ID})
	require.NoError(t, err)
	never, err := svc.Create(ctx, sc, students.CreateRequest{FullName: "Bé Cường", ContactID: contact.ID})
	require.NoError(t, err)

	insertEnrollment(t, db, sc.CenterID, teacher.ID, enrolled.ID, class.ID)
	endedID := insertEnrollment(t, db, sc.CenterID, teacher.ID, departed.ID, class.ID)
	require.NoError(t, db.Exec("UPDATE enrollments SET ended_on = '2026-02-01' WHERE id = ?", endedID).Error)

	rows, total, err := svc.List(ctx, sc, students.ListFilter{Unenrolled: true}, listParams(t))
	require.NoError(t, err)
	require.EqualValues(t, 2, total, "an ended enrollment leaves the student unenrolled again")
	ids := []uuid.UUID{rows[0].ID, rows[1].ID}
	require.ElementsMatch(t, []uuid.UUID{departed.ID, never.ID}, ids)
}

// sqlCounter counts executed statements through the GORM logger's Trace hook.
type sqlCounter struct {
	gormlogger.Interface
	n atomic.Int64
}

func (c *sqlCounter) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	c.n.Add(1)
	c.Interface.Trace(ctx, begin, fc, err)
}

// A class handoff moves the class to a new teacher but leaves the student
// rows with their creator. The new teacher must still read those students —
// the roster tab lists them and their detail pages resolve — while editing
// and deleting stay with the creator or the owner, and an unassigned peer
// keeps seeing nothing.
func TestHandedOffClassStudentsAreReadableByNewTeacher(t *testing.T) {
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
	student, err := svc.Create(ctx, ownerScope, students.CreateRequest{FullName: "Bé An", ContactID: contact.ID})
	require.NoError(t, err)
	insertEnrollment(t, db, ownerCenter, owner.ID, student.ID, class.ID)
	require.NoError(t, db.Exec(
		"UPDATE classes SET teacher_id = ? WHERE id = ?", newTeacher.ID, class.ID).Error,
		"simulate the handoff move of classes.teacher_id")

	rows, total, err := svc.List(ctx, newTeacherScope, students.ListFilter{ClassID: class.ID}, listParams(t))
	require.NoError(t, err)
	require.EqualValues(t, 1, total, "the new teacher must see the handed-off class's students")
	require.Equal(t, student.ID, rows[0].ID)
	require.Equal(t, contact.FullName, rows[0].ContactName, "rows carry the contact join")

	_, total, err = svc.List(ctx, newTeacherScope, students.ListFilter{}, listParams(t))
	require.NoError(t, err)
	require.EqualValues(t, 1, total, "the unfiltered list must include the handed-off student")

	got, err := svc.Get(ctx, newTeacherScope, student.ID)
	require.NoError(t, err, "the new teacher must open the student's detail page")
	require.Equal(t, student.ID, got.ID)

	// Reads widened, writes not: the student still belongs to the owner.
	_, err = svc.Update(ctx, newTeacherScope, student.ID,
		students.UpdateRequest{FullName: "Bé An (edited)", ContactID: contact.ID})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"the new teacher must not edit a student they do not own")
	err = svc.Delete(ctx, newTeacherScope, student.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"the new teacher must not delete a student they do not own")

	// The widening is keyed on class assignment, not center membership: an
	// unassigned peer still sees nothing.
	_, total, err = svc.List(ctx, peerScope, students.ListFilter{ClassID: class.ID}, listParams(t))
	require.NoError(t, err)
	require.EqualValues(t, 0, total, "an unassigned peer must not see the students")
	_, err = svc.Get(ctx, peerScope, student.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	// The owner keeps full oversight after the handoff.
	_, total, err = svc.List(ctx, ownerScope, students.ListFilter{ClassID: class.ID}, listParams(t))
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
}
