// Package validation translates gin binding / validator errors into the
// per-field messages of the 422 envelope.
package validation

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-playground/validator/v10"

	"teka/apps/api/internal/shared/apperror"
)

// BindError converts a c.ShouldBindJSON error into an AppError: validator
// failures become VALIDATION_ERROR with a fields map, everything else (bad
// JSON, wrong types) becomes BAD_REQUEST.
func BindError(err error) *apperror.AppError {
	var verrs validator.ValidationErrors
	if errors.As(err, &verrs) {
		fields := make(map[string]string, len(verrs))
		for _, fe := range verrs {
			fields[fieldName(fe)] = message(fe)
		}
		return apperror.Invalid("validation failed", fields)
	}
	return apperror.BadRequest("invalid request body")
}

// fieldName exposes the JSON-ish name: struct field lowercased with snake
// preserved via the json tag when gin's validator is configured with it;
// fall back to lowercasing the Go field name.
func fieldName(fe validator.FieldError) string {
	return strings.ToLower(fe.Field())
}

func message(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email"
	case "min":
		return fmt.Sprintf("must be at least %s characters", fe.Param())
	case "max":
		return fmt.Sprintf("must be at most %s characters", fe.Param())
	case "oneof":
		return "must be one of: " + strings.ReplaceAll(fe.Param(), " ", ", ")
	default:
		return "is invalid"
	}
}
