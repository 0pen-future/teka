package kanban

import (
	"context"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// --- fakes -------------------------------------------------------------
//
// Map-backed, single-process fakes. fakeTaskRepo keeps soft-deleted tasks
// (marked in `deleted`, never removed from `tasks`) so DeleteColumn's
// "counts soft-deleted rows too" contract is actually exercised.

type fakeColumnRepo struct {
	cols map[ColumnID]Column
}

func newFakeColumnRepo() *fakeColumnRepo {
	return &fakeColumnRepo{cols: map[ColumnID]Column{}}
}

func (r *fakeColumnRepo) List(_ context.Context, tenant TenantID) ([]Column, error) {
	var out []Column
	for _, c := range r.cols {
		if c.TenantID == tenant {
			out = append(out, c)
		}
	}
	return out, nil
}

func (r *fakeColumnRepo) Get(_ context.Context, tenant TenantID, id ColumnID) (Column, error) {
	c, ok := r.cols[id]
	if !ok || c.TenantID != tenant {
		return Column{}, ErrColumnNotFound
	}
	return c, nil
}

func (r *fakeColumnRepo) Create(_ context.Context, tenant TenantID, col Column) (Column, error) {
	col.TenantID = tenant
	r.cols[col.ID] = col
	return col, nil
}

func (r *fakeColumnRepo) Update(_ context.Context, tenant TenantID, col Column) (Column, error) {
	existing, ok := r.cols[col.ID]
	if !ok || existing.TenantID != tenant {
		return Column{}, ErrColumnNotFound
	}
	col.TenantID = tenant
	r.cols[col.ID] = col
	return col, nil
}

func (r *fakeColumnRepo) Delete(_ context.Context, tenant TenantID, id ColumnID) error {
	c, ok := r.cols[id]
	if !ok || c.TenantID != tenant {
		return ErrColumnNotFound
	}
	delete(r.cols, id)
	return nil
}

func (r *fakeColumnRepo) UpdatePositions(_ context.Context, tenant TenantID, order []ColumnID) error {
	for i, id := range order {
		c, ok := r.cols[id]
		if !ok || c.TenantID != tenant {
			return ErrColumnNotFound
		}
		c.Position = i
		r.cols[id] = c
	}
	return nil
}

func (r *fakeColumnRepo) CountByTenant(_ context.Context, tenant TenantID) (int, error) {
	n := 0
	for _, c := range r.cols {
		if c.TenantID == tenant {
			n++
		}
	}
	return n, nil
}

func (r *fakeColumnRepo) ExistsName(_ context.Context, tenant TenantID, name string) (bool, error) {
	for _, c := range r.cols {
		if c.TenantID == tenant && strings.EqualFold(c.Name, name) {
			return true, nil
		}
	}
	return false, nil
}

type fakeTaskRepo struct {
	tasks   map[TaskID]Task
	deleted map[TaskID]bool
	// renormalized records every RenormalizeColumn call's order, so a test
	// can assert both that renormalization happened and what it was told.
	renormalized [][]TaskID
	// listed counts ListColumnPositions calls: the port that also takes the
	// per-column lock, so a test can assert a placement went through it.
	listed int
}

func newFakeTaskRepo() *fakeTaskRepo {
	return &fakeTaskRepo{tasks: map[TaskID]Task{}, deleted: map[TaskID]bool{}}
}

// ListColumnPositions orders like the SQL adapter (Position, then
// CreatedAt), with ID as a final tie-break so map iteration cannot make the
// fake non-deterministic.
func (r *fakeTaskRepo) ListColumnPositions(_ context.Context, tenant TenantID, col ColumnID) ([]TaskPosition, error) {
	r.listed++
	var live []Task
	for id, t := range r.tasks {
		if t.TenantID != tenant || t.ColumnID != col || r.deleted[id] {
			continue
		}
		live = append(live, t)
	}
	sort.Slice(live, func(i, j int) bool {
		a, b := live[i], live[j]
		if a.Position != b.Position {
			return a.Position < b.Position
		}
		if !a.CreatedAt.Equal(b.CreatedAt) {
			return a.CreatedAt.Before(b.CreatedAt)
		}
		return uuid.UUID(a.ID).String() < uuid.UUID(b.ID).String()
	})
	out := make([]TaskPosition, len(live))
	for i, t := range live {
		out[i] = TaskPosition{ID: t.ID, Position: t.Position}
	}
	return out, nil
}

func (r *fakeTaskRepo) RenormalizeColumn(_ context.Context, tenant TenantID, col ColumnID, order []TaskID) error {
	r.renormalized = append(r.renormalized, order)
	for i, id := range order {
		t, ok := r.tasks[id]
		if !ok || t.TenantID != tenant || t.ColumnID != col {
			return errors.New("fake: renormalize order names a task outside the column")
		}
		t.Position = float64(i)
		r.tasks[id] = t
	}
	return nil
}

// ListBoard ignores limit: fakes exist to exercise Service's own logic, and
// the per-column cap is pushed all the way into the SQL adapter (see
// task_repository.go), so there is nothing for this in-memory stand-in to
// truncate.
func (r *fakeTaskRepo) ListBoard(_ context.Context, tenant TenantID, vis Visibility, _ int) ([]Task, error) {
	var out []Task
	for id, t := range r.tasks {
		if t.TenantID != tenant || r.deleted[id] {
			continue
		}
		if !vis.All {
			isCreator := t.CreatedBy == vis.Participant
			isAssignee := t.AssigneeID != nil && *t.AssigneeID == vis.Participant
			if !isCreator && !isAssignee {
				continue
			}
		}
		out = append(out, t)
	}
	return out, nil
}

func (r *fakeTaskRepo) MinPositionInColumn(_ context.Context, tenant TenantID, col ColumnID) (float64, bool, error) {
	minPos, found := 0.0, false
	for id, t := range r.tasks {
		if t.TenantID != tenant || t.ColumnID != col || r.deleted[id] {
			continue
		}
		if !found || t.Position < minPos {
			minPos = t.Position
			found = true
		}
	}
	return minPos, found, nil
}

func (r *fakeTaskRepo) Get(_ context.Context, tenant TenantID, id TaskID) (Task, error) {
	t, ok := r.tasks[id]
	if !ok || t.TenantID != tenant || r.deleted[id] {
		return Task{}, ErrTaskNotFound
	}
	return t, nil
}

func (r *fakeTaskRepo) Create(_ context.Context, tenant TenantID, task Task) (Task, error) {
	task.TenantID = tenant
	r.tasks[task.ID] = task
	return task, nil
}

func (r *fakeTaskRepo) Update(_ context.Context, tenant TenantID, task Task) (Task, error) {
	existing, ok := r.tasks[task.ID]
	if !ok || existing.TenantID != tenant {
		return Task{}, ErrTaskNotFound
	}
	task.TenantID = tenant
	r.tasks[task.ID] = task
	return task, nil
}

func (r *fakeTaskRepo) SoftDelete(_ context.Context, tenant TenantID, id TaskID) error {
	t, ok := r.tasks[id]
	if !ok || t.TenantID != tenant {
		return ErrTaskNotFound
	}
	r.deleted[id] = true
	return nil
}

func (r *fakeTaskRepo) CountInColumn(_ context.Context, tenant TenantID, col ColumnID) (int, error) {
	n := 0
	for _, t := range r.tasks {
		if t.TenantID == tenant && t.ColumnID == col {
			n++ // counts soft-deleted rows too, matching the port contract
		}
	}
	return n, nil
}

func (r *fakeTaskRepo) MoveAllToColumn(_ context.Context, tenant TenantID, from, to ColumnID, completedAt *time.Time) (int, error) {
	n := 0
	for id, t := range r.tasks {
		if t.TenantID == tenant && t.ColumnID == from {
			t.ColumnID = to
			t.CompletedAt = completedAt
			r.tasks[id] = t
			n++
		}
	}
	return n, nil
}

func (r *fakeTaskRepo) UnassignBy(_ context.Context, tenant TenantID, actor ActorID) (int, error) {
	n := 0
	for id, t := range r.tasks {
		if t.TenantID == tenant && t.AssigneeID != nil && *t.AssigneeID == actor {
			t.AssigneeID = nil
			r.tasks[id] = t
			n++
		}
	}
	return n, nil
}

func (r *fakeTaskRepo) ReassignCreator(_ context.Context, tenant TenantID, from, to ActorID) (int, error) {
	n := 0
	for id, t := range r.tasks {
		if t.TenantID == tenant && t.CreatedBy == from {
			t.CreatedBy = to
			r.tasks[id] = t
			n++
		}
	}
	return n, nil
}

type fakeMembers struct{ ids map[ActorID]bool }

func (m *fakeMembers) IsMember(_ context.Context, _ TenantID, actor ActorID) (bool, error) {
	return m.ids[actor], nil
}

type fakeSink struct{ events []any }

func (s *fakeSink) Publish(_ context.Context, e any) { s.events = append(s.events, e) }

// fakeUoW runs fn directly unless err is set, in which case it returns err
// without calling fn — enough to prove "no event published when the
// transaction fails" without a real database.
type fakeUoW struct {
	err   error
	calls int
}

func (u *fakeUoW) Within(ctx context.Context, fn func(ctx context.Context) error) error {
	u.calls++
	if u.err != nil {
		return u.err
	}
	return fn(ctx)
}

type fakeClock struct{ now time.Time }

func (c fakeClock) Now() time.Time { return c.now }

// --- test harness --------------------------------------------------------

type testEnv struct {
	svc     *Service
	cols    *fakeColumnRepo
	tasks   *fakeTaskRepo
	members *fakeMembers
	sink    *fakeSink
	uow     *fakeUoW
	clock   fakeClock
}

var fixedNow = time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

func newTestEnv(opts ...Option) *testEnv {
	cols := newFakeColumnRepo()
	tasks := newFakeTaskRepo()
	members := &fakeMembers{ids: map[ActorID]bool{}}
	sink := &fakeSink{}
	uow := &fakeUoW{}
	clock := fakeClock{now: fixedNow}

	allOpts := append([]Option{WithClock(clock)}, opts...)
	svc := NewService(Repositories{Columns: cols, Tasks: tasks}, uow, DefaultPolicy{}, members, sink, allOpts...)
	return &testEnv{svc: svc, cols: cols, tasks: tasks, members: members, sink: sink, uow: uow, clock: clock}
}

func (e *testEnv) addColumn(tenant TenantID, name string, isDone bool, position int) Column {
	col := Column{ID: ColumnID(uuid.New()), TenantID: tenant, Name: name, IsDone: isDone, Position: position}
	e.cols.cols[col.ID] = col
	return col
}

func (e *testEnv) addTask(tenant TenantID, col ColumnID, creator ActorID, assignee *ActorID) Task {
	task := Task{
		ID:         TaskID(uuid.New()),
		TenantID:   tenant,
		ColumnID:   col,
		CreatedBy:  creator,
		AssigneeID: assignee,
		Title:      "task",
		Position:   0,
	}
	e.tasks.tasks[task.ID] = task
	return task
}

func newTenant() TenantID { return TenantID(uuid.New()) }
func newActor() ActorID   { return ActorID(uuid.New()) }

func ownerActor(id ActorID) Actor { return Actor{ID: id, IsOwner: true} }
func plainActor(id ActorID) Actor { return Actor{ID: id, Perms: map[string]bool{}} }
func boardManager(id ActorID) Actor {
	return Actor{ID: id, Perms: map[string]bool{PermManageBoard: true}}
}

// --- Board -----------------------------------------------------------------

func TestBoardOwnerSeesEverythingParticipantSeesOwnOnly(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	col := env.addColumn(tenant, "Todo", false, 0)
	creator := newActor()
	other := newActor()
	env.addTask(tenant, col.ID, creator, nil)
	env.addTask(tenant, col.ID, other, nil)

	_, tasks, err := env.svc.Board(context.Background(), tenant, ownerActor(newActor()), 0)
	require.NoError(t, err)
	require.Len(t, tasks, 2)

	_, tasks, err = env.svc.Board(context.Background(), tenant, plainActor(creator), 0)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	require.Equal(t, creator, tasks[0].CreatedBy)
}

func TestBoardOrdersColumnsByPosition(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	second := env.addColumn(tenant, "Second", false, 1)
	first := env.addColumn(tenant, "First", false, 0)

	cols, _, err := env.svc.Board(context.Background(), tenant, ownerActor(newActor()), 0)
	require.NoError(t, err)
	require.Equal(t, []ColumnID{first.ID, second.ID}, []ColumnID{cols[0].ID, cols[1].ID})
}

// --- CreateColumn ------------------------------------------------------

func TestCreateColumnAppendsAtNextPosition(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	env.addColumn(tenant, "Todo", false, 0)

	col, err := env.svc.CreateColumn(context.Background(), tenant, ownerActor(newActor()), "Doing", false)
	require.NoError(t, err)
	require.Equal(t, 1, col.Position)
}

func TestCreateColumnRequiresManageBoard(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	_, err := env.svc.CreateColumn(context.Background(), tenant, plainActor(newActor()), "Doing", false)
	require.ErrorIs(t, err, ErrForbidden)
}

func TestCreateColumnLimit(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	for i := 0; i < defaultMaxColumns; i++ {
		env.addColumn(tenant, uuid.NewString(), false, i)
	}
	_, err := env.svc.CreateColumn(context.Background(), tenant, ownerActor(newActor()), "One too many", false)
	require.ErrorIs(t, err, ErrColumnLimit)
}

func TestCreateColumnDuplicateNameCaseInsensitive(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	env.addColumn(tenant, "Todo", false, 0)
	_, err := env.svc.CreateColumn(context.Background(), tenant, ownerActor(newActor()), "TODO", false)
	require.ErrorIs(t, err, ErrDuplicateColumnName)
}

func TestCreateColumnRejectsEmptyOrTooLongName(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	_, err := env.svc.CreateColumn(context.Background(), tenant, ownerActor(newActor()), "   ", false)
	require.ErrorIs(t, err, ErrInvalidInput)

	long := make([]byte, defaultMaxNameLen+1)
	for i := range long {
		long[i] = 'a'
	}
	_, err = env.svc.CreateColumn(context.Background(), tenant, ownerActor(newActor()), string(long), false)
	require.ErrorIs(t, err, ErrInvalidInput)
}

// --- UpdateColumn --------------------------------------------------------

func TestUpdateColumnRenameAndToggleIsDoneDoNotTouchTasks(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	col := env.addColumn(tenant, "Todo", false, 0)
	task := env.addTask(tenant, col.ID, newActor(), nil)

	newName := "Doing"
	isDone := true
	updated, err := env.svc.UpdateColumn(context.Background(), tenant, ownerActor(newActor()), col.ID, ColumnPatch{Name: &newName, IsDone: &isDone})
	require.NoError(t, err)
	require.Equal(t, "Doing", updated.Name)
	require.True(t, updated.IsDone)

	stillUnchanged := env.tasks.tasks[task.ID]
	require.Nil(t, stillUnchanged.CompletedAt, "toggling IsDone must not rewrite existing task history")
}

func TestUpdateColumnRenameToSameNameIsNotADuplicate(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	col := env.addColumn(tenant, "Todo", false, 0)
	same := "TODO"
	_, err := env.svc.UpdateColumn(context.Background(), tenant, ownerActor(newActor()), col.ID, ColumnPatch{Name: &same})
	require.NoError(t, err)
}

func TestUpdateColumnDuplicateName(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	env.addColumn(tenant, "Todo", false, 0)
	col := env.addColumn(tenant, "Doing", false, 1)
	name := "todo"
	_, err := env.svc.UpdateColumn(context.Background(), tenant, ownerActor(newActor()), col.ID, ColumnPatch{Name: &name})
	require.ErrorIs(t, err, ErrDuplicateColumnName)
}

// --- ReorderColumns ------------------------------------------------------

func TestReorderColumnsRejectsNonPermutation(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	a := env.addColumn(tenant, "A", false, 0)
	env.addColumn(tenant, "B", false, 1)

	// missing one id
	err := env.svc.ReorderColumns(context.Background(), tenant, ownerActor(newActor()), []ColumnID{a.ID})
	require.ErrorIs(t, err, ErrInvalidPermutation)

	// extra/foreign id
	err = env.svc.ReorderColumns(context.Background(), tenant, ownerActor(newActor()), []ColumnID{a.ID, a.ID, ColumnID(uuid.New())})
	require.ErrorIs(t, err, ErrInvalidPermutation)
}

func TestReorderColumnsPersistsOrder(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	a := env.addColumn(tenant, "A", false, 0)
	b := env.addColumn(tenant, "B", false, 1)

	err := env.svc.ReorderColumns(context.Background(), tenant, ownerActor(newActor()), []ColumnID{b.ID, a.ID})
	require.NoError(t, err)
	require.Equal(t, 0, env.cols.cols[b.ID].Position)
	require.Equal(t, 1, env.cols.cols[a.ID].Position)
	require.Equal(t, 1, env.uow.calls, "UpdatePositions must run inside UnitOfWork.Within")
}

// TestReorderColumnsDoesNotPersistWhenUnitOfWorkFails asserts a failing
// UnitOfWork stops UpdatePositions from applying a partial order — the same
// atomicity DeleteColumn already gets from wrapping its own writes.
func TestReorderColumnsDoesNotPersistWhenUnitOfWorkFails(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	env.uow.err = errors.New("boom")
	tenant := newTenant()
	a := env.addColumn(tenant, "A", false, 0)
	b := env.addColumn(tenant, "B", false, 1)

	err := env.svc.ReorderColumns(context.Background(), tenant, ownerActor(newActor()), []ColumnID{b.ID, a.ID})
	require.Error(t, err)
	require.Equal(t, 0, env.cols.cols[a.ID].Position, "positions must be unchanged when the transaction fails")
	require.Equal(t, 1, env.cols.cols[b.ID].Position, "positions must be unchanged when the transaction fails")
}

// --- DeleteColumn ----------------------------------------------------------

func TestDeleteColumnLastColumn(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	col := env.addColumn(tenant, "Only", false, 0)
	err := env.svc.DeleteColumn(context.Background(), tenant, ownerActor(newActor()), col.ID, nil)
	require.ErrorIs(t, err, ErrLastColumn)
}

func TestDeleteColumnNotEmptyRequiresMoveTo(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	col := env.addColumn(tenant, "Todo", false, 0)
	env.addColumn(tenant, "Done", true, 1)
	env.addTask(tenant, col.ID, newActor(), nil)

	err := env.svc.DeleteColumn(context.Background(), tenant, ownerActor(newActor()), col.ID, nil)
	require.ErrorIs(t, err, ErrColumnNotEmpty)
}

func TestDeleteColumnCountsSoftDeletedTasksToo(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	col := env.addColumn(tenant, "Todo", false, 0)
	env.addColumn(tenant, "Done", true, 1)
	task := env.addTask(tenant, col.ID, newActor(), nil)
	require.NoError(t, env.tasks.SoftDelete(context.Background(), tenant, task.ID))

	// The only task in the column is soft-deleted, but it still holds a
	// foreign key to the column, so moveTo is still required.
	err := env.svc.DeleteColumn(context.Background(), tenant, ownerActor(newActor()), col.ID, nil)
	require.ErrorIs(t, err, ErrColumnNotEmpty)
}

func TestDeleteColumnMoveToSelfIsInvalid(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	col := env.addColumn(tenant, "Todo", false, 0)
	env.addColumn(tenant, "Done", true, 1)
	err := env.svc.DeleteColumn(context.Background(), tenant, ownerActor(newActor()), col.ID, &col.ID)
	require.ErrorIs(t, err, ErrInvalidInput)
}

func TestDeleteColumnMoveToOtherTenantIsNotFound(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	other := newTenant()
	col := env.addColumn(tenant, "Todo", false, 0)
	env.addColumn(tenant, "Done", true, 1)
	foreign := env.addColumn(other, "Foreign", false, 0)

	err := env.svc.DeleteColumn(context.Background(), tenant, ownerActor(newActor()), col.ID, &foreign.ID)
	require.ErrorIs(t, err, ErrColumnNotFound)
}

func TestDeleteColumnMovesTasksSetsCompletedAtAndPublishesOnSuccess(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	todo := env.addColumn(tenant, "Todo", false, 0)
	done := env.addColumn(tenant, "Done", true, 1)
	task := env.addTask(tenant, todo.ID, newActor(), nil)

	err := env.svc.DeleteColumn(context.Background(), tenant, ownerActor(newActor()), todo.ID, &done.ID)
	require.NoError(t, err)

	moved := env.tasks.tasks[task.ID]
	require.Equal(t, done.ID, moved.ColumnID)
	require.NotNil(t, moved.CompletedAt)
	require.True(t, moved.CompletedAt.Equal(fixedNow))

	require.Len(t, env.sink.events, 1)
	evt, ok := env.sink.events[0].(ColumnDeleted)
	require.True(t, ok)
	require.Equal(t, todo.ID, evt.ColumnID)
	require.Equal(t, &done.ID, evt.MoveTo)
	require.Equal(t, 1, evt.MovedCount)

	_, err = env.cols.Get(context.Background(), tenant, todo.ID)
	require.ErrorIs(t, err, ErrColumnNotFound)
}

func TestDeleteColumnDoesNotPublishWhenUnitOfWorkFails(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	env.uow.err = errors.New("boom")
	tenant := newTenant()
	todo := env.addColumn(tenant, "Todo", false, 0)
	done := env.addColumn(tenant, "Done", true, 1)

	err := env.svc.DeleteColumn(context.Background(), tenant, ownerActor(newActor()), todo.ID, &done.ID)
	require.Error(t, err)
	require.Empty(t, env.sink.events, "no event may be published when the transaction fails")
}

func TestDeleteColumnRequiresManageBoard(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	col := env.addColumn(tenant, "Todo", false, 0)
	env.addColumn(tenant, "Done", true, 1)
	err := env.svc.DeleteColumn(context.Background(), tenant, plainActor(newActor()), col.ID, nil)
	require.ErrorIs(t, err, ErrForbidden)
}

// --- CreateTask --------------------------------------------------------

func TestCreateTaskAssigneeMustBeMember(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	col := env.addColumn(tenant, "Todo", false, 0)
	assignee := newActor()

	_, err := env.svc.CreateTask(context.Background(), tenant, plainActor(newActor()), CreateTaskInput{
		ColumnID: col.ID, Title: "Task", AssigneeID: &assignee,
	})
	require.ErrorIs(t, err, ErrAssigneeNotMember)

	env.members.ids[assignee] = true
	task, err := env.svc.CreateTask(context.Background(), tenant, plainActor(newActor()), CreateTaskInput{
		ColumnID: col.ID, Title: "Task", AssigneeID: &assignee,
	})
	require.NoError(t, err)
	require.Equal(t, assignee, *task.AssigneeID)
}

func TestCreateTaskUnknownColumnIsNotFound(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	_, err := env.svc.CreateTask(context.Background(), tenant, plainActor(newActor()), CreateTaskInput{
		ColumnID: ColumnID(uuid.New()), Title: "Task",
	})
	require.ErrorIs(t, err, ErrColumnNotFound)
}

func TestCreateTaskRejectsEmptyTitle(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	col := env.addColumn(tenant, "Todo", false, 0)
	_, err := env.svc.CreateTask(context.Background(), tenant, plainActor(newActor()), CreateTaskInput{ColumnID: col.ID, Title: "  "})
	require.ErrorIs(t, err, ErrInvalidInput)
}

func TestCreateTaskLandsAboveExistingTasks(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	col := env.addColumn(tenant, "Todo", false, 0)
	existing := env.addTask(tenant, col.ID, newActor(), nil)
	existing.Position = 5
	env.tasks.tasks[existing.ID] = existing

	created, err := env.svc.CreateTask(context.Background(), tenant, plainActor(newActor()), CreateTaskInput{ColumnID: col.ID, Title: "New"})
	require.NoError(t, err)
	require.Less(t, created.Position, 5.0)
}

// --- GetTask / UpdateTask / DeleteTask -----------------------------------

func TestGetTaskForbiddenForStranger(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	col := env.addColumn(tenant, "Todo", false, 0)
	task := env.addTask(tenant, col.ID, newActor(), nil)

	_, err := env.svc.GetTask(context.Background(), tenant, plainActor(newActor()), task.ID)
	require.ErrorIs(t, err, ErrForbidden)
}

func TestGetTaskUnknownIDIsNotFound(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	_, err := env.svc.GetTask(context.Background(), tenant, ownerActor(newActor()), TaskID(uuid.New()))
	require.ErrorIs(t, err, ErrTaskNotFound)
}

func TestUpdateTaskDeleteTaskPermissions(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	col := env.addColumn(tenant, "Todo", false, 0)
	creator := newActor()
	assignee := newActor()
	owner := newActor()
	stranger := newActor()

	for name, actor := range map[string]Actor{
		"creator":  plainActor(creator),
		"assignee": plainActor(assignee),
		"owner":    ownerActor(owner),
		"stranger": plainActor(stranger),
	} {
		t.Run(name, func(t *testing.T) {
			task := env.addTask(tenant, col.ID, creator, &assignee)
			_, err := env.svc.UpdateTask(context.Background(), tenant, actor, task.ID, UpdateTaskInput{Title: "Updated"})
			if name == "creator" || name == "owner" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrForbidden)
			}

			task2 := env.addTask(tenant, col.ID, creator, &assignee)
			err = env.svc.DeleteTask(context.Background(), tenant, actor, task2.ID)
			if name == "creator" || name == "owner" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrForbidden)
			}
		})
	}
}

