// Package testutil exercises the exempt-path rule for test fixtures: they
// legitimately build authctx.Scope by hand.
package testutil

import "teka/apps/api/internal/shared/authctx"

// GoodScopeLiteral builds a Scope by hand — a fixture's job, exempt from the
// module-wide R3 literal ban.
func GoodScopeLiteral() authctx.Scope {
	return authctx.Scope{TeacherID: "x"}
}
