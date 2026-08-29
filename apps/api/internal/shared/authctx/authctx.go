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
	// CanSendReports is the delegated report-sender permission on the
	// caller's live membership stint. Member-only: the owner never holds it
	// (grant refuses the owner as target), so IsOwner and CanSendReports are
	// mutually exclusive in practice.
	CanSendReports bool
	// Perms is the caller's effective permission set, resolved fresh from
	// the database alongside the rest of the scope. Read-only after
	// SetScope — Scope copies share the map.
	Perms PermSet
}

// ReportsOversight reports whether the caller may read billing
// periods/statements/debt center-wide AND create report sends — the owner or
// a member holding the delegated send-reports permission. Read paths and the
// send-creation gate both branch on this one helper so the two capabilities
// can never drift apart.
func (s Scope) ReportsOversight() bool {
	return s.IsOwner || s.CanSendReports
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