func TestUpdateTaskAssigneeMustBeMember(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	col := env.addColumn(tenant, "Todo", false, 0)
	creator := newActor()
	task := env.addTask(tenant, col.ID, creator, nil)
	newAssignee := newActor()

	_, err := env.svc.UpdateTask(context.Background(), tenant, plainActor(creator), task.ID, UpdateTaskInput{Title: "x", AssigneeID: &newAssignee})
	require.ErrorIs(t, err, ErrAssigneeNotMember)
}

func TestUpdateTaskClearsDueOnAndAssignee(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	col := env.addColumn(tenant, "Todo", false, 0)
	creator := newActor()
	assignee := newActor()
	task := env.addTask(tenant, col.ID, creator, &assignee)
	due := fixedNow
	task.DueOn = &due
	env.tasks.tasks[task.ID] = task

	updated, err := env.svc.UpdateTask(context.Background(), tenant, plainActor(creator), task.ID, UpdateTaskInput{Title: "x"})
	require.NoError(t, err)
	require.Nil(t, updated.DueOn)
	require.Nil(t, updated.AssigneeID)
}

// --- MoveTask --------------------------------------------------------------

func TestMoveTaskSetsAndClearsCompletedAt(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	todo := env.addColumn(tenant, "Todo", false, 0)
	done := env.addColumn(tenant, "Done", true, 1)
	creator := newActor()
	task := env.addTask(tenant, todo.ID, creator, nil)

	moved, err := env.svc.MoveTask(context.Background(), tenant, plainActor(creator), task.ID, done.ID, nil)
	require.NoError(t, err)
	require.NotNil(t, moved.CompletedAt)
	require.True(t, moved.CompletedAt.Equal(fixedNow))

	movedBack, err := env.svc.MoveTask(context.Background(), tenant, plainActor(creator), task.ID, todo.ID, nil)
	require.NoError(t, err)
	require.Nil(t, movedBack.CompletedAt)
}

