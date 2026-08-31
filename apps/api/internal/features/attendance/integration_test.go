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
	"teka/apps/api/internal/features/classstaff"
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
	classesSvc := classes.NewService(classes.NewRepository(db), txMgr, classstaff.NewRepository(db))
	teachersSvc := teachers.NewService(teachers.NewRepository(db))
	enrollmentsSvc := enrollments.NewService(enrollments.NewRepository(db), nil)
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
// created records carry the member's center and attribute the owner as the
// last writer — teacher_id on an attendance record names who saved the
// sheet, never a row filter, so pricing and read-back still resolve the
// member's roster in full.
func TestOwnerConfirmsMembersSessionAttendance(t *testing.T) {
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
	require.Equal(t, owner.ID, rows[0].TeacherID,
		"teacher_id is last-writer attribution: the confirming owner takes the credit")
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

// The four-status confirm persists against the real schema (the CHECK now
// admits 'late'), round-trips per-student notes, and — the money invariant —
// leaves the billable tally billing prices from identical to an all-present
// confirm: late and excused are billable exceptions, not discounts.
func TestFourStatusConfirmPersistsAndKeepsBillableTally(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	teacher, _ := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, teacher.ID)
	contact := testutil.Contact(t, db, teacher.ID)
	class := testutil.Class(t, db, teacher.ID, testutil.WithClassStartDate(date("2026-01-01")))
	allPresent := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-06"))
	mixed := testutil.Session(t, db, teacher.ID, class.ID, date("2026-01-13"))

	var enrollmentIDs []uuid.UUID
	var studentIDs []uuid.UUID
	for i := 0; i < 3; i++ {
		s := testutil.Student(t, db, teacher.ID, contact.ID)
		e := testutil.Enrollment(t, db, teacher.ID, s.ID, class.ID, date("2026-01-01"))
		studentIDs = append(studentIDs, s.ID)
		enrollmentIDs = append(enrollmentIDs, e.ID)
	}

	_, err := svc.Confirm(ctx, sc, allPresent.ID, attendance.ConfirmRequest{})
	require.NoError(t, err)
	out, err := svc.Confirm(ctx, sc, mixed.ID, attendance.ConfirmRequest{
		Marks: []attendance.ConfirmMark{
			{StudentID: studentIDs[0], Status: attendance.StatusLate},
			{StudentID: studentIDs[1], Status: attendance.StatusExcused, Note: "mẹ báo ốm"},
		},
	})
	require.NoError(t, err)

	byStudent := map[uuid.UUID]attendance.RowResponse{}
	for _, row := range out.Rows {
		byStudent[row.StudentID] = row
	}
	require.Equal(t, attendance.StatusLate, *byStudent[studentIDs[0]].Status)
	require.Equal(t, attendance.StatusExcused, *byStudent[studentIDs[1]].Status)
	require.Equal(t, "mẹ báo ốm", *byStudent[studentIDs[1]].Note)
	require.Equal(t, attendance.StatusPresent, *byStudent[studentIDs[2]].Status)

	var billableStatuses []string
	require.NoError(t, db.Table("attendance_records").
		Where("session_id = ? AND deleted_at IS NULL AND billable = true", mixed.ID).
		Order("status").Pluck("status", &billableStatuses).Error)
	require.Equal(t, []string{"excused", "late", "present"}, billableStatuses,
		"every status must persist as a billable row")

	tallies, err := svc.TallyByEnrollment(ctx, sc, date("2026-01-01"), date("2026-01-31"))
	require.NoError(t, err)
	require.Len(t, tallies, 3)
	// Attendance semantics on the reporting counts: late is present (the
	// student attended), excused is an absence with a reason — so present +
	// absent covers every recorded session and the parent-facing "buổi vắng"
	// line explains what the billable row charges for.
	type counts struct{ present, absent int }
	want := map[uuid.UUID]counts{
		enrollmentIDs[0]: {present: 2, absent: 0}, // present + late
		enrollmentIDs[1]: {present: 1, absent: 1}, // present + excused
		enrollmentIDs[2]: {present: 2, absent: 0}, // present + present
	}
	for _, tally := range tallies {
		require.Equalf(t, 2, tally.BillableCount,
			"enrollment %s: late/excused must bill exactly like present", tally.EnrollmentID)
		w := want[tally.EnrollmentID]
		require.Equalf(t, w.present, tally.PresentCount,
			"enrollment %s: late must count as present", tally.EnrollmentID)
		require.Equalf(t, w.absent, tally.AbsentCount,
			"enrollment %s: excused must count as an absence", tally.EnrollmentID)
	}
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

// After a class handoff the new teacher owns the class and its future
// sessions, but the enrollment and student rows stay with their creator. The
// attendance sheet must still work end to end for the new teacher: roster
// resolved through the handed-off class, student names included, and confirm
// writing records under the session's own (new) teacher.
func TestHandedOffClassAttendanceSheetWorksForNewTeacher(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, db)
	newTeacher, _ := testutil.Teacher(t, db)
	ownerCenter := testutil.ScopeFor(t, db, owner.ID).CenterID

	testutil.JoinCenter(t, db, newTeacher.ID, ownerCenter)
	newTeacherScope := testutil.ScopeFor(t, db, newTeacher.ID)

	// The owner built the class, roster, and a planned session…
	contact := testutil.Contact(t, db, owner.ID)
	class := testutil.Class(t, db, owner.ID, testutil.WithClassStartDate(date("2026-01-01")))
	session := testutil.Session(t, db, owner.ID, class.ID, date("2026-01-06"))
	student := testutil.Student(t, db, owner.ID, contact.ID, testutil.WithStudentFullName("Bé Bình"))
	testutil.Enrollment(t, db, owner.ID, student.ID, class.ID, date("2026-01-01"))

	// …then handed the class over: class, future sessions, and the giao_vien
	// stint move (the previous stint closes, the new teacher opens theirs);
	// the enrollment and student rows do not (mirrors handoff's writes).
	require.NoError(t, db.Exec(
		"UPDATE classes SET teacher_id = ? WHERE id = ?", newTeacher.ID, class.ID).Error)
	require.NoError(t, db.Exec(
		"UPDATE class_sessions SET teacher_id = ? WHERE id = ?", newTeacher.ID, session.ID).Error)
	require.NoError(t, db.Exec(
		"UPDATE class_staff SET ended_at = now() WHERE class_id = ? AND role_key = 'giao_vien' AND ended_at IS NULL",
		class.ID).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO class_staff (class_id, center_id, teacher_id, role_key) VALUES (?, ?, ?, 'giao_vien')",
		class.ID, ownerCenter, newTeacher.ID).Error)

	got, err := svc.Get(ctx, newTeacherScope, session.ID)
	require.NoError(t, err, "the new teacher must read the handed-off session's sheet")
	require.Len(t, got.Rows, 1)
	require.Equal(t, "Bé Bình", got.Rows[0].StudentName,
		"the owner-created student's name must resolve on the new teacher's sheet")

	out, err := svc.Confirm(ctx, newTeacherScope, session.ID, attendance.ConfirmRequest{})
	require.NoError(t, err, "the new teacher must confirm attendance on their handed-off class")
	require.Len(t, out.Rows, 1)
	require.NotNil(t, out.Rows[0].Status)
	require.Equal(t, attendance.StatusPresent, *out.Rows[0].Status)
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

