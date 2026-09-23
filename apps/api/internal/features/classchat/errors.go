package classchat

import (
	"errors"

	"teka/apps/api/internal/shared/apperror"
)

// ErrNotFound marks a message that does not exist under the class, was
// already retracted, or belongs to another center.
var ErrNotFound = errors.New("class message not found")

func errNotOnClass() *apperror.AppError {
	return apperror.Forbidden("Bạn không còn phụ trách lớp này")
}

func errNotAuthor() *apperror.AppError {
	return apperror.Forbidden("Chỉ người viết hoặc chủ trung tâm mới thu hồi được tin nhắn")
}

func errBlankBody() *apperror.AppError {
	return apperror.Invalid("Tin nhắn không được để trống", map[string]string{"body": "không được để trống"})
}
