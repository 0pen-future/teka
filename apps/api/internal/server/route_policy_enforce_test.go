package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"teka/apps/api/internal/shared/authctx"
)

// probePath swaps every route parameter for a literal so the request matches
// the registered template (the enforcer keys on c.FullPath(), which gin only
// fills for a matched route).
func probePath(template string) string {
	parts := strings.Split(template, "/")
	for i, p := range parts {
		if strings.HasPrefix(p, ":") || strings.HasPrefix(p, "*") {
			parts[i] = "x"
		}
	}
	return strings.Join(parts, "/")
}

// newPolicyProbe registers every authenticated manifest route on a bare
// engine behind a scope-injection stub and the enforcer — the policy layer
// exercised against the real manifest without JWTs or a database. A nil scope
// simulates a request that skipped ResolveScope entirely.
func newPolicyProbe(t *testing.T, scope *authctx.Scope, logs io.Writer) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	e := gin.New()
	if logs == nil {
		logs = io.Discard
	}
	inject := func(c *gin.Context) {
		if scope != nil {
			authctx.SetScope(c, *scope)
		}
		c.Next()
	}
	enforce := enforceRoutePolicy(slog.New(slog.NewJSONHandler(logs, nil)))
	ok := func(c *gin.Context) { c.Status(http.StatusOK) }
	for _, p := range routePolicies {
		if p.Kind == PolicyPublic || p.Kind == PolicyPublicToken {
			continue
		}
		e.Handle(p.Method, p.Path, inject, enforce, ok)
	}
	// A route registered behind the chain but absent from the manifest must
	// fail closed, whoever calls it.
	e.Handle(http.MethodGet, "/api/v1/unclassified-probe", inject, enforce, ok)
	return e
}

func authedManifestRoutes() []RoutePolicy {
	out := make([]RoutePolicy, 0, len(routePolicies))
	for _, p := range routePolicies {
		if p.Kind == PolicyPublic || p.Kind == PolicyPublicToken {
			continue
		}
		out = append(out, p)
	}
	return out
}

func hit(t *testing.T, e *gin.Engine, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func ownerScope() *authctx.Scope {
	return &authctx.Scope{IsOwner: true}
}

func memberScope(keys ...string) *authctx.Scope {
	return &authctx.Scope{Perms: authctx.BuildPermSet(keys, nil, nil)}
}

// The owner is the implicit superuser: every authenticated route — self,
// owner-only, and permission alike — answers before any grant exists.
func TestPolicyOwnerPassesEveryAuthedRoute(t *testing.T) {
	e := newPolicyProbe(t, ownerScope(), nil)
	for _, p := range authedManifestRoutes() {
		if rec := hit(t, e, p.Method, probePath(p.Path)); rec.Code != http.StatusOK {
			t.Errorf("%s %s: owner got %d, want 200", p.Method, p.Path, rec.Code)
		}
	}
}

// A member holding every grantable key passes every permission route and
// every self route, and still fails every owner-only route: no combination of
// grants escalates past the ownership gate.
func TestPolicyFullGrantMemberStopsAtOwnerOnly(t *testing.T) {
	e := newPolicyProbe(t, memberScope(authctx.GrantableKeys()...), nil)
	for _, p := range authedManifestRoutes() {
		rec := hit(t, e, p.Method, probePath(p.Path))
		want := http.StatusOK
		if p.Kind == PolicyOwnerOnly {
			want = http.StatusForbidden
		}
		if rec.Code != want {
			t.Errorf("%s %s (%s): full-grant member got %d, want %d", p.Method, p.Path, p.Kind, rec.Code, want)
		}
	}
}

// Each permission route demands exactly its own key: holding every other
// grantable key still yields 403 (so list never implies read, read never
// implies edit), and holding only that key yields 200.
func TestPolicyPermissionRoutesRequireExactKey(t *testing.T) {
	grantable := authctx.GrantableKeys()
	for _, p := range authedManifestRoutes() {
		if p.Kind != PolicyPermission {
			continue
		}
		allBut := make([]string, 0, len(grantable)-1)
		for _, k := range grantable {
			if k != p.Key {
				allBut = append(allBut, k)
			}
		}
		e := newPolicyProbe(t, memberScope(allBut...), nil)
		if rec := hit(t, e, p.Method, probePath(p.Path)); rec.Code != http.StatusForbidden {
			t.Errorf("%s %s: member without %s got %d, want 403", p.Method, p.Path, p.Key, rec.Code)
		}
		e = newPolicyProbe(t, memberScope(p.Key), nil)
		if rec := hit(t, e, p.Method, probePath(p.Path)); rec.Code != http.StatusOK {
			t.Errorf("%s %s: member with only %s got %d, want 200", p.Method, p.Path, p.Key, rec.Code)
		}
	}
}

// The notification routes carry their authorization inside the service —
// reports oversight for the family dimension, the class-send gate for class
// copies, and own-period scoping for reads (a member watching the ledger of
// their own period, delegated rows included). The policy layer must therefore
// pass any live member and leave the decision to the service; a route-level
// permission gate here would deny the class secretary and the period's own
// teacher before that authorization could run.
func TestPolicyServiceRoutesPassPlainMember(t *testing.T) {
	serviceRoutes := []string{
		"POST /api/v1/billing-periods/:id/notifications/bulk",
		"GET /api/v1/billing-periods/:id/notifications",
		"GET /api/v1/billing-periods/:id/notifications/preview",
		"GET /api/v1/billing-periods/:id/notifications/run",
		"POST /api/v1/billing-periods/:id/notifications/run/resume",
	}
	byID := map[string]RoutePolicy{}
	for _, p := range routePolicies {
		byID[p.Method+" "+p.Path] = p
	}
	e := newPolicyProbe(t, memberScope(), nil)
	for _, id := range serviceRoutes {
		p, ok := byID[id]
		if !ok {
			t.Errorf("service-authorized route %s missing from manifest", id)
			continue
		}
		if p.Kind != PolicyService {
			t.Errorf("route %s must be service-authorized, manifest says %s", id, p.Kind)
		}
		if rec := hit(t, e, p.Method, probePath(p.Path)); rec.Code != http.StatusOK {
			t.Errorf("%s: plain member got %d at the policy layer, want 200 (service decides)", id, rec.Code)
		}
	}
}

// A member deny wins over the same key arriving through the role — the
// precedence BuildPermSet establishes, proven here at the HTTP layer.
func TestPolicyDenyOverridesRoleGrant(t *testing.T) {
	scope := &authctx.Scope{
		Perms: authctx.BuildPermSet(authctx.GrantableKeys(), nil, []string{authctx.PermClassesList}),
	}
	e := newPolicyProbe(t, scope, nil)
	if rec := hit(t, e, http.MethodGet, "/api/v1/classes"); rec.Code != http.StatusForbidden {
		t.Errorf("denied classes.list: got %d, want 403", rec.Code)
	}
	// The deny is surgical: the read key still answers.
	if rec := hit(t, e, http.MethodGet, "/api/v1/classes/x"); rec.Code != http.StatusOK {
		t.Errorf("classes.read after list deny: got %d, want 200", rec.Code)
	}
}

// A request that reaches the policy layer without a resolved scope is a
// wiring bug and fails closed as unauthorized, never as an open door.
func TestPolicyMissingScopeUnauthorized(t *testing.T) {
	e := newPolicyProbe(t, nil, nil)
	for _, p := range authedManifestRoutes() {
		if rec := hit(t, e, p.Method, probePath(p.Path)); rec.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: no scope got %d, want 401", p.Method, p.Path, rec.Code)
		}
	}
}

