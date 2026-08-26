package invitations

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// Handler exposes the owner-only invitation endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds the invitations handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// scope resolves the authenticated caller's center scope — the only
// sanctioned source of tenant identity; request bodies and paths never carry
// it.
func (h *Handler) scope(c *gin.Context) (authctx.Scope, bool) {
	sc, ok := authctx.ScopeFrom(c)
	if !ok {
		response.Err(c, apperror.Unauthorized("authentication required"))
		return authctx.Scope{}, false
	}
	return sc, true
}

// invitationID parses the :id path parameter.
func invitationID(c *gin.Context) (uuid.UUID, bool) {
	parsed, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Err(c, apperror.NotFound("invitation"))
		return uuid.UUID{}, false
	}
	return parsed, true
}

// create issues a new invitation for a phone number.
//
//	@Summary		Create an invitation
//	@Description	Owner-only. Supersedes any pending invite already outstanding for the phone. Always returns a copy-link; dm_status ("skipped"/"failed"/"sent") reports a best-effort Zalo DM attempt that never blocks or fails the create.
//	@Tags			invitations
//	@Accept			json
//	@Produce		json
//	@Param			request	body		CreateRequest	true	"invitee phone"
//	@Success		201		{object}	response.Envelope{data=CreateResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"not the center owner"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"validation failed"
//	@Security		BearerAuth
//	@Router			/centers/me/invitations [post]
func (h *Handler) create(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	if err := requireOwner(sc); err != nil {
		response.Err(c, err)
		return
	}
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	resp, err := h.svc.Create(c.Request.Context(), sc, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, resp)
}

// list returns every invitation in the caller's center, pending first.
//
//	@Summary		List invitations
//	@Description	Owner-only. status is "pending", "accepted", "revoked", or the derived "expired" (pending past its deadline).
//	@Tags			invitations
//	@Produce		json
//	@Success		200	{object}	response.Envelope{data=[]InvitationResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}	"not the center owner"
//	@Security		BearerAuth
//	@Router			/centers/me/invitations [get]
func (h *Handler) list(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	if err := requireOwner(sc); err != nil {
		response.Err(c, err)
		return
	}
	rows, err := h.svc.List(c.Request.Context(), sc)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, rows)
}

// revoke cancels a pending invitation.
//
//	@Summary		Revoke an invitation
//	@Description	Owner-only. Idempotent: revoking an already-revoked invite still succeeds. An id belonging to another center answers 404, the same as one that never existed.
//	@Tags			invitations
//	@Produce		json
//	@Param			id	path	string	true	"invitation id"
//	@Success		204
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}	"not the center owner"
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/centers/me/invitations/{id} [delete]
func (h *Handler) revoke(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	if err := requireOwner(sc); err != nil {
		response.Err(c, err)
		return
	}
	invID, ok := invitationID(c)
	if !ok {
		return
	}
	if err := h.svc.Revoke(c.Request.Context(), sc, invID); err != nil {
		response.Err(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// preview is the public, pre-authentication read an invitee sees before
// accepting.
//
//	@Summary		Preview an invitation
//	@Description	Public, unauthenticated. Token travels in the body, never the path, so it never appears in an access log. Any invalid/used/expired/revoked token answers the same generic 404.
//	@Tags			invitations
//	@Accept			json
//	@Produce		json
//	@Param			request	body		PreviewRequest	true	"invite token"
//	@Success		200		{object}	response.Envelope{data=PreviewResponse}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"validation failed"
//	@Failure		429		{object}	response.Envelope{error=response.ErrorBody}
//	@Router			/invitations/preview [post]
func (h *Handler) preview(c *gin.Context) {
	var req PreviewRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	resp, err := h.svc.Preview(c.Request.Context(), req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, resp)
}

// accept redeems a pending invitation: creates a new account, or reactivates
// a previously-removed one, in the inviting center.
//
//	@Summary		Accept an invitation
//	@Description	Public, unauthenticated. Token travels in the body. Every rejection reason — unknown/used/expired/revoked token, an already-active account, or a disabled account that was never a member of this center — answers the same generic 400 body, by design (anti-enumeration).
//	@Tags			invitations
//	@Accept			json
//	@Produce		json
//	@Param			request	body	AcceptRequest	true	"token, full name, and new password"
//	@Success		204
//	@Failure		400	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422	{object}	response.Envelope{error=response.ErrorBody}	"validation failed"
//	@Failure		429	{object}	response.Envelope{error=response.ErrorBody}
//	@Router			/invitations/accept [post]
func (h *Handler) accept(c *gin.Context) {
	var req AcceptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	meta := ClientMeta{IP: c.ClientIP(), UserAgent: c.Request.UserAgent()}
	if err := h.svc.Accept(c.Request.Context(), req, meta); err != nil {
		response.Err(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
