package auth

import (
	"teka/apps/api/internal/features/teachers"
)

// LoginRequest is the payload for POST /auth/login.
type LoginRequest struct {
	Phone    string `json:"phone" binding:"required,vnphone"`
	Password string `json:"password" binding:"required"`
}

// TokenResponse is the body returned by register/login/refresh. The refresh
// token itself travels only in the httpOnly cookie, never in the body.
type TokenResponse struct {
	AccessToken string                   `json:"access_token"`
	TokenType   string                   `json:"token_type"`
	ExpiresIn   int64                    `json:"expires_in"`
	Teacher     teachers.TeacherResponse `json:"teacher"`
}

// NewTokenResponse builds the response body for a session.
func NewTokenResponse(sess *Session, accessTTLSeconds int64) TokenResponse {
	return TokenResponse{
		AccessToken: sess.AccessToken,
		TokenType:   "Bearer",
		ExpiresIn:   accessTTLSeconds,
		Teacher:     teachers.FromModel(&sess.Teacher.Account, &sess.Teacher.Teacher),
	}
}

// ForgotPasswordRequest is the payload for POST /auth/forgot-password.
type ForgotPasswordRequest struct {
	Phone string `json:"phone" binding:"required,vnphone"`
}

// ForgotPasswordResponse is the byte-identical generic body every call
// returns, regardless of whether the phone matched an eligible account — the
// caller must never be able to distinguish "member, reset sent" from
// "unknown phone" or "owner" by diffing responses.
type ForgotPasswordResponse struct {
	Message string `json:"message"`
}

// forgotPasswordResponse is the single package-level value the handler
// always returns, so its identity (not just its contents) is stable across
// every call — tests assert on it with require.Same.
var forgotPasswordResponse = ForgotPasswordResponse{
	Message: "if this phone is registered, a reset link has been sent",
}

// ResetPasswordRequest is the payload for POST /auth/reset-password.
type ResetPasswordRequest struct {
	Token    string `json:"token" binding:"required"`
	Password string `json:"password" binding:"required,min=8,max=72"`
}
