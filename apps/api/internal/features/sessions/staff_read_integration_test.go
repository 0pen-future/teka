//go:build integration

package sessions_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/testutil"
)

// TestStaffAssignmentWidensSessionReads pins the read-port contract: a
// class_staff stint (any role, ended included) grants classbook and session
// detail reads, without granting the generation or lifecycle writes, and an
// unassigned peer keeps the 404 that hides the class's existence.
func TestStaffAssignmentWidensSessionReads(t *testing.T) {
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

	class := testutil.Class(t, db, gv.ID, testutil.WithClassStartDate(date("2026-01-01")))
	testutil.Schedule(t, db, class, 2, "18:00") // Tuesdays
	seeded := testutil.Session(t, db, gv.ID, class.ID, date("2026-01-06"))
	testutil.StaffAssignment(t, db, class, staff.ID, authctx.StaffRoleHocVu)

	scStaff := testutil.ScopeFor(t, db, staff.ID)
	scPeer := testutil.ScopeFor(t, db, peer.ID)

	// The staff member reads the classbook, and their GET never materialises
	// the missing scheduled dates — only the seeded row comes back.
	rows, err := svc.ListRangeReadable(ctx, scStaff, class.ID, date("2026-01-01"), date("2026-01-31"))
	require.NoError(t, err)
	require.Len(t, rows, 1)
	var count int64
	require.NoError(t, db.Table("class_sessions").
		Where("class_id = ?", class.ID).Count(&count).Error)
	require.EqualValues(t, 1, count, "a staff member's classbook GET must not generate sessions")

	detail, err := svc.GetReadable(ctx, scStaff, seeded.ID)
	require.NoError(t, err)
	require.Equal(t, seeded.ID, detail.ID)

	// An unassigned peer gets 404 on both reads — no existence leak.
	_, err = svc.ListRangeReadable(ctx, scPeer, class.ID, date("2026-01-01"), date("2026-01-31"))
	require.Equal(t, 404, apperror.From(err).Status)
	_, err = svc.GetReadable(ctx, scPeer, seeded.ID)
	require.Equal(t, 404, apperror.From(err).Status)

	// Write-freeze: the stint grants no session lifecycle writes — the
	// capability gate refuses the role, and a caller who can read the session
	// gets an honest 403.
	_, err = svc.Cancel(ctx, scStaff, seeded.ID, "nghỉ lễ")
	require.Equal(t, 403, apperror.From(err).Status)
	require.Equal(t, 403, apperror.From(svc.Delete(ctx, scStaff, seeded.ID)).Status)
	_, err = svc.Hold(ctx, scStaff, seeded.ID)
	require.Equal(t, 403, apperror.From(err).Status)

	// The owner's classbook GET keeps the generating path: the four scheduled
	// Tuesdays materialise (the seeded 01-06 row is one of them).
	generated, err := svc.ListRangeReadable(ctx, scOwner, class.ID, date("2026-01-01"), date("2026-01-31"))
	require.NoError(t, err)
	require.Len(t, generated, 4, "the center-wide read still delegates to generation")

	// An ended stint keeps history readable.
	require.NoError(t, db.Exec(
		"UPDATE class_staff SET ended_at = now() WHERE class_id = ? AND teacher_id = ?",
		class.ID, staff.ID).Error)
	_, err = svc.GetReadable(ctx, scStaff, seeded.ID)
	require.NoError(t, err, "an ended stint keeps history readable")

	// A soft-deleted class grants nothing, stint or not.
	require.NoError(t, db.Exec(
		"UPDATE classes SET deleted_at = now() WHERE id = ?", class.ID).Error)
	_, err = svc.ListRangeReadable(ctx, scStaff, class.ID, date("2026-01-01"), date("2026-01-31"))
	require.Equal(t, 404, apperror.From(err).Status)
	_, err = svc.GetReadable(ctx, scStaff, seeded.ID)
	require.Equal(t, 404, apperror.From(err).Status)
}
