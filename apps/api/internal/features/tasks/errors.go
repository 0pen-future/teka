package tasks

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/pkg/kanban"
)

// translateDBError maps a Postgres constraint violation surfaced by a write
// against task_columns/tasks onto the matching kanban sentinel, following
// the same shape as centers/repository.go's translateError: switch on
// pgconn.PgError's constraint name, fall through unchanged otherwise. Only
// errs originating from this package's own INSERT/UPDATE statements ever
// reach here, so the constraint names are exactly the ones migration
// 000022_task_board.up.sql declares.
func translateDBError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	switch pgErr.Code {
	case "23505": // unique_violation
		if pgErr.ConstraintName == "uq_task_columns_name" {
			return kanban.ErrDuplicateColumnName
		}
	case "23503": // foreign_key_violation
		switch pgErr.ConstraintName {
		case "fk_tasks_column_center":
			// column_id has ON DELETE RESTRICT: a column still referenced by a
			// task (including a soft-deleted one) cannot be removed.
			return kanban.ErrColumnNotEmpty
		case "fk_tasks_creator_center", "fk_tasks_assignee_center":
			return kanban.ErrAssigneeNotMember
		}
	}
	return err
}

// translateError maps a kanban sentinel error onto the HTTP-facing
// *apperror.AppError the API contract pins for it. Everything else (a raw DB
// error that slipped past translateDBError, an unexpected error from a
// dependency) becomes apperror.From's generic 500.
func translateError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, kanban.ErrColumnNotFound):
		return apperror.NotFound("column")
	case errors.Is(err, kanban.ErrTaskNotFound):
		return apperror.NotFound("task")
	case errors.Is(err, kanban.ErrColumnLimit):
		return apperror.Invalid("column limit reached", map[string]string{"name": "column limit reached"})
	case errors.Is(err, kanban.ErrDuplicateColumnName):
		return apperror.Invalid("a column with this name already exists", map[string]string{"name": "a column with this name already exists"})
	case errors.Is(err, kanban.ErrColumnNotEmpty):
		return apperror.Conflict("column still has tasks; specify move_to")
	case errors.Is(err, kanban.ErrLastColumn):
		return apperror.Conflict("cannot delete the last column")
	case errors.Is(err, kanban.ErrInvalidPermutation):
		return apperror.Invalid("order must be a permutation of the current columns", map[string]string{"ids": "must list every current column exactly once"})
	case errors.Is(err, kanban.ErrAssigneeNotMember):
		return apperror.Invalid("assignee is not a member of this center", map[string]string{"assignee_id": "must be a live member of this center"})
	case errors.Is(err, kanban.ErrForbidden):
		return apperror.Forbidden("you are not allowed to perform this action")
	case errors.Is(err, kanban.ErrEmptyName):
		return apperror.Invalid("validation failed", map[string]string{"name": "không được để trống"})
	case errors.Is(err, kanban.ErrEmptyTitle):
		return apperror.Invalid("validation failed", map[string]string{"title": "không được để trống"})
	case errors.Is(err, kanban.ErrMoveToSelf):
		return apperror.Invalid("validation failed", map[string]string{"move_to": "phải là một cột khác"})
	case errors.Is(err, kanban.ErrInvalidAfterTask):
		return apperror.Invalid("validation failed", map[string]string{"after_task_id": "phải là việc đang nằm trong cột đích"})
	case errors.Is(err, kanban.ErrInvalidInput):
		return apperror.Invalid("invalid input", nil)
	default:
		return apperror.From(err)
	}
}
