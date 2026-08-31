package classes

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// listSorts whitelists the public sort keys for GET /classes.
var listSorts = map[string]string{
	"name":       "classes.name",
	"start_date": "classes.start_date",
	"created_at": "classes.created_at",
}

// Handler exposes the class and schedule endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds the classes handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// scope resolves the authenticated caller's center scope — the only
// sanctioned source of tenant identity; request bodies and paths never carry
// it.
func (h *Handler) scope(c *gin.Context) (authctx.Scope, bool) {
	sc, ok := authctx.ScopeFrom(c)
	if !ok {
		response.Err(c, apperror.Unauthorized("authentication required"))
		return authctx.Scope{}, false
	}
	return sc, true
}

// pathID parses one uuid path parameter, reading a malformed value as the
// named resource not existing.
func pathID(c *gin.Context, param, resource string) (uuid.UUID, bool) {
	parsed, err := uuid.Parse(c.Param(param))
	if err != nil {
		response.Err(c, apperror.NotFound(resource))
		return uuid.UUID{}, false
	}
	return parsed, true
}

// create registers a class with its weekly schedules in one transaction.
//
//	@Summary		Create class
//	@Description	Schedules are required — a class without a timetable generates no sessions.
//	@Tags			classes
//	@Accept			json
//	@Produce		json
//	@Param			request	body		CreateClassRequest	true	"class fields with schedules"
//	@Success		201		{object}	response.Envelope{data=ClassResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"validation failed"
//	@Security		BearerAuth
//	@Router			/classes [post]
func (h *Handler) create(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	var req CreateClassRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	class, err := h.svc.Create(c.Request.Context(), sc, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, FromModel(class))
}

// list returns a page of classes, active-only by default.
//
//	@Summary		List classes
//	@Description	status filters the list: active (default), archived, or all.
//	@Tags			classes
//	@Produce		json
//	@Param			status		query		string	false	"active (default), archived, or all"
//	@Param			page		query		int		false	"page number"
//	@Param			per_page	query		int		false	"page size (max 100)"
//	@Param			sort		query		string	false	"name, start_date, or created_at; - prefix for desc"
//	@Success		200			{object}	response.Envelope{data=[]ClassResponse,meta=response.Meta}
//	@Failure		401			{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422			{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/classes [get]
func (h *Handler) list(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	filter := ListFilter{}
	switch status := c.DefaultQuery("status", StatusActive); status {
	case StatusActive, StatusArchived:
		filter.Status = status
	case "all":
		// Status stays empty: every non-deleted class.
	default:
		response.Err(c, apperror.Invalid("validation failed",
			map[string]string{"status": "must be one of: active, archived, all"}))
		return
	}
	params := pagination.Parse(c, "name", listSorts)
	rows, roles, total, err := h.svc.ListReadable(c.Request.Context(), sc, filter, params)
	if err != nil {
		response.Err(c, err)
		return
	}
	out := make([]ClassResponse, 0, len(rows))
	for i := range rows {
		out = append(out, FromModelWithRoles(&rows[i], roles[rows[i].ID]))
	}
	response.List(c, out, params.Meta(total))
}

// get returns one class with its schedules; archived classes remain
// retrievable.
//
//	@Summary		Get class
//	@Tags			classes
//	@Produce		json
//	@Param			id	path		string	true	"class id"
//	@Success		200	{object}	response.Envelope{data=ClassResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/classes/{id} [get]
func (h *Handler) get(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	class, roles, err := h.svc.GetReadableWithRoles(c.Request.Context(), sc, classID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, FromModelWithRoles(class, roles))
}

// update edits the class's own fields.
//
//	@Summary		Update class
//	@Description	Edits name, dates, and default price; schedules and status have their own endpoints.
//	@Tags			classes
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"class id"
//	@Param			request	body		UpdateClassRequest	true	"class fields"
//	@Success		200		{object}	response.Envelope{data=ClassResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/classes/{id} [put]
func (h *Handler) update(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	var req UpdateClassRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	class, err := h.svc.Update(c.Request.Context(), sc, classID, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, FromModel(class))
}

// archive marks the class archived — the normal end-of-term action.
//
//	@Summary		Archive class
//	@Description	Keeps the class in history and reports; use delete only for classes created by mistake.
//	@Tags			classes
//	@Produce		json
//	@Param			id	path		string	true	"class id"
//	@Success		200	{object}	response.Envelope{data=ClassResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/classes/{id}/archive [post]
func (h *Handler) archive(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	class, err := h.svc.Archive(c.Request.Context(), sc, classID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, FromModel(class))
}

// remove soft-deletes a class with no open enrollments.
//
//	@Summary		Delete class
//	@Description	Soft delete for mistakes; blocked with 409 while open enrollments exist — archive instead.
//	@Tags			classes
//	@Produce		json
//	@Param			id	path	string	true	"class id"
//	@Success		204
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}	"open enrollments exist"
//	@Security		BearerAuth
//	@Router			/classes/{id} [delete]
func (h *Handler) remove(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), sc, classID); err != nil {
		response.Err(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// addSchedule appends a timetable row — the second half of close-and-replace.
//
//	@Summary		Add schedule row
//	@Description	Adds a weekly slot; effective_from defaults to the class start date.
//	@Tags			classes
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"class id"
//	@Param			request	body		ScheduleRequest	true	"schedule fields"
//	@Success		201		{object}	response.Envelope{data=ScheduleResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/classes/{id}/schedules [post]
func (h *Handler) addSchedule(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	var req ScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	schedule, err := h.svc.AddSchedule(c.Request.Context(), sc, classID, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, FromSchedule(schedule))
}

// updateSchedule edits one timetable row in place.
//
//	@Summary		Update schedule row
//	@Description	For correcting a mistyped row or closing it via effective_to; real timetable changes should close the old row and add a new one.
//	@Tags			classes
//	@Accept			json
//	@Produce		json
//	@Param			id			path		string					true	"class id"
//	@Param			scheduleID	path		string					true	"schedule id"
//	@Param			request		body		UpdateScheduleRequest	true	"schedule fields"
//	@Success		200			{object}	response.Envelope{data=ScheduleResponse}
//	@Failure		401			{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404			{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422			{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/classes/{id}/schedules/{scheduleID} [put]
func (h *Handler) updateSchedule(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	scheduleID, ok := pathID(c, "scheduleID", "schedule")
	if !ok {
		return
	}
	var req UpdateScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	schedule, err := h.svc.UpdateSchedule(c.Request.Context(), sc, classID, scheduleID, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, FromSchedule(schedule))
}

// removeSchedule soft-deletes a timetable row.
//
//	@Summary		Delete schedule row
//	@Tags			classes
//	@Produce		json
//	@Param			id			path	string	true	"class id"
//	@Param			scheduleID	path	string	true	"schedule id"
//	@Success		204
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/classes/{id}/schedules/{scheduleID} [delete]
func (h *Handler) removeSchedule(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	scheduleID, ok := pathID(c, "scheduleID", "schedule")
	if !ok {
		return
	}
	if err := h.svc.DeleteSchedule(c.Request.Context(), sc, classID, scheduleID); err != nil {
		response.Err(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
