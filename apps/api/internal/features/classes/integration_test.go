//go:build integration

package classes_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/classstaff"
	"teka/apps/api/internal/features/enrollments"
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

// StudentCounts must agree with what GET /enrollments?active=true lists for
// the class — the two feed the same screen — so an ended enrollment drops
// out of the count and a class with no enrollments reads 0.
func TestStudentCountsMatchActiveEnrollments(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	contact := testutil.Contact(t, db, teacher.ID)

	withStudents, err := svc.Create(ctx, sc, createRequest())
	require.NoError(t, err)
	emptyReq := createRequest()
	emptyReq.Name = "Văn 9"
	empty, err := svc.Create(ctx, sc, emptyReq)
	require.NoError(t, err)

	for i, endedOn := range []*time.Time{nil, nil, ptrTime(date("2026-02-01"))} {
		studentID := id.New()
		require.NoError(t, db.Exec(
			"INSERT INTO students (id, teacher_id, center_id, contact_id, full_name) VALUES (?, ?, ?, ?, ?)",
			studentID, teacher.ID, sc.CenterID, contact.ID, "Bé "+string(rune('A'+i)),
		).Error)
		require.NoError(t, db.Exec(
			"INSERT INTO enrollments (id, teacher_id, center_id, student_id, class_id, unit_price, started_on, ended_on) VALUES (?, ?, ?, ?, ?, 150000, ?, ?)",
			id.New(), teacher.ID, sc.CenterID, studentID, withStudents.ID, date("2026-01-05"), endedOn,
		).Error)
	}

	counts, err := svc.StudentCounts(ctx, sc, []uuid.UUID{withStudents.ID, empty.ID})
	require.NoError(t, err)
	require.Equal(t, map[uuid.UUID]int64{withStudents.ID: 2}, counts)

	active := true
	_, total, err := enrollments.NewRepository(db).List(ctx, sc,
		enrollments.ListFilter{ClassID: withStudents.ID, Active: &active}, listParams(t))
	require.NoError(t, err)
	require.Equal(t, total, counts[withStudents.ID], "count must match the active enrollments list")

	none, err := svc.StudentCounts(ctx, sc, nil)
	require.NoError(t, err)
	require.Empty(t, none)
}

func ptrTime(t time.Time) *time.Time { return &t }

