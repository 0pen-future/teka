package centers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/events"
	"teka/apps/api/internal/shared/validation"
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
	// bus receives the permission-mutation events for the audit trail. Nil is
	// a supported state — publish goes through the nil-safe helper below.
	bus events.Bus
}

// NewService builds the centers service. The bus is a constructor parameter
// rather than a setter because it exists before centers is built (unlike the
// auth cross-wiring below, there is no construction cycle to break).
func NewService(repo Repository, tx database.TxManager, bus events.Bus) *Service {
	return &Service{repo: repo, tx: tx, bus: bus}
}

// publish emits e when a bus is wired; a nil bus makes every publish a no-op.
func (s *Service) publish(e events.Event) {
	if s.bus != nil {
		s.bus.Publish(e)
	}
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
	perms := authctx.BuildPermSet(splitKeys(row.RolePerms), splitKeys(row.Grants), splitKeys(row.Denies))
	return authctx.Scope{
		TeacherID: teacherID,
		CenterID:  row.CenterID,
		IsOwner:   row.IsOwner,
		// Dual life: the column stays authoritative while grant/revoke
		// mirrors into reports.send override rows; the OR only widens, and
		// membership close/reopen resets both sides atomically so a revoked
		// permission cannot resurrect through either source.
		CanSendReports: row.CanSendReports || perms.HasKey(authctx.PermReportsSend),
		Perms:          perms,
	}, nil
}

