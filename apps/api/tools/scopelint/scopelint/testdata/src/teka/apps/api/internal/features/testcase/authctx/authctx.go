// Package authctx exercises the exempt-path rule for the scope-carrying
// types' own home: authctx legitimately builds and mints its own Scope and
// OwnerAnchor values.
package authctx

import realauthctx "teka/apps/api/internal/shared/authctx"

// GoodScopeLiteral builds a Scope by hand — authctx's own job, exempt from
// the module-wide R3 literal ban.
func GoodScopeLiteral() realauthctx.Scope {
	return realauthctx.Scope{TeacherID: "x"}
}
