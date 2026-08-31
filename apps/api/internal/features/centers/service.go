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
		// HasKey, not Has: the field mirrors the member's effective
		// reports.send only — the owner's authority flows through
		// ReportsOversight's IsOwner arm, never through this flag.
		CanSendReports: perms.HasKey(authctx.PermReportsSend),
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

// normalizeKeys validates keys against the catalog and returns them deduped
// in stable registry order. An unknown key is a 422 on the given field, and
// so is a deprecated or otherwise non-grantable key — existing rows for those
// stay effective, but new assignment writes must use the canonical keys.
func normalizeKeys(field string, keys []string) ([]string, error) {
	for _, key := range keys {
		d, ok := authctx.PermDefOf(key)
		if !ok {
			return nil, apperror.Invalid("validation failed",
				map[string]string{field: fmt.Sprintf("unknown permission key %q", key)})
		}
		if !d.Grantable {
			return nil, apperror.Invalid("validation failed",
				map[string]string{field: fmt.Sprintf("permission key %q is not assignable", key)})
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
// database but never surface in responses or events. Deprecated keys are
// filtered too: their rows stay effective through alias expansion, but the
// PUT endpoints accept only grantable keys, so emitting one would make every
// read-modify-write save of that role or member fail 422. The backfill
// materialized each deprecated row's canonical equivalents, so a save built
// from this list preserves effective access while converging storage onto
// canonical keys.
func knownKeysOf(joined string) []string {
	set := make(map[string]struct{})
	for _, key := range splitKeys(joined) {
		if d, ok := authctx.PermDefOf(key); ok && !d.Deprecated {
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
	defs := authctx.PermDefs()
	resp := &PermissionsResponse{
		Catalog:        make([]PermissionInfo, 0, len(defs)),
		Roles:          make([]RoleResponse, 0, len(roles)),
		Members:        make([]MemberPermissionsResponse, 0, len(members)),
		CatalogVersion: authctx.CatalogVersion,
	}
	// Deprecated keys stay out of the assignment catalog: they are not
	// grantable, and the UI must steer owners to the canonical keys.
	for _, d := range defs {
		if d.Deprecated {
			continue
		}
		resp.Catalog = append(resp.Catalog, PermissionInfo{
			Key: d.Key, Label: d.Label,
			Resource: d.Resource, Action: d.Action,
			Kind: string(d.Kind), Risk: string(d.Risk),
			Description: d.Description,
		})
	}
	for _, r := range roles {
		resp.Roles = append(resp.Roles, RoleResponse{
			ID: r.ID, Key: r.Key, Name: r.Name, Permissions: knownKeysOf(r.Perms),
			AssignmentVersion: r.AssignmentVersion,
		})
	}
	for _, m := range members {
		resp.Members = append(resp.Members, MemberPermissionsResponse{
			TeacherID:         m.TeacherID,
			FullName:          m.FullName,
			RoleID:            m.RoleID,
			RoleKey:           m.RoleKey,
			Grants:            knownKeysOf(m.Grants),
			Denies:            knownKeysOf(m.Denies),
			AssignmentVersion: m.AssignmentVersion,
		})
	}
	return resp, nil
}

// staleAssignmentConflict is the 409 both replacement writes return on a CAS
// miss: the client's read model is behind, reloading is the only cure.
func staleAssignmentConflict() error {
	return apperror.Conflict("Cấu hình quyền đã thay đổi từ lần tải trước, hãy tải lại rồi lưu lại")
}

// checkCatalogVersion rejects writes composed against a different catalog
// generation than the server's. Zero means a pre-CAS client — allowed until
// the UI cutover completes.
func checkCatalogVersion(v int) error {
	if v != 0 && v != authctx.CatalogVersion {
		return staleAssignmentConflict()
	}
	return nil
}

// ReplaceRolePermissions swaps a role's permission set; owner-only. The role
// must belong to the caller's center — the path id is only the target,
// tenancy comes from scope.
func (s *Service) ReplaceRolePermissions(ctx context.Context, scope authctx.Scope, roleID uuid.UUID, req RolePermissionsRequest) error {
	if err := requirePermissionAdmin(scope); err != nil {
		return err
	}
	if err := checkCatalogVersion(req.CatalogVersion); err != nil {
		return err
	}
	keys, err := normalizeKeys("permissions", req.Permissions)
	if err != nil {
		return err
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
		return s.repo.ReplaceRolePermissions(ctx, roleID, keys, req.AssignmentVersion)
	})
	if errors.Is(err, ErrNotFound) {
		return apperror.NotFound("role")
	}
	if errors.Is(err, ErrStaleVersion) {
		return staleAssignmentConflict()
	}
	if err != nil {
		return apperror.From(err)
	}
	s.publish(ev)
	return nil
}

// AssignMemberRole assigns a member's role; owner-only. Role and member must
// both resolve inside the caller's center; the owner can never be the target
// (they sit outside the role system).
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
// owner-only.
func (s *Service) ReplaceMemberOverrides(ctx context.Context, scope authctx.Scope, targetID uuid.UUID, req MemberOverridesRequest) error {
	if err := requirePermissionAdmin(scope); err != nil {
		return err
	}
	if err := checkCatalogVersion(req.CatalogVersion); err != nil {
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
		return s.repo.ReplaceMemberOverrides(ctx, scope.CenterID, targetID, grants, denies, req.AssignmentVersion)
	})
	if errors.Is(err, ErrNotFound) {
		return apperror.NotFound("member")
	}
	if errors.Is(err, ErrStaleVersion) {
		return staleAssignmentConflict()
	}
	if err != nil {
		return apperror.From(err)
	}
	s.publish(ev)
	return nil
}
