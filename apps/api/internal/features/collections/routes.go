package collections

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the collection board endpoints under
// /billing-periods, behind authentication.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth gin.HandlerFunc) {
	g := rg.Group("/billing-periods", requireAuth)
	g.GET("/:id/collections", h.list)
	g.GET("/:id/collections/summary", h.summary)
}
