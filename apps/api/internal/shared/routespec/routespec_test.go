package routespec

import (
	"fmt"
	"testing"

	"teka/apps/api/internal/shared/authctx"
)

func mutating(method string) bool { return IsMutating(method) }

// noSourceAllowlist is every mutating route deliberately excluded from the
// request-audit path with a reason recorded on the Spec itself (see Specs):
// the public invitation routes never carry a session, so the request
// middleware's authenticated-actor gate always skips them regardless of
// Source, and accept is audited instead through the invitations service's
// own MemberJoined event.
var noSourceAllowlist = map[string]bool{
	"POST /api/v1/invitations/preview": true,
	"POST /api/v1/invitations/accept":  true,
}

// TestNoDuplicateRoute proves the manifest declares each (method, path) at
// most once — a real risk during the migration since audit.actions was a map
// (dedup automatic) and routePolicies was a slice (dedup was not).
func TestNoDuplicateRoute(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range Specs {
		id := s.Method + " " + s.Path
		if seen[id] {
			t.Errorf("%s declared twice in the manifest", id)
		}
		seen[id] = true
	}
}

// TestEveryKindIsValid proves every Spec carries one of the six recognized
// Kind values.
func TestEveryKindIsValid(t *testing.T) {
	valid := map[Kind]bool{
		KindPublic: true, KindPublicToken: true, KindSelf: true,
		KindOwnerOnly: true, KindPermission: true, KindService: true,
	}
	for _, s := range Specs {
		if !valid[s.Kind] {
			t.Errorf("%s %s: unknown kind %q", s.Method, s.Path, s.Kind)
		}
	}
}

// TestPermissionKindHasGrantableKeyOthersDoNot proves KindPermission carries
// exactly one grantable catalog key and no other Kind carries one.
func TestPermissionKindHasGrantableKeyOthersDoNot(t *testing.T) {
	grantable := map[string]bool{}
	for _, key := range authctx.GrantableKeys() {
		grantable[key] = true
	}
	for _, s := range Specs {
		id := fmt.Sprintf("%s %s", s.Method, s.Path)
		switch s.Kind {
		case KindPermission:
			if !grantable[s.Key] {
				t.Errorf("%s: permission key %q is not a grantable catalog key", id, s.Key)
			}
		default:
			if s.Key != "" {
				t.Errorf("%s: kind %s must not carry a permission key, has %q", id, s.Kind, s.Key)
			}
		}
	}
}

// TestEveryValidAuditSource proves every Spec's Audit.Source is one of the
// five recognized values.
func TestEveryValidAuditSource(t *testing.T) {
	valid := map[AuditSource]bool{
		SourceRequest: true, SourceService: true, SourceAuthSession: true,
		SourceAnonymous: true, SourceNone: true,
	}
	for _, s := range Specs {
		if !valid[s.Audit.Source] {
			t.Errorf("%s %s: unknown audit source %q", s.Method, s.Path, s.Audit.Source)
		}
	}
}

// TestMutatingRoutesHaveAnAuditSource proves every mutating route (POST,
// PUT, PATCH, DELETE) carries a real audit classification — SourceNone is
// reserved for non-mutating routes, except the two documented exceptions in
// noSourceAllowlist. A mutating route silently defaulting to SourceNone
// would mean a write nobody decided whether to audit.
func TestMutatingRoutesHaveAnAuditSource(t *testing.T) {
	for _, s := range Specs {
		if !mutating(s.Method) {
			continue
		}
		id := s.Method + " " + s.Path
		if s.Audit.Source == SourceNone && !noSourceAllowlist[id] {
			t.Errorf("%s: mutating route has SourceNone and is not in the documented allowlist", id)
		}
	}
}

// TestRequestSourceRoutesHaveAnAction proves every SourceRequest Spec
// carries a non-empty Action. This is the fail-closed replacement for
// audit.LookupAction's old silent "METHOD route" fallback: a route
// classified as request-audited but missing its Action would previously
// degrade quietly to a less-readable label instead of failing a test.
func TestRequestSourceRoutesHaveAnAction(t *testing.T) {
	for _, s := range Specs {
		if s.Audit.Source != SourceRequest {
			continue
		}
		if s.Audit.Action == "" {
			t.Errorf("%s %s: SourceRequest with empty Action", s.Method, s.Path)
		}
	}
}

