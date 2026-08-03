// Package response owns the API response envelope:
//
//	{"success": true,  "data": ..., "meta": {...}}
//	{"success": false, "error": {"code": "...", "message": "...", "fields": {...}}}
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/logger"
)

// Meta carries pagination info alongside list responses.
type Meta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

// ErrorBody is the error half of the envelope; exported (with Envelope) so
// the OpenAPI annotations can reference the real wire shape.
type ErrorBody struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// Envelope is the wire format of every /api/v1 response.
type Envelope struct {
	Success bool       `json:"success"`
	Data    any        `json:"data,omitempty"`
	Meta    *Meta      `json:"meta,omitempty"`
	Error   *ErrorBody `json:"error,omitempty"`
}

// OK writes a success envelope with the given status and data.
func OK(c *gin.Context, status int, data any) {
	c.JSON(status, Envelope{Success: true, Data: data})
}

// List writes a success envelope with data plus pagination meta.
func List(c *gin.Context, data any, meta Meta) {
	c.JSON(http.StatusOK, Envelope{Success: true, Data: data, Meta: &meta})
}

// Err maps any error onto the envelope via apperror.From. Internal errors are
// logged with their cause; the client only sees a generic message.
func Err(c *gin.Context, err error) {
	appErr := apperror.From(err)
	if appErr.Status >= http.StatusInternalServerError {
		logger.FromContext(c.Request.Context()).Error("request failed",
			"code", appErr.Code, "error", appErr.Error())
	}
	c.AbortWithStatusJSON(appErr.Status, Envelope{
		Success: false,
		Error: &ErrorBody{
			Code:    appErr.Code,
			Message: appErr.Message,
			Fields:  appErr.Fields,
		},
	})
}
