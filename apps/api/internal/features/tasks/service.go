package tasks

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/events"
	"teka/apps/api/internal/shared/id"
	"teka/apps/api/pkg/kanban"
)

// defaultColumnColor is what an omitted CreateColumnRequest.Color resolves
// to — the board's cream default, matching the migration's own column
// default.
const defaultColumnColor = "none"

// boardTasksPerColumn caps how many tasks GET /tasks/board returns per
// column; a column past the cap reports HasMore instead of paging, since v1
// has no board-side pagination UI.
const boardTasksPerColumn = 50

// Service adapts pkg/kanban's framework-free use-cases to Teka: it turns an
// authctx.Scope into a kanban.Actor, a wire DTO into a kanban input, and a
// kanban sentinel error into an apperror. All object-level authorization
// (owner/creator/assignee rules) stays inside core; whether the caller may
// reach a Service method at all is the routespec capability tier, enforced
// by middleware before a handler ever calls here.
type Service struct {
	core *kanban.Service
}

// NewService wires a kanban.Service against db/tx/bus, following the same
// (repo, tx, bus) shape every other feature's NewService uses.
func NewService(db *gorm.DB, tx database.TxManager, bus events.Bus) *Service {
	core := kanban.NewService(
		newRepositories(db),
		newUnitOfWork(tx),
		kanban.DefaultPolicy{},
		newMemberChecker(db),
		newEventSink(bus),
		kanban.WithIDGen(id.New),
	)
	return &Service{core: core}
}

// Board returns the caller's board: every column, the tasks Policy and q
// make visible to them, and counts over that same visible set. Scope reports
// which visibility rule applied ("center" for an owner or a tasks.view_all
// holder, "mine" otherwise) so the client can label the view without
// re-deriving the rule itself.
func (s *Service) Board(ctx context.Context, sc authctx.Scope, q BoardQuery) (BoardResponse, error) {
	tenant := kanban.TenantID(sc.CenterID)
	actor := actorFrom(sc)

	today, err := parseBoardToday(q.Today)
	if err != nil {
		return BoardResponse{}, err
	}
	filter, err := boardFilterFrom(q, actor, today)
	if err != nil {
		return BoardResponse{}, err
	}

	// Ask the core for one task past the display cap per column, so the SQL
	// layer (not a full tenant scan) decides HasMore below.
	cols, tasks, err := s.core.Board(ctx, tenant, actor, filter, boardTasksPerColumn+1)
	if err != nil {
		return BoardResponse{}, translateError(err)
	}
	counts, err := s.core.BoardCounts(ctx, tenant, actor, today)
	if err != nil {
		return BoardResponse{}, translateError(err)
	}

	byColumn := make(map[uuid.UUID][]kanban.Task, len(cols))
	for _, t := range tasks {
		cid := uuid.UUID(t.ColumnID)
		byColumn[cid] = append(byColumn[cid], t)
	}

	columns := make([]BoardColumnResponse, len(cols))
	for i, c := range cols {
		inCol := byColumn[uuid.UUID(c.ID)]
		sort.SliceStable(inCol, func(a, b int) bool { return inCol[a].Position < inCol[b].Position })
		hasMore := len(inCol) > boardTasksPerColumn
		if hasMore {
			inCol = inCol[:boardTasksPerColumn]
		}
		taskResponses := make([]TaskResponse, len(inCol))
		for j, t := range inCol {
			taskResponses[j] = taskResponseFrom(t)
		}
		columns[i] = BoardColumnResponse{
			ColumnResponse: columnResponseFrom(c),
			Tasks:          taskResponses,
			HasMore:        hasMore,
		}
	}

	scope := "mine"
	if actor.IsOwner || actor.Perms[kanban.PermViewAll] {
		scope = "center"
	}
	return BoardResponse{
		Scope:   scope,
		Columns: columns,
		Counts:  boardCountsResponseFrom(counts, actor.ID),
	}, nil
}

// parseBoardToday converts BoardQuery.Today (already format-validated by its
// binding tag) into the calendar date board filtering/counting compares
// against, defaulting to the server's current UTC date when the client sends
// none — see BoardFilter's doc comment on why this is the client's own
// "today", not a server-derived timezone.
func parseBoardToday(today string) (time.Time, error) {
	if today == "" {
		now := time.Now().UTC()
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), nil
	}
	t, err := time.Parse(dateLayout, today)
	if err != nil {
		return time.Time{}, apperror.Invalid("today must be YYYY-MM-DD", map[string]string{"today": "must be YYYY-MM-DD"})
	}
	return t, nil
}

