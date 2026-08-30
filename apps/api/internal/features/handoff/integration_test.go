//go:build integration

package handoff_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/centers"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/classstaff"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/handoff"
	"teka/apps/api/internal/features/imports"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/testutil"
)

// fixture is one center with an owner and two member teachers: `from` currently
// teaches the class, `to` is the intended new teacher.
type fixture struct {
	svc      *handoff.Service
	db       *gorm.DB
	owner    authctx.Scope
	from, to uuid.UUID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	db := testutil.StartPostgres(t)
	txMgr := database.NewTxManager(db)

	centersSvc := centers.NewService(centers.NewRepository(db), txMgr, nil)
	classesSvc := classes.NewService(classes.NewRepository(db), txMgr, classstaff.NewRepository(db))
	enrollmentsSvc := enrollments.NewService(enrollments.NewRepository(db), nil)
	teachersSvc := teachers.NewService(teachers.NewRepository(db))
	sessionsSvc := sessions.NewService(sessions.NewRepository(db), classesSvc, teachersSvc, enrollmentsSvc)

	_, ownerTeacher := testutil.Teacher(t, db)
	owner := testutil.ScopeFor(t, db, ownerTeacher.ID)
	require.True(t, owner.IsOwner)

	_, from := testutil.Teacher(t, db)
	_, to := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, from.ID, owner.CenterID)
	testutil.JoinCenter(t, db, to.ID, owner.CenterID)

	svc := handoff.NewService(classesSvc, sessionsSvc, centersSvc, classstaff.NewRepository(db), imports.NewLocker(db), txMgr)
	return fixture{svc: svc, db: db, owner: owner, from: from.ID, to: to.ID}
}

// seedClass creates a class under `from` with one schedule row.
func (f fixture) seedClass(t *testing.T) *classes.Class {
	t.Helper()
	class := testutil.Class(t, f.db, f.from)
	testutil.Schedule(t, f.db, class, 1, "18:00")
	return class
}

// teacherOfClass reads the class's current teacher_id.
func (f fixture) teacherOfClass(t *testing.T, classID uuid.UUID) uuid.UUID {
	t.Helper()
	var row struct{ TeacherID uuid.UUID }
	require.NoError(t, f.db.Raw(
		"SELECT teacher_id FROM classes WHERE id = ?", classID).Scan(&row).Error)
	return row.TeacherID
}

// teacherOfSession reads one session's current teacher_id.
func (f fixture) teacherOfSession(t *testing.T, sessionID uuid.UUID) uuid.UUID {
	t.Helper()
	var row struct{ TeacherID uuid.UUID }
	require.NoError(t, f.db.Raw(
		"SELECT teacher_id FROM class_sessions WHERE id = ?", sessionID).Scan(&row).Error)
	return row.TeacherID
}

func TestReassignMovesClassScheduleAndFuturePlannedOnly(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	class := f.seedClass(t)

	// The boundary is today in the class teacher's timezone (the default
	// Asia/Ho_Chi_Minh), matching Service.ReassignPlanned — not UTC's calendar
	// day, which would drift past the seeded dates in the early-morning-UTC
	// window and make this test time-of-day flaky.
	loc, err := time.LoadLocation(teachers.DefaultTimezone)
	require.NoError(t, err)
	nowLocal := time.Now().In(loc)
	today := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, time.UTC)
	// Sessions the handoff must MOVE: planned, dated today or later.
	todayPlanned := testutil.Session(t, f.db, f.from, class.ID, today)
	futurePlanned := testutil.Session(t, f.db, f.from, class.ID, today.AddDate(0, 0, 7))
	// Sessions the handoff must LEAVE with the old teacher.
	pastPlanned := testutil.Session(t, f.db, f.from, class.ID, today.AddDate(0, 0, -7))
	futureHeld := testutil.Session(t, f.db, f.from, class.ID, today.AddDate(0, 0, 3),
		testutil.WithSessionStatus(sessions.StatusHeld))
	futureCancelled := testutil.Session(t, f.db, f.from, class.ID, today.AddDate(0, 0, 4),
		testutil.WithSessionStatus(sessions.StatusCancelled))

	res, err := f.svc.Reassign(context.Background(), f.owner, class.ID, f.to)
	require.NoError(t, err)
	require.Equal(t, class.ID, res.ClassID)
	require.Equal(t, f.to, res.TeacherID)
	require.Equal(t, int64(2), res.MovedPlannedSessions, "today's and the future planned session move")

	// The class and its schedule rows now belong to the new teacher.
	require.Equal(t, f.to, f.teacherOfClass(t, class.ID))
	var schedule struct{ TeacherID uuid.UUID }
	require.NoError(t, f.db.Raw(
		"SELECT teacher_id FROM class_schedules WHERE class_id = ?", class.ID).Scan(&schedule).Error)
	require.Equal(t, f.to, schedule.TeacherID)

	// Future and today's planned sessions moved.
	require.Equal(t, f.to, f.teacherOfSession(t, todayPlanned.ID))
	require.Equal(t, f.to, f.teacherOfSession(t, futurePlanned.ID))

	// History stays with the old teacher: past planned, held, and cancelled.
	require.Equal(t, f.from, f.teacherOfSession(t, pastPlanned.ID))
	require.Equal(t, f.from, f.teacherOfSession(t, futureHeld.ID))
	require.Equal(t, f.from, f.teacherOfSession(t, futureCancelled.ID))
}

