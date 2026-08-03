package contacts

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the contact endpoints under /contacts, all behind
// authentication.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth gin.HandlerFunc) {
	g := rg.Group("/contacts", requireAuth)
	g.POST("", h.create)
	g.GET("", h.list)
	g.GET("/:id", h.get)
	g.PUT("/:id", h.update)
	g.DELETE("/:id", h.remove)
}
