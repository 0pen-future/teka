package classes

import (
	"net/http"
	"strconv"
	"strings"

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
//	@Description	Schedules are required — a class without a timetable generates no sessions. A blank code is generated; a code another live class in the center uses is refused with 409 CLASS_CODE_TAKEN.
//	@Tags			classes
//	@Accept			json
//	@Produce		json
//	@Param			request	body		CreateClassRequest	true	"class fields with schedules"
//	@Success		201		{object}	response.Envelope{data=ClassResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"class code already taken"
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
//	@Description	status filters the list: active (default), archived, or all. The other filters combine with AND: q matches name or code, weekday/shift match a timetable row still in effect, tag matches one tag exactly, phase is derived from the dates.
//	@Tags			classes
//	@Produce		json
//	@Param			status		query		string	false	"active (default), archived, or all"
//	@Param			q			query		string	false	"substring of name or code, case-insensitive"
//	@Param			weekday		query		int		false	"0 (Sunday) to 6"
//	@Param			shift		query		string	false	"morning, afternoon, or evening"
//	@Param			tag			query		string	false	"exact tag"
//	@Param			phase		query		string	false	"upcoming, running, ended, or archived"
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
	filter, ok := parseListFilter(c)
	if !ok {
		return
	}
	params := pagination.Parse(c, "name", listSorts)
	rows, roles, total, err := h.svc.ListReadable(c.Request.Context(), sc, filter, params)
	if err != nil {
		response.Err(c, err)
		return
	}
	counts, err := h.svc.StudentCounts(c.Request.Context(), sc, ClassIDs(rows))
	if err != nil {
		response.Err(c, err)
		return
	}
	out := make([]ClassResponse, 0, len(rows))
	for i := range rows {
		resp := FromModelWithRoles(&rows[i], roles[rows[i].ID])
		resp.StudentCount = int(counts[rows[i].ID])
		out = append(out, resp)
	}
	response.List(c, out, params.Meta(total))
}

// parseListFilter reads the list query parameters, answering 422 with one
// message per offending field when any enum or range is off. A typo in a
// filter must never fall through to the unfiltered list.
func parseListFilter(c *gin.Context) (ListFilter, bool) {
	filter := ListFilter{Q: strings.TrimSpace(c.Query("q")), Tag: c.Query("tag")}
	fields := map[string]string{}
	switch status := c.DefaultQuery("status", StatusActive); status {
	case StatusActive, StatusArchived:
		filter.Status = status
	case "all":
		// Status stays empty: every non-deleted class.
	default:
		fields["status"] = "must be one of: active, archived, all"
	}
	if raw := c.Query("weekday"); raw != "" {
		n, err := strconv.ParseInt(raw, 10, 16)
		if err != nil || n < 0 || n > 6 {
			fields["weekday"] = "must be an integer from 0 (Sunday) to 6"
		} else {
			weekday := int16(n)
			filter.Weekday = &weekday
		}
	}
	switch shift := c.Query("shift"); shift {
	case "", ShiftMorning, ShiftAfternoon, ShiftEvening:
		filter.Shift = shift
	default:
		fields["shift"] = "must be one of: morning, afternoon, evening"
	}
	switch phase := c.Query("phase"); phase {
	case "", PhaseUpcoming, PhaseRunning, PhaseEnded, PhaseArchived:
		filter.Phase = phase
	default:
		fields["phase"] = "must be one of: upcoming, running, ended, archived"
	}
	if len(fields) > 0 {
		response.Err(c, apperror.Invalid("validation failed", fields))
		return ListFilter{}, false
	}
	return filter, true
}

// stats counts the caller's readable classes per phase.
//
//	@Summary		Class stats
//	@Description	Counts every class the caller can read, bucketed by phase on today, plus how many are recruiting.
//	@Tags			classes
//	@Produce		json
//	@Success		200	{object}	response.Envelope{data=ClassStatsResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/classes/stats [get]
func (h *Handler) stats(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	stats, err := h.svc.Stats(c.Request.Context(), sc, today())
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, stats)
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
	counts, err := h.svc.StudentCounts(c.Request.Context(), sc, []uuid.UUID{classID})
	if err != nil {
		response.Err(c, err)
		return
	}
	resp := FromModelWithRoles(class, roles)
	resp.StudentCount = int(counts[classID])
	response.OK(c, http.StatusOK, resp)
}

// update edits the class's own fields.
//
//	@Summary		Update class
//	@Description	Edits name, dates, and default price (full replace); code, tags, recruiting and note are optional and keep their stored value when absent. Schedules and status have their own endpoints.
//	@Tags			classes
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"class id"
//	@Param			request	body		UpdateClassRequest	true	"class fields"
//	@Success		200		{object}	response.Envelope{data=ClassResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"class code already taken"
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
