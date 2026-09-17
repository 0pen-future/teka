package kanban

import (
	"context"
	"sort"
	"strings"
	"time"
)

// CreateTaskInput is the input to Service.CreateTask. ColumnID must resolve
// within the caller's tenant. AssigneeID, when non-nil, must be a live
// tenant member (checked via MemberChecker).
type CreateTaskInput struct {
	ColumnID    ColumnID
	Title       string
	Description string
	Priority    Priority
	DueOn       *time.Time
	AssigneeID  *ActorID
}

// UpdateTaskInput is the input to Service.UpdateTask. It intentionally omits
// ColumnID: changing a task's column is Service.MoveTask's job, gated by
// CanMoveTask rather than CanWriteTask, so the two concerns (edit content vs.
// move) cannot be conflated through a single call. Every field here replaces
// the task's current value — including DueOn and AssigneeID, where nil means
// "no due date" / "unassigned" — because an edit form is expected to submit
// the task's whole editable state at once, which keeps this input a plain
// struct instead of a tri-state patch.
type UpdateTaskInput struct {
	Title       string
	Description string
	Priority    Priority
	DueOn       *time.Time
	AssigneeID  *ActorID
}

// Board returns tenant's columns (ordered by Position) and the tasks actor
// may see, narrowed by Policy.Visibility. There is no separate
// "CanViewBoard" check: which tasks come back is entirely determined by
// Visibility, and whether actor may call Board at all is a capability
// concern the host application enforces before reaching the core (see
// README.md "Policy split"). limitPerColumn is forwarded to
// TaskRepository.ListBoard unchanged: 0 fetches every visible task, a
// positive value caps how many come back per column (a caller that wants to
// report "more tasks exist" per column should ask for one more than it
// intends to display).
func (s *Service) Board(ctx context.Context, tenant TenantID, actor Actor, limitPerColumn int) ([]Column, []Task, error) {
	cols, err := s.repos.Columns.List(ctx, tenant)
	if err != nil {
		return nil, nil, err
	}
	sort.Slice(cols, func(i, j int) bool { return cols[i].Position < cols[j].Position })

	vis := s.policy.Visibility(actor, tenant)
	tasks, err := s.repos.Tasks.ListBoard(ctx, tenant, vis, limitPerColumn)
	if err != nil {
		return nil, nil, err
	}
	return cols, tasks, nil
}

// CreateColumn adds a column to tenant's board, appended after the current
// last position. It requires CanManageBoard, rejects a name that is empty,
// too long, or a case-insensitive duplicate, and enforces Config.MaxColumns.
//
// CountByTenant and the subsequent Create are not atomic from the core's
// point of view (there is no multi-repo state to wrap in UnitOfWork here);
// see README.md "Column cap contract" for why the adapter must serialize
// concurrent creates per tenant to make the cap exact.
func (s *Service) CreateColumn(ctx context.Context, tenant TenantID, actor Actor, name string, isDone bool) (Column, error) {
	if !s.policy.CanManageBoard(actor, tenant) {
		return Column{}, ErrForbidden
	}
	trimmed, err := validateName(name, s.cfg.MaxNameLen)
	if err != nil {
		return Column{}, err
	}
	dup, err := s.repos.Columns.ExistsName(ctx, tenant, trimmed)
	if err != nil {
		return Column{}, err
	}
	if dup {
		return Column{}, ErrDuplicateColumnName
	}
	count, err := s.repos.Columns.CountByTenant(ctx, tenant)
	if err != nil {
		return Column{}, err
	}
	if count >= s.cfg.MaxColumns {
		return Column{}, ErrColumnLimit
	}
	col := Column{
		ID:       ColumnID(s.cfg.idGen()),
		TenantID: tenant,
		Name:     trimmed,
		Position: count,
		IsDone:   isDone,
	}
	return s.repos.Columns.Create(ctx, tenant, col)
}

// UpdateColumn applies patch to the column identified by colID. A nil field
// in patch leaves that field unchanged, which is what lets a rename and an
// IsDone toggle share one intent. Toggling IsDone never touches existing
// tasks in the column (their CompletedAt is set only by MoveTask, at the
// moment a task enters or leaves a done column).
func (s *Service) UpdateColumn(ctx context.Context, tenant TenantID, actor Actor, colID ColumnID, patch ColumnPatch) (Column, error) {
	if !s.policy.CanManageBoard(actor, tenant) {
		return Column{}, ErrForbidden
	}
	col, err := s.repos.Columns.Get(ctx, tenant, colID)
	if err != nil {
		return Column{}, err
	}
	if patch.Name != nil {
		trimmed, err := validateName(*patch.Name, s.cfg.MaxNameLen)
		if err != nil {
			return Column{}, err
		}
		if !strings.EqualFold(trimmed, col.Name) {
			dup, err := s.repos.Columns.ExistsName(ctx, tenant, trimmed)
			if err != nil {
				return Column{}, err
			}
			if dup {
				return Column{}, ErrDuplicateColumnName
			}
		}
		col.Name = trimmed
	}
	if patch.IsDone != nil {
		col.IsDone = *patch.IsDone
	}
	return s.repos.Columns.Update(ctx, tenant, col)
}

