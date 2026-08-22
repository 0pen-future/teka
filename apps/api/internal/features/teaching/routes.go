package teaching

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the teaching endpoints: the class-scoped curriculum,
// lesson-plan, and month-marks routes join classes.RegisterRoutes's
// /classes/:id group, the session note/marks writes join
// sessions.RegisterRoutes's /sessions/:id group, and the owner review queue
// gets its own /teaching prefix (it is center-wide, not class-scoped).
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth, resolveScope gin.HandlerFunc) {
	classGroup := rg.Group("/classes", requireAuth, resolveScope)
	classGroup.GET("/:id/curriculum", h.getCurriculum)
	classGroup.PUT("/:id/curriculum", h.putCurriculum)
	classGroup.GET("/:id/lesson-plans", h.listPlans)
	classGroup.PUT("/:id/lesson-plans/:index", h.savePlan)
	classGroup.POST("/:id/lesson-plans/:index/submit", h.submitPlan)
	classGroup.POST("/:id/lesson-plans/:index/approve", h.approvePlan)
	classGroup.POST("/:id/lesson-plans/:index/request-redo", h.requestRedo)
	classGroup.POST("/:id/lesson-plans/:index/reopen", h.reopenPlan)
	classGroup.GET("/:id/marks", h.getMonthMarks)

	sessionGroup := rg.Group("/sessions", requireAuth, resolveScope)
	sessionGroup.PUT("/:id/note", h.putNote)
	sessionGroup.PUT("/:id/marks", h.putMarks)

	teachingGroup := rg.Group("/teaching", requireAuth, resolveScope)
	teachingGroup.GET("/review-queue", h.reviewQueue)
}
