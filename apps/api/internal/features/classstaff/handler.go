package classstaff

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// Handler exposes the class-staff endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds the classstaff handler.
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

// list returns the class's staff, active and ended stints.
//
//	@Summary		List class staff
//	@Description	Every staff stint of the class (nhân sự lớp) — active and ended — with teacher names and role labels. Visible to the owner and to anyone holding a stint on the class (an ended stint keeps history reads). Anyone else gets 404.
//	@Tags			class-staff
//	@Produce		json
//	@Param			id	path		string	true	"class id"
//	@Success		200	{object}	response.Envelope{data=[]StaffResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}	"class invisible to the caller"
//	@Security		BearerAuth
//	@Router			/classes/{id}/staff [get]
func (h *Handler) list(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	out, err := h.svc.List(c.Request.Context(), sc, classID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, out)
}

// assign gives a live member a role in the class (owner only).
//
//	@Summary		Assign class staff
//	@Description	Assigns a live center member a role in the class. Owner only. giao_vien is refused with 409 — the primary teacher changes only through the class handoff flow. A person holds at most one active stint per class (409 otherwise); the role must come from the code-owned registry (422); the target must be a live member (400).
//	@Tags			class-staff
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"class id"
//	@Param			request	body		AssignRequest	true	"member and role"
//	@Success		201		{object}	response.Envelope{data=StaffResponse}
//	@Failure		400		{object}	response.Envelope{error=response.ErrorBody}	"target is not a live member of the center"
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"caller reads the class but is not the owner"
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}	"class invisible to the caller"
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"giao_vien role, or an active stint already exists"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"role outside the registry"
//	@Security		BearerAuth
//	@Router			/classes/{id}/staff [post]
func (h *Handler) assign(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	var req AssignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	out, err := h.svc.Assign(c.Request.Context(), sc, classID, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, out)
}

// remove ends a stint (default) or revokes it entirely (mode=void).
//
//	@Summary		Remove class staff
//	@Description	Default: soft-closes the stint — the person keeps read access to the class's history. mode=void hard-deletes the row (the revocation path for a mistaken grant, valid on an already-ended stint too). Owner only. An active giao_vien refuses both modes with 409 — hand the class off instead. Soft-closing an already-ended stint is 404.
//	@Tags			class-staff
//	@Produce		json
//	@Param			id		path		string	true	"class id"
//	@Param			staffId	path		string	true	"staff assignment id"
//	@Param			mode	query		string	false	"void to hard-delete the stint"	Enums(void)
//	@Success		204		{object}	nil
//	@Failure		400		{object}	response.Envelope{error=response.ErrorBody}	"unknown mode"
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"caller reads the class but is not the owner"
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}	"class or stint invisible, or stint already ended"
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"active giao_vien — use the handoff flow"
//	@Security		BearerAuth
//	@Router			/classes/{id}/staff/{staffId} [delete]
func (h *Handler) remove(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, ok := pathID(c, "id", "class")
	if !ok {
		return
	}
	staffID, ok := pathID(c, "staffId", "staff assignment")
	if !ok {
		return
	}
	mode := c.Query("mode")
	if mode != "" && mode != "void" {
		response.Err(c, apperror.BadRequest("mode không hợp lệ — chỉ hỗ trợ mode=void"))
		return
	}
	if err := h.svc.Remove(c.Request.Context(), sc, classID, staffID, mode == "void"); err != nil {
		response.Err(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
