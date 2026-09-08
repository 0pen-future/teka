package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/response"
	"teka/apps/api/internal/shared/validation"
)

// KeyFunc extracts the rate-limit bucket key for a request. Keys are a
// business identity (an invite/reset token, a phone number). ClientIPKey is
// the one exception and is only mounted when the router has been told which
// proxies to trust: behind Traefik with SetTrustedProxies(nil), ClientIP()
// is the proxy's socket address and would collapse every caller into one
// shared bucket.
type KeyFunc func(c *gin.Context) string

// window is one fixed-window counter for a single key.
type window struct {
	count int
	start time.Time
}

// limiter is a shared in-memory fixed-window rate limiter. It sweeps idle
// keys lazily — inline in allow, throttled to at most once per period —
// instead of running a background goroutine, so no process keeps running
// past the request that triggered it (see process-management rule).
type limiter struct {
	mu        sync.Mutex
	windows   map[string]*window
	limit     int
	period    time.Duration
	lastSweep time.Time
	now       func() time.Time
}

func newLimiter(limit int, period time.Duration) *limiter {
	return &limiter{
		windows: map[string]*window{},
		limit:   limit,
		period:  period,
		now:     time.Now,
	}
}

// allow reports whether key may proceed under the current window, advancing
// or resetting that key's counter as a side effect.
func (l *limiter) allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	l.sweepLocked(now)

	w, ok := l.windows[key]
	if !ok || now.Sub(w.start) >= l.period {
		l.windows[key] = &window{count: 1, start: now}
		return true
	}
	if w.count >= l.limit {
		return false
	}
	w.count++
	return true
}

// sweepLocked evicts windows whose period has already elapsed. Must be
// called with mu held; throttled to run at most once per period so a burst
// of requests never turns every call into an O(n) scan of the whole map.
func (l *limiter) sweepLocked(now time.Time) {
	if !l.lastSweep.IsZero() && now.Sub(l.lastSweep) < l.period {
		return
	}
	l.lastSweep = now
	for key, w := range l.windows {
		if now.Sub(w.start) >= l.period {
			delete(l.windows, key)
		}
	}
}

// RateLimit throttles requests to limit per period, bucketed by keyFn. A
// request whose key is empty skips the limiter — an empty key would
// otherwise collapse every such request into one shared bucket, letting one
// caller (e.g. sending a malformed body) exhaust it for everyone else; that
// request still fails downstream binding on its own.
func RateLimit(keyFn KeyFunc, limit int, period time.Duration) gin.HandlerFunc {
	l := newLimiter(limit, period)
	return func(c *gin.Context) {
		key := keyFn(c)
		if key == "" {
			c.Next()
			return
		}
		if !l.allow(key) {
			response.Err(c, apperror.TooManyRequests("too many requests, try again later"))
			c.Abort()
			return
		}
		c.Next()
	}
}

// JSONBodyKey reads field from the JSON request body as the rate-limit key.
// It restores c.Request.Body afterward so downstream ShouldBindJSON still
// sees the full body. Returns "" (no limiting) when the body is absent,
// unreadable, not JSON, or missing the field — those requests fail binding
// on their own.
func JSONBodyKey(field string) KeyFunc {
	return func(c *gin.Context) string {
		if c.Request.Body == nil {
			return ""
		}
		raw, err := io.ReadAll(c.Request.Body)
		if err != nil {
			// The body is consumed, so hand downstream a reader that fails
			// the same way: a MaxBytesReader cut-off must still surface as
			// 413 from binding instead of an empty-body 400.
			c.Request.Body = io.NopCloser(errReader{err})
			return ""
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(raw))

		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			return ""
		}
		v, _ := payload[field].(string)
		return v
	}
}

// PhoneKey rate-limits on a phone field of the JSON body, normalized so the
// local (0…) and E.164 (+84…) spellings of one number share a bucket. A
// value that is not a Vietnamese number is still limited on its trimmed raw
// form, so a caller cannot dodge the bucket with a malformed spelling; only
// an absent field yields "" (binding rejects that request on its own).
func PhoneKey(field string) KeyFunc {
	raw := JSONBodyKey(field)
	return func(c *gin.Context) string {
		return validation.NormalizePhone(strings.TrimSpace(raw(c)))
	}
}

// ClientIPKey rate-limits on the caller's IP as gin resolves it. Mount it
// only when the router has SetTrustedProxies from configuration, so
// X-Forwarded-For is honoured from the real proxy and from nobody else.
func ClientIPKey() KeyFunc {
	return func(c *gin.Context) string { return c.ClientIP() }
}

// errReader replays one read error on every Read.
type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

// TeacherKey rate-limits on the authenticated caller's teacher id. Use it on
// authenticated routes whose cost is high enough that one account's retry loop
// degrades service for other tenants — the database connection pool is shared
// across every center.
//
// Unlike JSONBodyKey this reads nothing from the request, so it cannot be
// spoofed by the caller. It returns "" (no limiting) on an unauthenticated
// request, which cannot happen on a route mounted behind RequireAuth.
func TeacherKey() KeyFunc {
	return func(c *gin.Context) string {
		sc, ok := authctx.ScopeFrom(c)
		if !ok {
			return ""
		}
		return sc.TeacherID.String()
	}
}
