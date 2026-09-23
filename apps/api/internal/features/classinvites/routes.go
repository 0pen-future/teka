package classinvites

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the invitation endpoints. Sending is nested under the
// class; everything else lives under /class-invitations, a prefix distinct
// from the public /invitations onboarding routes.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, auth ...gin.HandlerFunc) {
	g := rg.Group("", auth...)
	g.POST("/classes/:id/invitations", h.send)
	g.GET("/class-invitations", h.list)
	g.POST("/class-invitations/:id/accept", h.accept)
	g.POST("/class-invitations/:id/decline", h.decline)
	g.POST("/class-invitations/:id/cancel", h.cancel)
	g.POST("/class-invitations/:id/remind", h.remind)
	g.POST("/class-invitations/:id/confirm", h.confirm)
}
