package classprogram

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the class program endpoints under /classes/:id,
// joining classes.RegisterRoutes's group the way teaching does.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, auth ...gin.HandlerFunc) {
	classGroup := rg.Group("/classes", auth...)
	classGroup.GET("/:id/program", h.get)
	classGroup.GET("/:id/program/lessons", h.lessons)
	classGroup.PUT("/:id/program", h.apply)
	classGroup.DELETE("/:id/program", h.remove)
}
