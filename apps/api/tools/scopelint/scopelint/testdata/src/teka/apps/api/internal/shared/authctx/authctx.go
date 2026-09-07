// Package authctx is a minimal stand-in for
// teka/apps/api/internal/shared/authctx, stubbed at its real import path so
// the analyzer's type-based checks resolve against it exactly as they do on
// the real package.
package authctx

// Scope mirrors the caller's tenant context. Field types are simplified
// (string instead of uuid.UUID) — only field names and the type's identity
// matter to the analyzer under test.
type Scope struct {
	TeacherID string
	CenterID  string
	IsOwner   bool
	Perms     PermSet
}

// PermSet mirrors the effective permission set.
type PermSet map[string]struct{}

// Has mirrors Scope.Has — banned in repository files.
func (s Scope) Has(key string) bool {
	_, ok := s.Perms[key]
	return ok
}

// CenterWideFor mirrors Scope.CenterWideFor — allowed only inside a
// read-named repository function.
func (s Scope) CenterWideFor(key string) bool {
	_, ok := s.Perms[key]
	return ok
}

// Anchor mirrors the caller-identified target of a write.
type Anchor struct {
	TeacherID string
	CenterID  string
}

// OwnerAnchor mirrors an Anchor proven to name the center owner.
type OwnerAnchor struct {
	Anchor
}

// MintOwnerAnchor mirrors the single OwnerAnchor constructor — allowed only
// inside the centers package.
func MintOwnerAnchor(ownerID, centerID string) OwnerAnchor {
	return OwnerAnchor{Anchor{TeacherID: ownerID, CenterID: centerID}}
}

// StaffRolesFor mirrors the capability-to-role lookup — banned in
// repository files.
func StaffRolesFor(capability string) []string {
	return nil
}

// StaffRoleCan mirrors the role capability check — banned in repository
// files.
func StaffRoleCan(roleKey string, capability string) bool {
	return false
}
