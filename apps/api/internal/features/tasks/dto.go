package tasks

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"teka/apps/api/pkg/kanban"
)

// dateLayout is the wire form of due_on.
const dateLayout = "2006-01-02"

// Optional distinguishes a JSON field that was omitted (leave the stored
// value untouched) from one explicitly sent as null (clear it). Mirrors
// teaching/dto.go's Optional[T] — kept package-local rather than shared,
// same as that package's own copy, since it is a small, self-contained
// unmarshal helper rather than a cross-feature abstraction.
type Optional[T any] struct {
	// Set is true when the field appeared in the request body at all.
	Set   bool
	Value *T
}

// UnmarshalJSON marks the field present; a JSON null leaves Value nil.
func (o *Optional[T]) UnmarshalJSON(data []byte) error {
	o.Set = true
	if string(data) == "null" {
		o.Value = nil
		return nil
	}
	return json.Unmarshal(data, &o.Value)
}

// ColumnResponse is the public Column shape.
type ColumnResponse struct {
	ID        uuid.UUID `json:"id"`
	Name      string    `json:"name"`
	Position  int       `json:"position"`
	IsDone    bool      `json:"is_done"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// columnResponseFrom builds a ColumnResponse from a core column.
func columnResponseFrom(c kanban.Column) ColumnResponse {
	return ColumnResponse{
		ID:        uuid.UUID(c.ID),
		Name:      c.Name,
		Position:  c.Position,
		IsDone:    c.IsDone,
		CreatedAt: c.CreatedAt,
		UpdatedAt: c.UpdatedAt,
	}
}

// TaskResponse is the public Task shape.
type TaskResponse struct {
	ID          uuid.UUID  `json:"id"`
	ColumnID    uuid.UUID  `json:"column_id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Priority    string     `json:"priority"`
	DueOn       *string    `json:"due_on"`
	AssigneeID  *uuid.UUID `json:"assignee_id"`
	CreatedBy   uuid.UUID  `json:"created_by"`
	Position    float64    `json:"position"`
	CompletedAt *time.Time `json:"completed_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// taskResponseFrom builds a TaskResponse from a core task.
func taskResponseFrom(t kanban.Task) TaskResponse {
	var dueOn *string
	if t.DueOn != nil {
		s := t.DueOn.Format(dateLayout)
		dueOn = &s
	}
	var assignee *uuid.UUID
	if t.AssigneeID != nil {
		v := uuid.UUID(*t.AssigneeID)
		assignee = &v
	}
	return TaskResponse{
		ID:          uuid.UUID(t.ID),
		ColumnID:    uuid.UUID(t.ColumnID),
		Title:       t.Title,
		Description: t.Description,
		Priority:    priorityToString(t.Priority),
		DueOn:       dueOn,
		AssigneeID:  assignee,
		CreatedBy:   uuid.UUID(t.CreatedBy),
		Position:    t.Position,
		CompletedAt: t.CompletedAt,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}

// BoardColumnResponse is a Column carrying the tasks visible to the caller
// that currently sit in it, capped at boardTasksPerColumn.
type BoardColumnResponse struct {
	ColumnResponse
	Tasks   []TaskResponse `json:"tasks"`
	HasMore bool           `json:"has_more"`
}

// BoardResponse is GET /tasks/board's data payload.
type BoardResponse struct {
	Scope   string                `json:"scope"`
	Columns []BoardColumnResponse `json:"columns"`
}

// CreateColumnRequest adds a column to the board.
type CreateColumnRequest struct {
	Name   string `json:"name" binding:"required,min=1,max=40"`
	IsDone *bool  `json:"is_done"`
}

// UpdateColumnRequest patches a column; a nil field leaves it unchanged.
type UpdateColumnRequest struct {
	Name   *string `json:"name" binding:"omitempty,min=1,max=40"`
	IsDone *bool   `json:"is_done"`
}

// ColumnsResponse wraps a column list ({"columns": [...]}), used by both the
// reorder response and anywhere a plain column list is returned as an
// object rather than a bare array.
type ColumnsResponse struct {
	Columns []ColumnResponse `json:"columns"`
}

// ReorderColumnsRequest is PUT /task-columns/order's body: a full
// permutation of the board's current column ids.
type ReorderColumnsRequest struct {
	IDs []uuid.UUID `json:"ids" binding:"required,min=1,dive,required"`
}

// DeleteColumnResponse reports how many tasks a column delete moved.
type DeleteColumnResponse struct {
	MovedCount int `json:"moved_count"`
}

// CreateTaskRequest adds a task. An absent ColumnID defaults to the board's
// position-0 column, per the API contract. Description is an HTML subset
// (see description.go): the binding cap bounds the raw markup a client may
// send, the service caps the text it carries at maxDescriptionRunes.
type CreateTaskRequest struct {
	Title       string     `json:"title" binding:"required,min=1,max=200"`
	Description string     `json:"description" binding:"max=20000"`
	ColumnID    *uuid.UUID `json:"column_id"`
	AssigneeID  *uuid.UUID `json:"assignee_id"`
	Priority    string     `json:"priority" binding:"omitempty,oneof=none low medium high"`
	DueOn       *string    `json:"due_on" binding:"omitempty,datetime=2006-01-02"`
}

// UpdateTaskRequest patches a task's editable content (not its column — see
// MoveTaskRequest). Title/Description/Priority are plain optionals (absent =
// unchanged; they can never validly become null). AssigneeID/DueOn are
// genuinely nullable domain fields, so they use Optional[T]'s three states:
// absent = unchanged, null = clear, value = set.
type UpdateTaskRequest struct {
	Title       *string             `json:"title" binding:"omitempty,min=1,max=200"`
	Description *string             `json:"description" binding:"omitempty,max=20000"`
	Priority    *string             `json:"priority" binding:"omitempty,oneof=none low medium high"`
	AssigneeID  Optional[uuid.UUID] `json:"assignee_id" swaggertype:"string"`
	DueOn       Optional[string]    `json:"due_on" swaggertype:"string"`
}

// MoveTaskRequest is POST /tasks/:id/move's body. AfterTaskID names the live
// task of ColumnID the moved task should sit directly below; absent or null
// keeps the original behaviour of landing at the top of the column, so
// callers that never send it are unaffected by drag-and-drop ordering.
type MoveTaskRequest struct {
	ColumnID    uuid.UUID           `json:"column_id" binding:"required"`
	AfterTaskID Optional[uuid.UUID] `json:"after_task_id" swaggertype:"string"`
}
