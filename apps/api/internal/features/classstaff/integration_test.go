//go:build integration

package classstaff_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/centers"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/classstaff"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/testutil"
)

// newIntegrationService wires the real dependency chain router.go uses: the
// staff repository doubles as the classes StaffSeeder (create-hook), and the
// live-membership check runs through the centers service.
func newIntegrationService(t *testing.T) (*classstaff.Service, *classes.Service, *gorm.DB) {
	t.Helper()
	db := testutil.StartPostgres(t)
	txMgr := database.NewTxManager(db)
	staffRepo := classstaff.NewRepository(db)
	classesSvc := classes.NewService(classes.NewRepository(db), txMgr, staffRepo)
	centersSvc := centers.NewService(centers.NewRepository(db), txMgr, nil)
	svc := classstaff.NewService(staffRepo, centersSvc)
	return svc, classesSvc, db
}

func createClass(t *testing.T, svc *classes.Service, sc authctx.Scope, name string) *classes.Class {
	t.Helper()
	weekday := int16(1)
	price := int64(100000)
	class, err := svc.Create(context.Background(), sc, classes.CreateClassRequest{
		Name:      name,
		StartDate: "2026-01-05",
		Schedules: []classes.ScheduleRequest{
			{Weekday: &weekday, StartTime: "18:00", DurationMin: 90},
		},
		DefaultUnitPrice: &price,
	})
	require.NoError(t, err)
	return class
}

func requireStatus(t *testing.T, err error, status int) {
	t.Helper()
	require.Error(t, err)
	appErr := apperror.From(err)
	require.Equal(t, status, appErr.Status, "unexpected status for error: %v", err)
}

// Creating a class through the service seeds exactly one active giao_vien
// assignment for the creator inside the same transaction.
func TestClassCreateSeedsPrimaryTeacher(t *testing.T) {
	t.Parallel()
	svc, classesSvc, db := newIntegrationService(t)
	ctx := context.Background()
	_, owner := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, owner.ID)

	class := createClass(t, classesSvc, sc, "Toán 9A")

	staff, err := svc.List(ctx, sc, class.ID)
	require.NoError(t, err)
	require.Len(t, staff, 1)
	require.Equal(t, owner.ID, staff[0].TeacherID)
	require.Equal(t, authctx.StaffRoleGiaoVien, staff[0].RoleKey)
	require.Nil(t, staff[0].EndedAt)
	require.NotEmpty(t, staff[0].TeacherName)
}

// The owner assigns and lists hoc_vu / tro_giang staff; responses carry the
// teacher's display name and the role's Vietnamese label.
func TestOwnerAssignAndList(t *testing.T) {
	t.Parallel()
	svc, classesSvc, db := newIntegrationService(t)
	ctx := context.Background()
	_, owner := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, owner.ID)
	class := createClass(t, classesSvc, sc, "Văn 8B")

	_, clerk := testutil.Teacher(t, db, testutil.WithFullName("Cô Học Vụ"))
	testutil.JoinCenter(t, db, clerk.ID, sc.CenterID)
	_, assistant := testutil.Teacher(t, db, testutil.WithFullName("Thầy Trợ Giảng"))
	testutil.JoinCenter(t, db, assistant.ID, sc.CenterID)

	created, err := svc.Assign(ctx, sc, class.ID, classstaff.AssignRequest{
		TeacherID: clerk.ID, RoleKey: authctx.StaffRoleHocVu,
	})
	require.NoError(t, err)
	require.Equal(t, "Cô Học Vụ", created.TeacherName)
	require.Equal(t, "Học vụ", created.RoleLabel)

	_, err = svc.Assign(ctx, sc, class.ID, classstaff.AssignRequest{
		TeacherID: assistant.ID, RoleKey: authctx.StaffRoleTroGiang,
	})
	require.NoError(t, err)

	staff, err := svc.List(ctx, sc, class.ID)
	require.NoError(t, err)
	require.Len(t, staff, 3, "giao_vien from the create hook + the two assigned roles")
}

