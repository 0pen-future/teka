package centers

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the center endpoints under /centers, all behind
// authentication plus per-request scope resolution.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth, resolveScope gin.HandlerFunc) {
	g := rg.Group("/centers", requireAuth, resolveScope)
	g.GET("/me", h.me)
	g.PATCH("/me", h.rename)
	g.POST("/join", h.join)
	g.DELETE("/me/members/:teacherId", h.removeMember)
}
