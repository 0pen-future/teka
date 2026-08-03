package server

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"teka/apps/api/internal/config"
	"teka/apps/api/internal/middleware"
)

// newTestRouter builds the full middleware stack without a database; tests
// must not touch /readyz, the only DB-dependent route.
func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	cfg := &config.Config{
		Env:         config.EnvTest,
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
