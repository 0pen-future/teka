package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
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

// An oversized body hits the MaxBytesReader cut-off inside JSONBodyKey's
// read; that must still reach the client as 413, not a silent 400, and must
// not spend the caller's bucket.
func TestJSONBodyKeyOversizedBodySurfacesAsPayloadTooLarge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(BodyLimit(32), RateLimit(JSONBodyKey("phone"), 1, time.Minute))
	r.POST("/x", func(c *gin.Context) {
		var body struct {
			Phone string `json:"phone"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			response.Err(c, validation.BindError(err))
			return
		}
		c.Status(http.StatusOK)
	})

	send := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.ContentLength = -1
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	if w := send(`{"phone":"0901234567","pad":"` + strings.Repeat("x", 64) + `"}`); w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413 for an oversized body, got %d %s", w.Code, w.Body.String())
	}
	if w := send(`{"phone":"0901234567"}`); w.Code != http.StatusOK {
		t.Fatalf("the refused request must not consume the bucket, got %d %s", w.Code, w.Body.String())
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

// TestJSONBodyKeyMatchesFieldLikeStructBinding proves the limiter sees the
// same value the handler will bind: encoding/json matches struct fields
// case-insensitively, so a differently cased key must not yield an empty
// (unlimited) bucket.
func TestJSONBodyKeyMatchesFieldLikeStructBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	key := JSONBodyKey("phone")
	keyFor := func(body string) string {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
		return key(c)
	}
	if got := keyFor(`{"Phone":"0901234567"}`); got != "0901234567" {
		t.Fatalf("capitalized field: key = %q, want the bound value", got)
	}
	if got := keyFor(`{"PHONE":"a","phone":"b"}`); got != "b" {
		t.Fatalf("exact match must win over a case-insensitive one, got %q", got)
	}
}

func TestPhoneKeyNormalizesLocalAndInternationalForms(t *testing.T) {
	gin.SetMode(gin.TestMode)
	key := PhoneKey("phone")
	keyFor := func(body string) string {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		return key(c)
	}

	local, intl := keyFor(`{"phone":" 0901234567 "}`), keyFor(`{"phone":"+84901234567"}`)
	if local != "+84901234567" || local != intl {
		t.Fatalf("local = %q, international = %q, want both +84901234567", local, intl)
	}
	if got := keyFor(`{"phone":"not-a-phone"}`); got != "not-a-phone" {
		t.Fatalf("malformed phone must still be limited on its raw form, got %q", got)
	}
	if got := keyFor(`{"password":"x"}`); got != "" {
		t.Fatalf("absent phone must yield an empty key, got %q", got)
	}
}

func TestClientIPKeyUsesGinResolvedIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	_ = r.SetTrustedProxies([]string{"10.0.0.0/8"})
	var got string
	r.GET("/x", func(c *gin.Context) { got = ClientIPKey()(c); c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "10.0.0.1:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	r.ServeHTTP(httptest.NewRecorder(), req)
	if got != "203.0.113.9" {
		t.Fatalf("key = %q, want the forwarded client IP behind a trusted proxy", got)
	}

	req = httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "198.51.100.4:1234"
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	r.ServeHTTP(httptest.NewRecorder(), req)
	if got != "198.51.100.4" {
		t.Fatalf("key = %q, want the socket address when the peer is not a trusted proxy", got)
	}
}
