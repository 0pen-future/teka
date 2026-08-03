package middleware

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/response"
)

// RequireAuth verifies the Bearer access token (HS256) and injects the
// principal into the request context. Token issuing lives in features/auth;
// only verification is shared infrastructure.
func RequireAuth(cfg config.JWTConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		raw, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || raw == "" {
			response.Err(c, apperror.Unauthorized("missing bearer token"))
			return
		}

		claims := &authctx.AccessClaims{}
		token, err := jwt.ParseWithClaims(raw, claims, func(_ *jwt.Token) (any, error) {
			return []byte(cfg.Secret), nil
		}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
		if err != nil || !token.Valid {
			response.Err(c, apperror.Unauthorized("invalid or expired token"))
			return
		}

		userID, err := uuid.Parse(claims.Subject)
		if err != nil {
			response.Err(c, apperror.Unauthorized("invalid token subject"))
			return
		}
		if claims.Role == "" {
			response.Err(c, apperror.Unauthorized("invalid token claims"))
			return
		}

		authctx.Set(c, authctx.Principal{UserID: userID, Role: claims.Role})
		c.Next()
	}
}

// RequireRole rejects principals lacking the given role; mount after
// RequireAuth.
func RequireRole(role string) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := authctx.From(c)
		if !ok {
			response.Err(c, apperror.Unauthorized("authentication required"))
			return
		}
		if p.Role != role {
			response.Err(c, apperror.Forbidden("insufficient permissions"))
			return
		}
		c.Next()
	}
}
