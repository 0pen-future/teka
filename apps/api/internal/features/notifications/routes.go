package notifications

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the notification endpoints under /billing-periods
// and /notifications, all behind authentication and center-scope resolution.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth, resolveScope gin.HandlerFunc) {
	periods := rg.Group("/billing-periods", requireAuth, resolveScope)
	periods.POST("/:id/notifications/bulk", h.bulkSend)
	periods.GET("/:id/notifications", h.list)
	periods.GET("/:id/notifications/run", h.runSnapshot)
	periods.POST("/:id/notifications/run/resume", h.resumeRun)

	g := rg.Group("/notifications", requireAuth, resolveScope)
	g.POST("/mark-sent", h.markSent)
}
