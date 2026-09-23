package paths

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the learning path endpoints under rg; auth runs
// before every handler.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, auth ...gin.HandlerFunc) {
	g := rg.Group("/paths", auth...)
	g.GET("", h.list)
	g.POST("", h.create)
	g.GET("/:id", h.get)
	g.PUT("/:id", h.update)
	g.DELETE("/:id", h.delete)
	g.POST("/:id/stages", h.createStage)
	g.PUT("/:id/stages/order", h.reorderStages)
	g.PUT("/:id/stages/:sid", h.updateStage)
	g.DELETE("/:id/stages/:sid", h.deleteStage)
	g.PUT("/:id/stages/:sid/courses", h.setStageCourses)
}