// splitKeys undoes the repository's string_agg(',') packing; empty input
// means no keys.
func splitKeys(joined string) []string {
	if joined == "" {
		return nil
	}
	return strings.Split(joined, ",")
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

// MemberIDsByPhone returns this center's phone -> teacher_id directory, keyed
// by the E.164 storage form. It exists for bulk flows that name teachers by
// phone rather than by id — chiefly the roster import, where the workbook
// carries phone numbers an operator typed.
//
// The scope parameter is the authorization check itself: there is no way to
// ask for another center's directory, so no separate "is this teacher mine?"
// test exists that a caller could forget. Callers must not report the
// difference between "not a member here" and "no such account" — that
// distinction is an account-enumeration oracle. Removed teachers are absent by
// construction, because ListMembers joins user_accounts on status = active.
//
// This runs on a process-lifetime service that also backs middleware
// ResolveScope on every authenticated request, so it stays a query per call.
// Never memoize the result on the service: a cached per-center map here would
// be a cross-tenant leak with a very quiet diff.
func (s *Service) MemberIDsByPhone(ctx context.Context, scope authctx.Scope) (map[string]uuid.UUID, error) {
	rows, err := s.repo.ListMembers(ctx, scope.CenterID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	out := make(map[string]uuid.UUID, len(rows))
	for _, r := range rows {
		phone := validation.NormalizePhone(r.Phone)
		if prev, dup := out[phone]; dup && prev != r.ID {
			// uq_users_phone should make this unreachable. If it ever happens,
			// keeping the last row would anchor imported data on an arbitrary
			// one of two teachers, so fail rather than guess.
			return nil, apperror.Internal(fmt.Errorf(
				"centers: two active members of center %s share phone %s", scope.CenterID, phone))
		}
		out[phone] = r.ID
	}
	return out, nil
}

// IsActiveMember reports whether teacherID is a live, active member of the
// caller's own center. It is the target check the class-handoff feature
// consumes: as with MemberIDsByPhone the scope is the authorization boundary
// itself — there is no way to ask about another center, so a caller cannot
// probe membership outside its own tenant, and callers must not distinguish
// "not a member here" from "no such account". A removed (disabled) or
// soft-deleted teacher is not a member, matching the live-roster rule
// ListMembers/GetTeacherInCenter apply.
func (s *Service) IsActiveMember(ctx context.Context, scope authctx.Scope, teacherID uuid.UUID) (bool, error) {
	_, err := s.repo.GetTeacherInCenter(ctx, scope.CenterID, teacherID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, apperror.Internal(err)
	}
	return true, nil
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
		return &MemberMeResponse{
			CenterName:     center.Name,
			CanSendReports: scope.CanSendReports,
			Permissions:    scope.EffectiveKeys(),
		}, nil
	}
	members, err := s.repo.ListMembers(ctx, scope.CenterID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	resp := &MeResponse{
		Center:      CenterResponse{ID: center.ID, Name: center.Name, IsOwner: true},
		Permissions: scope.EffectiveKeys(),
	}
	for _, m := range members {
		resp.Members = append(resp.Members, MemberResponse(m))
	}
	return resp, nil
}

// Rename changes the center's display name; takes center.manage.
func (s *Service) Rename(ctx context.Context, scope authctx.Scope, req RenameRequest) error {
	if !scope.Has(authctx.PermCenterManage) {
		return apperror.Forbidden("you are not allowed to rename the center")
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

// SetSendReports grants or revokes the delegated send-reports permission on
// a member's live stint; owner-only. The owner can never be the target — the
// permission is member-only by product decision, which also keeps the owner's
// send behavior (incl. the cross-teacher 409) frozen. An owner target, a left
// member, or a stranger all collapse into the same not-found per the tenancy
// convention.
func (s *Service) SetSendReports(ctx context.Context, scope authctx.Scope, targetID uuid.UUID, enabled bool) error {
	if !scope.IsOwner {
		return apperror.Forbidden("only the owner can manage the send-reports permission")
	}
	err := s.repo.SetSendReports(ctx, scope.CenterID, targetID, enabled)
	if errors.Is(err, ErrNotFound) {
		return apperror.NotFound("member")
	}
	if err != nil {
		return apperror.Internal(err)
	}
	return nil
}

// RemoveMember offboards a member: takes members.manage. The membership stint closes
// and the account is disabled — status flips to disabled and every refresh
// token it holds is revoked, via AccountDisabler (*auth.Service.Disable) — no
// new center is provisioned; the member simply loses access until re-invited.
// teachers.center_id is left pointing at this center: ResolveScope already
// 401s a disabled account, and the accept flow's reactivate path is what
// eventually moves it on re-invite. Nobody removes themselves, and the owner
// can never be the target: removing them would orphan the center — a
// members.manage grant must not become an escalation path over the owner.
func (s *Service) RemoveMember(ctx context.Context, scope authctx.Scope, targetID uuid.UUID) error {
	if !scope.Has(authctx.PermMembersManage) {
		return apperror.Forbidden("you are not allowed to remove a member")
	}
	if targetID == scope.TeacherID {
		return apperror.Invalid("you cannot remove yourself", nil)
	}
	center, err := s.repo.GetCenter(ctx, scope.CenterID)
	if err != nil {
		return apperror.Internal(err)
	}
	if targetID == center.OwnerID {
		return apperror.Invalid("the center owner cannot be removed", nil)
	}
	if _, err := s.repo.GetTeacherInCenter(ctx, scope.CenterID, targetID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return apperror.NotFound("member")
		}
		return apperror.Internal(err)
	}

	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
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

// requirePermissionAdmin gates the permission-management endpoints. This is
// deliberately Scope.IsOwner, NOT a catalog key: whoever manages permissions
// can grant themselves everything, so delegating it would be a one-hop
// escalation path.
func requirePermissionAdmin(scope authctx.Scope) error {
	if !scope.IsOwner {
		return apperror.Forbidden("only the owner can manage permissions")
	}
	return nil
}

// normalizeKeys validates keys against the registry and returns them deduped
// in stable registry order; an unknown key is a 422 on the given field.
func normalizeKeys(field string, keys []string) ([]string, error) {
	for _, key := range keys {
		if !authctx.KnownPerm(key) {
			return nil, apperror.Invalid("validation failed",
				map[string]string{field: fmt.Sprintf("unknown permission key %q", key)})
		}
	}
	seen := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		seen[key] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for _, key := range authctx.PermKeys() {
		if _, ok := seen[key]; ok {
			out = append(out, key)
		}
	}
	return out, nil
}

// knownKeysOf filters a comma-joined DB list through the registry into stable
// order — assignments for keys a code rollback no longer defines stay in the
// database but never surface in responses or events.
func knownKeysOf(joined string) []string {
	set := make(map[string]struct{})
	for _, key := range splitKeys(joined) {
		if authctx.KnownPerm(key) {
			set[key] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for _, key := range authctx.PermKeys() {
		if _, ok := set[key]; ok {
			out = append(out, key)
		}
	}
	return out
}

// Permissions returns the owner's permission-management read model: the
// code-owned catalog, the center's roles with their sets, and the non-owner
// members with role + overrides.
func (s *Service) Permissions(ctx context.Context, scope authctx.Scope) (*PermissionsResponse, error) {
	if err := requirePermissionAdmin(scope); err != nil {
		return nil, err
	}
	roles, err := s.repo.ListRoles(ctx, scope.CenterID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	members, err := s.repo.ListMemberRBAC(ctx, scope.CenterID)
	if err != nil {
		return nil, apperror.Internal(err)
	}
	resp := &PermissionsResponse{
		Catalog: make([]PermissionInfo, 0, len(authctx.PermKeys())),
		Roles:   make([]RoleResponse, 0, len(roles)),
		Members: make([]MemberPermissionsResponse, 0, len(members)),
	}
	for _, key := range authctx.PermKeys() {
		resp.Catalog = append(resp.Catalog, PermissionInfo{Key: key, Label: authctx.PermLabel(key)})
	}
	for _, r := range roles {
		resp.Roles = append(resp.Roles, RoleResponse{
			ID: r.ID, Key: r.Key, Name: r.Name, Permissions: knownKeysOf(r.Perms),
		})
	}
	for _, m := range members {
		resp.Members = append(resp.Members, MemberPermissionsResponse{
			TeacherID: m.TeacherID,
			FullName:  m.FullName,
			RoleID:    m.RoleID,
			RoleKey:   m.RoleKey,
			Grants:    knownKeysOf(m.Grants),
			Denies:    knownKeysOf(m.Denies),
		})
	}
	return resp, nil
}

// ReplaceRolePermissions swaps a role's permission set; owner-only. The role
// must belong to the caller's center — the path id is only the target,
// tenancy comes from scope.
func (s *Service) ReplaceRolePermissions(ctx context.Context, scope authctx.Scope, roleID uuid.UUID, req RolePermissionsRequest) error {
	if err := requirePermissionAdmin(scope); err != nil {
		return err
	}
	keys, err := normalizeKeys("permissions", req.Permissions)
	if err != nil {
		return err
	}
	// Dual life: while the legacy can_send_reports column is authoritative,
	// reports.send is assignable per member only — a role-held grant has no
	// column to mirror into and would break the phase-4 parity check.
	for _, key := range keys {
		if key == authctx.PermReportsSend {
			return apperror.Invalid("validation failed", map[string]string{
				"permissions": "reports.send can only be granted per member"})
		}
	}
	var ev RolePermissionsChanged
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		role, err := s.repo.GetRole(ctx, scope.CenterID, roleID)
		if err != nil {
			return err
		}
		ev = RolePermissionsChanged{
			OccurredAt: time.Now(),
			CenterID:   scope.CenterID,
			ActorID:    scope.TeacherID,
			RoleID:     role.ID,
			RoleKey:    role.Key,
			Before:     knownKeysOf(role.Perms),
			After:      keys,
		}
		return s.repo.ReplaceRolePermissions(ctx, roleID, keys)
	})
	if errors.Is(err, ErrNotFound) {
		return apperror.NotFound("role")
	}
	if err != nil {
		return apperror.From(err)
	}
	s.publish(ev)
	return nil
}

// AssignMemberRole assigns a member's role; owner-only. Role and member must
// both resolve inside the caller's center; the owner can never be the target
// (they sit outside the role system, same refusal as SetSendReports).
func (s *Service) AssignMemberRole(ctx context.Context, scope authctx.Scope, targetID uuid.UUID, req MemberRoleRequest) error {
	if err := requirePermissionAdmin(scope); err != nil {
		return err
	}
	// The role resolves outside the transaction: roles are never deleted (the
	// three system roles have no delete path), so there is no window in which
	// a resolved role could vanish before the assignment below.
	role, err := s.repo.GetRole(ctx, scope.CenterID, req.RoleID)
	if errors.Is(err, ErrNotFound) {
		return apperror.NotFound("role")
	}
	if err != nil {
		return apperror.Internal(err)
	}
	var ev MemberRoleChanged
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		member, err := s.repo.GetMemberRBAC(ctx, scope.CenterID, targetID)
		if err != nil {
			return err
		}
		ev = MemberRoleChanged{
			OccurredAt: time.Now(),
			CenterID:   scope.CenterID,
			ActorID:    scope.TeacherID,
			TeacherID:  targetID,
			Before:     member.RoleKey,
			After:      role.Key,
		}
		return s.repo.SetMemberRole(ctx, scope.CenterID, targetID, role.ID)
	})
	if errors.Is(err, ErrNotFound) {
		return apperror.NotFound("member")
	}
	if err != nil {
		return apperror.From(err)
	}
	s.publish(ev)
	return nil
}

// ReplaceMemberOverrides swaps a member's grant/deny override lists;
// owner-only. Adding or removing a reports.send grant dual-writes the legacy
// can_send_reports column in the same transaction — the column stays
// authoritative until phase 4 drops it.
func (s *Service) ReplaceMemberOverrides(ctx context.Context, scope authctx.Scope, targetID uuid.UUID, req MemberOverridesRequest) error {
	if err := requirePermissionAdmin(scope); err != nil {
		return err
	}
	grants, err := normalizeKeys("grants", req.Grants)
	if err != nil {
		return err
	}
	denies, err := normalizeKeys("denies", req.Denies)
	if err != nil {
		return err
	}
	for _, key := range denies {
		for _, g := range grants {
			if key == g {
				return apperror.Invalid("validation failed", map[string]string{
					"denies": fmt.Sprintf("key %q cannot be both granted and denied", key)})
			}
		}
	}
	canSend := false
	for _, key := range grants {
		if key == authctx.PermReportsSend {
			canSend = true
		}
	}
	var ev MemberOverridesChanged
	err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
		member, err := s.repo.GetMemberRBAC(ctx, scope.CenterID, targetID)
		if err != nil {
			return err
		}
		ev = MemberOverridesChanged{
			OccurredAt:   time.Now(),
			CenterID:     scope.CenterID,
			ActorID:      scope.TeacherID,
			TeacherID:    targetID,
			BeforeGrants: knownKeysOf(member.Grants),
			BeforeDenies: knownKeysOf(member.Denies),
			AfterGrants:  grants,
			AfterDenies:  denies,
		}
		return s.repo.ReplaceMemberOverrides(ctx, scope.CenterID, targetID, grants, denies, canSend)
	})
	if errors.Is(err, ErrNotFound) {
		return apperror.NotFound("member")
	}
	if err != nil {
		return apperror.From(err)
	}
	s.publish(ev)
	return nil
}