// The count reaches the client, so it follows the enrollments read filter: a
// member only counts classes they hold a stint on, and enrollments.view_all
// widens the count exactly as it widens the roster list.
func TestStudentCountsFollowEnrollmentReadScope(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, db)
	scOwner := testutil.ScopeFor(t, db, owner.ID)
	contact := testutil.Contact(t, db, owner.ID)

	staffed := testutil.Class(t, db, owner.ID, testutil.WithClassName("Toán 9"))
	other := testutil.Class(t, db, owner.ID, testutil.WithClassName("Văn 9"))
	for _, classID := range []uuid.UUID{staffed.ID, other.ID} {
		student := testutil.Student(t, db, owner.ID, contact.ID)
		testutil.Enrollment(t, db, owner.ID, student.ID, classID, date("2026-01-05"))
	}

	member, _ := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, member.ID, scOwner.CenterID)
	testutil.StaffAssignment(t, db, staffed, member.ID, "tro_giang")
	ids := []uuid.UUID{staffed.ID, other.ID}

	scMember := testutil.ScopeFor(t, db, member.ID)
	counts, err := svc.StudentCounts(ctx, scMember, ids)
	require.NoError(t, err)
	require.Equal(t, map[uuid.UUID]int64{staffed.ID: 1}, counts,
		"a member must not learn the headcount of a class they cannot list")

	scMember.Perms = authctx.BuildPermSet(nil, []string{authctx.PermEnrollmentsViewAll}, nil)
	widened, err := svc.StudentCounts(ctx, scMember, ids)
	require.NoError(t, err)
	require.Equal(t, map[uuid.UUID]int64{staffed.ID: 1, other.ID: 1}, widened)

	all, err := svc.StudentCounts(ctx, scOwner, ids)
	require.NoError(t, err)
	require.Equal(t, widened, all)
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
	got, roles, err := svc.GetReadableWithRoles(ctx, clerkSc, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)
	require.Equal(t, []string{"hoc_vu"}, roles)

	rows, listRoles, total, err := svc.ListReadable(ctx, clerkSc, classes.ListFilter{Status: classes.StatusActive}, listParams(t))
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, created.ID, rows[0].ID)
	require.Equal(t, []string{"hoc_vu"}, listRoles[created.ID])

	// An unassigned peer still gets nothing — no existence leak, on both
	// read-port variants.
	_, err = svc.GetReadable(ctx, peerSc, created.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	_, _, err = svc.GetReadableWithRoles(ctx, peerSc, created.ID)
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
	got, roles, err = svc.GetReadableWithRoles(ctx, clerkSc, created.ID)
	require.NoError(t, err)
	require.Equal(t, created.ID, got.ID)
	require.Empty(t, roles)

	// A soft-deleted class grants nothing even to an assignment holder.
	require.NoError(t, db.Exec(`UPDATE classes SET deleted_at = now() WHERE id = ?`, created.ID).Error)
	_, err = svc.GetReadable(ctx, clerkSc, created.ID)
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

	got, roles, err := svc.GetReadableWithRoles(ctx, sc, created.ID)
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

// A member holding classes.view_all reads any class in the center but cannot
// edit, archive, delete, or retime one they hold no stint on: the visibility
// key never widens the write port. Their own classes stay writable.
func TestViewAllWidensClassReadsNotWrites(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, db)
	scOwner := testutil.ScopeFor(t, db, owner.ID)
	member, _ := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, member.ID, scOwner.CenterID)

	ownerClass := testutil.Class(t, db, owner.ID, testutil.WithClassStartDate(date("2026-01-01")))
	ownerSchedule := testutil.Schedule(t, db, ownerClass, 2, "18:00")

	scMember := testutil.ScopeFor(t, db, member.ID)
	scMember.Perms = authctx.BuildPermSet(nil, []string{authctx.PermClassesViewAll}, nil)
	require.True(t, scMember.CenterWideFor(authctx.PermClassesViewAll))

	got, err := svc.GetReadable(ctx, scMember, ownerClass.ID)
	require.NoError(t, err)
	require.Equal(t, ownerClass.ID, got.ID)

	_, err = svc.Update(ctx, scMember, ownerClass.ID, classes.UpdateClassRequest{
		Name: "Lớp Sửa Trộm", StartDate: "2026-01-01", DefaultUnitPrice: int64Ptr(1),
	})
	require.Equal(t, 404, apperror.From(err).Status, "a visibility key must not widen edits")
	_, err = svc.Archive(ctx, scMember, ownerClass.ID)
	require.Equal(t, 404, apperror.From(err).Status, "a visibility key must not widen archiving")
	require.Equal(t, 404, apperror.From(svc.Delete(ctx, scMember, ownerClass.ID)).Status,
		"a visibility key must not widen deletion")
	_, err = svc.AddSchedule(ctx, scMember, ownerClass.ID, classes.ScheduleRequest{
		Weekday: int16Ptr(4), StartTime: "19:00", DurationMin: 60,
	})
	require.Equal(t, 404, apperror.From(err).Status, "a visibility key must not widen schedule creation")
	_, err = svc.UpdateSchedule(ctx, scMember, ownerClass.ID, ownerSchedule.ID, classes.UpdateScheduleRequest{
		Weekday: int16Ptr(5), StartTime: "19:00", DurationMin: 60, EffectiveFrom: "2026-01-01",
	})
	require.Equal(t, 404, apperror.From(err).Status, "a visibility key must not widen schedule edits")
	require.Equal(t, 404, apperror.From(svc.DeleteSchedule(ctx, scMember, ownerClass.ID, ownerSchedule.ID)).Status,
		"a visibility key must not widen schedule deletion")

	unchanged, err := svc.Get(ctx, scOwner, ownerClass.ID)
	require.NoError(t, err)
	require.Equal(t, ownerClass.Name, unchanged.Name)
	require.Equal(t, classes.StatusActive, unchanged.Status)
	require.Len(t, unchanged.Schedules, 1)
	require.EqualValues(t, 2, unchanged.Schedules[0].Weekday)

	ownClass := testutil.Class(t, db, member.ID, testutil.WithClassStartDate(date("2026-01-01")))
	updated, err := svc.Update(ctx, scMember, ownClass.ID, classes.UpdateClassRequest{
		Name: "Lớp Nhà Mình", StartDate: "2026-01-01", DefaultUnitPrice: int64Ptr(150_000),
	})
	require.NoError(t, err)
	require.Equal(t, "Lớp Nhà Mình", updated.Name)
	_, err = svc.AddSchedule(ctx, scMember, ownClass.ID, classes.ScheduleRequest{
		Weekday: int16Ptr(4), StartTime: "19:00", DurationMin: 60,
	})
	require.NoError(t, err)
	_, err = svc.Archive(ctx, scMember, ownClass.ID)
	require.NoError(t, err)
}

