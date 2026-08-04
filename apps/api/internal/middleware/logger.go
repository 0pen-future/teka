package middleware

import (
	"log/slog"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"teka/apps/api/internal/shared/logger"
)

// publicStatementPathPrefix is where the unauthenticated parent statement
// routes are mounted (see the statements package's RegisterPublicRoutes).
// The path segment right after it is a bearer token: logging it verbatim
// would turn every access log line into a standing credential leak.
const publicStatementPathPrefix = "/public/statements/"

// sanitizePath redacts a public statement token out of a request path before
// it is ever logged, leaving every other path untouched.
// "/public/statements/<token>/qr.png" becomes
// "/public/statements/[redacted]/qr.png"; a bare token path collapses to
// "/public/statements/[redacted]".
func sanitizePath(path string) string {
	rest, ok := strings.CutPrefix(path, publicStatementPathPrefix)
	if !ok {
		return path
	}
	if i := strings.IndexByte(rest, '/'); i >= 0 {
		return publicStatementPathPrefix + "[redacted]" + rest[i:]
	}
	return publicStatementPathPrefix + "[redacted]"
}

// Logger attaches a request-scoped logger (with request_id) to the request
// context and emits one structured line per request.
func Logger(base *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		reqLog := base.With("request_id", RequestIDFrom(c))
		c.Request = c.Request.WithContext(logger.IntoContext(c.Request.Context(), reqLog))

		c.Next()

		reqLog.Info("request",
			"method", c.Request.Method,
			"path", sanitizePath(c.Request.URL.Path),
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
			"client_ip", c.ClientIP(),
		)
	}
}
