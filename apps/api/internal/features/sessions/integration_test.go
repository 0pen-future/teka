//go:build integration

package sessions_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/testutil"
)

// newIntegrationService wires the real dependency chain router.go uses:
// classes and teachers services as sessions' consumer interfaces, enrollments
// as its roster source.
func newIntegrationService(t *testing.T) (*sessions.Service, *gorm.DB) {
	t.Helper()
	db := testutil.StartPostgres(t)
	classesSvc := classes.NewService(classes.NewRepository(db), database.NewTxManager(db))
	teachersSvc := teachers.NewService(teachers.NewRepository(db))
	enrollmentsSvc := enrollments.NewService(enrollments.NewRepository(db))
	svc := sessions.NewService(sessions.NewRepository(db), classesSvc, teachersSvc, enrollmentsSvc)
	return svc, db
}

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestListRangeGeneratesAndIsIdempotent(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	testutil.Schedule(t, db, class, 2, "18:00") // Tuesday

	first, err := svc.ListRange(ctx, sc, class.ID, date("2026-01-01"), date("2026-01-31"))
	require.NoError(t, err)
	require.Len(t, first, 4, "want 4 Tuesdays in January 2026")

	second, err := svc.ListRange(ctx, sc, class.ID, date("2026-01-01"), date("2026-01-31"))
	require.NoError(t, err)
	require.Len(t, second, len(first), "regenerating an overlapping range must not duplicate rows")

	var count int64
	require.NoError(t, db.Table("class_sessions").
		Where("class_id = ? AND deleted_at IS NULL", class.ID).Count(&count).Error)
	require.EqualValues(t, 4, count)
}

// TestConcurrentGenerationInsertsExactlyOneRowPerDate empirically verifies
// that clause.OnConflict{TargetWhere: "deleted_at IS NULL"} targets the
// partial unique index uq_class_sessions_per_day: two goroutines racing to
// generate the same range must leave exactly one row per date, not a
// constraint-violation error and not two rows.
func TestConcurrentGenerationInsertsExactlyOneRowPerDate(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	testutil.Schedule(t, db, class, 2, "18:00")

	const goroutines = 8
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	for i := range goroutines {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := svc.ListRange(ctx, sc, class.ID, date("2026-01-01"), date("2026-01-31"))
			errs[i] = err
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		require.NoError(t, err, "goroutine %d", i)
	}

	var count int64
	require.NoError(t, db.Table("class_sessions").
		Where("class_id = ? AND deleted_at IS NULL", class.ID).Count(&count).Error)
	require.EqualValues(t, 4, count, "concurrent generation must not duplicate a date")
}

func TestSoftDeletedSessionIsRegeneratedNextCall(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	testutil.Schedule(t, db, class, 2, "18:00")

	rows, err := svc.ListRange(ctx, sc, class.ID, date("2026-01-01"), date("2026-01-31"))
	require.NoError(t, err)
	require.NoError(t, svc.Delete(ctx, sc, rows[0].ID))

	regenerated, err := svc.ListRange(ctx, sc, class.ID, date("2026-01-01"), date("2026-01-31"))
	require.NoError(t, err)
	require.Len(t, regenerated, len(rows), "a soft-deleted date must be regenerated with a fresh row")
	require.NotEqual(t, rows[0].ID, regenerated[0].ID, "the regenerated row must be a new id, not the deleted one")
}

func TestCancelledSessionKeepsItsDateAndIsNotRegenerated(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	testutil.Schedule(t, db, class, 2, "18:00")

	rows, err := svc.ListRange(ctx, sc, class.ID, date("2026-01-01"), date("2026-01-31"))
	require.NoError(t, err)
	cancelled, err := svc.Cancel(ctx, sc, rows[0].ID, "nghỉ lễ")
	require.NoError(t, err)

	regenerated, err := svc.ListRange(ctx, sc, class.ID, date("2026-01-01"), date("2026-01-31"))
	require.NoError(t, err)
	require.Len(t, regenerated, len(rows))
	var found *sessions.Detail
	for i := range regenerated {
		if regenerated[i].ID == cancelled.ID {
			found = &regenerated[i]
		}
	}
	require.NotNil(t, found, "the cancelled session's row must survive regeneration")
	require.Equal(t, sessions.StatusCancelled, found.Status)
	require.True(t, found.SessionDate.Equal(cancelled.SessionDate))
}

