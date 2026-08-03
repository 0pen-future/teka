package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestIDHeader is the header used to receive and expose the request id.
const RequestIDHeader = "X-Request-ID"

const requestIDKey = "request_id"

// RequestID propagates an inbound X-Request-ID or generates one, exposing it
// on the response header and the gin context.
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(RequestIDHeader)
		if id == "" {
			id = uuid.NewString()
		}
		c.Set(requestIDKey, id)
		c.Writer.Header().Set(RequestIDHeader, id)
		c.Next()
	}
}

// RequestIDFrom returns the request id set by RequestID, or "".
func RequestIDFrom(c *gin.Context) string {
	return c.GetString(requestIDKey)
}
