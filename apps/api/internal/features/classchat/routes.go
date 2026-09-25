package classchat

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the class chat endpoints under /classes/:id,
// joining classes.RegisterRoutes's group the way teaching does.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, auth ...gin.HandlerFunc) {
	classGroup := rg.Group("/classes", auth...)
	classGroup.GET("/:id/messages", h.list)
	classGroup.POST("/:id/messages", h.post)
	classGroup.DELETE("/:id/messages/:mid", h.remove)
}