// boardFilterFrom translates q's already-validated Filter/Assignee into a
// kanban.BoardFilter, per the mapping table in the API contract: mine, unlike
// overdue/today/unassigned, does not restrict to open tasks (a completed task
// the caller is assigned to still belongs in their "mine" view). Assignee, when
// present, ANDs an assignee_id predicate onto whatever Filter already set,
// taking precedence over the one "mine" implies.
func boardFilterFrom(q BoardQuery, actor kanban.Actor, today time.Time) (kanban.BoardFilter, error) {
	var filter kanban.BoardFilter
	switch q.Filter {
	case "mine":
		v := actor.ID
		filter.AssigneeID = &v
	case "overdue":
		v := today
		filter.DueBefore = &v
		filter.OpenOnly = true
	case "today":
		v := today
		filter.DueOn = &v
		filter.OpenOnly = true
	case "unassigned":
		filter.Unassigned = true
		filter.OpenOnly = true
	}
	if q.Assignee != "" {
		parsed, err := uuid.Parse(q.Assignee)
		if err != nil {
			return kanban.BoardFilter{}, apperror.Invalid("assignee must be a uuid", map[string]string{"assignee": "must be a uuid"})
		}
		v := kanban.ActorID(parsed)
		filter.AssigneeID = &v
		filter.Unassigned = false
	}
	return filter, nil
}

// boardCountsResponseFrom builds a BoardCountsResponse from the core's
// tenant-wide counts, deriving Mine from ByAssignee[actorID] (mine is not a
// core concept — see kanban.Service.Board's doc comment) and sorting
// ByAssignee by count descending, then teacher id, so the response is
// deterministic across calls.
func boardCountsResponseFrom(counts kanban.BoardCounts, actorID kanban.ActorID) BoardCountsResponse {
	byAssignee := make([]AssigneeCountResponse, 0, len(counts.ByAssignee))
	for assigneeID, count := range counts.ByAssignee {
		byAssignee = append(byAssignee, AssigneeCountResponse{TeacherID: uuid.UUID(assigneeID), Count: count})
	}
	sort.Slice(byAssignee, func(i, j int) bool {
		if byAssignee[i].Count != byAssignee[j].Count {
			return byAssignee[i].Count > byAssignee[j].Count
		}
		return byAssignee[i].TeacherID.String() < byAssignee[j].TeacherID.String()
	})
	return BoardCountsResponse{
		All:        counts.All,
		Mine:       counts.ByAssignee[actorID],
		Overdue:    counts.Overdue,
		Today:      counts.Today,
		Unassigned: counts.Unassigned,
		ByAssignee: byAssignee,
	}
}

// CreateColumn adds a column to the caller's board. An absent Color defaults
// to "none", the board's cream default.
func (s *Service) CreateColumn(ctx context.Context, sc authctx.Scope, req CreateColumnRequest) (ColumnResponse, error) {
	isDone := false
	if req.IsDone != nil {
		isDone = *req.IsDone
	}
	color := defaultColumnColor
	if req.Color != nil {
		color = *req.Color
	}
	col, err := s.core.CreateColumn(ctx, kanban.TenantID(sc.CenterID), actorFrom(sc), req.Name, isDone, color)
	if err != nil {
		return ColumnResponse{}, translateError(err)
	}
	return columnResponseFrom(col), nil
}

// UpdateColumn renames a column, toggles its IsDone flag, and/or recolors it.
func (s *Service) UpdateColumn(ctx context.Context, sc authctx.Scope, colID uuid.UUID, req UpdateColumnRequest) (ColumnResponse, error) {
	patch := kanban.ColumnPatch{Name: req.Name, IsDone: req.IsDone, Color: req.Color}
	col, err := s.core.UpdateColumn(ctx, kanban.TenantID(sc.CenterID), actorFrom(sc), kanban.ColumnID(colID), patch)
	if err != nil {
		return ColumnResponse{}, translateError(err)
	}
	return columnResponseFrom(col), nil
}

