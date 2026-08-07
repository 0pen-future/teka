package notifications

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the notification endpoints under /billing-periods
// and /notifications, all behind authentication.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth gin.HandlerFunc) {
	periods := rg.Group("/billing-periods", requireAuth)
	periods.POST("/:id/notifications/bulk", h.bulkSend)
	periods.GET("/:id/notifications", h.list)
	periods.GET("/:id/notifications/run", h.runSnapshot)
	periods.POST("/:id/notifications/run/resume", h.resumeRun)

	g := rg.Group("/notifications", requireAuth)
	g.POST("/mark-sent", h.markSent)
}
