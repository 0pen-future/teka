package tasks

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// Handler exposes the task board's HTTP endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds the tasks handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// scope resolves the caller's center scope set by the ResolveScope
// middleware — the only sanctioned source of center identity.
func (h *Handler) scope(c *gin.Context) (authctx.Scope, bool) {
	s, ok := authctx.ScopeFrom(c)
	if !ok {
		response.Err(c, apperror.Unauthorized("authentication required"))
		return authctx.Scope{}, false
	}
	return s, true
}

// pathUUID parses a path parameter, or a 404 for the resource it names.
func (h *Handler) pathUUID(c *gin.Context, name, resource string) (uuid.UUID, bool) {
	v, err := uuid.Parse(c.Param(name))
	if err != nil {
		response.Err(c, apperror.NotFound(resource))
		return uuid.Nil, false
	}
	return v, true
}

// board returns the caller's kanban board.
//
//	@Summary		Get the task board
//	@Description	Every column, and the tasks Policy makes visible to the caller (their own vs. the whole center's, reported as scope).
//	@Tags			tasks
//	@Produce		json
//	@Success		200	{object}	response.Envelope{data=BoardResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/tasks/board [get]
func (h *Handler) board(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	resp, err := h.svc.Board(c.Request.Context(), scope)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, resp)
}

// createColumn adds a column to the board.
//
//	@Summary		Create a column
//	@Description	Requires tasks.manage_board (owner always has it).
//	@Tags			tasks
//	@Accept			json
//	@Produce		json
//	@Param			request	body		CreateColumnRequest	true	"column"
//	@Success		201		{object}	response.Envelope{data=ColumnResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"missing tasks.manage_board"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"validation failed, duplicate name, or column limit reached"
//	@Security		BearerAuth
//	@Router			/task-columns [post]
func (h *Handler) createColumn(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var req CreateColumnRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	resp, err := h.svc.CreateColumn(c.Request.Context(), scope, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, resp)
}

// updateColumn renames a column and/or toggles its IsDone flag.
//
//	@Summary		Update a column
//	@Description	Requires tasks.manage_board. A nil field is left unchanged.
//	@Tags			tasks
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"column id"	format(uuid)
//	@Param			request	body		UpdateColumnRequest	true	"patch"
//	@Success		200		{object}	response.Envelope{data=ColumnResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"missing tasks.manage_board"
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}	"column not found"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"validation failed or duplicate name"
//	@Security		BearerAuth
//	@Router			/task-columns/{id} [patch]
func (h *Handler) updateColumn(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	colID, ok := h.pathUUID(c, "id", "column")
	if !ok {
		return
	}
	var req UpdateColumnRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	resp, err := h.svc.UpdateColumn(c.Request.Context(), scope, colID, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, resp)
}

// reorderColumns persists a full reordering of the board's columns.
//
//	@Summary		Reorder columns
//	@Description	Requires tasks.manage_board. ids must be a permutation of the board's current column ids.
//	@Tags			tasks
//	@Accept			json
//	@Produce		json
//	@Param			request	body		ReorderColumnsRequest	true	"full column id order"
//	@Success		200		{object}	response.Envelope{data=ColumnsResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"missing tasks.manage_board"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"not a permutation of the current columns"
//	@Security		BearerAuth
//	@Router			/task-columns/order [put]
func (h *Handler) reorderColumns(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var req ReorderColumnsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	resp, err := h.svc.ReorderColumns(c.Request.Context(), scope, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, resp)
}

// deleteColumn removes a column. If it holds tasks, move_to must name another
// column to receive them.
//
//	@Summary		Delete a column
//	@Description	Requires tasks.manage_board. Cannot delete the board's last column. A non-empty column requires move_to.
//	@Tags			tasks
//	@Produce		json
//	@Param			id		path		string	true	"column id"	format(uuid)
//	@Param			move_to	query		string	false	"destination column id for the deleted column's tasks"	format(uuid)
//	@Success		200		{object}	response.Envelope{data=DeleteColumnResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"missing tasks.manage_board"
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}	"column not found"
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"cannot delete the last column, or column still has tasks; specify move_to"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"move_to names the column being deleted"
//	@Security		BearerAuth
//	@Router			/task-columns/{id} [delete]
func (h *Handler) deleteColumn(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	colID, ok := h.pathUUID(c, "id", "column")
	if !ok {
		return
	}
	var moveTo *uuid.UUID
	if raw := c.Query("move_to"); raw != "" {
		v, err := uuid.Parse(raw)
		if err != nil {
			response.Err(c, apperror.Invalid("validation failed", map[string]string{"move_to": "must be a uuid"}))
			return
		}
		moveTo = &v
	}
	resp, err := h.svc.DeleteColumn(c.Request.Context(), scope, colID, moveTo)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, resp)
}

