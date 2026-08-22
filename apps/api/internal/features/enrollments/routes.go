package enrollments

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the enrollment endpoints under /enrollments, all
// behind authentication and scope resolution.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth, resolveScope gin.HandlerFunc) {
	g := rg.Group("/enrollments", requireAuth, resolveScope)
	g.POST("", h.create)
	g.GET("", h.list)
	g.GET("/:id", h.get)
	g.POST("/:id/end", h.end)
	g.DELETE("/:id", h.remove)
}