func TestMoveTaskAssigneeCanMoveButNotEdit(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	todo := env.addColumn(tenant, "Todo", false, 0)
	done := env.addColumn(tenant, "Done", true, 1)
	creator := newActor()
	assignee := newActor()
	task := env.addTask(tenant, todo.ID, creator, &assignee)

	_, err := env.svc.MoveTask(context.Background(), tenant, plainActor(assignee), task.ID, done.ID, nil)
	require.NoError(t, err)

	_, err = env.svc.UpdateTask(context.Background(), tenant, plainActor(assignee), task.ID, UpdateTaskInput{Title: "hack"})
	require.ErrorIs(t, err, ErrForbidden)
}

func TestMoveTaskStrangerForbidden(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	todo := env.addColumn(tenant, "Todo", false, 0)
	done := env.addColumn(tenant, "Done", true, 1)
	task := env.addTask(tenant, todo.ID, newActor(), nil)

	_, err := env.svc.MoveTask(context.Background(), tenant, plainActor(newActor()), task.ID, done.ID, nil)
	require.ErrorIs(t, err, ErrForbidden)
}

func TestMoveTaskTargetColumnNotFound(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	todo := env.addColumn(tenant, "Todo", false, 0)
	creator := newActor()
	task := env.addTask(tenant, todo.ID, creator, nil)

	_, err := env.svc.MoveTask(context.Background(), tenant, plainActor(creator), task.ID, ColumnID(uuid.New()), nil)
	require.ErrorIs(t, err, ErrColumnNotFound)
}

