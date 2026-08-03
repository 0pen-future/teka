package auth

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts the auth endpoints under rg (expected: /api/v1).
// Refresh and logout authenticate via the refresh cookie, not the access
// token, so no auth route needs requireAuth; the profile lives at /me on the
// teachers feature.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler) {
	g := rg.Group("/auth")
	g.POST("/register", h.register)
	g.POST("/login", h.login)
	g.POST("/refresh", h.refresh)
	g.POST("/logout", h.logout)
}
