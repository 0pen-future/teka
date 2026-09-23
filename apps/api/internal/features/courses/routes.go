package courses

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the course catalog under /courses.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, auth ...gin.HandlerFunc) {
	g := rg.Group("/courses", auth...)
	g.GET("", h.list)
	g.POST("", h.create)
	g.GET("/:id", h.get)
	g.PUT("/:id", h.update)
	g.DELETE("/:id", h.delete)
	g.POST("/:id/archive", h.archive)
	g.PUT("/:id/tuition-packs", h.setTuitionPacks)
	g.GET("/:id/paths", h.listPaths)
}
