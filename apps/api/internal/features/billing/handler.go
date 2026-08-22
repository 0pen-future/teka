package billing

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// listSorts whitelists the public sort keys for GET /billing-periods.
var listSorts = map[string]string{
	"period_start": "billing_periods.period_start",
	"created_at":   "billing_periods.created_at",
}

// Handler exposes the billing period endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds the billing handler.
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

// ensurePeriod creates or fetches a billing period for one calendar month.
//
//	@Summary		Ensure billing period
//	@Description	Idempotent create-or-get: calling this twice for the same (teacher, year, month) returns the same period id rather than a 409.
//	@Tags			billing
//	@Accept			json
//	@Produce		json
//	@Param			request	body		EnsurePeriodRequest	true	"year and month"
//	@Success		201		{object}	response.Envelope{data=PeriodResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"validation failed"
//	@Security		BearerAuth
//	@Router			/billing-periods [post]
func (h *Handler) ensurePeriod(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	var req EnsurePeriodRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	period, err := h.svc.EnsurePeriod(c.Request.Context(), sc, req.Year, req.Month)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, FromPeriodModel(period))
}

// list returns a page of the teacher's billing periods.
//
//	@Summary		List billing periods
//	@Produce		json
//	@Tags			billing
//	@Param			page		query		int		false	"page number"
//	@Param			per_page	query		int		false	"page size (max 100)"
//	@Param			sort		query		string	false	"period_start or created_at; - prefix for desc"
//	@Success		200			{object}	response.Envelope{data=[]PeriodResponse,meta=response.Meta}
//	@Failure		401			{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/billing-periods [get]
func (h *Handler) list(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	params := pagination.Parse(c, "-period_start", listSorts)
	rows, total, err := h.svc.ListPeriods(c.Request.Context(), sc, params)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.List(c, FromPeriodModels(rows), params.Meta(total))
}

// get returns one billing period.
//
//	@Summary		Get billing period
//	@Tags			billing
//	@Produce		json
//	@Param			id	path		string	true	"billing period id"
//	@Success		200	{object}	response.Envelope{data=PeriodResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/billing-periods/{id} [get]
func (h *Handler) get(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	periodID, ok := pathID(c, "id", "billing period")
	if !ok {
		return
	}
	period, err := h.svc.GetPeriod(c.Request.Context(), sc, periodID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, FromPeriodModel(period))
}

// preview computes the chốt sổ review screen for a period without writing
// anything (R4): every student with their per-class session counts,
// amounts, carried debt, and a grand total.
//
//	@Summary		Preview a billing period
//	@Description	Pure read — never creates or updates invoices. Every student with a billable session or carried debt from the previous closed period appears once with one line per class.
//	@Tags			billing
//	@Produce		json
//	@Param			id	path		string	true	"billing period id"
//	@Success		200	{object}	response.Envelope{data=PreviewResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/billing-periods/{id}/preview [get]
func (h *Handler) preview(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	periodID, ok := pathID(c, "id", "billing period")
	if !ok {
		return
	}
	resp, err := h.svc.Preview(c.Request.Context(), sc, periodID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, resp)
}

// draft persists the current preview as draft invoices and invoice_lines, so
// review adjustments (phase 4) have a row to attach to before the period
// closes. Idempotent: calling it again after attendance changes updates the
// same rows rather than duplicating them.
//
//	@Summary		Draft a billing period's invoices
//	@Description	Only allowed while the period is open. Refuses to touch an invoice that has already moved past draft.
//	@Tags			billing
//	@Produce		json
//	@Param			id	path		string	true	"billing period id"
//	@Success		200	{object}	response.Envelope{data=PreviewResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}	"period is closed, or an invoice for it has already been issued"
//	@Security		BearerAuth
//	@Router			/billing-periods/{id}/draft [post]
func (h *Handler) draft(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	periodID, ok := pathID(c, "id", "billing period")
	if !ok {
		return
	}
	resp, err := h.svc.Draft(c.Request.Context(), sc, periodID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, resp)
}