// TestSamePathSameAuditSource proves every mutating Spec sharing a path
// agrees on Audit.Source. WithSource derives its skip sets keyed by path
// only (matching how the request middleware checks c.FullPath(), not
// method+path), so two mutating Specs at the same path with different
// Source values would make the derived set mean something different from
// what either Spec intended.
func TestSamePathSameAuditSource(t *testing.T) {
	sourceByPath := map[string]AuditSource{}
	specByPath := map[string]string{}
	for _, s := range Specs {
		if !mutating(s.Method) {
			continue
		}
		if prev, ok := sourceByPath[s.Path]; ok && prev != s.Audit.Source {
			t.Errorf("%s: audit source %q conflicts with %s's %q at the same path",
				s.Method+" "+s.Path, s.Audit.Source, specByPath[s.Path], prev)
			continue
		}
		sourceByPath[s.Path] = s.Audit.Source
		specByPath[s.Path] = s.Method
	}
}

// TestSkipSourceNeverSharesPathWithAuditedRoute proves the failure mode the
// path-only derivation actually creates cannot happen: because the
// middleware skips by path, a Spec of any method carrying a skip source
// (service, auth_session, anonymous) at a path would silently swallow the
// audit row of a SourceRequest mutation at that same path — e.g. marking
// GET /api/v1/students as SourceService would stop POST /api/v1/students
// from ever being audited while every other test stayed green.
func TestSkipSourceNeverSharesPathWithAuditedRoute(t *testing.T) {
	skipAt := map[string]string{}
	auditedAt := map[string]string{}
	for _, s := range Specs {
		switch s.Audit.Source {
		case SourceService, SourceAuthSession, SourceAnonymous:
			skipAt[s.Path] = s.Method + " " + s.Path + " (" + string(s.Audit.Source) + ")"
		case SourceRequest:
			auditedAt[s.Path] = s.Method + " " + s.Path
		}
	}
	for path, skip := range skipAt {
		if audited, ok := auditedAt[path]; ok {
			t.Errorf("%s would skip the request audit of %s: the middleware matches skip sets by path only", skip, audited)
		}
	}
}

// TestPoliciesReturnsACopy proves Policies() callers cannot mutate Specs
// through the returned slice's backing array.
func TestPoliciesReturnsACopy(t *testing.T) {
	out := Policies()
	if len(out) == 0 {
		t.Fatal("Policies() returned no specs")
	}
	original := Specs[0]
	out[0].Key = "tampered"
	if Specs[0] != original {
		t.Errorf("mutating Policies() result changed Specs: got %+v, want %+v", Specs[0], original)
	}
}

// TestLookupFindsEveryDeclaredRoute proves Lookup resolves every Spec by its
// own method and path, and reports ok=false for an undeclared route.
func TestLookupFindsEveryDeclaredRoute(t *testing.T) {
	for _, s := range Specs {
		got, ok := Lookup(s.Method, s.Path)
		if !ok {
			t.Errorf("Lookup(%q, %q) = not found, want the declared spec", s.Method, s.Path)
			continue
		}
		if got != s {
			t.Errorf("Lookup(%q, %q) = %+v, want %+v", s.Method, s.Path, got, s)
		}
	}
	if _, ok := Lookup("GET", "/api/v1/definitely-not-a-route"); ok {
		t.Error("Lookup found a spec for an undeclared route")
	}
}

// TestWithSourceMatchesManifest proves WithSource(s) returns exactly the
// paths carrying a Spec with that source, independent of method.
func TestWithSourceMatchesManifest(t *testing.T) {
	for _, source := range []AuditSource{SourceRequest, SourceService, SourceAuthSession, SourceAnonymous, SourceNone} {
		want := map[string]struct{}{}
		for _, s := range Specs {
			if s.Audit.Source == source {
				want[s.Path] = struct{}{}
			}
		}
		got := WithSource(source)
		if len(got) != len(want) {
			t.Errorf("WithSource(%s): %d paths, want %d", source, len(got), len(want))
		}
		for p := range want {
			if _, ok := got[p]; !ok {
				t.Errorf("WithSource(%s): missing path %q", source, p)
			}
		}
	}
}

// TestKnownSkipSetsUnchanged pins the three skip sets the middleware package
// used to hard-code as literal maps, so the derive-from-manifest migration
// cannot silently change which routes are skipped.
func TestKnownSkipSetsUnchanged(t *testing.T) {
	cases := []struct {
		source AuditSource
		want   []string
	}{
		{SourceAuthSession, []string{
			"/api/v1/auth/login", "/api/v1/auth/logout", "/api/v1/auth/refresh",
		}},
		{SourceService, []string{"/api/v1/enrollments"}},
		{SourceAnonymous, []string{
			"/api/v1/auth/forgot-password", "/api/v1/auth/reset-password",
		}},
	}
	for _, c := range cases {
		got := WithSource(c.source)
		if len(got) != len(c.want) {
			t.Errorf("WithSource(%s): %d paths, want %d (%v)", c.source, len(got), len(c.want), c.want)
			continue
		}
		for _, p := range c.want {
			if _, ok := got[p]; !ok {
				t.Errorf("WithSource(%s): missing expected path %q", c.source, p)
			}
		}
	}
}
