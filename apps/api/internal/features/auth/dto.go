package auth

import (
	"teka/apps/api/internal/features/teachers"
)

// RegisterRequest is the payload for POST /auth/register. Phone accepts
// local (0xxxxxxxxx) or E.164 (+84xxxxxxxxx) form; it is normalized to E.164
// before storage.
type RegisterRequest struct {
	Phone    string `json:"phone" binding:"required,vnphone"`
	Password string `json:"password" binding:"required,min=8,max=72"`
	FullName string `json:"full_name" binding:"required,min=1,max=100"`
}

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
