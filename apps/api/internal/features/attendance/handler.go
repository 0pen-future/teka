package attendance

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// Handler exposes the attendance endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds the attendance handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// scope resolves the authenticated tenant's center scope — the only
// sanctioned source of tenancy; request bodies and paths never carry it.
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

// get returns the attendance sheet for a session.
//
//	@Summary		Get session attendance
//	@Description	Returns one row per student enrolled as of the session date, present students included. status is null for a session never confirmed.
//	@Tags			attendance
//	@Produce		json
//	@Param			id	path		string	true	"session id"
//	@Success		200	{object}	response.Envelope{data=Response}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/sessions/{id}/attendance [get]
func (h *Handler) get(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	sessionID, ok := pathID(c, "id", "session")
	if !ok {
		return
	}
	out, err := h.svc.Get(c.Request.Context(), sc, sessionID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// confirm records attendance for a session in one call: the exceptions from
// the all-present default, everyone else present. Also transitions the
// session to held.
//
//	@Summary		Confirm session attendance
//	@Description	One-touch attendance (PRD R2): pass only the exceptions in marks (status late, absent, or excused, each with an optional per-student note) — an unlisted roster student is recorded present, so an empty body means everyone was on time. Writes one billable record per roster student, drops records for students no longer enrolled, and marks the session held. The legacy absent_student_ids body (absent ids only) is deprecated but still accepted; sending it together with marks is a 400. Refuses (409) a cancelled session; rejects (422) an unknown mark status or any student not on the roster active for the session's date.
//	@Tags			attendance
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"session id"
//	@Param			request	body		ConfirmRequest	true	"per-student status marks (or deprecated absent student ids) and optional session note"
//	@Success		200		{object}	response.Envelope{data=Response}
//	@Failure		400		{object}	response.Envelope{error=response.ErrorBody}	"marks and absent_student_ids sent together, or a duplicated student in marks"
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"session is cancelled"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"an unknown mark status, or a student not on the roster"
//	@Security		BearerAuth
//	@Router			/sessions/{id}/attendance [post]
func (h *Handler) confirm(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	sessionID, ok := pathID(c, "id", "session")
	if !ok {
		return
	}
	var req ConfirmRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.Confirm(c.Request.Context(), sc, sessionID, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}
