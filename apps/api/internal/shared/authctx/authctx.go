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

// ReportsOversight reports whether the caller may CREATE a report send — bulk
// send, send preview, resume, the class-send gate's oversight arm, and the
// zalo-mapping rewrite that redirects where a family's messages land: the
// owner, or a member holding the delegated reports.send permission. It is no
// longer a read gate: reading the billing/statements/notifications/contacts
// data a send touches goes through CenterWideFor(<resource>.view_all)
// instead, because reports.send implies those four view_all keys (see
// impliedKeys in catalog.go) — a send-reports holder already reads
// everything a send needs, without this helper's involvement. Keeping the
// send-creation gate on this single helper, separate from the read keys it
// implies, is what lets a role hold billing.view_all/statements.view_all/
// notifications.view_all/contacts.view_all for reading without ever gaining
// the ability to send.
func (s Scope) ReportsOversight() bool {
	return s.IsOwner || s.CanSendReports
}

// PhoneVisible is the single phone-privacy rule for every surface that could
// carry a contact's phone: whoever reads contacts center-wide sees it (the
// owner, an explicit contacts.view_all grant, or reports.send through the
// keys it implies); anyone else only when the row itself is visible to them
// (their own contact, or a student they are assigned to as hoc_vu). Surfaces
// pass their own row-visibility verdict in; nothing else may decide phone
// privacy on its own.
func (s Scope) PhoneVisible(rowVisible bool) bool {
	return s.CenterWideFor(PermContactsViewAll) || rowVisible
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

// Anchor names whose rows a service acts on: the teacher that owns them and
// the center they sit in. A service passes one down when it works on another
// teacher's data on the caller's behalf — closing a colleague's billing
// period, targeting the owner's statements, replaying a roster import — after
// the caller's Scope has already authorised the action. An Anchor carries no
// authority: repositories that take one filter by exactly these two columns
// and never widen, so nothing below the service can be handed borrowed rights
// through a hand-built Scope.
type Anchor struct {
	TeacherID uuid.UUID
	CenterID  uuid.UUID
}

// AnchorTo names teacherID's rows inside the caller's center. The caller's
// Scope must already have authorised acting on that teacher; this only carries
// the identity down to the repository.
func (s Scope) AnchorTo(teacherID uuid.UUID) Anchor {
	return Anchor{TeacherID: teacherID, CenterID: s.CenterID}
}

// Self names the caller's own rows.
func (s Scope) Self() Anchor {
	return s.AnchorTo(s.TeacherID)
}

// OwnerAnchor is an Anchor proven to name the center owner's rows. It embeds
// Anchor so anchored repository helpers accept it unchanged, while a plain
// Anchor can never stand in where an OwnerAnchor is required. Only the
// centers service mints one, after reading the owner off the center row, so a
// feature that must write into the owner's data asks centers for the proof
// instead of asserting ownership itself.
type OwnerAnchor struct {
	Anchor
}

// MintOwnerAnchor is the single constructor for OwnerAnchor; the scoping guard
// keeps its callers inside the centers feature.
func MintOwnerAnchor(ownerID, centerID uuid.UUID) OwnerAnchor {
	return OwnerAnchor{Anchor{TeacherID: ownerID, CenterID: centerID}}
}
