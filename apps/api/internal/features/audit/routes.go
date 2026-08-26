package audit

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the audit trail read endpoint under /audit-logs,
// behind authentication and center-scope resolution. Owner enforcement lives
// in the service so the rule is testable without HTTP.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth, resolveScope gin.HandlerFunc) {
	g := rg.Group("/audit-logs", requireAuth, resolveScope)
	g.GET("", h.list)
}
