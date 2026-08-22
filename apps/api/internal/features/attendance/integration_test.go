//go:build integration

package attendance_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/attendance"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/testutil"
)

// newIntegrationService wires the real dependency chain router.go uses:
// attendance consumes enrollments (roster) and sessions (session lookup +
// held/confirmed transition) through its consumer interfaces.
func newIntegrationService(t *testing.T) (*attendance.Service, *gorm.DB) {
	t.Helper()
	db := testutil.StartPostgres(t)
	txMgr := database.NewTxManager(db)
	classesSvc := classes.NewService(classes.NewRepository(db), txMgr)
	teachersSvc := teachers.NewService(teachers.NewRepository(db))
	enrollmentsSvc := enrollments.NewService(enrollments.NewRepository(db))
	sessionsSvc := sessions.NewService(sessions.NewRepository(db), classesSvc, teachersSvc, enrollmentsSvc)
	svc := attendance.NewService(attendance.NewRepository(db), enrollmentsSvc, sessionsSvc, txMgr)
	return svc, db
}

func date(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

// An owner reads and confirms a member's session attendance sheet; the
// created records are stamped with the MEMBER's own teacher and center, never
// the owner's — the same precedent sessions' generated/ad-hoc rows follow.
func TestOwnerConfirmsMembersSessionAttendanceRecordsInheritMembersAnchors(t *testing.T) {
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
	class := testutil.Class(t, db, member.ID, testutil.WithClassStartDate(date("2026-01-01")))
	session := testutil.Session(t, db, member.ID, class.ID, date("2026-01-06"))
	student := testutil.Student(t, db, member.ID, contact.ID)
	testutil.Enrollment(t, db, member.ID, student.ID, class.ID, date("2026-01-01"))

	got, err := svc.Get(ctx, ownerScope, session.ID)
	require.NoError(t, err, "owner must read a member's attendance sheet")
	require.Len(t, got.Rows, 1)

	out, err := svc.Confirm(ctx, ownerScope, session.ID, attendance.ConfirmRequest{})
	require.NoError(t, err, "owner must confirm a member's session attendance")
	require.Equal(t, sessions.StatusHeld, out.Status)
	// The sheet the owner reads back must surface the member's recorded rows,
	// not come back blank because the read path filtered them to the owner's
	// own teacher id.
	require.Len(t, out.Rows, 1)
	require.NotNil(t, out.Rows[0].Status, "owner's read-back must include the recorded status")
	require.Equal(t, attendance.StatusPresent, *out.Rows[0].Status)
	require.NotEmpty(t, out.Rows[0].StudentName, "owner's read-back must resolve the member's student name")

	var rows []struct {
		TeacherID uuid.UUID
		CenterID  uuid.UUID
	}
	require.NoError(t, db.Table("attendance_records").
		Where("session_id = ? AND deleted_at IS NULL", session.ID).Find(&rows).Error)
	require.Len(t, rows, 1)
	require.Equal(t, member.ID, rows[0].TeacherID,
		"records must be stamped with the member's own teacher id, not the confirming owner's")
	require.Equal(t, ownerCenter, rows[0].CenterID)
}

// Two non-owning teachers in the same center are still isolated from each
// other's session attendance — center scope grants the owner oversight, not
// peer-to-peer access.
func TestPeersInSameCenterCannotSeeEachOthersAttendance(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, db)
	memberB, _ := testutil.Teacher(t, db)
	memberC, _ := testutil.Teacher(t, db)
	ownerCenter := testutil.ScopeFor(t, db, owner.ID).CenterID

	testutil.JoinCenter(t, db, memberB.ID, ownerCenter)
	testutil.JoinCenter(t, db, memberC.ID, ownerCenter)
	scopeC := testutil.ScopeFor(t, db, memberC.ID)

	contact := testutil.Contact(t, db, memberB.ID)
	class := testutil.Class(t, db, memberB.ID, testutil.WithClassStartDate(date("2026-01-01")))
	session := testutil.Session(t, db, memberB.ID, class.ID, date("2026-01-06"))
	student := testutil.Student(t, db, memberB.ID, contact.ID)
	testutil.Enrollment(t, db, memberB.ID, student.ID, class.ID, date("2026-01-01"))

	_, err := svc.Get(ctx, scopeC, session.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code, "a peer must not read another member's attendance sheet")

	_, err = svc.Confirm(ctx, scopeC, session.ID, attendance.ConfirmRequest{})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code, "a peer must not confirm another member's session attendance")
}

