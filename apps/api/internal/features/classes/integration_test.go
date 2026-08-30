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
	"teka/apps/api/internal/features/classstaff"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/testutil"
)

func newIntegrationService(t *testing.T) (*classes.Service, *gorm.DB) {
	t.Helper()
	db := testutil.StartPostgres(t)
	return classes.NewService(classes.NewRepository(db), database.NewTxManager(db), classstaff.NewRepository(db)), db
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

// Read scoping by assignment: any class_staff stint — active or ended — lets
// the holder read the class through the readable port, while an unassigned
// peer keeps getting 404 and the write-gate port (Get/Update) stays own-rows.
func TestAssignmentHoldersReadThroughReadablePort(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, owner.ID)
	_, clerk := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, clerk.ID, sc.CenterID)
	_, peer := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, peer.ID, sc.CenterID)
	clerkSc := testutil.ScopeFor(t, db, clerk.ID)
	peerSc := testutil.ScopeFor(t, db, peer.ID)

	created, err := svc.Create(ctx, sc, createRequest())
	require.NoError(t, err)
	stint := testutil.StaffAssignment(t, db, created, clerk.ID, "hoc_vu")

	// The assignment holder reads detail + list, and sees their roles.
	got, roles, err := svc.GetReadable(ctx, clerkSc, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)
	require.Equal(t, []string{"hoc_vu"}, roles)

	rows, listRoles, total, err := svc.ListReadable(ctx, clerkSc, classes.ListFilter{Status: classes.StatusActive}, listParams(t))
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, created.ID, rows[0].ID)
	require.Equal(t, []string{"hoc_vu"}, listRoles[created.ID])

	// An unassigned peer still gets nothing — no existence leak.
	_, _, err = svc.GetReadable(ctx, peerSc, created.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	_, _, peerTotal, err := svc.ListReadable(ctx, peerSc, classes.ListFilter{Status: classes.StatusActive}, listParams(t))
	require.NoError(t, err)
	require.Zero(t, peerTotal)

	// Write-freeze: reading is not writing. The stint holder cannot update,
	// and the write-gate port never widened.
	_, err = svc.Update(ctx, clerkSc, created.ID, classes.UpdateClassRequest{Name: "Đổi tên"})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code, "the write gate stays own-rows")
	_, err = svc.Get(ctx, clerkSc, created.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code, "classes.Get is the shared write gate and must not widen")

	// An ended stint keeps history reads but drops the role from
	// my_staff_roles: roles describe what the caller IS, not what they can
	// still read.
	require.NoError(t, db.Exec(`UPDATE class_staff SET ended_at = now() WHERE id = ?`, stint).Error)
	got, roles, err = svc.GetReadable(ctx, clerkSc, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)
	require.Empty(t, roles)

	// A soft-deleted class grants nothing even to an assignment holder.
	require.NoError(t, db.Exec(`UPDATE classes SET deleted_at = now() WHERE id = ?`, created.ID).Error)
	_, _, err = svc.GetReadable(ctx, clerkSc, created.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
}

// The owner's readable port matches the old behavior: center-wide, roles
// empty (the owner reads by ownership, not by stint).
func TestOwnerReadablePortIsCenterWide(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, owner.ID)
	_, member := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, member.ID, sc.CenterID)
	memberSc := testutil.ScopeFor(t, db, member.ID)

	created, err := svc.Create(ctx, memberSc, createRequest())
	require.NoError(t, err)

	got, roles, err := svc.GetReadable(ctx, sc, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)
	require.Empty(t, roles)
}

// The capability write gate: an ACTIVE stint in a writing role passes, a
// wrong-role or ended stint gets an honest 403 (they can read the class), and
// a teacher with no relationship gets 404 so class ids stay unprobeable.
func TestGetWritableCapabilityGate(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, db)
	ownerSc := testutil.ScopeFor(t, db, owner.ID)
	_, teacher := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, teacher.ID, ownerSc.CenterID)
	teacherSc := testutil.ScopeFor(t, db, teacher.ID)
	_, assistant := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, assistant.ID, ownerSc.CenterID)
	assistantSc := testutil.ScopeFor(t, db, assistant.ID)
	_, outsider := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, outsider.ID, ownerSc.CenterID)
	outsiderSc := testutil.ScopeFor(t, db, outsider.ID)

	class := testutil.Class(t, db, teacher.ID)
	testutil.StaffAssignment(t, db, class, assistant.ID, "tro_giang")

	// Owner bypasses the fragment via CenterWide, whatever the capability.
	_, err := svc.GetWritable(ctx, ownerSc, class.ID, authctx.CapSessionsWrite)
	require.NoError(t, err, "center owner writes without a stint")

	// The creator holds the auto-seeded active giao_vien stint.
	_, err = svc.GetWritable(ctx, teacherSc, class.ID, authctx.CapSessionsWrite)
	require.NoError(t, err, "active giao_vien stint writes sessions")

	// tro_giang is in the attendance role list but not the sessions one.
	_, err = svc.GetWritable(ctx, assistantSc, class.ID, authctx.CapAttendanceWrite)
	require.NoError(t, err, "active tro_giang stint writes attendance")
	_, err = svc.GetWritable(ctx, assistantSc, class.ID, authctx.CapSessionsWrite)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code,
		"tro_giang can read the class, so the sessions denial is an honest 403")

	// No stint at all: the class must look nonexistent.
	_, err = svc.GetWritable(ctx, outsiderSc, class.ID, authctx.CapSessionsWrite)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code,
		"no relationship to the class → 404, not 403")

	// Handoff shape: ending the giao_vien stint keeps history reads but must
	// drop every write, even though classes.teacher_id still names them.
	require.NoError(t, db.Exec(
		`UPDATE class_staff SET ended_at = now() WHERE class_id = ? AND teacher_id = ?`,
		class.ID, teacher.ID).Error)
	_, err = svc.GetWritable(ctx, teacherSc, class.ID, authctx.CapSessionsWrite)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code,
		"an ended stint reads history but never writes — creator rows grant nothing")
}
