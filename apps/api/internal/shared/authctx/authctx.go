// Package authctx carries the authenticated principal between the auth
// middleware and feature handlers without coupling them to each other.
package authctx

import (
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// AccessClaims is the access-token payload shared by the issuer
// (features/auth) and the verifier (middleware): sub = account id, role
// custom claim.
type AccessClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// Roles a principal can hold; mirrored by the user_accounts.role CHECK
// constraint. V1 only issues teacher accounts; parent and student exist in
// the schema for later phases.
const (
	RoleTeacher = "teachers"
	RoleParent  = "parent"
	RoleStudent = "students"
)

// Principal is the authenticated caller extracted from a verified access
// token.
type Principal struct {
	UserID uuid.UUID
	Role   string
}

// Scope is the caller's full tenant context: the teacher, the center their
// requests operate in, and whether they own it. It is resolved from the
// database on every request and never cached in the JWT, so a membership
// change (kick, leave, join) takes effect on the very next request.
type Scope struct {
	TeacherID uuid.UUID
	CenterID  uuid.UUID
	IsOwner   bool
}

const (
	ginKey   = "auth_principal"
	scopeKey = "auth_scope"
)

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

// SetScope attaches the resolved center scope to the request context.
func SetScope(c *gin.Context, s Scope) {
	c.Set(scopeKey, s)
}

// ScopeFrom returns the scope set by the scope-resolution middleware; ok is
// false on routes mounted without it.
func ScopeFrom(c *gin.Context) (Scope, bool) {
	v, exists := c.Get(scopeKey)
	if !exists {
		return Scope{}, false
	}
	s, ok := v.(Scope)
	return s, ok
}

// TeacherID returns the tenant id for the authenticated teacher. This is the
// only sanctioned source of teacher_id for scoping queries — accepting one
// from a request body, query, or path would be an authorization bypass. ok is
// false when unauthenticated or when the caller is not a teacher.
func TeacherID(c *gin.Context) (uuid.UUID, bool) {
	p, ok := From(c)
	if !ok || p.Role != RoleTeacher {
		return uuid.Nil, false
	}
	return p.UserID, true
}