// The attendance write capability admits the trợ giảng alongside the giáo
// viên: recording who showed up is exactly the assistant's job. Hoc_vu reads
// the sheet's class but cannot confirm (403); a member with no stint gets 404.
// Every record row is attributed to whoever actually wrote it, and the billing
// tally keys on the enrollment — an assistant-recorded session still bills to
// the roster owner's invoice.
func TestConfirmCapabilityGateAndAttribution(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, db)
	ownerSc := testutil.ScopeFor(t, db, owner.ID)
	_, gv := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, gv.ID, ownerSc.CenterID)
	_, tg := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, tg.ID, ownerSc.CenterID)
	tgSc := testutil.ScopeFor(t, db, tg.ID)
	_, hv := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, hv.ID, ownerSc.CenterID)
	hvSc := testutil.ScopeFor(t, db, hv.ID)
	_, outsider := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, outsider.ID, ownerSc.CenterID)
	outsiderSc := testutil.ScopeFor(t, db, outsider.ID)

	contact := testutil.Contact(t, db, owner.ID)
	class := testutil.Class(t, db, gv.ID, testutil.WithClassStartDate(date("2026-01-01")))
	testutil.StaffAssignment(t, db, class, tg.ID, "tro_giang")
	testutil.StaffAssignment(t, db, class, hv.ID, "hoc_vu")
	session := testutil.Session(t, db, gv.ID, class.ID, date("2026-01-06"))
	student := testutil.Student(t, db, owner.ID, contact.ID)
	testutil.Enrollment(t, db, owner.ID, student.ID, class.ID, date("2026-01-01"))

	_, err := svc.Confirm(ctx, hvSc, session.ID, attendance.ConfirmRequest{})
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code,
		"hoc_vu reads the class, so the attendance denial is an honest 403")
	_, err = svc.Confirm(ctx, outsiderSc, session.ID, attendance.ConfirmRequest{})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)

	out, err := svc.Confirm(ctx, tgSc, session.ID, attendance.ConfirmRequest{})
	require.NoError(t, err, "trợ giảng holds the attendance write capability")
	require.Len(t, out.Rows, 1)

	// Attribution: the record names the assistant who wrote it, not the
	// session's or enrollment's teacher.
	var recordedBy string
	require.NoError(t, db.Raw(
		"SELECT teacher_id::text FROM attendance_records WHERE session_id = ? AND deleted_at IS NULL",
		session.ID).Scan(&recordedBy).Error)
	require.Equal(t, tg.ID.String(), recordedBy,
		"attendance_records.teacher_id is last-writer attribution")

	// A re-confirm by the giáo viên re-attributes the surviving row.
	gvSc := testutil.ScopeFor(t, db, gv.ID)
	_, err = svc.Confirm(ctx, gvSc, session.ID, attendance.ConfirmRequest{})
	require.NoError(t, err)
	require.NoError(t, db.Raw(
		"SELECT teacher_id::text FROM attendance_records WHERE session_id = ? AND deleted_at IS NULL",
		session.ID).Scan(&recordedBy).Error)
	require.Equal(t, gv.ID.String(), recordedBy, "an update re-attributes to the new writer")

	// Billing keys on the enrollment, not the record's writer: the roster
	// owner's tally counts the assistant/GV-recorded session.
	tallies, err := svc.TallyByEnrollment(ctx, ownerSc, date("2026-01-01"), date("2026-01-31"))
	require.NoError(t, err)
	found := false
	for _, tl := range tallies {
		if tl.BillableCount == 1 && tl.PresentCount == 1 {
			found = true
		}
	}
	require.True(t, found,
		"staff-recorded attendance must still land on the enrollment owner's billing tally")
}

