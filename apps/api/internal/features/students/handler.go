package students

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

// listSorts whitelists the public sort keys for GET /students.
var listSorts = map[string]string{
	"full_name":  "students.full_name",
	"created_at": "students.created_at",
}

// Handler exposes the student endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds the students handler.
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

// studentID parses the :id path parameter, reading a malformed value as the
// student not existing.
func studentID(c *gin.Context) (uuid.UUID, bool) {
	parsed, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Err(c, apperror.NotFound("student"))
		return uuid.UUID{}, false
	}
	return parsed, true
}

// queryUUID parses an optional uuid query filter; a malformed value is a
// validation error, not an empty result.
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

// create registers a student under one of the teacher's contacts.
//
//	@Summary		Create student
//	@Description	The field list is closed by design: full name, contact, and the attendance disambiguator.
//	@Tags			students
//	@Accept			json
//	@Produce		json
//	@Param			request	body		CreateRequest	true	"student fields"
//	@Success		201		{object}	response.Envelope{data=StudentResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"validation failed, including a contact that is not yours"
//	@Security		BearerAuth
//	@Router			/students [post]
func (h *Handler) create(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	var req CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	row, err := h.svc.Create(c.Request.Context(), sc, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusCreated, FromRow(row))
}

// list returns a page of students with contact details.
//
//	@Summary		List students
//	@Description	class_id filters through open enrollments — the attendance screen's class roster.
//	@Tags			students
//	@Produce		json
//	@Param			query		query		string	false	"matches the student name"
//	@Param			contact_id	query		string	false	"only students under this contact"
//	@Param			class_id	query		string	false	"only students with an open enrollment in this class"
//	@Param			unenrolled	query		bool	false	"only students with no open enrollment in any class"
//	@Param			page		query		int		false	"page number"
//	@Param			per_page	query		int		false	"page size (max 100)"
//	@Param			sort		query		string	false	"full_name or created_at; - prefix for desc"
//	@Success		200			{object}	response.Envelope{data=[]StudentResponse,meta=response.Meta}
//	@Failure		401			{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422			{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/students [get]
func (h *Handler) list(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	contactID, ok := queryUUID(c, "contact_id")
	if !ok {
		return
	}
	classID, ok := queryUUID(c, "class_id")
	if !ok {
		return
	}
	filter := ListFilter{
		Query:      c.Query("query"),
		ContactID:  contactID,
		ClassID:    classID,
		Unenrolled: c.Query("unenrolled") == "true",
	}
	params := pagination.Parse(c, "full_name", listSorts)
	rows, total, err := h.svc.List(c.Request.Context(), sc, filter, params)
	if err != nil {
		response.Err(c, err)
		return
	}
	out := make([]StudentResponse, 0, len(rows))
	for i := range rows {
		out = append(out, FromRow(&rows[i]))
	}
	response.List(c, out, params.Meta(total))
}

// get returns one student.
//
//	@Summary		Get student
//	@Tags			students
//	@Produce		json
//	@Param			id	path		string	true	"student id"
//	@Success		200	{object}	response.Envelope{data=StudentResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/students/{id} [get]
func (h *Handler) get(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	sid, ok := studentID(c)
	if !ok {
		return
	}
	row, err := h.svc.Get(c.Request.Context(), sc, sid)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, FromRow(row))
}

// update edits the student's closed field list.
//
//	@Summary		Update student
//	@Tags			students
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"student id"
//	@Param			request	body		UpdateRequest	true	"student fields"
//	@Success		200		{object}	response.Envelope{data=StudentResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/students/{id} [put]
func (h *Handler) update(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	sid, ok := studentID(c)
	if !ok {
		return
	}
	var req UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	row, err := h.svc.Update(c.Request.Context(), sc, sid, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, FromRow(row))
}

// remove anonymises and hides the student; financial records keep their
// snapshot of the name.
//
//	@Summary		Delete student
//	@Description	Scrubs the student's personal data and ends open enrollments; invoices keep their name snapshot.
//	@Tags			students
//	@Produce		json
//	@Param			id	path	string	true	"student id"
//	@Success		204
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/students/{id} [delete]
func (h *Handler) remove(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	sid, ok := studentID(c)
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), sc, sid); err != nil {
		response.Err(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
