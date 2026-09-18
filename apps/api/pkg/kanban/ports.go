package kanban

import (
	"context"
	"time"
)

// Actor is the caller performing an action, built by the adapter from its own
// identity/session model. IsOwner and Perms carry exactly what Policy needs;
// the core never inspects anything else about the caller.
//
// Perms must be a map built fresh per request. Do not hand Service a map that
// is shared with (and mutated by) some other part of the host application —
// Policy reads it but the core makes no copy.
type Actor struct {
	ID      ActorID
	IsOwner bool
	Perms   map[string]bool
}

// Visibility narrows a board read to either every task in the tenant, or only
// tasks where the given actor is the creator or the assignee. It is produced
// by Policy.Visibility and consumed by TaskRepository.ListBoard: the read
// rule lives in exactly one place (DefaultPolicy), and a repository adapter
// only translates it into a query, it never re-derives the rule from Actor.
type Visibility struct {
	// All, when true, means every task in the tenant is visible; Participant
	// is then ignored.
	All bool
	// Participant is the actor whose created-or-assigned tasks are visible,
	// meaningful only when All is false.
	Participant ActorID
}

// ColumnPatch carries optional field updates for UpdateColumn. A nil field
// means "leave unchanged"; this is what lets a single use-case cover both a
// rename and an IsDone toggle (or both at once) without two different
// intents.
type ColumnPatch struct {
	Name   *string
	IsDone *bool
	Color  *string
}

// BoardFilter narrows ListBoard beyond Visibility. Zero value applies no
// filter. Fields are ANDed together and with Visibility; a caller sets only
// the ones meaningful to the filter it is expressing (see Service.Board).
type BoardFilter struct {
	// AssigneeID, when non-nil, restricts to tasks assigned to that actor.
	AssigneeID *ActorID
	// Unassigned restricts to tasks with no assignee.
	Unassigned bool
	// DueBefore, when non-nil, restricts to tasks whose DueOn is before this
	// calendar date (time-of-day is ignored).
	DueBefore *time.Time
	// DueOn, when non-nil, restricts to tasks whose DueOn is this calendar
	// date (time-of-day is ignored).
	DueOn *time.Time
	// OpenOnly restricts to tasks with no CompletedAt.
	OpenOnly bool
}

// BoardCounts summarizes open (CompletedAt nil) tasks within a Visibility
// scope, independent of any BoardFilter and unbounded by ListBoard's
// per-column limit. ByAssignee omits any actor with a zero count.
type BoardCounts struct {
	All        int
	Overdue    int
	Today      int
	Unassigned int
	ByAssignee map[ActorID]int
}

// ColumnRepository persists Column entities for one tenant. Every method
// takes tenant as its first parameter after ctx, so an adapter cannot
// accidentally query across tenants: the tenant boundary is enforced by the
// method signature, not by adapter discipline.
type ColumnRepository interface {
	// List returns every column of tenant, in no particular order; callers
	// that need Position order sort client-side.
	List(ctx context.Context, tenant TenantID) ([]Column, error)
	// Get returns the column identified by id within tenant. It returns
	// ErrColumnNotFound when id does not exist, or exists under a different
	// tenant.
	Get(ctx context.Context, tenant TenantID, id ColumnID) (Column, error)
	Create(ctx context.Context, tenant TenantID, col Column) (Column, error)
	Update(ctx context.Context, tenant TenantID, col Column) (Column, error)
	// Delete removes the column. Callers must have already moved out any
	// tasks it held (see Service.DeleteColumn).
	Delete(ctx context.Context, tenant TenantID, id ColumnID) error
	// UpdatePositions persists a full ordering of the tenant's columns as
	// returned by ReorderColumns.
	UpdatePositions(ctx context.Context, tenant TenantID, order []ColumnID) error
	// CountByTenant returns how many columns the tenant currently has. This
	// is a check-then-act read with respect to CreateColumn's cap
	// enforcement: see README.md "Column cap contract" for why the adapter,
	// not the core, must serialize concurrent creates per tenant.
	CountByTenant(ctx context.Context, tenant TenantID) (int, error)
	// ExistsName reports whether tenant already has a column named name,
	// compared case-insensitively.
	ExistsName(ctx context.Context, tenant TenantID, name string) (bool, error)
}

// TaskPosition is one live task's ordering key inside a column: the minimum
// a repository needs to return for MoveTask to place a task between two
// neighbours without loading whole Task rows.
type TaskPosition struct {
	ID       TaskID
	Position float64
}

