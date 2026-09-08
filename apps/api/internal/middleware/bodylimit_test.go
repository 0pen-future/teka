package middleware

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// bodyLimitEngine mounts BodyLimit(limit) in front of a handler that reads the
// whole body and reports a MaxBytesReader cut-off through BindError, the
// way a real handler's ShouldBindJSON would.
func bodyLimitEngine(limit int64, exempt ...string) (*gin.Engine, *bool) {
	gin.SetMode(gin.TestMode)
	ran := false
	r := gin.New()
	r.Use(BodyLimit(limit, exempt...))
	read := func(c *gin.Context) {
		ran = true
		raw, err := io.ReadAll(c.Request.Body)
		if err != nil {
			response.Err(c, validation.BindError(err))
			return
		}
		c.String(http.StatusOK, "%d", len(raw))
	}
	r.POST("/x", read)
	r.POST("/api/v1/imports/roster", read)
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	return r, &ran
}

func post(r http.Handler, path string, body string, chunked bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if chunked {
		req.ContentLength = -1
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestBodyLimitRefusesDeclaredLengthOverCap(t *testing.T) {
	r, ran := bodyLimitEngine(16)
	w := post(r, "/x", strings.Repeat("a", 32), false)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413, got %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"PAYLOAD_TOO_LARGE"`) {
		t.Fatalf("want PAYLOAD_TOO_LARGE envelope, got %s", w.Body.String())
	}
	if *ran {
		t.Fatal("handler must not run for a body refused up front")
	}
}

func TestBodyLimitCutsOffUndeclaredBodyOverCap(t *testing.T) {
	r, ran := bodyLimitEngine(16)
	w := post(r, "/x", strings.Repeat("a", 32), true)

	if !*ran {
		t.Fatal("an undeclared length cannot be refused up front; the handler must run")
	}
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413 from the read cut-off, got %d %s", w.Code, w.Body.String())
	}
}

func TestBodyLimitPassesBodiesWithinCap(t *testing.T) {
	r, _ := bodyLimitEngine(16)
	w := post(r, "/x", strings.Repeat("a", 16), false)
	if w.Code != http.StatusOK || w.Body.String() != "16" {
		t.Fatalf("want 200 with full body, got %d %s", w.Code, w.Body.String())
	}
}

func TestBodyLimitExemptRouteReadsPastCap(t *testing.T) {
	r, _ := bodyLimitEngine(16, "/api/v1/imports/roster")
	w := post(r, "/api/v1/imports/roster", strings.Repeat("a", 64), false)
	if w.Code != http.StatusOK || w.Body.String() != "64" {
		t.Fatalf("exempt route must read the whole body, got %d %s", w.Code, w.Body.String())
	}
}

func TestBodyLimitIgnoresRequestsWithoutBody(t *testing.T) {
	r, _ := bodyLimitEngine(16)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", w.Code)
	}
}

// gin's JSON binding must hand the MaxBytesReader error back unwrapped, or
// BindError could not tell an oversized body from a malformed one.
func TestShouldBindJSONSurfacesMaxBytesError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BodyLimit(8))
	var got error
	r.POST("/x", func(c *gin.Context) {
		var body struct {
			Phone string `json:"phone"`
		}
		got = c.ShouldBindJSON(&body)
		c.Status(http.StatusOK)
	})
	post(r, "/x", `{"phone":"0901234567"}`, true)

	var tooLarge *http.MaxBytesError
	if !errors.As(got, &tooLarge) {
		t.Fatalf("ShouldBindJSON error = %v, want *http.MaxBytesError", got)
	}
}
