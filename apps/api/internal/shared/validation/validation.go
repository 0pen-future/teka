// Package validation translates gin binding / validator errors into the
// per-field messages of the 422 envelope, and owns the custom validators
// registered on gin's binding engine.
package validation

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"

	"teka/apps/api/internal/shared/apperror"
)

// vnPhonePattern accepts Vietnamese mobile numbers in local (0xxxxxxxxx) or
// E.164 (+84xxxxxxxxx) form: prefix 3/5/7/8/9 followed by 8 digits.
var vnPhonePattern = regexp.MustCompile(`^(0|\+84)(3|5|7|8|9)\d{8}$`)

// hhmmPattern accepts a 24-hour wall-clock time, "HH:MM". Postgres TIME
// columns travel through the API in this form — never as time.Time, which
// would drag a date and timezone along.
var hhmmPattern = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// init registers the custom validators and the json-tag field naming on gin's
// binding engine. Handlers import this package for BindError, so registration
// is guaranteed to run before any request binding.
func init() {
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		_ = v.RegisterValidation("vnphone", func(fl validator.FieldLevel) bool {
			return vnPhonePattern.MatchString(fl.Field().String())
		})
		_ = v.RegisterValidation("hhmm", func(fl validator.FieldLevel) bool {
			return hhmmPattern.MatchString(fl.Field().String())
		})
		// Error field keys must match the request JSON ("full_name"), not the
		// Go field name ("FullName").
		v.RegisterTagNameFunc(func(fld reflect.StructField) string {
			name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
			if name == "" || name == "-" {
				return fld.Name
			}
			return name
		})
	}
}

// NormalizePhone converts a local-form Vietnamese number (0xxxxxxxxx) to
// E.164 (+84xxxxxxxxx); already-normalized input passes through unchanged.
// Storage and lookups always use the E.164 form so the two input spellings
// resolve to one account.
func NormalizePhone(phone string) string {
	if strings.HasPrefix(phone, "0") {
		return "+84" + phone[1:]
	}
	return phone
}

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

// fieldName exposes the request JSON name, supplied by the RegisterTagNameFunc
// in init; the lowercase fallback only fires for fields with no json tag.
func fieldName(fe validator.FieldError) string {
	return strings.ToLower(fe.Field())
}

func message(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email"
	case "vnphone":
		return "must be a valid Vietnamese phone number"
	case "hhmm":
		return "must be a time in HH:MM form"
	case "datetime":
		return "must be a date in YYYY-MM-DD form"
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
