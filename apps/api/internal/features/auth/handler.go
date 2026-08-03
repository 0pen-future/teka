package auth

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/shared/apperror"
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

// register creates a teacher account and opens a session.
//
//	@Summary		Register a new teacher
//	@Description	Creates a teacher account from a Vietnamese phone number (0xxxxxxxxx or +84xxxxxxxxx) and returns an access token; the refresh token is set as an httpOnly cookie.
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		RegisterRequest	true	"registration payload"
//	@Success		201		{object}	response.Envelope{data=TokenResponse}
//	@Failure		409		{object}	response.Envelope{error=response.ErrorBody}	"phone already registered"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}	"validation failed"
//	@Router			/auth/register [post]
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

// login verifies credentials and opens a session.
//
//	@Summary		Log in
//	@Description	Verifies phone and password; returns an access token and sets the refresh cookie (new token family).
//	@Tags			auth
//	@Accept			json
//	@Produce		json
//	@Param			request	body		LoginRequest	true	"credentials"
//	@Success		200		{object}	response.Envelope{data=TokenResponse}
//	@Failure		401		{object}	response.Envelope{error=response.ErrorBody}	"invalid phone or password"
//	@Failure		422		{object}	response.Envelope{error=response.ErrorBody}
//	@Router			/auth/login [post]
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

// refresh rotates the refresh token from the cookie.
//
//	@Summary		Refresh the session
//	@Description	Rotates the refresh-token cookie and returns a fresh access token. Replaying an already-rotated token revokes the whole token family.
//	@Tags			auth
//	@Produce		json
//	@Success		200	{object}	response.Envelope{data=TokenResponse}
//	@Failure		401	{object}	response.Envelope{error=response.ErrorBody}	"missing, expired, or revoked refresh token"
//	@Router			/auth/refresh [post]
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

// logout revokes the session's token family.
//
//	@Summary		Log out
//	@Description	Revokes the refresh-token family and clears the cookie. Idempotent: succeeds without a valid cookie.
//	@Tags			auth
//	@Produce		json
//	@Success		200	{object}	response.Envelope
//	@Router			/auth/logout [post]
func (h *Handler) logout(c *gin.Context) {
	plaintext, _ := c.Cookie(refreshCookieName)
	if err := h.svc.Logout(c.Request.Context(), plaintext); err != nil {
		response.Err(c, err)
		return
	}
	h.clearRefreshCookie(c)
	response.OK(c, http.StatusOK, gin.H{"message": "logged out"})
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
