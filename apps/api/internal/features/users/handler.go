package users

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

// listSorts whitelists sortable columns for GET /users.
var listSorts = map[string]string{
	"created_at": "created_at",
	"name":       "name",
	"email":      "email",
}

// Handler binds HTTP requests to the users service; no business logic here.
type Handler struct {
	svc *Service
}

// NewHandler builds the users handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// create adds a user with an explicit role.
//
//	@Summary		Create a user (admin)
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			request	body		CreateRequest	true	"user payload"
//	@Success		201		{object}	response.Envelope{data=Response}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"email already in use"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/users [post]
func (h *Handler) create(c *gin.Context) {
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	u, err := h.svc.Create(c.Request.Context(), req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, FromModel(u))
}

// list returns a paginated, filterable user listing.
//
//	@Summary		List users (admin)
//	@Tags			users
//	@Produce		json
//	@Param			page		query		int		false	"page number"				default(1)
//	@Param			per_page	query		int		false	"items per page (max 100)"	default(20)
//	@Param			sort		query		string	false	"sort column; leading - for descending"	Enums(created_at, -created_at, name, -name, email, -email)
//	@Param			q			query		string	false	"substring match on name or email"
//	@Param			role		query		string	false	"filter by role"	Enums(admin, user)
//	@Success		200			{object}	response.Envelope{data=[]Response,meta=response.Meta}
//	@Failure		401			{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403			{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/users [get]
func (h *Handler) list(c *gin.Context) {
	caller, _ := authctx.From(c)
	params := pagination.Parse(c, "-created_at", listSorts)
	filter := ListFilter{Query: c.Query("q"), Role: c.Query("role")}

	rows, total, err := h.svc.List(c.Request.Context(), caller, filter, params)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.List(c, FromModels(rows), params.Meta(total))
}

// get returns one user (admin or self).
//
//	@Summary		Get a user
//	@Tags			users
//	@Produce		json
//	@Param			id	path		string	true	"user id (UUID)"
//	@Success		200	{object}	response.Envelope{data=Response}
//	@Failure		400	{object}	response.Envelope{error=response.ErrorBody}	"invalid user id"
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/users/{id} [get]
func (h *Handler) get(c *gin.Context) {
	caller, _ := authctx.From(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Err(c, apperror.BadRequest("invalid user id"))
		return
	}
	u, err := h.svc.Get(c.Request.Context(), caller, id)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, FromModel(u))
}

// update partially updates a user (admin or self; role changes admin-only).
//
//	@Summary		Update a user
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"user id (UUID)"
//	@Param			request	body		UpdateRequest	true	"fields to change; omitted fields keep their value"
//	@Success		200		{object}	response.Envelope{data=Response}
//	@Failure		400		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/users/{id} [patch]
func (h *Handler) update(c *gin.Context) {
	caller, _ := authctx.From(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Err(c, apperror.BadRequest("invalid user id"))
		return
	}
	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	u, err := h.svc.Update(c.Request.Context(), caller, id, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, FromModel(u))
}

// remove soft-deletes a user.
//
//	@Summary		Delete a user (admin)
//	@Description	Soft delete; admins cannot delete their own account.
//	@Tags			users
//	@Produce		json
//	@Param			id	path		string	true	"user id (UUID)"
//	@Success		200	{object}	response.Envelope
//	@Failure		400	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}	"admins cannot delete themselves"
//	@Security		BearerAuth
//	@Router			/users/{id} [delete]
func (h *Handler) remove(c *gin.Context) {
	caller, _ := authctx.From(c)
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Err(c, apperror.BadRequest("invalid user id"))
		return
	}
	if err := h.svc.Delete(c.Request.Context(), caller, id); err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, gin.H{"deleted": true})
}
