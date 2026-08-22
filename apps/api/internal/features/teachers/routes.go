package teachers

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts the teacher profile endpoints under rg (expected:
// /api/v1). Both operate on "the authenticated teacher", so both sit behind
// requireAuth plus scope resolution — the scope is where account liveness is
// enforced.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth, resolveScope gin.HandlerFunc) {
	rg.GET("/me", requireAuth, resolveScope, h.me)
	rg.PUT("/me", requireAuth, resolveScope, h.updateMe)
}
