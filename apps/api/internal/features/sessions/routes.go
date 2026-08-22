package sessions

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the session endpoints: generation/listing and ad-hoc
// creation nest under /classes/:id/sessions; the lifecycle actions address a
// session directly under /sessions/:id once it exists.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth, resolveScope gin.HandlerFunc) {
	classGroup := rg.Group("/classes", requireAuth, resolveScope)
	classGroup.GET("/:id/sessions", h.listRange)
	classGroup.POST("/:id/sessions", h.createAdHoc)

	sessionGroup := rg.Group("/sessions", requireAuth, resolveScope)
	// /pending must register before /:id — Gin matches routes in
	// registration order within a group, and a static segment loses to an
	// already-registered wildcard if it comes second.
	sessionGroup.GET("/pending", h.pending)
	sessionGroup.GET("/:id", h.get)
	sessionGroup.DELETE("/:id", h.remove)
	sessionGroup.POST("/:id/cancel", h.cancel)
	sessionGroup.POST("/:id/uncancel", h.uncancel)
	sessionGroup.POST("/:id/hold", h.hold)
}
