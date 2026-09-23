package classinvites

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
)

type testDeps struct {
	repo     *fakeRepo
	classes  *fakeClasses
	members  *fakeMembers
	roles    *fakeStaffRoles
	assigner *fakeAssigner
	tx       *fakeTx
	center   uuid.UUID
	ownerID  uuid.UUID
	memberID uuid.UUID
	classID  uuid.UUID
}

func newTestService() (*Service, *testDeps) {
	d := &testDeps{
		repo:     newFakeRepo(),
		center:   uuid.New(),
		ownerID:  uuid.New(),
		memberID: uuid.New(),
		classID:  uuid.New(),
		roles:    &fakeStaffRoles{roles: map[uuid.UUID][]string{}},
		assigner: &fakeAssigner{},
		tx:       &fakeTx{},
	}
	d.classes = &fakeClasses{class: &classes.Class{ID: d.classID, CenterID: d.center, TeacherID: d.ownerID, Name: "Toán 9A"}}
	d.members = &fakeMembers{active: map[uuid.UUID]bool{d.memberID: true, d.ownerID: true}}
	svc := NewService(d.repo, d.classes, d.members, d.roles, d.assigner, d.assigner, d.tx)
	return svc, d
}

func (d *testDeps) owner() authctx.Scope {
	return authctx.Scope{TeacherID: d.ownerID, CenterID: d.center, IsOwner: true}
}

func (d *testDeps) member(id uuid.UUID) authctx.Scope {
	return authctx.Scope{TeacherID: id, CenterID: d.center}
}

func (d *testDeps) pending(teacherID uuid.UUID, role string) *Row {
	return d.repo.add(Invitation{
		ID: uuid.New(), CenterID: d.center, ClassID: d.classID, TeacherID: teacherID,
		RoleKey: role, Status: StatusPending, InvitedBy: d.ownerID,
	}, "Toán 9A", "Thầy B")
}

func requireAppError(t *testing.T, err error, status int) *apperror.AppError {
	t.Helper()
	var appErr *apperror.AppError
	if !errors.As(err, &appErr) {
		t.Fatalf("want AppError %d, got %v", status, err)
	}
	if appErr.Status != status {
		t.Fatalf("want status %d, got %d (%s)", status, appErr.Status, appErr.Message)
	}
	return appErr
}

// requireStatus asserts the status only; use requireAppError when the code or fields matter too.
func requireStatus(t *testing.T, err error, status int) {
	t.Helper()
	_ = requireAppError(t, err, status)
}

func TestSendCreatesPendingInvitation(t *testing.T) {
	svc, d := newTestService()
	got, err := svc.Send(context.Background(), d.owner(), d.classID, SendRequest{TeacherID: d.memberID, RoleKey: authctx.StaffRoleTroGiang})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if got.Status != StatusPending || got.RoleLabel != "Trợ giảng" || got.TeacherID != d.memberID {
		t.Fatalf("unexpected response %+v", got)
	}
	if len(d.members.checked) != 1 || d.members.checked[0] != d.memberID {
		t.Fatalf("membership must be checked for the invitee, got %v", d.members.checked)
	}
}

func TestSendRefusesNonOwner(t *testing.T) {
	svc, d := newTestService()
	_, err := svc.Send(context.Background(), d.member(d.memberID), d.classID, SendRequest{TeacherID: d.ownerID, RoleKey: authctx.StaffRoleHocVu})
	requireStatus(t, err, 403)
}

func TestSendRefusesSelfInvite(t *testing.T) {
	svc, d := newTestService()
	_, err := svc.Send(context.Background(), d.owner(), d.classID, SendRequest{TeacherID: d.ownerID, RoleKey: authctx.StaffRoleHocVu})
	appErr := requireAppError(t, err, 422)
	if appErr.Code != CodeSelfInvite {
		t.Fatalf("want code %s, got %s", CodeSelfInvite, appErr.Code)
	}
}

