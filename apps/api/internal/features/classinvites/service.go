package classinvites

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/classes"
	"teka/apps/api/internal/features/classstaff"
	"teka/apps/api/internal/features/handoff"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
)

// ClassReader is the slice of classes this feature reads: the class under the
// owner's scope, so a class outside the center is a clean 404.
// *classes.Service satisfies it.
type ClassReader interface {
	Get(ctx context.Context, sc authctx.Scope, classID uuid.UUID) (*classes.Class, error)
}

// MemberChecker validates the invitee against the caller's own center.
// *centers.Service satisfies it.
type MemberChecker interface {
	IsActiveMember(ctx context.Context, sc authctx.Scope, teacherID uuid.UUID) (bool, error)
}

// StaffRoles reports the invitee's active role keys on the class, so an
// invitation for a role the person already holds is refused up front.
// classstaff.Repository satisfies it.
type StaffRoles interface {
	RolesByClass(ctx context.Context, teacherID, centerID uuid.UUID, classIDs []uuid.UUID) (map[uuid.UUID][]string, error)
}

// StaffAssigner writes a tro_giang / hoc_vu stint on confirm.
// *classstaff.Service satisfies it; it enforces the owner gate itself.
type StaffAssigner interface {
	Assign(ctx context.Context, sc authctx.Scope, classID uuid.UUID, req classstaff.AssignRequest) (*classstaff.StaffResponse, error)
}

// TeacherReassigner hands the class to the invitee on a giao_vien confirm.
// *handoff.Service satisfies it; it takes the center lock and returns 409
// when another center-wide operation holds it.
type TeacherReassigner interface {
	Reassign(ctx context.Context, sc authctx.Scope, classID, newTeacherID uuid.UUID) (*handoff.Result, error)
}

// Service implements the class-invitation business logic.
type Service struct {
	repo       Repository
	classes    ClassReader
	members    MemberChecker
	staffRoles StaffRoles
	staff      StaffAssigner
	handoff    TeacherReassigner
	tx         database.TxManager
}

// NewService builds the service.
func NewService(
	repo Repository,
	classReader ClassReader,
	members MemberChecker,
	staffRoles StaffRoles,
	staff StaffAssigner,
	teacherReassigner TeacherReassigner,
	tx database.TxManager,
) *Service {
	return &Service{
		repo:       repo,
		classes:    classReader,
		members:    members,
		staffRoles: staffRoles,
		staff:      staff,
		handoff:    teacherReassigner,
		tx:         tx,
	}
}

// Send creates a pending invitation (owner only). The invitee must be an
// active member of the center, not the caller, not already pending on the
// class, and not already holding the proposed role there.
func (s *Service) Send(ctx context.Context, sc authctx.Scope, classID uuid.UUID, req SendRequest) (*InvitationResponse, error) {
	if !sc.IsOwner {
		return nil, apperror.Forbidden("chỉ chủ trung tâm được gửi lời mời nhận lớp")
	}
	if _, err := s.classes.Get(ctx, sc, classID); err != nil {
		if errors.Is(err, classes.ErrNotFound) {
			return nil, apperror.NotFound("class")
		}
		return nil, err
	}
	if !authctx.ValidStaffRole(req.RoleKey) {
		return nil, apperror.Invalid("vai trò không hợp lệ",
			map[string]string{"role_key": "không nằm trong danh mục vai trò"})
	}
	if req.TeacherID == sc.TeacherID {
		return nil, apperror.New(CodeSelfInvite, 422, "bạn không thể tự mời chính mình")
	}
	if err := s.requireActiveMember(ctx, sc, req.TeacherID); err != nil {
		return nil, err
	}
	pending, err := s.repo.HasPending(ctx, sc, classID, req.TeacherID)
	if err != nil {
		return nil, err
	}
	if pending {
		return nil, apperror.Conflict("người này đã có lời mời đang chờ cho lớp")
	}
	roles, err := s.staffRoles.RolesByClass(ctx, req.TeacherID, sc.CenterID, []uuid.UUID{classID})
	if err != nil {
		return nil, err
	}
	if err := refuseHeldRole(roles[classID], req.RoleKey); err != nil {
		return nil, err
	}
	if req.Message != nil {
		trimmed := strings.TrimSpace(*req.Message)
		if trimmed == "" {
			req.Message = nil
		} else {
			req.Message = &trimmed
		}
	}

	inv := &Invitation{
		CenterID:  sc.CenterID,
		ClassID:   classID,
		TeacherID: req.TeacherID,
		RoleKey:   req.RoleKey,
		InvitedBy: sc.TeacherID,
		Message:   req.Message,
	}
	if err := s.repo.Create(ctx, inv); err != nil {
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, apperror.Conflict("người này đã có lời mời đang chờ cho lớp")
		}
		return nil, err
	}
	return s.load(ctx, sc, inv.ID)
}