// A re-confirm after handoff must hit the SAME billable rows, not add a
// second set: the partial unique keys on (session_id, student_id) with no
// teacher column, so the new giáo viên's confirm updates the old teacher's
// rows in place — one live billable row per student, attributed to the new
// writer, and one billable count on the tally.
func TestHandoffReconfirmDoesNotDuplicateBillableRows(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	owner, _ := testutil.Teacher(t, db)
	ownerSc := testutil.ScopeFor(t, db, owner.ID)
	_, oldGV := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, oldGV.ID, ownerSc.CenterID)
	oldSc := testutil.ScopeFor(t, db, oldGV.ID)
	_, newGV := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, newGV.ID, ownerSc.CenterID)
	newSc := testutil.ScopeFor(t, db, newGV.ID)

	contact := testutil.Contact(t, db, owner.ID)
	class := testutil.Class(t, db, oldGV.ID, testutil.WithClassStartDate(date("2026-01-01")))
	session := testutil.Session(t, db, oldGV.ID, class.ID, date("2026-01-06"))
	student := testutil.Student(t, db, owner.ID, contact.ID)
	testutil.Enrollment(t, db, owner.ID, student.ID, class.ID, date("2026-01-01"))

	_, err := svc.Confirm(ctx, oldSc, session.ID, attendance.ConfirmRequest{})
	require.NoError(t, err, "pre-handoff confirm by the original teacher")

	require.NoError(t, db.Exec(
		"UPDATE classes SET teacher_id = ? WHERE id = ?", newGV.ID, class.ID).Error)
	require.NoError(t, db.Exec(
		"UPDATE class_sessions SET teacher_id = ? WHERE id = ?", newGV.ID, session.ID).Error)
	require.NoError(t, db.Exec(
		"UPDATE class_staff SET ended_at = now() WHERE class_id = ? AND role_key = 'giao_vien' AND ended_at IS NULL",
		class.ID).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO class_staff (class_id, center_id, teacher_id, role_key) VALUES (?, ?, ?, 'giao_vien')",
		class.ID, ownerSc.CenterID, newGV.ID).Error)

	_, err = svc.Confirm(ctx, newSc, session.ID, attendance.ConfirmRequest{AbsentStudentIDs: []uuid.UUID{student.ID}})
	require.NoError(t, err, "post-handoff re-confirm by the new teacher")

	var live int64
	require.NoError(t, db.Raw(
		"SELECT count(*) FROM attendance_records WHERE session_id = ? AND deleted_at IS NULL",
		session.ID).Scan(&live).Error)
	require.EqualValues(t, 1, live,
		"the re-confirm must update the existing row, never add a parallel billable one")

	var writer string
	require.NoError(t, db.Raw(
		"SELECT teacher_id::text FROM attendance_records WHERE session_id = ? AND deleted_at IS NULL",
		session.ID).Scan(&writer).Error)
	require.Equal(t, newGV.ID.String(), writer)

	tallies, err := svc.TallyByEnrollment(ctx, ownerSc, date("2026-01-01"), date("2026-01-31"))
	require.NoError(t, err)
	var billable, absent int
	for _, tl := range tallies {
		billable += tl.BillableCount
		absent += tl.AbsentCount
	}
	require.Equal(t, 1, billable, "one session, one billable unit — regardless of who recorded it")
	require.Equal(t, 1, absent, "the re-confirm's absent flag replaced the old present flag")
}