func TestSendValidation(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()

	_, err := svc.Send(ctx, d.owner(), uuid.New(), SendRequest{TeacherID: d.memberID, RoleKey: authctx.StaffRoleHocVu})
	requireStatus(t, err, 404)

	_, err = svc.Send(ctx, d.owner(), d.classID, SendRequest{TeacherID: d.memberID, RoleKey: "thu_ky"})
	requireStatus(t, err, 422)

	stranger := uuid.New()
	_, err = svc.Send(ctx, d.owner(), d.classID, SendRequest{TeacherID: stranger, RoleKey: authctx.StaffRoleHocVu})
	if appErr := requireAppError(t, err, 422); appErr.Code != CodeMemberInactive {
		t.Fatalf("want code %s, got %s", CodeMemberInactive, appErr.Code)
	}

	d.roles.roles[d.classID] = []string{authctx.StaffRoleHocVu}
	_, err = svc.Send(ctx, d.owner(), d.classID, SendRequest{TeacherID: d.memberID, RoleKey: authctx.StaffRoleHocVu})
	requireStatus(t, err, 409)

	// class_staff allows one active stint per person: a support role on top
	// of another support role can never be confirmed, so it is refused now…
	d.roles.roles[d.classID] = []string{authctx.StaffRoleTroGiang}
	_, err = svc.Send(ctx, d.owner(), d.classID, SendRequest{TeacherID: d.memberID, RoleKey: authctx.StaffRoleHocVu})
	requireStatus(t, err, 409)
	d.roles.roles[d.classID] = []string{authctx.StaffRoleGiaoVien}
	_, err = svc.Send(ctx, d.owner(), d.classID, SendRequest{TeacherID: d.memberID, RoleKey: authctx.StaffRoleTroGiang})
	requireStatus(t, err, 409)

	// …while giao_vien for a current tro_giang is a handoff that closes the
	// old stint, hence a valid proposal.
	d.roles.roles[d.classID] = []string{authctx.StaffRoleTroGiang}
	if _, err = svc.Send(ctx, d.owner(), d.classID, SendRequest{TeacherID: d.memberID, RoleKey: authctx.StaffRoleGiaoVien}); err != nil {
		t.Fatalf("giao_vien should be invitable for a tro_giang: %v", err)
	}
	// The second pending for the same person collides.
	_, err = svc.Send(ctx, d.owner(), d.classID, SendRequest{TeacherID: d.memberID, RoleKey: authctx.StaffRoleGiaoVien})
	requireStatus(t, err, 409)
}

