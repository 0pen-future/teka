package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/events"
)

// recorder collects every event the middleware publishes through a SyncBus,
// so assertions run against fully delivered events with no goroutines.
type recorder struct {
	events []events.Event
}

func (r *recorder) handle(_ context.Context, e events.Event) {
	r.events = append(r.events, e)
}

var (
	testActor  = uuid.MustParse("22222222-2222-2222-2222-222222222222")
	testCenter = uuid.MustParse("11111111-1111-1111-1111-111111111111")
)

// newEventsRouter builds a minimal router shaped like the real one: request id
// first, then RequestEvents on the /api/v1 group, then routes. withIdentity
// injects a principal+scope before the handler, standing in for
// RequireAuth+ResolveScope.
func newEventsRouter(rec *recorder) (*gin.Engine, *gin.RouterGroup) {
	gin.SetMode(gin.TestMode)
	bus := events.NewSync()
	bus.Subscribe("test", 0, rec.handle)

	r := gin.New()
	r.Use(RequestID())
	v1 := r.Group("/api/v1")
	v1.Use(RequestEvents(bus))
	return r, v1
}

func withIdentity(role string, isOwner bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		authctx.Set(c, authctx.Principal{UserID: testActor, Role: role})
		authctx.SetScope(c, authctx.Scope{TeacherID: testActor, CenterID: testCenter, IsOwner: isOwner})
		c.Next()
	}
}

func do(r *gin.Engine, method, path string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("User-Agent", "go-test")
	r.ServeHTTP(w, req)
	return w
}

// TestPublishesMutationWithIdentity proves one fully populated event per
// authenticated mutating request.
func TestPublishesMutationWithIdentity(t *testing.T) {
	rec := &recorder{}
	r, v1 := newEventsRouter(rec)
	v1.POST("/classes/:id/archive", withIdentity("teachers", true), func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{})
	})

	do(r, http.MethodPost, "/api/v1/classes/abc-1/archive?phone=0901234567")

	if len(rec.events) != 1 {
		t.Fatalf("events published = %d, want 1", len(rec.events))
	}
	e, ok := rec.events[0].(RequestCompleted)
	if !ok {
		t.Fatalf("event type = %T, want RequestCompleted", rec.events[0])
	}
	if e.Method != http.MethodPost || e.Route != "/api/v1/classes/:id/archive" {
		t.Errorf("method/route = %q %q", e.Method, e.Route)
	}
	if e.Path != "/api/v1/classes/abc-1/archive" {
		t.Errorf("path = %q, must be URL path without query string", e.Path)
	}
	if e.Params["id"] != "abc-1" {
		t.Errorf("params = %v, want id=abc-1", e.Params)
	}
	if e.StatusCode != http.StatusCreated {
		t.Errorf("status = %d, want 201", e.StatusCode)
	}
	if e.ActorID != testActor || e.CenterID != testCenter || e.ActorRole != "owner" {
		t.Errorf("identity = %v %v %q", e.ActorID, e.CenterID, e.ActorRole)
	}
	if e.RequestID == "" || e.IP == "" || e.UserAgent != "go-test" {
		t.Errorf("request context = %q %q %q", e.RequestID, e.IP, e.UserAgent)
	}
	if e.OccurredAt.IsZero() {
		t.Error("OccurredAt is zero")
	}
}

// TestPublishesOnErrorStatus proves failed mutations stay in the trail.
func TestPublishesOnErrorStatus(t *testing.T) {
	rec := &recorder{}
	r, v1 := newEventsRouter(rec)
	v1.POST("/classes", withIdentity("teachers", false), func(c *gin.Context) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{})
	})

	do(r, http.MethodPost, "/api/v1/classes")

	if len(rec.events) != 1 {
		t.Fatalf("events = %d, want 1 (4xx still audited)", len(rec.events))
	}
	e := rec.events[0].(RequestCompleted)
	if e.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422", e.StatusCode)
	}
	if e.ActorRole != "member" {
		t.Errorf("role = %q, want member", e.ActorRole)
	}
}

// TestSkipsReadsAndAnonymous proves GETs and unauthenticated mutations (off
// the allowlist) publish nothing.
func TestSkipsReadsAndAnonymous(t *testing.T) {
	rec := &recorder{}
	r, v1 := newEventsRouter(rec)
	v1.GET("/classes", withIdentity("teachers", true), func(c *gin.Context) { c.JSON(200, gin.H{}) })
	v1.POST("/invitations/accept", func(c *gin.Context) { c.JSON(200, gin.H{}) })

	do(r, http.MethodGet, "/api/v1/classes")
	do(r, http.MethodPost, "/api/v1/invitations/accept")

	if len(rec.events) != 0 {
		t.Fatalf("events = %d, want 0 (GET + anonymous mutation)", len(rec.events))
	}
}