// TaskRepository persists Task entities for one tenant. As with
// ColumnRepository, tenant is always the first parameter after ctx.
type TaskRepository interface {
	// ListBoard returns tasks for tenant narrowed by vis and filter,
	// excluding soft-deleted tasks, ordered by column then Position then
	// creation time. filter is ANDed with vis (see BoardFilter). limit, when
	// > 0, caps how many tasks are returned per column (still following that
	// same order, applied after filtering) rather than the whole tenant; 0
	// means no per-column cap.
	ListBoard(ctx context.Context, tenant TenantID, vis Visibility, filter BoardFilter, limit int) ([]Task, error)
	// CountBoard summarizes open tasks visible under vis as of today (a
	// calendar date; time-of-day is ignored), independent of any
	// BoardFilter and unbounded by ListBoard's limit.
	CountBoard(ctx context.Context, tenant TenantID, vis Visibility, today time.Time) (BoardCounts, error)
	// GetDeleted returns the task identified by id within tenant, but only
	// when it is soft-deleted — the mirror image of Get, which excludes
	// soft-deleted tasks. It returns ErrTaskNotFound when id does not exist,
	// exists under a different tenant, or is still live.
	GetDeleted(ctx context.Context, tenant TenantID, id TaskID) (Task, error)
	// Restore clears the soft-delete marker on the task identified by id
	// within tenant, leaving every other field (ColumnID, Position,
	// CompletedAt, ...) exactly as it was when deleted. It returns
	// ErrTaskNotFound when id does not exist, exists under a different
	// tenant, or is still live.
	Restore(ctx context.Context, tenant TenantID, id TaskID) error
	// MinPositionInColumn returns the lowest Position among col's tasks,
	// excluding soft-deleted ones — only live tasks count, since Position
	// ordering is a live-board concept. found is false when col holds no
	// live task, in which case minPos is meaningless.
	MinPositionInColumn(ctx context.Context, tenant TenantID, col ColumnID) (minPos float64, found bool, err error)
	// ListColumnPositions returns the live tasks of col ordered like
	// ListBoard (Position, then creation time), reduced to id and position —
	// the ordering key MoveTask needs to place a task between two
	// neighbours. It must be called inside a UnitOfWork and must serialize
	// concurrent callers on the same column for the rest of that
	// transaction (Postgres: pg_advisory_xact_lock keyed on tenant+column),
	// so two moves cannot compute the same midpoint from the same snapshot.
	ListColumnPositions(ctx context.Context, tenant TenantID, col ColumnID) ([]TaskPosition, error)
	// RenormalizeColumn rewrites Position to 0..len(order)-1 following order,
	// which lists every live task of col exactly once. It is called inside
	// the caller's transaction when float gaps get too small to bisect.
	RenormalizeColumn(ctx context.Context, tenant TenantID, col ColumnID, order []TaskID) error
	// Get returns the task identified by id within tenant, excluding
	// soft-deleted tasks. It returns ErrTaskNotFound when id does not exist,
	// exists under a different tenant, or is soft-deleted.
	Get(ctx context.Context, tenant TenantID, id TaskID) (Task, error)
	Create(ctx context.Context, tenant TenantID, task Task) (Task, error)
	Update(ctx context.Context, tenant TenantID, task Task) (Task, error)
	SoftDelete(ctx context.Context, tenant TenantID, id TaskID) error
	// CountInColumn counts tasks in col, including soft-deleted ones. A
	// soft-deleted task still holds a foreign key to its column at the
	// database layer (typically FK RESTRICT), so ignoring it here would let
	// DeleteColumn report the column empty and then fail at the database.
	CountInColumn(ctx context.Context, tenant TenantID, col ColumnID) (int, error)
	// MoveAllToColumn reassigns every task in from (including soft-deleted
	// ones, for the same reason as CountInColumn) to to, and reports how many
	// rows moved. completedAt is applied to every moved task: non-nil when
	// the destination column IsDone, nil otherwise.
	MoveAllToColumn(ctx context.Context, tenant TenantID, from, to ColumnID, completedAt *time.Time) (int, error)
	// UnassignBy clears AssigneeID on every task in tenant currently assigned
	// to actor, and reports how many rows changed.
	UnassignBy(ctx context.Context, tenant TenantID, actor ActorID) (int, error)
	// ReassignCreator reassigns CreatedBy from `from` to `to` on every task in
	// tenant created by `from`, and reports how many rows changed.
	ReassignCreator(ctx context.Context, tenant TenantID, from, to ActorID) (int, error)
}

// Repositories groups the two repository ports so NewService takes one
// parameter instead of two.
type Repositories struct {
	Columns ColumnRepository
	Tasks   TaskRepository
}

// UnitOfWork runs fn atomically. Its shape mirrors the host application's own
// transaction manager (Within joins an ambient transaction found in ctx, or
// opens a new one) so an adapter can wrap its existing manager directly
// without an adjustment layer.
type UnitOfWork interface {
	Within(ctx context.Context, fn func(ctx context.Context) error) error
}

// Policy is the single place that decides what an Actor may do. Service
// enforces Policy itself rather than trusting a caller-side check, because
// this library may be reused by a host with no equivalent middleware.
type Policy interface {
	// CanManageBoard reports whether actor may create/rename/reorder/delete
	// columns in tenant.
	CanManageBoard(actor Actor, tenant TenantID) bool
	// CanReadTask reports whether actor may see task.
	CanReadTask(actor Actor, task Task) bool
	// CanWriteTask reports whether actor may edit or delete task.
	CanWriteTask(actor Actor, task Task) bool
	// CanMoveTask reports whether actor may change task's column.
	CanMoveTask(actor Actor, task Task) bool
	// Visibility returns the board-read scope for actor in tenant, consumed
	// by TaskRepository.ListBoard.
	Visibility(actor Actor, tenant TenantID) Visibility
}

// MemberChecker tells whether an ActorID is a live member of a tenant. It is
// a narrow port (ISP) so CreateTask/UpdateTask can reject an assignee who is
// not a tenant member without the core knowing what a "member" table is.
type MemberChecker interface {
	IsMember(ctx context.Context, tenant TenantID, actor ActorID) (bool, error)
}

// EventSink publishes domain events. Publish is called only after a
// use-case's own UnitOfWork.Within has returned nil (see Service.DeleteColumn)
// so nothing is ever announced before its transaction commits. The parameter
// is `any` rather than a closed event interface so a future event type can be
// added without changing this signature.
type EventSink interface {
	Publish(ctx context.Context, event any)
}

// Clock supplies the current time, so tests can inject a fixed value instead
// of asserting against wall-clock time.
type Clock interface {
	Now() time.Time
}