// Assignment validation: unknown role 422, giao_vien 409 (handoff owns it),
// duplicate active assignment 409, non-member target 400.
func TestAssignValidation(t *testing.T) {
	t.Parallel()
	svc, classesSvc, db := newIntegrationService(t)
	ctx := context.Background()
	_, owner := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, owner.ID)
	class := createClass(t, classesSvc, sc, "Lý 9C")

	_, member := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, member.ID, sc.CenterID)

	_, err := svc.Assign(ctx, sc, class.ID, classstaff.AssignRequest{TeacherID: member.ID, RoleKey: "thu_ky"})
	requireStatus(t, err, 422)

	_, err = svc.Assign(ctx, sc, class.ID, classstaff.AssignRequest{TeacherID: member.ID, RoleKey: authctx.StaffRoleGiaoVien})
	requireStatus(t, err, 409)

	_, err = svc.Assign(ctx, sc, class.ID, classstaff.AssignRequest{TeacherID: member.ID, RoleKey: authctx.StaffRoleHocVu})
	require.NoError(t, err)
	_, err = svc.Assign(ctx, sc, class.ID, classstaff.AssignRequest{TeacherID: member.ID, RoleKey: authctx.StaffRoleTroGiang})
	requireStatus(t, err, 409)

	// A teacher from a different center is not a live member here.
	_, outsider := testutil.Teacher(t, db)
	_, err = svc.Assign(ctx, sc, class.ID, classstaff.AssignRequest{TeacherID: outsider.ID, RoleKey: authctx.StaffRoleHocVu})
	requireStatus(t, err, 400)

	// A kicked member is no longer assignable. RemoveMember closes the
	// membership stint and disables the account in one transaction — mirror
	// both halves, since the live-member check reads the account status.
	_, gone := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, gone.ID, sc.CenterID)
	require.NoError(t, db.Exec(
		"UPDATE center_members SET left_at = now() WHERE teacher_id = ?", gone.ID).Error)
	require.NoError(t, db.Exec(
		"UPDATE user_accounts SET status = ? WHERE id = ?", teachers.StatusDisabled, gone.ID).Error)
	_, err = svc.Assign(ctx, sc, class.ID, classstaff.AssignRequest{TeacherID: gone.ID, RoleKey: authctx.StaffRoleHocVu})
	requireStatus(t, err, 400)
}

// Read access follows assignments: an assigned member lists staff (even after
// their assignment ends), an unassigned peer gets 404 (no existence leak), a
// non-owner with read access gets 403 on writes, cross-center is 404.
func TestAccessMatrix(t *testing.T) {
	t.Parallel()
	svc, classesSvc, db := newIntegrationService(t)
	ctx := context.Background()
	_, owner := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, owner.ID)
	class := createClass(t, classesSvc, sc, "Hóa 9D")

	_, clerk := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, clerk.ID, sc.CenterID)
	_, peer := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, peer.ID, sc.CenterID)

	created, err := svc.Assign(ctx, sc, class.ID, classstaff.AssignRequest{
		TeacherID: clerk.ID, RoleKey: authctx.StaffRoleHocVu,
	})
	require.NoError(t, err)

	clerkScope := testutil.ScopeFor(t, db, clerk.ID)
	peerScope := testutil.ScopeFor(t, db, peer.ID)

	_, err = svc.List(ctx, clerkScope, class.ID)
	require.NoError(t, err, "an assigned member reads the staff list")

	_, err = svc.List(ctx, peerScope, class.ID)
	requireStatus(t, err, 404)

	// Writes stay owner-only: an assigned member is 403 (they can read the
	// class, so its existence is no secret), an unassigned peer stays 404.
	_, err = svc.Assign(ctx, clerkScope, class.ID, classstaff.AssignRequest{
		TeacherID: peer.ID, RoleKey: authctx.StaffRoleTroGiang,
	})
	requireStatus(t, err, 403)
	_, err = svc.Assign(ctx, peerScope, class.ID, classstaff.AssignRequest{
		TeacherID: clerk.ID, RoleKey: authctx.StaffRoleTroGiang,
	})
	requireStatus(t, err, 404)
	err = svc.Remove(ctx, clerkScope, class.ID, created.ID, false)
	requireStatus(t, err, 403)

	// Ended assignment keeps read access (soft-close = history stays).
	require.NoError(t, svc.Remove(ctx, sc, class.ID, created.ID, false))
	_, err = svc.List(ctx, clerkScope, class.ID)
	require.NoError(t, err, "a soft-closed assignment still grants history reads")

	// Cross-center: another center's owner sees nothing.
	_, otherOwner := testutil.Teacher(t, db)
	otherScope := testutil.ScopeFor(t, db, otherOwner.ID)
	_, err = svc.List(ctx, otherScope, class.ID)
	requireStatus(t, err, 404)
	err = svc.Remove(ctx, otherScope, class.ID, created.ID, true)
	requireStatus(t, err, 404)
}

