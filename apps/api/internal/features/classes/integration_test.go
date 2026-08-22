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
	sc := testutil.ScopeFor(t, db, teacher.ID)

	// duration_min = 0 violates the CHECK constraint on class_schedules; the
	// failing schedule insert must roll the class insert back with it. The
	// invalid value has to bypass binding, which is exactly what a service-level
	// atomicity test wants.
	req := createRequest()
	req.Schedules = append(req.Schedules, classes.ScheduleRequest{
		Weekday: int16Ptr(4), StartTime: "18:00", DurationMin: 0,
	})
	_, err := svc.Create(ctx, sc, req)
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
	sc := testutil.ScopeFor(t, db, teacher.ID)

	req := createRequest()
	req.Schedules[0].Weekday = int16Ptr(0) // Chủ nhật
	created, err := svc.Create(ctx, sc, req)
	require.NoError(t, err)

	got, err := svc.Get(ctx, sc, created.ID)
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
	sc := testutil.ScopeFor(t, db, teacher.ID)
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
	rows, err := svc.ListEffectiveSchedules(ctx, sc, class.ID, date("2026-03-01"), date("2026-03-31"))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, closed.ID, rows[0].ID)

	// An April window sees only the open-ended row.
	rows, err = svc.ListEffectiveSchedules(ctx, sc, class.ID, date("2026-04-01"), date("2026-04-30"))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, open.ID, rows[0].ID)

	// A window spanning the changeover sees both.
	rows, err = svc.ListEffectiveSchedules(ctx, sc, class.ID, date("2026-03-15"), date("2026-04-15"))
	require.NoError(t, err)
	require.Len(t, rows, 2)

	// Boundary inclusivity: a window ending exactly on effective_from and one
	// starting exactly on effective_to both match.
	rows, err = svc.ListEffectiveSchedules(ctx, sc, class.ID, date("2026-04-01"), date("2026-04-01"))
	require.NoError(t, err)
	require.Len(t, rows, 1, "window touching effective_from must match")
	rows, err = svc.ListEffectiveSchedules(ctx, sc, class.ID, date("2026-03-31"), date("2026-03-31"))
	require.NoError(t, err)
	require.Len(t, rows, 1, "window touching effective_to must match")

	// A distant-future window before any row applies sees nothing... and a
	// pre-opening window sees nothing either.
	rows, err = svc.ListEffectiveSchedules(ctx, sc, class.ID, date("2025-12-01"), date("2025-12-31"))
	require.NoError(t, err)
	require.Empty(t, rows, "window before every effective_from must be empty")
}

func TestCloseAndReplaceKeepsOldRowQueryable(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)

	created, err := svc.Create(ctx, sc, createRequest())
	require.NoError(t, err)
	oldRow := created.Schedules[0]

	// Close the Tuesday row at the end of March…
	_, err = svc.UpdateSchedule(ctx, sc, created.ID, oldRow.ID, classes.UpdateScheduleRequest{
		Weekday:       int16Ptr(2),
		StartTime:     "18:00",
		DurationMin:   90,
		EffectiveFrom: "2026-01-05",
		EffectiveTo:   "2026-03-31",
	})
	require.NoError(t, err)
	// …and add the Thursday replacement from April.
	_, err = svc.AddSchedule(ctx, sc, created.ID, classes.ScheduleRequest{
		Weekday: int16Ptr(4), StartTime: "18:00", DurationMin: 90, EffectiveFrom: "2026-04-01",
	})
	require.NoError(t, err)

	// The old row still explains March sessions.
	rows, err := svc.ListEffectiveSchedules(ctx, sc, created.ID, date("2026-03-01"), date("2026-03-31"))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, oldRow.ID, rows[0].ID)

	got, err := svc.Get(ctx, sc, created.ID)
	require.NoError(t, err)
	require.Len(t, got.Schedules, 2, "both timetable rows stay on the class")
}

func TestArchivedExcludedFromDefaultListButRetrievable(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)

	created, err := svc.Create(ctx, sc, createRequest())
	require.NoError(t, err)
	_, err = svc.Archive(ctx, sc, created.ID)
	require.NoError(t, err)

	rows, total, err := svc.List(ctx, sc, classes.ListFilter{Status: classes.StatusActive}, listParams(t))
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, rows, "archived class must not appear in the default active list")

	got, err := svc.Get(ctx, sc, created.ID)
	require.NoError(t, err)
	require.Equal(t, classes.StatusArchived, got.Status, "archived class stays retrievable by id")
}

func TestDeleteBlockedByOpenEnrollmentThenAllowed(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	contact := testutil.Contact(t, db, teacher.ID)

	created, err := svc.Create(ctx, sc, createRequest())
	require.NoError(t, err)

	// The enrollments feature arrives in a later phase, so the blocking rows
	// are inserted directly.
	studentID := id.New()
	require.NoError(t, db.Exec(
		"INSERT INTO students (id, teacher_id, center_id, contact_id, full_name) VALUES (?, ?, ?, ?, ?)",
		studentID, teacher.ID, sc.CenterID, contact.ID, "Bé An",
	).Error)
	enrollmentID := id.New()
	require.NoError(t, db.Exec(
		"INSERT INTO enrollments (id, teacher_id, center_id, student_id, class_id, unit_price, started_on) VALUES (?, ?, ?, ?, ?, 150000, ?)",
		enrollmentID, teacher.ID, sc.CenterID, studentID, created.ID, date("2026-01-05"),
	).Error)

	err = svc.Delete(ctx, sc, created.ID)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)
	require.Contains(t, apperror.From(err).Message, "archive", "the 409 must point at archiving")

	// Ending the enrollment clears the block.
	require.NoError(t, db.Exec(
		"UPDATE enrollments SET ended_on = ? WHERE id = ?", date("2026-02-01"), enrollmentID,
	).Error)
	require.NoError(t, svc.Delete(ctx, sc, created.ID))
}