// addTaskAt seeds a task at an explicit position, for ordering tests.
func (e *testEnv) addTaskAt(tenant TenantID, col ColumnID, creator ActorID, pos float64) Task {
	task := e.addTask(tenant, col, creator, nil)
	task.Position = pos
	e.tasks.tasks[task.ID] = task
	return task
}

func columnOrder(t *testing.T, env *testEnv, tenant TenantID, col ColumnID) []TaskID {
	t.Helper()
	rows, err := env.tasks.ListColumnPositions(context.Background(), tenant, col)
	require.NoError(t, err)
	ids := make([]TaskID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids
}

func TestPositionAfter(t *testing.T) {
	t.Parallel()
	a, b, c, moving := TaskID(uuid.New()), TaskID(uuid.New()), TaskID(uuid.New()), TaskID(uuid.New())
	rows := []TaskPosition{{ID: a, Position: 0}, {ID: b, Position: 1}, {ID: c, Position: 2}}

	tests := []struct {
		name       string
		rows       []TaskPosition
		moving     TaskID
		anchor     TaskID
		wantPos    float64
		wantRenorm bool
		wantOK     bool
	}{
		{name: "anchor is last: one past it", rows: rows, moving: moving, anchor: c, wantPos: 3, wantOK: true},
		{name: "anchor in the middle: midpoint to successor", rows: rows, moving: moving, anchor: a, wantPos: 0.5, wantOK: true},
		{name: "anchor equals moving", rows: rows, moving: a, anchor: a},
		{name: "anchor absent", rows: rows, moving: moving, anchor: TaskID(uuid.New())},
		{name: "empty rows", rows: nil, moving: moving, anchor: a},
		{
			name:   "same-column reorder skips moving as successor",
			rows:   []TaskPosition{{ID: a, Position: 0}, {ID: moving, Position: 1}, {ID: b, Position: 2}},
			moving: moving, anchor: a, wantPos: 1, wantOK: true,
		},
		{
			name:   "same-column reorder: moving was the only successor",
			rows:   []TaskPosition{{ID: a, Position: 0}, {ID: moving, Position: 1}},
			moving: moving, anchor: a, wantPos: 1, wantOK: true,
		},
		{
			name:   "bisecting below the floor asks for renormalization",
			rows:   []TaskPosition{{ID: a, Position: 0}, {ID: b, Position: minPositionGap}},
			moving: moving, anchor: a, wantPos: minPositionGap / 2, wantRenorm: true, wantOK: true,
		},
		{
			name:   "bisecting exactly onto the floor still bisects",
			rows:   []TaskPosition{{ID: a, Position: 0}, {ID: b, Position: 2 * minPositionGap}},
			moving: moving, anchor: a, wantPos: minPositionGap, wantOK: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pos, renorm, ok := positionAfter(tc.rows, tc.moving, tc.anchor)
			require.Equal(t, tc.wantOK, ok)
			require.Equal(t, tc.wantRenorm, renorm)
			require.InDelta(t, tc.wantPos, pos, 1e-12)
		})
	}
}

