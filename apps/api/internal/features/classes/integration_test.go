//go:build integration

package classes_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/testutil"
)

func newIntegrationService(t *testing.T) (*classes.Service, *gorm.DB) {
	t.Helper()
	db := testutil.StartPostgres(t)
	return classes.NewService(classes.NewRepository(db), database.NewTxManager(db)), db
}

func int16Ptr(v int16) *int16 { return &v }
func int64Ptr(v int64) *int64 { return &v }

// listParams builds pagination params the way a handler would.
func listParams(t *testing.T) pagination.Params {
	t.Helper()
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	return pagination.Parse(c, "name", map[string]string{"name": "classes.name"})
}

func date(s string) time.Time {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return d
}

func createRequest() classes.CreateClassRequest {
	return classes.CreateClassRequest{
		Name:             "Toán 8",
		StartDate:        "2026-01-05",
		DefaultUnitPrice: int64Ptr(150_000),
		Schedules: []classes.ScheduleRequest{
			{Weekday: int16Ptr(2), StartTime: "18:00", DurationMin: 90},
		},
	}
}

func TestCreateIsAtomic(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)

	// duration_min = 0 violates the CHECK constraint on class_schedules; the
	// failing schedule insert must roll the class insert back with it. The
	// invalid value has to bypass binding, which is exactly what a service-level
	// atomicity test wants.
	req := createRequest()
	req.Schedules = append(req.Schedules, classes.ScheduleRequest{
		Weekday: int16Ptr(4), StartTime: "18:00", DurationMin: 0,
	})
	_, err := svc.Create(ctx, teacher.ID, req)
	require.Error(t, err, "a schedule violating a CHECK constraint must fail the create")

	var classCount int64
	require.NoError(t, db.Raw("SELECT count(*) FROM classes WHERE teacher_id = ?", teacher.ID).Scan(&classCount).Error)
	require.Zero(t, classCount, "the class insert must have rolled back")
	var scheduleCount int64
	require.NoError(t, db.Raw("SELECT count(*) FROM class_schedules WHERE teacher_id = ?", teacher.ID).Scan(&scheduleCount).Error)
	require.Zero(t, scheduleCount, "no schedule row may survive the rollback")
}

func TestPriceAndWeekdayRoundTripExactly(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)

	req := createRequest()
	req.Schedules[0].Weekday = int16Ptr(0) // Chủ nhật
	created, err := svc.Create(ctx, teacher.ID, req)
	require.NoError(t, err)

	got, err := svc.Get(ctx, teacher.ID, created.ID)
	require.NoError(t, err)
	require.EqualValues(t, 150_000, got.DefaultUnitPrice, "BIGINT đồng must round-trip exactly")
	require.Len(t, got.Schedules, 1)
	require.EqualValues(t, int(time.Sunday), got.Schedules[0].Weekday,
		"weekday 0 must mean Sunday, matching time.Weekday")
	require.Equal(t, "18:00", string(got.Schedules[0].StartTime), "TIME must round-trip as HH:MM")
	require.Equal(t, "2026-01-05", got.Schedules[0].EffectiveFrom.Format("2006-01-02"),
		"effective_from must default to the class start date")
}

func TestListEffectiveSchedulesWindowOverlap(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-05")))

	// Closed row: effective through March only.
	closed := testutil.Schedule(t, db, class, 2, "18:00")
	end := date("2026-03-31")
	require.NoError(t, db.Model(&classes.Schedule{}).Where("id = ?", closed.ID).
		Update("effective_to", end).Error)
	// Open-ended replacement starting in April.
	open := testutil.Schedule(t, db, class, 4, "19:00")
	require.NoError(t, db.Model(&classes.Schedule{}).Where("id = ?", open.ID).
		Update("effective_from", date("2026-04-01")).Error)

	// A March window sees only the closed row.
	rows, err := svc.ListEffectiveSchedules(ctx, teacher.ID, class.ID, date("2026-03-01"), date("2026-03-31"))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, closed.ID, rows[0].ID)

	// An April window sees only the open-ended row.
	rows, err = svc.ListEffectiveSchedules(ctx, teacher.ID, class.ID, date("2026-04-01"), date("2026-04-30"))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, open.ID, rows[0].ID)

	// A window spanning the changeover sees both.
	rows, err = svc.ListEffectiveSchedules(ctx, teacher.ID, class.ID, date("2026-03-15"), date("2026-04-15"))
	require.NoError(t, err)
	require.Len(t, rows, 2)

	// Boundary inclusivity: a window ending exactly on effective_from and one
	// starting exactly on effective_to both match.
	rows, err = svc.ListEffectiveSchedules(ctx, teacher.ID, class.ID, date("2026-04-01"), date("2026-04-01"))
	require.NoError(t, err)
	require.Len(t, rows, 1, "window touching effective_from must match")
	rows, err = svc.ListEffectiveSchedules(ctx, teacher.ID, class.ID, date("2026-03-31"), date("2026-03-31"))
	require.NoError(t, err)
	require.Len(t, rows, 1, "window touching effective_to must match")

	// A distant-future window before any row applies sees nothing... and a
	// pre-opening window sees nothing either.
	rows, err = svc.ListEffectiveSchedules(ctx, teacher.ID, class.ID, date("2025-12-01"), date("2025-12-31"))
	require.NoError(t, err)
	require.Empty(t, rows, "window before every effective_from must be empty")
}