func TestCancelAndDeleteRefuseWhenAttendanceConfirmed(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	testutil.Schedule(t, db, class, 2, "18:00")
	rows, err := svc.ListRange(ctx, sc, class.ID, date("2026-01-01"), date("2026-01-31"))
	require.NoError(t, err)

	// Simulate phase 2 confirming attendance via a direct SQL stamp, mirroring
	// how attendance_confirmed_at gets set before the attendance feature
	// exists. status stays 'held' so the schema's CHECK constraint
	// (status <> 'cancelled' OR attendance_confirmed_at IS NULL) is satisfied.
	require.NoError(t, db.Exec(
		"UPDATE class_sessions SET status = 'held', attendance_confirmed_at = now() WHERE id = ?",
		rows[0].ID).Error)

	_, err = svc.Cancel(ctx, sc, rows[0].ID, "nghỉ lễ")
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)

	err = svc.Delete(ctx, sc, rows[0].ID)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)
}

func TestCreateAdHocConflictsWithExistingDate(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	testutil.Schedule(t, db, class, 2, "18:00")
	rows, err := svc.ListRange(ctx, sc, class.ID, date("2026-01-01"), date("2026-01-31"))
	require.NoError(t, err)

	_, err = svc.CreateAdHoc(ctx, sc, class.ID, sessions.CreateSessionRequest{
		SessionDate: rows[0].SessionDate.Format("2006-01-02"),
	})
	require.ErrorIs(t, err, sessions.ErrSessionExists)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)

	created, err := svc.CreateAdHoc(ctx, sc, class.ID, sessions.CreateSessionRequest{
		SessionDate: "2026-01-15", StartTime: "10:00",
	})
	require.NoError(t, err)
	require.NotNil(t, created.StartTime)
	require.Equal(t, "10:00", string(*created.StartTime))
}

func TestCrossTenantReadsAreNotFound(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacherA, _ := testutil.Teacher(t, db)
	teacherB, _ := testutil.Teacher(t, db)
	scopeA := testutil.ScopeFor(t, db, teacherA.ID)
	scopeB := testutil.ScopeFor(t, db, teacherB.ID)
	class := testutil.Class(t, db, teacherA.ID, testutil.WithClassStartDate(date("2026-01-01")))
	testutil.Schedule(t, db, class, 2, "18:00")
	rows, err := svc.ListRange(ctx, scopeA, class.ID, date("2026-01-01"), date("2026-01-31"))
	require.NoError(t, err)

	_, err = svc.Get(ctx, scopeB, rows[0].ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	_, err = svc.ListRange(ctx, scopeB, class.ID, date("2026-01-01"), date("2026-01-31"))
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code, "teacher B's request against A's class must 404")
}

func TestGenerationRespectsClassAndScheduleBoundaries(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	classEnd := date("2026-01-20")
	class := testutil.Class(t, db, teacher.ID,
		testutil.WithClassStartDate(date("2026-01-06")))
	require.NoError(t, db.Model(class).Update("end_date", classEnd).Error)
	testutil.Schedule(t, db, class, 2, "18:00") // Tuesday, effective from class start

	rows, err := svc.ListRange(ctx, sc, class.ID, date("2026-01-01"), date("2026-01-31"))
	require.NoError(t, err)
	for _, r := range rows {
		require.False(t, r.SessionDate.Before(class.StartDate), "no session before class start_date")
		require.False(t, r.SessionDate.After(classEnd), "no session after class end_date")
	}
	require.Len(t, rows, 3, "Tuesdays 2026-01-06, 01-13, 01-20 fall within [start_date, end_date]")
}

// todayIn mirrors the service's own cutoff formula (now converted into the
// teacher's calendar day, then re-stamped at UTC midnight) so pending-feed
// boundary tests stay deterministic regardless of the wall-clock hour the
// suite happens to run at, instead of comparing against a hardcoded date.
func todayIn(loc *time.Location) time.Time {
	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func pendingIDs(resp *sessions.PendingResponse) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(resp.Items))
	for _, it := range resp.Items {
		ids = append(ids, it.SessionID)
	}
	return ids
}

