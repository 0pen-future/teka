package classstaff

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the staff endpoints on the /classes/:id group — the
// same grouping grading and teaching use for their class-nested resources.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth, resolveScope gin.HandlerFunc) {
	g := rg.Group("/classes", requireAuth, resolveScope)
	g.GET("/:id/staff", h.list)
	g.POST("/:id/staff", h.assign)
	g.DELETE("/:id/staff/:staffId", h.remove)
}
