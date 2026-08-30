package statements

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/shared/response"
)

// listSorts whitelists the public sort keys for GET .../statements.
var listSorts = map[string]string{
	"created_at": "statements.created_at",
	"total_due":  "statements.total_due",
}

// Handler exposes the statement endpoints. Every one of them requires
// authentication — the token/url a StatementResponse carries must never
// reach an unauthenticated caller.
type Handler struct {
	svc *Service
}

// NewHandler builds the statements handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// scope resolves the authenticated tenant scope — the only sanctioned source
// of teacher/center identity; the path never carries it.
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

// toResponses maps a page of statement rows onto their DTOs, masked for sc.
func (h *Handler) toResponses(sc authctx.Scope, rows []Row) []StatementResponse {
	out := make([]StatementResponse, 0, len(rows))
	for i := range rows {
		out = append(out, h.svc.ToResponse(sc, rows[i]))
	}
	return out
}

// generate creates or refreshes one statement per eligible contact in a
// closed billing period.
//
//	@Summary		Generate statements for a billing period
//	@Description	The period must be closed. Re-running is safe: existing, not-yet-revoked statements have only their total refreshed, their link is unchanged, and revoked statements are left untouched.
//	@Tags			statements
//	@Produce		json
//	@Param			id	path		string	true	"billing period id"
//	@Success		200	{object}	response.Envelope{data=GenerateResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}	"period is not closed"
//	@Security		BearerAuth
//	@Router			/billing-periods/{id}/statements/generate [post]
func (h *Handler) generate(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	periodID, ok := pathID(c, "id", "billing period")
	if !ok {
		return
	}
	result, err := h.svc.Generate(c.Request.Context(), sc, periodID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, GenerateResponse{
		Created:        result.Created,
		Refreshed:      result.Refreshed,
		SkippedRevoked: result.SkippedRevoked,
		Statements:     h.toResponses(sc, result.Statements),
	})
}

// list returns a page of one billing period's statements — the family ones
// by default, or one class's copies when class_id is given.
//
//	@Summary		List a billing period's statements
//	@Description	Without class_id lists the period's family statements. With class_id lists that class's class-scoped copies; requires class-send access to the class (active sending-role stint or reports oversight) — a readable-but-not-sendable role gets 403, no stint a neutral 404.
//	@Tags			statements
//	@Produce		json
//	@Param			id			path		string	true	"billing period id"
//	@Param			class_id	query		string	false	"list one class's class-scoped copies instead of the family statements"
//	@Param			page		query		int		false	"page number"
//	@Param			per_page	query		int		false	"page size (max 100)"
//	@Param			sort		query		string	false	"created_at (default) or total_due, - prefix for desc"
//	@Success		200			{object}	response.Envelope{data=[]StatementResponse,meta=response.Meta}
//	@Failure		401			{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403			{object}	response.Envelope{error=response.ErrorBody}	"role on the class does not allow sending"
//	@Failure		404			{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/billing-periods/{id}/statements [get]
func (h *Handler) list(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	periodID, ok := pathID(c, "id", "billing period")
	if !ok {
		return
	}
	params := pagination.Parse(c, "created_at", listSorts)

	if raw := c.Query("class_id"); raw != "" {
		classID, err := uuid.Parse(raw)
		if err != nil {
			// A malformed class id reads as the class not existing, matching
			// pathID's treatment of malformed path ids.
			response.Err(c, apperror.NotFound("class"))
			return
		}
		rows, total, err := h.svc.ListClass(c.Request.Context(), sc, periodID, classID, params)
		if err != nil {
			response.Err(c, err)
			return
		}
		// ListClass already ran the class-send gate, so every row's link may
		// be shown to this caller.
		out := make([]StatementResponse, 0, len(rows))
		for i := range rows {
			out = append(out, h.svc.ToResponseForSend(sc, rows[i]))
		}
		response.List(c, out, params.Meta(total))
		return
	}

	rows, total, err := h.svc.List(c.Request.Context(), sc, periodID, params)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.List(c, h.toResponses(sc, rows), params.Meta(total))
}

// get returns one statement.
//
//	@Summary		Get a statement
//	@Tags			statements
//	@Produce		json
//	@Param			id	path		string	true	"statement id"
//	@Success		200	{object}	response.Envelope{data=StatementResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/statements/{id} [get]
func (h *Handler) get(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	statementID, ok := pathID(c, "id", "statement")
	if !ok {
		return
	}
	row, err := h.svc.Get(c.Request.Context(), sc, statementID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, h.svc.ToResponseAuthorized(c.Request.Context(), sc, *row))
}

// revoke kills one statement's link.
//
//	@Summary		Revoke a statement
//	@Description	Idempotent: revoking an already-revoked statement succeeds without changing it.
//	@Tags			statements
//	@Produce		json
//	@Param			id	path	string	true	"statement id"
//	@Success		204
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/statements/{id}/revoke [post]
func (h *Handler) revoke(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	statementID, ok := pathID(c, "id", "statement")
	if !ok {
		return
	}
	if err := h.svc.Revoke(c.Request.Context(), sc, statementID); err != nil {
		response.Err(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
