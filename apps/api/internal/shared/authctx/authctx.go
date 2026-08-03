// Package authctx carries the authenticated principal between the auth
// middleware and feature handlers without coupling them to each other.
package authctx

import (
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// AccessClaims is the access-token payload shared by the issuer
// (features/auth) and the verifier (middleware): sub = user id, role custom
// claim.
type AccessClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// Roles a principal can hold; mirrored by the users table CHECK constraint.
const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

// Principal is the authenticated caller extracted from a verified access
// token.
type Principal struct {
	UserID uuid.UUID
	Role   string
}

// IsAdmin reports whether the caller holds the admin role.
func (p Principal) IsAdmin() bool { return p.Role == RoleAdmin }

const ginKey = "auth_principal"

// Set attaches the principal to the request context.
func Set(c *gin.Context, p Principal) {
	c.Set(ginKey, p)
}

// From returns the principal set by the auth middleware; ok is false on
// unauthenticated routes.
func From(c *gin.Context) (Principal, bool) {
	v, exists := c.Get(ginKey)
	if !exists {
		return Principal{}, false
	}
	p, ok := v.(Principal)
	return p, ok
}
