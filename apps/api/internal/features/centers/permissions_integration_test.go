//go:build integration

package centers_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/audit"
	"teka/apps/api/internal/features/centers"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/testutil"
)

// All four permission-management operations are hard-gated on ownership —
// a member holding every catalog key still cannot reach them, because the
// gate is scope.IsOwner, not a grantable permission (one grant away from
// self-escalation otherwise).
func TestPermissionManagementIsOwnerOnly(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)
	ownerScope := e.scope(t, owner.ID)
	roleID := e.roleID(t, ownerScope.CenterID, "hoc_vu")

	// Give the member every grantable key — the gate must still refuse.
	require.NoError(t, e.centersSvc.ReplaceMemberOverrides(ctx, ownerScope, member.ID,
		centers.MemberOverridesRequest{Grants: authctx.PermKeys()}))
	memberScope := e.scope(t, member.ID)

	_, err := e.centersSvc.Permissions(ctx, memberScope)
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
	err = e.centersSvc.ReplaceRolePermissions(ctx, memberScope, roleID, centers.RolePermissionsRequest{})
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
	err = e.centersSvc.AssignMemberRole(ctx, memberScope, member.ID, centers.MemberRoleRequest{RoleID: roleID})
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
	err = e.centersSvc.ReplaceMemberOverrides(ctx, memberScope, member.ID, centers.MemberOverridesRequest{})
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
}

// The owner's read model: the full labeled catalog, the three system roles
// in key order, and only non-owner members with their role and overrides.
func TestPermissionsReadModel(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db, testutil.WithFullName("Giáo Viên B"))
	e.join(t, member.ID, owner.ID)
	ownerScope := e.scope(t, owner.ID)

	resp, err := e.centersSvc.Permissions(ctx, ownerScope)
	require.NoError(t, err)

	require.Len(t, resp.Catalog, len(authctx.PermKeys()))
	for i, key := range authctx.PermKeys() {
		require.Equal(t, key, resp.Catalog[i].Key)
		require.NotEmpty(t, resp.Catalog[i].Label, "every catalog key ships a vi label")
	}

	require.Len(t, resp.Roles, 3)
	require.Equal(t, "giao_vien", resp.Roles[0].Key)
	require.Equal(t, "hoc_vu", resp.Roles[1].Key)
	require.Equal(t, "tro_giang", resp.Roles[2].Key)
	for _, r := range resp.Roles {
		require.Empty(t, r.Permissions, "system roles are born with empty sets (v1 parity)")
	}

	// The owner never appears as a manageable member.
	require.Len(t, resp.Members, 1)
	m := resp.Members[0]
	require.Equal(t, member.ID, m.TeacherID)
	require.Equal(t, "Giáo Viên B", m.FullName)
	// The raw fixture join leaves role_id NULL — a valid state the read model
	// must report as "no role" rather than invent a default. The production
	// join path's giao_vien default is pinned by
	// TestMembershipReopenResetsRoleAndOverrides.
	require.Nil(t, m.RoleID)
	require.Empty(t, m.RoleKey)
	require.Empty(t, m.Grants)
	require.Empty(t, m.Denies)

	// Unknown keys left behind by a code rollback stay out of the read model.
	require.NoError(t, e.db.Exec(
		`INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed)
		 VALUES (?, ?, 'ghost.key', TRUE)`, member.ID, ownerScope.CenterID).Error)
	resp, err = e.centersSvc.Permissions(ctx, ownerScope)
	require.NoError(t, err)
	require.Empty(t, resp.Members[0].Grants, "unknown DB keys are filtered on read")
}

// The role matrix round-trip: replace, read back, and the very next resolved
// member scope carries the role's set. reports.send stays rejected while the
// legacy column is authoritative, and unknown keys never reach the table.
func TestReplaceRolePermissions(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)
	ownerScope := e.scope(t, owner.ID)
	hocVu := e.roleID(t, ownerScope.CenterID, "hoc_vu")

	err := e.centersSvc.ReplaceRolePermissions(ctx, ownerScope, hocVu,
		centers.RolePermissionsRequest{Permissions: []string{"audit.read", "ghost.key"}})
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code, "unknown key must 422")

	err = e.centersSvc.ReplaceRolePermissions(ctx, ownerScope, hocVu,
		centers.RolePermissionsRequest{Permissions: []string{authctx.PermReportsSend}})
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code,
		"reports.send is per-member only while the column is authoritative")

	require.NoError(t, e.centersSvc.ReplaceRolePermissions(ctx, ownerScope, hocVu,
		centers.RolePermissionsRequest{Permissions: []string{
			authctx.PermAuditRead, authctx.PermDashboardView}}))
	require.NoError(t, e.centersSvc.AssignMemberRole(ctx, ownerScope, member.ID,
		centers.MemberRoleRequest{RoleID: hocVu}))

	resp, err := e.centersSvc.Permissions(ctx, ownerScope)
	require.NoError(t, err)
	require.Equal(t, []string{authctx.PermAuditRead, authctx.PermDashboardView},
		resp.Roles[1].Permissions)

	sc := e.scope(t, member.ID)
	require.True(t, sc.Has(authctx.PermAuditRead))
	require.True(t, sc.Has(authctx.PermDashboardView))

	// Replacing with the empty set strips the role again.
	require.NoError(t, e.centersSvc.ReplaceRolePermissions(ctx, ownerScope, hocVu,
		centers.RolePermissionsRequest{}))
	sc = e.scope(t, member.ID)
	require.False(t, sc.Has(authctx.PermAuditRead))

	// A role id from another center is a 404 — tenancy comes from scope, not
	// from the path.
	otherOwner, _ := testutil.Teacher(t, e.db)
	otherRole := e.roleID(t, e.scope(t, otherOwner.ID).CenterID, "hoc_vu")
	err = e.centersSvc.ReplaceRolePermissions(ctx, ownerScope, otherRole, centers.RolePermissionsRequest{})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
	err = e.centersSvc.ReplaceRolePermissions(ctx, ownerScope, uuid.New(), centers.RolePermissionsRequest{})
	require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code)
}

