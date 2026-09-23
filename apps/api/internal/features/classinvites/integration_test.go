//go:build integration

package classinvites_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/auth"
	"teka/apps/api/internal/features/centers"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/classinvites"
	"teka/apps/api/internal/features/classstaff"
	"teka/apps/api/internal/features/enrollments"
	"teka/apps/api/internal/features/handoff"
	"teka/apps/api/internal/features/imports"
	"teka/apps/api/internal/features/sessions"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/testutil"
)

// noopDMSender satisfies auth.ResetDMSender; no test here sends a DM.
type noopDMSender struct{}

func (noopDMSender) LookupPhone(_ context.Context, _ uuid.UUID, _ string) (string, bool, error) {
	return "", false, nil
}

func (noopDMSender) SendDM(_ context.Context, _ uuid.UUID, _, _ string) (string, error) {
	return "", nil
}

// fixture is one center: an owner, two member teachers (a, b), and the real
// service graph router.go wires — centers, classes, classstaff, handoff —
// so confirm exercises the actual stint writers and the centers callback
// runs against the real RemoveMember transaction.
type fixture struct {
	db      *gorm.DB
	svc     *classinvites.Service
	centers *centers.Service
	owner   authctx.Scope
	a, b    uuid.UUID
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	db := testutil.StartPostgres(t)
	txMgr := database.NewTxManager(db)

	teachersSvc := teachers.NewService(teachers.NewRepository(db))
	centersSvc := centers.NewService(centers.NewRepository(db), txMgr, nil)
	jwtCfg := config.JWTConfig{Secret: testutil.JWTSecret, AccessTTL: 15 * time.Minute}
	onboardingCfg := config.OnboardingConfig{ResetTTL: 48 * time.Hour, ResetCooldown: 15 * time.Minute}
	authSvc := auth.NewService(teachersSvc, auth.NewRepository(db), auth.NewTokenIssuer(jwtCfg), txMgr,
		centersSvc, noopDMSender{}, onboardingCfg, "https://app.example.com", nil)
	centersSvc.SetAccountDisabler(authSvc)

	staffRepo := classstaff.NewRepository(db)
	classesSvc := classes.NewService(classes.NewRepository(db), txMgr, staffRepo)
	staffSvc := classstaff.NewService(staffRepo, centersSvc)
	enrollmentsSvc := enrollments.NewService(enrollments.NewRepository(db), nil)
	sessionsSvc := sessions.NewService(sessions.NewRepository(db), classesSvc, teachersSvc, enrollmentsSvc)
	handoffSvc := handoff.NewService(classesSvc, sessionsSvc, centersSvc, staffRepo, imports.NewLocker(db), txMgr)

	svc := classinvites.NewService(classinvites.NewRepository(db), classesSvc, centersSvc, staffRepo,
		staffSvc, handoffSvc, txMgr)
	centersSvc.SetClassInviteCanceller(svc)

	_, ownerTeacher := testutil.Teacher(t, db, testutil.WithFullName("Chủ trung tâm"))
	owner := testutil.ScopeFor(t, db, ownerTeacher.ID)
	require.True(t, owner.IsOwner)
	_, a := testutil.Teacher(t, db, testutil.WithFullName("Giáo viên A"))
	_, b := testutil.Teacher(t, db, testutil.WithFullName("Giáo viên B"))
	testutil.JoinCenter(t, db, a.ID, owner.CenterID)
	testutil.JoinCenter(t, db, b.ID, owner.CenterID)

	return fixture{db: db, svc: svc, centers: centersSvc, owner: owner, a: a.ID, b: b.ID}
}

func (f fixture) scope(t *testing.T, teacherID uuid.UUID) authctx.Scope {
	t.Helper()
	return testutil.ScopeFor(t, f.db, teacherID)
}

// classOf creates a class taught by the owner, with the giao_vien stint the
// classes fixture seeds.
func (f fixture) classOf(t *testing.T, teacherID uuid.UUID) *classes.Class {
	t.Helper()
	class := testutil.Class(t, f.db, teacherID)
	testutil.Schedule(t, f.db, class, 1, "18:00")
	return class
}

