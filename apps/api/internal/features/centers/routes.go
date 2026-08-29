package centers

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the center endpoints under /centers, all behind
// authentication plus per-request scope resolution.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth, resolveScope gin.HandlerFunc) {
	g := rg.Group("/centers", requireAuth, resolveScope)
	g.GET("/me", h.me)
	g.PATCH("/me", h.rename)
	g.DELETE("/me/members/:teacherId", h.removeMember)
	g.POST("/me/members/:teacherId/send-reports", h.grantSendReports)
	g.DELETE("/me/members/:teacherId/send-reports", h.revokeSendReports)
}

// RegisterDashboardRoutes mounts the owner dashboard separately from
// RegisterRoutes: the dashboard consumes classes, sessions, and attendance,
// which are constructed after the membership endpoints are registered.
func RegisterDashboardRoutes(rg *gin.RouterGroup, h *DashboardHandler, requireAuth, resolveScope gin.HandlerFunc) {
	g := rg.Group("/centers/dashboard", requireAuth, resolveScope)
	g.GET("/teachers", h.teachers)
	g.GET("/overview", h.overview)
	g.GET("/teachers/:teacherId/classes", h.teacherClasses)
	g.GET("/teachers/:teacherId/classes/:classId/sessions", h.classSessions)
	g.GET("/sessions/:sessionId", h.session)
}
