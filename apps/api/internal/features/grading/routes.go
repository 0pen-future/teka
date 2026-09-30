package grading

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the grading endpoints: the class component read joins
// the /classes/:id group; the session score read/write join the /sessions/:id
// group — the same grouping teaching uses. A class's components are written
// only by applying a program template (classprogram), never over HTTP here.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, auth ...gin.HandlerFunc) {
	classGroup := rg.Group("/classes", auth...)
	classGroup.GET("/:id/score-components", h.getClassComponents)

	sessionGroup := rg.Group("/sessions", auth...)
	sessionGroup.GET("/:id/scores", h.getSessionScores)
	sessionGroup.PUT("/:id/scores", h.putSessionScores)
}
