package server

import (
	"fmt"
	"testing"

	"github.com/gin-gonic/gin"

	"teka/apps/api/internal/shared/authctx"
)

// The route-policy manifest and the live engine must agree bidirectionally:
// a route registered without a policy classification fails here, and so does
// a manifest row for a route that no longer exists. Adding a route means
// deciding its policy in the same change.
func TestRoutePolicyCoversEveryRegisteredRoute(t *testing.T) {
	engine, ok := newTestRouter(t).(*gin.Engine)
	if !ok {
		t.Fatalf("test router is not a gin engine")
	}

	registered := map[string]bool{}
	for _, ri := range engine.Routes() {
		registered[ri.Method+" "+ri.Path] = true
	}

	declared := map[string]bool{}
	for _, p := range routePolicies {
		id := p.Method + " " + p.Path
		if declared[id] {
			t.Errorf("route %s declared twice in the manifest", id)
		}
		declared[id] = true
		if !registered[id] {
			t.Errorf("manifest declares %s but the engine does not register it", id)
		}
	}
	for id := range registered {
		if !declared[id] {
			t.Errorf("route %s is registered but has no policy classification", id)
		}
	}
}

func TestRoutePolicyEntriesWellFormed(t *testing.T) {
	grantable := map[string]bool{}
	for _, key := range authctx.GrantableKeys() {
		grantable[key] = true
	}
	for _, p := range routePolicies {
		id := fmt.Sprintf("%s %s", p.Method, p.Path)
		switch p.Kind {
		case PolicyPublic, PolicyPublicToken, PolicySelf, PolicyOwnerOnly:
			if p.Key != "" {
				t.Errorf("%s: kind %s must not carry a permission key, has %q", id, p.Kind, p.Key)
			}
		case PolicyPermission:
			if !grantable[p.Key] {
				t.Errorf("%s: permission key %q is not a grantable catalog key", id, p.Key)
			}
		default:
			t.Errorf("%s: unknown policy kind %q", id, p.Kind)
		}
	}
}

// The owner-only list is frozen by the phase-1 inventory: permission
// administration, class staffing and handoff, sensitive lesson-plan review
// writes, and score-set configuration. None of these may ever appear with a
// grantable policy. (The legacy send-reports toggle routes retired with the
// can_send_reports column — reports.send is granted through overrides now.)
func TestOwnerOnlyRoutesStayHardGated(t *testing.T) {
	frozen := []string{
		"GET /api/v1/centers/me/permissions",
		"PUT /api/v1/centers/me/roles/:roleId/permissions",
		"PUT /api/v1/centers/me/members/:teacherId/role",
		"PUT /api/v1/centers/me/members/:teacherId/overrides",
		"POST /api/v1/classes/:id/staff",
		"DELETE /api/v1/classes/:id/staff/:staffId",
		"PUT /api/v1/classes/:id/teacher",
		"POST /api/v1/classes/:id/lesson-plans/:index/approve",
		"POST /api/v1/classes/:id/lesson-plans/:index/request-redo",
		"POST /api/v1/classes/:id/lesson-plans/:index/reopen",
		"GET /api/v1/score-sets",
		"POST /api/v1/score-sets",
		"PUT /api/v1/score-sets/:id",
		"DELETE /api/v1/score-sets/:id",
		"POST /api/v1/classes/:id/score-set",
		"DELETE /api/v1/classes/:id/score-set",
	}
	byID := map[string]RoutePolicy{}
	for _, p := range routePolicies {
		byID[p.Method+" "+p.Path] = p
	}
	for _, id := range frozen {
		p, ok := byID[id]
		if !ok {
			t.Errorf("frozen owner-only route %s missing from manifest", id)
			continue
		}
		if p.Kind != PolicyOwnerOnly {
			t.Errorf("route %s must stay owner-only, manifest says %s", id, p.Kind)
		}
	}
}
