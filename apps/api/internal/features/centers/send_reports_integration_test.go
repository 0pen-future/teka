//go:build integration

package centers_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/centers"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/testutil"
)

// grantReportsSend flips a member's reports.send override through the real
// owner flow (ReplaceMemberOverrides), the only remaining write path since
// the dedicated send-reports endpoint retired.
func grantReportsSend(t *testing.T, e *env, ownerID, memberID uuid.UUID, granted bool) {
	t.Helper()
	req := centers.MemberOverridesRequest{}
	if granted {
		req.Grants = []string{authctx.PermReportsSend}
	}
	require.NoError(t, e.centersSvc.ReplaceMemberOverrides(
		context.Background(), e.scope(t, ownerID), memberID, req))
}

// TestSendReportsScopeResolution pins how the effective reports.send
// permission travels into request scope: an override grant → true, a
// replacement without it → false, and a closed stint resolves false even
// though the center_members row still exists.
func TestSendReportsScopeResolution(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	owner, _ := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)

	// The owner holds reports.send implicitly; a fresh member does not.
	require.True(t, e.scope(t, owner.ID).ReportsOversight(), "the owner is the implicit superuser")
	require.False(t, e.scope(t, member.ID).Has(authctx.PermReportsSend))
	require.False(t, e.scope(t, member.ID).ReportsOversight())

	grantReportsSend(t, e, owner.ID, member.ID, true)
	require.True(t, e.scope(t, member.ID).Has(authctx.PermReportsSend))
	require.True(t, e.scope(t, member.ID).ReportsOversight())

	grantReportsSend(t, e, owner.ID, member.ID, false)
	require.False(t, e.scope(t, member.ID).Has(authctx.PermReportsSend))

	// A granted member whose stint closes resolves false: the LEFT JOIN only
	// matches the live stint. The teachers.center_id pointer still aims at the
	// old center, which is exactly the state after RemoveMember.
	grantReportsSend(t, e, owner.ID, member.ID, true)
	require.NoError(t, e.db.Exec(
		"UPDATE center_members SET left_at = now() WHERE teacher_id = ? AND left_at IS NULL",
		member.ID).Error)
	require.False(t, e.scope(t, member.ID).Has(authctx.PermReportsSend),
		"a closed stint must not leak the permission into scope")
}

// TestSendReportsDoesNotSurviveRejoin pins the stint-scoped lifecycle:
// grant → close → reopen resolves false, whether the override delete came
// from CloseMembership or from OpenMembership's clearing CTE.
func TestSendReportsDoesNotSurviveRejoin(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, ownerTeacher := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)

	grantReportsSend(t, e, owner.ID, member.ID, true)

	// Close through the service (the RemoveMember path) — the override rows
	// must die with the stint itself, not only on a later reopen.
	require.NoError(t, e.centersSvc.RemoveMember(ctx, e.scope(t, owner.ID), member.ID))
	require.Empty(t, e.overrideKeys(t, member.ID, ownerTeacher.CenterID),
		"CloseMembership must delete the stint's override rows")

	// Reopen the same row via the accept flow's upsert; even a stale override
	// row planted directly on the closed stint must not resurrect.
	require.NoError(t, e.db.Exec(`
		INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed)
		VALUES (?, ?, ?, TRUE)`,
		member.ID, ownerTeacher.CenterID, authctx.PermReportsSend).Error)
	require.NoError(t, e.centersSvc.OpenMembership(ctx, member.ID, ownerTeacher.CenterID))
	require.Empty(t, e.overrideKeys(t, member.ID, ownerTeacher.CenterID),
		"OpenMembership must clear stale override rows on reopen")
	// RemoveMember disabled the account, and only the invitation accept flow
	// reactivates it — so resolve the scope straight from SQL: the reopened
	// stint must not carry the old grant.
	require.False(t, testutil.ScopeFor(t, e.db, member.ID).Has(authctx.PermReportsSend))
}

// TestMeExposesSendReportsPermission pins how a member learns their own
// send authority from /centers/me: reports.send appears in, then leaves, the
// effective permission list as the grant comes and goes.
func TestMeExposesSendReportsPermission(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)
	grantReportsSend(t, e, owner.ID, member.ID, true)

	memberMe, err := e.centersSvc.Me(ctx, e.scope(t, member.ID))
	require.NoError(t, err)
	memberResp, ok := memberMe.(*centers.MemberMeResponse)
	require.True(t, ok)
	require.Contains(t, memberResp.Permissions, authctx.PermReportsSend)

	grantReportsSend(t, e, owner.ID, member.ID, false)
	memberMe, err = e.centersSvc.Me(ctx, e.scope(t, member.ID))
	require.NoError(t, err)
	require.NotContains(t, memberMe.(*centers.MemberMeResponse).Permissions, authctx.PermReportsSend)
}
