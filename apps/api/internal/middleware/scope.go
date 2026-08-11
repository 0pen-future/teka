package middleware

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/response"
)

// ScopeResolver resolves a teacher's center scope from the database
// (consumer-defined interface; implemented by *centers.Service).
type ScopeResolver interface {
	ResolveScope(ctx context.Context, teacherID uuid.UUID) (authctx.Scope, error)
}

// ResolveScope loads the caller's center scope fresh on every request and
// injects it after RequireAuth. Membership deliberately lives in the database
// and not in JWT claims: a kicked or departed member loses (and a joiner
// gains) their center on the very next request, not at token expiry.
func ResolveScope(resolver ScopeResolver) gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := authctx.From(c)
		if !ok || p.Role != authctx.RoleTeacher {
			response.Err(c, apperror.Unauthorized("authentication required"))
			return
		}
		scope, err := resolver.ResolveScope(c.Request.Context(), p.UserID)
		if err != nil {
			response.Err(c, err)
			return
		}
		authctx.SetScope(c, scope)
		c.Next()
	}
}