func TestListPendingIncludesPastPlannedAndHeldExcludesOthers(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2020-01-01")))

	loc, err := time.LoadLocation(teachers.DefaultTimezone)
	require.NoError(t, err)
	today := todayIn(loc)
	yesterday := today.AddDate(0, 0, -1)

	pastPlanned := testutil.Session(t, db, teacher.ID, class.ID, yesterday)
	pastHeldUnconfirmed := testutil.Session(t, db, teacher.ID, class.ID, yesterday.AddDate(0, 0, -1),
		testutil.WithSessionStatus(sessions.StatusHeld))
	pastConfirmed := testutil.Session(t, db, teacher.ID, class.ID, yesterday.AddDate(0, 0, -2),
		testutil.WithSessionAttendanceConfirmed(time.Now()))
	pastCancelled := testutil.Session(t, db, teacher.ID, class.ID, yesterday.AddDate(0, 0, -3),
		testutil.WithSessionStatus(sessions.StatusCancelled))
	toDelete := testutil.Session(t, db, teacher.ID, class.ID, yesterday.AddDate(0, 0, -4))
	require.NoError(t, svc.Delete(ctx, sc, toDelete.ID))
	todaySession := testutil.Session(t, db, teacher.ID, class.ID, today)

	resp, err := svc.ListPending(ctx, sc, nil, nil, 50)
	require.NoError(t, err)

	require.ElementsMatch(t, []uuid.UUID{pastPlanned.ID, pastHeldUnconfirmed.ID}, pendingIDs(resp),
		"only the unconfirmed past planned/held sessions are pending")
	require.NotContains(t, pendingIDs(resp), pastConfirmed.ID, "confirmed attendance must never be pending")
	require.NotContains(t, pendingIDs(resp), pastCancelled.ID, "a cancelled session is never pending")
	require.NotContains(t, pendingIDs(resp), toDelete.ID, "a soft-deleted session must never resurface")
	require.NotContains(t, pendingIDs(resp), todaySession.ID, "today's own session is not yet overdue")

	// Newest-first ordering and DaysOverdue, computed off the same cutoff.
	require.Equal(t, pastPlanned.ID, resp.Items[0].SessionID)
	require.Equal(t, pastHeldUnconfirmed.ID, resp.Items[1].SessionID)
	require.Equal(t, 1, resp.Items[0].DaysOverdue)
	require.Equal(t, 2, resp.Items[1].DaysOverdue)
}

// TestListPendingRespectsTeacherTimezoneBoundary uses a teacher on a
// deliberately non-default zone (UTC+14, as far from the server/default
// Asia/Ho_Chi_Minh as the tz database allows) and checks the boundary purely
// against that teacher's own today/yesterday. This stays 100% deterministic
// regardless of the hour the suite runs at — comparing two different-zone
// teachers against the same real "now" instant would only manifest a
// difference during certain hours of the day and would be flaky.
func TestListPendingRespectsTeacherTimezoneBoundary(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	const tz = "Pacific/Kiritimati"
	require.NoError(t, db.Exec("UPDATE teachers SET timezone = ? WHERE id = ?", tz, teacher.ID).Error)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2020-01-01")))

	loc, err := time.LoadLocation(tz)
	require.NoError(t, err)
	today := todayIn(loc)
	yesterday := today.AddDate(0, 0, -1)

	pending := testutil.Session(t, db, teacher.ID, class.ID, yesterday)
	notYetPending := testutil.Session(t, db, teacher.ID, class.ID, today)

	resp, err := svc.ListPending(ctx, sc, nil, nil, 50)
	require.NoError(t, err)

	ids := pendingIDs(resp)
	require.Contains(t, ids, pending.ID, "yesterday in the teacher's own timezone must be pending")
	require.NotContains(t, ids, notYetPending.ID, "today in the teacher's own timezone must not be pending yet")
}

func TestListPendingFromToFiltersInclusive(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))

	s1 := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-10"))
	s2 := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-15"))
	s3 := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-20"))

	from := date("2026-01-10")
	to := date("2026-01-15")
	resp, err := svc.ListPending(ctx, sc, &from, &to, 50)
	require.NoError(t, err)

	require.ElementsMatch(t, []uuid.UUID{s1.ID, s2.ID}, pendingIDs(resp), "from and to are both inclusive")
	require.NotContains(t, pendingIDs(resp), s3.ID)
}

func TestListPendingIsTeacherScoped(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacherA, _ := testutil.Teacher(t, db)
	teacherB, _ := testutil.Teacher(t, db)
	scopeA := testutil.ScopeFor(t, db, teacherA.ID)
	classA := testutil.Class(t, db, teacherA.ID, testutil.WithClassStartDate(date("2026-01-01")))
	classB := testutil.Class(t, db, teacherB.ID, testutil.WithClassStartDate(date("2026-01-01")))
	sessA := testutil.Session(t, db, teacherA.ID, classA.ID, date("2026-01-10"))
	sessB := testutil.Session(t, db, teacherB.ID, classB.ID, date("2026-01-10"))

	resp, err := svc.ListPending(ctx, scopeA, nil, nil, 50)
	require.NoError(t, err)

	require.Contains(t, pendingIDs(resp), sessA.ID)
	require.NotContains(t, pendingIDs(resp), sessB.ID, "teacher A must never see teacher B's pending sessions")
}

