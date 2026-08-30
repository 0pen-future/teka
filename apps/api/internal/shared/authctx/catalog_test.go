package authctx

import (
	"reflect"
	"testing"
)

// The nine pre-catalog keys. Eight stay canonical under the same string; the
// data-scoping key is deprecated and aliased to the per-resource view_all set.
var legacyIdentityKeys = []string{
	PermReportsSend,
	PermMembersManage,
	PermCenterManage,
	PermInvitationsManage,
	PermAuditRead,
	PermImportsRun,
	PermDashboardView,
	PermTeachingReviewQueue,
}

// The full decomposition of data.view_center_wide: one scope key per
// resource whose repository widens on CenterWide().
var expectedScopeKeys = []string{
	PermClassesViewAll,
	PermContactsViewAll,
	PermStudentsViewAll,
	PermEnrollmentsViewAll,
	PermSessionsViewAll,
	PermAttendanceViewAll,
	PermScoresViewAll,
	PermTeachingViewAll,
	PermBillingViewAll,
	PermPaymentsViewAll,
	PermStatementsViewAll,
	PermNotificationsViewAll,
}

func TestCatalogWellFormed(t *testing.T) {
	defs := PermDefs()
	if len(defs) == 0 {
		t.Fatal("catalog is empty")
	}
	crudVerbs := map[string]bool{"create": true, "list": true, "read": true, "edit": true, "delete": true}
	seen := map[string]bool{}
	for i, d := range defs {
		if seen[d.Key] {
			t.Fatalf("key %q defined twice", d.Key)
		}
		seen[d.Key] = true
		if d.Key == "" || d.Resource == "" || d.Action == "" || d.Label == "" || d.Description == "" {
			t.Fatalf("key %q has empty identity/label/description fields: %+v", d.Key, d)
		}
		if d.Key != d.Resource+"."+d.Action {
			t.Fatalf("key %q does not compose from resource %q + action %q", d.Key, d.Resource, d.Action)
		}
		if d.Order != i {
			t.Fatalf("key %q order %d != position %d — serialization must be deterministic", d.Key, d.Order, i)
		}
		switch d.Kind {
		case PermKindCRUD:
			if !crudVerbs[d.Action] {
				t.Fatalf("crud key %q uses non-canonical verb %q", d.Key, d.Action)
			}
		case PermKindScope:
			if d.Action != "view_all" && d.Key != PermDataViewCenterWide {
				t.Fatalf("scope key %q must use action view_all", d.Key)
			}
		case PermKindSpecial:
			if crudVerbs[d.Action] {
				t.Fatalf("special key %q must not reuse a CRUD verb", d.Key)
			}
		default:
			t.Fatalf("key %q has unknown kind %q", d.Key, d.Kind)
		}
		switch d.Risk {
		case RiskLow, RiskMedium, RiskHigh:
		default:
			t.Fatalf("key %q has invalid risk %q", d.Key, d.Risk)
		}
	}
}

// Registry guard: catalog keys and class-staff capability strings are two
// independent vocabularies that must stay mechanically disjoint.
func TestCatalogNeverCollidesWithClassStaffCapabilities(t *testing.T) {
	caps := []ClassCapability{
		CapAttendanceWrite, CapScoresWrite, CapRemarksWrite, CapLessonPlanWrite,
		CapEnrollmentWrite, CapSessionsWrite, CapStatementSend,
	}
	for _, cap := range caps {
		if KnownPerm(string(cap)) {
			t.Fatalf("catalog key %q collides with a class-staff capability string", cap)
		}
	}
}

func TestLegacyKeysRemainCanonical(t *testing.T) {
	for _, key := range legacyIdentityKeys {
		d, ok := PermDefOf(key)
		if !ok {
			t.Fatalf("legacy key %q missing from catalog", key)
		}
		if d.Deprecated || !d.Grantable {
			t.Fatalf("legacy identity key %q must stay grantable and non-deprecated: %+v", key, d)
		}
	}
	d, ok := PermDefOf(PermDataViewCenterWide)
	if !ok {
		t.Fatal("data.view_center_wide must remain a known key during compatibility")
	}
	if !d.Deprecated || d.Grantable {
		t.Fatalf("data.view_center_wide must be deprecated and non-grantable for assignment: %+v", d)
	}
}