// close runs chốt sổ (R4): locks the period, hard-blocks on any past session
// without confirmed attendance, then issues every drafted invoice with
// money owed and voids the rest. Irreversible — there is no reopen; a
// mistake after close is corrected through an adjustment on the next period
// or by voiding the affected invoice.
//
//	@Summary		Close a billing period
//	@Description	Irreversible. Blocked (409) if any session in the period is past due without confirmed attendance — the response names each one. There is no reopen.
//	@Tags			billing
//	@Produce		json
//	@Param			id	path		string	true	"billing period id"
//	@Success		200	{object}	response.Envelope{data=CloseResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}	"period not open, or has unconfirmed sessions (details.unconfirmed_sessions)"
//	@Security		BearerAuth
//	@Router			/billing-periods/{id}/close [post]
func (h *Handler) close(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	periodID, ok := pathID(c, "id", "billing period")
	if !ok {
		return
	}
	resp, err := h.svc.Close(c.Request.Context(), sc, periodID)
	if err != nil {
		var blocked *ErrUnconfirmedSessions
		if errors.As(err, &blocked) {
			response.ErrWithDetails(c, apperror.Conflict("period has unconfirmed sessions"),
				gin.H{"unconfirmed_sessions": blocked.Sessions})
			return
		}
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, resp)
}

// voidInvoice corrects an issued invoice — the only path available once it
// can no longer be re-drafted. The period may already be closed.
//
//	@Summary		Void an invoice
//	@Description	Only an issued or partially-paid invoice with paid_amount=0 can be voided. A paid invoice's payment must be reversed first.
//	@Tags			billing
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"invoice id"
//	@Param			request	body		VoidInvoiceRequest	true	"reason"
//	@Success		200		{object}	response.Envelope{data=InvoiceResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"invoice not issued/partially paid, or has a recorded payment"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"validation failed"
//	@Security		BearerAuth
//	@Router			/invoices/{id}/void [post]
func (h *Handler) voidInvoice(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	invoiceID, ok := pathID(c, "id", "invoice")
	if !ok {
		return
	}
	var req VoidInvoiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	resp, err := h.svc.VoidInvoice(c.Request.Context(), sc, invoiceID, req.Reason)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, resp)
}

// addAdjustment posts one manual, signed correction to an invoice (R4).
// There is no delete endpoint: reversing a correction posts a new,
// opposite-signed adjustment instead, so the audit trail stays complete.
//
//	@Summary		Add a manual invoice adjustment
//	@Description	Allowed while the invoice is draft, issued, or partially paid. Refused on void and paid — a paid invoice's correction belongs on the next period.
//	@Tags			billing
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"invoice id"
//	@Param			request	body		AdjustmentRequest	true	"signed amount and reason"
//	@Success		201		{object}	response.Envelope{data=AdjustmentResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"invoice is void or fully paid"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"validation failed"
//	@Security		BearerAuth
//	@Router			/invoices/{id}/adjustments [post]
func (h *Handler) addAdjustment(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	invoiceID, ok := pathID(c, "id", "invoice")
	if !ok {
		return
	}
	var req AdjustmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	adjResp, _, err := h.svc.AddAdjustment(c.Request.Context(), sc, invoiceID, req.Amount, req.Reason)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, adjResp)
}

// listAdjustments returns one invoice's adjustment audit trail (R4), oldest
// first.
//
//	@Summary		List an invoice's adjustments
//	@Tags			billing
//	@Produce		json
//	@Param			id	path		string	true	"invoice id"
//	@Success		200	{object}	response.Envelope{data=[]AdjustmentResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/invoices/{id}/adjustments [get]
func (h *Handler) listAdjustments(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	invoiceID, ok := pathID(c, "id", "invoice")
	if !ok {
		return
	}
	rows, err := h.svc.ListAdjustments(c.Request.Context(), sc, invoiceID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, rows)
}