// A teacher from a different center is refused with 404, never 403 — a 403
// would confirm the session exists in another center.
func TestCrossCenterSessionAttendanceIsNotFound(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacherA, _ := testutil.Teacher(t, db)
	teacherB, _ := testutil.Teacher(t, db)
	scopeA := testutil.ScopeFor(t, db, teacherA.ID)
	scopeB := testutil.ScopeFor(t, db, teacherB.ID)

	contact := testutil.Contact(t, db, teacherA.ID)
	class := testutil.Class(t, db, teacherA.ID, testutil.WithClassStartDate(date("2026-01-01")))
	session := testutil.Session(t, db, teacherA.ID, class.ID, date("2026-01-06"))
	student := testutil.Student(t, db, teacherA.ID, contact.ID)
	testutil.Enrollment(t, db, teacherA.ID, student.ID, class.ID, date("2026-01-01"))

	_, err := svc.Get(ctx, scopeA, session.ID)
	require.NoError(t, err, "teacher A must read their own attendance sheet")

	_, err = svc.Get(ctx, scopeB, session.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	_, err = svc.Confirm(ctx, scopeB, session.ID, attendance.ConfirmRequest{})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
}

func TestConfirmWritesOneRecordPerRosterStudentInOneCall(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	session := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-06"))

	const total = 30
	studentIDs := make([]uuid.UUID, 0, total)
	for i := 0; i < total; i++ {
		s := testutil.Student(t, db, teacher.ID, contact.ID)
		testutil.Enrollment(t, db, teacher.ID, s.ID, class.ID, date("2026-01-01"))
		studentIDs = append(studentIDs, s.ID)
	}
	absent := []uuid.UUID{studentIDs[0], studentIDs[1]}

	out, err := svc.Confirm(ctx, sc, session.ID, attendance.ConfirmRequest{AbsentStudentIDs: absent})
	require.NoError(t, err)
	require.Len(t, out.Rows, total, "one HTTP call must write every roster student's row")

	var count int64
	require.NoError(t, db.Table("attendance_records").
		Where("session_id = ? AND deleted_at IS NULL", session.ID).Count(&count).Error)
	require.EqualValues(t, total, count)

	absentCount := 0
	for _, row := range out.Rows {
		require.NotNil(t, row.Status)
		if *row.Status == attendance.StatusAbsent {
			absentCount++
		}
	}
	require.Equal(t, 2, absentCount)
}

func TestReConfirmIsIdempotentAndPreservesRecordedAt(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	session := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-06"))
	s1 := testutil.Student(t, db, teacher.ID, contact.ID)
	s2 := testutil.Student(t, db, teacher.ID, contact.ID)
	testutil.Enrollment(t, db, teacher.ID, s1.ID, class.ID, date("2026-01-01"))
	testutil.Enrollment(t, db, teacher.ID, s2.ID, class.ID, date("2026-01-01"))

	_, err := svc.Confirm(ctx, sc, session.ID, attendance.ConfirmRequest{AbsentStudentIDs: []uuid.UUID{s1.ID}})
	require.NoError(t, err)

	type recordRow struct {
		ID         uuid.UUID
		StudentID  uuid.UUID
		RecordedAt time.Time
		UpdatedAt  time.Time
	}
	var firstRows []recordRow
	require.NoError(t, db.Table("attendance_records").
		Where("session_id = ? AND deleted_at IS NULL", session.ID).
		Order("student_id").Find(&firstRows).Error)
	require.Len(t, firstRows, 2)

	time.Sleep(10 * time.Millisecond) // ensure a measurable updated_at delta

	second, err := svc.Confirm(ctx, sc, session.ID, attendance.ConfirmRequest{AbsentStudentIDs: []uuid.UUID{s2.ID}})
	require.NoError(t, err)
	require.Len(t, second.Rows, 2)

	var secondRows []recordRow
	require.NoError(t, db.Table("attendance_records").
		Where("session_id = ? AND deleted_at IS NULL", session.ID).
		Order("student_id").Find(&secondRows).Error)
	require.Len(t, secondRows, 2)

	byID := make(map[uuid.UUID]recordRow, len(firstRows))
	for _, r := range firstRows {
		byID[r.StudentID] = r
	}
	for _, r := range secondRows {
		first, ok := byID[r.StudentID]
		require.True(t, ok)
		require.Equal(t, first.ID, r.ID, "record ids must stay stable across re-confirm")
		require.True(t, first.RecordedAt.Equal(r.RecordedAt), "recorded_at must be preserved across re-confirm")
		require.True(t, r.UpdatedAt.After(first.UpdatedAt), "updated_at must advance on re-confirm")
	}
}

func TestConfirmOnlyIncludesRosterActiveOnSessionDate(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	session := testutil.Session(t, db, teacher.ID, class.ID, date("2026-03-14"))

	startsOnDate := testutil.Student(t, db, teacher.ID, contact.ID, testutil.WithStudentFullName("Starts On Date"))
	testutil.Enrollment(t, db, teacher.ID, startsOnDate.ID, class.ID, date("2026-03-14"))

	joinsAfter := testutil.Student(t, db, teacher.ID, contact.ID, testutil.WithStudentFullName("Joins After"))
	testutil.Enrollment(t, db, teacher.ID, joinsAfter.ID, class.ID, date("2026-03-15"))

	leavesBefore := testutil.Student(t, db, teacher.ID, contact.ID, testutil.WithStudentFullName("Leaves Before"))
	leftEnrollment := testutil.Enrollment(t, db, teacher.ID, leavesBefore.ID, class.ID, date("2026-01-01"))
	require.NoError(t, db.Model(leftEnrollment).Update("ended_on", date("2026-03-13")).Error)

	out, err := svc.Confirm(ctx, sc, session.ID, attendance.ConfirmRequest{})
	require.NoError(t, err)
	require.Len(t, out.Rows, 1, "only the student active on the session date belongs on the sheet")
	require.Equal(t, startsOnDate.ID, out.Rows[0].StudentID)
}

// TestConcurrentConfirmsProduceExactlyOneRecordPerStudent empirically verifies
// that clause.OnConflict{TargetWhere: "deleted_at IS NULL"} targets the
// partial unique index uq_attendance_records: two goroutines racing to
// confirm the same session must leave exactly one row per roster student, not
// a constraint-violation error and not duplicate rows.
func TestConcurrentConfirmsProduceExactlyOneRecordPerStudent(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	session := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-06"))

	const total = 10
	for i := 0; i < total; i++ {
		s := testutil.Student(t, db, teacher.ID, contact.ID)
		testutil.Enrollment(t, db, teacher.ID, s.ID, class.ID, date("2026-01-01"))
	}

	const goroutines = 6
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	for i := range goroutines {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := svc.Confirm(ctx, sc, session.ID, attendance.ConfirmRequest{})
			errs[i] = err
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		require.NoError(t, err, "goroutine %d", i)
	}

	var count int64
	require.NoError(t, db.Table("attendance_records").
		Where("session_id = ? AND deleted_at IS NULL", session.ID).Count(&count).Error)
	require.EqualValues(t, total, count, "concurrent confirms must not duplicate a student's record")
}

func TestRemovedFromRosterStudentIsSoftDeletedAbsentStudentNeverIs(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	session := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-06"))

	sc := testutil.ScopeFor(t, db, teacher.ID)
	stays := testutil.Student(t, db, teacher.ID, contact.ID, testutil.WithStudentFullName("Stays Absent"))
	leaves := testutil.Student(t, db, teacher.ID, contact.ID, testutil.WithStudentFullName("Leaves Roster"))
	testutil.Enrollment(t, db, teacher.ID, stays.ID, class.ID, date("2026-01-01"))
	leaveEnrollment := testutil.Enrollment(t, db, teacher.ID, leaves.ID, class.ID, date("2026-01-01"))

	_, err := svc.Confirm(ctx, sc, session.ID, attendance.ConfirmRequest{AbsentStudentIDs: []uuid.UUID{stays.ID}})
	require.NoError(t, err)

	// The student leaves the roster before the session date, so the next
	// confirm no longer sees them active.
	require.NoError(t, db.Model(leaveEnrollment).Update("ended_on", date("2026-01-05")).Error)

	_, err = svc.Confirm(ctx, sc, session.ID, attendance.ConfirmRequest{AbsentStudentIDs: []uuid.UUID{stays.ID}})
	require.NoError(t, err)

	type recordRow struct {
		StudentID uuid.UUID
		DeletedAt *time.Time
	}
	var rows []recordRow
	require.NoError(t, db.Unscoped().Table("attendance_records").
		Where("session_id = ?", session.ID).Find(&rows).Error)
	byStudent := make(map[uuid.UUID]*time.Time, len(rows))
	for _, r := range rows {
		byStudent[r.StudentID] = r.DeletedAt
	}
	require.Nil(t, byStudent[stays.ID], "an absent student must never be soft-deleted")
	require.NotNil(t, byStudent[leaves.ID], "a student removed from the roster must be soft-deleted")
}

func TestConfirmSetsSessionHeldAndConfirmedAt(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	session := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-06"))
	student := testutil.Student(t, db, teacher.ID, contact.ID)
	testutil.Enrollment(t, db, teacher.ID, student.ID, class.ID, date("2026-01-01"))

	sc := testutil.ScopeFor(t, db, teacher.ID)
	out, err := svc.Confirm(ctx, sc, session.ID, attendance.ConfirmRequest{})
	require.NoError(t, err)
	require.Equal(t, sessions.StatusHeld, out.Status)
	require.NotNil(t, out.AttendanceConfirmedAt)

	var row struct {
		Status                string
		AttendanceConfirmedAt *time.Time
	}
	require.NoError(t, db.Table("class_sessions").
		Where("id = ?", session.ID).Find(&row).Error)
	require.Equal(t, "held", row.Status)
	require.NotNil(t, row.AttendanceConfirmedAt)
}

func TestConfirmCancelledSessionIs409(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	session := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-06"),
		testutil.WithSessionStatus(sessions.StatusCancelled), testutil.WithSessionCancelReason("nghỉ lễ"))
	student := testutil.Student(t, db, teacher.ID, contact.ID)
	testutil.Enrollment(t, db, teacher.ID, student.ID, class.ID, date("2026-01-01"))

	sc := testutil.ScopeFor(t, db, teacher.ID)
	_, err := svc.Confirm(ctx, sc, session.ID, attendance.ConfirmRequest{})
	require.ErrorIs(t, err, attendance.ErrSessionCancelled)
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)
}

