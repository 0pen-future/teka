package enrollments

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the enrollment endpoints under /enrollments — plus
// the enrollment picker, which lives under /classes because it answers "who
// can I enroll into this class" — all behind authentication and scope
// resolution.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, auth ...gin.HandlerFunc) {
	g := rg.Group("/enrollments", auth...)
	g.POST("", h.create)
	g.GET("", h.list)
	g.GET("/:id", h.get)
	g.POST("/:id/end", h.end)
	g.DELETE("/:id", h.remove)

	classes := rg.Group("/classes", auth...)
	classes.GET("/:id/enrollable-students", h.enrollableStudents)
}
