package notifications

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// Handler exposes the notification bulk-send, list, and mark-sent endpoints.
// Every one of them requires authentication.
type Handler struct {
	svc *Service
}

// NewHandler builds the notifications handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// scope resolves the authenticated caller's teacher/center/owner scope — the
// only sanctioned source of tenant identity; the path never carries it.
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

// bulkSend queues one notification per eligible contact in a billing period.
//
//	@Summary		Bulk-send statement notifications
//	@Description	Regenerates/refreshes the period's statements, then queues one message per eligible contact: purpose=statement (or "statements") targets every contact with a non-void invoice; purpose=reminder further narrows to contacts with outstanding > 0. One row per contact, never one per child.
//	@Tags			notifications
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"billing period id"
//	@Param			request	body		BulkSendRequest	true	"purpose and optional channel"
//	@Success		200		{object}	response.Envelope{data=BulkSendResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"caller lacks the send-reports permission"
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"period is not closed"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"validation failed"
//	@Security		BearerAuth
//	@Router			/billing-periods/{id}/notifications/bulk [post]
func (h *Handler) bulkSend(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	periodID, ok := pathID(c, "id", "billing period")
	if !ok {
		return
	}
	var req BulkSendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	result, err := h.svc.BulkSend(c.Request.Context(), sc, periodID, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, result)
}

// sendPreview returns the server-computed pre-send buckets for one period.
//
//	@Summary		Preview a zalo_personal bulk send
//	@Description	Computes, without writing anything, the exact buckets a zalo_personal bulk send would produce right now: auto_send (mapped + friend of the caller's Zalo account), mapped_not_friend, and unmapped (manual fallback), plus the server's max_run_size cap. purpose=reminder narrows to contacts with outstanding > 0.
//	@Tags			notifications
//	@Produce		json
//	@Param			id		path		string	true	"billing period id"
//	@Param			purpose	query		string	false	"statements (default) or reminder"
//	@Success		200		{object}	response.Envelope{data=SendPreviewResponse}
//	@Failure		400		{object}	response.Envelope{error=response.ErrorBody}	"no linked Zalo account"
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"caller lacks the send-reports permission"
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"cross-teacher period without the permission, or the Zalo session expired"
//	@Security		BearerAuth
//	@Router			/billing-periods/{id}/notifications/preview [get]
func (h *Handler) sendPreview(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	periodID, ok := pathID(c, "id", "billing period")
	if !ok {
		return
	}
	preview, err := h.svc.SendPreview(c.Request.Context(), sc, periodID, c.Query("purpose"))
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, preview)
}

// runSnapshot reports the period's latest zalo_personal run and its progress.
//
//	@Summary		Poll a period's zalo_personal run
//	@Description	Returns the period's latest background send run with progress counters derived from its rows. A period that never had a run answers active=false with a null run_id, not a 404 — poll this after a zalo_personal bulk send until active turns false.
//	@Tags			notifications
//	@Produce		json
//	@Param			id	path		string	true	"billing period id"
//	@Success		200	{object}	response.Envelope{data=RunSnapshotResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/billing-periods/{id}/notifications/run [get]
func (h *Handler) runSnapshot(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	periodID, ok := pathID(c, "id", "billing period")
	if !ok {
		return
	}
	snapshot, err := h.svc.RunSnapshot(c.Request.Context(), sc, periodID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, snapshot)
}

// resumeRun restarts the period's interrupted zalo_personal run.
//
//	@Summary		Resume an interrupted zalo_personal run
//	@Description	Re-renders and re-sends only the run's still-queued rows; rows already sent keep their verdict. Only a run in the interrupted state qualifies. Rows that can no longer be auto-sent (mapping removed, balance since paid) are failed with a reason.
//	@Tags			notifications
//	@Produce		json
//	@Param			id	path		string	true	"billing period id"
//	@Success		200	{object}	response.Envelope{data=RunSnapshotResponse}
//	@Failure		400	{object}	response.Envelope{error=response.ErrorBody}	"no linked Zalo account"
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}	"caller lacks the send-reports permission"
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}	"period has no run"
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}	"run is not interrupted, another run is sending, or the Zalo session expired"
//	@Security		BearerAuth
//	@Router			/billing-periods/{id}/notifications/run/resume [post]
func (h *Handler) resumeRun(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	periodID, ok := pathID(c, "id", "billing period")
	if !ok {
		return
	}
	snapshot, err := h.svc.ResumeRun(c.Request.Context(), sc, periodID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, snapshot)
}

// markSent marks the given notification ids as sent.
//
//	@Summary		Mark notifications sent
//	@Description	Idempotent: an id already sent, or not belonging to the caller, is silently skipped rather than erroring.
//	@Tags			notifications
//	@Accept			json
//	@Produce		json
//	@Param			request	body	MarkSentRequest	true	"notification ids"
//	@Success		204
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422	{object}	response.Envelope{error=response.ErrorBody}	"validation failed"
//	@Security		BearerAuth
//	@Router			/notifications/mark-sent [post]
func (h *Handler) markSent(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	var req MarkSentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	if err := h.svc.MarkSent(c.Request.Context(), sc, req.IDs); err != nil {
		response.Err(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// list returns one billing period's notification ledger.
//
//	@Summary		List a billing period's notifications
//	@Tags			notifications
//	@Produce		json
//	@Param			id		path		string	true	"billing period id"
//	@Param			purpose	query		string	false	"statements or reminder"
//	@Param			status	query		string	false	"queued, sent, delivered, or failed"
//	@Success		200		{object}	response.Envelope{data=[]NotificationResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/billing-periods/{id}/notifications [get]
func (h *Handler) list(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	periodID, ok := pathID(c, "id", "billing period")
	if !ok {
		return
	}
	filter := ListFilter{
		Purpose: c.Query("purpose"),
		Status:  c.Query("status"),
	}
	rows, err := h.svc.List(c.Request.Context(), sc, periodID, filter)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, rows)
}
