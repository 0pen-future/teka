//go:build integration

package grading_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/grading"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/testutil"
)

// TestStaffAssignmentWidensGradingReads pins the read-port contract on
// grading: a hoc_vu/tro_giang stint (ended included) reads the class's score
// components and a session's score grid, while the score writes keep their
// gates — PUT needs the scores capability (giao_vien), assign/clear stay
// owner-only.
func TestStaffAssignmentWidensGradingReads(t *testing.T) {
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
	testutil.Enrollment(t, db, gv.ID, student.ID, class.ID, date("2026-01-01"))
	session := testutil.Session(t, db, gv.ID, class.ID, date("2026-01-06"))
	testutil.StaffAssignment(t, db, class, staff.ID, authctx.StaffRoleHocVu)

	set, err := svc.CreateSet(ctx, scOwner, grading.ScoreSetRequest{
		Name: "Bộ điểm chuẩn", Components: []string{"Giữa kỳ", "Cuối kỳ"},
	})
	require.NoError(t, err)
	_, err = svc.AssignScoreSet(ctx, scOwner, class.ID, set.ID)
	require.NoError(t, err)

	scStaff := testutil.ScopeFor(t, db, staff.ID)
	scPeer := testutil.ScopeFor(t, db, peer.ID)

	// The assigned member reads the component config and the session grid.
	components, err := svc.GetClassComponents(ctx, scStaff, class.ID)
	require.NoError(t, err)
	require.Len(t, components.Components, 2)
	grid, err := svc.GetSessionScores(ctx, scStaff, session.ID)
	require.NoError(t, err)
	require.Len(t, grid.Components, 2)
	require.Empty(t, grid.Scores)

	// The unassigned peer keeps 404 on both reads.
	_, err = svc.GetClassComponents(ctx, scPeer, class.ID)
	require.Equal(t, 404, apperror.From(err).Status)
	_, err = svc.GetSessionScores(ctx, scPeer, session.ID)
	require.Equal(t, 404, apperror.From(err).Status)

	// Write-freeze: the stint reaches no score write. PUT resolves through the
	// write capability, so a reader without it gets an honest 403; assign/clear
	// refuse every non-owner (403).
	score := 8.5
	_, err = svc.PutSessionScores(ctx, scStaff, session.ID, []grading.ScoreEntryRequest{
		{StudentID: student.ID, ComponentID: components.Components[0].ID, Score: &score},
	})
	require.Equal(t, 403, apperror.From(err).Status)
	_, err = svc.AssignScoreSet(ctx, scStaff, class.ID, set.ID)
	require.Equal(t, 403, apperror.From(err).Status)
	require.Equal(t, 403, apperror.From(svc.ClearScoreSet(ctx, scStaff, class.ID)).Status)

	// An ended stint keeps the grid readable.
	require.NoError(t, db.Exec(
		"UPDATE class_staff SET ended_at = now() WHERE class_id = ? AND teacher_id = ?",
		class.ID, staff.ID).Error)
	_, err = svc.GetClassComponents(ctx, scStaff, class.ID)
	require.NoError(t, err, "an ended stint keeps the score config readable")

	// A soft-deleted class grants nothing, stint or not.
	require.NoError(t, db.Exec(
		"UPDATE classes SET deleted_at = now() WHERE id = ?", class.ID).Error)
	_, err = svc.GetClassComponents(ctx, scStaff, class.ID)
	require.Equal(t, 404, apperror.From(err).Status)
	_, err = svc.GetSessionScores(ctx, scStaff, session.ID)
	require.Equal(t, 404, apperror.From(err).Status)
}

// Score entry follows the active giao_vien stint, not the session's teacher
// anchor: the owner and active GV write, tro_giang/hoc_vu get 403, an
// unassigned member 404 — and after a handoff the old GV loses score writes on
// every session (their own past anchors included) while the new GV gains them.
func TestPutSessionScoresCapabilityGateAndHandoff(t *testing.T) {
	t.Parallel()
	svc, db := newIntegrationService(t)
	ctx := context.Background()

	_, owner := testutil.Teacher(t, db)
	scOwner := testutil.ScopeFor(t, db, owner.ID)
	_, gv := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, gv.ID, scOwner.CenterID)
	scGV := testutil.ScopeFor(t, db, gv.ID)
	_, tg := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, tg.ID, scOwner.CenterID)
	scTG := testutil.ScopeFor(t, db, tg.ID)
	_, outsider := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, outsider.ID, scOwner.CenterID)
	scOut := testutil.ScopeFor(t, db, outsider.ID)
	_, newGV := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, newGV.ID, scOwner.CenterID)
	scNew := testutil.ScopeFor(t, db, newGV.ID)

	class := testutil.Class(t, db, gv.ID)
	testutil.StaffAssignment(t, db, class, tg.ID, authctx.StaffRoleTroGiang)
	contact := testutil.Contact(t, db, gv.ID)
	student := testutil.Student(t, db, gv.ID, contact.ID)
	testutil.Enrollment(t, db, gv.ID, student.ID, class.ID, date("2026-01-01"))
	session := testutil.Session(t, db, gv.ID, class.ID, date("2026-01-06"))

	set, err := svc.CreateSet(ctx, scOwner, grading.ScoreSetRequest{
		Name: "Bộ điểm", Components: []string{"Giữa kỳ"},
	})
	require.NoError(t, err)
	assigned, err := svc.AssignScoreSet(ctx, scOwner, class.ID, set.ID)
	require.NoError(t, err)
	comp := assigned.Components[0].ID
	score := 7.0
	entries := []grading.ScoreEntryRequest{{StudentID: student.ID, ComponentID: comp, Score: &score}}

	_, err = svc.PutSessionScores(ctx, scGV, session.ID, entries)
	require.NoError(t, err, "active giáo viên enters scores")
	_, err = svc.PutSessionScores(ctx, scOwner, session.ID, entries)
	require.NoError(t, err, "owner enters scores center-wide")
	_, err = svc.PutSessionScores(ctx, scTG, session.ID, entries)
	require.Equal(t, 403, apperror.From(err).Status, "tro_giang reads but cannot grade")
	_, err = svc.PutSessionScores(ctx, scOut, session.ID, entries)
	require.Equal(t, 404, apperror.From(err).Status)

	// Handoff: writes move with the stint even on the session still anchored
	// to the old teacher.
	require.NoError(t, db.Exec(
		"UPDATE classes SET teacher_id = ? WHERE id = ?", newGV.ID, class.ID).Error)
	require.NoError(t, db.Exec(
		"UPDATE class_staff SET ended_at = now() WHERE class_id = ? AND role_key = 'giao_vien' AND ended_at IS NULL",
		class.ID).Error)
	require.NoError(t, db.Exec(
		"INSERT INTO class_staff (class_id, center_id, teacher_id, role_key) VALUES (?, ?, ?, 'giao_vien')",
		class.ID, scOwner.CenterID, newGV.ID).Error)

	_, err = svc.PutSessionScores(ctx, scGV, session.ID, entries)
	require.Equal(t, 403, apperror.From(err).Status,
		"the old GV keeps reads but loses score writes, even on their own session anchor")
	_, err = svc.PutSessionScores(ctx, scNew, session.ID, entries)
	require.NoError(t, err, "the new GV grades the pre-handoff session")
}