func TestMoveTaskWithoutAfterLandsAtTop(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	todo := env.addColumn(tenant, "Todo", false, 0)
	doing := env.addColumn(tenant, "Doing", false, 1)
	creator := newActor()
	env.addTaskAt(tenant, doing.ID, creator, 3)
	task := env.addTaskAt(tenant, todo.ID, creator, 0)

	moved, err := env.svc.MoveTask(context.Background(), tenant, plainActor(creator), task.ID, doing.ID, nil)
	require.NoError(t, err)
	require.Equal(t, 2.0, moved.Position, "nil after keeps the v1 top-of-column placement (min - 1)")
	require.Equal(t, 1, env.uow.calls, "placement and write share one unit of work")
	require.Equal(t, 1, env.tasks.listed, "the top placement lists the column too, so it holds the column lock")
	require.Empty(t, env.tasks.renormalized)
}

func TestMoveTaskWithoutAfterIntoAnEmptyColumnStartsAtZero(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	todo := env.addColumn(tenant, "Todo", false, 0)
	doing := env.addColumn(tenant, "Doing", false, 1)
	creator := newActor()
	task := env.addTaskAt(tenant, todo.ID, creator, 7)

	moved, err := env.svc.MoveTask(context.Background(), tenant, plainActor(creator), task.ID, doing.ID, nil)
	require.NoError(t, err)
	require.Equal(t, 0.0, moved.Position)
	require.Equal(t, []TaskID{task.ID}, columnOrder(t, env, tenant, doing.ID))
}