// TestListFiltersMatchLiterallyAndOnEffectiveTimetable drives every list
// filter through Postgres: the search escapes ILIKE metacharacters so "100%"
// and "a_b" match literally and also hit the code; the tag filter is JSONB
// containment of one whole element; weekday/shift only see timetable rows
// still effective today; and the phase filter follows the same calendar rules
// PhaseOf applies in Go.
func TestListFiltersMatchLiterallyAndOnEffectiveTimetable(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)

	running := testutil.Class(t, db, teacher.ID,
		testutil.WithClassName("Toán 100% nâng cao"), testutil.WithClassCode("TOAN-A"),
		testutil.WithClassTags("Toán", "Khối 8"), testutil.WithClassStartDate(date("2000-01-01")))
	testutil.Schedule(t, db, running, 2, "18:00")
	testutil.Schedule(t, db, running, 4, "08:00")

	upcoming := testutil.Class(t, db, teacher.ID,
		testutil.WithClassName("Văn a_b"), testutil.WithClassCode("VAN-B"),
		testutil.WithClassTags("Văn"), testutil.WithClassStartDate(date("2099-01-01")))
	testutil.Schedule(t, db, upcoming, 2, "14:00")

	ended := testutil.Class(t, db, teacher.ID,
		testutil.WithClassName("Lý cũ"), testutil.WithClassCode("LY-C"),
		testutil.WithClassTags("Khối 8"),
		testutil.WithClassStartDate(date("2000-01-01")), testutil.WithClassEndDate(date("2000-12-31")))
	closed := testutil.Schedule(t, db, ended, 6, "18:00")
	require.NoError(t, db.Model(closed).Update("effective_to", date("2001-01-01")).Error)

	archived := testutil.Class(t, db, teacher.ID,
		testutil.WithClassName("Lớp lưu trữ"), testutil.WithClassCode("ARCH-D"),
		testutil.WithClassStatus(classes.StatusArchived), testutil.WithClassStartDate(date("2000-01-01")))

	// Would match "100%" and "a_b" if the metacharacters were left unescaped.
	decoy := testutil.Class(t, db, teacher.ID,
		testutil.WithClassName("Toán 1000 acb"), testutil.WithClassCode("DECOY-E"),
		testutil.WithClassStartDate(date("2000-01-01")))

	ids := func(filter classes.ListFilter) []uuid.UUID {
		t.Helper()
		rows, _, total, err := svc.ListReadable(ctx, sc, filter, listParams(t))
		require.NoError(t, err)
		require.EqualValues(t, len(rows), total)
		return classes.ClassIDs(rows)
	}
	cases := []struct {
		name   string
		filter classes.ListFilter
		want   []uuid.UUID
	}{
		{"percent literal", classes.ListFilter{Q: "100%"}, []uuid.UUID{running.ID}},
		{"underscore literal", classes.ListFilter{Q: "a_b"}, []uuid.UUID{upcoming.ID}},
		{"code case-insensitive", classes.ListFilter{Q: "van-b"}, []uuid.UUID{upcoming.ID}},
		{"tag whole element", classes.ListFilter{Tag: "Khối 8"}, []uuid.UUID{ended.ID, running.ID}},
		{"tag no prefix match", classes.ListFilter{Tag: "Khối"}, nil},
		{"weekday effective only", classes.ListFilter{Weekday: int16Ptr(2)}, []uuid.UUID{running.ID, upcoming.ID}},
		{"weekday closed row ignored", classes.ListFilter{Weekday: int16Ptr(6)}, nil},
		{"shift morning", classes.ListFilter{Shift: classes.ShiftMorning}, []uuid.UUID{running.ID}},
		{"shift afternoon", classes.ListFilter{Shift: classes.ShiftAfternoon}, []uuid.UUID{upcoming.ID}},
		{"shift evening", classes.ListFilter{Shift: classes.ShiftEvening}, []uuid.UUID{running.ID}},
		{"weekday and shift on one row", classes.ListFilter{Weekday: int16Ptr(2), Shift: classes.ShiftAfternoon}, []uuid.UUID{upcoming.ID}},
		{"phase upcoming", classes.ListFilter{Phase: classes.PhaseUpcoming}, []uuid.UUID{upcoming.ID}},
		{"phase running", classes.ListFilter{Phase: classes.PhaseRunning}, []uuid.UUID{running.ID, decoy.ID}},
		{"phase ended", classes.ListFilter{Phase: classes.PhaseEnded}, []uuid.UUID{ended.ID}},
		{"phase archived", classes.ListFilter{Phase: classes.PhaseArchived}, []uuid.UUID{archived.ID}},
		{"phase and status disagree", classes.ListFilter{Phase: classes.PhaseArchived, Status: classes.StatusActive}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.ElementsMatch(t, tc.want, ids(tc.filter))
		})
	}

	// The recruiting filter is the SQL form of OpenForRecruitment: the flag
	// alone is not enough once the class has ended or been archived.
	for _, c := range []*classes.Class{running, upcoming, ended, archived} {
		require.NoError(t, db.Model(c).Update("recruiting", true).Error)
	}
	require.ElementsMatch(t, []uuid.UUID{running.ID, upcoming.ID}, ids(classes.ListFilter{Recruiting: true}))
	require.ElementsMatch(t, []uuid.UUID{upcoming.ID}, ids(classes.ListFilter{Recruiting: true, Phase: classes.PhaseUpcoming}))
	recruitingStats, err := svc.Stats(ctx, sc, time.Now(), true)
	require.NoError(t, err)
	require.Equal(t, classes.ClassStatsResponse{All: 2, Upcoming: 1, Running: 1, Recruiting: 2}, recruitingStats)

	// The phase the SQL predicate selected must be the phase the DTO reports.
	for _, phase := range []string{classes.PhaseUpcoming, classes.PhaseRunning, classes.PhaseEnded, classes.PhaseArchived} {
		rows, _, _, err := svc.ListReadable(ctx, sc, classes.ListFilter{Phase: phase}, listParams(t))
		require.NoError(t, err)
		for i := range rows {
			require.Equal(t, phase, classes.FromModel(&rows[i]).Phase, rows[i].Name)
		}
	}
}

