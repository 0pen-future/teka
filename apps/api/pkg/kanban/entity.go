// Package kanban is a hard-boundary domain library for a per-tenant Kanban
// board: configurable columns and tasks moving between them. It knows
// nothing about HTTP, SQL, or any specific host application — see README.md
// for the port contracts an adapter must implement.
package kanban

import (
	"time"

	"github.com/google/uuid"
)

// TenantID identifies the owning tenant (a "center" in Teka, but the lib does
// not know that name). Defined as a distinct type over uuid.UUID, not an
// alias, so the compiler rejects passing one ID kind where another is
// expected.
type TenantID uuid.UUID

// ActorID identifies the person performing an action.
type ActorID uuid.UUID

// TaskID identifies a Task.
type TaskID uuid.UUID

// ColumnID identifies a Column.
type ColumnID uuid.UUID

// Priority is a task's relative urgency. The zero value is PriorityNone.
type Priority int

const (
	// PriorityNone is the zero value: no priority set.
	PriorityNone Priority = iota
	// PriorityLow marks a low-urgency task.
	PriorityLow
	// PriorityMedium marks a medium-urgency task.
	PriorityMedium
	// PriorityHigh marks a high-urgency task.
	PriorityHigh
)

// Column is one stage of a tenant's board. IsDone marks a column whose tasks
// are considered complete: moving a task into such a column sets its
// CompletedAt (see Service.MoveTask). Color is an opaque label an adapter may
// use to tint the column in its UI; the library only bounds its length (see
// validateColor) and never interprets its value.
type Column struct {
	ID        ColumnID
	TenantID  TenantID
	Name      string
	Position  int
	IsDone    bool
	Color     string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Task is one unit of work on a tenant's board. AssigneeID is nil when
// unassigned. Position is a float64 so a task can be inserted before the
// current minimum without renumbering the rest of the column (see README.md
// "Position strategy").
type Task struct {
	ID          TaskID
	TenantID    TenantID
	ColumnID    ColumnID
	CreatedBy   ActorID
	AssigneeID  *ActorID
	Title       string
	Description string
	Priority    Priority
	DueOn       *time.Time
	Position    float64
	CompletedAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
