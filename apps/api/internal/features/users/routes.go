package users

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts the users endpoints. requireAuth guards every route;
// requireAdmin additionally guards create, list, and delete. Get/update do
// their own admin-or-self checks in the service.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth, requireAdmin gin.HandlerFunc) {
	g := rg.Group("/users", requireAuth)
	g.POST("", requireAdmin, h.create)
	g.GET("", requireAdmin, h.list)
	g.GET("/:id", h.get)
	g.PATCH("/:id", h.update)
	g.DELETE("/:id", requireAdmin, h.remove)
}