// TestStatsCountReadableClassesOnly checks the stats counters run through the
// same readable port as the list: a member counts only their own classes, the
// owner and a classes.view_all holder count the whole center, and the
// per-phase buckets partition the total.
func TestStatsCountReadableClassesOnly(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, db)
	scOwner := testutil.ScopeFor(t, db, owner.ID)
	member, _ := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, member.ID, scOwner.CenterID)
	scMember := testutil.ScopeFor(t, db, member.ID)

	testutil.Class(t, db, owner.ID, testutil.WithClassStartDate(date("2000-01-01")), testutil.WithClassRecruiting(true))
	testutil.Class(t, db, owner.ID, testutil.WithClassStartDate(date("2099-01-01")))
	testutil.Class(t, db, owner.ID, testutil.WithClassStartDate(date("2000-01-01")), testutil.WithClassEndDate(date("2000-12-31")))
	testutil.Class(t, db, owner.ID, testutil.WithClassStartDate(date("2000-01-01")), testutil.WithClassStatus(classes.StatusArchived))
	testutil.Class(t, db, member.ID, testutil.WithClassStartDate(date("2000-01-01")))

	// A class in another center never leaks into anyone's counters.
	stranger, _ := testutil.Teacher(t, db)
	testutil.Class(t, db, stranger.ID, testutil.WithClassRecruiting(true))

	now := time.Now()
	got, err := svc.Stats(ctx, scMember, now, false)
	require.NoError(t, err)
	require.Equal(t, classes.ClassStatsResponse{All: 1, Running: 1}, got)

	centerWide := classes.ClassStatsResponse{All: 5, Upcoming: 1, Running: 2, Ended: 1, Archived: 1, Recruiting: 1}
	got, err = svc.Stats(ctx, scOwner, now, false)
	require.NoError(t, err)
	require.Equal(t, centerWide, got)

	scMember.Perms = authctx.BuildPermSet(nil, []string{authctx.PermClassesViewAll}, nil)
	got, err = svc.Stats(ctx, scMember, now, false)
	require.NoError(t, err)
	require.Equal(t, centerWide, got, "classes.view_all widens the counters like it widens the list")
	require.Equal(t, got.All, got.Upcoming+got.Running+got.Ended+got.Archived, "phases partition the total")
}