// A member granted classes.view_all reads any class's staff list — the gate
// follows CenterWideFor(classes.view_all), the same rule the classes read
// port applies, so whoever can GET the class never 404s on its staff. Writes
// stay owner-only regardless.
func TestCenterWideMemberReadsStaffList(t *testing.T) {
	t.Parallel()
	svc, classesSvc, db := newIntegrationService(t)
	ctx := context.Background()
	_, owner := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, owner.ID)
	class := createClass(t, classesSvc, sc, "Sinh 9E")

	_, wide := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, wide.ID, sc.CenterID)
	wideScope := testutil.ScopeFor(t, db, wide.ID)

	// Without the permission the unassigned member is an ordinary peer: 404.
	_, err := svc.List(ctx, wideScope, class.ID)
	requireStatus(t, err, 404)

	wideScope.Perms = authctx.BuildPermSet(nil, []string{authctx.PermClassesViewAll}, nil)
	staff, err := svc.List(ctx, wideScope, class.ID)
	require.NoError(t, err, "a center-wide reader lists staff of an unassigned class")
	require.Len(t, staff, 1)

	// The widened read does not widen writes: they can see the class, so the
	// refusal is 403, not 404.
	_, err = svc.Assign(ctx, wideScope, class.ID, classstaff.AssignRequest{
		TeacherID: wide.ID, RoleKey: authctx.StaffRoleHocVu,
	})
	requireStatus(t, err, 403)
}

// Removal: default soft-close stamps ended_at and keeps the row; a second
// soft-close is 404; void hard-deletes (the revocation path for a mistaken
// grant — including one already soft-closed). An active giao_vien refuses
// both modes with 409.
func TestRemoveSoftCloseAndVoid(t *testing.T) {
	t.Parallel()
	svc, classesSvc, db := newIntegrationService(t)
	ctx := context.Background()
	_, owner := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, owner.ID)
	class := createClass(t, classesSvc, sc, "Sinh 9E")

	_, clerk := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, clerk.ID, sc.CenterID)

	created, err := svc.Assign(ctx, sc, class.ID, classstaff.AssignRequest{
		TeacherID: clerk.ID, RoleKey: authctx.StaffRoleHocVu,
	})
	require.NoError(t, err)

	require.NoError(t, svc.Remove(ctx, sc, class.ID, created.ID, false))
	staff, err := svc.List(ctx, sc, class.ID)
	require.NoError(t, err)
	var closed *classstaff.StaffResponse
	for i := range staff {
		if staff[i].ID == created.ID {
			closed = &staff[i]
		}
	}
	require.NotNil(t, closed, "a soft-closed assignment stays listed")
	require.NotNil(t, closed.EndedAt)

	err = svc.Remove(ctx, sc, class.ID, created.ID, false)
	requireStatus(t, err, 404)

	// Void erases the row — even an already-ended one.
	require.NoError(t, svc.Remove(ctx, sc, class.ID, created.ID, true))
	var n int64
	require.NoError(t, db.Raw("SELECT count(*) FROM class_staff WHERE id = ?", created.ID).Scan(&n).Error)
	require.Zero(t, n)

	// The active primary teacher can only leave through a handoff.
	staff, err = svc.List(ctx, sc, class.ID)
	require.NoError(t, err)
	require.Len(t, staff, 1)
	gvID := staff[0].ID
	err = svc.Remove(ctx, sc, class.ID, gvID, false)
	requireStatus(t, err, 409)
	err = svc.Remove(ctx, sc, class.ID, gvID, true)
	requireStatus(t, err, 409)

	// A staff id from another class in the same center is 404, not a leak.
	other := createClass(t, classesSvc, sc, "Sử 9G")
	err = svc.Remove(ctx, sc, other.ID, gvID, false)
	requireStatus(t, err, 404)
}

