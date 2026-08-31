package grading

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the grading endpoints: the owner-only score-set CRUD
// gets its own /score-sets group; the class snapshot assign/clear and the
// shared component read join the /classes/:id group; the session score
// read/write join the /sessions/:id group — the same grouping teaching uses.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, auth ...gin.HandlerFunc) {
	setGroup := rg.Group("/score-sets", auth...)
	setGroup.GET("", h.listSets)
	setGroup.POST("", h.createSet)
	setGroup.PUT("/:id", h.updateSet)
	setGroup.DELETE("/:id", h.deleteSet)

	classGroup := rg.Group("/classes", auth...)
	classGroup.POST("/:id/score-set", h.assignScoreSet)
	classGroup.DELETE("/:id/score-set", h.clearScoreSet)
	classGroup.GET("/:id/score-components", h.getClassComponents)

	sessionGroup := rg.Group("/sessions", auth...)
	sessionGroup.GET("/:id/scores", h.getSessionScores)
	sessionGroup.PUT("/:id/scores", h.putSessionScores)
}