func TestScopeKeysCompleteAndHighRisk(t *testing.T) {
	for _, key := range expectedScopeKeys {
		d, ok := PermDefOf(key)
		if !ok {
			t.Fatalf("scope key %q missing from catalog", key)
		}
		if d.Kind != PermKindScope || d.Risk != RiskHigh || !d.Grantable {
			t.Fatalf("scope key %q must be kind=scope, risk=high, grantable: %+v", key, d)
		}
	}
	var scopeCount int
	for _, d := range PermDefs() {
		if d.Kind == PermKindScope && !d.Deprecated {
			scopeCount++
		}
	}
	if scopeCount != len(expectedScopeKeys) {
		t.Fatalf("catalog has %d active scope keys, expected %d", scopeCount, len(expectedScopeKeys))
	}
}

// A legacy grant resolves to every per-resource canonical grant, and the
// legacy key itself stays effective so pre-cutover CenterWide() repositories
// keep their behavior.
func TestLegacyGrantExpandsToAllScopeKeys(t *testing.T) {
	set := BuildPermSet([]string{PermDataViewCenterWide}, nil, nil)
	for _, key := range expectedScopeKeys {
		if !set.HasKey(key) {
			t.Fatalf("legacy grant must expand to %q", key)
		}
	}
	if !set.HasKey(PermDataViewCenterWide) {
		t.Fatal("legacy key must stay effective during compatibility")
	}
	sc := Scope{Perms: set}
	if !sc.CenterWide() {
		t.Fatal("CenterWide() must hold for legacy holders")
	}
	for _, key := range expectedScopeKeys {
		if !sc.CenterWideFor(key) {
			t.Fatalf("CenterWideFor(%q) must hold for legacy holders", key)
		}
	}
}

// Any equivalent deny wins: a legacy deny removes the whole equivalence
// class, even keys granted under their canonical name.
func TestLegacyDenyBeatsCanonicalGrant(t *testing.T) {
	set := BuildPermSet(
		[]string{PermStudentsViewAll},
		[]string{PermClassesViewAll},
		[]string{PermDataViewCenterWide},
	)
	for _, key := range append([]string{PermDataViewCenterWide}, expectedScopeKeys...) {
		if set.HasKey(key) {
			t.Fatalf("legacy deny must remove %q", key)
		}
	}
}

// A deny of one canonical key never propagates back through the legacy key:
// the other resources stay widened.
func TestCanonicalDenyDoesNotPropagateBack(t *testing.T) {
	set := BuildPermSet([]string{PermDataViewCenterWide}, nil, []string{PermStudentsViewAll})
	sc := Scope{Perms: set}
	if sc.CenterWideFor(PermStudentsViewAll) {
		t.Fatal("denied canonical key must not widen")
	}
	if !sc.CenterWideFor(PermClassesViewAll) || !sc.CenterWideFor(PermBillingViewAll) {
		t.Fatal("deny of one resource must leave the others widened")
	}
	// Known pre-cutover gap, closed per-resource in the repository cutover:
	// the legacy key itself stays effective for repositories still branching
	// on CenterWide().
	if !sc.CenterWide() {
		t.Fatal("legacy key must survive a single-canonical deny during compatibility")
	}
}

// Forged assignment rows for keys outside the catalog — including class-staff
// capability strings — must drop out of the effective set.
func TestForgedRowsDropped(t *testing.T) {
	set := BuildPermSet(nil, []string{"handoff.execute", "attendance.write", "classes.*"}, nil)
	if len(set) != 0 {
		t.Fatalf("forged keys must be dropped, got %v", set)
	}
}

// GrantableKeys feeds assignment validation: deprecated keys are not
// assignable, everything else in the catalog is.
func TestGrantableKeys(t *testing.T) {
	grantable := map[string]bool{}
	for _, key := range GrantableKeys() {
		grantable[key] = true
	}
	if grantable[PermDataViewCenterWide] {
		t.Fatal("deprecated key must not be assignable")
	}
	for _, key := range append(append([]string{}, legacyIdentityKeys...), expectedScopeKeys...) {
		if !grantable[key] {
			t.Fatalf("key %q must be assignable", key)
		}
	}
}

