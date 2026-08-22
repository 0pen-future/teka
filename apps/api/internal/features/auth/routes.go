package auth

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts the auth endpoints under rg (expected: /api/v1).
// Refresh and logout authenticate via the refresh cookie, not the access
// token, so no auth route needs requireAuth; the profile lives at /me on the
// teachers feature. Account creation is invite-only now (see
// features/invitations) — there is no public self-registration route.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler) {
	g := rg.Group("/auth")
	g.POST("/login", h.login)
	g.POST("/refresh", h.refresh)
	g.POST("/logout", h.logout)
}

// RegisterPublicRoutes mounts the password-reset flow under /auth, outside
// any authentication — a caller who forgot their password has no session yet.
// forgotLimit and resetLimit are per-route rate limiters keyed on business
// identity (phone, token — never the caller's IP) that bound brute-force
// guessing against the reset token space and spam against a phone number.
func RegisterPublicRoutes(rg *gin.RouterGroup, h *Handler, forgotLimit, resetLimit gin.HandlerFunc) {
	g := rg.Group("/auth")
	g.POST("/forgot-password", forgotLimit, h.forgotPassword)
	g.POST("/reset-password", resetLimit, h.resetPassword)
}
