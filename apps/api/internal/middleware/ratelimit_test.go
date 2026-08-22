package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestLimiterAllowsUpToLimitWithinWindow(t *testing.T) {
	l := newLimiter(2, time.Minute)
	base := time.Now()
	l.now = func() time.Time { return base }

	if !l.allow("k") {
		t.Fatal("1st request within limit must be allowed")
	}
	if !l.allow("k") {
		t.Fatal("2nd request within limit must be allowed")
	}
	if l.allow("k") {
		t.Fatal("3rd request must be rejected once the limit is hit")
	}
}

func TestLimiterWindowRollsOverAndResets(t *testing.T) {
	l := newLimiter(1, 10*time.Second)
	now := time.Now()
	l.now = func() time.Time { return now }

	if !l.allow("k") {
		t.Fatal("1st request must be allowed")
	}
	if l.allow("k") {
		t.Fatal("2nd request in the same window must be rejected")
	}

	// Jump past the window: the counter must reset, not keep accumulating.
	now = now.Add(11 * time.Second)
	if !l.allow("k") {
		t.Fatal("request in a new window must be allowed again")
	}
}

func TestLimiterPerKeyIsolation(t *testing.T) {
	l := newLimiter(1, time.Minute)
	base := time.Now()
	l.now = func() time.Time { return base }

	if !l.allow("a") {
		t.Fatal("first key must be allowed")
	}
	if !l.allow("b") {
		t.Fatal("a different key must have its own independent counter")
	}
	if l.allow("a") {
		t.Fatal("key a must still be limited by its own counter")
	}
}

// TestLimiterSweepEvictsIdleKeys proves the lazy sweep reclaims memory for
// keys nobody has hit in over a period, without a background goroutine.
func TestLimiterSweepEvictsIdleKeys(t *testing.T) {
	l := newLimiter(1, 10*time.Millisecond)
	now := time.Now()
	l.now = func() time.Time { return now }

	l.allow("a")
	now = now.Add(5 * time.Millisecond)
	l.allow("b")

	// Advance far enough that both a's and b's windows have elapsed, and past
	// the sweep throttle (one period since the last sweep at t=0).
	now = now.Add(10 * time.Millisecond)
	l.allow("a")

	l.mu.Lock()
	defer l.mu.Unlock()
	if _, ok := l.windows["b"]; ok {
		t.Fatal("idle key b must be swept once its window has elapsed")
	}
	if len(l.windows) != 1 {
		t.Fatalf("only the freshly-reset key a must remain, got %v", l.windows)
	}
}

func TestRateLimitMiddlewareReturns429ThenResetsNextWindow(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RateLimit(JSONBodyKey("token"), 2, time.Hour))
	r.POST("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	do := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"token":"`+token+`"}`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	if w := do("tok-a"); w.Code != http.StatusOK {
		t.Fatalf("1st request: want 200, got %d", w.Code)
	}
	if w := do("tok-a"); w.Code != http.StatusOK {
		t.Fatalf("2nd request: want 200, got %d", w.Code)
	}

	w := do("tok-a")
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("3rd request: want 429, got %d %s", w.Code, w.Body.String())
	}
	var env struct {
		Success bool `json:"success"`
		Error   struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if env.Success || env.Error.Code != "TOO_MANY_REQUESTS" {
		t.Fatalf("want TOO_MANY_REQUESTS envelope, got %+v", env)
	}

	// A different key (different token) is a fresh, independent bucket.
	if w := do("tok-b"); w.Code != http.StatusOK {
		t.Fatalf("a different key must not be limited by tok-a's bucket, got %d", w.Code)
	}
}

// TestJSONBodyKeyPreservesBodyForDownstreamBinding proves the key extraction
// peek does not consume the body ShouldBindJSON needs afterward.
func TestJSONBodyKeyPreservesBodyForDownstreamBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RateLimit(JSONBodyKey("token"), 10, time.Minute))
	r.POST("/x", func(c *gin.Context) {
		var body struct {
			Token string `json:"token"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		c.JSON(http.StatusOK, gin.H{"token": body.Token})
	})

	req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"token":"abc"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("want 200, got %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"abc"`) {
		t.Fatalf("downstream binding must still see the full body, got %s", w.Body.String())
	}
}

// TestRateLimitSkipsEmptyKey proves a request that carries no usable key
// (malformed/absent body) is never limited, since binding will reject it on
// its own.
func TestRateLimitSkipsEmptyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RateLimit(JSONBodyKey("token"), 1, time.Hour))
	r.POST("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	for range 5 {
		req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`not-json`))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("request with an unusable key must never be limited, got %d", w.Code)
		}
	}
}
