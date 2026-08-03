package attendance

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the attendance endpoints under an existing session,
// alongside sessions.RegisterRoutes's /sessions/:id group.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth gin.HandlerFunc) {
	sessionGroup := rg.Group("/sessions", requireAuth)
	sessionGroup.GET("/:id/attendance", h.get)
	sessionGroup.POST("/:id/attendance", h.confirm)
}
