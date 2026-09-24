package library

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the program-template library under /library. Templates,
// materials and exercises are addressed by :id, versions by :vid and lessons
// by :lid so the audit entity id parameter is unambiguous per route.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, auth ...gin.HandlerFunc) {
	g := rg.Group("/library", auth...)
	g.GET("/templates", h.listTemplates)
	g.POST("/templates", h.createTemplate)
	g.GET("/templates/:id", h.getTemplate)
	g.PUT("/templates/:id", h.updateTemplate)
	g.DELETE("/templates/:id", h.deleteTemplate)
	g.GET("/templates/:id/versions", h.listVersions)
	g.POST("/templates/:id/versions", h.createVersion)
	g.POST("/versions/:vid/publish", h.publishVersion)
	g.POST("/versions/:vid/archive", h.archiveVersion)
	g.GET("/versions/:vid", h.getVersion)
	g.GET("/versions/:vid/board", h.getBoard)
	g.PUT("/versions/:vid/log-fields", h.setLogFields)
	g.PUT("/versions/:vid/score-set", h.setScoreSet)
	g.GET("/versions/:vid/lessons", h.listLessons)
	g.POST("/versions/:vid/lessons", h.createLesson)
	g.PUT("/versions/:vid/lessons/order", h.reorderLessons)
	g.GET("/lessons/:lid", h.getLesson)
	g.PUT("/lessons/:lid", h.updateLesson)
	g.DELETE("/lessons/:lid", h.deleteLesson)
	g.PUT("/lessons/:lid/materials", h.setLessonMaterials)
	g.PUT("/lessons/:lid/exercises", h.setLessonExercises)
	g.PATCH("/lessons/:lid/prep", h.updateLessonPrep)
	g.PATCH("/lessons/:lid/assignment", h.updateLessonAssignment)
	g.GET("/assignees", h.listAssignees)
	g.GET("/materials", h.listMaterials)
	g.POST("/materials", h.createMaterial)
	g.GET("/materials/:id", h.getMaterial)
	g.PUT("/materials/:id", h.updateMaterial)
	g.DELETE("/materials/:id", h.deleteMaterial)
	g.GET("/exercises", h.listExercises)
	g.POST("/exercises", h.createExercise)
	g.GET("/exercises/:id", h.getExercise)
	g.PUT("/exercises/:id", h.updateExercise)
	g.DELETE("/exercises/:id", h.deleteExercise)
}