// TestClassCodeIsUniquePerCenterThroughTheIndex exercises the code
// uniqueness rules against the real partial unique index: a taken code is
// refused with 409 on create and on update, keeping your own code on update
// is fine, a soft-deleted class frees its code, and another center may reuse
// it.
func TestClassCodeIsUniquePerCenterThroughTheIndex(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)

	req := createRequest()
	req.Code = strPtr(" TOAN8 ")
	first, err := svc.Create(ctx, sc, req)
	require.NoError(t, err)
	require.Equal(t, "TOAN8", first.Code, "the code is trimmed before storing")

	req.Name = "Toán 8 B"
	_, err = svc.Create(ctx, sc, req)
	require.Equal(t, http.StatusConflict, apperror.From(err).Status)
	require.Equal(t, classes.CodeClassCodeTaken, apperror.From(err).Code)

	req.Code = nil
	second, err := svc.Create(ctx, sc, req)
	require.NoError(t, err)
	require.NotEmpty(t, second.Code, "a blank code is minted")
	require.NotEqual(t, first.Code, second.Code)

	_, err = svc.Update(ctx, sc, second.ID, classes.UpdateClassRequest{
		Name: second.Name, StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(150_000), Code: strPtr("TOAN8"),
	})
	require.Equal(t, http.StatusConflict, apperror.From(err).Status, "moving onto another class's code is a collision")
	_, err = svc.Update(ctx, sc, second.ID, classes.UpdateClassRequest{
		Name: second.Name, StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(150_000), Code: strPtr("toan8"),
	})
	require.Equal(t, http.StatusUnprocessableEntity, apperror.From(err).Status, "lowercase never reaches the index")

	kept, err := svc.Update(ctx, sc, first.ID, classes.UpdateClassRequest{
		Name: "Toán 8 đổi tên", StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(150_000), Code: strPtr("TOAN8"),
	})
	require.NoError(t, err, "re-sending your own code is not a collision")
	require.Equal(t, "TOAN8", kept.Code)

	require.NoError(t, svc.Delete(ctx, sc, first.ID))
	req.Code = strPtr("TOAN8")
	req.Name = "Toán 8 mới"
	reused, err := svc.Create(ctx, sc, req)
	require.NoError(t, err, "a soft-deleted class no longer holds its code")
	require.Equal(t, "TOAN8", reused.Code)

	other, _ := testutil.Teacher(t, db)
	scOther := testutil.ScopeFor(t, db, other.ID)
	_, err = svc.Create(ctx, scOther, req)
	require.NoError(t, err, "codes are unique per center, not globally")
}

func strPtr(s string) *string { return &s }

// insertCourse seeds a catalog course directly so this package's tests do not
// depend on the courses service; the classes side only reads the row.
func insertCourse(t *testing.T, db *gorm.DB, centerID uuid.UUID, code string, price int64) uuid.UUID {
	t.Helper()
	courseID := uuid.New()
	require.NoError(t, db.Exec(
		`INSERT INTO courses (id, center_id, code, name, status, default_unit_price)
		 VALUES (?, ?, ?, ?, 'active', ?)`,
		courseID, centerID, code, "Khóa "+code, price,
	).Error)
	return courseID
}

func TestCourseLinkStaysInsideCenterAndCopiesPrice(t *testing.T) {
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	other, _ := testutil.Teacher(t, db)
	scOther := testutil.ScopeFor(t, db, other.ID)

	mine := insertCourse(t, db, sc.CenterID, "TOAN-8", 180_000)
	foreign := insertCourse(t, db, scOther.CenterID, "TOAN-8", 90_000)

	req := createRequest()
	req.DefaultUnitPrice = nil
	req.CourseID = strPtr(foreign.String())
	_, err := svc.Create(ctx, sc, req)
	var appErr *apperror.AppError
	require.ErrorAs(t, err, &appErr, "another center's course is rejected before insert")
	require.Equal(t, http.StatusUnprocessableEntity, appErr.Status)
	require.Contains(t, appErr.Fields, "course_id")

	req.CourseID = nil
	_, err = svc.Create(ctx, sc, req)
	require.ErrorAs(t, err, &appErr, "no course and no price is a validation error")
	require.Equal(t, http.StatusUnprocessableEntity, appErr.Status)
	require.Contains(t, appErr.Fields, "default_unit_price")

	req.CourseID = strPtr(mine.String())
	attached, err := svc.Create(ctx, sc, req)
	require.NoError(t, err)
	require.Equal(t, int64(180_000), attached.DefaultUnitPrice, "price copied from the course")
	require.NotNil(t, attached.CourseID)
	require.Equal(t, mine, *attached.CourseID)

	got, err := svc.Get(ctx, sc, attached.ID)
	require.NoError(t, err)
	require.NotNil(t, got.Course, "GET preloads the course reference")
	require.Equal(t, "TOAN-8", got.Course.Code)
	require.Equal(t, "Khóa TOAN-8", got.Course.Name)

	plain, err := svc.Create(ctx, sc, createRequest())
	require.NoError(t, err)
	require.Nil(t, plain.CourseID)

	rows, total, err := svc.List(ctx, sc, classes.ListFilter{CourseID: &mine}, listParams(t))
	require.NoError(t, err)
	require.Equal(t, int64(1), total, "course_id filter narrows to attached classes")
	require.Equal(t, attached.ID, rows[0].ID)
	require.NotNil(t, rows[0].Course)

	explicit := createRequest()
	explicit.CourseID = strPtr(mine.String())
	explicit.DefaultUnitPrice = int64Ptr(200_000)
	own, err := svc.Create(ctx, sc, explicit)
	require.NoError(t, err)
	require.Equal(t, int64(200_000), own.DefaultUnitPrice, "an explicit price wins over the course default")

	detached, err := svc.Update(ctx, sc, attached.ID, classes.UpdateClassRequest{
		Name: "Toán 8", StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(180_000), CourseID: strPtr(""),
	})
	require.NoError(t, err)
	require.Nil(t, detached.CourseID, "an empty course_id detaches")
	require.Nil(t, detached.Course)

	kept, err := svc.Update(ctx, sc, own.ID, classes.UpdateClassRequest{
		Name: "Toán 8", StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(200_000),
	})
	require.NoError(t, err)
	require.NotNil(t, kept.CourseID, "an absent course_id keeps the stored course")

	_, err = svc.Update(ctx, sc, own.ID, classes.UpdateClassRequest{
		Name: "Toán 8", StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(200_000), CourseID: strPtr(foreign.String()),
	})
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, http.StatusUnprocessableEntity, appErr.Status)

	_, _, err = svc.List(ctx, scOther, classes.ListFilter{CourseID: &mine}, listParams(t))
	require.NoError(t, err, "filtering by a foreign course just yields nothing")
}

