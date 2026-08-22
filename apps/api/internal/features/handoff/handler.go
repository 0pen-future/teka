package handoff

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// Handler binds HTTP to the handoff service.
type Handler struct {
	svc *Service
}

// NewHandler builds the handoff handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// scope resolves the authenticated caller's center scope — the only sanctioned
// source of tenant identity; the request body carries only the target teacher.
func (h *Handler) scope(c *gin.Context) (authctx.Scope, bool) {
	sc, ok := authctx.ScopeFrom(c)
	if !ok {
		response.Err(c, apperror.Unauthorized("authentication required"))
		return authctx.Scope{}, false
	}
	return sc, true
}

// reassign hands a class to another teacher in the same center.
//
//	@Summary		Bàn giao lớp cho giáo viên khác
//	@Description	Chuyển lớp, lịch học và các buổi đã lên lịch từ hôm nay trở đi sang giáo viên mới. Buổi đã dạy, buổi đã huỷ, điểm danh và học phí giữ nguyên. Chỉ chủ trung tâm được gọi; giáo viên nhận phải thuộc trung tâm.
//	@Tags			classes
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string			true	"class id"
//	@Param			request	body		ReassignRequest	true	"giáo viên nhận lớp"
//	@Success		200		{object}	response.Envelope{data=ReassignResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		403		{object}	response.Envelope{error=response.ErrorBody}	"chỉ chủ trung tâm"
//	@Failure		404		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"giáo viên không thuộc trung tâm"
//	@Security		BearerAuth
//	@Router			/classes/{id}/teacher [put]
func (h *Handler) reassign(c *gin.Context) {
	sc, ok := h.scope(c)
	if !ok {
		return
	}
	classID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		response.Err(c, apperror.NotFound("class"))
		return
	}
	var req ReassignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	result, err := h.svc.Reassign(c.Request.Context(), sc, classID, *req.TeacherID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, fromResult(result))
}
