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
	"teka/apps/api/internal/shared/id"
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
		centers.MemberOverridesRequest{Grants: authctx.GrantableKeys()}))
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

	// The assignment catalog lists every non-deprecated definition in catalog
	// order, with the structured fields the permission UI groups and warns on.
	var active []authctx.PermDef
	for _, d := range authctx.PermDefs() {
		if !d.Deprecated {
			active = append(active, d)
		}
	}
	require.Len(t, resp.Catalog, len(active))
	for i, d := range active {
		require.Equal(t, d.Key, resp.Catalog[i].Key)
		require.NotEmpty(t, resp.Catalog[i].Label, "every catalog key ships a vi label")
		require.Equal(t, d.Resource, resp.Catalog[i].Resource)
		require.Equal(t, d.Action, resp.Catalog[i].Action)
		require.Equal(t, string(d.Kind), resp.Catalog[i].Kind)
		require.Equal(t, string(d.Risk), resp.Catalog[i].Risk)
		require.NotEmpty(t, resp.Catalog[i].Description)
	}

	require.Len(t, resp.Roles, 3)
	require.Equal(t, "giao_vien", resp.Roles[0].Key)
	require.Equal(t, "hoc_vu", resp.Roles[1].Key)
	require.Equal(t, "tro_giang", resp.Roles[2].Key)
	// System roles are born with the operational default baseline — the same
	// set the compatibility backfill grants pre-catalog centers. The read
	// model serializes in catalog order.
	for _, r := range resp.Roles {
		require.Equal(t, authctx.DefaultRoleKeys(), r.Permissions,
			"system roles are born with the default operational baseline")
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
// member scope carries the role's set — reports.send included, now that the
// role-matrix restriction retired with the legacy column. Unknown keys never
// reach the table.
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

	// A role may carry reports.send now that the override rows are the only
	// source: a member holding the role sends without a per-member grant.
	require.NoError(t, e.centersSvc.ReplaceRolePermissions(ctx, ownerScope, hocVu,
		centers.RolePermissionsRequest{Permissions: []string{authctx.PermReportsSend}}))
	require.NoError(t, e.centersSvc.AssignMemberRole(ctx, ownerScope, member.ID,
		centers.MemberRoleRequest{RoleID: hocVu}))
	require.True(t, e.scope(t, member.ID).CanSendReports,
		"role-granted reports.send must resolve into the member's scope")

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

// Assignment writes accept only grantable catalog keys: the deprecated
// data.view_center_wide is rejected with a field error even though existing
// rows for it stay effective — new writes must use the per-resource view_all
// keys.
func TestDeprecatedKeyRejectedOnWrite(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)
	ownerScope := e.scope(t, owner.ID)
	hocVu := e.roleID(t, ownerScope.CenterID, "hoc_vu")

	err := e.centersSvc.ReplaceRolePermissions(ctx, ownerScope, hocVu,
		centers.RolePermissionsRequest{Permissions: []string{authctx.PermDataViewCenterWide}})
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code)

	err = e.centersSvc.ReplaceMemberOverrides(ctx, ownerScope, member.ID,
		centers.MemberOverridesRequest{Grants: []string{authctx.PermDataViewCenterWide}})
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code)

	err = e.centersSvc.ReplaceMemberOverrides(ctx, ownerScope, member.ID,
		centers.MemberOverridesRequest{Denies: []string{authctx.PermDataViewCenterWide}})
	require.Equal(t, apperror.CodeValidation, apperror.From(err).Code)
}

// A pre-migration data.view_center_wide row resolves through alias expansion:
// the member's very next scope carries every per-resource view_all key, both
// CenterWide() and CenterWideFor() widen, and a canonical deny narrows one
// resource without touching the rest.
func TestLegacyScopeRowExpandsOnResolve(t *testing.T) {
	t.Parallel()
	e := newEnv(t)

	owner, _ := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)
	ownerScope := e.scope(t, owner.ID)

	require.NoError(t, e.db.Exec(
		`INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed)
		 VALUES (?, ?, 'data.view_center_wide', TRUE)`, member.ID, ownerScope.CenterID).Error)

	sc := e.scope(t, member.ID)
	require.True(t, sc.CenterWide())
	require.True(t, sc.CenterWideFor(authctx.PermStudentsViewAll))
	require.True(t, sc.CenterWideFor(authctx.PermBillingViewAll))

	require.NoError(t, e.db.Exec(
		`INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed)
		 VALUES (?, ?, 'students.view_all', FALSE)`, member.ID, ownerScope.CenterID).Error)

	sc = e.scope(t, member.ID)
	require.False(t, sc.CenterWideFor(authctx.PermStudentsViewAll),
		"canonical deny narrows its resource")
	require.True(t, sc.CenterWideFor(authctx.PermClassesViewAll),
		"a single-canonical deny must not propagate to the other resources")
}

