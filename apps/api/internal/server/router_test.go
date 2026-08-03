package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/middleware"
)

// newTestRouter builds the full middleware stack without a database; tests
// must not touch /readyz, the only DB-dependent route.
func newTestRouter(t *testing.T) http.Handler {
	return newTestRouterEnv(t, config.EnvTest)
}

func newTestRouterEnv(t *testing.T, env string) http.Handler {
	t.Helper()
	cfg := &config.Config{
		Env:         env,
		LogLevel:    "info",
		CORSOrigins: []string{"http://localhost:5173"},
		HTTP:        config.HTTPConfig{Port: 0},
		Database:    config.DatabaseConfig{ConnMaxLifetime: time.Minute},
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewRouter(cfg, log, nil)
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
