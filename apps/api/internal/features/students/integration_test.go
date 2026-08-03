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
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/testutil"
)

// sqlEnder closes open enrollments with the same UPDATE the enrollments
// feature will issue (phase 4). Running through database.FromContext proves
// the closure joins the delete transaction.
type sqlEnder struct{ db *gorm.DB }

func (e sqlEnder) EndOpenEnrollments(ctx context.Context, teacherID, studentID uuid.UUID, on time.Time) error {
	return database.FromContext(ctx, e.db).Exec(
		"UPDATE enrollments SET ended_on = ? WHERE teacher_id = ? AND student_id = ? AND ended_on IS NULL AND deleted_at IS NULL",
		on, teacherID, studentID,
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

func insertEnrollment(t *testing.T, db *gorm.DB, teacherID, studentID, classID uuid.UUID) uuid.UUID {
	t.Helper()
	eid := id.New()
	require.NoError(t, db.Exec(
		"INSERT INTO enrollments (id, teacher_id, student_id, class_id, unit_price, started_on) VALUES (?, ?, ?, ?, 150000, '2026-01-05')",
		eid, teacherID, studentID, classID,
	).Error)
	return eid
}

func TestDeleteAnonymisesButPreservesFinancialRecords(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID)

	created, err := svc.Create(ctx, teacher.ID, students.CreateRequest{
		FullName: "Bé An", ContactID: contact.ID, DisplayNote: "An lớp 9A",
	})
	require.NoError(t, err)

	enrollmentID := insertEnrollment(t, db, teacher.ID, created.ID, class.ID)

	// A held session with a billable attendance record — the history that must
	// survive the delete because it backs money already reported to a parent.
	sessionID := id.New()
	require.NoError(t, db.Exec(
		"INSERT INTO class_sessions (id, teacher_id, class_id, session_date, status) VALUES (?, ?, ?, '2026-01-06', 'held')",
		sessionID, teacher.ID, class.ID,
	).Error)
	attendanceID := id.New()
	require.NoError(t, db.Exec(
		"INSERT INTO attendance_records (id, teacher_id, session_id, student_id, enrollment_id, status) VALUES (?, ?, ?, ?, ?, 'present')",
		attendanceID, teacher.ID, sessionID, created.ID, enrollmentID,
	).Error)

	// A closed-period invoice holding the name snapshot.
	periodID := id.New()
	require.NoError(t, db.Exec(
		"INSERT INTO billing_periods (id, teacher_id, year, month, period_start, period_end, status, closed_at) VALUES (?, ?, 2026, 1, '2026-01-01', '2026-01-31', 'closed', now())",
		periodID, teacher.ID,
	).Error)
	invoiceID := id.New()
	require.NoError(t, db.Exec(
		"INSERT INTO invoices (id, teacher_id, period_id, student_id, contact_id, student_name, contact_name, current_charge, total_due) VALUES (?, ?, ?, ?, ?, ?, ?, 150000, 150000)",
		invoiceID, teacher.ID, periodID, created.ID, contact.ID, "Bé An", contact.FullName,
	).Error)

	require.NoError(t, svc.Delete(ctx, teacher.ID, created.ID))

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
	foreignContact := testutil.Contact(t, db, teacherB.ID)

	_, err := svc.Create(ctx, teacherA.ID, students.CreateRequest{
		FullName: "Bé An", ContactID: foreignContact.ID,
	})
	appErr := apperror.From(err)
	require.Equal(t, apperror.CodeValidation, appErr.Code, "a foreign contact must be 422, never 500")
	require.NotEmpty(t, appErr.Fields["contact_id"])
}

func TestCrossTenantReadsAreNotFound(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacherA, _ := testutil.Teacher(t, db)
	teacherB, _ := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacherA.ID)

	created, err := svc.Create(ctx, teacherA.ID, students.CreateRequest{FullName: "Bé An", ContactID: contact.ID})
	require.NoError(t, err)

	_, err = svc.Get(ctx, teacherB.ID, created.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	rows, total, err := svc.List(ctx, teacherB.ID, students.ListFilter{}, listParams(t))
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, rows)
}

func TestListByClassFiltersThroughOpenEnrollmentsBounded(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID)

	enrolled, err := svc.Create(ctx, teacher.ID, students.CreateRequest{FullName: "Bé An", ContactID: contact.ID})
	require.NoError(t, err)
	departed, err := svc.Create(ctx, teacher.ID, students.CreateRequest{FullName: "Bé Bình", ContactID: contact.ID})
	require.NoError(t, err)
	_, err = svc.Create(ctx, teacher.ID, students.CreateRequest{FullName: "Bé Cường", ContactID: contact.ID})
	require.NoError(t, err)

	insertEnrollment(t, db, teacher.ID, enrolled.ID, class.ID)
	endedID := insertEnrollment(t, db, teacher.ID, departed.ID, class.ID)
	require.NoError(t, db.Exec("UPDATE enrollments SET ended_on = '2026-02-01' WHERE id = ?", endedID).Error)

	counter := &sqlCounter{Interface: gormlogger.Discard}
	counted := students.NewService(
		students.NewRepository(db.Session(&gorm.Session{Logger: counter})),
		sqlEnder{db: db}, database.NewTxManager(db))

	rows, total, err := counted.List(ctx, teacher.ID, students.ListFilter{ClassID: class.ID}, listParams(t))
	require.NoError(t, err)
	require.EqualValues(t, 1, total, "only the open enrollment counts")
	require.Len(t, rows, 1)
	require.Equal(t, enrolled.ID, rows[0].ID)
	require.Equal(t, contact.FullName, rows[0].ContactName, "rows carry the contact join")
	require.LessOrEqual(t, counter.n.Load(), int64(2),
		"class filtering must stay at count+select — join, do not loop")
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
