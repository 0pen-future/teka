//go:build integration

package attendance_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/attendance"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/testutil"
)

// TestStaffAssignmentWidensAttendanceReads pins the read-port contract on the
// attendance sheet: a class_staff stint (ended included) lets a member read a
// session's sheet — roster, recorded rows, and student names resolved — while
// a stint whose role lacks the attendance capability still cannot confirm,
// and an unassigned peer stays 404.
func TestStaffAssignmentWidensAttendanceReads(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()

	_, owner := testutil.Teacher(t, db)
	scOwner := testutil.ScopeFor(t, db, owner.ID)
	_, gv := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, gv.ID, scOwner.CenterID)
	_, staff := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, staff.ID, scOwner.CenterID)
	_, peer := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, peer.ID, scOwner.CenterID)

	class := testutil.Class(t, db, gv.ID)
	contact := testutil.Contact(t, db, gv.ID)
	student := testutil.Student(t, db, gv.ID, contact.ID)
	enrollment := testutil.Enrollment(t, db, gv.ID, student.ID, class.ID, date("2026-01-01"))
	session := testutil.Session(t, db, gv.ID, class.ID, date("2026-01-06"))
	testutil.AttendanceRecord(t, db, gv.ID, session.ID, student.ID, enrollment.ID)
	testutil.StaffAssignment(t, db, class, staff.ID, authctx.StaffRoleHocVu)

	scStaff := testutil.ScopeFor(t, db, staff.ID)
	scPeer := testutil.ScopeFor(t, db, peer.ID)

	// The assigned member reads the sheet: the roster row appears with the
	// recorded status and the student's name resolved despite the student row
	// belonging to the giáo viên.
	sheet, err := svc.Get(ctx, scStaff, session.ID)
	require.NoError(t, err)
	require.Len(t, sheet.Rows, 1)
	require.Equal(t, student.ID, sheet.Rows[0].StudentID)
	require.Equal(t, "Fixture Student", sheet.Rows[0].StudentName)
	require.NotNil(t, sheet.Rows[0].Status)
	require.Equal(t, attendance.StatusPresent, *sheet.Rows[0].Status)

	// The unassigned peer gets the session's 404 — no existence leak.
	_, err = svc.Get(ctx, scPeer, session.ID)
	require.Equal(t, 404, apperror.From(err).Status)

	// Write-freeze: hoc_vu reads the class but the role holds no attendance
	// capability, so the write gate answers an honest 403 — never a 404 that
	// would deny the session exists to someone who can read its sheet.
	_, err = svc.Confirm(ctx, scStaff, session.ID, attendance.ConfirmRequest{})
	require.Equal(t, 403, apperror.From(err).Status)

	// An ended stint keeps the sheet readable.
	require.NoError(t, db.Exec(
		"UPDATE class_staff SET ended_at = now() WHERE class_id = ? AND teacher_id = ?",
		class.ID, staff.ID).Error)
	_, err = svc.Get(ctx, scStaff, session.ID)
	require.NoError(t, err, "an ended stint keeps the sheet readable")

	// A soft-deleted class grants nothing, stint or not.
	require.NoError(t, db.Exec(
		"UPDATE classes SET deleted_at = now() WHERE id = ?", class.ID).Error)
	_, err = svc.Get(ctx, scStaff, session.ID)
	require.Equal(t, 404, apperror.From(err).Status)
}