// Member-targeted endpoints collapse the owner, strangers, and other
// centers' members into the same 404 — the SetSendReports precedent.
func TestMemberTargetTenancy(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)
	otherOwner, _ := testutil.Teacher(t, e.db)
	otherMember, _ := testutil.Teacher(t, e.db)
	e.join(t, otherMember.ID, otherOwner.ID)

	ownerScope := e.scope(t, owner.ID)
	roleID := e.roleID(t, ownerScope.CenterID, "hoc_vu")

	for name, target := range map[string]uuid.UUID{
		"owner as target":        owner.ID,
		"other center's member":  otherMember.ID,
		"stranger without stint": uuid.New(),
	} {
		err := e.centersSvc.AssignMemberRole(ctx, ownerScope, target, centers.MemberRoleRequest{RoleID: roleID})
		require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code, "assign role: %s", name)
		err = e.centersSvc.ReplaceMemberOverrides(ctx, ownerScope, target, centers.MemberOverridesRequest{})
		require.Equal(t, apperror.CodeNotFound, apperror.From(err).Code, "replace overrides: %s", name)
	}
}

// Overrides round-trip: validation (unknown key, grant∩deny), the
// reports.send dual-write parity with the legacy column in both directions,
// and the read model reflecting the stored lists.
func TestReplaceMemberOverrides(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)
	ownerScope := e.scope(t, owner.ID)

	err := e.centersSvc.ReplaceMemberOverrides(ctx, ownerScope, member.ID,
		centers.MemberOverridesRequest{Grants: []string{"ghost.key"}})
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code, "unknown grant must 422")
	err = e.centersSvc.ReplaceMemberOverrides(ctx, ownerScope, member.ID,
		centers.MemberOverridesRequest{
			Grants: []string{authctx.PermAuditRead}, Denies: []string{authctx.PermAuditRead}})
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code, "grant∩deny must 422")

	// Granting reports.send dual-writes the authoritative column…
	require.NoError(t, e.centersSvc.ReplaceMemberOverrides(ctx, ownerScope, member.ID,
		centers.MemberOverridesRequest{
			Grants: []string{authctx.PermReportsSend, authctx.PermAuditRead},
			Denies: []string{authctx.PermDashboardView}}))
	require.True(t, e.liveMembership(t, member.ID).CanSendReports,
		"reports.send grant must set the legacy column in the same tx")
	sc := e.scope(t, member.ID)
	require.True(t, sc.CanSendReports)
	require.True(t, sc.Has(authctx.PermAuditRead))
	require.False(t, sc.Has(authctx.PermDashboardView))

	resp, err := e.centersSvc.Permissions(ctx, ownerScope)
	require.NoError(t, err)
	require.Equal(t, []string{authctx.PermReportsSend, authctx.PermAuditRead},
		resp.Members[0].Grants, "grants come back in registry order")
	require.Equal(t, []string{authctx.PermDashboardView}, resp.Members[0].Denies)

	// …and a replacement without it clears the column again.
	require.NoError(t, e.centersSvc.ReplaceMemberOverrides(ctx, ownerScope, member.ID,
		centers.MemberOverridesRequest{Grants: []string{authctx.PermAuditRead}}))
	require.False(t, e.liveMembership(t, member.ID).CanSendReports,
		"dropping the reports.send grant must clear the legacy column")
	require.Equal(t, []string{"audit.read"}, e.overrideKeys(t, member.ID, ownerScope.CenterID))
	sc = e.scope(t, member.ID)
	require.False(t, sc.CanSendReports)
	require.False(t, sc.Has(authctx.PermReportsSend))
}