// A parent must be a live class of the same center: another center's class
// and a soft-deleted class are both refused, against the real query
// (LiveClassInCenter), not the in-memory fake the unit tests use.
func TestParentClassMustBeLiveAndInCenter(t *testing.T) {
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	other, _ := testutil.Teacher(t, db)
	scOther := testutil.ScopeFor(t, db, other.ID)

	theirs, err := svc.Create(ctx, scOther, createRequest())
	require.NoError(t, err)

	toDelete, err := svc.Create(ctx, sc, createRequest())
	require.NoError(t, err)
	require.NoError(t, svc.Delete(ctx, sc, toDelete.ID))

	class, err := svc.Create(ctx, sc, createRequest())
	require.NoError(t, err)

	upd := classes.UpdateClassRequest{Name: class.Name, StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(150_000)}

	upd.ParentClassID = strPtr(theirs.ID.String())
	_, err = svc.Update(ctx, sc, class.ID, upd)
	var appErr *apperror.AppError
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, http.StatusUnprocessableEntity, appErr.Status)
	require.Contains(t, appErr.Fields, "parent_class_id")

	upd.ParentClassID = strPtr(toDelete.ID.String())
	_, err = svc.Update(ctx, sc, class.ID, upd)
	require.ErrorAs(t, err, &appErr)
	require.Equal(t, http.StatusUnprocessableEntity, appErr.Status)
	require.Contains(t, appErr.Fields, "parent_class_id")

	live, err := svc.Create(ctx, sc, createRequest())
	require.NoError(t, err)
	upd.ParentClassID = strPtr(live.ID.String())
	got, err := svc.Update(ctx, sc, class.ID, upd)
	require.NoError(t, err)
	require.NotNil(t, got.ParentClassID)
	require.Equal(t, live.ID, *got.ParentClassID)
}

