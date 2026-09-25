package paths

import (
	"errors"
	"net/http"

	"teka/apps/api/internal/shared/apperror"
)

// ErrNotFound marks a learning path that does not exist under the caller's
// center; a cross-center id is indistinguishable from a missing one.
var ErrNotFound = errors.New("learning path not found")

// ErrStageNotFound marks a stage that does not belong to the given path of
// the caller's center.
var ErrStageNotFound = errors.New("path stage not found")

// Error codes the web client branches on.
const (
	// CodeCodeTaken: another live path of the center uses this code.
	CodeCodeTaken = "CODE_TAKEN"
)

func errCodeTaken() *apperror.AppError {
	return apperror.New(CodeCodeTaken, http.StatusConflict,
		"Mã lộ trình đã được dùng trong trung tâm")
}

// notFound maps the repository sentinels onto 404s and passes every other
// error through.
func notFound(err error) error {
	switch {
	case errors.Is(err, ErrNotFound):
		return apperror.NotFound("learning path")
	case errors.Is(err, ErrStageNotFound):
		return apperror.NotFound("path stage")
	}
	return err
}
