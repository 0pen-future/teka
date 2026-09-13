// Package tasks adapts the framework-free pkg/kanban core to Teka: GORM
// repositories over task_columns/tasks (migration 000022), a Unit of Work
// mirroring database.TxManager, a policy adapter translating authctx.Scope
// into a kanban.Actor, and Gin handlers exposing the task board API. Object-
// level authorization (owner/creator/assignee rules) lives entirely in
// pkg/kanban; this package only decides whether a caller may reach a Service
// method at all, via the routespec capability tier.
package tasks

import (
	"time"

	"github.com/google/uuid"

	"teka/apps/api/pkg/kanban"
)

// columnModel is the GORM row for task_columns (migration 000022). It exists
// only to carry data across the database boundary — every other layer of
// this package works with kanban.Column.
type columnModel struct {
	ID        uuid.UUID `gorm:"column:id"`
	CenterID  uuid.UUID `gorm:"column:center_id"`
	Name      string    `gorm:"column:name"`
	Position  int       `gorm:"column:position"`
	IsDone    bool      `gorm:"column:is_done"`
	CreatedAt time.Time `gorm:"column:created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

// TableName implements gorm's Tabler.
func (columnModel) TableName() string { return "task_columns" }

// taskModel is the GORM row for tasks (migration 000022). DeletedAt is a
// plain *time.Time, not gorm.DeletedAt: soft-delete filtering here is fully
// manual (CountInColumn/MoveAllToColumn must see soft-deleted rows too, see
// pkg/kanban/ports.go), and gorm.DeletedAt would inject an automatic
// "deleted_at IS NULL" this package does not want on every query.
type taskModel struct {
	ID          uuid.UUID  `gorm:"column:id"`
	CenterID    uuid.UUID  `gorm:"column:center_id"`
	ColumnID    uuid.UUID  `gorm:"column:column_id"`
	CreatedBy   uuid.UUID  `gorm:"column:created_by"`
	AssigneeID  *uuid.UUID `gorm:"column:assignee_id"`
	Title       string     `gorm:"column:title"`
	Description string     `gorm:"column:description"`
	Priority    string     `gorm:"column:priority"`
	DueOn       *time.Time `gorm:"column:due_on"`
	Position    float64    `gorm:"column:position"`
	CompletedAt *time.Time `gorm:"column:completed_at"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at"`
	DeletedAt   *time.Time `gorm:"column:deleted_at"`
}

// TableName implements gorm's Tabler.
func (taskModel) TableName() string { return "tasks" }

// Priority wire/DB values. These are also the exact JSON strings the API
// contract serves for Task.Priority, and the only four values the priority
// column's CHECK constraint accepts. The column's own default ('none')
// matches kanban.PriorityNone's zero value, but is never actually read back
// — every insert sets Priority explicitly from a kanban.Priority.
const (
	priorityNone   = "none"
	priorityLow    = "low"
	priorityMedium = "medium"
	priorityHigh   = "high"
)

// priorityToString converts a core kanban.Priority to its DB/wire string.
func priorityToString(p kanban.Priority) string {
	switch p {
	case kanban.PriorityLow:
		return priorityLow
	case kanban.PriorityMedium:
		return priorityMedium
	case kanban.PriorityHigh:
		return priorityHigh
	default:
		return priorityNone
	}
}

// priorityFromString converts a DB/wire string to a core kanban.Priority.
// Any value this package did not itself write (there is none, since every
// write goes through priorityToString) falls back to PriorityNone rather
// than erroring — a row must never become unreadable because of its
// priority column.
func priorityFromString(s string) kanban.Priority {
	switch s {
	case priorityLow:
		return kanban.PriorityLow
	case priorityMedium:
		return kanban.PriorityMedium
	case priorityHigh:
		return kanban.PriorityHigh
	default:
		return kanban.PriorityNone
	}
}

// columnToCore converts a persisted row to the core entity.
func columnToCore(m columnModel) kanban.Column {
	return kanban.Column{
		ID:        kanban.ColumnID(m.ID),
		TenantID:  kanban.TenantID(m.CenterID),
		Name:      m.Name,
		Position:  m.Position,
		IsDone:    m.IsDone,
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
	}
}

// columnFromCore converts a core entity to the row shape used for
// inserts/updates.
func columnFromCore(c kanban.Column) columnModel {
	return columnModel{
		ID:        uuid.UUID(c.ID),
		CenterID:  uuid.UUID(c.TenantID),
		Name:      c.Name,
		Position:  c.Position,
		IsDone:    c.IsDone,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

// taskToCore converts a persisted row to the core entity.
func taskToCore(m taskModel) kanban.Task {
	var assignee *kanban.ActorID
	if m.AssigneeID != nil {
		a := kanban.ActorID(*m.AssigneeID)
		assignee = &a
	}
	return kanban.Task{
		ID:          kanban.TaskID(m.ID),
		TenantID:    kanban.TenantID(m.CenterID),
		ColumnID:    kanban.ColumnID(m.ColumnID),
		CreatedBy:   kanban.ActorID(m.CreatedBy),
		AssigneeID:  assignee,
		Title:       m.Title,
		Description: m.Description,
		Priority:    priorityFromString(m.Priority),
		DueOn:       m.DueOn,
		Position:    m.Position,
		CompletedAt: m.CompletedAt,
		CreatedAt:   m.CreatedAt,
		UpdatedAt:   m.UpdatedAt,
	}
}

// taskFromCore converts a core entity to the row shape used for
// inserts/updates. DeletedAt is intentionally never set here: soft-delete
// happens only through the repository's dedicated SoftDelete statement.
func taskFromCore(t kanban.Task) taskModel {
	var assignee *uuid.UUID
	if t.AssigneeID != nil {
		a := uuid.UUID(*t.AssigneeID)
		assignee = &a
	}
	return taskModel{
		ID:          uuid.UUID(t.ID),
		CenterID:    uuid.UUID(t.TenantID),
		ColumnID:    uuid.UUID(t.ColumnID),
		CreatedBy:   uuid.UUID(t.CreatedBy),
		AssigneeID:  assignee,
		Title:       t.Title,
		Description: t.Description,
		Priority:    priorityToString(t.Priority),
		DueOn:       t.DueOn,
		Position:    t.Position,
		CompletedAt: t.CompletedAt,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}