// ReorderColumns persists a new column order. order must be a permutation of
// tenant's current column ids — missing or extra ids yield
// ErrInvalidPermutation rather than silently dropping or ignoring one.
func (s *Service) ReorderColumns(ctx context.Context, tenant TenantID, actor Actor, order []ColumnID) error {
	if !s.policy.CanManageBoard(actor, tenant) {
		return ErrForbidden
	}
	current, err := s.repos.Columns.List(ctx, tenant)
	if err != nil {
		return err
	}
	currentIDs := make([]ColumnID, len(current))
	for i, c := range current {
		currentIDs[i] = c.ID
	}
	if err := validatePermutation(currentIDs, order); err != nil {
		return err
	}
	return s.uow.Within(ctx, func(ctx context.Context) error {
		return s.repos.Columns.UpdatePositions(ctx, tenant, order)
	})
}

// DeleteColumn removes the column identified by colID. It refuses to delete
// the tenant's last remaining column (ErrLastColumn). If the column holds any
// task — including soft-deleted ones, since the database still has to
// satisfy a foreign key from them — moveTo must name another column of the
// same tenant to receive them; a nil moveTo then yields ErrColumnNotEmpty,
// and moveTo == colID yields ErrMoveToSelf. The move and the delete happen
// inside one UnitOfWork.Within; ColumnDeleted is published only once that
// returns nil, and never otherwise.
func (s *Service) DeleteColumn(ctx context.Context, tenant TenantID, actor Actor, colID ColumnID, moveTo *ColumnID) error {
	if !s.policy.CanManageBoard(actor, tenant) {
		return ErrForbidden
	}
	if _, err := s.repos.Columns.Get(ctx, tenant, colID); err != nil {
		return err
	}
	all, err := s.repos.Columns.List(ctx, tenant)
	if err != nil {
		return err
	}
	if len(all) <= 1 {
		return ErrLastColumn
	}
	if moveTo != nil && *moveTo == colID {
		return ErrMoveToSelf
	}
	count, err := s.repos.Tasks.CountInColumn(ctx, tenant, colID)
	if err != nil {
		return err
	}
	if count > 0 && moveTo == nil {
		return ErrColumnNotEmpty
	}

	var target *Column
	if moveTo != nil {
		t, err := s.repos.Columns.Get(ctx, tenant, *moveTo)
		if err != nil {
			return err
		}
		target = &t
	}

	var moved int
	err = s.uow.Within(ctx, func(ctx context.Context) error {
		if target != nil {
			completedAt := completedAtIf(s.cfg.clock, target.IsDone)
			n, err := s.repos.Tasks.MoveAllToColumn(ctx, tenant, colID, target.ID, completedAt)
			if err != nil {
				return err
			}
			moved = n
		}
		return s.repos.Columns.Delete(ctx, tenant, colID)
	})
	if err != nil {
		return err
	}

	s.sink.Publish(ctx, ColumnDeleted{ColumnID: colID, MoveTo: moveTo, MovedCount: moved})
	return nil
}

// CreateTask adds a task to in.ColumnID, which must resolve within tenant.
// The new task is placed at the top of its column (see README.md "Position
// strategy"). CreateTask carries no object-level Policy check: whether actor
// may create tasks at all is a capability the host application's own
// middleware enforces (see README.md "Policy split") — the core only
// validates the data.
func (s *Service) CreateTask(ctx context.Context, tenant TenantID, actor Actor, in CreateTaskInput) (Task, error) {
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Task{}, ErrEmptyTitle
	}
	if _, err := s.repos.Columns.Get(ctx, tenant, in.ColumnID); err != nil {
		return Task{}, err
	}
	if err := s.checkAssignee(ctx, tenant, in.AssigneeID); err != nil {
		return Task{}, err
	}
	pos, err := s.topPositionInColumn(ctx, tenant, in.ColumnID)
	if err != nil {
		return Task{}, err
	}
	task := Task{
		ID:          TaskID(s.cfg.idGen()),
		TenantID:    tenant,
		ColumnID:    in.ColumnID,
		CreatedBy:   actor.ID,
		AssigneeID:  in.AssigneeID,
		Title:       title,
		Description: in.Description,
		Priority:    in.Priority,
		DueOn:       in.DueOn,
		Position:    pos,
	}
	return s.repos.Tasks.Create(ctx, tenant, task)
}

