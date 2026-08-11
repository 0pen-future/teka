package centers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// Handler exposes the center membership endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds the centers handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// scope resolves the caller's center scope set by the ResolveScope
// middleware — the only sanctioned source of center identity; request bodies
// and paths never carry it.
func (h *Handler) scope(c *gin.Context) (authctx.Scope, bool) {
	s, ok := authctx.ScopeFrom(c)
	if !ok {
		response.Err(c, apperror.Unauthorized("authentication required"))
		return authctx.Scope{}, false
	}
	return s, true
}

// me returns the caller's center and member roster.
//
//	@Summary		Get my center
//	@Tags			centers
//	@Produce		json
//	@Success		200	{object}	response.Envelope{data=MeResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/centers/me [get]
func (h *Handler) me(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	resp, err := h.svc.Me(c.Request.Context(), scope)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, resp)
}

// rename changes the center's name; owner only.
//
//	@Summary		Rename my center
//	@Tags			centers
//	@Accept			json
//	@Produce		json
//	@Param			request	body		RenameRequest	true	"new name"
//	@Success		200		{object}	response.Envelope{data=MeResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"not the owner"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"validation failed"
//	@Security		BearerAuth
//	@Router			/centers/me [patch]
func (h *Handler) rename(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var req RenameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	if err := h.svc.Rename(c.Request.Context(), scope, req); err != nil {
		response.Err(c, err)
		return
	}
	resp, err := h.svc.Me(c.Request.Context(), scope)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, resp)
}

// join moves the caller into the center owned by the phone's teacher.
//
//	@Summary		Join a center by its owner's phone
//	@Tags			centers
//	@Accept			json
//	@Produce		json
//	@Param			request	body		JoinRequest	true	"owner phone"
//	@Success		201		{object}	response.Envelope{data=JoinResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}	"no joinable center behind that phone"
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"current center not empty, or already a member"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"validation failed or joining own center"
//	@Security		BearerAuth
//	@Router			/centers/join [post]
func (h *Handler) join(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	var req JoinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	resp, err := h.svc.Join(c.Request.Context(), scope, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, resp)
}

// removeMember ends a membership: owner removes a member, or a member leaves.
//
//	@Summary		Remove a member (or leave the center)
//	@Tags			centers
//	@Produce		json
//	@Param			teacherId	path	string	true	"teacher id"	format(uuid)
//	@Success		204
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}	"neither owner nor self"
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}	"not a member of this center"
//	@Failure		422	{object}	response.Envelope{error=response.ErrorBody}	"owner cannot leave"
//	@Security		BearerAuth
//	@Router			/centers/me/members/{teacherId} [delete]
func (h *Handler) removeMember(c *gin.Context) {
	scope, ok := h.scope(c)
	if !ok {
		return
	}
	targetID, err := uuid.Parse(c.Param("teacherId"))
	if err != nil {
		response.Err(c, apperror.NotFound("member"))
		return
	}
	if err := h.svc.RemoveMember(c.Request.Context(), scope, targetID); err != nil {
		response.Err(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