// ReorderColumns persists a full reordering of the board's columns and
// returns them back in their new order.
func (s *Service) ReorderColumns(ctx context.Context, sc authctx.Scope, req ReorderColumnsRequest) (ColumnsResponse, error) {
	tenant := kanban.TenantID(sc.CenterID)
	actor := actorFrom(sc)
	order := make([]kanban.ColumnID, len(req.IDs))
	for i, id := range req.IDs {
		order[i] = kanban.ColumnID(id)
	}
	if err := s.core.ReorderColumns(ctx, tenant, actor, order); err != nil {
		return ColumnsResponse{}, translateError(err)
	}
	// Only the columns are used here; an empty filter and 0 keep this call's
	// existing unlimited-tasks behavior (out of scope for the per-column cap
	// below).
	cols, _, err := s.core.Board(ctx, tenant, actor, kanban.BoardFilter{}, 0)
	if err != nil {
		return ColumnsResponse{}, translateError(err)
	}
	out := make([]ColumnResponse, len(cols))
	for i, c := range cols {
		out[i] = columnResponseFrom(c)
	}
	return ColumnsResponse{Columns: out}, nil
}

// DeleteColumn removes a column, moving any tasks it held into moveTo (nil
// when the column is expected to be empty). MovedCount in the response comes
// back through the same context channel events.go uses to enrich the
// published event, since kanban.Service.DeleteColumn itself only reports
// success or failure.
func (s *Service) DeleteColumn(ctx context.Context, sc authctx.Scope, colID uuid.UUID, moveTo *uuid.UUID) (DeleteColumnResponse, error) {
	tenant := kanban.TenantID(sc.CenterID)
	actor := actorFrom(sc)
	var target *kanban.ColumnID
	if moveTo != nil {
		v := kanban.ColumnID(*moveTo)
		target = &v
	}
	var moved int
	ctx = withDeleteColumnCaller(ctx, sc.CenterID, sc.TeacherID, &moved)
	if err := s.core.DeleteColumn(ctx, tenant, actor, kanban.ColumnID(colID), target); err != nil {
		return DeleteColumnResponse{}, translateError(err)
	}
	return DeleteColumnResponse{MovedCount: moved}, nil
}

// CreateTask adds a task to the caller's board. A nil req.ColumnID defaults
// to the board's first column (by Position), per the API contract.
func (s *Service) CreateTask(ctx context.Context, sc authctx.Scope, req CreateTaskRequest) (TaskResponse, error) {
	tenant := kanban.TenantID(sc.CenterID)
	actor := actorFrom(sc)

	columnID := req.ColumnID
	if columnID == nil {
		cols, _, err := s.core.Board(ctx, tenant, actor, kanban.BoardFilter{}, 0)
		if err != nil {
			return TaskResponse{}, translateError(err)
		}
		if len(cols) == 0 {
			return TaskResponse{}, apperror.Invalid("board has no columns", map[string]string{"column_id": "required: board has no columns"})
		}
		v := uuid.UUID(cols[0].ID)
		columnID = &v
	}

	dueOn, err := parseDueOn(req.DueOn)
	if err != nil {
		return TaskResponse{}, err
	}
	description, err := normalizeDescription(req.Description)
	if err != nil {
		return TaskResponse{}, translateError(err)
	}
	var assignee *kanban.ActorID
	if req.AssigneeID != nil {
		v := kanban.ActorID(*req.AssigneeID)
		assignee = &v
	}

	in := kanban.CreateTaskInput{
		ColumnID:    kanban.ColumnID(*columnID),
		Title:       req.Title,
		Description: description,
		Priority:    priorityFromString(req.Priority),
		DueOn:       dueOn,
		AssigneeID:  assignee,
	}
	task, err := s.core.CreateTask(ctx, tenant, actor, in)
	if err != nil {
		return TaskResponse{}, translateError(err)
	}
	return taskResponseFrom(task), nil
}

// GetTask returns one task the caller may read.
func (s *Service) GetTask(ctx context.Context, sc authctx.Scope, id uuid.UUID) (TaskResponse, error) {
	task, err := s.core.GetTask(ctx, kanban.TenantID(sc.CenterID), actorFrom(sc), kanban.TaskID(id))
	if err != nil {
		return TaskResponse{}, translateError(err)
	}
	return taskResponseFrom(task), nil
}

