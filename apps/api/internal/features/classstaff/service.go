package classstaff

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
)

// MemberChecker validates the assignment target against the caller's own
// center — the same consumer-defined slice handoff uses; *centers.Service
// satisfies it.
type MemberChecker interface {
	IsActiveMember(ctx context.Context, sc authctx.Scope, teacherID uuid.UUID) (bool, error)
}

// Service owns the staff-assignment business rules: the 404-before-403 read
// gate, owner-only writes, and the giao_vien lockout (handoff owns that role
// while classes.teacher_id stays the source of truth).
type Service struct {
	repo    Repository
	members MemberChecker
}

// NewService builds the classstaff service.
func NewService(repo Repository, members MemberChecker) *Service {
	return &Service{repo: repo, members: members}
}

// readAccess decides whether the caller may see the class at all. A caller
// with no relationship to the class gets 404 for every verb — the class's
// existence is not leaked through a 403. Center-wide readers (the owner, or a
// member granted classes.view_all) see every class of their center — the
// same rule the classes read port applies, so a caller who can GET the class
// never 404s on its staff list; anyone else needs a stint on the class (ended
// included: history stays readable after a soft-close).
func (s *Service) readAccess(ctx context.Context, sc authctx.Scope, classID uuid.UUID) error {
	exists, err := s.repo.ClassInCenter(ctx, sc, classID)
	if err != nil {
		return err
	}
	if !exists {
		return apperror.NotFound("class")
	}
	if sc.CenterWideFor(authctx.PermClassesViewAll) {
		return nil
	}
	assigned, err := s.repo.CallerHasAssignment(ctx, sc, classID)
	if err != nil {
		return err
	}
	if !assigned {
		return apperror.NotFound("class")
	}
	return nil
}

// List returns the class's staff stints, active and ended.
func (s *Service) List(ctx context.Context, sc authctx.Scope, classID uuid.UUID) ([]StaffResponse, error) {
	if err := s.readAccess(ctx, sc, classID); err != nil {
		return nil, err
	}
	rows, err := s.repo.ListByClass(ctx, sc, classID)
	if err != nil {
		return nil, err
	}
	out := make([]StaffResponse, len(rows))
	for i, row := range rows {
		out[i] = toResponse(row)
	}
	return out, nil
}

// Assign gives a live member a role in the class (owner only). giao_vien is
// refused with a 409 pointing at the handoff flow — during the dual-write
// window the primary teacher changes only together with classes.teacher_id.
func (s *Service) Assign(ctx context.Context, sc authctx.Scope, classID uuid.UUID, req AssignRequest) (*StaffResponse, error) {
	if err := s.readAccess(ctx, sc, classID); err != nil {
		return nil, err
	}
	if !sc.IsOwner {
		return nil, apperror.Forbidden("chỉ chủ trung tâm được gán nhân sự lớp")
	}
	if !authctx.ValidStaffRole(req.RoleKey) {
		return nil, apperror.Invalid("vai trò không hợp lệ",
			map[string]string{"role_key": "không nằm trong danh mục vai trò"})
	}
	if req.RoleKey == authctx.StaffRoleGiaoVien {
		return nil, apperror.Conflict("giáo viên chính chỉ thay đổi qua bàn giao lớp")
	}
	member, err := s.members.IsActiveMember(ctx, sc, req.TeacherID)
	if err != nil {
		return nil, err
	}
	if !member {
		return nil, apperror.BadRequest("giáo viên này không phải thành viên đang hoạt động của trung tâm")
	}
	active, err := s.repo.HasActiveAssignment(ctx, sc, classID, req.TeacherID)
	if err != nil {
		return nil, err
	}
	if active {
		return nil, apperror.Conflict("người này đã có vai trò đang hoạt động trong lớp")
	}

	assignment := &Assignment{
		ClassID:   classID,
		CenterID:  sc.CenterID,
		TeacherID: req.TeacherID,
		RoleKey:   req.RoleKey,
	}
	if err := s.repo.Create(ctx, assignment); err != nil {
		// A racing assign of the same person lost to uq_class_staff_active —
		// the same 409 the pre-check would have produced.
		if errors.Is(err, gorm.ErrDuplicatedKey) {
			return nil, apperror.Conflict("người này đã có vai trò đang hoạt động trong lớp")
		}
		return nil, err
	}
	row, err := s.repo.GetRow(ctx, sc, classID, assignment.ID)
	if err != nil {
		return nil, err
	}
	resp := toResponse(*row)
	return &resp, nil
}

// Remove ends or revokes a stint (owner only). The default soft-close keeps
// the row — the person keeps read access to the class's history. void=true is
// the revocation path for a mistaken grant: it hard-deletes the row, active or
// already ended, so nothing keeps granting reads. An active giao_vien refuses
// both modes — the primary teacher leaves only through a handoff.
func (s *Service) Remove(ctx context.Context, sc authctx.Scope, classID, staffID uuid.UUID, void bool) error {
	if err := s.readAccess(ctx, sc, classID); err != nil {
		return err
	}
	if !sc.IsOwner {
		return apperror.Forbidden("chỉ chủ trung tâm được gỡ nhân sự lớp")
	}
	row, err := s.repo.GetRow(ctx, sc, classID, staffID)
	if errors.Is(err, ErrNotFound) {
		return apperror.NotFound("staff assignment")
	}
	if err != nil {
		return err
	}
	if row.RoleKey == authctx.StaffRoleGiaoVien && row.EndedAt == nil {
		return apperror.Conflict("giáo viên chính chỉ thay đổi qua bàn giao lớp")
	}
	if void {
		if err := s.repo.Delete(ctx, sc, staffID); errors.Is(err, ErrNotFound) {
			return apperror.NotFound("staff assignment")
		} else if err != nil {
			return err
		}
		return nil
	}
	if err := s.repo.Close(ctx, sc, staffID); errors.Is(err, ErrNotFound) {
		// Already ended: there is nothing left to close — void is the only
		// action that still applies to an ended stint.
		return apperror.NotFound("staff assignment")
	} else if err != nil {
		return err
	}
	return nil
}