func TestCloseAndReplaceKeepsOldRowQueryable(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)

	created, err := svc.Create(ctx, teacher.ID, createRequest())
	require.NoError(t, err)
	oldRow := created.Schedules[0]

	// Close the Tuesday row at the end of March…
	_, err = svc.UpdateSchedule(ctx, teacher.ID, created.ID, oldRow.ID, classes.UpdateScheduleRequest{
		Weekday:       int16Ptr(2),
		StartTime:     "18:00",
		DurationMin:   90,
		EffectiveFrom: "2026-01-05",
		EffectiveTo:   "2026-03-31",
	})
	require.NoError(t, err)
	// …and add the Thursday replacement from April.
	_, err = svc.AddSchedule(ctx, teacher.ID, created.ID, classes.ScheduleRequest{
		Weekday: int16Ptr(4), StartTime: "18:00", DurationMin: 90, EffectiveFrom: "2026-04-01",
	})
	require.NoError(t, err)

	// The old row still explains March sessions.
	rows, err := svc.ListEffectiveSchedules(ctx, teacher.ID, created.ID, date("2026-03-01"), date("2026-03-31"))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, oldRow.ID, rows[0].ID)

	got, err := svc.Get(ctx, teacher.ID, created.ID)
	require.NoError(t, err)
	require.Len(t, got.Schedules, 2, "both timetable rows stay on the class")
}

func TestArchivedExcludedFromDefaultListButRetrievable(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)

	created, err := svc.Create(ctx, teacher.ID, createRequest())
	require.NoError(t, err)
	_, err = svc.Archive(ctx, teacher.ID, created.ID)
	require.NoError(t, err)

	rows, total, err := svc.List(ctx, teacher.ID, classes.ListFilter{Status: classes.StatusActive}, listParams(t))
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, rows, "archived class must not appear in the default active list")

	got, err := svc.Get(ctx, teacher.ID, created.ID)
	require.NoError(t, err)
	require.Equal(t, classes.StatusArchived, got.Status, "archived class stays retrievable by id")
}

func TestDeleteBlockedByOpenEnrollmentThenAllowed(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)

	created, err := svc.Create(ctx, teacher.ID, createRequest())
	require.NoError(t, err)

	// The students and enrollments features arrive in later phases, so the
	// blocking rows are inserted directly.
	studentID := id.New()
	require.NoError(t, db.Exec(
		"INSERT INTO students (id, teacher_id, contact_id, full_name) VALUES (?, ?, ?, ?)",
		studentID, teacher.ID, contact.ID, "Bé An",
	).Error)
	enrollmentID := id.New()
	require.NoError(t, db.Exec(
		"INSERT INTO enrollments (id, teacher_id, student_id, class_id, unit_price, started_on) VALUES (?, ?, ?, ?, 150000, ?)",
		enrollmentID, teacher.ID, studentID, created.ID, date("2026-01-05"),
	).Error)

	err = svc.Delete(ctx, teacher.ID, created.ID)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)
	require.Contains(t, apperror.From(err).Message, "archive", "the 409 must point at archiving")

	// Ending the enrollment clears the block.
	require.NoError(t, db.Exec(
		"UPDATE enrollments SET ended_on = ? WHERE id = ?", date("2026-02-01"), enrollmentID,
	).Error)
	require.NoError(t, svc.Delete(ctx, teacher.ID, created.ID))
}

func TestCrossTenantReadsAreNotFound(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacherA, _ := testutil.Teacher(t, db)
	teacherB, _ := testutil.Teacher(t, db)

	created, err := svc.Create(ctx, teacherA.ID, createRequest())
	require.NoError(t, err)

	_, err = svc.Get(ctx, teacherB.ID, created.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	_, err = svc.Update(ctx, teacherB.ID, created.ID, classes.UpdateClassRequest{
		Name: "Chiếm lớp", StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(1),
	})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	err = svc.Delete(ctx, teacherB.ID, created.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	_, err = svc.Archive(ctx, teacherB.ID, created.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	scheduleID := created.Schedules[0].ID
	err = svc.DeleteSchedule(ctx, teacherB.ID, created.ID, scheduleID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	// The owner still sees everything intact.
	got, err := svc.Get(ctx, teacherA.ID, created.ID)
	require.NoError(t, err)
	require.Equal(t, "Toán 8", got.Name)
	require.Len(t, got.Schedules, 1)
}
