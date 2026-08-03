package teachers

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts the teacher profile endpoints under rg (expected:
// /api/v1). Both operate on "the authenticated teacher", so both sit behind
// requireAuth.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth gin.HandlerFunc) {
	rg.GET("/me", requireAuth, h.me)
	rg.PUT("/me", requireAuth, h.updateMe)
}
