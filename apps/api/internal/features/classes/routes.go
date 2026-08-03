package classes

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the class endpoints under /classes, all behind
// authentication; schedules are a nested sub-resource.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth gin.HandlerFunc) {
	g := rg.Group("/classes", requireAuth)
	g.POST("", h.create)
	g.GET("", h.list)
	g.GET("/:id", h.get)
	g.PUT("/:id", h.update)
	g.POST("/:id/archive", h.archive)
	g.DELETE("/:id", h.remove)
	g.POST("/:id/schedules", h.addSchedule)
	g.PUT("/:id/schedules/:scheduleID", h.updateSchedule)
	g.DELETE("/:id/schedules/:scheduleID", h.removeSchedule)
}
