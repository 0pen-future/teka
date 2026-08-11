package zalo

import (
	"github.com/gin-gonic/gin"
)

// RegisterRoutes mounts the Zalo linking endpoints under rg (expected:
// /api/v1). All of them act on "the authenticated teacher's own account", so
// all of them sit behind requireAuth and none takes a teacher id. resolveScope
// runs too — not for center filtering (a Zalo account is personal), but so a
// teacher kicked from their center loses access here the same request it
// bites everywhere else.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth, resolveScope gin.HandlerFunc) {
	g := rg.Group("/me/zalo", requireAuth, resolveScope)
	g.GET("", h.status)
	g.DELETE("", h.unlink)
	g.GET("/friends", h.friends)
	g.POST("/friends/match", h.matchFriends)
	g.POST("/friends/request", h.friendRequest)
	g.POST("/link/start", h.startLink)
	g.GET("/link/status", h.linkStatus)
}
