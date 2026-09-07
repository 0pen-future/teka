package audit

import "teka/apps/api/internal/shared/routespec"

// ActionSpec names a mutating route for humans: a stable dotted action, the
// entity type it touches, and which route parameter carries the entity id.
type ActionSpec struct {
	Action     string
	EntityType string
	// IDParam is the route parameter holding the entity id ("" when the
	// route addresses no single entity, e.g. collection creates).
	IDParam string
}

// LookupAction resolves a mutating route to its audit spec from the shared
// route manifest (routespec.Specs), which is the single place a route's
// Action is decided. ok is false when the route has no Action to report —
// either it is not registered at all, or its trail comes from a service
// event instead of this request path (e.g. login/logout, or enrollment
// create, both published as domain events) — and callers then fall back to
// "METHOD route".
func LookupAction(method, route string) (ActionSpec, bool) {
	spec, found := routespec.Lookup(method, route)
	if !found || spec.Audit.Action == "" {
		return ActionSpec{}, false
	}
	return ActionSpec{
		Action:     spec.Audit.Action,
		EntityType: spec.Audit.EntityType,
		IDParam:    spec.Audit.IDParam,
	}, true
}
