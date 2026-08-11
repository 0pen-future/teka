package contacts

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the contact endpoints under /contacts, all behind
// authentication and center-scope resolution.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth, resolveScope gin.HandlerFunc) {
	g := rg.Group("/contacts", requireAuth, resolveScope)
	g.POST("", h.create)
	g.GET("", h.list)
	g.GET("/:id", h.get)
	g.PUT("/:id", h.update)
	g.DELETE("/:id", h.remove)
	g.PUT("/:id/zalo-mapping", h.setZaloMapping)
	g.DELETE("/:id/zalo-mapping", h.clearZaloMapping)
}
