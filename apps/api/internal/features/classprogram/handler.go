package classprogram

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// Handler exposes the class program endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds a Handler over svc.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

func (h *Handler) scope(c *gin.Context) (authctx.Scope, bool) {
	sc, ok := authctx.ScopeFrom(c)
	if !ok {
		response.Err(c, apperror.Unauthorized("authentication required"))
		return authctx.Scope{}, false
	}
	return sc, true
}

func classID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Err(c, apperror.NotFound("class"))
		return uuid.Nil, false
	}
	return id, true
}

// get returns the class's applied program.
//
//	@Summary		Get the class's applied program
//	@Description	The published template version the class applies, or null when none is applied. Readable by the owner and any teacher with a stint on the class.
//	@Tags			classes
//	@Produce		json
//	@Param			id	path		string	true	"class id"
//	@Success		200	{object}	response.Envelope{data=ProgramResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/classes/{id}/program [get]
func (h *Handler) get(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := classID(c)
	if !ok {
		return
	}
	out, err := h.svc.Get(c.Request.Context(), sc, id)
	if err != nil {
		response.Err(c, err)
		return
	}
	if out == nil {
		response.OK(c, http.StatusOK, nil)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// lessons returns the applied version's lessons with materials and exercises.
//
//	@Summary		List the applied program's lessons
//	@Description	Lessons of the applied template version with their materials and exercises, in position order; an empty list when the class applies no program.
//	@Tags			classes
//	@Produce		json
//	@Param			id	path		string	true	"class id"
//	@Success		200	{object}	response.Envelope{data=[]library.LessonDetailResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/classes/{id}/program/lessons [get]
func (h *Handler) lessons(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := classID(c)
	if !ok {
		return
	}
	out, err := h.svc.Lessons(c.Request.Context(), sc, id)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// apply links a published template version to the class.
//
//	@Summary		Apply a program template version to the class
//	@Description	Owner only. Copies the version's lesson titles into the class curriculum. When the class already keeps a different lesson list the call returns 409 CURRICULUM_DIFFERS (fields.current_count / fields.template_count) until re-sent with confirm=true; the curriculum pointer and every lesson plan are kept.
//	@Tags			classes
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"class id"
//	@Param			body	body		ApplyRequest	true	"version to apply"
//	@Success		200		{object}	response.Envelope{data=ProgramResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/classes/{id}/program [put]
func (h *Handler) apply(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := classID(c)
	if !ok {
		return
	}
	var req ApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.Apply(c.Request.Context(), sc, id, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// remove drops the class's program link.
//
//	@Summary		Remove the class's program
//	@Description	Owner only. Drops the link to the template version; the class curriculum and lesson plans stay as they are.
//	@Tags			classes
//	@Produce		json
//	@Param			id	path		string	true	"class id"
//	@Success		200	{object}	response.Envelope{data=object}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/classes/{id}/program [delete]
func (h *Handler) remove(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := classID(c)
	if !ok {
		return
	}
	if err := h.svc.Remove(c.Request.Context(), sc, id); err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, gin.H{"deleted": true})
}
