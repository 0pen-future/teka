package centers

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
)

// AccountDisabler is the slice of account lifecycle this service consumes to
// offboard a removed member (consumer-defined interface; implemented by
// *auth.Service — it must both flip the account to disabled AND revoke every
// refresh token it holds, atomically, so a removed member cannot keep using
// an old access token). Centers never touches user_accounts directly.
type AccountDisabler interface {
	Disable(ctx context.Context, accountID uuid.UUID) error
}

// Service implements center membership business logic.
type Service struct {
	repo     Repository
	disabler AccountDisabler
	tx       database.TxManager
}

// NewService builds the centers service.
func NewService(repo Repository, tx database.TxManager) *Service {
	return &Service{repo: repo, tx: tx}
}

// SetAccountDisabler wires the auth dependency after construction — a
// setter, not a NewService parameter, because auth.Service itself depends on
// teachers.Service as its AccountService, and router.go constructs centers
// before auth; a direct parameter here would cycle (same pattern as
// teachers.SetTokenRevoker).
func (s *Service) SetAccountDisabler(d AccountDisabler) {
	s.disabler = d
}

// ResolveScope loads the caller's center scope; it satisfies
// middleware.ScopeResolver. Disabled accounts, soft-deleted accounts or
// teachers, and retired centers all resolve to the same 401 — a token whose
// account can no longer act is simply not authenticated.
func (s *Service) ResolveScope(ctx context.Context, teacherID uuid.UUID) (authctx.Scope, error) {
	row, err := s.repo.ResolveScope(ctx, teacherID)
	if errors.Is(err, ErrNotFound) {
		return authctx.Scope{}, apperror.Unauthorized("account is not active")
	}
	if err != nil {
		return authctx.Scope{}, apperror.Internal(err)
	}
	return authctx.Scope{TeacherID: teacherID, CenterID: row.CenterID, IsOwner: row.IsOwner}, nil
}

// CenterOwner returns the owner teacher id of teacherID's current center, and
// whether teacherID IS that owner; it satisfies auth.OwnerResolver so
// ForgotPassword can exclude a center owner from self-service reset (owners
// recover via operator CLI only). ErrNotFound — unknown teacher, or one whose
// center is gone — maps to apperror.NotFound rather than the 401 ResolveScope
// uses: this is an internal lookup, not a request-authentication check.
func (s *Service) CenterOwner(ctx context.Context, teacherID uuid.UUID) (ownerID uuid.UUID, isOwner bool, err error) {
	row, err := s.repo.CenterOwner(ctx, teacherID)
	if errors.Is(err, ErrNotFound) {
		return uuid.Nil, false, apperror.NotFound("teacher")
	}
	if err != nil {
		return uuid.Nil, false, apperror.Internal(err)
	}
	return row.OwnerID, row.IsOwner, nil
}

// OpenMembership records a live membership stint for a teacher in a center;
// it satisfies invitations.MembershipOpener for both the new-account and the
// reactivate branches of the accept flow.
func (s *Service) OpenMembership(ctx context.Context, teacherID, centerID uuid.UUID) error {
	if _, err := s.repo.OpenMembership(ctx, teacherID, centerID); err != nil {
		return apperror.Internal(err)
	}
	return nil
}

// SwitchTeacherCenter moves teachers.center_id to centerID unconditionally;
// it satisfies invitations.MembershipOpener for the accept flow's reactivate
// path. The account's previous membership is already closed by the time it
// reactivates (RemoveMember closes it at disable time), so there is no "from"
// to guard against.
func (s *Service) SwitchTeacherCenter(ctx context.Context, teacherID, centerID uuid.UUID) error {
	if err := s.repo.SwitchTeacherCenter(ctx, teacherID, centerID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return apperror.NotFound("teacher")
		}
		return apperror.Internal(err)
	}
	return nil
}

// WasEverMember reports whether the teacher is a current or past member of
// the center; it satisfies invitations.MembershipOpener so the accept flow
// can gate a disabled account's reactivation on real prior membership in the
// inviting center, instead of trusting the token alone.
func (s *Service) WasEverMember(ctx context.Context, teacherID, centerID uuid.UUID) (bool, error) {
	ok, err := s.repo.WasEverMember(ctx, centerID, teacherID)
	if err != nil {
		return false, apperror.Internal(err)
	}
	return ok, nil
}

// Me returns the caller's center read model: the owner sees the full member
// roster, a member sees only the center's name — the roster is owner-only
// data.
func (s *Service) Me(ctx context.Context, scope authctx.Scope) (any, error) {
	center, err := s.repo.GetCenter(ctx, scope.CenterID)
	if errors.Is(err, ErrNotFound) {
		return nil, apperror.NotFound("center")
	}
	if err != nil {
		return nil, apperror.Internal(err)
	}
	if !scope.IsOwner {
		return &MemberMeResponse{CenterName: center.Name}, nil
	}
	members, err := s.repo.ListMembers(ctx, scope.CenterID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	resp := &MeResponse{
		Center: CenterResponse{ID: center.ID, Name: center.Name, IsOwner: true},
	}
	for _, m := range members {
		resp.Members = append(resp.Members, MemberResponse(m))
	}
	return resp, nil
}

// Rename changes the center's display name; owners only.
func (s *Service) Rename(ctx context.Context, scope authctx.Scope, req RenameRequest) error {
	if !scope.IsOwner {
		return apperror.Forbidden("only the center owner can rename it")
	}
	err := s.repo.Rename(ctx, scope.CenterID, req.Name)
	if errors.Is(err, ErrNotFound) {
		return apperror.NotFound("center")
	}
	if err != nil {
		return apperror.Internal(err)
	}
	return nil
}

// RemoveMember offboards a member: owner-only. The membership stint closes
// and the account is disabled — status flips to disabled and every refresh
// token it holds is revoked, via AccountDisabler (*auth.Service.Disable) — no
// new center is provisioned; the member simply loses access until re-invited.
// teachers.center_id is left pointing at this center: ResolveScope already
// 401s a disabled account, and the accept flow's reactivate path is what
// eventually moves it on re-invite. The owner cannot remove themselves: with
// members around it would orphan the center, and alone there is nothing to
// remove.
func (s *Service) RemoveMember(ctx context.Context, scope authctx.Scope, targetID uuid.UUID) error {
	if !scope.IsOwner {
		return apperror.Forbidden("only the owner can remove a member")
	}
	if targetID == scope.TeacherID {
		return apperror.Invalid("the owner cannot remove themselves", nil)
	}
	if _, err := s.repo.GetTeacherInCenter(ctx, scope.CenterID, targetID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return apperror.NotFound("member")
		}
		return apperror.Internal(err)
	}

	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.repo.CloseMembership(ctx, targetID, scope.CenterID); err != nil {
			return err
		}
		return s.disabler.Disable(ctx, targetID)
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apperror.Conflict("membership changed concurrently, retry")
		}
		return apperror.From(err)
	}
	return nil
}
