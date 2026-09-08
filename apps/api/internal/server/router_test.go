package server

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/features/auth"
	"teka/apps/api/internal/features/centers"
	"teka/apps/api/internal/features/notifications"
	"teka/apps/api/internal/features/statements"
	"teka/apps/api/internal/features/teachers"
	"teka/apps/api/internal/features/zalo"
	"teka/apps/api/internal/middleware"
	"teka/apps/api/internal/shared/events"
	"teka/apps/api/internal/shared/secrets"
)

// newTestRouter builds the full middleware stack without a database; tests
// must not touch /readyz, the only DB-dependent route.
func newTestRouter(t *testing.T) http.Handler {
	return newTestRouterEnv(t, config.EnvTest)
}

func newTestRouterEnv(t *testing.T, env string) http.Handler {
	return newTestRouterWith(t, env, nil)
}

// newTestRouterWith builds the router for env with the given trusted proxy
// list (nil trusts none, the production default).
func newTestRouterWith(t *testing.T, env string, trustedProxies []string) http.Handler {
	t.Helper()
	cfg := &config.Config{
		Env:         env,
		LogLevel:    "info",
		CORSOrigins: []string{"http://localhost:5173"},
		HTTP:        config.HTTPConfig{Port: 0, MaxBodyBytes: 1 << 20, TrustedProxies: trustedProxies},
		Database:    config.DatabaseConfig{ConnMaxLifetime: time.Minute},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	zaloSvc := newTestZaloService(t)
	statementsSvc := statements.NewService(statements.NewRepository(nil), database.NewTxManager(nil),
		cfg.Statements, statements.BankConfig{}, statements.NewQRBuilder())
	notificationsSvc := notifications.NewService(notifications.NewRepository(nil), database.NewTxManager(nil),
		statementsSvc, zaloSvc, log, cfg.Notifications)
	t.Cleanup(notificationsSvc.Close)

	// Mirrors app.Container's identity wiring (NewRouter no longer builds
	// these itself) with a nil db — no test in this package issues a query
	// through them.
	txMgr := database.NewTxManager(nil)
	teachersSvc := teachers.NewService(teachers.NewRepository(nil))
	centersSvc := centers.NewService(centers.NewRepository(nil), txMgr, nil)
	authSvc := auth.NewService(teachersSvc, auth.NewRepository(nil), auth.NewTokenIssuer(cfg.JWT), txMgr,
		centersSvc, zaloSvc, cfg.Onboarding, cfg.Statements.PublicBaseURL, nil)
	centersSvc.SetAccountDisabler(authSvc)
	teachersSvc.SetTokenRevoker(authSvc)

	return NewRouter(cfg, log, nil, zaloSvc, statementsSvc, notificationsSvc, teachersSvc, centersSvc, authSvc, events.NewSync())
}

// newTestZaloService builds the one feature service NewRouter does not build
// itself. Its key protects nothing here — no test in this package links an
// account — but the cipher has to exist for the routes to mount.
func newTestZaloService(t *testing.T) *zalo.Service {
	t.Helper()
	cipher, err := secrets.New([]byte("router-test-zalo-credential-key-32"))
	if err != nil {
		t.Fatalf("build test cipher: %v", err)
	}
	svc := zalo.NewService(zalo.NewRepository(nil), cipher, zalo.ServiceOptions{})
	t.Cleanup(svc.Close)
	return svc
}

func TestHealthzAlwaysOK(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("/healthz status = %d, want 200", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf(`body = %v, want {"status":"ok"}`, body)
	}
}

func TestUnknownRouteReturns404Envelope(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/nope", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	var body struct {
		Success bool `json:"success"`
		Error   *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON body: %v", err)
	}
	if body.Success {
		t.Error("success = true, want false")
	}
	if body.Error == nil || body.Error.Code != "NOT_FOUND" {
		t.Errorf("error = %+v, want code NOT_FOUND", body.Error)
	}
}

// The OpenAPI UI must ship in every environment except production; the gate
// lives in NewRouter, so a manual check is not enough — pin it here.
func TestSwaggerServedOutsideProductionOnly(t *testing.T) {
	// NewRouter flips gin into release mode for production; restore test mode
	// so later tests are unaffected by this global.
	t.Cleanup(func() { gin.SetMode(gin.TestMode) })

	rec := httptest.NewRecorder()
	newTestRouterEnv(t, config.EnvTest).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("swagger outside production: want 200, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	newTestRouterEnv(t, config.EnvProduction).ServeHTTP(rec,
		httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("swagger in production: want 404, got %d", rec.Code)
	}
}

// The body cap is enforced by the global chain, so a public route whose
// limiter reads the JSON body must refuse an oversized declared length
// before that read ever happens.
func TestOversizedBodyIsRefusedBeforeHandlers(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/forgot-password", strings.NewReader(`{"phone":"0901234567"}`))
	req.Header.Set("Content-Type", "application/json")
	req.ContentLength = 100 << 20
	rec := httptest.NewRecorder()
	newTestRouter(t).ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"PAYLOAD_TOO_LARGE"`) {
		t.Fatalf("body = %s, want PAYLOAD_TOO_LARGE envelope", rec.Body.String())
	}
}

// loginFromDistinctPhones posts n login attempts from one socket address,
// each with a different phone and no password, so the per-phone limiter
// never trips and only a per-IP limiter could answer 429. Binding rejects
// each attempt before any service call, so no database is touched.
func loginFromDistinctPhones(t *testing.T, h http.Handler, n int, remoteAddr string) (last int) {
	t.Helper()
	for i := range n {
		body := fmt.Sprintf(`{"phone":"+849%08d"}`, i)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = remoteAddr
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		last = rec.Code
		if last == http.StatusTooManyRequests {
			return last
		}
	}
	return last
}

// TestLoginIPLimiterOnlyMountedWithTrustedProxies proves the per-IP login
// limiter is absent by default (every caller would share Traefik's address)
// and present once trusted proxies are configured.
func TestLoginIPLimiterOnlyMountedWithTrustedProxies(t *testing.T) {
	const perIPPerMinute = 60

	if code := loginFromDistinctPhones(t, newTestRouter(t), perIPPerMinute+1, "10.0.0.1:4000"); code == http.StatusTooManyRequests {
		t.Fatal("without trusted proxies no per-IP limiter may run")
	}

	trusted := newTestRouterWith(t, config.EnvTest, []string{"10.0.0.0/8"})
	if code := loginFromDistinctPhones(t, trusted, perIPPerMinute, "10.0.0.1:4000"); code == http.StatusTooManyRequests {
		t.Fatalf("request %d from one IP must still pass", perIPPerMinute)
	}
	if code := loginFromDistinctPhones(t, trusted, 1, "10.0.0.1:4000"); code != http.StatusTooManyRequests {
		t.Fatalf("request %d from one IP: status = %d, want 429", perIPPerMinute+1, code)
	}
}

func TestRequestIDGeneratedAndEchoed(t *testing.T) {
	r := newTestRouter(t)

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Header().Get(middleware.RequestIDHeader) == "" {
		t.Error("response missing generated X-Request-ID")
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set(middleware.RequestIDHeader, "test-id-123")
	r.ServeHTTP(rec, req)
	if got := rec.Header().Get(middleware.RequestIDHeader); got != "test-id-123" {
		t.Errorf("X-Request-ID = %q, want passthrough of test-id-123", got)
	}
}
