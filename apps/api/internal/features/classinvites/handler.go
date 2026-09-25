package classinvites

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// Handler exposes the class-invitation endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds the classinvites handler.
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

// send creates a pending invitation for a member to take a role in the class.
//
//	@Summary		Invite a member to a class role
//	@Description	Owner only. Proposes a role (giao_vien, tro_giang, hoc_vu) to an active member of the center. No class_staff stint is written until the owner confirms. 422 SELF_INVITE when inviting oneself, 422 MEMBER_INACTIVE when the invitee is not an active member, 409 when a pending invitation exists or the person already holds that role.
//	@Tags			class-invitations
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string		true	"class id"
//	@Param			body	body		SendRequest	true	"invitee, role and optional message"
//	@Success		201		{object}	response.Envelope{data=InvitationResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/classes/{id}/invitations [post]
func (h *Handler) send(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	var req SendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.Send(c.Request.Context(), sc, classID, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, out)
}

// list returns the invitations the caller may see.
//
//	@Summary		List class invitations
//	@Description	The owner sees every invitation of the center; a member sees only invitations addressed to them. Optional status and class_id filters never widen visibility.
//	@Tags			class-invitations
//	@Produce		json
//	@Param			status		query		string	false	"pending | accepted | declined | cancelled | assigned"
//	@Param			class_id	query		string	false	"class id"
//	@Success		200			{object}	response.Envelope{data=[]InvitationResponse}
//	@Failure		401			{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422			{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/class-invitations [get]
func (h *Handler) list(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	var q ListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.List(c.Request.Context(), sc, q)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// accept marks the caller's pending invitation accepted.
//
//	@Summary		Accept a class invitation
//	@Description	Invitee only; pending → accepted. Writes no class_staff row — the owner completes the assignment with confirm. Another member's invitation is a 404.
//	@Tags			class-invitations
//	@Produce		json
//	@Param			id	path		string	true	"invitation id"
//	@Success		200	{object}	response.Envelope{data=InvitationResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}	"no longer pending"
//	@Security		BearerAuth
//	@Router			/class-invitations/{id}/accept [post]
func (h *Handler) accept(c *gin.Context) {
	h.act(c, h.svc.Accept)
}

// decline marks the caller's pending invitation declined.
//
//	@Summary		Decline a class invitation
//	@Description	Invitee only; pending → declined.
//	@Tags			class-invitations
//	@Produce		json
//	@Param			id	path		string	true	"invitation id"
//	@Success		200	{object}	response.Envelope{data=InvitationResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}	"no longer pending"
//	@Security		BearerAuth
//	@Router			/class-invitations/{id}/decline [post]
func (h *Handler) decline(c *gin.Context) {
	h.act(c, h.svc.Decline)
}

// cancel withdraws a pending or accepted invitation.
//
//	@Summary		Cancel a class invitation
//	@Description	Owner only; pending | accepted → cancelled.
//	@Tags			class-invitations
//	@Produce		json
//	@Param			id	path		string	true	"invitation id"
//	@Success		200	{object}	response.Envelope{data=InvitationResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}	"already ended"
//	@Security		BearerAuth
//	@Router			/class-invitations/{id}/cancel [post]
func (h *Handler) cancel(c *gin.Context) {
	h.act(c, h.svc.Cancel)
}

// remind stamps reminded_at on a pending invitation.
//
//	@Summary		Remind an invitee
//	@Description	Owner only; sets reminded_at = now() on a pending invitation. No message is sent.
//	@Tags			class-invitations
//	@Produce		json
//	@Param			id	path		string	true	"invitation id"
//	@Success		200	{object}	response.Envelope{data=InvitationResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}	"not pending"
//	@Security		BearerAuth
//	@Router			/class-invitations/{id}/remind [post]
func (h *Handler) remind(c *gin.Context) {
	h.act(c, h.svc.Remind)
}

// confirm completes the invitation and writes the stint.
//
//	@Summary		Confirm a class invitation (GV nhận lớp)
//	@Description	Owner only; pending | accepted → assigned. A giao_vien invitation is a class handoff (the current teacher's stint closes and future planned sessions move); any other role becomes a staff assignment. Both happen in one transaction with the status change. 409 when the center is busy with another operation, 422 MEMBER_INACTIVE when the invitee has left.
//	@Tags			class-invitations
//	@Produce		json
//	@Param			id	path		string	true	"invitation id"
//	@Success		200	{object}	response.Envelope{data=ConfirmResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/class-invitations/{id}/confirm [post]
func (h *Handler) confirm(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id", "class invitation")
	if !ok {
		return
	}
	out, err := h.svc.Confirm(c.Request.Context(), sc, id)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// act runs one of the single-invitation state changes that share the same
// request shape and response.
func (h *Handler) act(c *gin.Context, fn func(ctx context.Context, sc authctx.Scope, id uuid.UUID) (*InvitationResponse, error)) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	id, ok := pathID(c, "id", "class invitation")
	if !ok {
		return
	}
	out, err := fn(c.Request.Context(), sc, id)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}
