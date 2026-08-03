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