// UpdateTask patches a task's editable content. Title/Description/Priority
// are plain optionals; AssigneeID/DueOn use Optional[T]'s three states. A
// sent description is normalized like on create, so an update can never
// store markup outside the allowlist. Since
// kanban.UpdateTaskInput replaces a task's whole editable state at once (see
// pkg/kanban/entity.go), this loads the current task first and merges the
// patch on top of it — CanWriteTask (owner or creator) is a subset of
// CanReadTask, so a caller who may write can always read first.
func (s *Service) UpdateTask(ctx context.Context, sc authctx.Scope, id uuid.UUID, req UpdateTaskRequest) (TaskResponse, error) {
	tenant := kanban.TenantID(sc.CenterID)
	actor := actorFrom(sc)

	current, err := s.core.GetTask(ctx, tenant, actor, kanban.TaskID(id))
	if err != nil {
		return TaskResponse{}, translateError(err)
	}

	title := current.Title
	if req.Title != nil {
		title = *req.Title
	}
	description := current.Description
	if req.Description != nil {
		description, err = normalizeDescription(*req.Description)
		if err != nil {
			return TaskResponse{}, translateError(err)
		}
	}
	priority := current.Priority
	if req.Priority != nil {
		priority = priorityFromString(*req.Priority)
	}
	dueOn := current.DueOn
	if req.DueOn.Set {
		dueOn, err = parseDueOn(req.DueOn.Value)
		if err != nil {
			return TaskResponse{}, err
		}
	}
	assignee := current.AssigneeID
	if req.AssigneeID.Set {
		assignee = nil
		if req.AssigneeID.Value != nil {
			v := kanban.ActorID(*req.AssigneeID.Value)
			assignee = &v
		}
	}

	in := kanban.UpdateTaskInput{
		Title:       title,
		Description: description,
		Priority:    priority,
		DueOn:       dueOn,
		AssigneeID:  assignee,
	}
	updated, err := s.core.UpdateTask(ctx, tenant, actor, kanban.TaskID(id), in)
	if err != nil {
		return TaskResponse{}, translateError(err)
	}
	return taskResponseFrom(updated), nil
}

// MoveTask changes a task's column and/or its place in it: at the top when
// after_task_id is absent or null, directly below that task otherwise.
func (s *Service) MoveTask(ctx context.Context, sc authctx.Scope, id uuid.UUID, req MoveTaskRequest) (TaskResponse, error) {
	var after *kanban.TaskID
	if req.AfterTaskID.Set && req.AfterTaskID.Value != nil {
		anchor := kanban.TaskID(*req.AfterTaskID.Value)
		after = &anchor
	}
	task, err := s.core.MoveTask(ctx, kanban.TenantID(sc.CenterID), actorFrom(sc), kanban.TaskID(id), kanban.ColumnID(req.ColumnID), after)
	if err != nil {
		return TaskResponse{}, translateError(err)
	}
	return taskResponseFrom(task), nil
}

// DeleteTask soft-deletes a task.
func (s *Service) DeleteTask(ctx context.Context, sc authctx.Scope, id uuid.UUID) error {
	err := s.core.DeleteTask(ctx, kanban.TenantID(sc.CenterID), actorFrom(sc), kanban.TaskID(id))
	return translateError(err)
}

// RestoreTask reverses a soft-delete, reviving the task in its original
// column/position/completion state. Requires the same tasks.delete route
// policy and CanWriteTask ownership as DeleteTask itself.
func (s *Service) RestoreTask(ctx context.Context, sc authctx.Scope, id uuid.UUID) (TaskResponse, error) {
	task, err := s.core.RestoreTask(ctx, kanban.TenantID(sc.CenterID), actorFrom(sc), kanban.TaskID(id))
	if err != nil {
		return TaskResponse{}, translateError(err)
	}
	return taskResponseFrom(task), nil
}

// HandoverOnDeparture implements centers.TaskHandover: it unassigns the
// departing member's tasks and reassigns creatorship of the tasks they
// created to newOwner. It must run inside the caller's own ambient
// transaction (see kanban.Service.HandoverOnDeparture) — centers.Service is
// expected to call this before its RemoveMember transaction commits, then
// publish its own event once that commit succeeds.
func (s *Service) HandoverOnDeparture(ctx context.Context, centerID, departed, newOwner uuid.UUID) (unassigned, reassigned int, err error) {
	unassigned, reassigned, err = s.core.HandoverOnDeparture(ctx, kanban.TenantID(centerID), kanban.ActorID(departed), kanban.ActorID(newOwner))
	if err != nil {
		return unassigned, reassigned, translateError(err)
	}
	return unassigned, reassigned, nil
}

// parseDueOn converts a due_on wire value (nil, or YYYY-MM-DD) into the core
// input shape. A non-nil, malformed value would only reach here if a caller
// bypassed request binding, but is still rejected explicitly rather than
// passed through.
func parseDueOn(s *string) (*time.Time, error) {
	if s == nil {
		return nil, nil
	}
	t, err := time.Parse(dateLayout, *s)
	if err != nil {
		return nil, apperror.Invalid("due_on must be YYYY-MM-DD", map[string]string{"due_on": "must be YYYY-MM-DD"})
	}
	return &t, nil
}