// A granted permission bites on the very next request through a real gated
// endpoint: rename is Has(center.manage)-gated, so an override grant flips a
// member's rename from 403 to 200 with no token change in between.
func TestOverrideGrantUnlocksGatedEndpoint(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, ownerTeacher := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)
	ownerScope := e.scope(t, owner.ID)

	err := e.centersSvc.Rename(ctx, e.scope(t, member.ID), centers.RenameRequest{Name: "Trước Khi Cấp"})
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)

	require.NoError(t, e.centersSvc.ReplaceMemberOverrides(ctx, ownerScope, member.ID,
		centers.MemberOverridesRequest{Grants: []string{authctx.PermCenterManage}}))

	require.NoError(t, e.centersSvc.Rename(ctx, e.scope(t, member.ID),
		centers.RenameRequest{Name: "Sau Khi Cấp"}))
	var reloaded centers.Center
	require.NoError(t, e.db.First(&reloaded, "id = ?", ownerTeacher.CenterID).Error)
	require.Equal(t, "Sau Khi Cấp", reloaded.Name)

	// Revocation bites just as fast: an empty replacement flips the same
	// endpoint back to 403.
	require.NoError(t, e.centersSvc.ReplaceMemberOverrides(ctx, ownerScope, member.ID,
		centers.MemberOverridesRequest{}))
	err = e.centersSvc.Rename(ctx, e.scope(t, member.ID), centers.RenameRequest{Name: "Sau Khi Thu Hồi"})
	require.Equal(t, apperror.CodeForbidden, apperror.From(err).Code)
}

// The phase's acceptance criterion verbatim: a member granted audit.read —
// via role or via override — can read the audit trail on the very next
// request, and revocation flips it back.
func TestAuditReadGrantRevokeNextRequest(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)
	ownerScope := e.scope(t, owner.ID)
	hocVu := e.roleID(t, ownerScope.CenterID, "hoc_vu")
	auditSvc := audit.NewService(audit.NewRepository(e.db))
	listAs := func(teacherID uuid.UUID) error {
		_, _, err := auditSvc.List(ctx, e.scope(t, teacherID), audit.ListQuery{})
		return err
	}

	require.Equal(t, apperror.CodeForbidden, apperror.From(listAs(member.ID)).Code)

	// Via role…
	require.NoError(t, e.centersSvc.ReplaceRolePermissions(ctx, ownerScope, hocVu,
		centers.RolePermissionsRequest{Permissions: []string{authctx.PermAuditRead}}))
	require.NoError(t, e.centersSvc.AssignMemberRole(ctx, ownerScope, member.ID,
		centers.MemberRoleRequest{RoleID: hocVu}))
	require.NoError(t, listAs(member.ID))
	require.NoError(t, e.centersSvc.ReplaceRolePermissions(ctx, ownerScope, hocVu,
		centers.RolePermissionsRequest{}))
	require.Equal(t, apperror.CodeForbidden, apperror.From(listAs(member.ID)).Code)

	// …and via override, independently of the (now empty) role.
	require.NoError(t, e.centersSvc.ReplaceMemberOverrides(ctx, ownerScope, member.ID,
		centers.MemberOverridesRequest{Grants: []string{authctx.PermAuditRead}}))
	require.NoError(t, listAs(member.ID))
	require.NoError(t, e.centersSvc.ReplaceMemberOverrides(ctx, ownerScope, member.ID,
		centers.MemberOverridesRequest{}))
	require.Equal(t, apperror.CodeForbidden, apperror.From(listAs(member.ID)).Code)
}

// /centers/me carries the caller's effective permission list in both shapes:
// the owner sees the full catalog (implicit superuser), a member sees role ∪
// grants − denies.
func TestMeReturnsEffectivePermissions(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)
	ownerScope := e.scope(t, owner.ID)

	ownerMe, err := e.centersSvc.Me(ctx, ownerScope)
	require.NoError(t, err)
	require.Equal(t, authctx.PermKeys(), ownerMe.(*centers.MeResponse).Permissions,
		"the owner's effective set is the whole catalog")

	memberMe, err := e.centersSvc.Me(ctx, e.scope(t, member.ID))
	require.NoError(t, err)
	require.Empty(t, memberMe.(*centers.MemberMeResponse).Permissions,
		"a fresh member holds no effective permissions")

	require.NoError(t, e.centersSvc.ReplaceMemberOverrides(ctx, ownerScope, member.ID,
		centers.MemberOverridesRequest{Grants: []string{
			authctx.PermAuditRead, authctx.PermDashboardView}}))
	memberMe, err = e.centersSvc.Me(ctx, e.scope(t, member.ID))
	require.NoError(t, err)
	require.Equal(t, []string{authctx.PermAuditRead, authctx.PermDashboardView},
		memberMe.(*centers.MemberMeResponse).Permissions,
		"effective permissions come back in registry order")
}
