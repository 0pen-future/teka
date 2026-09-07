package server

import "teka/apps/api/internal/shared/routespec"

// PolicyKind classifies how a route is authorized. Every registered route
// carries exactly one intentional classification; the manifest test fails
// closed on any route added without one. The recognized values and their
// meaning live on routespec.Kind — this package is one of its consumers, not
// a second source of truth.
type PolicyKind = routespec.Kind

// PolicyXxx are local names for the routespec.KindXxx values, kept so
// existing route_policy.go callers and tests read unchanged. See
// routespec.Kind's constants for what each classification means.
const (
	PolicyPublic      = routespec.KindPublic
	PolicyPublicToken = routespec.KindPublicToken
	PolicySelf        = routespec.KindSelf
	PolicyOwnerOnly   = routespec.KindOwnerOnly
	PolicyPermission  = routespec.KindPermission
	PolicyService     = routespec.KindService
)

// RoutePolicy is one route's frozen authorization classification. Method and
// Path match gin's registration exactly. Key is set only for
// PolicyPermission.
type RoutePolicy struct {
	Method string
	Path   string
	Kind   PolicyKind
	Key    string
}

// routePolicies is derived from the shared route manifest (routespec.Specs)
// so this package, audit.LookupAction, and the request-audit middleware's
// skip sets read the same authorization decision for every route instead of
// three independently maintained tables. Adding, removing, or reclassifying a
// route happens once, in routespec — never here.
var routePolicies = derivePolicies()

func derivePolicies() []RoutePolicy {
	specs := routespec.Policies()
	out := make([]RoutePolicy, len(specs))
	for i, s := range specs {
		out[i] = RoutePolicy{Method: s.Method, Path: s.Path, Kind: s.Kind, Key: s.Key}
	}
	return out
}