func TestReassignToSameTeacherIsNoOp(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	class := f.seedClass(t)
	future := testutil.Session(t, f.db, f.from, class.ID, time.Now().UTC().AddDate(0, 0, 7))

	res, err := f.svc.Reassign(context.Background(), f.owner, class.ID, f.from)
	require.NoError(t, err)
	require.Equal(t, f.from, res.TeacherID)
	require.Zero(t, res.MovedPlannedSessions)

	// Nothing changed.
	require.Equal(t, f.from, f.teacherOfClass(t, class.ID))
	require.Equal(t, f.from, f.teacherOfSession(t, future.ID))
}

// staffStints reads a teacher's class_staff rows for the class as
// (role_key, ended) pairs.
func (f fixture) staffStints(t *testing.T, classID, teacherID uuid.UUID) []struct {
	RoleKey string
	Ended   bool
} {
	t.Helper()
	var rows []struct {
		RoleKey string
		Ended   bool
	}
	require.NoError(t, f.db.Raw(`
		SELECT role_key, (ended_at IS NOT NULL) AS ended FROM class_staff
		WHERE class_id = ? AND teacher_id = ? ORDER BY started_at`,
		classID, teacherID).Scan(&rows).Error)
	return rows
}

func TestReassignDualWritesClassStaff(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	class := f.seedClass(t)

	_, err := f.svc.Reassign(context.Background(), f.owner, class.ID, f.to)
	require.NoError(t, err)

	// The new teacher holds the one active giao_vien stint…
	toStints := f.staffStints(t, class.ID, f.to)
	require.Len(t, toStints, 1)
	require.Equal(t, "giao_vien", toStints[0].RoleKey)
	require.False(t, toStints[0].Ended)

	// …and the old teacher's stint is soft-closed, not deleted: their history
	// reads survive the handoff.
	fromStints := f.staffStints(t, class.ID, f.from)
	require.Len(t, fromStints, 1)
	require.Equal(t, "giao_vien", fromStints[0].RoleKey)
	require.True(t, fromStints[0].Ended)
}

func TestReassignNoOpRepairsStaffDrift(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	class := f.seedClass(t)

	// Drift: the class lost its giao_vien stint (hand-edited data, or a bug).
	require.NoError(t, f.db.Exec(
		"DELETE FROM class_staff WHERE class_id = ?", class.ID).Error)

	// Handing the class to its current teacher is the repair command.
	res, err := f.svc.Reassign(context.Background(), f.owner, class.ID, f.from)
	require.NoError(t, err)
	require.Equal(t, f.from, res.TeacherID)

	stints := f.staffStints(t, class.ID, f.from)
	require.Len(t, stints, 1)
	require.Equal(t, "giao_vien", stints[0].RoleKey)
	require.False(t, stints[0].Ended)
}

// failingStaff makes the staff sync — the transaction's last write — blow up,
// proving the class move and the stint change commit or roll back as one unit.
type failingStaff struct{}

func (failingStaff) SyncPrimaryTeacher(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) error {
	return errors.New("staff sync failed")
}

func TestReassignRollbackKeepsStaffIntact(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	class := f.seedClass(t)

	txMgr := database.NewTxManager(f.db)
	centersSvc := centers.NewService(centers.NewRepository(f.db), txMgr, nil)
	classesSvc := classes.NewService(classes.NewRepository(f.db), txMgr, classstaff.NewRepository(f.db))
	enrollmentsSvc := enrollments.NewService(enrollments.NewRepository(f.db), nil)
	teachersSvc := teachers.NewService(teachers.NewRepository(f.db))
	sessionsSvc := sessions.NewService(sessions.NewRepository(f.db), classesSvc, teachersSvc, enrollmentsSvc)
	broken := handoff.NewService(classesSvc, sessionsSvc, centersSvc, failingStaff{}, imports.NewLocker(f.db), txMgr)

	_, err := broken.Reassign(context.Background(), f.owner, class.ID, f.to)
	require.Error(t, err)

	// The class move happened before the failing sync, and rolled back with
	// it: teacher_id and the giao_vien stint still agree on the old teacher.
	require.Equal(t, f.from, f.teacherOfClass(t, class.ID))
	stints := f.staffStints(t, class.ID, f.from)
	require.Len(t, stints, 1)
	require.False(t, stints[0].Ended)
	require.Empty(t, f.staffStints(t, class.ID, f.to))
}

func TestReassignByMemberIsForbidden(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	class := f.seedClass(t)
	member := testutil.ScopeFor(t, f.db, f.from)
	require.False(t, member.IsOwner)

	_, err := f.svc.Reassign(context.Background(), member, class.ID, f.to)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
	require.Equal(t, f.from, f.teacherOfClass(t, class.ID), "a forbidden call moves nothing")
}

func TestReassignToNonMemberIsRejected(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	class := f.seedClass(t)

	// A real teacher who is not a member of this center.
	_, stranger := testutil.Teacher(t, f.db)

	_, err := f.svc.Reassign(context.Background(), f.owner, class.ID, stranger.ID)
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code)
	require.Equal(t, f.from, f.teacherOfClass(t, class.ID), "a rejected target moves nothing")
}

func TestReassignUnknownClassIsNotFound(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	_, err := f.svc.Reassign(context.Background(), f.owner, uuid.New(), f.to)
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
}
