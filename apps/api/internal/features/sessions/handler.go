package sessions

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// Handler exposes the session endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds the sessions handler.
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

// queryDate parses a required YYYY-MM-DD query parameter.
func queryDate(c *gin.Context, name string) (time.Time, bool) {
	raw := c.Query(name)
	if raw == "" {
		response.Err(c, apperror.Invalid("validation failed", map[string]string{name: "is required"}))
		return time.Time{}, false
	}
	t, err := time.Parse(dateLayout, raw)
	if err != nil {
		response.Err(c, apperror.Invalid("validation failed",
			map[string]string{name: "must be a date in YYYY-MM-DD form"}))
		return time.Time{}, false
	}
	return t, true
}

// optionalQueryDate parses an optional YYYY-MM-DD query parameter; an unset
// value returns (nil, true), a malformed one reports a 422 and (nil, false).
func optionalQueryDate(c *gin.Context, name string) (*time.Time, bool) {
	raw := c.Query(name)
	if raw == "" {
		return nil, true
	}
	t, err := time.Parse(dateLayout, raw)
	if err != nil {
		response.Err(c, apperror.Invalid("validation failed",
			map[string]string{name: "must be a date in YYYY-MM-DD form"}))
		return nil, false
	}
	return &t, true
}

// queryLimit parses an optional "limit" query parameter; unset or malformed
// values return 0, letting Service.ListPending apply its own default.
func queryLimit(c *gin.Context) int {
	v, err := strconv.Atoi(c.Query("limit"))
	if err != nil {
		return 0
	}
	return v
}

// pending returns the teacher's unconfirmed past sessions.
//
//	@Summary		List pending attendance
//	@Description	Sessions in the past (evaluated in the teacher's timezone) that are still unconfirmed and planned or held — cancelled sessions never appear. Ordered newest first. from/to optionally bound the range (both inclusive), the same predicate period-closing uses. limit defaults to 50 and is capped at 200; total reflects the unlimited count.
//	@Tags			sessions
//	@Produce		json
//	@Param			from	query		string	false	"range start, YYYY-MM-DD"
//	@Param			to		query		string	false	"range end, YYYY-MM-DD"
//	@Param			limit	query		int		false	"max rows to return, default 50, capped at 200"
//	@Success		200		{object}	response.Envelope{data=PendingResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/sessions/pending [get]
func (h *Handler) pending(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	from, ok := optionalQueryDate(c, "from")
	if !ok {
		return
	}
	to, ok := optionalQueryDate(c, "to")
	if !ok {
		return
	}
	limit := queryLimit(c)

	out, err := h.svc.ListPending(c.Request.Context(), sc, from, to, limit)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// listRange returns a class's sessions in [from, to], generating any missing
// rows first.
//
//	@Summary		List (and generate) class sessions
//	@Description	Generates any session rows missing for [from, to] from the class's effective schedules, then returns every session in the range — including cancelled ones. Idempotent: calling it again with an overlapping range never duplicates a row. Range is capped at 400 days.
//	@Tags			sessions
//	@Produce		json
//	@Param			id		path		string	true	"class id"
//	@Param			from	query		string	true	"range start, YYYY-MM-DD"
//	@Param			to		query		string	true	"range end, YYYY-MM-DD"
//	@Success		200		{object}	response.Envelope{data=[]SessionResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"range too large or to before from"
//	@Security		BearerAuth
//	@Router			/classes/{id}/sessions [get]
func (h *Handler) listRange(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	from, ok := queryDate(c, "from")
	if !ok {
		return
	}
	to, ok := queryDate(c, "to")
	if !ok {
		return
	}

	rows, err := h.svc.ListRange(c.Request.Context(), sc, classID, from, to)
	if err != nil {
		response.Err(c, err)
		return
	}
	out := make([]SessionResponse, 0, len(rows))
	for i := range rows {
		out = append(out, FromDetail(&rows[i]))
	}
	response.OK(c, http.StatusOK, out)
}

// createAdHoc adds a single session outside any schedule.
//
//	@Summary		Add an ad-hoc session
//	@Description	Adds a make-up session on a date not covered by any schedule. A date already occupied by another session for the class returns 409.
//	@Tags			sessions
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string					true	"class id"
//	@Param			request	body		CreateSessionRequest	true	"session date and optional start time"
//	@Success		201		{object}	response.Envelope{data=SessionResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"a session already exists on this date"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/classes/{id}/sessions [post]
func (h *Handler) createAdHoc(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	var req CreateSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	row, err := h.svc.CreateAdHoc(c.Request.Context(), sc, classID, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, FromDetail(row))
}

// get returns one session.
//
//	@Summary		Get session
//	@Tags			sessions
//	@Produce		json
//	@Param			id	path		string	true	"session id"
//	@Success		200	{object}	response.Envelope{data=SessionResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/sessions/{id} [get]
func (h *Handler) get(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	sessionID, ok := pathID(c, "id", "session")
	if !ok {
		return
	}
	row, err := h.svc.Get(c.Request.Context(), sc, sessionID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, FromDetail(row))
}

// remove soft-deletes a session.
//
//	@Summary		Delete session
//	@Description	Refuses (409) a session whose attendance is already confirmed.
//	@Tags			sessions
//	@Param			id	path	string	true	"session id"
//	@Success		204
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}	"attendance already confirmed"
//	@Security		BearerAuth
//	@Router			/sessions/{id} [delete]
func (h *Handler) remove(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	sessionID, ok := pathID(c, "id", "session")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), sc, sessionID); err != nil {
		response.Err(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// cancel marks a session cancelled.
//
//	@Summary		Cancel session
//	@Description	reason is required and becomes the line parents see on their statement. Refuses (409) a session whose attendance is already confirmed.
//	@Tags			sessions
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"session id"
//	@Param			request	body		CancelRequest	true	"cancellation reason"
//	@Success		200		{object}	response.Envelope{data=SessionResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"attendance already confirmed"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"reason is required"
//	@Security		BearerAuth
//	@Router			/sessions/{id}/cancel [post]
func (h *Handler) cancel(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	sessionID, ok := pathID(c, "id", "session")
	if !ok {
		return
	}
	var req CancelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	row, err := h.svc.Cancel(c.Request.Context(), sc, sessionID, req.Reason)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, FromDetail(row))
}

// uncancel returns a cancelled session to planned.
//
//	@Summary		Uncancel session
//	@Tags			sessions
//	@Produce		json
//	@Param			id	path		string	true	"session id"
//	@Success		200	{object}	response.Envelope{data=SessionResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/sessions/{id}/uncancel [post]
func (h *Handler) uncancel(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	sessionID, ok := pathID(c, "id", "session")
	if !ok {
		return
	}
	row, err := h.svc.Uncancel(c.Request.Context(), sc, sessionID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, FromDetail(row))
}

// hold marks a session held.
//
//	@Summary		Hold session
//	@Description	Explicitly marks a session held, ahead of attendance confirmation (phase 2) implicitly doing the same.
//	@Tags			sessions
//	@Produce		json
//	@Param			id	path		string	true	"session id"
//	@Success		200	{object}	response.Envelope{data=SessionResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/sessions/{id}/hold [post]
func (h *Handler) hold(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	sessionID, ok := pathID(c, "id", "session")
	if !ok {
		return
	}
	row, err := h.svc.Hold(c.Request.Context(), sc, sessionID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, FromDetail(row))
}
