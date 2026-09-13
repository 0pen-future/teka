package tasks

import "github.com/gin-gonic/gin"

// RegisterRoutes mounts the task board under /tasks and its columns under
// /task-columns, both behind authentication plus per-request scope
// resolution. Two prefixes share one group ("" base) rather than one nested
// under the other, since a column is not a sub-resource path of a task.
func RegisterRoutes(rg *gin.RouterGroup, h *Handler, auth ...gin.HandlerFunc) {
	g := rg.Group("", auth...)

	g.GET("/tasks/board", h.board)
	g.POST("/tasks", h.createTask)
	g.GET("/tasks/:id", h.getTask)
	g.PATCH("/tasks/:id", h.updateTask)
	g.POST("/tasks/:id/move", h.moveTask)
	g.DELETE("/tasks/:id", h.deleteTask)

	g.POST("/task-columns", h.createColumn)
	g.PATCH("/task-columns/:id", h.updateColumn)
	g.PUT("/task-columns/order", h.reorderColumns)
	g.DELETE("/task-columns/:id", h.deleteColumn)
}