func TestMoveTaskAfterPlacesBetweenNeighboursAcrossColumns(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	todo := env.addColumn(tenant, "Todo", false, 0)
	done := env.addColumn(tenant, "Done", true, 1)
	creator := newActor()
	first := env.addTaskAt(tenant, done.ID, creator, 0)
	second := env.addTaskAt(tenant, done.ID, creator, 1)
	task := env.addTaskAt(tenant, todo.ID, creator, 0)

	moved, err := env.svc.MoveTask(context.Background(), tenant, plainActor(creator), task.ID, done.ID, &first.ID)
	require.NoError(t, err)
	require.Equal(t, done.ID, moved.ColumnID)
	require.Equal(t, 0.5, moved.Position)
	require.NotNil(t, moved.CompletedAt, "CompletedAt still follows the destination column's IsDone")
	require.Equal(t, []TaskID{first.ID, task.ID, second.ID}, columnOrder(t, env, tenant, done.ID))
	require.Empty(t, env.tasks.renormalized)
}

func TestMoveTaskAfterReordersWithinTheSameColumn(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	todo := env.addColumn(tenant, "Todo", false, 0)
	creator := newActor()
	a := env.addTaskAt(tenant, todo.ID, creator, 0)
	b := env.addTaskAt(tenant, todo.ID, creator, 1)
	c := env.addTaskAt(tenant, todo.ID, creator, 2)

	// Move the first task after the last: [a b c] -> [b c a].
	moved, err := env.svc.MoveTask(context.Background(), tenant, plainActor(creator), a.ID, todo.ID, &c.ID)
	require.NoError(t, err)
	require.Equal(t, 3.0, moved.Position)
	require.Equal(t, []TaskID{b.ID, c.ID, a.ID}, columnOrder(t, env, tenant, todo.ID))

	// Move it back after b: the successor search must skip a itself, so it
	// lands between b and c rather than "after b, before a".
	moved, err = env.svc.MoveTask(context.Background(), tenant, plainActor(creator), a.ID, todo.ID, &b.ID)
	require.NoError(t, err)
	require.Equal(t, 1.5, moved.Position)
	require.Equal(t, []TaskID{b.ID, a.ID, c.ID}, columnOrder(t, env, tenant, todo.ID))
}