// A parent link may not close a loop through the lineage chain, whether the
// loop is direct (a<-b, then a<-b) or runs through a longer chain
// (a<-b<-c, then a<-c) — proven here against the real WITH RECURSIVE query.
func TestParentClassCycleIsRefusedAcrossTheChain(t *testing.T) {
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)

	classA, err := svc.Create(ctx, sc, createRequest())
	require.NoError(t, err)
	classB, err := svc.Create(ctx, sc, createRequest())
	require.NoError(t, err)
	classC, err := svc.Create(ctx, sc, createRequest())
	require.NoError(t, err)

	_, err = svc.Update(ctx, sc, classB.ID, classes.UpdateClassRequest{
		Name: classB.Name, StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(150_000),
		ParentClassID: strPtr(classA.ID.String()),
	})
	require.NoError(t, err, "attach b under a")

	updA := classes.UpdateClassRequest{Name: classA.Name, StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(150_000)}
	updA.ParentClassID = strPtr(classB.ID.String())
	_, err = svc.Update(ctx, sc, classA.ID, updA)
	var appErr *apperror.AppError
	require.ErrorAs(t, err, &appErr, "a<-b, a->b closes a direct loop")
	require.Equal(t, http.StatusUnprocessableEntity, appErr.Status)
	require.Contains(t, appErr.Fields, "parent_class_id")

	_, err = svc.Update(ctx, sc, classC.ID, classes.UpdateClassRequest{
		Name: classC.Name, StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(150_000),
		ParentClassID: strPtr(classB.ID.String()),
	})
	require.NoError(t, err, "attach c under b")

	updA.ParentClassID = strPtr(classC.ID.String())
	_, err = svc.Update(ctx, sc, classA.ID, updA)
	require.ErrorAs(t, err, &appErr, "a<-b<-c, a->c closes a longer loop")
	require.Equal(t, http.StatusUnprocessableEntity, appErr.Status)
	require.Contains(t, appErr.Fields, "parent_class_id")

	// c<-b remains a valid, acyclic reattachment: c's own current chain
	// (through b, through a) does not go through c itself.
	got, err := svc.Update(ctx, sc, classC.ID, classes.UpdateClassRequest{
		Name: classC.Name, StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(150_000),
		ParentClassID: strPtr(classB.ID.String()),
	})
	require.NoError(t, err)
	require.NotNil(t, got.ParentClassID)
	require.Equal(t, classB.ID, *got.ParentClassID)
}

// A self-paced class carries no timetable: zero schedules must pass create,
// the stored study_mode and room must round-trip through Get, and adding a
// schedule afterwards must still be refused against the real DB-backed
// class, not just the in-memory fake the unit tests use.
func TestSelfPacedCreateWithZeroSchedulesRoundTrips(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)

	req := classes.CreateClassRequest{
		Name:             "Tự học Python",
		StartDate:        "2026-01-05",
		DefaultUnitPrice: int64Ptr(0),
		StudyMode:        classes.StudyModeSelfPaced,
		Room:             "P101",
	}
	created, err := svc.Create(ctx, sc, req)
	require.NoError(t, err, "self-paced create with zero schedules must succeed")
	require.Empty(t, created.Schedules)
	require.Equal(t, classes.StudyModeSelfPaced, created.StudyMode)
	require.Equal(t, "P101", created.Room)

	got, err := svc.Get(ctx, sc, created.ID)
	require.NoError(t, err)
	require.Equal(t, classes.StudyModeSelfPaced, got.StudyMode)
	require.Equal(t, "P101", got.Room)
	require.Empty(t, got.Schedules)

	_, err = svc.AddSchedule(ctx, sc, created.ID, classes.ScheduleRequest{
		Weekday: int16Ptr(2), StartTime: "18:00", DurationMin: 60,
	})
	var appErr *apperror.AppError
	require.ErrorAs(t, err, &appErr, "a self-paced class must refuse a schedule even against the real DB")
	require.Equal(t, http.StatusUnprocessableEntity, appErr.Status)

	// A scheduled class (the default study_mode) still requires at least one
	// schedule.
	scheduled := createRequest()
	scheduled.Schedules = nil
	_, err = svc.Create(ctx, sc, scheduled)
	require.ErrorAs(t, err, &appErr, "a scheduled class with no schedules must be refused")
	require.Equal(t, http.StatusUnprocessableEntity, appErr.Status)
}

