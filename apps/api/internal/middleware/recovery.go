package middleware

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/logger"
	"teka/apps/api/internal/shared/response"
)

// Recovery converts panics into a 500 envelope and logs the stack trace.
// http.ErrAbortHandler is re-panicked (net/http's sanctioned silent abort),
// and broken-connection panics skip the doomed response write.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			if err, ok := r.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(r)
			}
			log := logger.FromContext(c.Request.Context())
			if isBrokenConnection(r) {
				log.Warn("client connection broken", "error", fmt.Sprint(r))
				c.Abort()
				return
			}
			log.Error("panic recovered",
				"panic", fmt.Sprint(r),
				"stack", string(debug.Stack()),
			)
			response.Err(c, apperror.Internal(fmt.Errorf("panic: %v", r)))
		}()
		c.Next()
	}
}

// isBrokenConnection reports whether a panic value stems from writing to a
// closed client connection (EPIPE / ECONNRESET), where responding is futile.
func isBrokenConnection(r any) bool {
	err, ok := r.(error)
	if !ok {
		return false
	}
	var opErr *net.OpError
	return errors.As(err, &opErr)
}
