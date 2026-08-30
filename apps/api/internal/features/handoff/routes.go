package handoff

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the handoff endpoint on the classes path, behind the
// auth chain (authentication, scope resolution, route policy — owner-only per
// the manifest). The service keeps its own owner check so the rule is
// testable without HTTP.
//
// It lives on /classes/:id/teacher rather than under its own group because it
// is a class action, not a resource of its own. The :id param name matches the
// classes group's, so the two never conflict at the router.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, auth ...gin.HandlerFunc) {
	rg.Group("", auth...).PUT("/classes/:id/teacher", h.reassign)
}
