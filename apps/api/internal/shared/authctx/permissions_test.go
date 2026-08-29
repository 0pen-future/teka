package authctx

import "testing"

// Every exported Perm* constant must be registered exactly once — a constant
// missing from the registry would validate as unknown and be silently
// dropped from every resolved scope.
func TestPermRegistryCompleteAndUnique(t *testing.T) {
	constants := []string{
		PermDataViewCenterWide,
		PermReportsSend,
		PermMembersManage,
		PermCenterManage,
		PermInvitationsManage,
		PermAuditRead,
		PermImportsRun,
		PermDashboardView,
		PermTeachingReviewQueue,
	}
	if len(permRegistry) != len(constants) {
		t.Fatalf("registry has %d keys, %d constants declared", len(permRegistry), len(constants))
	}
	seen := map[string]bool{}
	for _, key := range permRegistry {
		if seen[key] {
			t.Fatalf("key %q registered twice", key)
		}
		seen[key] = true
	}
	for _, key := range constants {
		if !seen[key] {
			t.Fatalf("constant %q missing from registry", key)
		}
	}
}

func TestBuildPermSet(t *testing.T) {
	set := BuildPermSet(
		[]string{PermDashboardView, "ghost.key"},
		[]string{PermAuditRead, PermDashboardView},
		[]string{PermDashboardView},
	)
	if _, ok := set[PermAuditRead]; !ok {
		t.Error("grant dropped")
	}
	if _, ok := set[PermDashboardView]; ok {
		t.Error("deny must beat role + grant")
	}
	if _, ok := set["ghost.key"]; ok {
		t.Error("unknown key must be ignored on read")
	}
	if len(set) != 1 {
		t.Errorf("want exactly 1 effective key, got %v", set)
	}
}

func TestHasAndCenterWide(t *testing.T) {
	owner := Scope{IsOwner: true}
	if !owner.Has("anything.at.all") || !owner.CenterWide() {
		t.Error("owner bypass must hold unconditionally")
	}

	member := Scope{Perms: BuildPermSet([]string{PermAuditRead}, nil, nil)}
	if !member.Has(PermAuditRead) {
		t.Error("member must hold granted key")
	}
	if member.Has(PermImportsRun) || member.CenterWide() {
		t.Error("member must not hold ungranted keys")
	}

	wide := Scope{Perms: BuildPermSet(nil, []string{PermDataViewCenterWide}, nil)}
	if !wide.CenterWide() {
		t.Error("data.view_center_wide must widen scoping")
	}

	var empty Scope
	if empty.Has(PermAuditRead) || empty.CenterWide() {
		t.Error("nil Perms must behave as empty set")
	}
}
