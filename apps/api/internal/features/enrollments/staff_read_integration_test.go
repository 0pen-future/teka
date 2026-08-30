//go:build integration

package enrollments_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/testutil"
)

// TestStaffAssignmentWidensEnrollmentReads pins the read-port contract: a
// hoc_vu/tro_giang stint (ended included) lets a member read the class's
// enrollments, while the end/delete writes keep their own-rows gate and an
// unassigned peer sees nothing.
func TestStaffAssignmentWidensEnrollmentReads(t *testing.T) {
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
	testutil.StaffAssignment(t, db, class, staff.ID, authctx.StaffRoleHocVu)

	scStaff := testutil.ScopeFor(t, db, staff.ID)
	scPeer := testutil.ScopeFor(t, db, peer.ID)

	// The assigned member reads the roster: detail, filtered list, and the
	// ActiveOn feed the attendance sheet builds on.
	got, err := svc.Get(ctx, scStaff, enrollment.ID)
	require.NoError(t, err)
	require.Equal(t, enrollment.ID, got.ID)
	rows, total, err := svc.List(ctx, scStaff, enrollments.ListFilter{ClassID: class.ID}, listParams(t))
	require.NoError(t, err)
	require.EqualValues(t, 1, total)
	require.Equal(t, enrollment.ID, rows[0].ID)
	roster, err := svc.ActiveOn(ctx, scStaff, class.ID, date("2026-01-06"))
	require.NoError(t, err)
	require.Len(t, roster, 1)

	// The unassigned peer sees nothing — 404 on detail, empty list.
	_, err = svc.Get(ctx, scPeer, enrollment.ID)
	require.Equal(t, 404, apperror.From(err).Status)
	_, err = svc.End(ctx, scPeer, enrollment.ID, enrollments.EndRequest{})
	require.Equal(t, 404, apperror.From(err).Status,
		"a member with no stint cannot even resolve the row on a write")
	_, total, err = svc.List(ctx, scPeer, enrollments.ListFilter{ClassID: class.ID}, listParams(t))
	require.NoError(t, err)
	require.Zero(t, total)

	// Write-freeze: the stint grants no enrollment writes — a reader gets an
	// honest 403.
	_, err = svc.End(ctx, scStaff, enrollment.ID, enrollments.EndRequest{EndedOn: "2026-01-31"})
	require.Equal(t, 403, apperror.From(err).Status)
	require.Equal(t, 403, apperror.From(svc.Delete(ctx, scStaff, enrollment.ID)).Status)

	// The freeze answers 403 even where a writer would get a state error:
	// ending an already-ended enrollment must not downgrade to the writer's
	// 409, which would tell a non-writer whether the row is already closed.
	endedStudent := testutil.Student(t, db, gv.ID, contact.ID)
	endedEnrollment := testutil.Enrollment(t, db, gv.ID, endedStudent.ID, class.ID, date("2026-01-06"))
	require.NoError(t, db.Exec(
		"UPDATE enrollments SET ended_on = '2026-01-20' WHERE id = ?", endedEnrollment.ID).Error)
	_, err = svc.End(ctx, scStaff, endedEnrollment.ID, enrollments.EndRequest{EndedOn: "2026-01-31"})
	require.Equal(t, 403, apperror.From(err).Status)
	scGv := testutil.ScopeFor(t, db, gv.ID)
	_, err = svc.End(ctx, scGv, endedEnrollment.ID, enrollments.EndRequest{EndedOn: "2026-01-31"})
	require.Equal(t, 409, apperror.From(err).Status, "the writer keeps the double-end conflict")

	// An ended stint keeps history readable.
	require.NoError(t, db.Exec(
		"UPDATE class_staff SET ended_at = now() WHERE class_id = ? AND teacher_id = ?",
		class.ID, staff.ID).Error)
	_, err = svc.Get(ctx, scStaff, enrollment.ID)
	require.NoError(t, err, "an ended stint keeps history readable")

	// A soft-deleted class grants nothing, stint or not.
	require.NoError(t, db.Exec(
		"UPDATE classes SET deleted_at = now() WHERE id = ?", class.ID).Error)
	_, err = svc.Get(ctx, scStaff, enrollment.ID)
	require.Equal(t, 404, apperror.From(err).Status)
}

// The tro_giang stint follows the same enrollment write-freeze as hoc_vu:
// roster reads work, End and Delete answer an honest 403.
func TestTroGiangCannotManageEnrollments(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()
	_, owner := testutil.Teacher(t, db)
	scOwner := testutil.ScopeFor(t, db, owner.ID)
	_, gv := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, gv.ID, scOwner.CenterID)
	_, tg := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, tg.ID, scOwner.CenterID)
	scTG := testutil.ScopeFor(t, db, tg.ID)

	class := testutil.Class(t, db, gv.ID)
	testutil.StaffAssignment(t, db, class, tg.ID, authctx.StaffRoleTroGiang)
	contact := testutil.Contact(t, db, gv.ID)
	student := testutil.Student(t, db, gv.ID, contact.ID)
	enrollment := testutil.Enrollment(t, db, gv.ID, student.ID, class.ID, date("2026-01-05"))

	got, err := svc.Get(ctx, scTG, enrollment.ID)
	require.NoError(t, err, "the assistant reads the roster")
	require.Equal(t, enrollment.ID, got.ID)

	_, err = svc.End(ctx, scTG, enrollment.ID, enrollments.EndRequest{EndedOn: "2026-01-31"})
	require.Equal(t, 403, apperror.From(err).Status)
	require.Equal(t, 403, apperror.From(svc.Delete(ctx, scTG, enrollment.ID)).Status)
}
