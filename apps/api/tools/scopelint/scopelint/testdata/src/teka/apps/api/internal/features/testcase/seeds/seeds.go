// Package seeds exercises the exempt-path rule for development seed data:
// it legitimately builds authctx.Scope by hand to seed fixture rows.
package seeds

import "teka/apps/api/internal/shared/authctx"

// GoodScopeLiteral builds a Scope by hand — a seed fixture's job, exempt
// from the module-wide R3 literal ban.
func GoodScopeLiteral() authctx.Scope {
	return authctx.Scope{TeacherID: "x"}
}
