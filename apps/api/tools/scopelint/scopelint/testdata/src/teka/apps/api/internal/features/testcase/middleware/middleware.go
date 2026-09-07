// Package middleware exercises the exempt-path rule for the request
// middleware layer: it legitimately builds authctx.Scope by hand while
// resolving the caller's scope for every request.
package middleware

import "teka/apps/api/internal/shared/authctx"

// GoodScopeLiteral builds a Scope by hand — middleware's job, exempt from
// the module-wide R3 literal ban.
func GoodScopeLiteral() authctx.Scope {
	return authctx.Scope{TeacherID: "x"}
}