func (f fixture) send(t *testing.T, classID, teacherID uuid.UUID, role string) *classinvites.InvitationResponse {
	t.Helper()
	inv, err := f.svc.Send(context.Background(), f.owner, classID, classinvites.SendRequest{
		TeacherID: teacherID, RoleKey: role,
	})
	require.NoError(t, err)
	return inv
}

type staffRow struct {
	TeacherID uuid.UUID
	RoleKey   string
	EndedAt   *time.Time
}

func (f fixture) staffOf(t *testing.T, classID uuid.UUID) []staffRow {
	t.Helper()
	var rows []staffRow
	require.NoError(t, f.db.Raw(
		"SELECT teacher_id, role_key, ended_at FROM class_staff WHERE class_id = ? ORDER BY started_at, role_key",
		classID).Scan(&rows).Error)
	return rows
}

func (f fixture) activeStaff(t *testing.T, classID uuid.UUID) []staffRow {
	t.Helper()
	var out []staffRow
	for _, r := range f.staffOf(t, classID) {
		if r.EndedAt == nil {
			out = append(out, r)
		}
	}
	return out
}

func (f fixture) status(t *testing.T, id uuid.UUID) string {
	t.Helper()
	var row struct{ Status string }
	require.NoError(t, f.db.Raw("SELECT status FROM class_invitations WHERE id = ?", id).Scan(&row).Error)
	return row.Status
}

func requireAppError(t *testing.T, err error, status int) *apperror.AppError {
	t.Helper()
	var appErr *apperror.AppError
	require.True(t, errors.As(err, &appErr), "want AppError %d, got %v", status, err)
	require.Equal(t, status, appErr.Status, "message: %s", appErr.Message)
	return appErr
}

// requireStatus asserts the status only; use requireAppError when the code or fields matter too.
func requireStatus(t *testing.T, err error, status int) {
	t.Helper()
	_ = requireAppError(t, err, status)
}

func TestInvitationIsPrivateToInvitee(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	class := f.classOf(t, f.owner.TeacherID)
	inv := f.send(t, class.ID, f.b, authctx.StaffRoleTroGiang)
	require.Equal(t, classinvites.StatusPending, inv.Status)
	require.Equal(t, "Giáo viên B", inv.TeacherName)
	require.Equal(t, "Chủ trung tâm", inv.InvitedByName)
	require.Equal(t, class.Name, inv.ClassName)

	// A lists nothing and cannot accept B's invitation — 404, not 403, so
	// its existence never leaks.
	listed, err := f.svc.List(context.Background(), f.scope(t, f.a), classinvites.ListQuery{})
	require.NoError(t, err)
	require.Empty(t, listed)
	_, err = f.svc.Accept(context.Background(), f.scope(t, f.a), inv.ID)
	requireStatus(t, err, 404)

	// The owner sees the whole center.
	listed, err = f.svc.List(context.Background(), f.owner, classinvites.ListQuery{Status: "pending"})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, inv.ID, listed[0].ID)

	// B sees and accepts it.
	listed, err = f.svc.List(context.Background(), f.scope(t, f.b), classinvites.ListQuery{})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	accepted, err := f.svc.Accept(context.Background(), f.scope(t, f.b), inv.ID)
	require.NoError(t, err)
	require.Equal(t, classinvites.StatusAccepted, accepted.Status)
	require.NotNil(t, accepted.RespondedAt)
}

func TestAcceptWritesNoStint(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	class := f.classOf(t, f.owner.TeacherID)
	before := f.staffOf(t, class.ID)
	require.Len(t, before, 1, "the class fixture seeds the owner's giao_vien stint")

	inv := f.send(t, class.ID, f.a, authctx.StaffRoleHocVu)
	_, err := f.svc.Accept(context.Background(), f.scope(t, f.a), inv.ID)
	require.NoError(t, err)
	require.Equal(t, before, f.staffOf(t, class.ID), "accept must not touch class_staff")

	// Confirm by the owner is what writes the stint.
	got, err := f.svc.Confirm(context.Background(), f.owner, inv.ID)
	require.NoError(t, err)
	require.Equal(t, classinvites.StatusAssigned, got.Status)
	require.NotNil(t, got.AssignedAt)
	require.Zero(t, got.MovedPlannedSessions)
	active := f.activeStaff(t, class.ID)
	require.Len(t, active, 2)
	require.Equal(t, staffRow{TeacherID: f.a, RoleKey: authctx.StaffRoleHocVu}, active[1])
}