// Member-targeted endpoints collapse the owner, strangers, and other
// centers' members into the same 404 — no target-existence leak.
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
// reports.send grant resolving into scope in both directions, and the read
// model reflecting the stored lists.
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

	// Granting reports.send resolves into the member's next scope…
	require.NoError(t, e.centersSvc.ReplaceMemberOverrides(ctx, ownerScope, member.ID,
		centers.MemberOverridesRequest{
			Grants: []string{authctx.PermReportsSend, authctx.PermAuditRead},
			Denies: []string{authctx.PermDashboardView}}))
	sc := e.scope(t, member.ID)
	require.True(t, sc.CanSendReports)
	require.True(t, sc.Has(authctx.PermAuditRead))
	require.False(t, sc.Has(authctx.PermDashboardView))

	resp, err := e.centersSvc.Permissions(ctx, ownerScope)
	require.NoError(t, err)
	require.Equal(t, []string{authctx.PermReportsSend, authctx.PermAuditRead},
		resp.Members[0].Grants, "grants come back in registry order")
	require.Equal(t, []string{authctx.PermDashboardView}, resp.Members[0].Denies)

	// …and a replacement without it revokes on the very next resolve.
	require.NoError(t, e.centersSvc.ReplaceMemberOverrides(ctx, ownerScope, member.ID,
		centers.MemberOverridesRequest{Grants: []string{authctx.PermAuditRead}}))
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

// Every center-creation path seeds the system roles with the centralized
// default baseline: membership alone granted all operational access before
// the catalog, so a role born empty would silently revoke it at cutover.
func TestNewCenterRolesCarryDefaultBaseline(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, e.db)
	ownerScope := e.scope(t, owner.ID)

	// The read model serializes permissions in catalog order — exactly what
	// DefaultRoleKeys returns.
	want := authctx.DefaultRoleKeys()
	resp, err := e.centersSvc.Permissions(ctx, ownerScope)
	require.NoError(t, err)
	require.Len(t, resp.Roles, 3)
	for _, r := range resp.Roles {
		require.Equalf(t, want, r.Permissions,
			"role %s must be born with the default baseline", r.Key)
	}

	// The repository creation path (registration flow) seeds identically.
	repo := centers.NewRepository(e.db)
	fresh := &centers.Center{ID: id.New(), Name: "Trung tâm mới", OwnerID: owner.ID}
	// A second live center per owner is rejected by uq_centers_owner, so
	// retire the fixture center first.
	require.NoError(t, e.db.Exec(
		`UPDATE centers SET deleted_at = now() WHERE id = ?`, ownerScope.CenterID).Error)
	require.NoError(t, repo.CreateCenter(ctx, fresh))
	var n int64
	require.NoError(t, e.db.Raw(
		`SELECT count(*) FROM center_role_permissions rp
		 JOIN center_roles cr ON cr.id = rp.role_id
		 WHERE cr.center_id = ?`, fresh.ID).Scan(&n).Error)
	require.EqualValues(t, 3*len(authctx.DefaultRoleKeys()), n)
}