// SyncPrimaryTeacher is the shared dual-write primitive: it must close a
// drifted giao_vien, promote a target with another active role, and stay
// idempotent — the invariant is "exactly one active giao_vien = the target".
func TestSyncPrimaryTeacherSelfHeals(t *testing.T) {
	t.Parallel()
	svc, classesSvc, db := newIntegrationService(t)
	ctx := context.Background()
	_, owner := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, owner.ID)
	class := createClass(t, classesSvc, sc, "Địa 9H")

	repo := classstaff.NewRepository(db)

	// Idempotent: syncing the current teacher changes nothing.
	require.NoError(t, repo.SyncPrimaryTeacher(ctx, class.ID, sc.CenterID, owner.ID))
	requireActivePrimary(t, db, class.ID, owner.ID, 1)

	// Drift: someone deleted the assignment row by hand — sync restores it.
	require.NoError(t, db.Exec("DELETE FROM class_staff WHERE class_id = ?", class.ID).Error)
	require.NoError(t, repo.SyncPrimaryTeacher(ctx, class.ID, sc.CenterID, owner.ID))
	requireActivePrimary(t, db, class.ID, owner.ID, 1)

	// Promotion: the new primary already holds an active hoc_vu assignment —
	// sync closes it and inserts the giao_vien stint.
	_, next := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, next.ID, sc.CenterID)
	_, err := svc.Assign(ctx, sc, class.ID, classstaff.AssignRequest{
		TeacherID: next.ID, RoleKey: authctx.StaffRoleHocVu,
	})
	require.NoError(t, err)
	require.NoError(t, repo.SyncPrimaryTeacher(ctx, class.ID, sc.CenterID, next.ID))
	requireActivePrimary(t, db, class.ID, next.ID, 1)

	// The old primary's stint is closed, not deleted (history reads survive).
	var oldRows int64
	require.NoError(t, db.Raw(
		"SELECT count(*) FROM class_staff WHERE class_id = ? AND teacher_id = ? AND ended_at IS NOT NULL",
		class.ID, owner.ID).Scan(&oldRows).Error)
	require.Equal(t, int64(1), oldRows)
}

func requireActivePrimary(t *testing.T, db *gorm.DB, classID, teacherID uuid.UUID, wantRows int64) {
	t.Helper()
	var got struct {
		N       int64
		Teacher uuid.UUID
	}
	require.NoError(t, db.Raw(`
		SELECT count(*) AS n, min(teacher_id::text)::uuid AS teacher
		FROM class_staff
		WHERE class_id = ? AND role_key = 'giao_vien' AND ended_at IS NULL`,
		classID).Scan(&got).Error)
	require.Equal(t, wantRows, got.N, "exactly one active giao_vien expected")
	require.Equal(t, teacherID, got.Teacher)
}

// A teacher who left the center must never regain access through the
// dual-write primitive: syncing a kicked member is a full no-op — no stint
// is inserted for them and the current primary's stint stays untouched —
// instead of resurrecting their class access.
func TestSyncPrimaryTeacherSkipsKickedMember(t *testing.T) {
	t.Parallel()
	_, classesSvc, db := newIntegrationService(t)
	ctx := context.Background()
	_, owner := testutil.Teacher(t, db)
	sc := testutil.ScopeFor(t, db, owner.ID)
	class := createClass(t, classesSvc, sc, "Lý 9K")
	repo := classstaff.NewRepository(db)

	_, gone := testutil.Teacher(t, db)
	testutil.JoinCenter(t, db, gone.ID, sc.CenterID)
	require.NoError(t, db.Exec(
		"UPDATE center_members SET left_at = now() WHERE teacher_id = ? AND center_id = ?",
		gone.ID, sc.CenterID).Error)

	require.NoError(t, repo.SyncPrimaryTeacher(ctx, class.ID, sc.CenterID, gone.ID))

	var goneRows int64
	require.NoError(t, db.Raw(
		"SELECT count(*) FROM class_staff WHERE class_id = ? AND teacher_id = ?",
		class.ID, gone.ID).Scan(&goneRows).Error)
	require.Zero(t, goneRows, "a kicked member must not gain a stint")
	requireActivePrimary(t, db, class.ID, owner.ID, 1)
}