func TestConfirmGiaoVienHandsOverTheClass(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	// A currently teaches the class; the owner invites B as giao_vien.
	class := f.classOf(t, f.a)
	nowLocal := time.Now()
	today := time.Date(nowLocal.Year(), nowLocal.Month(), nowLocal.Day(), 0, 0, 0, 0, time.UTC)
	future := testutil.Session(t, f.db, f.a, class.ID, today.AddDate(0, 0, 7))
	past := testutil.Session(t, f.db, f.a, class.ID, today.AddDate(0, 0, -7))

	inv := f.send(t, class.ID, f.b, authctx.StaffRoleGiaoVien)
	// Pending is enough for the owner to confirm — acceptance is optional.
	got, err := f.svc.Confirm(context.Background(), f.owner, inv.ID)
	require.NoError(t, err)
	require.Equal(t, classinvites.StatusAssigned, got.Status)
	require.Equal(t, int64(1), got.MovedPlannedSessions)

	var cls struct{ TeacherID uuid.UUID }
	require.NoError(t, f.db.Raw("SELECT teacher_id FROM classes WHERE id = ?", class.ID).Scan(&cls).Error)
	require.Equal(t, f.b, cls.TeacherID)

	// The old giao_vien stint closed and exactly one is active: the partial
	// unique index uq_class_staff_one_gv still holds.
	active := f.activeStaff(t, class.ID)
	require.Len(t, active, 1)
	require.Equal(t, staffRow{TeacherID: f.b, RoleKey: authctx.StaffRoleGiaoVien}, active[0])
	all := f.staffOf(t, class.ID)
	require.Len(t, all, 2)
	require.Equal(t, f.a, all[0].TeacherID)
	require.NotNil(t, all[0].EndedAt)

	var sess struct{ TeacherID uuid.UUID }
	require.NoError(t, f.db.Raw("SELECT teacher_id FROM class_sessions WHERE id = ?", future.ID).Scan(&sess).Error)
	require.Equal(t, f.b, sess.TeacherID)
	require.NoError(t, f.db.Raw("SELECT teacher_id FROM class_sessions WHERE id = ?", past.ID).Scan(&sess).Error)
	require.Equal(t, f.a, sess.TeacherID)

	// A second confirm has nothing left to do.
	_, err = f.svc.Confirm(context.Background(), f.owner, inv.ID)
	requireStatus(t, err, 409)
}

func TestConfirmRollsBackWhenStintWriteFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	class := f.classOf(t, f.owner.TeacherID)
	inv := f.send(t, class.ID, f.a, authctx.StaffRoleTroGiang)
	// A already holds tro_giang by the time the owner confirms (assigned
	// directly in between): classstaff.Assign refuses with 409 and the
	// status change must roll back with it.
	testutil.StaffAssignment(t, f.db, class, f.a, authctx.StaffRoleTroGiang)

	_, err := f.svc.Confirm(context.Background(), f.owner, inv.ID)
	requireStatus(t, err, 409)
	require.Equal(t, classinvites.StatusPending, f.status(t, inv.ID))
}

