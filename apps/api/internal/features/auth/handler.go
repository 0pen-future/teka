package auth

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/features/users"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

const (
	refreshCookieName = "refresh_token"
	// refreshCookiePath scopes the cookie to the auth endpoints so it is not
	// sent on every API request.
	refreshCookiePath = "/api/v1/auth"
)

// Handler exposes the auth endpoints.
type Handler struct {
	svc *Service
	cfg *config.Config
}

// NewHandler builds the auth handler.
func NewHandler(svc *Service, cfg *config.Config) *Handler {
	return &Handler{svc: svc, cfg: cfg}
}

func (h *Handler) register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	sess, err := h.svc.Register(c.Request.Context(), req)
	if err != nil {
		response.Err(c, err)
		return
	}
	h.setRefreshCookie(c, sess.RefreshToken)
	response.OK(c, http.StatusCreated, h.tokenResponse(sess))
}

func (h *Handler) login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Err(c, validation.BindError(err))
		return
	}
	sess, err := h.svc.Login(c.Request.Context(), req)
	if err != nil {
		response.Err(c, err)
		return
	}
	h.setRefreshCookie(c, sess.RefreshToken)
	response.OK(c, http.StatusOK, h.tokenResponse(sess))
}

func (h *Handler) refresh(c *gin.Context) {
	plaintext, err := c.Cookie(refreshCookieName)
	if err != nil || plaintext == "" {
		response.Err(c, apperror.Unauthorized("missing refresh token"))
		return
	}
	sess, err := h.svc.Refresh(c.Request.Context(), plaintext)
	if err != nil {
		// Only a rejected token clears the cookie; a transient 500 must not
		// log the user out.
		var appErr *apperror.AppError
		if errors.As(err, &appErr) && appErr.Code == apperror.CodeUnauthorized {
			h.clearRefreshCookie(c)
		}
		response.Err(c, err)
		return
	}
	h.setRefreshCookie(c, sess.RefreshToken)
	response.OK(c, http.StatusOK, h.tokenResponse(sess))
}

func (h *Handler) logout(c *gin.Context) {
	plaintext, _ := c.Cookie(refreshCookieName)
	if err := h.svc.Logout(c.Request.Context(), plaintext); err != nil {
		response.Err(c, err)
		return
	}
	h.clearRefreshCookie(c)
	response.OK(c, http.StatusOK, gin.H{"message": "logged out"})
}

func (h *Handler) me(c *gin.Context) {
	p, ok := authctx.From(c)
	if !ok {
		response.Err(c, apperror.Unauthorized("authentication required"))
		return
	}
	u, err := h.svc.Me(c.Request.Context(), p.UserID)
	if err != nil {
		response.Err(c, err)
		return
	}
	response.OK(c, http.StatusOK, users.FromModel(u))
}

func (h *Handler) tokenResponse(sess *Session) TokenResponse {
	return NewTokenResponse(sess, int64(h.svc.issuer.AccessTTL().Seconds()))
}

// setRefreshCookie stores the refresh token in an httpOnly SameSite=Lax
// cookie. Secure is enabled in production only: Safari refuses Secure cookies
// on http://localhost, which would break local development.
func (h *Handler) setRefreshCookie(c *gin.Context, plaintext string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(
		refreshCookieName,
		plaintext,
		int(h.svc.issuer.RefreshTTL().Seconds()),
		refreshCookiePath,
		"",
		h.cfg.IsProduction(),
		true,
	)
}

func (h *Handler) clearRefreshCookie(c *gin.Context) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(refreshCookieName, "", -1, refreshCookiePath, "", h.cfg.IsProduction(), true)
}