// Assignment writes are CAS-guarded: reads return the catalog version and a
// per-role / per-member assignment version, replacement writes echo them, a
// mismatch is a 409 that mutates nothing, and a client omitting the fields
// (version zero — pre-CAS clients) keeps last-write-wins.
func TestAssignmentVersionCAS(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)
	ownerScope := e.scope(t, owner.ID)
	roleID := e.roleID(t, ownerScope.CenterID, "hoc_vu")

	resp, err := e.centersSvc.Permissions(ctx, ownerScope)
	require.NoError(t, err)
	require.Equal(t, authctx.CatalogVersion, resp.CatalogVersion)
	for _, r := range resp.Roles {
		require.EqualValues(t, 1, r.AssignmentVersion, "roles start at version 1")
	}
	require.Len(t, resp.Members, 1)
	require.EqualValues(t, 1, resp.Members[0].AssignmentVersion)

	// A correct-version write lands and bumps the version.
	require.NoError(t, e.centersSvc.ReplaceRolePermissions(ctx, ownerScope, roleID,
		centers.RolePermissionsRequest{
			Permissions:       []string{authctx.PermClassesRead},
			CatalogVersion:    authctx.CatalogVersion,
			AssignmentVersion: 1,
		}))
	resp, err = e.centersSvc.Permissions(ctx, ownerScope)
	require.NoError(t, err)
	var hocVu *centers.RoleResponse
	for i := range resp.Roles {
		if resp.Roles[i].ID == roleID {
			hocVu = &resp.Roles[i]
		}
	}
	require.NotNil(t, hocVu)
	require.EqualValues(t, 2, hocVu.AssignmentVersion)
	require.Equal(t, []string{authctx.PermClassesRead}, hocVu.Permissions)

	// Replaying the stale version is a conflict and mutates nothing.
	err = e.centersSvc.ReplaceRolePermissions(ctx, ownerScope, roleID,
		centers.RolePermissionsRequest{
			Permissions:       []string{authctx.PermClassesEdit},
			CatalogVersion:    authctx.CatalogVersion,
			AssignmentVersion: 1,
		})
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)
	resp, err = e.centersSvc.Permissions(ctx, ownerScope)
	require.NoError(t, err)
	for _, r := range resp.Roles {
		if r.ID == roleID {
			require.Equal(t, []string{authctx.PermClassesRead}, r.Permissions,
				"a stale write must not mutate the role")
			require.EqualValues(t, 2, r.AssignmentVersion)
		}
	}

	// A stale catalog generation is refused before touching anything.
	err = e.centersSvc.ReplaceRolePermissions(ctx, ownerScope, roleID,
		centers.RolePermissionsRequest{
			Permissions:       []string{authctx.PermClassesEdit},
			CatalogVersion:    authctx.CatalogVersion - 1,
			AssignmentVersion: 2,
		})
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)

	// A pre-CAS client that sends no versions keeps last-write-wins.
	require.NoError(t, e.centersSvc.ReplaceRolePermissions(ctx, ownerScope, roleID,
		centers.RolePermissionsRequest{Permissions: []string{}}))

	// The same contract guards member overrides.
	require.NoError(t, e.centersSvc.ReplaceMemberOverrides(ctx, ownerScope, member.ID,
		centers.MemberOverridesRequest{
			Grants:            []string{authctx.PermAuditRead},
			CatalogVersion:    authctx.CatalogVersion,
			AssignmentVersion: 1,
		}))
	resp, err = e.centersSvc.Permissions(ctx, ownerScope)
	require.NoError(t, err)
	require.EqualValues(t, 2, resp.Members[0].AssignmentVersion)
	require.Equal(t, []string{authctx.PermAuditRead}, resp.Members[0].Grants)

	err = e.centersSvc.ReplaceMemberOverrides(ctx, ownerScope, member.ID,
		centers.MemberOverridesRequest{
			Denies:            []string{authctx.PermAuditRead},
			CatalogVersion:    authctx.CatalogVersion,
			AssignmentVersion: 1,
		})
	require.Equal(t, apperror.CodeConflict, apperror.From(err).Code)
	resp, err = e.centersSvc.Permissions(ctx, ownerScope)
	require.NoError(t, err)
	require.Equal(t, []string{authctx.PermAuditRead}, resp.Members[0].Grants,
		"a stale member write must not mutate the overrides")
	require.Empty(t, resp.Members[0].Denies)
}

// TestLegacyScopeKeyRoundTripStaysSavable pins the deprecated-key contract on
// the assignment read models. The backfill deliberately keeps legacy
// data.view_center_wide rows effective through the soak window, but the read
// model must never emit them: the PUT endpoints reject non-grantable keys, so
// an emitted legacy key would make every save of such a role or member fail
// 422 — for exactly the pre-catalog centers the backfill was written for.
func TestLegacyScopeKeyRoundTripStaysSavable(t *testing.T) {
	t.Parallel()
	e := newEnv(t)
	ctx := context.Background()

	owner, _ := testutil.Teacher(t, e.db)
	member, _ := testutil.Teacher(t, e.db)
	e.join(t, member.ID, owner.ID)
	ownerScope := e.scope(t, owner.ID)
	hocVu := e.roleID(t, ownerScope.CenterID, "hoc_vu")

	// A pre-catalog center: role and member still hold the legacy scope row.
	require.NoError(t, e.db.Exec(
		`INSERT INTO center_role_permissions (role_id, permission_key)
		 VALUES (?, 'data.view_center_wide')`, hocVu).Error)
	require.NoError(t, e.db.Exec(
		`INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed)
		 VALUES (?, ?, 'data.view_center_wide', TRUE)`,
		member.ID, ownerScope.CenterID).Error)

	resp, err := e.centersSvc.Permissions(ctx, ownerScope)
	require.NoError(t, err)
	require.NotContains(t, resp.Roles[1].Permissions, authctx.PermDataViewCenterWide,
		"deprecated keys stay out of the role read model")
	require.NotContains(t, resp.Members[0].Grants, authctx.PermDataViewCenterWide,
		"deprecated keys stay out of the override read model")

	// The UI saves exactly what it read back; neither write may 422.
	require.NoError(t, e.centersSvc.ReplaceRolePermissions(ctx, ownerScope, hocVu,
		centers.RolePermissionsRequest{Permissions: resp.Roles[1].Permissions}))
	require.NoError(t, e.centersSvc.ReplaceMemberOverrides(ctx, ownerScope, member.ID,
		centers.MemberOverridesRequest{
			Grants: resp.Members[0].Grants,
			Denies: resp.Members[0].Denies,
		}))
}
