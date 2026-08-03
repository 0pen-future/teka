//go:build integration

package contacts_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"teka/apps/api/internal/features/contacts"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/testutil"
)

func newIntegrationService(t *testing.T) (*contacts.Service, *gorm.DB) {
	t.Helper()
	db := testutil.StartPostgres(t)
	return contacts.NewService(contacts.NewRepository(db)), db
}

// listParams builds pagination params the way a handler would, from a raw
// query string.
func listParams(t *testing.T, query string) pagination.Params {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/?"+query, nil)
	return pagination.Parse(c, "full_name", map[string]string{"full_name": "contacts.full_name"})
}

// insertStudent writes a students row directly; the students feature package
// arrives in phase 2, but the count join must be proven against real rows now.
func insertStudent(t *testing.T, db *gorm.DB, teacherID, contactID any, name string) (studentID any) {
	t.Helper()
	sid := id.New()
	require.NoError(t, db.Exec(
		"INSERT INTO students (id, teacher_id, contact_id, full_name) VALUES (?, ?, ?, ?)",
		sid, teacherID, contactID, name,
	).Error)
	return sid
}

func TestPhoneReusableAcrossTeachersButUniqueWithinOne(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacherA, _ := testutil.Teacher(t, db)
	teacherB, _ := testutil.Teacher(t, db)

	_, err := svc.Create(ctx, teacherA.ID, contacts.CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	require.NoError(t, err)

	// Same number under a second teacher is a separate tenant's customer.
	_, err = svc.Create(ctx, teacherB.ID, contacts.CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	require.NoError(t, err, "phone uniqueness must be per-teacher, not global")

	// The E.164 spelling of the number already registered under teacher A must
	// collide with the local-form original — one number, one contact.
	_, err = svc.Create(ctx, teacherA.ID, contacts.CreateRequest{FullName: "Chị Hoa (bis)", Phone: "+84912345678"})
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code,
		"0912345678 and +84912345678 must be the same number")
}

func TestSoftDeleteFreesPhoneForReuse(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)

	first, err := svc.Create(ctx, teacher.ID, contacts.CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	require.NoError(t, err)
	require.NoError(t, svc.Delete(ctx, teacher.ID, first.ID))

	// The partial unique index only covers non-deleted rows, so the number is
	// free again.
	again, err := svc.Create(ctx, teacher.ID, contacts.CreateRequest{FullName: "Chị Hoa mới", Phone: "0912345678"})
	require.NoError(t, err, "soft delete must free the phone for reuse")
	require.NotEqual(t, first.ID, again.ID)
}

func TestCrossTenantReadsAreNotFound(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacherA, _ := testutil.Teacher(t, db)
	teacherB, _ := testutil.Teacher(t, db)

	row, err := svc.Create(ctx, teacherA.ID, contacts.CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	require.NoError(t, err)

	// 404, never 403: a 403 would confirm the id exists in another tenant.
	_, err = svc.Get(ctx, teacherB.ID, row.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	err = svc.Delete(ctx, teacherB.ID, row.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	_, err = svc.Update(ctx, teacherB.ID, row.ID, contacts.UpdateRequest{FullName: "X", Phone: "0912345679"})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
}

func TestUpdateRenormalisesPhoneAndDetectsCollision(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)

	kept, err := svc.Create(ctx, teacher.ID, contacts.CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	require.NoError(t, err)
	moved, err := svc.Create(ctx, teacher.ID, contacts.CreateRequest{FullName: "Anh Tuấn", Phone: "0912345679"})
	require.NoError(t, err)

	updated, err := svc.Update(ctx, teacher.ID, moved.ID, contacts.UpdateRequest{FullName: "Anh Tuấn", Phone: "0399999999"})
	require.NoError(t, err)
	require.Equal(t, "+84399999999", updated.Phone)

	_, err = svc.Update(ctx, teacher.ID, moved.ID, contacts.UpdateRequest{FullName: "Anh Tuấn", Phone: kept.Phone})
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)
}

func TestDeleteBlockedByLiveStudentsNamesThem(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)

	row, err := svc.Create(ctx, teacher.ID, contacts.CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	require.NoError(t, err)
	studentID := insertStudent(t, db, teacher.ID, row.ID, "Bé An")

	err = svc.Delete(ctx, teacher.ID, row.ID)
	appErr := apperror.From(err)
	require.Equal(t, apperror.CodeConflict, appErr.Code)
	require.Contains(t, appErr.Message, "Bé An", "the 409 must name the blocking student")

	// Soft-deleting the student clears the block.
	require.NoError(t, db.Exec("UPDATE students SET deleted_at = now() WHERE id = ?", studentID).Error)
	require.NoError(t, svc.Delete(ctx, teacher.ID, row.ID))
}

func TestListCountsStudentsWithoutNPlusOne(t *testing.T) {
	t.Parallel()
	_, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)

	// 150 contacts — the PRD's roster scale. The one carrying students gets a
	// name that sorts first so the default full_name ordering keeps it inside
	// the asserted page.
	first := testutil.Contact(t, db, teacher.ID, testutil.WithContactFullName("An An"))
	for i := 0; i < 149; i++ {
		testutil.Contact(t, db, teacher.ID)
	}
	insertStudent(t, db, teacher.ID, first.ID, "Bé An")
	insertStudent(t, db, teacher.ID, first.ID, "Bé Bình")
	gone := insertStudent(t, db, teacher.ID, first.ID, "Bé Cũ")
	require.NoError(t, db.Exec("UPDATE students SET deleted_at = now() WHERE id = ?", gone).Error)

	counter := &sqlCounter{Interface: gormlogger.Discard}
	counted := contacts.NewService(contacts.NewRepository(db.Session(&gorm.Session{Logger: counter})))

	rows, total, err := counted.List(ctx, teacher.ID, contacts.ListFilter{}, listParams(t, "per_page=100"))
	require.NoError(t, err)
	require.EqualValues(t, 150, total)
	require.Len(t, rows, 100)
	require.LessOrEqual(t, counter.n.Load(), int64(2),
		"listing must stay at count+select regardless of row count — no per-row student query")

	byID := map[string]int64{}
	for _, r := range rows {
		byID[r.ID.String()] = r.StudentCount
	}
	require.EqualValues(t, 2, byID[first.ID.String()],
		"student_count must count live students only")
}

func TestListSearchMatchesNameAndPhone(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)

	_, err := svc.Create(ctx, teacher.ID, contacts.CreateRequest{FullName: "Chị Hoa", Phone: "0912345678"})
	require.NoError(t, err)
	_, err = svc.Create(ctx, teacher.ID, contacts.CreateRequest{FullName: "Anh Tuấn", Phone: "0987654321"})
	require.NoError(t, err)

	rows, total, err := svc.List(ctx, teacher.ID, contacts.ListFilter{Query: "hoa"}, listParams(t, ""))
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, "Chị Hoa", rows[0].FullName)

	rows, total, err = svc.List(ctx, teacher.ID, contacts.ListFilter{Query: "8765"}, listParams(t, ""))
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, "Anh Tuấn", rows[0].FullName)
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