// A teacher from a different center is refused on every operation with 404,
// never 403 — a 403 would confirm the id exists in another center. Schedule
// sub-resources are refused the same way, and the stranger's list stays
// empty.
func TestCrossCenterReadsAreNotFound(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacherA, _ := testutil.Teacher(t, db)
	teacherB, _ := testutil.Teacher(t, db)
	scopeA := testutil.ScopeFor(t, db, teacherA.ID)
	scopeB := testutil.ScopeFor(t, db, teacherB.ID)

	created, err := svc.Create(ctx, scopeA, createRequest())
	require.NoError(t, err)
	scheduleID := created.Schedules[0].ID

	_, err = svc.Get(ctx, scopeB, created.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	_, err = svc.Update(ctx, scopeB, created.ID, classes.UpdateClassRequest{
		Name: "Chiếm lớp", StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(1),
	})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	_, err = svc.Archive(ctx, scopeB, created.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	err = svc.Delete(ctx, scopeB, created.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	_, err = svc.AddSchedule(ctx, scopeB, created.ID, classes.ScheduleRequest{
		Weekday: int16Ptr(1), StartTime: "10:00", DurationMin: 30,
	})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	_, err = svc.UpdateSchedule(ctx, scopeB, created.ID, scheduleID, classes.UpdateScheduleRequest{
		Weekday: int16Ptr(2), StartTime: "18:00", DurationMin: 90, EffectiveFrom: "2026-01-05",
	})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	err = svc.DeleteSchedule(ctx, scopeB, created.ID, scheduleID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	rows, total, err := svc.List(ctx, scopeB, classes.ListFilter{Status: classes.StatusActive}, listParams(t))
	require.NoError(t, err)
	require.Zero(t, total)
	require.Empty(t, rows)

	// The owner still sees everything intact.
	got, err := svc.Get(ctx, scopeA, created.ID)
	require.NoError(t, err)
	require.Equal(t, "Toán 8", got.Name)
	require.Len(t, got.Schedules, 1)
}

// An owner reads, updates, and manages a member's class and its schedules —
// center-wide oversight, not per-teacher isolation. A schedule the owner adds
// still inherits the parent class's own teacher, and a class the owner
// creates from scratch is stamped as the owner's own, never on behalf of a
// member.
func TestOwnerHasFullOversightOfMembersClasses(t *testing.T) {
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

	created, err := svc.Create(ctx, memberScope, createRequest())
	require.NoError(t, err)

	got, err := svc.Get(ctx, ownerScope, created.ID)
	require.NoError(t, err, "owner must read a member's class")
	require.Equal(t, created.ID, got.ID)

	rows, total, err := svc.List(ctx, ownerScope, classes.ListFilter{Status: classes.StatusActive}, listParams(t))
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, created.ID, rows[0].ID, "owner's list must include the member's class")

	updated, err := svc.Update(ctx, ownerScope, created.ID, classes.UpdateClassRequest{
		Name: "Toán 8 (updated)", StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(200_000),
	})
	require.NoError(t, err, "owner must update a member's class")
	require.Equal(t, "Toán 8 (updated)", updated.Name)

	scheduleID := created.Schedules[0].ID
	_, err = svc.UpdateSchedule(ctx, ownerScope, created.ID, scheduleID, classes.UpdateScheduleRequest{
		Weekday: int16Ptr(3), StartTime: "19:00", DurationMin: 60, EffectiveFrom: "2026-01-05",
	})
	require.NoError(t, err, "owner must update a member's class schedule")

	added, err := svc.AddSchedule(ctx, ownerScope, created.ID, classes.ScheduleRequest{
		Weekday: int16Ptr(5), StartTime: "08:00", DurationMin: 45,
	})
	require.NoError(t, err, "owner must add a schedule to a member's class")
	require.Equal(t, member.ID, added.TeacherID,
		"a schedule added by the owner still inherits the parent class's own teacher, not the owner")

	_, err = svc.Archive(ctx, ownerScope, created.ID)
	require.NoError(t, err, "owner must archive a member's class")
	require.NoError(t, svc.Delete(ctx, ownerScope, created.ID), "owner must delete a member's class")
	_, err = svc.Get(ctx, ownerScope, created.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	// An owner creates rows as themselves, never on behalf of a member.
	ownerRow, err := svc.Create(ctx, ownerScope, createRequest())
	require.NoError(t, err)
	require.Equal(t, owner.ID, ownerRow.TeacherID, "owner-created class must be stamped as the owner's own")
}

// Two non-owning teachers in the same center are still isolated from each
// other: center scope grants the owner oversight, not peer-to-peer access.
func TestPeersInSameCenterCannotSeeEachOthersClasses(t *testing.T) {
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

	created, err := svc.Create(ctx, scopeB, createRequest())
	require.NoError(t, err)

	_, err = svc.Get(ctx, scopeC, created.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code, "a peer must not read another member's class")

	rows, total, err := svc.List(ctx, scopeC, classes.ListFilter{Status: classes.StatusActive}, listParams(t))
	require.NoError(t, err)
	require.EqualValues(t, 0, total)
	for _, r := range rows {
		require.NotEqual(t, created.ID, r.ID, "a peer's list must not include another member's class")
	}
}