// A next_class_id link must point at a live class of the same center, the
// same rule parent_class_id follows, exercised here against the real
// LiveClassInCenter/ParentCreatesCycle queries instead of the in-memory fake:
// an unknown id, a foreign-center class, and a direct two-class cycle are all
// refused with a 422 on next_class_id, while a valid link round-trips
// through Get and an explicit "" clears it.
func TestNextClassLinkAgainstRealDB(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	other, _ := testutil.Teacher(t, db)
	scOther := testutil.ScopeFor(t, db, other.ID)

	classA, err := svc.Create(ctx, sc, createRequest())
	require.NoError(t, err)
	classB, err := svc.Create(ctx, sc, createRequest())
	require.NoError(t, err)
	theirs, err := svc.Create(ctx, scOther, createRequest())
	require.NoError(t, err)

	updA := classes.UpdateClassRequest{Name: classA.Name, StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(150_000)}

	updA.NextClassID = strPtr(uuid.New().String())
	_, err = svc.Update(ctx, sc, classA.ID, updA)
	var appErr *apperror.AppError
	require.ErrorAs(t, err, &appErr, "an unknown next_class_id must be refused")
	require.Equal(t, http.StatusUnprocessableEntity, appErr.Status)
	require.Contains(t, appErr.Fields, "next_class_id")

	updA.NextClassID = strPtr(theirs.ID.String())
	_, err = svc.Update(ctx, sc, classA.ID, updA)
	require.ErrorAs(t, err, &appErr, "a foreign-center class must be refused")
	require.Equal(t, http.StatusUnprocessableEntity, appErr.Status)
	require.Contains(t, appErr.Fields, "next_class_id")

	updA.NextClassID = strPtr(classB.ID.String())
	got, err := svc.Update(ctx, sc, classA.ID, updA)
	require.NoError(t, err, "a live class of the same center links")
	require.NotNil(t, got.NextClassID)
	require.Equal(t, classB.ID, *got.NextClassID)

	reread, err := svc.Get(ctx, sc, classA.ID)
	require.NoError(t, err)
	require.NotNil(t, reread.NextClassID)
	require.Equal(t, classB.ID, *reread.NextClassID)

	// B's next cannot be A: that would close a direct two-class loop.
	updB := classes.UpdateClassRequest{Name: classB.Name, StartDate: "2026-01-05", DefaultUnitPrice: int64Ptr(150_000)}
	updB.NextClassID = strPtr(classA.ID.String())
	_, err = svc.Update(ctx, sc, classB.ID, updB)
	require.ErrorAs(t, err, &appErr, "a<-next-b, b->next-a closes a direct loop")
	require.Equal(t, http.StatusUnprocessableEntity, appErr.Status)
	require.Contains(t, appErr.Fields, "next_class_id")

	// Clearing with an explicit "" unlinks.
	updA.NextClassID = strPtr("")
	cleared, err := svc.Update(ctx, sc, classA.ID, updA)
	require.NoError(t, err)
	require.Nil(t, cleared.NextClassID)

	rereadCleared, err := svc.Get(ctx, sc, classA.ID)
	require.NoError(t, err)
	require.Nil(t, rereadCleared.NextClassID, "clearing next_class_id must persist")
}

// The availability endpoint reports a room/teacher busy only when a live
// class's active schedule overlaps a requested slot on the same weekday,
// exercised here against the real batched queries (AvailabilityClasses,
// ActiveMemberDirectory) instead of the in-memory fake: exclude_class_id
// frees the excluded class's room/teacher, and a non-overlapping slot leaves
// everyone free.
func TestAvailabilityAgainstRealDB(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, owner.ID)

	busyReq := createRequest()
	busyReq.Room = "P101"
	busyReq.Schedules = []classes.ScheduleRequest{{Weekday: int16Ptr(2), StartTime: "18:00", DurationMin: 90}}
	busy, err := svc.Create(ctx, sc, busyReq)
	require.NoError(t, err)

	freeReq := createRequest()
	freeReq.Name = "Lớp trống"
	freeReq.Room = "P102"
	freeReq.Schedules = []classes.ScheduleRequest{{Weekday: int16Ptr(3), StartTime: "18:00", DurationMin: 90}}
	_, err = svc.Create(ctx, sc, freeReq)
	require.NoError(t, err)

	overlapping := []classes.AvailabilitySlot{{Weekday: 2, StartTime: "18:30", DurationMin: 30}}
	resp, err := svc.Availability(ctx, sc, overlapping, nil)
	require.NoError(t, err)

	roomFree := map[string]bool{}
	for _, r := range resp.Rooms {
		roomFree[r.Name] = r.Free
	}
	require.False(t, roomFree["P101"], "an overlapping schedule must mark its room busy")
	require.True(t, roomFree["P102"], "a room with no overlapping schedule stays free")

	teacherFree := map[uuid.UUID]bool{}
	for _, tRow := range resp.Teachers {
		teacherFree[tRow.TeacherID] = tRow.Free
	}
	require.False(t, teacherFree[owner.ID], "the owner teaching the overlapping class must be busy")

	// Excluding the busy class from the check frees its room and teacher —
	// this is the "editing my own class" case.
	respExcluded, err := svc.Availability(ctx, sc, overlapping, &busy.ID)
	require.NoError(t, err)
	for _, r := range respExcluded.Rooms {
		if r.Name == "P101" {
			require.True(t, r.Free, "exclude_class_id must free the excluded class's own room")
		}
	}

	// A non-overlapping slot on the same weekday leaves the room free.
	nonOverlapping := []classes.AvailabilitySlot{{Weekday: 2, StartTime: "08:00", DurationMin: 30}}
	respFree, err := svc.Availability(ctx, sc, nonOverlapping, nil)
	require.NoError(t, err)
	for _, r := range respFree.Rooms {
		if r.Name == "P101" {
			require.True(t, r.Free, "a non-overlapping slot must not mark the room busy")
		}
	}
}
