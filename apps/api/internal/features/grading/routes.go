package grading

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the grading endpoints: the owner-only score-set CRUD
// gets its own /score-sets group; the class snapshot assign/clear and the
// shared component read join the /classes/:id group; the session score
// read/write join the /sessions/:id group — the same grouping teaching uses.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, requireAuth, resolveScope gin.HandlerFunc) {
	setGroup := rg.Group("/score-sets", requireAuth, resolveScope)
	setGroup.GET("", h.listSets)
	setGroup.POST("", h.createSet)
	setGroup.PUT("/:id", h.updateSet)
	setGroup.DELETE("/:id", h.deleteSet)

	classGroup := rg.Group("/classes", requireAuth, resolveScope)
	classGroup.POST("/:id/score-set", h.assignScoreSet)
	classGroup.DELETE("/:id/score-set", h.clearScoreSet)
	classGroup.GET("/:id/score-components", h.getClassComponents)

	sessionGroup := rg.Group("/sessions", requireAuth, resolveScope)
	sessionGroup.GET("/:id/scores", h.getSessionScores)
	sessionGroup.PUT("/:id/scores", h.putSessionScores)
}
