package payments

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// listSorts whitelists the public sort keys for GET /payments.
var listSorts = map[string]string{
	"received_on": "payments.received_on",
	"created_at":  "payments.created_at",
}

// Handler exposes the payment recording and read endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds the payments handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// scope resolves the authenticated tenant scope — the only sanctioned source
// of teacher/center identity; request bodies and paths never carry it.
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

// queryUUID parses an optional uuid query filter; absent means unset, a
// malformed value is a 422 naming the parameter.
func queryUUID(c *gin.Context, name string) (uuid.UUID, bool) {
	raw := c.Query(name)
	if raw == "" {
		return uuid.Nil, true
	}
	parsed, err := uuid.Parse(raw)
	if err != nil {
		response.Err(c, apperror.Invalid("validation failed",
			map[string]string{name: "must be a UUID"}))
		return uuid.Nil, false
	}
	return parsed, true
}

// queryDate parses an optional YYYY-MM-DD query filter; absent means unset, a
// malformed value is a 422 naming the parameter.
func queryDate(c *gin.Context, name string) (*time.Time, bool) {
	raw := c.Query(name)
	if raw == "" {
		return nil, true
	}
	parsed, err := time.Parse(dateLayout, raw)
	if err != nil {
		response.Err(c, apperror.Invalid("validation failed",
			map[string]string{name: "must be a date in YYYY-MM-DD form"}))
		return nil, false
	}
	return &parsed, true
}

// record records a payment against a contact and auto-allocates it across
// that family's outstanding invoices per D8.
//
//	@Summary		Record payment
//	@Description	Allocates the amount across the contact's outstanding invoices: opening balance across all of them before any current charge, ties broken by earlier class start date then invoice id. Any amount that could not be placed is returned as unallocated_amount, never lost.
//	@Tags			payments
//	@Accept			json
//	@Produce		json
//	@Param			request	body		RecordPaymentRequest	true	"payment fields"
//	@Success		201		{object}	response.Envelope{data=PaymentResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}	"contact not found"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"validation failed"
//	@Security		BearerAuth
//	@Router			/payments [post]
func (h *Handler) record(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	var req RecordPaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	detail, err := h.svc.Record(c.Request.Context(), sc, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, FromDetail(*detail))
}

// list returns a page of payments, each with its allocation breakdown.
//
//	@Summary		List payments
//	@Produce		json
//	@Tags			payments
//	@Param			contact_id		query		string	false	"filter by contact"
//	@Param			period_id		query		string	false	"filter to payments with an allocation in this billing period"
//	@Param			received_from	query		string	false	"received_on >= this date (YYYY-MM-DD)"
//	@Param			received_to		query		string	false	"received_on <= this date (YYYY-MM-DD)"
//	@Param			page			query		int		false	"page number"
//	@Param			per_page		query		int		false	"page size (max 100)"
//	@Param			sort			query		string	false	"received_on or created_at; - prefix for desc"
//	@Success		200				{object}	response.Envelope{data=[]PaymentResponse,meta=response.Meta}
//	@Failure		401				{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422				{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/payments [get]
func (h *Handler) list(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	filter := ListFilter{}
	if filter.ContactID, ok = queryUUID(c, "contact_id"); !ok {
		return
	}
	if filter.PeriodID, ok = queryUUID(c, "period_id"); !ok {
		return
	}
	if filter.ReceivedFrom, ok = queryDate(c, "received_from"); !ok {
		return
	}
	if filter.ReceivedTo, ok = queryDate(c, "received_to"); !ok {
		return
	}
	params := pagination.Parse(c, "-received_on", listSorts)
	details, total, err := h.svc.List(c.Request.Context(), sc, filter, params)
	if err != nil {
		response.Err(c, err)
		return
	}
	out := make([]PaymentResponse, 0, len(details))
	for _, d := range details {
		out = append(out, FromDetail(d))
	}
	response.List(c, out, params.Meta(total))
}

// get returns one payment with its allocation breakdown.
//
//	@Summary		Get payment
//	@Tags			payments
//	@Produce		json
//	@Param			id	path		string	true	"payment id"
//	@Success		200	{object}	response.Envelope{data=PaymentResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/payments/{id} [get]
func (h *Handler) get(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	paymentID, ok := pathID(c, "id", "payment")
	if !ok {
		return
	}
	detail, err := h.svc.Get(c.Request.Context(), sc, paymentID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, FromDetail(*detail))
}

// reallocate overrides a payment's split with a teacher-supplied set of
// amounts, replacing D8's automatic choice.
//
//	@Summary		Reallocate payment
//	@Description	Replaces the payment's allocation rows with the given set, marked allocated_by=manual. Every invoice must belong to the same teacher and the same contact as the payment, be issued/partially_paid/paid, and receive no more than its outstanding balance; the sum of amounts must not exceed the payment. A reversed payment, or a reversal entry itself, cannot be reallocated.
//	@Tags			payments
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"payment id"
//	@Param			request	body		ReallocateRequest	true	"new allocation set"
//	@Success		200		{object}	response.Envelope{data=PaymentResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"validation failed"
//	@Security		BearerAuth
//	@Router			/payments/{id}/allocations [put]
func (h *Handler) reallocate(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	paymentID, ok := pathID(c, "id", "payment")
	if !ok {
		return
	}
	var req ReallocateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	detail, err := h.svc.Reallocate(c.Request.Context(), sc, paymentID, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, FromDetail(*detail))
}

// autoAllocate re-runs D8's allocation over a payment's unallocated
// remainder, leaving its existing allocations untouched.
//
//	@Summary		Re-run auto-allocation for the remainder
//	@Description	Allocates only the amount not yet placed by this payment, across the contact's currently outstanding invoices. Existing allocations are left alone; a fresh allocation merges into any row this payment already holds on the same invoice. 409 when there is no remainder to place, or when the payment is reversed or is itself a reversal entry.
//	@Tags			payments
//	@Produce		json
//	@Param			id	path		string	true	"payment id"
//	@Success		200	{object}	response.Envelope{data=PaymentResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/payments/{id}/allocations/auto [post]
func (h *Handler) autoAllocate(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	paymentID, ok := pathID(c, "id", "payment")
	if !ok {
		return
	}
	detail, err := h.svc.AutoAllocateRemainder(c.Request.Context(), sc, paymentID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, FromDetail(*detail))
}

// reverse cancels a payment with a counter-entry: a new payments row
// pointing reverses_payment_id at the original, which is stamped
// reversed_at. Nothing is ever deleted.
//
//	@Summary		Reverse payment
//	@Description	Creates a new payment row that cancels this one — same contact, amount, and method, received today, with the original's allocations mirrored onto it — and stamps reversed_at on the original. A payment that is already reversed, or is itself a reversal entry, cannot be reversed again.
//	@Tags			payments
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"payment id"
//	@Param			request	body		ReverseRequest	true	"reversal reason"
//	@Success		201		{object}	response.Envelope{data=PaymentResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"already reversed or is a reversal entry"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"validation failed"
//	@Security		BearerAuth
//	@Router			/payments/{id}/reverse [post]
func (h *Handler) reverse(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	paymentID, ok := pathID(c, "id", "payment")
	if !ok {
		return
	}
	var req ReverseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	detail, err := h.svc.Reverse(c.Request.Context(), sc, paymentID, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, FromDetail(*detail))
}