// List returns invitations visible to the caller: the owner sees the whole
// center, a member only their own. The filter never widens visibility.
func (s *Service) List(ctx context.Context, sc authctx.Scope, q ListQuery) ([]InvitationResponse, error) {
	if q.Status != "" && !ValidStatus(q.Status) {
		return nil, apperror.Invalid("trạng thái không hợp lệ",
			map[string]string{"status": "không nằm trong danh mục trạng thái"})
	}
	f := Filter{Status: q.Status}
	if q.ClassID != "" {
		classID, err := uuid.Parse(q.ClassID)
		if err != nil {
			return nil, apperror.Invalid("mã lớp không hợp lệ",
				map[string]string{"class_id": "must be a uuid"})
		}
		f.ClassID = &classID
	}
	if !sc.IsOwner {
		teacherID := sc.TeacherID
		f.TeacherID = &teacherID
	}
	rows, err := s.repo.List(ctx, sc, f)
	if err != nil {
		return nil, err
	}
	out := make([]InvitationResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, toResponse(row))
	}
	return out, nil
}

// Accept marks a pending invitation accepted (invitee only). It writes no
// class_staff row: the stint is the owner's confirm.
func (s *Service) Accept(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*InvitationResponse, error) {
	return s.respond(ctx, sc, id, StatusAccepted)
}

// Decline marks a pending invitation declined (invitee only).
func (s *Service) Decline(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*InvitationResponse, error) {
	return s.respond(ctx, sc, id, StatusDeclined)
}

func (s *Service) respond(ctx context.Context, sc authctx.Scope, id uuid.UUID, to string) (*InvitationResponse, error) {
	row, err := s.visible(ctx, sc, id)
	if err != nil {
		return nil, err
	}
	// Only the invitee responds — the owner's own view of the row is not a
	// licence to answer on someone's behalf, and a stranger gets the same
	// 404 as a missing row so nothing leaks.
	if row.TeacherID != sc.TeacherID {
		return nil, apperror.NotFound("class invitation")
	}
	// A member who already left the center holds no invitations any more:
	// their row is a 404 like a stranger's, even on a token issued earlier.
	active, err := s.members.IsActiveMember(ctx, sc, sc.TeacherID)
	if err != nil {
		return nil, err
	}
	if !active {
		return nil, apperror.NotFound("class invitation")
	}
	if row.Status != StatusPending {
		return nil, apperror.Conflict("lời mời không còn ở trạng thái chờ")
	}
	if err := s.transition(ctx, sc, id, []string{StatusPending}, to); err != nil {
		return nil, err
	}
	return s.load(ctx, sc, id)
}

// Cancel withdraws a pending or accepted invitation (owner only).
func (s *Service) Cancel(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*InvitationResponse, error) {
	row, err := s.ownerRow(ctx, sc, id)
	if err != nil {
		return nil, err
	}
	if row.Status != StatusPending && row.Status != StatusAccepted {
		return nil, apperror.Conflict("lời mời đã kết thúc, không thể hủy")
	}
	if err := s.transition(ctx, sc, id, []string{StatusPending, StatusAccepted}, StatusCancelled); err != nil {
		return nil, err
	}
	return s.load(ctx, sc, id)
}

// Remind stamps reminded_at on a pending invitation (owner only). No message
// is sent; the stamp is what both sides see as "Đã nhắc lúc …".
func (s *Service) Remind(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*InvitationResponse, error) {
	row, err := s.ownerRow(ctx, sc, id)
	if err != nil {
		return nil, err
	}
	if row.Status != StatusPending {
		return nil, apperror.Conflict("chỉ nhắc được lời mời đang chờ")
	}
	if err := s.repo.Remind(ctx, sc, id); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, apperror.Conflict("lời mời vừa thay đổi trạng thái, tải lại")
		}
		return nil, err
	}
	return s.load(ctx, sc, id)
}