// GetTask returns the task identified by id if actor may read it
// (CanReadTask), else ErrForbidden.
func (s *Service) GetTask(ctx context.Context, tenant TenantID, actor Actor, id TaskID) (Task, error) {
	task, err := s.repos.Tasks.Get(ctx, tenant, id)
	if err != nil {
		return Task{}, err
	}
	if !s.policy.CanReadTask(actor, task) {
		return Task{}, ErrForbidden
	}
	return task, nil
}

// UpdateTask replaces the editable content of the task identified by id. It
// requires CanWriteTask (owner or creator); an assignee who is not also the
// creator can move the task (MoveTask) but not edit it.
func (s *Service) UpdateTask(ctx context.Context, tenant TenantID, actor Actor, id TaskID, in UpdateTaskInput) (Task, error) {
	task, err := s.repos.Tasks.Get(ctx, tenant, id)
	if err != nil {
		return Task{}, err
	}
	if !s.policy.CanWriteTask(actor, task) {
		return Task{}, ErrForbidden
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return Task{}, ErrEmptyTitle
	}
	if err := s.checkAssignee(ctx, tenant, in.AssigneeID); err != nil {
		return Task{}, err
	}
	task.Title = title
	task.Description = in.Description
	task.Priority = in.Priority
	task.DueOn = in.DueOn
	task.AssigneeID = in.AssigneeID
	return s.repos.Tasks.Update(ctx, tenant, task)
}

// MoveTask moves the task identified by id into target, which must resolve
// within tenant, and places it in that column. It requires CanMoveTask
// (owner, creator, or assignee). A nil after lands the task at the top of
// the destination column; a non-nil after places it directly after that
// task, which must be a live task of target other than id itself
// (ErrInvalidAfterTask otherwise). The placement and the write run inside
// one UnitOfWork.Within so the neighbours read to compute the position are
// the ones the row is written next to (see README.md "Position strategy").
// CompletedAt is set to Clock.Now() when target.IsDone, and cleared
// otherwise.
func (s *Service) MoveTask(ctx context.Context, tenant TenantID, actor Actor, id TaskID, target ColumnID, after *TaskID) (Task, error) {
	task, err := s.repos.Tasks.Get(ctx, tenant, id)
	if err != nil {
		return Task{}, err
	}
	if !s.policy.CanMoveTask(actor, task) {
		return Task{}, ErrForbidden
	}
	col, err := s.repos.Columns.Get(ctx, tenant, target)
	if err != nil {
		return Task{}, err
	}
	var moved Task
	err = s.uow.Within(ctx, func(ctx context.Context) error {
		pos, err := s.positionInColumn(ctx, tenant, target, id, after)
		if err != nil {
			return err
		}
		task.ColumnID = target
		task.Position = pos
		task.CompletedAt = completedAtIf(s.cfg.clock, col.IsDone)
		moved, err = s.repos.Tasks.Update(ctx, tenant, task)
		return err
	})
	if err != nil {
		return Task{}, err
	}
	return moved, nil
}

// DeleteTask soft-deletes the task identified by id. It requires
// CanWriteTask (owner or creator).
func (s *Service) DeleteTask(ctx context.Context, tenant TenantID, actor Actor, id TaskID) error {
	task, err := s.repos.Tasks.Get(ctx, tenant, id)
	if err != nil {
		return err
	}
	if !s.policy.CanWriteTask(actor, task) {
		return ErrForbidden
	}
	return s.repos.Tasks.SoftDelete(ctx, tenant, id)
}

// HandoverOnDeparture unassigns every task in tenant assigned to departed and
// reassigns creatorship of every task departed created to newOwner. Unlike
// every other use-case, it does NOT open UnitOfWork.Within and does NOT
// publish an event: it is designed to run inside the caller's own ambient
// transaction (e.g. a "remove member" flow), so the caller owns the commit
// and therefore owns the event it publishes afterward. Calling this from
// outside such a transaction, or expecting it to publish anything, is a
// misuse of the contract — see README.md "Consumer-owned transactions".
func (s *Service) HandoverOnDeparture(ctx context.Context, tenant TenantID, departed, newOwner ActorID) (unassigned, reassigned int, err error) {
	unassigned, err = s.repos.Tasks.UnassignBy(ctx, tenant, departed)
	if err != nil {
		return 0, 0, err
	}
	reassigned, err = s.repos.Tasks.ReassignCreator(ctx, tenant, departed, newOwner)
	if err != nil {
		return unassigned, 0, err
	}
	return unassigned, reassigned, nil
}