// createTask adds a task to the board.
//
//	@Summary		Create a task
//	@Description	An absent column_id defaults to the board's first column. description is an HTML subset (p, br, strong, em, u, s, ul, ol, li, a[href] with http/https/mailto); the server sanitizes it, treats a value not starting with "<" as plain text, and rejects more than 4000 text characters.
//	@Tags			tasks
//	@Accept			json
//	@Produce		json
//	@Param			request	body		CreateTaskRequest	true	"task"
//	@Success		201		{object}	response.Envelope{data=TaskResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}	"column not found"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"validation failed, or assignee is not a member of this center"
//	@Security		BearerAuth
//	@Router			/tasks [post]
func (h *Handler) createTask(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	resp, err := h.svc.CreateTask(c.Request.Context(), scope, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, resp)
}

// getTask returns one task the caller may read.
//
//	@Summary		Get a task
//	@Description	Requires being the center owner, holding tasks.view_all, or being the task's creator or assignee.
//	@Tags			tasks
//	@Produce		json
//	@Param			id	path		string	true	"task id"	format(uuid)
//	@Success		200	{object}	response.Envelope{data=TaskResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}	"not visible to the caller"
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}	"task not found"
//	@Security		BearerAuth
//	@Router			/tasks/{id} [get]
func (h *Handler) getTask(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	taskID, ok := h.pathUUID(c, "id", "task")
	if !ok {
		return
	}
	resp, err := h.svc.GetTask(c.Request.Context(), scope, taskID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, resp)
}

// updateTask patches a task's editable content.
//
//	@Summary		Update a task
//	@Description	Requires being the center owner or the task's creator. assignee_id and due_on are tri-state: absent leaves the field unchanged, null clears it, a value sets it. description is an HTML subset (p, br, strong, em, u, s, ul, ol, li, a[href] with http/https/mailto); the server sanitizes it, treats a value not starting with "<" as plain text, and rejects more than 4000 text characters.
//	@Tags			tasks
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"task id"	format(uuid)
//	@Param			request	body		UpdateTaskRequest	true	"patch"
//	@Success		200		{object}	response.Envelope{data=TaskResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"not the owner or the task's creator"
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}	"task not found"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"validation failed, or assignee is not a member of this center"
//	@Security		BearerAuth
//	@Router			/tasks/{id} [patch]
func (h *Handler) updateTask(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	taskID, ok := h.pathUUID(c, "id", "task")
	if !ok {
		return
	}
	var req UpdateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	resp, err := h.svc.UpdateTask(c.Request.Context(), scope, taskID, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, resp)
}

// moveTask changes a task's column.
//
//	@Summary		Move a task to another column
//	@Description	Requires being the center owner, the task's creator, or its assignee. With after_task_id the task lands directly below that task, which must be a live task of the destination column; without it (or with null) the task lands at the top of the column.
//	@Tags			tasks
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"task id"	format(uuid)
//	@Param			request	body		MoveTaskRequest	true	"destination column"
//	@Success		200		{object}	response.Envelope{data=TaskResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"not the owner, creator, or assignee"
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}	"task or destination column not found"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"after_task_id is not a live task of the destination column"
//	@Security		BearerAuth
//	@Router			/tasks/{id}/move [post]
func (h *Handler) moveTask(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	taskID, ok := h.pathUUID(c, "id", "task")
	if !ok {
		return
	}
	var req MoveTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	resp, err := h.svc.MoveTask(c.Request.Context(), scope, taskID, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, resp)
}

// deleteTask soft-deletes a task.
//
//	@Summary		Delete a task
//	@Description	Requires being the center owner or the task's creator. Soft delete.
//	@Tags			tasks
//	@Param			id	path	string	true	"task id"	format(uuid)
//	@Success		204
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}	"not the owner or the task's creator"
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}	"task not found"
//	@Security		BearerAuth
//	@Router			/tasks/{id} [delete]
func (h *Handler) deleteTask(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	taskID, ok := h.pathUUID(c, "id", "task")
	if !ok {
		return
	}
	if err := h.svc.DeleteTask(c.Request.Context(), scope, taskID); err != nil {
		response.Err(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
