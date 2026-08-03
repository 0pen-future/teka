package auth

import (
	"teka/apps/api/internal/features/users"
)

// RegisterRequest is the payload for POST /auth/register.
type RegisterRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8,max=72"`
	Name     string `json:"name" binding:"required,min=1,max=255"`
}

// LoginRequest is the payload for POST /auth/login.
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// TokenResponse is the body returned by register/login/refresh. The refresh
// token itself travels only in the httpOnly cookie, never in the body.
type TokenResponse struct {
	AccessToken string         `json:"access_token"`
	TokenType   string         `json:"token_type"`
	ExpiresIn   int64          `json:"expires_in"`
	User        users.Response `json:"user"`
}

// NewTokenResponse builds the response body for a session.
func NewTokenResponse(sess *Session, accessTTLSeconds int64) TokenResponse {
	return TokenResponse{
		AccessToken: sess.AccessToken,
		TokenType:   "Bearer",
		ExpiresIn:   accessTTLSeconds,
		User:        users.FromModel(sess.User),
	}
}
