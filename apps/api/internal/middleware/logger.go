package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"teka/apps/api/internal/shared/logger"
)

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
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"latency_ms", time.Since(start).Milliseconds(),
			"client_ip", c.ClientIP(),
		)
	}
}
