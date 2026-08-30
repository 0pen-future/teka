package handoff

import (
	"errors"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestReassignMovesClassAndPlannedSessions(t *testing.T) {
	t.Parallel()
	f := newHandoffFixture()

	res, err := f.reassign()
	require.NoError(t, err)
	require.Equal(t, f.classID, res.ClassID)
	require.Equal(t, f.newTeacher, res.TeacherID)
	require.Equal(t, int64(3), res.MovedPlannedSessions)

	// The class, its future planned sessions, and the giao_vien stint all move
	// to the same teacher — the dual write runs inside the same transaction.
	require.Equal(t, []reassignCall{{f.classID, f.newTeacher}}, f.classes.reassigned)
	require.Equal(t, []reassignCall{{f.classID, f.newTeacher}}, f.sessions.reassigned)
	require.Equal(t, []reassignCall{{f.classID, f.newTeacher}}, f.staff.synced)
	// The future boundary is anchored on the pre-handoff teacher's timezone.
	require.Equal(t, f.oldTeacher, f.sessions.gotOldTeacher)

	// The move ran under the center lock, keyed on the caller's center.
	require.Equal(t, []uuid.UUID{f.scope.CenterID}, f.locker.centers)
	require.Equal(t, 1, f.locker.timeouts)
}

func TestReassignToSameTeacherIsNoOp(t *testing.T) {
	t.Parallel()
	f := newHandoffFixture()

	res, err := f.svc.Reassign(t.Context(), f.scope, f.classID, f.oldTeacher)
	require.NoError(t, err)
	require.Equal(t, f.oldTeacher, res.TeacherID)
	require.Zero(t, res.MovedPlannedSessions)

	// Nothing moves and no membership check: re-affirming the current teacher
	// must never fail on their roster status.
	require.Empty(t, f.members.checked)
	require.Empty(t, f.classes.reassigned)
	require.Empty(t, f.sessions.reassigned)

	// But it is the class_staff repair command: the giao_vien stint re-syncs
	// under the center lock, healing any drift.
	require.Equal(t, []uuid.UUID{f.scope.CenterID}, f.locker.centers)
	require.Equal(t, []reassignCall{{f.classID, f.oldTeacher}}, f.staff.synced)
}

func TestReassignRequiresOwner(t *testing.T) {
	t.Parallel()
	f := newHandoffFixture()
	f.scope.IsOwner = false

	_, err := f.reassign()
	require.Equal(t, http.StatusForbidden, status(err))

	// The gate runs before any work: no check, no lock, no move.
	require.Empty(t, f.members.checked)
	require.Empty(t, f.locker.centers)
	require.Empty(t, f.classes.reassigned)
}

func TestReassignRejectsNonMemberTarget(t *testing.T) {
	t.Parallel()
	f := newHandoffFixture()
	f.members.active = false

	_, err := f.reassign()
	require.Equal(t, http.StatusUnprocessableEntity, status(err))
	require.Equal(t, []uuid.UUID{f.newTeacher}, f.members.checked)

	// A rejected target moves nothing and never takes the lock.
	require.Empty(t, f.locker.centers)
	require.Empty(t, f.classes.reassigned)
	require.Empty(t, f.sessions.reassigned)
}

func TestReassignSurfacesClassNotFound(t *testing.T) {
	t.Parallel()
	f := newHandoffFixture()
	f.classes.getErr = errors.New("class: not found")

	_, err := f.reassign()
	require.ErrorIs(t, err, f.classes.getErr)
	require.Empty(t, f.members.checked)
	require.Empty(t, f.locker.centers)
}

func TestReassignRefusesWhenCenterLocked(t *testing.T) {
	t.Parallel()
	f := newHandoffFixture()
	f.locker.locked = false

	_, err := f.reassign()
	require.Equal(t, http.StatusConflict, status(err))

	// The membership check passed, the lock was attempted on the caller's
	// center, but nothing moved.
	require.Equal(t, []uuid.UUID{f.scope.CenterID}, f.locker.centers)
	require.Empty(t, f.classes.reassigned)
	require.Empty(t, f.sessions.reassigned)
}

func TestReassignSurfacesLockerFailure(t *testing.T) {
	t.Parallel()
	f := newHandoffFixture()
	f.locker.lockErr = errors.New("connection reset")

	_, err := f.reassign()
	require.ErrorIs(t, err, f.locker.lockErr)
	require.Empty(t, f.classes.reassigned)
}

func TestReassignPropagatesSessionFailure(t *testing.T) {
	t.Parallel()
	f := newHandoffFixture()
	f.sessions.movedErr = errors.New("session update failed")

	_, err := f.reassign()
	require.ErrorIs(t, err, f.sessions.movedErr)

	// The class move ran before the session move inside the same transaction;
	// the fake tx propagates the error so the real one would roll both back.
	require.Len(t, f.classes.reassigned, 1)
}