func TestMoveTaskAfterRejectsAnchorsOutsideTheDestination(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	other := newTenant()
	todo := env.addColumn(tenant, "Todo", false, 0)
	done := env.addColumn(tenant, "Done", true, 1)
	creator := newActor()
	task := env.addTaskAt(tenant, todo.ID, creator, 0)
	inTodo := env.addTaskAt(tenant, todo.ID, creator, 1)
	deleted := env.addTaskAt(tenant, done.ID, creator, 0)
	require.NoError(t, env.tasks.SoftDelete(context.Background(), tenant, deleted.ID))
	foreign := env.addTaskAt(other, env.addColumn(other, "Done", true, 0).ID, creator, 0)
	missing := TaskID(uuid.New())

	for name, anchor := range map[string]TaskID{
		"anchor in another column":  inTodo.ID,
		"anchor soft-deleted":       deleted.ID,
		"anchor in another tenant":  foreign.ID,
		"anchor missing":            missing,
		"anchor is the moving task": task.ID,
	} {
		anchor := anchor
		_, err := env.svc.MoveTask(context.Background(), tenant, plainActor(creator), task.ID, done.ID, &anchor)
		require.ErrorIs(t, err, ErrInvalidAfterTask, name)
		require.ErrorIs(t, err, ErrInvalidInput, name)
	}
	require.Equal(t, todo.ID, env.tasks.tasks[task.ID].ColumnID, "a rejected move leaves the task where it was")
}

