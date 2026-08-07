package zalo

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts the Zalo linking endpoints under rg (expected:
// /api/v1). All of them act on "the authenticated teacher's own account", so
// all of them sit behind requireAuth and none takes a teacher id.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth gin.HandlerFunc) {
	g := rg.Group("/me/zalo", requireAuth)
	g.GET("", h.status)
	g.DELETE("", h.unlink)
	g.POST("/link/start", h.startLink)
	g.GET("/link/status", h.linkStatus)
}
