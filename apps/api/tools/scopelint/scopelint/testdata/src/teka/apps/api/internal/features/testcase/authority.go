package testcase

import "teka/apps/api/internal/shared/authctx"

// BadScopeLiteral builds an authctx.Scope by hand with fields set instead of
// resolving one — the module-wide R3 literal ban, not restricted to
// repository files.
func BadScopeLiteral() authctx.Scope {
	return authctx.Scope{TeacherID: "x"} // want "authctx.Scope is built by hand with fields set"
}

// GoodScopeZeroValue is a bare zero-value Scope literal, which carries no
// authority and is not banned.
func GoodScopeZeroValue() authctx.Scope {
	return authctx.Scope{}
}

// BadScopeAssign assigns into a field of a copied Scope value — the
// s := sc; s.TeacherID = x bypass a name-based guard cannot see.
func BadScopeAssign(sc authctx.Scope) authctx.Scope {
	s := sc
	s.TeacherID = "x" // want "a field of an authctx.Scope value is assigned directly"
	return s
}

// BadScopeAssignPtr assigns into a field of a Scope value through one level
// of pointer indirection — p := &sc; p.TeacherID = x — which a receiver-type
// check on sel.X alone would miss without unwrapping the pointer.
func BadScopeAssignPtr(sc authctx.Scope) *authctx.Scope {
	p := &sc
	p.TeacherID = "x" // want "a field of an authctx.Scope value is assigned directly"
	return p
}

// BadOwnerAnchorLiteral builds an OwnerAnchor by hand instead of obtaining
// one as a proof.
func BadOwnerAnchorLiteral() authctx.OwnerAnchor {
	return authctx.OwnerAnchor{} // want "authctx.OwnerAnchor is a proof"
}

// BadMintOwnerAnchor calls the OwnerAnchor constructor outside the centers
// package.
func BadMintOwnerAnchor() authctx.OwnerAnchor {
	return authctx.MintOwnerAnchor("x", "y") // want "MintOwnerAnchor is minted by centers only"
}
