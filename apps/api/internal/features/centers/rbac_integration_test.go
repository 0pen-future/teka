//go:build integration

package centers_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/centers"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/internal/testutil"
)

// roleID returns the center's system role with the given key.
func (e *env) roleID(t *testing.T, centerID uuid.UUID, key string) uuid.UUID {
	t.Helper()
	var row struct{ ID uuid.UUID }
	require.NoError(t, e.db.Raw(
		"SELECT id FROM center_roles WHERE center_id = ? AND key = ?", centerID, key).Scan(&row).Error)
	require.NotEqual(t, uuid.Nil, row.ID, "center %s misses role %s", centerID, key)
	return row.ID
}

func (e *env) overrideKeys(t *testing.T, teacherID, centerID uuid.UUID) []string {
	t.Helper()
	var keys []string
	require.NoError(t, e.db.Raw(
		`SELECT permission_key FROM center_member_permissions
		 WHERE teacher_id = ? AND center_id = ? ORDER BY permission_key`,
		teacherID, centerID).Scan(&keys).Error)
	return keys
}

// A center created through the repository is born with exactly its three
// system roles.
func TestCreateCenterSeedsSystemRoles(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	_, member := testutil.Teacher(t, e.db)
	// Joining retires the member's personal center, freeing them to own the
	// new center below without tripping uq_centers_owner.
	testutil.JoinCenter(t, e.db, member.ID, owner.CenterID)

	repo := centers.NewRepository(e.db)
	newCenter := &centers.Center{ID: id.New(), Name: "Trung tâm mới", OwnerID: member.ID}
	require.NoError(t, e.tx.WithinTx(context.Background(), func(ctx context.Context) error {
		return repo.CreateCenter(ctx, newCenter)
	}))

	var roles []struct {
		Key      string
		IsSystem bool
	}
	require.NoError(t, e.db.Raw(
		"SELECT key, is_system FROM center_roles WHERE center_id = ? ORDER BY key",
		newCenter.ID).Scan(&roles).Error)
	require.Len(t, roles, 3)
	require.Equal(t, "giao_vien", roles[0].Key)
	require.Equal(t, "hoc_vu", roles[1].Key)
	require.Equal(t, "tro_giang", roles[2].Key)
	for _, r := range roles {
		require.True(t, r.IsSystem)
	}
}

// Grant/revoke keeps the legacy column and the reports.send override row in
// parity, and the next resolved scope reflects the change immediately.
func TestSendReportsDualWriteParity(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	_, member := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)
	ownerScope := e.scope(t, owner.ID)
	ctx := context.Background()

	require.NoError(t, e.centersSvc.SetSendReports(ctx, ownerScope, member.ID, true))
	require.True(t, e.liveMembership(t, member.ID).CanSendReports)
	require.Equal(t, []string{"reports.send"}, e.overrideKeys(t, member.ID, ownerScope.CenterID))
	memberScope := e.scope(t, member.ID)
	require.True(t, memberScope.CanSendReports)
	require.True(t, memberScope.Has(authctx.PermReportsSend))

	require.NoError(t, e.centersSvc.SetSendReports(ctx, ownerScope, member.ID, false))
	require.False(t, e.liveMembership(t, member.ID).CanSendReports)
	require.Empty(t, e.overrideKeys(t, member.ID, ownerScope.CenterID))
	memberScope = e.scope(t, member.ID)
	require.False(t, memberScope.CanSendReports)
	require.False(t, memberScope.Has(authctx.PermReportsSend))
}

// Role permissions, member overrides, and denies all land in the very next
// resolved scope — the fresh-from-DB invariant the middleware depends on.
func TestResolveScopeEffectivePermissions(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	_, member := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)
	centerID := e.scope(t, owner.ID).CenterID
	hocVu := e.roleID(t, centerID, "hoc_vu")

	// Assign the member a role holding audit.read + an unknown key (a code
	// rollback scenario) and a grant/deny pair on top.
	require.NoError(t, e.db.Exec(
		"UPDATE center_members SET role_id = ? WHERE teacher_id = ? AND left_at IS NULL",
		hocVu, member.ID).Error)
	require.NoError(t, e.db.Exec(
		`INSERT INTO center_role_permissions (role_id, permission_key)
		 VALUES (?, 'audit.read'), (?, 'ghost.key'), (?, 'dashboard.view')`,
		hocVu, hocVu, hocVu).Error)
	require.NoError(t, e.db.Exec(
		`INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed)
		 VALUES (?, ?, 'data.view_center_wide', TRUE), (?, ?, 'dashboard.view', FALSE)`,
		member.ID, centerID, member.ID, centerID).Error)

	sc := e.scope(t, member.ID)
	require.True(t, sc.Has(authctx.PermAuditRead), "role grant must apply")
	require.False(t, sc.Has("ghost.key"), "unknown keys are ignored on read")
	require.False(t, sc.Has(authctx.PermDashboardView), "deny must beat the role grant")
	require.True(t, sc.CenterWide(), "override grant must widen data scope")
	require.False(t, sc.IsOwner)

	ownerScope := e.scope(t, owner.ID)
	require.True(t, ownerScope.Has(authctx.PermAuditRead), "owner bypass")
	require.True(t, ownerScope.CenterWide())
}

// Closing and reopening a membership resets the stint's permission state
// through the real repository statements: overrides are wiped, the flag
// drops, and the role returns to the default giao_vien.
func TestMembershipReopenResetsRoleAndOverrides(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	_, member := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)
	ownerScope := e.scope(t, owner.ID)
	centerID := ownerScope.CenterID
	ctx := context.Background()
	repo := centers.NewRepository(e.db)

	// Build up stint-scoped state: send-reports grant (column + override
	// row), an elevated role, and an extra deny row.
	require.NoError(t, e.centersSvc.SetSendReports(ctx, ownerScope, member.ID, true))
	hocVu := e.roleID(t, centerID, "hoc_vu")
	require.NoError(t, e.db.Exec(
		"UPDATE center_members SET role_id = ? WHERE teacher_id = ? AND left_at IS NULL",
		hocVu, member.ID).Error)
	require.NoError(t, e.db.Exec(
		`INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed)
		 VALUES (?, ?, 'audit.read', FALSE)`, member.ID, centerID).Error)

	require.NoError(t, repo.CloseMembership(ctx, member.ID, centerID))
	require.Empty(t, e.overrideKeys(t, member.ID, centerID), "close must wipe override rows")

	_, err := repo.OpenMembership(ctx, member.ID, centerID)
	require.NoError(t, err)
	m := e.liveMembership(t, member.ID)
	require.False(t, m.CanSendReports)
	require.NotNil(t, m.RoleID, "reopened member stint gets the default role")
	require.Equal(t, e.roleID(t, centerID, "giao_vien"), *m.RoleID)
	require.Empty(t, e.overrideKeys(t, member.ID, centerID))

	sc := e.scope(t, member.ID)
	require.False(t, sc.CanSendReports, "no source may resurrect the revoked permission")
	require.False(t, sc.Has(authctx.PermReportsSend))
}

// The owner's own membership stint stays outside the role system even when
// opened through the production repository path.
func TestOwnerStintStaysRoleless(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	ctx := context.Background()
	repo := centers.NewRepository(e.db)

	// Reopen the owner's own stint — the role subquery must refuse them.
	require.NoError(t, repo.CloseMembership(ctx, owner.ID, owner.CenterID))
	_, err := repo.OpenMembership(ctx, owner.ID, owner.CenterID)
	require.NoError(t, err)
	m := e.liveMembership(t, owner.ID)
	require.Nil(t, m.RoleID, "owner must never hold a role row")
}
