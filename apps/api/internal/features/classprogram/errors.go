package classprogram

import (
	"net/http"
	"strconv"

	"teka/apps/api/internal/features/teaching"
	"teka/apps/api/internal/shared/apperror"
)

// CodeCurriculumDiffers marks a class whose curriculum already holds a
// lesson list that differs from the template's; the client re-sends with
// confirm=true to overwrite it. fields carries both counts for the dialog.
const CodeCurriculumDiffers = "CURRICULUM_DIFFERS"

func errCurriculumDiffers(currentCount, templateCount int) *apperror.AppError {
	e := apperror.New(CodeCurriculumDiffers, http.StatusConflict,
		"Sổ đầu bài của lớp đang có danh sách bài khác với chương trình mẫu, xác nhận để thay thế")
	e.Fields = map[string]string{
		"current_count":  strconv.Itoa(currentCount),
		"template_count": strconv.Itoa(templateCount),
	}
	return e
}

// CodeTooManyLessons marks a template whose lesson count exceeds what a class
// curriculum holds; applying it would leave the classbook unsaveable.
const CodeTooManyLessons = "TEMPLATE_TOO_LONG"

func errTooManyLessons(count int) *apperror.AppError {
	e := apperror.New(CodeTooManyLessons, http.StatusUnprocessableEntity,
		"Chương trình mẫu có "+strconv.Itoa(count)+" buổi, vượt quá "+strconv.Itoa(teaching.MaxCurriculumLessons)+" buổi mà sổ đầu bài giữ được")
	e.Fields = map[string]string{
		"lesson_count": strconv.Itoa(count),
		"max_lessons":  strconv.Itoa(teaching.MaxCurriculumLessons),
	}
	return e
}

func errOwnerOnly() *apperror.AppError {
	return apperror.Forbidden("Chỉ chủ trung tâm mới áp dụng hoặc gỡ chương trình của lớp")
}
