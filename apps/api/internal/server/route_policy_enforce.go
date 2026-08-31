package server

import (
	"log/slog"

	"github.com/gin-gonic/gin"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/response"
)

// enforceRoutePolicy turns the route-policy manifest into the live
// authorization layer for authenticated routes. It is the last link of the
// one ordered chain every authenticated feature route mounts — authentication
// (RequireAuth), then fresh membership (ResolveScope), then this — and
// decides from the manifest entry of the matched route template:
//
//   - self: authentication and live membership are the whole policy.
//   - owner_only: hard ownership gate; a member holding every catalog key
//     still fails it.
//   - permission: one grantable catalog key; the owner passes implicitly.
//     Tenant scope, object visibility, and class-capability checks remain
//     independent layers in the services and repositories behind it.
//
// A route that reaches it without a manifest entry fails closed even for the
// owner: registering an authenticated route means classifying it, and the
// bidirectional manifest test catches that before runtime does. A request
// with no resolved scope is a wiring bug and fails closed as unauthorized.
//
// Denials are logged with the canonical key and reason — countable per key
// without carrying any request payload.
func enforceRoutePolicy(log *slog.Logger) gin.HandlerFunc {
	type routeID struct{ method, path string }
	policies := make(map[routeID]RoutePolicy, len(routePolicies))
	for _, p := range routePolicies {
		policies[routeID{p.Method, p.Path}] = p
	}
	deny := func(c *gin.Context, p RoutePolicy, reason, message string) {
		scope, _ := authctx.ScopeFrom(c)
		log.Warn("authorization denied",
			"method", c.Request.Method,
			"route", c.FullPath(),
			"policy", string(p.Kind),
			"key", p.Key,
			"reason", reason,
			"center_id", scope.CenterID,
			"teacher_id", scope.TeacherID,
		)
		response.Err(c, apperror.Forbidden(message))
	}
	return func(c *gin.Context) {
		scope, ok := authctx.ScopeFrom(c)
		if !ok {
			response.Err(c, apperror.Unauthorized("authentication required"))
			return
		}
		p, ok := policies[routeID{c.Request.Method, c.FullPath()}]
		if !ok {
			deny(c, RoutePolicy{}, "unclassified_route", "Bạn không có quyền thực hiện thao tác này")
			return
		}
		switch p.Kind {
		case PolicyOwnerOnly:
			if !scope.IsOwner {
				deny(c, p, "owner_only", "Chỉ chủ trung tâm được thực hiện thao tác này")
				return
			}
		case PolicyPermission:
			if !scope.Has(p.Key) {
				deny(c, p, "missing_permission", "Bạn không có quyền thực hiện thao tác này")
				return
			}
		}
		c.Next()
	}
}
