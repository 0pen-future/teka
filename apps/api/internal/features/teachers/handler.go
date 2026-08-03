package teachers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// Handler exposes the teacher profile endpoints.
type Handler struct {
	svc *Service
}

// NewHandler builds the teachers handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// me returns the authenticated teacher's profile.
//
//	@Summary		Current teacher profile
//	@Tags			teachers
//	@Produce		json
//	@Success		200	{object}	response.Envelope{data=TeacherResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}
//	@Security		BearerAuth
//	@Router			/me [get]
func (h *Handler) me(c *gin.Context) {
	p, ok := h.currentProfile(c)
	if !ok {
		return
	}
	response.OK(c, http.StatusOK, FromModel(&p.Account, &p.Teacher))
}

// updateMe updates the authenticated teacher's display name and timezone.
//
//	@Summary		Update current teacher profile
//	@Description	Updates full_name and timezone only; other fields are ignored.
//	@Tags			teachers
//	@Accept			json
//	@Produce		json
//	@Param			request	body		UpdateProfileRequest	true	"profile fields"
//	@Success		200		{object}	response.Envelope{data=TeacherResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"validation failed"
//	@Security		BearerAuth
//	@Router			/me [put]
func (h *Handler) updateMe(c *gin.Context) {
	p, ok := h.currentProfile(c)
	if !ok {
		return
	}
	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	updated, err := h.svc.UpdateProfile(c.Request.Context(), p.Account.ID, req)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, FromModel(&updated.Account, &updated.Teacher))
}

// currentProfile resolves the authenticated teacher and enforces that the
// account is still live: an access token issued before a soft-delete or
// disable is cryptographically valid but must stop working here.
func (h *Handler) currentProfile(c *gin.Context) (*Profile, bool) {
	unauthorized := apperror.Unauthorized("authentication required")
	teacherID, ok := authctx.TeacherID(c)
	if !ok {
		response.Err(c, unauthorized)
		return nil, false
	}
	p, err := h.svc.GetByID(c.Request.Context(), teacherID)
	if err != nil {
		if apperror.From(err).Code == apperror.CodeNotFound {
			response.Err(c, unauthorized)
		} else {
			response.Err(c, err)
		}
		return nil, false
	}
	if p.Account.Status != StatusActive {
		response.Err(c, unauthorized)
		return nil, false
	}
	return p, true
}
