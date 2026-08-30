package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/events"
)

// RequestCompleted is published on the event bus after a mutating API request
// finishes, regardless of status code. It lives here — next to its future
// publisher — so subscribers depend on the middleware package and not the
// other way around.
type RequestCompleted struct {
	OccurredAt time.Time
	// CenterID is uuid.Nil when the request carried no center scope.
	CenterID uuid.UUID
	ActorID  uuid.UUID
	// ActorRole is "owner" or "member"; empty when the route resolves no
	// center scope.
	ActorRole string
	Method    string
	// Route is the gin route template (e.g. /api/v1/classes/:id), stable
	// across requests; Path is the concrete URL path.
	Route string
	Path  string
	// Params holds the route parameters by name, so subscribers can extract
	// entity ids without this package knowing which parameter matters.
	Params     map[string]string
	StatusCode int
	RequestID  string
	IP         string
	UserAgent  string
}

// EventName implements events.Event.
func (RequestCompleted) EventName() string { return "http.request_completed" }

// authSessionRoutes are mutating routes the middleware must never publish:
// login and logout are audited by the auth service's own events (publishing
// here would double-log them), and refresh is deliberate noise-avoidance —
// token rotation is not a user action.
var authSessionRoutes = map[string]bool{
	"/api/v1/auth/login":   true,
	"/api/v1/auth/logout":  true,
	"/api/v1/auth/refresh": true,
}

// serviceAuditedRoutes are mutating routes whose owning feature publishes its
// own domain event for the same action — publishing here too would land two
// audit rows for one request. The service event is the richer of the pair
// (enrollments.StudentEnrolled carries the class and student ids the request
// row could not), so the request row is the one that yields.
var serviceAuditedRoutes = map[string]bool{
	"/api/v1/enrollments": true,
}

// anonymousAuditedRoutes are the only unauthenticated mutations worth a row:
// a password change must never escape the trail even though the caller has
// no session yet. Every other principal-less mutation (public invitation
// accept, statement views) is skipped here — the owning feature publishes
// its own event when the action deserves one.
var anonymousAuditedRoutes = map[string]bool{
	"/api/v1/auth/forgot-password": true,
	"/api/v1/auth/reset-password":  true,
}

// RequestEvents publishes one RequestCompleted event per mutating API request
// after the handler chain finishes, success or failure alike. It depends only
// on the bus — never on the audit feature — so publishers stay ignorant of
// their subscribers.
func RequestEvents(bus events.Bus) gin.HandlerFunc {
	if bus == nil {
		// Same tolerance the publishing services extend: a nil bus disables
		// capture instead of panicking on the first mutation.
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		// Publishing runs deferred so a panicking handler still leaves a row:
		// the panic unwinds through here before Recovery writes the 500, so
		// on that path the status is forced rather than read from the writer.
		defer func() {
			if r := recover(); r != nil {
				publishRequest(c, bus, http.StatusInternalServerError)
				panic(r)
			}
			publishRequest(c, bus, c.Writer.Status())
		}()
		c.Next()
	}
}

func publishRequest(c *gin.Context, bus events.Bus, status int) {
	switch c.Request.Method {
	case "POST", "PUT", "PATCH", "DELETE":
	default:
		return
	}
	// An empty template means no registered route matched: arbitrary
	// attacker-chosen 404 paths stay out of the audit table.
	route := c.FullPath()
	if route == "" || authSessionRoutes[route] || serviceAuditedRoutes[route] {
		return
	}
	p, authed := authctx.From(c)
	if !authed && !anonymousAuditedRoutes[route] {
		return
	}

	e := RequestCompleted{
		OccurredAt: time.Now(),
		ActorID:    p.UserID,
		Method:     c.Request.Method,
		Route:      route,
		// URL.Path, never RequestURI: query strings can carry phone
		// numbers and must not reach the audit table.
		Path:       c.Request.URL.Path,
		StatusCode: status,
		RequestID:  RequestIDFrom(c),
		IP:         c.ClientIP(),
		UserAgent:  c.Request.UserAgent(),
	}
	if sc, ok := authctx.ScopeFrom(c); ok {
		e.CenterID = sc.CenterID
		if sc.IsOwner {
			e.ActorRole = "owner"
		} else {
			e.ActorRole = "member"
		}
	}
	if params := c.Params; len(params) > 0 {
		e.Params = make(map[string]string, len(params))
		for _, prm := range params {
			e.Params[prm.Key] = prm.Value
		}
	}
	bus.Publish(e)
}
