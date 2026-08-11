package centers

import (
	"bytes"
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/id"
)

// TeacherLookup is the slice of the teachers feature this service consumes
// (consumer-defined interface; implemented by *teachers.Service). GetByPhone
// deliberately stays behind this seam — it performs no authorization and
// must never back an endpoint directly.
type TeacherLookup interface {
	GetByPhone(ctx context.Context, phone string) (*teachers.Profile, error)
}

// Service implements center membership business logic.
type Service struct {
	repo     Repository
	teachers TeacherLookup
	tx       database.TxManager
}

// NewService builds the centers service.
func NewService(repo Repository, lookup TeacherLookup, tx database.TxManager) *Service {
	return &Service{repo: repo, teachers: lookup, tx: tx}
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

// CreatePersonalCenter inserts the centers row for a brand-new teacher; it
// satisfies teachers.CenterProvisioner and must run on the registration
// transaction (the owner FK is deferred until the teachers row follows).
func (s *Service) CreatePersonalCenter(ctx context.Context, ownerID uuid.UUID, name string) (uuid.UUID, error) {
	c := &Center{ID: id.New(), Name: name, OwnerID: ownerID}
	if err := s.repo.CreateCenter(ctx, c); err != nil {
		if errors.Is(err, ErrOwnerHasLiveCenter) {
			return uuid.Nil, apperror.Conflict("teacher already owns a center")
		}
		return uuid.Nil, apperror.Internal(err)
	}
	return c.ID, nil
}

// OpenMembership records the live membership stint for a freshly created
// teacher; it satisfies teachers.CenterProvisioner.
func (s *Service) OpenMembership(ctx context.Context, teacherID, centerID uuid.UUID) error {
	if _, err := s.repo.OpenMembership(ctx, teacherID, centerID); err != nil {
		return apperror.Internal(err)
	}
	return nil
}

// Me returns the caller's center and its member roster; every member may
// look.
func (s *Service) Me(ctx context.Context, scope authctx.Scope) (*MeResponse, error) {
	center, err := s.repo.GetCenter(ctx, scope.CenterID)
	if errors.Is(err, ErrNotFound) {
		return nil, apperror.NotFound("center")
	}
	if err != nil {
		return nil, apperror.Internal(err)
	}
	members, err := s.repo.ListMembers(ctx, scope.CenterID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	resp := &MeResponse{
		Center: CenterResponse{ID: center.ID, Name: center.Name, IsOwner: scope.IsOwner},
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

// errLostRace marks an in-transaction re-check that no longer holds: a
// concurrent request changed the caller's center between the friendly
// pre-checks and the locked ones.
var errLostRace = errors.New("centers: state changed concurrently")

// Join moves the caller into the center owned by the teacher behind
// owner_phone. Consent flows from the caller (they initiate with a phone the
// owner gave them); every way the phone cannot be joined — unknown, not a
// teacher, disabled, or not currently an owner — collapses into one generic
// 404 so the endpoint cannot be used to probe who is registered. The
// caller-side eligibility checks run BEFORE the phone lookup for the same
// reason: an ineligible caller gets their 409 regardless of what they typed,
// so the 404-vs-409 split never confirms a phone.
//
// V1 keeps the move simple: the caller must still be alone in their own
// empty personal center. Their old center is retired (soft delete), never
// deleted — its membership history stays as the anchor for any future data
// rules.
func (s *Service) Join(ctx context.Context, scope authctx.Scope, req JoinRequest) (*JoinResponse, error) {
	if !scope.IsOwner {
		return nil, apperror.Conflict("leave your current center before joining another")
	}
	others, err := s.repo.CountOtherMembers(ctx, scope.CenterID, scope.TeacherID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	if others > 0 {
		return nil, apperror.Conflict("your center still has other members")
	}
	rows, err := s.repo.CountBusinessRows(ctx, scope.CenterID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	if rows > 0 {
		return nil, apperror.Conflict("your center still has classes, students, or contacts")
	}

	notFound := apperror.NotFound("center owner")
	p, err := s.teachers.GetByPhone(ctx, req.OwnerPhone)
	if err != nil {
		if apperror.From(err).Code == apperror.CodeNotFound {
			return nil, notFound
		}
		return nil, err
	}
	if p.Account.Status != teachers.StatusActive {
		return nil, notFound
	}
	if p.Account.ID == scope.TeacherID {
		return nil, apperror.Invalid("cannot join using your own phone number", nil)
	}
	target, err := s.repo.GetLiveCenterByOwner(ctx, p.Account.ID)
	if errors.Is(err, ErrNotFound) {
		return nil, notFound
	}
	if err != nil {
		return nil, apperror.Internal(err)
	}

	var joinedAt time.Time
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		// Serialize concurrent moves on the two centers rows (stable lock
		// order to avoid deadlocks) and repeat the eligibility checks under
		// the lock. Without this, a request joining the caller's center in
		// parallel could land its teacher inside the center this transaction
		// is about to retire — a dead scope the account could never leave.
		if err := s.lockBothLiveCenters(ctx, scope.CenterID, target.ID); err != nil {
			return err
		}
		others, err := s.repo.CountOtherMembers(ctx, scope.CenterID, scope.TeacherID)
		if err != nil {
			return err
		}
		rows, err := s.repo.CountBusinessRows(ctx, scope.CenterID)
		if err != nil {
			return err
		}
		if others > 0 || rows > 0 {
			return errLostRace
		}
		// Close before open: uq_center_members_active is checked per
		// statement, so the live stint must end before the next begins.
		if err := s.repo.CloseMembership(ctx, scope.TeacherID, scope.CenterID); err != nil {
			return err
		}
		ja, err := s.repo.OpenMembership(ctx, scope.TeacherID, target.ID)
		if err != nil {
			return err
		}
		joinedAt = ja
		if err := s.repo.SwitchTeacherCenter(ctx, scope.TeacherID, scope.CenterID, target.ID); err != nil {
			return err
		}
		return s.repo.SoftDeleteCenter(ctx, scope.CenterID, scope.TeacherID)
	})
	if err != nil {
		// Losing any of the guarded writes means a concurrent request moved
		// the caller (or retired the target center) first.
		if errors.Is(err, ErrNotFound) || errors.Is(err, ErrActiveMembershipExists) || errors.Is(err, errLostRace) {
			return nil, apperror.Conflict("membership changed concurrently, retry")
		}
		return nil, apperror.From(err)
	}
	return &JoinResponse{CenterID: target.ID, JoinedAt: joinedAt}, nil
}

// lockBothLiveCenters takes FOR UPDATE row locks on both centers in a
// deterministic order so two opposite joins cannot deadlock; either center
// being retired already surfaces as ErrNotFound.
func (s *Service) lockBothLiveCenters(ctx context.Context, a, b uuid.UUID) error {
	first, second := a, b
	if bytes.Compare(second[:], first[:]) < 0 {
		first, second = second, first
	}
	if err := s.repo.LockLiveCenter(ctx, first); err != nil {
		return err
	}
	return s.repo.LockLiveCenter(ctx, second)
}

// RemoveMember ends a membership: the owner may remove any member, a member
// may remove themselves (leave). The removed teacher lands in a fresh
// personal center; everything they created stays in the old center, anchored
// by the closed membership stint.
func (s *Service) RemoveMember(ctx context.Context, scope authctx.Scope, targetID uuid.UUID) error {
	if !scope.IsOwner && targetID != scope.TeacherID {
		return apperror.Forbidden("only the owner or the member themselves can end a membership")
	}
	target, err := s.repo.GetTeacherInCenter(ctx, scope.CenterID, targetID)
	if errors.Is(err, ErrNotFound) {
		return apperror.NotFound("member")
	}
	if err != nil {
		return apperror.Internal(err)
	}
	if scope.IsOwner && targetID == scope.TeacherID {
		// The owner IS the center: with members around, kicking themselves
		// would orphan everyone; alone, "leaving" their own center means
		// nothing — there is nowhere to go that they are not already.
		return apperror.Invalid("the owner cannot leave their own center", nil)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		personal := &Center{ID: id.New(), Name: target.FullName, OwnerID: targetID}
		if err := s.repo.CreateCenter(ctx, personal); err != nil {
			return err
		}
		if err := s.repo.CloseMembership(ctx, targetID, scope.CenterID); err != nil {
			return err
		}
		if _, err := s.repo.OpenMembership(ctx, targetID, personal.ID); err != nil {
			return err
		}
		return s.repo.SwitchTeacherCenter(ctx, targetID, scope.CenterID, personal.ID)
	})
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apperror.NotFound("member")
		}
		if errors.Is(err, ErrOwnerHasLiveCenter) || errors.Is(err, ErrActiveMembershipExists) {
			return apperror.Conflict("membership changed concurrently, retry")
		}
		return apperror.From(err)
	}
	return nil
}