// Confirm completes a pending or accepted invitation (owner only): the
// invitation becomes assigned and the stint is written in the same
// transaction — a giao_vien invitation is a class handoff (the current
// teacher's stint closes, future planned sessions move), any other role is a
// plain staff assignment. The handoff's center lock makes a busy center a
// 409 that rolls the whole confirm back.
func (s *Service) Confirm(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*ConfirmResponse, error) {
	row, err := s.ownerRow(ctx, sc, id)
	if err != nil {
		return nil, err
	}
	if row.Status != StatusPending && row.Status != StatusAccepted {
		return nil, apperror.Conflict("lời mời đã kết thúc, không thể phân công")
	}
	if err := s.requireActiveMember(ctx, sc, row.TeacherID); err != nil {
		return nil, err
	}

	var moved int64
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.transition(ctx, sc, id, []string{StatusPending, StatusAccepted}, StatusAssigned); err != nil {
			return err
		}
		if row.RoleKey == authctx.StaffRoleGiaoVien {
			result, err := s.handoff.Reassign(ctx, sc, row.ClassID, row.TeacherID)
			if err != nil {
				return err
			}
			moved = result.MovedPlannedSessions
			return nil
		}
		_, err := s.staff.Assign(ctx, sc, row.ClassID, classstaff.AssignRequest{
			TeacherID: row.TeacherID, RoleKey: row.RoleKey,
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	resp, err := s.load(ctx, sc, id)
	if err != nil {
		return nil, err
	}
	return &ConfirmResponse{InvitationResponse: *resp, MovedPlannedSessions: moved}, nil
}

// CancelOpenForMember cancels the member's pending and accepted invitations
// in the center. centers.Service calls it inside RemoveMember's transaction:
// a removed member must not keep an invitation the owner could later confirm.
func (s *Service) CancelOpenForMember(ctx context.Context, centerID, teacherID uuid.UUID) (int64, error) {
	return s.repo.CancelOpenForMember(ctx, centerID, teacherID)
}

// visible loads the row the caller may see: the owner sees every invitation
// of the center, a member only their own; anything else is a 404 so an
// invitation's existence never leaks across members.
func (s *Service) visible(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Row, error) {
	row, err := s.repo.Get(ctx, sc, id)
	if errors.Is(err, ErrNotFound) {
		return nil, apperror.NotFound("class invitation")
	}
	if err != nil {
		return nil, err
	}
	if !sc.IsOwner && row.TeacherID != sc.TeacherID {
		return nil, apperror.NotFound("class invitation")
	}
	return row, nil
}

func (s *Service) ownerRow(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*Row, error) {
	if !sc.IsOwner {
		return nil, apperror.Forbidden("chỉ chủ trung tâm được thao tác lời mời nhận lớp")
	}
	return s.visible(ctx, sc, id)
}

func (s *Service) requireActiveMember(ctx context.Context, sc authctx.Scope, teacherID uuid.UUID) error {
	member, err := s.members.IsActiveMember(ctx, sc, teacherID)
	if err != nil {
		return err
	}
	if !member {
		e := apperror.New(CodeMemberInactive, 422, "người này không còn là thành viên đang hoạt động của trung tâm")
		e.Fields = map[string]string{"teacher_id": "không thuộc trung tâm"}
		return e
	}
	return nil
}

// refuseHeldRole mirrors the class_staff invariant of one active stint per
// person per class. A support role cannot be proposed to anyone already on
// the class, since confirm would always collide; giao_vien may be proposed
// to a current tro_giang or hoc_vu because the handoff closes that stint.
func refuseHeldRole(held []string, roleKey string) error {
	for _, role := range held {
		if role == roleKey {
			return apperror.Conflict("người này đã giữ vai trò này trong lớp")
		}
		if roleKey != authctx.StaffRoleGiaoVien {
			return apperror.Conflict("người này đang giữ vai trò khác trong lớp, gỡ vai trò cũ trước")
		}
	}
	return nil
}

// transition applies a repository state change, reading a no-op as the row
// having changed under the caller since it was loaded.
func (s *Service) transition(ctx context.Context, sc authctx.Scope, id uuid.UUID, from []string, to string) error {
	err := s.repo.Transition(ctx, sc, id, from, to)
	if errors.Is(err, ErrNotFound) {
		return apperror.Conflict("lời mời vừa thay đổi trạng thái, tải lại")
	}
	return err
}

func (s *Service) load(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*InvitationResponse, error) {
	row, err := s.repo.Get(ctx, sc, id)
	if errors.Is(err, ErrNotFound) {
		return nil, apperror.NotFound("class invitation")
	}
	if err != nil {
		return nil, err
	}
	resp := toResponse(*row)
	return &resp, nil
}
