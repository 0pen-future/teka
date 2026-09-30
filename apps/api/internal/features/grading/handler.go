package grading

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// Handler exposes the grading endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds the grading handler.
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

// getClassComponents returns a class's snapshot components.
//
//	@Summary		Get a class's score components
//	@Description	The class's snapshot components (score grid columns), position order. An empty list means the class uses the plain general-score UI. Readable by the class's teacher and any center-wide reader.
//	@Tags			grading
//	@Produce		json
//	@Param			id	path		string	true	"class id"
//	@Success		200	{object}	response.Envelope{data=ClassComponentsResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/classes/{id}/score-components [get]
func (h *Handler) getClassComponents(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	out, err := h.svc.GetClassComponents(c.Request.Context(), sc, classID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// getSessionScores returns a session's component columns and recorded cells.
//
//	@Summary		Get session component scores
//	@Description	The class's component columns plus every recorded cell for the session — one round-trip the score grid rebuilds from. Read gate is session resolution (the session's teacher and any center-wide reader).
//	@Tags			grading
//	@Produce		json
//	@Param			id	path		string	true	"session id"
//	@Success		200	{object}	response.Envelope{data=SessionScoresResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/sessions/{id}/scores [get]
func (h *Handler) getSessionScores(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	sessionID, ok := pathID(c, "id", "session")
	if !ok {
		return
	}
	out, err := h.svc.GetSessionScores(c.Request.Context(), sc, sessionID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// putSessionScores batch-upserts a session's component scores.
//
//	@Summary		Save session component scores
//	@Description	Batch merge of per-cell scores. Per entry a value upserts the cell and null deletes it; the table never holds empty cells. A new cell requires the student to have been on the session's roster; an already-scored student stays editable after their enrollment ends. Returns the session's full score set after the write. Writable by the session's teacher OR the center owner (deliberate divergence from marks, which are teacher-only).
//	@Tags			grading
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"session id"
//	@Param			request	body		[]ScoreEntryRequest	true	"score entries"
//	@Success		200		{object}	response.Envelope{data=[]ScoreResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"caller is neither the session's teacher nor the owner"
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"duplicate cell, score outside 0–10, component not in the class, or student not on the roster"
//	@Security		BearerAuth
//	@Router			/sessions/{id}/scores [put]
func (h *Handler) putSessionScores(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	sessionID, ok := pathID(c, "id", "session")
	if !ok {
		return
	}
	var req []ScoreEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.PutSessionScores(c.Request.Context(), sc, sessionID, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}
