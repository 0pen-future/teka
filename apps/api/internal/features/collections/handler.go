package collections

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

// Handler exposes the collection board endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds the collections handler.
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

// parseFilter reads status, class_id, and q off the query string. class_id
// must be a well-formed uuid when present — that check is a 400, distinct
// from view=class requiring it at all, which the service reports as 422.
func parseFilter(c *gin.Context) (Filter, error) {
	filter := Filter{
		Status: c.Query("status"),
		Query:  c.Query("q"),
	}
	if filter.Status != "" && !validStatuses[filter.Status] {
		return filter, apperror.Invalid("validation failed", map[string]string{"status": "must be unpaid, partial, or paid"})
	}
	if raw := c.Query("class_id"); raw != "" {
		classID, err := uuid.Parse(raw)
		if err != nil {
			return filter, apperror.BadRequest("class_id is not a valid id")
		}
		filter.ClassID = &classID
	}
	return filter, nil
}

// list returns either view of one billing period's collection board.
//
//	@Summary		List a billing period's collections
//	@Description	view=contact (default) merges every child under one row per family. view=class lists one row per invoice line for the given class_id, which is required in that view.
//	@Tags			collections
//	@Produce		json
//	@Param			id			path		string	true	"billing period id"
//	@Param			view		query		string	false	"contact (default) or class"
//	@Param			class_id	query		string	false	"required when view=class"
//	@Param			status		query		string	false	"unpaid, partial, or paid"
//	@Param			q			query		string	false	"substring match on contact or student name"
//	@Param			page		query		int		false	"page number"
//	@Param			per_page	query		int		false	"page size (max 100)"
//	@Param			sort		query		string	false	"contact view: outstanding (default, desc), full_name, total_due. class view: student_name (default), outstanding. - prefix for desc"
//	@Success		200			{object}	response.Envelope{data=[]ContactBalanceRow,meta=response.Meta}
//	@Success		200			{object}	response.Envelope{data=[]ClassCollectionRow,meta=response.Meta}
//	@Failure		401			{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404			{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422			{object}	response.Envelope{error=response.ErrorBody}	"validation failed"
//	@Security		BearerAuth
//	@Router			/billing-periods/{id}/collections [get]
func (h *Handler) list(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	periodID, ok := pathID(c, "id", "billing period")
	if !ok {
		return
	}
	filter, err := parseFilter(c)
	if err != nil {
		response.Err(c, err)
		return
	}

	view := c.DefaultQuery("view", ViewContact)
	var params pagination.Params
	if view == ViewClass {
		params = pagination.Parse(c, "student_name", ClassSortColumns())
	} else {
		params = pagination.Parse(c, "-outstanding", ContactSortColumns())
	}

	result, err := h.svc.List(c.Request.Context(), sc, periodID, view, filter, params)
	if err != nil {
		response.Err(c, err)
		return
	}
	if view == ViewClass {
		response.List(c, result.ClassRows, params.Meta(result.Total))
		return
	}
	response.List(c, result.ContactRows, params.Meta(result.Total))
}

// summary returns one billing period's collection totals.
//
//	@Summary		Summarize a billing period's collections
//	@Tags			collections
//	@Produce		json
//	@Param			id	path		string	true	"billing period id"
//	@Success		200	{object}	response.Envelope{data=SummaryResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/billing-periods/{id}/collections/summary [get]
func (h *Handler) summary(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	periodID, ok := pathID(c, "id", "billing period")
	if !ok {
		return
	}
	resp, err := h.svc.Summary(c.Request.Context(), sc, periodID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, resp)
}

// contactBalances returns each family's outstanding balance for one month.
//
//	@Summary		List monthly outstanding balances per contact
//	@Description	Sums each contact's outstanding balance over every billing period of the center in the given month (periods are per teacher, so one family may owe on several). Uses the same figures as the collection board: draft invoices of an open period count, void invoices do not. Only contacts whose sum is non-zero are returned; a month with no period returns an empty list. Not paginated. Requires billing.view_all.
//	@Tags			collections
//	@Produce		json
//	@Param			year	query		int	true	"calendar year (2020-2100)"
//	@Param			month	query		int	true	"calendar month (1-12)"
//	@Success		200		{object}	response.Envelope{data=[]ContactOutstanding}
//	@Failure		400		{object}	response.Envelope{error=response.ErrorBody}	"non-numeric year or month"
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"caller lacks billing.view_all"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"validation failed"
//	@Security		BearerAuth
//	@Router			/collections/contact-balances [get]
func (h *Handler) contactBalances(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	var q MonthBalancesQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	rows, err := h.svc.ContactOutstandingByMonth(c.Request.Context(), sc, q.Year, q.Month)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, rows)
}