func TestOnlyOwnerSendsAndSelfInviteIsRefused(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	class := f.classOf(t, f.owner.TeacherID)

	_, err := f.svc.Send(context.Background(), f.scope(t, f.a), class.ID, classinvites.SendRequest{
		TeacherID: f.b, RoleKey: authctx.StaffRoleTroGiang,
	})
	requireStatus(t, err, 403)

	_, err = f.svc.Send(context.Background(), f.owner, class.ID, classinvites.SendRequest{
		TeacherID: f.owner.TeacherID, RoleKey: authctx.StaffRoleTroGiang,
	})
	appErr := requireAppError(t, err, 422)
	require.Equal(t, classinvites.CodeSelfInvite, appErr.Code)

	// The same person cannot hold two pending invitations on one class.
	f.send(t, class.ID, f.a, authctx.StaffRoleTroGiang)
	_, err = f.svc.Send(context.Background(), f.owner, class.ID, classinvites.SendRequest{
		TeacherID: f.a, RoleKey: authctx.StaffRoleHocVu,
	})
	requireStatus(t, err, 409)

	// Cancel, remind, confirm are owner gates even for the invitee.
	listed, err := f.svc.List(context.Background(), f.scope(t, f.a), classinvites.ListQuery{})
	require.NoError(t, err)
	require.Len(t, listed, 1)
	_, err = f.svc.Cancel(context.Background(), f.scope(t, f.a), listed[0].ID)
	requireStatus(t, err, 403)
	_, err = f.svc.Confirm(context.Background(), f.scope(t, f.a), listed[0].ID)
	requireStatus(t, err, 403)

	reminded, err := f.svc.Remind(context.Background(), f.owner, listed[0].ID)
	require.NoError(t, err)
	require.NotNil(t, reminded.RemindedAt)
	cancelled, err := f.svc.Cancel(context.Background(), f.owner, listed[0].ID)
	require.NoError(t, err)
	require.Equal(t, classinvites.StatusCancelled, cancelled.Status)
}

func TestRemovedMemberLosesOpenInvitations(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	class := f.classOf(t, f.owner.TeacherID)
	other := f.classOf(t, f.owner.TeacherID)
	pendingInv := f.send(t, class.ID, f.a, authctx.StaffRoleTroGiang)
	acceptedInv := f.send(t, other.ID, f.a, authctx.StaffRoleHocVu)
	_, err := f.svc.Accept(context.Background(), f.scope(t, f.a), acceptedInv.ID)
	require.NoError(t, err)
	declinedInv := f.send(t, class.ID, f.b, authctx.StaffRoleHocVu)
	_, err = f.svc.Decline(context.Background(), f.scope(t, f.b), declinedInv.ID)
	require.NoError(t, err)

	// Snapshot A's scope before the membership closes: this is the token a
	// removed member may still be holding.
	aScope := f.scope(t, f.a)
	require.NoError(t, f.centers.RemoveMember(context.Background(), f.owner, f.a))

	require.Equal(t, classinvites.StatusCancelled, f.status(t, pendingInv.ID))
	require.Equal(t, classinvites.StatusCancelled, f.status(t, acceptedInv.ID))
	require.Equal(t, classinvites.StatusDeclined, f.status(t, declinedInv.ID), "B's row is untouched")

	// A can no longer be invited, and the cancelled row is not acceptable.
	_, err = f.svc.Send(context.Background(), f.owner, other.ID, classinvites.SendRequest{
		TeacherID: f.a, RoleKey: authctx.StaffRoleTroGiang,
	})
	appErr := requireAppError(t, err, 422)
	require.Equal(t, classinvites.CodeMemberInactive, appErr.Code)
	_, err = f.svc.Accept(context.Background(), aScope, pendingInv.ID)
	requireStatus(t, err, 404)
	_, err = f.svc.Confirm(context.Background(), f.owner, pendingInv.ID)
	requireStatus(t, err, 409)
}

func TestConfirmRefusesInviteeWhoLeftBeforeCancellation(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	class := f.classOf(t, f.owner.TeacherID)
	inv := f.send(t, class.ID, f.a, authctx.StaffRoleTroGiang)
	// The account was disabled without the invitation callback (an older
	// offboarding path): the service must still refuse to write a stint for
	// someone who is no longer an active member.
	require.NoError(t, f.db.Exec(
		"UPDATE user_accounts SET status = ? WHERE id = ?", teachers.StatusDisabled, f.a).Error)

	_, err := f.svc.Confirm(context.Background(), f.owner, inv.ID)
	appErr := requireAppError(t, err, 422)
	require.Equal(t, classinvites.CodeMemberInactive, appErr.Code)
	require.Equal(t, classinvites.StatusPending, f.status(t, inv.ID))
	require.Len(t, f.activeStaff(t, class.ID), 1)
}