func TestPermDefsDeterministicAndImmutable(t *testing.T) {
	a, b := PermDefs(), PermDefs()
	if !reflect.DeepEqual(a, b) {
		t.Fatal("PermDefs must serialize deterministically")
	}
	a[0].Key = "mutated"
	if PermDefs()[0].Key == "mutated" {
		t.Fatal("callers must not be able to mutate the catalog")
	}
}

// Require is the service-boundary check: internal callers that bypass HTTP
// middleware still hit the same policy.
func TestRequire(t *testing.T) {
	owner := Scope{IsOwner: true}
	if err := Require(owner, PermPaymentsReverse); err != nil {
		t.Fatalf("owner bypass must hold: %v", err)
	}
	holder := Scope{Perms: BuildPermSet([]string{PermPaymentsReverse}, nil, nil)}
	if err := Require(holder, PermPaymentsReverse); err != nil {
		t.Fatalf("holder must pass: %v", err)
	}
	if err := Require(Scope{}, PermPaymentsReverse); err == nil {
		t.Fatal("missing key must be forbidden")
	}
}

func TestEffectiveKeysCoversCatalogInOrder(t *testing.T) {
	owner := Scope{IsOwner: true}
	keys := owner.EffectiveKeys()
	if !reflect.DeepEqual(keys, PermKeys()) {
		t.Fatal("owner effective keys must equal the full registry in catalog order")
	}
}

// DefaultRoleKeys is the baseline every system role (and role-less legacy
// stint) receives in the compatibility backfill: membership alone granted all
// operational access before the catalog existed, so preserving behavior means
// granting every operational key. Scope keys stay out (visibility must not
// widen), and the legacy identity keys stay out (they were permission-gated
// already — granting them would escalate).
func TestDefaultRoleKeysPreserveLegacyBaseline(t *testing.T) {
	defaults := DefaultRoleKeys()
	if len(defaults) != 53 {
		t.Fatalf("default baseline must hold the 53 operational keys, got %d", len(defaults))
	}
	inDefaults := map[string]bool{}
	for _, key := range defaults {
		if inDefaults[key] {
			t.Errorf("duplicate default key %q", key)
		}
		inDefaults[key] = true
		d, ok := PermDefOf(key)
		if !ok || !d.Grantable {
			t.Errorf("default key %q must be a grantable catalog key", key)
		}
		if d.Kind == PermKindScope {
			t.Errorf("scope key %q must never be a default — it widens visibility", key)
		}
	}
	for _, key := range legacyIdentityKeys {
		if inDefaults[key] {
			t.Errorf("legacy identity key %q must not be granted by default", key)
		}
	}
	// Bidirectional: every grantable operational key outside the legacy
	// identity set is a default — no silent access loss at cutover.
	legacy := map[string]bool{}
	for _, key := range legacyIdentityKeys {
		legacy[key] = true
	}
	for _, d := range PermDefs() {
		if !d.Grantable || d.Kind == PermKindScope || legacy[d.Key] {
			continue
		}
		if !inDefaults[d.Key] {
			t.Errorf("grantable operational key %q missing from the default baseline", d.Key)
		}
	}
	if !reflect.DeepEqual(DefaultRoleKeys(), defaults) {
		t.Fatal("DefaultRoleKeys must be deterministic")
	}
	defaults[0] = "mutated"
	if DefaultRoleKeys()[0] == "mutated" {
		t.Fatal("callers must not be able to mutate the default set")
	}
}

// The catalog version is the CAS anchor for permission-assignment writes: a
// client that loaded the read model under an older catalog must get 409, not
// a silent partial write. Version 1 was the legacy 9-key registry; the
// resource-action catalog is version 2. Bump it on any catalog change that
// alters what a stored assignment means.
func TestCatalogVersion(t *testing.T) {
	if CatalogVersion != 2 {
		t.Fatalf("catalog version must be 2 for the resource-action catalog, got %d", CatalogVersion)
	}
}
