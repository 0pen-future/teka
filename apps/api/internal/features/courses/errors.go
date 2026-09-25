package courses

import (
	"errors"
	"net/http"

	"teka/apps/api/internal/shared/apperror"
)

// ErrNotFound marks a course that does not exist under the caller's center;
// a cross-center id is indistinguishable from a missing one.
var ErrNotFound = errors.New("course not found")

// Error codes the web client branches on.
const (
	// CodeCodeTaken: another live course of the center uses this code.
	CodeCodeTaken = "CODE_TAKEN"
	// CodeCourseArchived: the course is already archived.
	CodeCourseArchived = "COURSE_ARCHIVED"
	// CodeCourseInUse: live classes still point at the course, so it
	// cannot be deleted; archive it instead.
	CodeCourseInUse = "COURSE_IN_USE"
	// CodeCourseInPath: a live learning path still recommends the course,
	// so it cannot be deleted; remove it from the path or archive it.
	CodeCourseInPath = "COURSE_IN_PATH"
)

func errCodeTaken() *apperror.AppError {
	return apperror.New(CodeCodeTaken, http.StatusConflict,
		"Mã khóa học đã được dùng trong trung tâm")
}

func errCourseArchived() *apperror.AppError {
	return apperror.New(CodeCourseArchived, http.StatusConflict,
		"Khóa học đã được lưu trữ")
}

func errCourseInUse() *apperror.AppError {
	return apperror.New(CodeCourseInUse, http.StatusConflict,
		"Khóa học đang có lớp gắn vào, hãy lưu trữ thay vì xoá")
}

func errCourseInPath() *apperror.AppError {
	return apperror.New(CodeCourseInPath, http.StatusConflict,
		"Khóa học đang nằm trong lộ trình học, hãy gỡ khỏi lộ trình hoặc lưu trữ thay vì xoá")
}
