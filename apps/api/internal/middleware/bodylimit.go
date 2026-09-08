package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/response"
)

// BodyLimit caps every request body at limit bytes. A declared Content-Length
// over the cap is refused up front with 413 before any later middleware or
// handler touches the body; everything else is wrapped in
// http.MaxBytesReader so a chunked or lying client is cut off at the cap
// while reading, which validation.BindError then reports as the same 413.
//
// Routes named in exemptFullPaths (gin full paths, e.g.
// "/api/v1/imports/roster") pass through untouched; they own a cap of their
// own sized for the upload they accept. gin resolves the route before the
// global chain runs, so c.FullPath() is already set here.
func BodyLimit(limit int64, exemptFullPaths ...string) gin.HandlerFunc {
	exempt := make(map[string]struct{}, len(exemptFullPaths))
	for _, p := range exemptFullPaths {
		exempt[p] = struct{}{}
	}
	return func(c *gin.Context) {
		if _, ok := exempt[c.FullPath()]; ok || c.Request.Body == nil || c.Request.Body == http.NoBody {
			c.Next()
			return
		}
		if c.Request.ContentLength > limit {
			response.Err(c, apperror.PayloadTooLarge("request body too large"))
			c.Abort()
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}
