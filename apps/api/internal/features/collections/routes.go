package collections

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the collection board endpoints under
// /billing-periods, behind authentication and scope resolution.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth, resolveScope gin.HandlerFunc) {
	g := rg.Group("/billing-periods", requireAuth, resolveScope)
	g.GET("/:id/collections", h.list)
	g.GET("/:id/collections/summary", h.summary)
}