func TestConfirmRejectsAbsentIDOutsideRosterAndEmptyMeansPresent(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	session := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-06"))
	student := testutil.Student(t, db, teacher.ID, contact.ID)
	testutil.Enrollment(t, db, teacher.ID, student.ID, class.ID, date("2026-01-01"))

	sc := testutil.ScopeFor(t, db, teacher.ID)
	_, err := svc.Confirm(ctx, sc, session.ID,
		attendance.ConfirmRequest{AbsentStudentIDs: []uuid.UUID{uuid.New()}})
	require.ErrorIs(t, err, attendance.ErrStudentNotEnrolled)
	appErr := apperror.From(err)
	require.Equal(t, apperror.CodeValidation, appErr.Code)
	require.NotEmpty(t, appErr.Fields["absent_student_ids"])

	out, err := svc.Confirm(ctx, sc, session.ID, attendance.ConfirmRequest{})
	require.NoError(t, err)
	require.Len(t, out.Rows, 1)
	require.NotNil(t, out.Rows[0].Status)
	require.Equal(t, attendance.StatusPresent, *out.Rows[0].Status)
}

func TestCrossTenantAttendanceIsNotFound(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacherA, _ := testutil.Teacher(t, db)
	teacherB, _ := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacherA.ID)
	class := testutil.Class(t, db, teacherA.ID, testutil.WithClassStartDate(date("2026-01-01")))
	session := testutil.Session(t, db, teacherA.ID, class.ID, date("2026-01-06"))
	student := testutil.Student(t, db, teacherA.ID, contact.ID)
	testutil.Enrollment(t, db, teacherA.ID, student.ID, class.ID, date("2026-01-01"))

	scopeB := testutil.ScopeFor(t, db, teacherB.ID)
	_, err := svc.Get(ctx, scopeB, session.ID)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	_, err = svc.Confirm(ctx, scopeB, session.ID, attendance.ConfirmRequest{})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
}