// TestSkipsAuthSessionRoutes proves login/logout/refresh never double-log —
// the auth service publishes its own events.
func TestSkipsAuthSessionRoutes(t *testing.T) {
	rec := &recorder{}
	r, v1 := newEventsRouter(rec)
	v1.POST("/auth/login", func(c *gin.Context) { c.JSON(200, gin.H{}) })
	v1.POST("/auth/refresh", func(c *gin.Context) { c.JSON(200, gin.H{}) })
	// logout carries a principal in production; the skip must not depend on
	// the principal's absence.
	v1.POST("/auth/logout", withIdentity("teachers", false), func(c *gin.Context) { c.JSON(200, gin.H{}) })

	do(r, http.MethodPost, "/api/v1/auth/login")
	do(r, http.MethodPost, "/api/v1/auth/refresh")
	do(r, http.MethodPost, "/api/v1/auth/logout")

	if len(rec.events) != 0 {
		t.Fatalf("events = %d, want 0 (auth session routes are service-audited)", len(rec.events))
	}
}

// TestAuditsPasswordResetWithoutPrincipal proves the two password-reset
// routes are audited despite having no authenticated actor — a password
// change must never escape the trail.
func TestAuditsPasswordResetWithoutPrincipal(t *testing.T) {
	rec := &recorder{}
	r, v1 := newEventsRouter(rec)
	v1.POST("/auth/forgot-password", func(c *gin.Context) { c.JSON(200, gin.H{}) })
	v1.POST("/auth/reset-password", func(c *gin.Context) { c.JSON(200, gin.H{}) })

	do(r, http.MethodPost, "/api/v1/auth/forgot-password")
	do(r, http.MethodPost, "/api/v1/auth/reset-password")

	if len(rec.events) != 2 {
		t.Fatalf("events = %d, want 2", len(rec.events))
	}
	for _, ev := range rec.events {
		e := ev.(RequestCompleted)
		if e.ActorID != uuid.Nil || e.CenterID != uuid.Nil || e.ActorRole != "" {
			t.Errorf("anonymous reset row must carry no identity: %+v", e)
		}
	}
}

// TestSkipsUnmatchedRoutes proves a 404 with an arbitrary path publishes
// nothing — unbounded attacker-chosen paths stay out of the table.
func TestSkipsUnmatchedRoutes(t *testing.T) {
	rec := &recorder{}
	r, _ := newEventsRouter(rec)

	do(r, http.MethodPost, "/api/v1/definitely/not/a/route")

	if len(rec.events) != 0 {
		t.Fatalf("events = %d, want 0 for unmatched route", len(rec.events))
	}
}

// TestPanickingMutationStillPublishes proves a handler panic leaves an audit
// event: publishing runs deferred, records a forced 500, and re-panics so
// Recovery still writes the response.
func TestPanickingMutationStillPublishes(t *testing.T) {
	rec := &recorder{}
	gin.SetMode(gin.TestMode)
	bus := events.NewSync()
	bus.Subscribe("test", 0, rec.handle)

	r := gin.New()
	r.Use(RequestID(), Recovery())
	v1 := r.Group("/api/v1")
	v1.Use(RequestEvents(bus))
	v1.POST("/classes", withIdentity("teachers", true), func(_ *gin.Context) {
		panic("boom")
	})

	w := do(r, http.MethodPost, "/api/v1/classes")

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (Recovery must still answer)", w.Code)
	}
	if len(rec.events) != 1 {
		t.Fatalf("events = %d, want 1 (panic must not erase the trail)", len(rec.events))
	}
	e := rec.events[0].(RequestCompleted)
	if e.StatusCode != http.StatusInternalServerError {
		t.Errorf("event status = %d, want 500", e.StatusCode)
	}
	if e.ActorID != testActor {
		t.Errorf("actor = %v, want %v", e.ActorID, testActor)
	}
}

// TestNilBusDisablesCapture proves a nil bus yields a pass-through handler
// instead of a panic on the first mutation.
func TestNilBusDisablesCapture(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestID())
	v1 := r.Group("/api/v1")
	v1.Use(RequestEvents(nil))
	v1.POST("/classes", withIdentity("teachers", true), func(c *gin.Context) {
		c.JSON(http.StatusCreated, gin.H{})
	})

	if w := do(r, http.MethodPost, "/api/v1/classes"); w.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", w.Code)
	}
}