func TestMoveTaskRenormalizesWhenTheGapIsTooSmall(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	todo := env.addColumn(tenant, "Todo", false, 0)
	doing := env.addColumn(tenant, "Doing", false, 1)
	creator := newActor()
	a := env.addTaskAt(tenant, doing.ID, creator, 0)
	b := env.addTaskAt(tenant, doing.ID, creator, minPositionGap/2)
	c := env.addTaskAt(tenant, doing.ID, creator, 7)
	task := env.addTaskAt(tenant, todo.ID, creator, 0)

	moved, err := env.svc.MoveTask(context.Background(), tenant, plainActor(creator), task.ID, doing.ID, &a.ID)
	require.NoError(t, err)
	require.Equal(t, [][]TaskID{{a.ID, b.ID, c.ID}}, env.tasks.renormalized, "renormalize once, in the column's current order")
	require.Equal(t, 0.5, moved.Position, "midpoint of the renormalized neighbours 0 and 1")
	require.Equal(t, []TaskID{a.ID, task.ID, b.ID, c.ID}, columnOrder(t, env, tenant, doing.ID))
	require.Equal(t, 1.0, env.tasks.tasks[b.ID].Position)
	require.Equal(t, 2.0, env.tasks.tasks[c.ID].Position)
}

func TestMoveTaskDoesNotWriteWhenUnitOfWorkFails(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	env.uow.err = errors.New("boom")
	tenant := newTenant()
	todo := env.addColumn(tenant, "Todo", false, 0)
	done := env.addColumn(tenant, "Done", true, 1)
	creator := newActor()
	task := env.addTaskAt(tenant, todo.ID, creator, 0)

	_, err := env.svc.MoveTask(context.Background(), tenant, plainActor(creator), task.ID, done.ID, nil)
	require.EqualError(t, err, "boom")
	require.Equal(t, todo.ID, env.tasks.tasks[task.ID].ColumnID)
}

// --- HandoverOnDeparture -----------------------------------------------

func TestHandoverOnDepartureUnassignsAndReassignsWithoutTxOrEvent(t *testing.T) {
	t.Parallel()
	env := newTestEnv()
	tenant := newTenant()
	col := env.addColumn(tenant, "Todo", false, 0)
	departed := newActor()
	newOwner := newActor()
	other := newActor()

	assignedTask := env.addTask(tenant, col.ID, other, &departed)
	createdTask := env.addTask(tenant, col.ID, departed, nil)

	unassigned, reassigned, err := env.svc.HandoverOnDeparture(context.Background(), tenant, departed, newOwner)
	require.NoError(t, err)
	require.Equal(t, 1, unassigned)
	require.Equal(t, 1, reassigned)

	require.Nil(t, env.tasks.tasks[assignedTask.ID].AssigneeID)
	require.Equal(t, newOwner, env.tasks.tasks[createdTask.ID].CreatedBy)

	require.Equal(t, 0, env.uow.calls, "HandoverOnDeparture must not open its own UnitOfWork")
	require.Empty(t, env.sink.events, "HandoverOnDeparture must not publish; the consumer owns that after its own commit")
}

// --- DefaultColumns ------------------------------------------------------

func TestDefaultColumnsAssignsPositionsAndKeepsIsDone(t *testing.T) {
	t.Parallel()
	specs := []DefaultColumnSpec{
		{Name: "Todo", IsDone: false},
		{Name: "Doing", IsDone: false},
		{Name: "Done", IsDone: true},
	}
	cols := DefaultColumns(uuid.New, specs)
	require.Len(t, cols, 3)
	seen := map[ColumnID]bool{}
	for i, c := range cols {
		require.Equal(t, i, c.Position)
		require.Equal(t, specs[i].Name, c.Name)
		require.Equal(t, specs[i].IsDone, c.IsDone)
		require.NotEqual(t, uuid.Nil, uuid.UUID(c.ID))
		require.False(t, seen[c.ID], "ids must be unique")
		seen[c.ID] = true
	}
}
