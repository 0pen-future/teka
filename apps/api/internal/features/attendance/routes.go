package attendance

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the attendance endpoints under an existing session,
// alongside sessions.RegisterRoutes's /sessions/:id group.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth, resolveScope gin.HandlerFunc) {
	sessionGroup := rg.Group("/sessions", requireAuth, resolveScope)
	sessionGroup.GET("/:id/attendance", h.get)
	sessionGroup.POST("/:id/attendance", h.confirm)
}
