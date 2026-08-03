// Package apperror defines the typed error contract between services and the
// HTTP layer. Services return *AppError (or wrap one); the response package
// maps it onto the API envelope.
package apperror

import (
	"errors"
	"fmt"
	"net/http"
)

// Stable machine-readable error codes exposed in the API error envelope.
const (
	CodeBadRequest   = "BAD_REQUEST"
	CodeValidation   = "VALIDATION_ERROR"
	CodeUnauthorized = "UNAUTHORIZED"
	CodeForbidden    = "FORBIDDEN"
	CodeNotFound     = "NOT_FOUND"
	CodeConflict     = "CONFLICT"
	CodeInternal     = "INTERNAL_ERROR"
)

// AppError carries an error code, HTTP status, and client-safe message across
// the service boundary.
type AppError struct {
	Code    string
	Status  int
	Message string
	// Fields holds per-field validation messages for VALIDATION_ERROR.
	Fields map[string]string
	// Err is the underlying cause; logged, never exposed to clients.
	Err error
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

func (e *AppError) Unwrap() error { return e.Err }

// New builds an AppError with an explicit code, HTTP status, and message.
func New(code string, status int, message string) *AppError {
	return &AppError{Code: code, Status: status, Message: message}
}

// BadRequest is a 400 for malformed or unparsable requests.
func BadRequest(message string) *AppError {
	return New(CodeBadRequest, http.StatusBadRequest, message)
}

// Invalid is a 422 validation failure with per-field messages.
func Invalid(message string, fields map[string]string) *AppError {
	e := New(CodeValidation, http.StatusUnprocessableEntity, message)
	e.Fields = fields
	return e
}

// Unauthorized is a 401 for missing or invalid credentials.
func Unauthorized(message string) *AppError {
	return New(CodeUnauthorized, http.StatusUnauthorized, message)
}

// Forbidden is a 403 for authenticated callers lacking permission.
func Forbidden(message string) *AppError {
	return New(CodeForbidden, http.StatusForbidden, message)
}

// NotFound is a 404 naming the missing resource.
func NotFound(resource string) *AppError {
	return New(CodeNotFound, http.StatusNotFound, resource+" not found")
}

// Conflict is a 409 for state conflicts such as duplicates.
func Conflict(message string) *AppError {
	return New(CodeConflict, http.StatusConflict, message)
}

// Internal is a 500 that keeps the cause for logs and hides it from clients.
func Internal(err error) *AppError {
	e := New(CodeInternal, http.StatusInternalServerError, "internal server error")
	e.Err = err
	return e
}

// From returns err as an *AppError, wrapping unknown errors as Internal so
// their details are never leaked to clients.
func From(err error) *AppError {
	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr
	}
	return Internal(err)
}