func TestListScopesMemberToOwnInvitations(t *testing.T) {
	svc, d := newTestService()
	other := uuid.New()
	d.pending(d.memberID, authctx.StaffRoleTroGiang)
	d.pending(other, authctx.StaffRoleHocVu)
	ctx := context.Background()

	mine, err := svc.List(ctx, d.member(d.memberID), ListQuery{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(mine) != 1 || mine[0].TeacherID != d.memberID {
		t.Fatalf("member must only see own invitations, got %+v", mine)
	}

	all, err := svc.List(ctx, d.owner(), ListQuery{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("owner sees the whole center, got %d", len(all))
	}

	_, err = svc.List(ctx, d.owner(), ListQuery{Status: "weird"})
	requireStatus(t, err, 422)
}

func TestAcceptOnlyByInvitee(t *testing.T) {
	svc, d := newTestService()
	other := uuid.New()
	inv := d.pending(d.memberID, authctx.StaffRoleTroGiang)
	ctx := context.Background()

	_, err := svc.Accept(ctx, d.member(other), inv.ID)
	requireStatus(t, err, 404)
	_, err = svc.Accept(ctx, d.owner(), inv.ID)
	requireStatus(t, err, 404)

	got, err := svc.Accept(ctx, d.member(d.memberID), inv.ID)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if got.Status != StatusAccepted {
		t.Fatalf("want accepted, got %s", got.Status)
	}
	if len(d.assigner.assigns) != 0 || len(d.assigner.reassigns) != 0 {
		t.Fatal("accept must not write class_staff")
	}
	_, err = svc.Accept(ctx, d.member(d.memberID), inv.ID)
	requireStatus(t, err, 409)

	// A member who left the center gets a 404 on their own row, even with a
	// token issued before they left.
	left := d.pending(d.memberID, authctx.StaffRoleHocVu)
	d.members.active[d.memberID] = false
	_, err = svc.Accept(ctx, d.member(d.memberID), left.ID)
	requireStatus(t, err, 404)
	_, err = svc.Decline(ctx, d.member(d.memberID), left.ID)
	requireStatus(t, err, 404)
}

func TestSendStoresBlankMessageAsNull(t *testing.T) {
	svc, d := newTestService()
	blank := "   "
	got, err := svc.Send(context.Background(), d.owner(), d.classID, SendRequest{
		TeacherID: d.memberID, RoleKey: authctx.StaffRoleTroGiang, Message: &blank,
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if got.Message != nil {
		t.Fatalf("blank message must be stored as null, got %q", *got.Message)
	}
}

func TestDeclineOnlyPending(t *testing.T) {
	svc, d := newTestService()
	inv := d.pending(d.memberID, authctx.StaffRoleTroGiang)
	ctx := context.Background()
	got, err := svc.Decline(ctx, d.member(d.memberID), inv.ID)
	if err != nil || got.Status != StatusDeclined {
		t.Fatalf("decline: %v %+v", err, got)
	}
	_, err = svc.Decline(ctx, d.member(d.memberID), inv.ID)
	requireStatus(t, err, 409)
}

func TestOwnerActionsRefuseNonOwner(t *testing.T) {
	svc, d := newTestService()
	inv := d.pending(d.memberID, authctx.StaffRoleTroGiang)
	ctx := context.Background()
	sc := d.member(d.memberID)
	_, err := svc.Cancel(ctx, sc, inv.ID)
	requireStatus(t, err, 403)
	_, err = svc.Remind(ctx, sc, inv.ID)
	requireStatus(t, err, 403)
	_, err = svc.Confirm(ctx, sc, inv.ID)
	requireStatus(t, err, 403)
}

func TestCancelAllowsPendingAndAccepted(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	inv := d.pending(d.memberID, authctx.StaffRoleTroGiang)
	inv.Status = StatusAccepted
	got, err := svc.Cancel(ctx, d.owner(), inv.ID)
	if err != nil || got.Status != StatusCancelled {
		t.Fatalf("cancel accepted: %v %+v", err, got)
	}
	_, err = svc.Cancel(ctx, d.owner(), inv.ID)
	requireStatus(t, err, 409)
	_, err = svc.Cancel(ctx, d.owner(), uuid.New())
	requireStatus(t, err, 404)
}

func TestRemindStampsPendingOnly(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	inv := d.pending(d.memberID, authctx.StaffRoleTroGiang)
	got, err := svc.Remind(ctx, d.owner(), inv.ID)
	if err != nil || got.RemindedAt == nil {
		t.Fatalf("remind: %v %+v", err, got)
	}
	inv.Status = StatusAccepted
	_, err = svc.Remind(ctx, d.owner(), inv.ID)
	requireStatus(t, err, 409)
}

func TestConfirmRoutesRoleToTheRightWrite(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()

	staffInv := d.pending(d.memberID, authctx.StaffRoleHocVu)
	got, err := svc.Confirm(ctx, d.owner(), staffInv.ID)
	if err != nil {
		t.Fatalf("confirm hoc_vu: %v", err)
	}
	if got.Status != StatusAssigned || len(d.assigner.assigns) != 1 || d.assigner.assigns[0].RoleKey != authctx.StaffRoleHocVu {
		t.Fatalf("hoc_vu must go through classstaff assign, got %+v / %+v", got, d.assigner.assigns)
	}
	if d.tx.calls != 1 {
		t.Fatalf("confirm must run in one transaction, got %d", d.tx.calls)
	}

	other := uuid.New()
	d.members.active[other] = true
	gvInv := d.pending(other, authctx.StaffRoleGiaoVien)
	gvInv.Status = StatusAccepted
	d.assigner.movedCount = 3
	got, err = svc.Confirm(ctx, d.owner(), gvInv.ID)
	if err != nil {
		t.Fatalf("confirm giao_vien: %v", err)
	}
	if len(d.assigner.reassigns) != 1 || d.assigner.reassigns[0] != other || got.MovedPlannedSessions != 3 {
		t.Fatalf("giao_vien must go through handoff, got %+v / %+v", got, d.assigner.reassigns)
	}
	_, err = svc.Confirm(ctx, d.owner(), gvInv.ID)
	requireStatus(t, err, 409)
}

func TestConfirmRefusesInactiveInviteeAndPropagatesLockConflict(t *testing.T) {
	svc, d := newTestService()
	ctx := context.Background()
	left := uuid.New()
	inv := d.pending(left, authctx.StaffRoleGiaoVien)
	_, err := svc.Confirm(ctx, d.owner(), inv.ID)
	if appErr := requireAppError(t, err, 422); appErr.Code != CodeMemberInactive {
		t.Fatalf("want %s, got %s", CodeMemberInactive, appErr.Code)
	}
	if got, _ := d.repo.Get(ctx, d.owner(), inv.ID); got.Status != StatusPending {
		t.Fatalf("refused confirm must leave the invitation untouched, got %s", got.Status)
	}

	inv2 := d.pending(d.memberID, authctx.StaffRoleGiaoVien)
	d.assigner.reassErr = apperror.Conflict("một thao tác khác của trung tâm đang chạy; thử lại sau")
	_, err = svc.Confirm(ctx, d.owner(), inv2.ID)
	requireStatus(t, err, 409)
}

func TestCancelOpenForMemberClosesPendingAndAccepted(t *testing.T) {
	svc, d := newTestService()
	a := d.pending(d.memberID, authctx.StaffRoleTroGiang)
	b := d.pending(d.memberID, authctx.StaffRoleHocVu)
	b.Status = StatusAccepted
	c := d.pending(d.memberID, authctx.StaffRoleHocVu)
	c.Status = StatusDeclined
	n, err := svc.CancelOpenForMember(context.Background(), d.center, d.memberID)
	if err != nil || n != 2 {
		t.Fatalf("want 2 cancelled, got %d %v", n, err)
	}
	if a.Status != StatusCancelled || b.Status != StatusCancelled || c.Status != StatusDeclined {
		t.Fatalf("unexpected statuses %s %s %s", a.Status, b.Status, c.Status)
	}
}

func TestSendRepoErrorSurfaces(t *testing.T) {
	svc, d := newTestService()
	d.repo.createErr = errBoom
	_, err := svc.Send(context.Background(), d.owner(), d.classID, SendRequest{TeacherID: d.memberID, RoleKey: authctx.StaffRoleHocVu})
	if !errors.Is(err, errBoom) {
		t.Fatalf("want repo error, got %v", err)
	}
}
