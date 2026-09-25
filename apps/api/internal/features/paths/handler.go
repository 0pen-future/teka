package paths

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// listSorts whitelists the public sort keys for GET /paths.
var listSorts = map[string]string{
	"name":       "learning_paths.name",
	"code":       "learning_paths.code",
	"status":     "learning_paths.status",
	"created_at": "learning_paths.created_at",
}

// Handler exposes the learning path endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds the paths handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// scope resolves the authenticated tenant's center scope — the only sanctioned
// source of tenancy; request bodies and paths never carry it.
func (h *Handler) scope(c *gin.Context) (authctx.Scope, bool) {
	sc, ok := authctx.ScopeFrom(c)
	if !ok {
		response.Err(c, apperror.Unauthorized("authentication required"))
		return authctx.Scope{}, false
	}
	return sc, true
}

// pathID parses the path id, reading a malformed value as not existing.
func pathID(c *gin.Context) (uuid.UUID, bool) {
	parsed, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Err(c, apperror.NotFound("learning path"))
		return uuid.UUID{}, false
	}
	return parsed, true
}

// stageID parses the stage id, reading a malformed value as not existing.
func stageID(c *gin.Context) (uuid.UUID, bool) {
	parsed, err := uuid.Parse(c.Param("sid"))
	if err != nil {
		response.Err(c, apperror.NotFound("path stage"))
		return uuid.UUID{}, false
	}
	return parsed, true
}

// list pages the center's learning paths.
//
//	@Summary		List learning paths
//	@Description	Live learning paths of the caller's center, each with its stages and recommended courses. q matches code or name.
//	@Tags			paths
//	@Produce		json
//	@Param			status		query		string	false	"draft | active | archived"
//	@Param			q			query		string	false	"code or name fragment"
//	@Param			page		query		int		false	"page (default 1)"
//	@Param			per_page	query		int		false	"page size (default 20, max 100)"
//	@Param			sort		query		string	false	"name | code | status | created_at, prefix - for descending"
//	@Success		200			{object}	response.Envelope{data=[]PathResponse,meta=response.Meta}
//	@Failure		401			{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403			{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422			{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/paths [get]
func (h *Handler) list(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	params := pagination.Parse(c, "name", listSorts)
	filter := ListFilter{Status: strings.TrimSpace(c.Query("status")), Q: strings.TrimSpace(c.Query("q"))}
	switch filter.Status {
	case "", StatusDraft, StatusActive, StatusArchived:
	default:
		response.Err(c, apperror.Invalid("Trạng thái lộ trình không hợp lệ",
			map[string]string{"status": "draft, active hoặc archived"}))
		return
	}
	rows, total, err := h.svc.List(c.Request.Context(), sc, filter, params)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.List(c, rows, params.Meta(total))
}

// create adds a learning path.
//
//	@Summary		Create a learning path
//	@Description	The code is upper-cased and must be unique among the center's live paths (409 CODE_TAKEN). Status defaults to draft.
//	@Tags			paths
//	@Accept			json
//	@Produce		json
//	@Param			body	body		PathRequest	true	"learning path"
//	@Success		201		{object}	response.Envelope{data=PathResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/paths [post]
func (h *Handler) create(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	var req PathRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.Create(c.Request.Context(), sc, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, out)
}

// get returns one learning path with its stages.
//
//	@Summary		Get a learning path
//	@Tags			paths
//	@Produce		json
//	@Param			id	path		string	true	"learning path id"
//	@Success		200	{object}	response.Envelope{data=PathResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/paths/{id} [get]
func (h *Handler) get(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	out, err := h.svc.Get(c.Request.Context(), sc, id)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// update replaces a learning path's own fields.
//
//	@Summary		Update a learning path
//	@Description	Full replace of the path's own fields; a blank status keeps the stored one. Stages have their own endpoints.
//	@Tags			paths
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string		true	"learning path id"
//	@Param			body	body		PathRequest	true	"learning path"
//	@Success		200		{object}	response.Envelope{data=PathResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/paths/{id} [put]
func (h *Handler) update(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req PathRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.Update(c.Request.Context(), sc, id, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// delete soft-deletes a learning path.
//
//	@Summary		Delete a learning path
//	@Description	Soft delete: the code becomes reusable and the stages go with the path. Courses are never touched.
//	@Tags			paths
//	@Produce		json
//	@Param			id	path		string	true	"learning path id"
//	@Success		200	{object}	response.Envelope{data=object}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/paths/{id} [delete]
func (h *Handler) delete(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), sc, id); err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, gin.H{"deleted": true})
}

// createStage appends a stage to the path.
//
//	@Summary		Add a stage
//	@Description	Appends a stage after the last one and returns the whole path.
//	@Tags			paths
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"learning path id"
//	@Param			body	body		StageRequest	true	"stage"
//	@Success		201		{object}	response.Envelope{data=PathResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/paths/{id}/stages [post]
func (h *Handler) createStage(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req StageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.CreateStage(c.Request.Context(), sc, id, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, out)
}

// reorderStages applies a new stage order.
//
//	@Summary		Reorder stages
//	@Description	stage_ids must list every stage of the path exactly once (422 otherwise); positions are assigned in that order.
//	@Tags			paths
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"learning path id"
//	@Param			body	body		ReorderRequest	true	"new order"
//	@Success		200		{object}	response.Envelope{data=PathResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/paths/{id}/stages/order [put]
func (h *Handler) reorderStages(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req ReorderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.ReorderStages(c.Request.Context(), sc, id, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// updateStage replaces a stage's name and goal.
//
//	@Summary		Update a stage
//	@Tags			paths
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"learning path id"
//	@Param			sid		path		string			true	"stage id"
//	@Param			body	body		StageRequest	true	"stage"
//	@Success		200		{object}	response.Envelope{data=PathResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/paths/{id}/stages/{sid} [put]
func (h *Handler) updateStage(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	sid, ok := stageID(c)
	if !ok {
		return
	}
	var req StageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.UpdateStage(c.Request.Context(), sc, id, sid, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// deleteStage removes a stage and renumbers the rest.
//
//	@Summary		Delete a stage
//	@Description	Removes the stage with its course links; the remaining stages keep positions 1..n.
//	@Tags			paths
//	@Produce		json
//	@Param			id	path		string	true	"learning path id"
//	@Param			sid	path		string	true	"stage id"
//	@Success		200	{object}	response.Envelope{data=PathResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/paths/{id}/stages/{sid} [delete]
func (h *Handler) deleteStage(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	sid, ok := stageID(c)
	if !ok {
		return
	}
	out, err := h.svc.DeleteStage(c.Request.Context(), sc, id, sid)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// setStageCourses replaces the courses a stage recommends.
//
//	@Summary		Replace a stage's courses
//	@Description	Wholesale replace in body order; [] clears the list. Every id must be a live course of the center (422 otherwise); at most 20 per stage.
//	@Tags			paths
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"learning path id"
//	@Param			sid		path		string				true	"stage id"
//	@Param			body	body		StageCoursesRequest	true	"course ids; [] clears"
//	@Success		200		{object}	response.Envelope{data=PathResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/paths/{id}/stages/{sid}/courses [put]
func (h *Handler) setStageCourses(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	sid, ok := stageID(c)
	if !ok {
		return
	}
	var req StageCoursesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.SetStageCourses(c.Request.Context(), sc, id, sid, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}
