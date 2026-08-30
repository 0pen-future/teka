package authctx

// Permission keys are the code-owned RBAC vocabulary: the database stores
// assignments (role sets, member overrides), never definitions. Keys are
// frozen per release — adding one is a code change, and an unknown key read
// back from the database is ignored so a code rollback cannot poison scope
// resolution.
//
// Deliberately NOT keys (owner-only forever, one-hop escalation risk):
// permission management itself, ownership handoff, and every write-on-behalf
// branch — those gates stay on Scope.IsOwner.
const (
	// PermDataViewCenterWide is the single data-scoping axis: repositories
	// widen from own-rows to center-wide reads through Scope.CenterWide()
	// only, never through Has() directly.
	PermDataViewCenterWide = "data.view_center_wide"
	PermReportsSend        = "reports.send"
	PermMembersManage      = "members.manage"
	PermCenterManage       = "center.manage"
	PermInvitationsManage  = "invitations.manage"
	PermAuditRead          = "audit.read"
	PermImportsRun         = "imports.run"
	PermDashboardView      = "dashboard.view"
	// PermTeachingReviewQueue grants lesson-plan review visibility on its
	// own, without exposing the center-wide financial/attendance dashboard
	// behind PermDashboardView. Review WRITES (approve/redo/reopen) stay
	// owner-only.
	PermTeachingReviewQueue = "teaching.review_queue"
)

// permRegistry is the closed set of valid keys in stable display order, and
// permLabels the Vietnamese label per key. Both are derived views over
// permCatalog (catalog.go), filled by its init — the catalog is the single
// definition source. The API response is the single label source — the web
// renders what it receives and keeps no duplicate label map.
var (
	permRegistry = make([]string, 0, 80)
	permLabels   = make(map[string]string, 80)
)

// PermKeys returns the registered permission keys in stable order; callers
// get a copy and cannot mutate the registry.
func PermKeys() []string {
	out := make([]string, len(permRegistry))
	copy(out, permRegistry)
	return out
}

// PermLabel returns the Vietnamese display label of a registry key ("" for
// keys outside the registry).
func PermLabel(key string) string {
	return permLabels[key]
}

// KnownPerm reports whether key belongs to the registry.
func KnownPerm(key string) bool {
	for _, k := range permRegistry {
		if k == key {
			return true
		}
	}
	return false
}

// PermSet is a member's effective permission keys. It is resolved once per
// request and read-only after SetScope: Scope is stored by value in the gin
// context but copies share this map, so nothing may write to it post-resolve.
type PermSet map[string]struct{}

// BuildPermSet combines role permissions with member overrides into the
// effective set: (role ∪ grants) − denies. Keys outside the registry are
// dropped — the database may hold assignments for keys a code rollback no
// longer defines. Aliased keys expand on every input list before the set
// algebra, so a legacy grant grants its whole canonical equivalence class and
// a legacy deny removes it; a canonical key never expands back to its legacy
// alias.
func BuildPermSet(rolePerms, grants, denies []string) PermSet {
	set := make(PermSet, len(rolePerms)+len(grants))
	add := func(key string) {
		if KnownPerm(key) {
			set[key] = struct{}{}
		}
		for _, alias := range permAliases[key] {
			set[alias] = struct{}{}
		}
	}
	for _, key := range rolePerms {
		add(key)
	}
	for _, key := range grants {
		add(key)
	}
	for _, key := range denies {
		delete(set, key)
		for _, alias := range permAliases[key] {
			delete(set, alias)
		}
	}
	return set
}

// HasKey reports raw set membership — no owner bypass. Scope.Has is the
// authorization check; this exists for callers combining sources before a
// Scope exists.
func (p PermSet) HasKey(key string) bool {
	_, ok := p[key]
	return ok
}

// Has reports whether the caller holds the permission. The owner is the
// implicit superuser outside the role tables: Has is true unconditionally
// for them. Repositories must not call Has — data scoping goes through
// CenterWideFor(<resource>.view_all) only.
func (s Scope) Has(key string) bool {
	if s.IsOwner {
		return true
	}
	_, ok := s.Perms[key]
	return ok
}

// EffectiveKeys returns the caller's effective permission keys in registry
// order — the owner, as implicit superuser, gets the full registry. This is
// the array API responses expose so clients can gate navigation without
// re-deriving the owner bypass.
func (s Scope) EffectiveKeys() []string {
	out := make([]string, 0, len(permRegistry))
	for _, key := range permRegistry {
		if s.Has(key) {
			out = append(out, key)
		}
	}
	return out
}

// CenterWide reports whether the caller sees the whole center's data rather
// than only their own rows — the single scoping switch repositories branch
// on.
func (s Scope) CenterWide() bool {
	return s.IsOwner || s.Has(PermDataViewCenterWide)
}