func TestListPendingTotalCountsAllItemsRespectsLimit(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	for i := 1; i <= 5; i++ {
		testutil.Session(t, db, teacher.ID, class.ID, date(fmt.Sprintf("2026-01-%02d", i)))
	}

	resp, err := svc.ListPending(ctx, sc, nil, nil, 2)
	require.NoError(t, err)
	require.EqualValues(t, 5, resp.Total, "total must count every pending session regardless of limit")
	require.Len(t, resp.Items, 2, "items must respect the limit")
}

func TestListPendingExpectedStudentCountReflectsActiveEnrollmentsOnSessionDate(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))

	contact1 := testutil.Contact(t, db, teacher.ID)
	contact2 := testutil.Contact(t, db, teacher.ID)
	contact3 := testutil.Contact(t, db, teacher.ID)
	student1 := testutil.Student(t, db, teacher.ID, contact1.ID)
	student2 := testutil.Student(t, db, teacher.ID, contact2.ID)
	student3 := testutil.Student(t, db, teacher.ID, contact3.ID)

	sessionDate := date("2026-01-10")
	// Active on the session date: counts.
	testutil.Enrollment(t, db, teacher.ID, student1.ID, class.ID, date("2026-01-01"))
	// Ended before the session date: must not count.
	ended := testutil.Enrollment(t, db, teacher.ID, student2.ID, class.ID, date("2026-01-01"))
	require.NoError(t, db.Model(ended).Update("ended_on", date("2026-01-05")).Error)
	// Starts after the session date: must not count.
	testutil.Enrollment(t, db, teacher.ID, student3.ID, class.ID, date("2026-01-15"))

	session := testutil.Session(t, db, teacher.ID, class.ID, sessionDate)

	resp, err := svc.ListPending(ctx, sc, nil, nil, 50)
	require.NoError(t, err)
	require.Len(t, resp.Items, 1)
	require.Equal(t, session.ID, resp.Items[0].SessionID)
	require.Equal(t, 1, resp.Items[0].ExpectedStudentCount,
		"only the enrollment active on the session's own date counts")

	loc, err := time.LoadLocation(teachers.DefaultTimezone)
	require.NoError(t, err)
	wantDaysOverdue := int(todayIn(loc).Sub(sessionDate).Hours() / 24)
	require.Equal(t, wantDaysOverdue, resp.Items[0].DaysOverdue)
}

// sqlCounter is a minimal gorm logger.Interface that counts every statement
// traced, so a test can assert ListPending stays a bounded, fixed number of
// round trips (one Count, one Find) regardless of how many pending sessions
// or enrollments exist — never a per-row roster lookup.
type sqlCounter struct {
	mu    sync.Mutex
	count int
}

func (c *sqlCounter) LogMode(logger.LogLevel) logger.Interface      { return c }
func (c *sqlCounter) Info(context.Context, string, ...interface{})  {}
func (c *sqlCounter) Warn(context.Context, string, ...interface{})  {}
func (c *sqlCounter) Error(context.Context, string, ...interface{}) {}
func (c *sqlCounter) Trace(_ context.Context, _ time.Time, _ func() (string, int64), _ error) {
	c.mu.Lock()
	c.count++
	c.mu.Unlock()
}

