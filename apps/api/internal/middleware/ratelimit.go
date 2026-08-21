package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/response"
)

// KeyFunc extracts the rate-limit bucket key for a request. Keys must be a
// business identity (an invite/reset token, a phone number) — never
// c.ClientIP(): the API runs behind Traefik with SetTrustedProxies(nil), so
// ClientIP() collapses every caller into one shared bucket.
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
