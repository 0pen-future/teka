package enrollments

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/pagination"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// listSorts whitelists the public sort keys for GET /enrollments.
var listSorts = map[string]string{
	"started_on": "enrollments.started_on",
	"created_at": "enrollments.created_at",
}

// Handler exposes the enrollment endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds the enrollments handler.
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

// create enrolls a student in a class.
//
//	@Summary		Enroll student
//	@Description	unit_price is copied from the class's default_unit_price server-side and cannot be supplied. started_on defaults to today and is stored verbatim — mid-cycle joins are expected.
//	@Tags			enrollments
//	@Accept			json
//	@Produce		json
//	@Param			request	body		CreateRequest	true	"student, class, and optional start date"
//	@Success		201		{object}	response.Envelope{data=EnrollmentResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"already enrolled"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"validation failed"
//	@Security		BearerAuth
//	@Router			/enrollments [post]
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
	response.OK(c, http.StatusCreated, FromRow(row))
}

// list returns a page of enrollments.
//
//	@Summary		List enrollments
//	@Produce		json
//	@Tags			enrollments
//	@Param			student_id	query		string	false	"filter by student"
//	@Param			class_id	query		string	false	"filter by class"
//	@Param			active		query		string	false	"true = open only, false = ended only"
//	@Param			page		query		int		false	"page number"
//	@Param			per_page	query		int		false	"page size (max 100)"
//	@Param			sort		query		string	false	"started_on or created_at; - prefix for desc"
//	@Success		200			{object}	response.Envelope{data=[]EnrollmentResponse,meta=response.Meta}
//	@Failure		401			{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422			{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/enrollments [get]
func (h *Handler) list(c *gin.Context) {
	teacherID, ok := h.teacherID(c)
	if !ok {
		return
	}
	filter := ListFilter{}
	if filter.StudentID, ok = queryUUID(c, "student_id"); !ok {
		return
	}
	if filter.ClassID, ok = queryUUID(c, "class_id"); !ok {
		return
	}
	switch c.Query("active") {
	case "":
	case "true":
		active := true
		filter.Active = &active
	case "false":
		active := false
		filter.Active = &active
	default:
		response.Err(c, apperror.Invalid("validation failed",
			map[string]string{"active": "must be true or false"}))
		return
	}
	params := pagination.Parse(c, "-started_on", listSorts)
	rows, total, err := h.svc.List(c.Request.Context(), teacherID, filter, params)
	if err != nil {
		response.Err(c, err)
		return
	}
	out := make([]EnrollmentResponse, 0, len(rows))
	for i := range rows {
		out = append(out, FromRow(&rows[i]))
	}
	response.List(c, out, params.Meta(total))
}

// get returns one enrollment; ended enrollments remain retrievable.
//
//	@Summary		Get enrollment
//	@Tags			enrollments
//	@Produce		json
//	@Param			id	path		string	true	"enrollment id"
//	@Success		200	{object}	response.Envelope{data=EnrollmentResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/enrollments/{id} [get]
func (h *Handler) get(c *gin.Context) {
	teacherID, ok := h.teacherID(c)
	if !ok {
		return
	}
	enrollmentID, ok := pathID(c, "id", "enrollment")
	if !ok {
		return
	}
	row, err := h.svc.Get(c.Request.Context(), teacherID, enrollmentID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, FromRow(row))
}

// end closes an enrollment — the student leaves, the history stays.
//
//	@Summary		End enrollment
//	@Description	ended_on defaults to today. Ending an already-ended enrollment returns 409 so a double-submit cannot move the departure date.
//	@Tags			enrollments
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string		true	"enrollment id"
//	@Param			request	body		EndRequest	false	"optional end date"
//	@Success		200		{object}	response.Envelope{data=EnrollmentResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"already ended"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"ended_on before started_on"
//	@Security		BearerAuth
//	@Router			/enrollments/{id}/end [post]
func (h *Handler) end(c *gin.Context) {
	teacherID, ok := h.teacherID(c)
	if !ok {
		return
	}
	enrollmentID, ok := pathID(c, "id", "enrollment")
	if !ok {
		return
	}
	req := EndRequest{}
	// The body is optional (ended_on defaults to today), but when present it
	// must win. Gating on ContentLength drops a chunked-encoded body — where
	// ContentLength is -1 — and silently reverts the departure date to today,
	// the exact move the double-end 409 exists to prevent. Bind unconditionally
	// and forgive only an empty body.
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		response.Err(c, validation.BindError(err))
		return
	}
	row, err := h.svc.End(c.Request.Context(), teacherID, enrollmentID, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, FromRow(row))
}

// remove soft-deletes an enrollment created by mistake.
//
//	@Summary		Delete enrollment
//	@Description	For enrollments created by mistake. A student leaving is an end, not a delete.
//	@Tags			enrollments
//	@Param			id	path	string	true	"enrollment id"
//	@Success		204
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		404	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/enrollments/{id} [delete]
func (h *Handler) remove(c *gin.Context) {
	teacherID, ok := h.teacherID(c)
	if !ok {
		return
	}
	enrollmentID, ok := pathID(c, "id", "enrollment")
	if !ok {
		return
	}
	if err := h.svc.Delete(c.Request.Context(), teacherID, enrollmentID); err != nil {
		response.Err(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
