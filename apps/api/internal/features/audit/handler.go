package audit

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/response"
)

// Handler exposes the owner-only audit trail read endpoint.
type Handler struct {
	svc *Service
}

// NewHandler builds the audit handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// scope resolves the authenticated caller's center scope — the only
// sanctioned source of tenant identity; query params never carry it.
func (h *Handler) scope(c *gin.Context) (authctx.Scope, bool) {
	sc, ok := authctx.ScopeFrom(c)
	if !ok {
		response.Err(c, apperror.Unauthorized("authentication required"))
		return authctx.Scope{}, false
	}
	return sc, true
}

// parseListQuery validates the raw query params into a ListQuery. Filters are
// optional; a present-but-malformed value is a client error, not "no filter".
func parseListQuery(c *gin.Context) (ListQuery, error) {
	var q ListQuery
	if raw := c.Query("actor_id"); raw != "" {
		actorID, err := uuid.Parse(raw)
		if err != nil {
			return ListQuery{}, apperror.BadRequest("actor_id must be a UUID")
		}
		q.ActorID = actorID
	}
	q.Action = c.Query("action")
	if raw := c.Query("from"); raw != "" {
		from, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return ListQuery{}, apperror.BadRequest("from must be an RFC3339 timestamp")
		}
		q.From = from
	}
	if raw := c.Query("to"); raw != "" {
		to, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return ListQuery{}, apperror.BadRequest("to must be an RFC3339 timestamp")
		}
		q.To = to
	}
	q.Cursor = c.Query("cursor")
	if raw := c.Query("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil {
			return ListQuery{}, apperror.BadRequest("limit must be an integer")
		}
		q.Limit = limit
	}
	return q, nil
}

// list returns one page of the center's audit trail, newest first.
//
//	@Summary		List audit logs
//	@Description	Owner-only. Keyset-paginated, newest first; pass next_cursor back to fetch the following page. A cursor is only valid alongside the same filters that produced it.
//	@Tags			audit-logs
//	@Produce		json
//	@Param			actor_id	query		string	false	"filter by actor teacher id (UUID)"
//	@Param			action		query		string	false	"action prefix, e.g. class. or auth.login"
//	@Param			from		query		string	false	"RFC3339 lower bound on occurred_at"
//	@Param			to			query		string	false	"RFC3339 upper bound on occurred_at"
//	@Param			cursor		query		string	false	"opaque cursor from a previous page"
//	@Param			limit		query		int		false	"page size (default 50, max 100; out-of-range values are clamped)"
//	@Success		200			{object}	response.Envelope{data=ListResponse}
//	@Failure		400			{object}	response.Envelope{error=response.ErrorBody}	"malformed filter or cursor"
//	@Failure		401			{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403			{object}	response.Envelope{error=response.ErrorBody}	"caller is not the center owner"
//	@Security		BearerAuth
//	@Router			/audit-logs [get]
func (h *Handler) list(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	q, err := parseListQuery(c)
	if err != nil {
		response.Err(c, err)
		return
	}
	rows, next, err := h.svc.List(c.Request.Context(), sc, q)
	if err != nil {
		response.Err(c, err)
		return
	}
	items := make([]LogResponse, 0, len(rows))
	for i := range rows {
		items = append(items, FromRow(&rows[i]))
	}
	response.OK(c, http.StatusOK, ListResponse{Items: items, NextCursor: next})
}
