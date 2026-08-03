package students

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the student endpoints under /students, all behind
// authentication.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth gin.HandlerFunc) {
	g := rg.Group("/students", requireAuth)
	g.POST("", h.create)
	g.GET("", h.list)
	g.GET("/:id", h.get)
	g.PUT("/:id", h.update)
	g.DELETE("/:id", h.remove)
}