// TestAnonymisedStudentNameStaysReadableOnHistoricalSheet proves
// StudentNames's join deliberately skips the deleted_at filter: students.Delete
// (students/repository.go's AnonymizeAndDelete) stamps both full_name and
// deleted_at in the same update, so a plain GORM model query would make an
// anonymised student's history vanish instead of showing the placeholder name.
func TestAnonymisedStudentNameStaysReadableOnHistoricalSheet(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	session := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-06"))
	student := testutil.Student(t, db, teacher.ID, contact.ID, testutil.WithStudentFullName("Bé An"))
	testutil.Enrollment(t, db, teacher.ID, student.ID, class.ID, date("2026-01-01"))

	sc := testutil.ScopeFor(t, db, teacher.ID)
	_, err := svc.Confirm(ctx, sc, session.ID, attendance.ConfirmRequest{})
	require.NoError(t, err)

	// Mirror students.AnonymizeAndDelete's exact effect directly (this package
	// must not depend on the students feature): full_name scrubbed to the
	// placeholder, display_note cleared, deleted_at stamped, all at once.
	const anonymizedName = "Đã xoá"
	require.NoError(t, db.Exec(
		`UPDATE students SET full_name = ?, display_note = NULL, anonymized_at = now(), deleted_at = now() WHERE id = ?`,
		anonymizedName, student.ID).Error)

	names, err := attendance.NewRepository(db).StudentNames(ctx, sc, []uuid.UUID{student.ID})
	require.NoError(t, err)
	require.Equal(t, anonymizedName, names[student.ID].FullName,
		"an anonymised student's name must still resolve on a historical attendance sheet, not vanish")
}

