package payments

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the payment endpoints under /payments, all behind
// authentication.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth, resolveScope gin.HandlerFunc) {
	g := rg.Group("/payments", requireAuth, resolveScope)
	g.POST("", h.record)
	g.GET("", h.list)
	g.GET("/:id", h.get)
	g.PUT("/:id/allocations", h.reallocate)
	g.POST("/:id/allocations/auto", h.autoAllocate)
	g.POST("/:id/reverse", h.reverse)
}
