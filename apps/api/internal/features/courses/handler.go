package courses

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// listSorts whitelists the public sort keys for GET /courses.
var listSorts = map[string]string{
	"name":       "courses.name",
	"code":       "courses.code",
	"status":     "courses.status",
	"created_at": "courses.created_at",
}

// Handler exposes the course catalog endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds the courses handler.
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

// pathID parses the course id, reading a malformed value as not existing.
func pathID(c *gin.Context) (uuid.UUID, bool) {
	parsed, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Err(c, apperror.NotFound("course"))
		return uuid.UUID{}, false
	}
	return parsed, true
}

// list pages the center's courses.
//
//	@Summary		List courses
//	@Description	Live courses of the caller's center with running/upcoming class counters and tuition packs. q matches code or name.
//	@Tags			courses
//	@Produce		json
//	@Param			status		query		string	false	"draft | active | archived"
//	@Param			q			query		string	false	"code or name fragment"
//	@Param			page		query		int		false	"page (default 1)"
//	@Param			per_page	query		int		false	"page size (default 20, max 100)"
//	@Param			sort		query		string	false	"name | code | status | created_at, prefix - for descending"
//	@Success		200			{object}	response.Envelope{data=[]CourseResponse,meta=response.Meta}
//	@Failure		401			{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403			{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422			{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/courses [get]
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
		response.Err(c, apperror.Invalid("Trạng thái khóa học không hợp lệ",
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

// create adds a course.
//
//	@Summary		Create a course
//	@Description	The code is upper-cased and must be unique among the center's live courses (409 CODE_TAKEN). default_template_version_id must be a published version of the center's library (422).
//	@Tags			courses
//	@Accept			json
//	@Produce		json
//	@Param			body	body		CourseRequest	true	"course"
//	@Success		201		{object}	response.Envelope{data=CourseResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/courses [post]
func (h *Handler) create(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	var req CourseRequest
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

// get returns one course.
//
//	@Summary		Get a course
//	@Tags			courses
//	@Produce		json
//	@Param			id	path		string	true	"course id"
//	@Success		200	{object}	response.Envelope{data=CourseResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/courses/{id} [get]
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

// update replaces a course's own fields.
//
//	@Summary		Update a course
//	@Description	Full replace of the course's own fields; tuition packs have their own endpoint.
//	@Tags			courses
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"course id"
//	@Param			body	body		CourseRequest	true	"course"
//	@Success		200		{object}	response.Envelope{data=CourseResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/courses/{id} [put]
func (h *Handler) update(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	var req CourseRequest
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

// delete soft-deletes a course.
//
//	@Summary		Delete a course
//	@Description	Soft delete: the code becomes reusable. Refused with 409 COURSE_IN_USE while live classes point at the course — archive it instead.
//	@Tags			courses
//	@Produce		json
//	@Param			id	path		string	true	"course id"
//	@Success		200	{object}	response.Envelope{data=object}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/courses/{id} [delete]
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

// archive retires a course from the catalog.
//
//	@Summary		Archive a course
//	@Description	Moves the course to archived; classes attached to it keep running. 409 COURSE_ARCHIVED when it already is.
//	@Tags			courses
//	@Produce		json
//	@Param			id	path		string	true	"course id"
//	@Success		200	{object}	response.Envelope{data=CourseResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/courses/{id}/archive [post]
func (h *Handler) archive(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	out, err := h.svc.Archive(c.Request.Context(), sc, id)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// setTuitionPacks replaces a course's tuition packs.
//
//	@Summary		Replace a course's tuition packs
//	@Description	Wholesale replace, positions assigned in body order; [] clears the list. At most 20 packs.
//	@Tags			courses
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"course id"
//	@Param			body	body		[]TuitionPackInput	true	"tuition packs; [] clears"
//	@Success		200		{object}	response.Envelope{data=[]TuitionPackResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/courses/{id}/tuition-packs [put]
func (h *Handler) setTuitionPacks(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c)
	if !ok {
		return
	}
	var items []TuitionPackInput
	if err := c.ShouldBindJSON(&items); err != nil {
		var elementErrs binding.SliceValidationError
		if appErr := validation.Elements(items); errors.As(err, &elementErrs) && appErr != nil {
			response.Err(c, appErr)
		} else {
			response.Err(c, validation.BindError(err))
		}
		return
	}
	out, err := h.svc.SetTuitionPacks(c.Request.Context(), sc, id, items)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}