// An owner generates, reads, and manages a member's class sessions — center-
// wide oversight, not per-teacher isolation. Generated and ad-hoc rows are
// stamped with the member's own teacher and center, never the owner's — the
// same rule AddSchedule follows for a schedule added to a member's class.
func TestOwnerHasFullOversightOfMembersSessions(t *testing.T) {
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

	class := testutil.Class(t, db, member.ID, testutil.WithClassStartDate(date("2026-01-01")))
	testutil.Schedule(t, db, class, 2, "18:00") // Tuesday

	rows, err := svc.ListRange(ctx, ownerScope, class.ID, date("2026-01-01"), date("2026-01-31"))
	require.NoError(t, err, "owner must generate sessions for a member's class")
	require.Len(t, rows, 4, "want 4 Tuesdays in January 2026")
	for _, r := range rows {
		require.Equal(t, member.ID, r.TeacherID, "generated rows must be stamped with the member's own teacher id")
		require.Equal(t, ownerCenter, r.CenterID)
	}

	got, err := svc.Get(ctx, ownerScope, rows[0].ID)
	require.NoError(t, err, "owner must read a member's session")
	require.Equal(t, rows[0].ID, got.ID)

	cancelled, err := svc.Cancel(ctx, ownerScope, rows[0].ID, "nghỉ lễ")
	require.NoError(t, err, "owner must cancel a member's session")
	require.Equal(t, sessions.StatusCancelled, cancelled.Status)

	restored, err := svc.Uncancel(ctx, ownerScope, rows[0].ID)
	require.NoError(t, err, "owner must restore a member's cancelled session")
	require.Equal(t, sessions.StatusPlanned, restored.Status)

	created, err := svc.CreateAdHoc(ctx, ownerScope, class.ID, sessions.CreateSessionRequest{
		SessionDate: "2026-01-15",
	})
	require.NoError(t, err, "owner must add an ad-hoc session to a member's class")
	require.Equal(t, member.ID, created.TeacherID, "ad-hoc session on a member's class must be stamped as the member's own")
	require.Equal(t, ownerCenter, created.CenterID)
}

// Two non-owning teachers in the same center are still isolated from each
// other: center scope grants the owner oversight, not peer-to-peer access.
func TestPeersInSameCenterCannotSeeEachOthersSessions(t *testing.T) {
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

	classB := testutil.Class(t, db, memberB.ID, testutil.WithClassStartDate(date("2026-01-01")))
	testutil.Schedule(t, db, classB, 2, "18:00")

	rows, err := svc.ListRange(ctx, scopeB, classB.ID, date("2026-01-01"), date("2026-01-31"))
	require.NoError(t, err)
	require.NotEmpty(t, rows)

	_, err = svc.Get(ctx, scopeC, rows[0].ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code, "a peer must not read another member's session")

	_, err = svc.ListRange(ctx, scopeC, classB.ID, date("2026-01-01"), date("2026-01-31"))
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code, "a peer must not generate sessions for another member's class")
}

// A teacher from a different center is refused on every session operation
// with 404, never 403 — a 403 would confirm the id exists in another center.
func TestCrossCenterSessionsAreNotFound(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacherA, _ := testutil.Teacher(t, db)
	teacherB, _ := testutil.Teacher(t, db)
	scopeA := testutil.ScopeFor(t, db, teacherA.ID)
	scopeB := testutil.ScopeFor(t, db, teacherB.ID)

	class := testutil.Class(t, db, teacherA.ID, testutil.WithClassStartDate(date("2026-01-01")))
	testutil.Schedule(t, db, class, 2, "18:00")
	rows, err := svc.ListRange(ctx, scopeA, class.ID, date("2026-01-01"), date("2026-01-31"))
	require.NoError(t, err)

	_, err = svc.Get(ctx, scopeB, rows[0].ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	_, err = svc.ListRange(ctx, scopeB, class.ID, date("2026-01-01"), date("2026-01-31"))
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code, "teacher B's request against A's class must 404")

	_, err = svc.Cancel(ctx, scopeB, rows[0].ID, "nghỉ lễ")
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	_, err = svc.Uncancel(ctx, scopeB, rows[0].ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	_, err = svc.Hold(ctx, scopeB, rows[0].ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	err = svc.Delete(ctx, scopeB, rows[0].ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	_, err = svc.CreateAdHoc(ctx, scopeB, class.ID, sessions.CreateSessionRequest{SessionDate: "2026-01-15"})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
}

func TestListPendingIssuesBoundedQueryCount(t *testing.T) {
	t.Parallel()
	db := testutil.StartPostgres(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	for i := 1; i <= 3; i++ {
		testutil.Session(t, db, teacher.ID, class.ID, date(fmt.Sprintf("2026-01-%02d", i)))
		for j := 0; j < 3; j++ {
			contact := testutil.Contact(t, db, teacher.ID)
			student := testutil.Student(t, db, teacher.ID, contact.ID)
			testutil.Enrollment(t, db, teacher.ID, student.ID, class.ID, date("2026-01-01"))
		}
	}

	counter := &sqlCounter{}
	repo := sessions.NewRepository(db.Session(&gorm.Session{Logger: counter}))
	before := time.Now().AddDate(0, 0, 1)
	_, total, err := repo.ListPending(ctx, sc, before, nil, nil, 50)
	require.NoError(t, err)
	require.EqualValues(t, 3, total)
	require.Equal(t, 2, counter.count,
		"ListPending must issue exactly one Count and one Find, never a per-row roster lookup")
}