// An authenticated route missing from the manifest fails closed even for the
// owner: registering a route means classifying it first.
func TestPolicyUnclassifiedRouteFailsClosed(t *testing.T) {
	e := newPolicyProbe(t, ownerScope(), nil)
	if rec := hit(t, e, http.MethodGet, "/api/v1/unclassified-probe"); rec.Code != http.StatusForbidden {
		t.Errorf("unclassified route: got %d, want 403", rec.Code)
	}
}

// Denials are countable by canonical key and reason from the structured log,
// and the log line carries no request payload.
func TestPolicyDenialLogsKeyAndReason(t *testing.T) {
	var buf bytes.Buffer
	e := newPolicyProbe(t, memberScope(), &buf)
	if rec := hit(t, e, http.MethodPost, "/api/v1/classes"); rec.Code != http.StatusForbidden {
		t.Fatalf("got %d, want 403", rec.Code)
	}
	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("denial log is not one JSON line: %v (%q)", err, buf.String())
	}
	if entry["key"] != authctx.PermClassesCreate {
		t.Errorf("log key = %v, want %s", entry["key"], authctx.PermClassesCreate)
	}
	if entry["reason"] != "missing_permission" {
		t.Errorf("log reason = %v, want missing_permission", entry["reason"])
	}
	if entry["route"] != "/api/v1/classes" {
		t.Errorf("log route = %v, want /api/v1/classes", entry["route"])
	}
}

// The error envelope stays the shared apperror shape on both denial codes.
func TestPolicyDenialEnvelope(t *testing.T) {
	e := newPolicyProbe(t, memberScope(), nil)
	rec := hit(t, e, http.MethodPost, "/api/v1/classes")
	var body struct {
		Success bool `json:"success"`
		Error   *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON body: %v", err)
	}
	if body.Success || body.Error == nil || body.Error.Code != "FORBIDDEN" {
		t.Errorf("body = %s, want success=false code=FORBIDDEN", rec.Body.String())
	}
}

// With enforcement wired into the live router, an authenticated route without
// a token dies at RequireAuth — 401 before scope or policy ever run.
func TestRouterRejectsMissingTokenBeforePolicy(t *testing.T) {
	r := newTestRouter(t)
	for _, path := range []string{"/api/v1/classes", "/api/v1/students", "/api/v1/me"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("GET %s without token: got %d, want 401", path, rec.Code)
		}
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/classes", nil)
	req.Header.Set("Authorization", "Bearer not-a-jwt")
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("GET /api/v1/classes with garbage token: got %d, want 401", rec.Code)
	}
}
