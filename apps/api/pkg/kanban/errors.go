package kanban

import (
	"errors"
	"fmt"
)

// Sentinel errors returned by Service use-cases. There is deliberately no
// "cross-tenant" error: every repository method is bound to a TenantID, so an
// id belonging to another tenant is indistinguishable from a missing one and
// surfaces as ErrColumnNotFound / ErrTaskNotFound. That keeps a caller from
// learning that an object exists somewhere else.
var (
	// ErrColumnNotFound is returned when a column id does not resolve within
	// the caller's tenant.
	ErrColumnNotFound = errors.New("kanban: column not found")
	// ErrTaskNotFound is returned when a task id does not resolve within the
	// caller's tenant.
	ErrTaskNotFound = errors.New("kanban: task not found")
	// ErrColumnLimit is returned by CreateColumn when the tenant already has
	// Config.MaxColumns columns.
	ErrColumnLimit = errors.New("kanban: column limit reached")
	// ErrDuplicateColumnName is returned by CreateColumn when a column with
	// the same name (case-insensitive) already exists for the tenant.
	ErrDuplicateColumnName = errors.New("kanban: duplicate column name")
	// ErrColumnNotEmpty is returned by DeleteColumn when the column holds
	// tasks and no moveTo target was given.
	ErrColumnNotEmpty = errors.New("kanban: column not empty")
	// ErrLastColumn is returned by DeleteColumn when it is the tenant's only
	// column.
	ErrLastColumn = errors.New("kanban: cannot delete the last column")
	// ErrInvalidPermutation is returned by ReorderColumns when the incoming
	// id list is not a permutation of the tenant's current column ids.
	ErrInvalidPermutation = errors.New("kanban: invalid column permutation")
	// ErrAssigneeNotMember is returned by CreateTask/UpdateTask when the
	// requested assignee is not a live member of the tenant.
	ErrAssigneeNotMember = errors.New("kanban: assignee is not a tenant member")
	// ErrForbidden is returned when Policy denies the requested action.
	ErrForbidden = errors.New("kanban: forbidden")
	// ErrInvalidInput is returned for malformed input not covered by a more
	// specific sentinel (e.g. a column name over the length cap).
	ErrInvalidInput = errors.New("kanban: invalid input")
	// ErrEmptyName wraps ErrInvalidInput for an empty (post-trim) column
	// name, so an adapter can report it on the "name" field instead of
	// falling back to a generic message.
	ErrEmptyName = fmt.Errorf("%w: column name is empty", ErrInvalidInput)
	// ErrEmptyTitle wraps ErrInvalidInput for an empty (post-trim) task
	// title, so an adapter can report it on the "title" field.
	ErrEmptyTitle = fmt.Errorf("%w: task title is empty", ErrInvalidInput)
	// ErrMoveToSelf wraps ErrInvalidInput for DeleteColumn's moveTo naming
	// the column being deleted, so an adapter can report it on the
	// "move_to" field.
	ErrMoveToSelf = fmt.Errorf("%w: move target is the column being deleted", ErrInvalidInput)
)
