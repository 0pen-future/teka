package invitations

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the invitation endpoints under /centers/me/invitations,
// behind authentication and center-scope resolution. Every handler further
// enforces scope.IsOwner — members do not manage onboarding.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth, resolveScope gin.HandlerFunc) {
	g := rg.Group("/centers/me/invitations", requireAuth, resolveScope)
	g.POST("", h.create)
	g.GET("", h.list)
	g.DELETE("/:id", h.revoke)
}

// RegisterPublicRoutes mounts the unauthenticated accept flow under
// /invitations, outside requireAuth/resolveScope — an invitee has no session
// yet. previewLimit and acceptLimit are per-route rate limiters (the token
// travels in the body, so callers key on it via middleware.JSONBodyKey) that
// bound brute-force guessing against the token space.
func RegisterPublicRoutes(rg *gin.RouterGroup, h *Handler, previewLimit, acceptLimit gin.HandlerFunc) {
	g := rg.Group("/invitations")
	g.POST("/preview", previewLimit, h.preview)
	g.POST("/accept", acceptLimit, h.accept)
}
