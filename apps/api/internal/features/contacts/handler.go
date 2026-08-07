package contacts

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

// listSorts whitelists the public sort keys for GET /contacts.
var listSorts = map[string]string{
	"full_name":  "contacts.full_name",
	"created_at": "contacts.created_at",
}

// Handler exposes the contact CRUD endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds the contacts handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// teacherID resolves the authenticated tenant — the only sanctioned source of
// teacher identity; request bodies and paths never carry it.
func (h *Handler) teacherID(c *gin.Context) (uuid.UUID, bool) {
	teacherID, ok := authctx.TeacherID(c)
	if !ok {
		response.Err(c, apperror.Unauthorized("authentication required"))
		return uuid.UUID{}, false
	}
	return teacherID, true
}

// contactID parses the :id path parameter.
func contactID(c *gin.Context) (uuid.UUID, bool) {
	parsed, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Err(c, apperror.NotFound("contact"))
		return uuid.UUID{}, false
	}
	return parsed, true
}

// create registers a new contact.
//
//	@Summary		Create contact
//	@Tags			contacts
//	@Accept			json
//	@Produce		json
//	@Param			request	body		CreateRequest	true	"contact fields"
//	@Success		201		{object}	response.Envelope{data=ContactResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"phone already used"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"validation failed"
//	@Security		BearerAuth
//	@Router			/contacts [post]
func (h *Handler) create(c *gin.Context) {
	teacherID, ok := h.teacherID(c)
	if !ok {
		return
	}
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	row, err := h.svc.Create(c.Request.Context(), teacherID, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, FromModel(row))
}

// list returns a page of contacts with student counts.
//
//	@Summary		List contacts
//	@Description	Paginated; query matches full_name or phone case-insensitively.
//	@Tags			contacts
//	@Produce		json
//	@Param			query		query		string	false	"search full_name or phone"
//	@Param			page		query		int		false	"page number"
//	@Param			per_page	query		int		false	"page size (max 100)"
//	@Param			sort		query		string	false	"full_name or created_at, - prefix for desc"
//	@Success		200			{object}	response.Envelope{data=[]ContactResponse,meta=response.Meta}
//	@Failure		401			{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/contacts [get]
func (h *Handler) list(c *gin.Context) {
	teacherID, ok := h.teacherID(c)
	if !ok {
		return
	}
	params := pagination.Parse(c, "full_name", listSorts)
	rows, total, err := h.svc.List(c.Request.Context(), teacherID, ListFilter{Query: c.Query("query")}, params)
	if err != nil {
		response.Err(c, err)
		return
	}
	out := make([]ContactResponse, 0, len(rows))
	for i := range rows {
		out = append(out, FromModel(&rows[i]))
	}
	response.List(c, out, params.Meta(total))
}

// get returns one contact.
//
//	@Summary		Get contact
//	@Tags			contacts
//	@Produce		json
//	@Param			id	path		string	true	"contact id"
//	@Success		200	{object}	response.Envelope{data=ContactResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/contacts/{id} [get]
func (h *Handler) get(c *gin.Context) {
	teacherID, ok := h.teacherID(c)
	if !ok {
		return
	}
	cid, ok := contactID(c)
	if !ok {
		return
	}
	row, err := h.svc.Get(c.Request.Context(), teacherID, cid)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, FromModel(row))
}

// update replaces the contact's name and phone.
//
//	@Summary		Update contact
//	@Tags			contacts
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"contact id"
//	@Param			request	body		UpdateRequest	true	"contact fields"
//	@Success		200		{object}	response.Envelope{data=ContactResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"phone already used"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"validation failed"
//	@Security		BearerAuth
//	@Router			/contacts/{id} [put]
func (h *Handler) update(c *gin.Context) {
	teacherID, ok := h.teacherID(c)
	if !ok {
		return
	}
	cid, ok := contactID(c)
	if !ok {
		return
	}
	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	row, err := h.svc.Update(c.Request.Context(), teacherID, cid, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, FromModel(row))
}

// remove soft-deletes a contact with no live students.
//
//	@Summary		Delete contact
//	@Description	Soft delete; blocked with 409 while live students still reference the contact.
//	@Tags			contacts
//	@Produce		json
//	@Param			id	path	string	true	"contact id"
//	@Success		204
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409	{object}	response.Envelope{error=response.ErrorBody}	"students still reference the contact"
//	@Security		BearerAuth
//	@Router			/contacts/{id} [delete]
func (h *Handler) remove(c *gin.Context) {
	teacherID, ok := h.teacherID(c)
	if !ok {
		return
	}
	cid, ok := contactID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), teacherID, cid); err != nil {
		response.Err(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// setZaloMapping binds the contact to a Zalo friend chosen in the picker.
//
//	@Summary		Map contact to a Zalo friend
//	@Description	Stores the picked friend's id and display name on the contact. Values come from GET /me/zalo/friends; the backend does not re-check them against the live friend list.
//	@Tags			contacts
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string				true	"contact id"
//	@Param			request	body		ZaloMappingRequest	true	"picked Zalo friend"
//	@Success		200		{object}	response.Envelope{data=ContactResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"friend already mapped to another contact"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"validation failed"
//	@Security		BearerAuth
//	@Router			/contacts/{id}/zalo-mapping [put]
func (h *Handler) setZaloMapping(c *gin.Context) {
	teacherID, ok := h.teacherID(c)
	if !ok {
		return
	}
	cid, ok := contactID(c)
	if !ok {
		return
	}
	var req ZaloMappingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	row, err := h.svc.UpdateZaloMapping(c.Request.Context(), teacherID, cid, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, FromModel(row))
}

// clearZaloMapping detaches the contact from its Zalo friend.
//
//	@Summary		Unmap contact from its Zalo friend
//	@Description	Nulls both mapping fields. Idempotent: unmapping an unmapped contact is still 204.
//	@Tags			contacts
//	@Produce		json
//	@Param			id	path	string	true	"contact id"
//	@Success		204
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/contacts/{id}/zalo-mapping [delete]
func (h *Handler) clearZaloMapping(c *gin.Context) {
	teacherID, ok := h.teacherID(c)
	if !ok {
		return
	}
	cid, ok := contactID(c)
	if !ok {
		return
	}
	if err := h.svc.ClearZaloMapping(c.Request.Context(), teacherID, cid); err != nil {
		response.Err(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