// checkAssignee validates assignee membership when assignee is non-nil.
func (s *Service) checkAssignee(ctx context.Context, tenant TenantID, assignee *ActorID) error {
	if assignee == nil {
		return nil
	}
	ok, err := s.members.IsMember(ctx, tenant, *assignee)
	if err != nil {
		return err
	}
	if !ok {
		return ErrAssigneeNotMember
	}
	return nil
}

// topPositionInColumn returns the position a new/moved task must take to
// land above every task currently in col: one less than col's current
// minimum Position, or 0 when col holds no live task. Repeatedly inserting
// at the top only ever decreases this value (see README.md "Position
// strategy" for the renormalization trade-off this implies). It asks
// TaskRepository for col's minimum directly rather than listing and
// scanning every task in the tenant, so CreateTask/MoveTask stay O(1) in
// tenant size regardless of how many tasks the tenant has accumulated.
func (s *Service) topPositionInColumn(ctx context.Context, tenant TenantID, col ColumnID) (float64, error) {
	minPos, found, err := s.repos.Tasks.MinPositionInColumn(ctx, tenant, col)
	if err != nil {
		return 0, err
	}
	if !found {
		return 0, nil
	}
	return minPos - 1, nil
}

// minPositionGap is the smallest gap MoveTask ever leaves between two
// neighbours. positionAfter bisects a gap only while both halves stay at or
// above it; otherwise the column is renormalized to whole numbers first, so
// repeatedly inserting between the same two tasks can never exhaust float64
// precision and collapse two tasks onto one position.
const minPositionGap = 1e-6

// positionInColumn returns the Position that places moving inside col
// according to after: at the top when after is nil (the v1 behaviour), or
// directly after *after otherwise. It must run inside UnitOfWork.Within.
// Both placements go through ListColumnPositions, which serializes
// concurrent moves on col for the rest of the transaction: a top move that
// skipped the lock could commit its min-1 while another move is
// renormalizing the column, and the renormalization's per-row update would
// then overwrite it and silently undo the move. When after is set and the
// gap to bisect is too small the column is renormalized once and the
// position recomputed from the fresh rows.
func (s *Service) positionInColumn(ctx context.Context, tenant TenantID, col ColumnID, moving TaskID, after *TaskID) (float64, error) {
	rows, err := s.repos.Tasks.ListColumnPositions(ctx, tenant, col)
	if err != nil {
		return 0, err
	}
	if after == nil {
		return topOf(rows), nil
	}
	pos, renormalize, ok := positionAfter(rows, moving, *after)
	if !ok {
		return 0, ErrInvalidAfterTask
	}
	if !renormalize {
		return pos, nil
	}
	order := make([]TaskID, len(rows))
	for i, r := range rows {
		order[i] = r.ID
	}
	if err := s.repos.Tasks.RenormalizeColumn(ctx, tenant, col, order); err != nil {
		return 0, err
	}
	rows, err = s.repos.Tasks.ListColumnPositions(ctx, tenant, col)
	if err != nil {
		return 0, err
	}
	pos, _, ok = positionAfter(rows, moving, *after)
	if !ok {
		return 0, ErrInvalidAfterTask
	}
	return pos, nil
}

// topOf is topPositionInColumn computed from an already listed column:
// one below the lowest position (rows are ascending), 0 when empty. The
// moving task counts like any other row, so moving the top task "to the
// top" again simply decrements it.
func topOf(rows []TaskPosition) float64 {
	if len(rows) == 0 {
		return 0
	}
	return rows[0].Position - 1
}

// positionAfter returns the Position that places moving directly after
// anchor within rows, which are already ordered by Position then creation
// time and exclude soft-deleted tasks. moving may or may not be in rows
// (same-column reorder vs. cross-column move) and is skipped when looking
// for anchor's successor, so "after anchor" means the same thing in both
// cases. With no successor the result is anchor.Position + 1; otherwise it
// is the midpoint, and renormalize reports that bisecting would leave a gap
// below minPositionGap — the caller must renormalize the column and call
// again. ok is false when anchor is absent from rows or equals moving.
func positionAfter(rows []TaskPosition, moving, anchor TaskID) (pos float64, renormalize, ok bool) {
	if anchor == moving {
		return 0, false, false
	}
	for i, r := range rows {
		if r.ID != anchor {
			continue
		}
		for _, next := range rows[i+1:] {
			if next.ID == moving {
				continue
			}
			gap := next.Position - r.Position
			return r.Position + gap/2, gap/2 < minPositionGap, true
		}
		return r.Position + 1, false, true
	}
	return 0, false, false
}

// completedAtIf returns clock.Now() when done is true, else nil.
func completedAtIf(clock Clock, done bool) *time.Time {
	if !done {
		return nil
	}
	now := clock.Now()
	return &now
}
