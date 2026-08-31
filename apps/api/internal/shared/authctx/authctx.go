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
	// CanSendReports mirrors the caller's effective reports.send permission
	// (role grant or member override, minus denies). It exists as a resolved
	// field — not a Has() call at the check site — because the notifications
	// run manager snapshots it at send creation and re-probes it per item.
	// The owner never carries it (they sit outside the role tables); their
	// authority flows through ReportsOversight's IsOwner arm.
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

// PhoneVisible is the single phone-privacy rule for every surface that could
// carry a contact's phone: the owner, reports oversight, and the explicit
// contacts.view_all grant always see it; anyone else only when the row itself
// grants it (rowVisible — a repo-derived phone_visible column, true when the
// caller holds an ACTIVE hoc_vu stint on a class where one of the contact's
// students is actively enrolled). contacts.view_all sits here rather than in
// per-surface checks because a contact row IS its phone — granting "Xem mọi
// liên hệ" without the phones would be an empty grant. Services null the phone
// field when this returns false; there is deliberately no per-surface
// variation, so the same contact can never show a phone in one list and hide
// it in another.
func (s Scope) PhoneVisible(rowVisible bool) bool {
	return s.ReportsOversight() || s.CenterWideFor(PermContactsViewAll) || rowVisible
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