// sqlCapture is a minimal gorm logger.Interface that records the last
// statement traced, so a test can assert on the exact SQL a repository method
// emits without executing against a real connection twice.
type sqlCapture struct {
	mu   sync.Mutex
	last string
}

func (c *sqlCapture) LogMode(logger.LogLevel) logger.Interface      { return c }
func (c *sqlCapture) Info(context.Context, string, ...interface{})  {}
func (c *sqlCapture) Warn(context.Context, string, ...interface{})  {}
func (c *sqlCapture) Error(context.Context, string, ...interface{}) {}
func (c *sqlCapture) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	c.mu.Lock()
	c.last = sql
	c.mu.Unlock()
}

func (c *sqlCapture) SQL() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.last
}

// TestUpsertManyTargetsPartialUniqueIndex verifies, by inspecting the actual
// SQL gorm emits, that UpsertMany's clause.OnConflict reproduces
// uq_attendance_records's exact predicate (WHERE deleted_at IS NULL) as its
// conflict target — without it Postgres cannot match the partial index and
// every re-confirm would fail outright instead of updating in place.
func TestUpsertManyTargetsPartialUniqueIndex(t *testing.T) {
	t.Parallel()
	db := testutil.StartPostgres(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	session := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-06"))
	student := testutil.Student(t, db, teacher.ID, contact.ID)
	enrollment := testutil.Enrollment(t, db, teacher.ID, student.ID, class.ID, date("2026-01-01"))

	capture := &sqlCapture{}
	repo := attendance.NewRepository(db.Session(&gorm.Session{Logger: capture}))

	sc := testutil.ScopeFor(t, db, teacher.ID)
	now := time.Now()
	err := repo.UpsertMany(ctx, []attendance.Record{{
		ID:           id.New(),
		TeacherID:    teacher.ID,
		CenterID:     sc.CenterID,
		SessionID:    session.ID,
		StudentID:    student.ID,
		EnrollmentID: enrollment.ID,
		Status:       attendance.StatusPresent,
		Billable:     true,
		RecordedAt:   now,
		UpdatedAt:    now,
	}})
	require.NoError(t, err)

	sql := strings.ToLower(capture.SQL())
	require.Contains(t, sql, "on conflict")
	require.Contains(t, sql, "session_id")
	require.Contains(t, sql, "student_id")
	require.Contains(t, sql, "where deleted_at is null")
	require.Contains(t, sql, "do update")
}
