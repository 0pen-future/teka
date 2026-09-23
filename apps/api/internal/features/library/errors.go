package library

import (
	"errors"
	"net/http"

	"teka/apps/api/internal/shared/apperror"
)

// ErrNotFound marks a template, version or lesson that does not exist under
// the caller's center; a cross-center id is indistinguishable from a missing
// one.
var ErrNotFound = errors.New("library row not found")

// Error codes the web client branches on.
const (
	// CodeVersionLocked: the version is published or archived, so its
	// lessons can no longer change.
	CodeVersionLocked = "VERSION_LOCKED"
	// CodeDraftExists: the template already has an open draft; edit or
	// publish it before opening another.
	CodeDraftExists = "DRAFT_EXISTS"
	// CodeVersionNotDraft: only a draft can be published.
	CodeVersionNotDraft = "VERSION_NOT_DRAFT"
	// CodeVersionNotPublished: only a published version can be archived.
	CodeVersionNotPublished = "VERSION_NOT_PUBLISHED"
	// CodeCodeTaken: another live template of the center uses this code.
	CodeCodeTaken = "CODE_TAKEN"
)

func errVersionLocked() *apperror.AppError {
	return apperror.New(CodeVersionLocked, http.StatusConflict,
		"Phiên bản đã phát hành hoặc lưu trữ, không thể sửa buổi học mẫu")
}

func errDraftExists() *apperror.AppError {
	return apperror.New(CodeDraftExists, http.StatusConflict,
		"Chương trình mẫu đang có bản nháp, hãy phát hành hoặc sửa bản nháp đó")
}

func errVersionNotDraft() *apperror.AppError {
	return apperror.New(CodeVersionNotDraft, http.StatusConflict,
		"Chỉ bản nháp mới có thể phát hành")
}

func errVersionNotPublished() *apperror.AppError {
	return apperror.New(CodeVersionNotPublished, http.StatusConflict,
		"Chỉ phiên bản đã phát hành mới có thể lưu trữ")
}

func errCodeTaken() *apperror.AppError {
	return apperror.New(CodeCodeTaken, http.StatusConflict,
		"Mã chương trình mẫu đã được dùng trong trung tâm")
}
