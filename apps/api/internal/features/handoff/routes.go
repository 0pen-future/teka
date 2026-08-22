package handoff

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the handoff endpoint on the classes path, behind
// authentication and center-scope resolution. The service enforces owner-only
// access; there is no owner middleware in this codebase and one endpoint is not
// reason enough to introduce one.
//
// It lives on /classes/:id/teacher rather than under its own group because it
// is a class action, not a resource of its own. The :id param name matches the
// classes group's, so the two never conflict at the router.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth, resolveScope gin.HandlerFunc) {
	rg.PUT("/classes/:id/teacher", requireAuth, resolveScope, h.reassign)
}
