package tasks

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/shared/events"
	"teka/apps/api/pkg/kanban"
)

// fakeColumnRepo is an in-memory kanban.ColumnRepository, keyed by (tenant,
// id), so Service can be exercised against pkg/kanban's real DefaultPolicy
// and use-case logic without a database.
type fakeColumnRepo struct {
	cols map[kanban.ColumnID]kanban.Column
}

func newFakeColumnRepo() *fakeColumnRepo {
	return &fakeColumnRepo{cols: map[kanban.ColumnID]kanban.Column{}}
}

func (r *fakeColumnRepo) List(_ context.Context, tenant kanban.TenantID) ([]kanban.Column, error) {
	var out []kanban.Column
	for _, c := range r.cols {
		if c.TenantID == tenant {
			out = append(out, c)
		}
	}
	return out, nil
}

func (r *fakeColumnRepo) Get(_ context.Context, tenant kanban.TenantID, id kanban.ColumnID) (kanban.Column, error) {
	c, ok := r.cols[id]
	if !ok || c.TenantID != tenant {
		return kanban.Column{}, kanban.ErrColumnNotFound
	}
	return c, nil
}

func (r *fakeColumnRepo) Create(_ context.Context, _ kanban.TenantID, col kanban.Column) (kanban.Column, error) {
	r.cols[col.ID] = col
	return col, nil
}

func (r *fakeColumnRepo) Update(_ context.Context, tenant kanban.TenantID, col kanban.Column) (kanban.Column, error) {
	if existing, ok := r.cols[col.ID]; !ok || existing.TenantID != tenant {
		return kanban.Column{}, kanban.ErrColumnNotFound
	}
	r.cols[col.ID] = col
	return col, nil
}

func (r *fakeColumnRepo) Delete(_ context.Context, tenant kanban.TenantID, id kanban.ColumnID) error {
	c, ok := r.cols[id]
	if !ok || c.TenantID != tenant {
		return kanban.ErrColumnNotFound
	}
	delete(r.cols, id)
	return nil
}

func (r *fakeColumnRepo) UpdatePositions(_ context.Context, _ kanban.TenantID, order []kanban.ColumnID) error {
	for i, id := range order {
		c, ok := r.cols[id]
		if !ok {
			return kanban.ErrColumnNotFound
		}
		c.Position = i
		r.cols[id] = c
	}
	return nil
}

func (r *fakeColumnRepo) CountByTenant(_ context.Context, tenant kanban.TenantID) (int, error) {
	n := 0
	for _, c := range r.cols {
		if c.TenantID == tenant {
			n++
		}
	}
	return n, nil
}

func (r *fakeColumnRepo) ExistsName(_ context.Context, tenant kanban.TenantID, name string) (bool, error) {
	for _, c := range r.cols {
		if c.TenantID == tenant && c.Name == name {
			return true, nil
		}
	}
	return false, nil
}

// fakeTaskRepo is an in-memory kanban.TaskRepository, keyed by id.
type fakeTaskRepo struct {
	tasks   map[kanban.TaskID]kanban.Task
	deleted map[kanban.TaskID]bool
}

func newFakeTaskRepo() *fakeTaskRepo {
	return &fakeTaskRepo{tasks: map[kanban.TaskID]kanban.Task{}, deleted: map[kanban.TaskID]bool{}}
}

// ListBoard ignores limit: this fake backs the adapter's own Board tests,
// which assert on the Go-side hasMore/slicing logic against an unfiltered
// result — the per-column SQL cap itself is covered by the real repository.
func (r *fakeTaskRepo) ListBoard(_ context.Context, tenant kanban.TenantID, vis kanban.Visibility, _ int) ([]kanban.Task, error) {
	var out []kanban.Task
	for id, t := range r.tasks {
		if r.deleted[id] || t.TenantID != tenant {
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

func (r *fakeTaskRepo) MinPositionInColumn(_ context.Context, tenant kanban.TenantID, col kanban.ColumnID) (float64, bool, error) {
	minPos, found := 0.0, false
	for id, t := range r.tasks {
		if r.deleted[id] || t.TenantID != tenant || t.ColumnID != col {
			continue
		}
		if !found || t.Position < minPos {
			minPos = t.Position
			found = true
		}
	}
	return minPos, found, nil
}

func (r *fakeTaskRepo) Get(_ context.Context, tenant kanban.TenantID, id kanban.TaskID) (kanban.Task, error) {
	t, ok := r.tasks[id]
	if !ok || r.deleted[id] || t.TenantID != tenant {
		return kanban.Task{}, kanban.ErrTaskNotFound
	}
	return t, nil
}

func (r *fakeTaskRepo) Create(_ context.Context, _ kanban.TenantID, task kanban.Task) (kanban.Task, error) {
	r.tasks[task.ID] = task
	return task, nil
}

func (r *fakeTaskRepo) Update(_ context.Context, tenant kanban.TenantID, task kanban.Task) (kanban.Task, error) {
	existing, ok := r.tasks[task.ID]
	if !ok || r.deleted[task.ID] || existing.TenantID != tenant {
		return kanban.Task{}, kanban.ErrTaskNotFound
	}
	r.tasks[task.ID] = task
	return task, nil
}

func (r *fakeTaskRepo) SoftDelete(_ context.Context, tenant kanban.TenantID, id kanban.TaskID) error {
	t, ok := r.tasks[id]
	if !ok || r.deleted[id] || t.TenantID != tenant {
		return kanban.ErrTaskNotFound
	}
	r.deleted[id] = true
	return nil
}

func (r *fakeTaskRepo) CountInColumn(_ context.Context, tenant kanban.TenantID, col kanban.ColumnID) (int, error) {
	n := 0
	for _, t := range r.tasks {
		if t.TenantID == tenant && t.ColumnID == col {
			n++
		}
	}
	return n, nil
}

func (r *fakeTaskRepo) MoveAllToColumn(_ context.Context, tenant kanban.TenantID, from, to kanban.ColumnID, completedAt *time.Time) (int, error) {
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

func (r *fakeTaskRepo) UnassignBy(_ context.Context, tenant kanban.TenantID, actor kanban.ActorID) (int, error) {
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

func (r *fakeTaskRepo) ReassignCreator(_ context.Context, tenant kanban.TenantID, from, to kanban.ActorID) (int, error) {
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

// fakeUnitOfWork runs fn directly: the fakes above have no real transaction
// to join, so Within only needs to preserve "run fn, propagate its error".
type fakeUnitOfWork struct{}

func (fakeUnitOfWork) Within(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

// fakeMemberChecker reports membership from a fixed set, so assignee
// validation can be tested without a center_members table.
type fakeMemberChecker struct{ members map[kanban.ActorID]bool }

func (c fakeMemberChecker) IsMember(_ context.Context, _ kanban.TenantID, actor kanban.ActorID) (bool, error) {
	return c.members[actor], nil
}

// testEnv bundles a Service backed entirely by fakes plus the repos, so a
// test can seed data directly and then exercise Service's adapter behavior
// (scope translation, DTO shaping, error translation) against pkg/kanban's
// real, unmodified use-cases and DefaultPolicy.
type testEnv struct {
	svc      *Service
	cols     *fakeColumnRepo
	tasksRep *fakeTaskRepo
	bus      *events.SyncBus
}

func newTestEnv(members ...uuid.UUID) *testEnv {
	cols := newFakeColumnRepo()
	tasksRep := newFakeTaskRepo()
	memberSet := make(map[kanban.ActorID]bool, len(members))
	for _, m := range members {
		memberSet[kanban.ActorID(m)] = true
	}
	bus := events.NewSync()
	core := kanban.NewService(
		kanban.Repositories{Columns: cols, Tasks: tasksRep},
		fakeUnitOfWork{},
		kanban.DefaultPolicy{},
		fakeMemberChecker{members: memberSet},
		newEventSink(bus),
		kanban.WithIDGen(uuid.New),
	)
	return &testEnv{svc: &Service{core: core}, cols: cols, tasksRep: tasksRep, bus: bus}
}

func scopeFor(teacherID, centerID uuid.UUID, isOwner bool, perms ...string) authctx.Scope {
	set := make(authctx.PermSet, len(perms))
	for _, p := range perms {
		set[p] = struct{}{}
	}
	return authctx.Scope{TeacherID: teacherID, CenterID: centerID, IsOwner: isOwner, Perms: set}
}

func appErr(t *testing.T, err error) *apperror.AppError {
	t.Helper()
	var ae *apperror.AppError
	require.True(t, errors.As(err, &ae), "expected *apperror.AppError, got %T (%v)", err, err)
	return ae
}

func TestBoardScopeMineForOrdinaryMember(t *testing.T) {
	env := newTestEnv()
	center := uuid.New()
	owner := uuid.New()
	member := uuid.New()

	col := kanban.Column{ID: kanban.ColumnID(uuid.New()), TenantID: kanban.TenantID(center), Name: "To do"}
	env.cols.cols[col.ID] = col
	env.tasksRep.tasks[kanban.TaskID(uuid.New())] = kanban.Task{
		ID: kanban.TaskID(uuid.New()), TenantID: kanban.TenantID(center), ColumnID: col.ID,
		CreatedBy: kanban.ActorID(owner), Title: "owner's task",
	}

	resp, err := env.svc.Board(context.Background(), scopeFor(member, center, false))
	require.NoError(t, err)
	require.Equal(t, "mine", resp.Scope, "a plain member without tasks.view_all sees only their own rows")
	require.Len(t, resp.Columns, 1)
	require.Empty(t, resp.Columns[0].Tasks, "the owner's task must not appear in a non-owner's board")
}

func TestBoardScopeCenterForOwner(t *testing.T) {
	env := newTestEnv()
	center := uuid.New()
	col := kanban.Column{ID: kanban.ColumnID(uuid.New()), TenantID: kanban.TenantID(center)}
	env.cols.cols[col.ID] = col

	resp, err := env.svc.Board(context.Background(), scopeFor(uuid.New(), center, true))
	require.NoError(t, err)
	require.Equal(t, "center", resp.Scope, "the owner always sees the whole board")
}

func TestBoardScopeCenterForViewAllHolder(t *testing.T) {
	env := newTestEnv()
	center := uuid.New()
	col := kanban.Column{ID: kanban.ColumnID(uuid.New()), TenantID: kanban.TenantID(center)}
	env.cols.cols[col.ID] = col

	resp, err := env.svc.Board(context.Background(), scopeFor(uuid.New(), center, false, authctx.PermTasksViewAll))
	require.NoError(t, err)
	require.Equal(t, "center", resp.Scope, "tasks.view_all degrades a non-owner into the center-wide view")
}

func TestBoardCapsTasksPerColumnAndReportsHasMore(t *testing.T) {
	env := newTestEnv()
	center := uuid.New()
	owner := uuid.New()
	col := kanban.Column{ID: kanban.ColumnID(uuid.New()), TenantID: kanban.TenantID(center)}
	env.cols.cols[col.ID] = col
	for i := 0; i < boardTasksPerColumn+1; i++ {
		tid := kanban.TaskID(uuid.New())
		env.tasksRep.tasks[tid] = kanban.Task{
			ID: tid, TenantID: kanban.TenantID(center), ColumnID: col.ID,
			CreatedBy: kanban.ActorID(owner), Title: "t", Position: float64(i),
		}
	}

	resp, err := env.svc.Board(context.Background(), scopeFor(owner, center, true))
	require.NoError(t, err)
	require.Len(t, resp.Columns, 1)
	require.Len(t, resp.Columns[0].Tasks, boardTasksPerColumn)
	require.True(t, resp.Columns[0].HasMore, "a column past the cap must report has_more")
}

func TestBoardUnderCapReportsNoHasMore(t *testing.T) {
	env := newTestEnv()
	center := uuid.New()
	owner := uuid.New()
	col := kanban.Column{ID: kanban.ColumnID(uuid.New()), TenantID: kanban.TenantID(center)}
	env.cols.cols[col.ID] = col
	tid := kanban.TaskID(uuid.New())
	env.tasksRep.tasks[tid] = kanban.Task{ID: tid, TenantID: kanban.TenantID(center), ColumnID: col.ID, CreatedBy: kanban.ActorID(owner)}

	resp, err := env.svc.Board(context.Background(), scopeFor(owner, center, true))
	require.NoError(t, err)
	require.False(t, resp.Columns[0].HasMore)
}

func TestCreateTaskDefaultsToFirstColumnByPosition(t *testing.T) {
	env := newTestEnv()
	center := uuid.New()
	owner := uuid.New()
	first := kanban.Column{ID: kanban.ColumnID(uuid.New()), TenantID: kanban.TenantID(center), Position: 0, Name: "first"}
	second := kanban.Column{ID: kanban.ColumnID(uuid.New()), TenantID: kanban.TenantID(center), Position: 1, Name: "second"}
	env.cols.cols[first.ID] = first
	env.cols.cols[second.ID] = second

	resp, err := env.svc.CreateTask(context.Background(), scopeFor(owner, center, true), CreateTaskRequest{Title: "no column given"})
	require.NoError(t, err)
	require.Equal(t, uuid.UUID(first.ID), resp.ColumnID, "an absent column_id must default to the board's position-0 column")
}

func TestCreateTaskWithNoColumnsIsRejected(t *testing.T) {
	env := newTestEnv()
	center := uuid.New()
	_, err := env.svc.CreateTask(context.Background(), scopeFor(uuid.New(), center, true), CreateTaskRequest{Title: "x"})
	ae := appErr(t, err)
	require.Equal(t, apperror.CodeValidation, ae.Code)
}

func TestCreateTaskRejectsMalformedDueOn(t *testing.T) {
	env := newTestEnv()
	center := uuid.New()
	owner := uuid.New()
	col := kanban.Column{ID: kanban.ColumnID(uuid.New()), TenantID: kanban.TenantID(center)}
	env.cols.cols[col.ID] = col

	bad := "13/45/2026"
	_, err := env.svc.CreateTask(context.Background(), scopeFor(owner, center, true), CreateTaskRequest{Title: "x", DueOn: &bad})
	ae := appErr(t, err)
	require.Equal(t, apperror.CodeValidation, ae.Code)
	require.Contains(t, ae.Fields, "due_on")
}

func TestCreateTaskRejectsNonMemberAssignee(t *testing.T) {
	env := newTestEnv() // no members registered
	center := uuid.New()
	owner := uuid.New()
	col := kanban.Column{ID: kanban.ColumnID(uuid.New()), TenantID: kanban.TenantID(center)}
	env.cols.cols[col.ID] = col
	notMember := uuid.New()

	_, err := env.svc.CreateTask(context.Background(), scopeFor(owner, center, true), CreateTaskRequest{
		Title: "x", ColumnID: (*uuid.UUID)(&col.ID), AssigneeID: &notMember,
	})
	ae := appErr(t, err)
	require.Equal(t, apperror.CodeValidation, ae.Code)
	require.Contains(t, ae.Fields, "assignee_id")
}

func TestUpdateTaskMergesTriStateAssigneeAndDueOn(t *testing.T) {
	member := uuid.New()
	env := newTestEnv(member)
	center := uuid.New()
	owner := uuid.New()
	col := kanban.Column{ID: kanban.ColumnID(uuid.New()), TenantID: kanban.TenantID(center)}
	env.cols.cols[col.ID] = col
	tid := kanban.TaskID(uuid.New())
	due := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	env.tasksRep.tasks[tid] = kanban.Task{
		ID: tid, TenantID: kanban.TenantID(center), ColumnID: col.ID,
		CreatedBy: kanban.ActorID(owner), Title: "original", DueOn: &due,
	}

	// Absent title/description/priority leave them unchanged; explicit
	// assignee_id sets it.
	memberCopy := member
	resp, err := env.svc.UpdateTask(context.Background(), scopeFor(owner, center, true), uuid.UUID(tid), UpdateTaskRequest{
		AssigneeID: Optional[uuid.UUID]{Set: true, Value: &memberCopy},
	})
	require.NoError(t, err)
	require.Equal(t, "original", resp.Title, "an absent field must stay unchanged")
	require.NotNil(t, resp.AssigneeID)
	require.Equal(t, member, *resp.AssigneeID)
	require.NotNil(t, resp.DueOn, "due_on must stay unchanged when the patch never mentions it")

	// A second update explicitly nulls both assignee_id and due_on.
	resp, err = env.svc.UpdateTask(context.Background(), scopeFor(owner, center, true), uuid.UUID(tid), UpdateTaskRequest{
		AssigneeID: Optional[uuid.UUID]{Set: true, Value: nil},
		DueOn:      Optional[string]{Set: true, Value: nil},
	})
	require.NoError(t, err)
	require.Nil(t, resp.AssigneeID, "an explicit null must clear assignee_id")
	require.Nil(t, resp.DueOn, "an explicit null must clear due_on")
}

func TestUpdateTaskByNonCreatorNonOwnerIsForbidden(t *testing.T) {
	env := newTestEnv()
	center := uuid.New()
	creator := uuid.New()
	other := uuid.New()
	col := kanban.Column{ID: kanban.ColumnID(uuid.New()), TenantID: kanban.TenantID(center)}
	env.cols.cols[col.ID] = col
	tid := kanban.TaskID(uuid.New())
	env.tasksRep.tasks[tid] = kanban.Task{ID: tid, TenantID: kanban.TenantID(center), ColumnID: col.ID, CreatedBy: kanban.ActorID(creator), Title: "t"}

	title := "hijacked"
	_, err := env.svc.UpdateTask(context.Background(), scopeFor(other, center, false), uuid.UUID(tid), UpdateTaskRequest{Title: &title})
	ae := appErr(t, err)
	require.Equal(t, apperror.CodeForbidden, ae.Code)
}

func TestAssigneeAloneCannotEditButCanMove(t *testing.T) {
	assignee := uuid.New()
	env := newTestEnv(assignee)
	center := uuid.New()
	creator := uuid.New()
	col := kanban.Column{ID: kanban.ColumnID(uuid.New()), TenantID: kanban.TenantID(center), Name: "a"}
	target := kanban.Column{ID: kanban.ColumnID(uuid.New()), TenantID: kanban.TenantID(center), Name: "b"}
	env.cols.cols[col.ID] = col
	env.cols.cols[target.ID] = target
	tid := kanban.TaskID(uuid.New())
	aid := kanban.ActorID(assignee)
	env.tasksRep.tasks[tid] = kanban.Task{
		ID: tid, TenantID: kanban.TenantID(center), ColumnID: col.ID,
		CreatedBy: kanban.ActorID(creator), AssigneeID: &aid, Title: "t",
	}

	title := "hijacked"
	_, err := env.svc.UpdateTask(context.Background(), scopeFor(assignee, center, false), uuid.UUID(tid), UpdateTaskRequest{Title: &title})
	require.Equal(t, apperror.CodeForbidden, appErr(t, err).Code, "an assignee who is not the creator may not edit")

	moved, err := env.svc.MoveTask(context.Background(), scopeFor(assignee, center, false), uuid.UUID(tid), MoveTaskRequest{ColumnID: uuid.UUID(target.ID)})
	require.NoError(t, err, "an assignee may move a task even though they may not edit it")
	require.Equal(t, uuid.UUID(target.ID), moved.ColumnID)
}

func TestDeleteColumnReportsMovedCountAndPublishesEvent(t *testing.T) {
	env := newTestEnv()
	center := uuid.New()
	owner := uuid.New()
	source := kanban.Column{ID: kanban.ColumnID(uuid.New()), TenantID: kanban.TenantID(center), Name: "source"}
	dest := kanban.Column{ID: kanban.ColumnID(uuid.New()), TenantID: kanban.TenantID(center), Name: "dest"}
	env.cols.cols[source.ID] = source
	env.cols.cols[dest.ID] = dest
	for i := 0; i < 3; i++ {
		tid := kanban.TaskID(uuid.New())
		env.tasksRep.tasks[tid] = kanban.Task{ID: tid, TenantID: kanban.TenantID(center), ColumnID: source.ID, CreatedBy: kanban.ActorID(owner)}
	}

	var published []ColumnDeleted
	env.bus.Subscribe("test", 0, func(_ context.Context, e events.Event) {
		if cd, ok := e.(ColumnDeleted); ok {
			published = append(published, cd)
		}
	})

	destID := uuid.UUID(dest.ID)
	resp, err := env.svc.DeleteColumn(context.Background(), scopeFor(owner, center, true), uuid.UUID(source.ID), &destID)
	require.NoError(t, err)
	require.Equal(t, 3, resp.MovedCount, "the response must report how many tasks the delete moved")

	require.Len(t, published, 1)
	require.Equal(t, 3, published[0].MovedCount)
	require.Equal(t, uuid.UUID(source.ID), published[0].ColumnID)
	require.NotNil(t, published[0].MoveTo)
	require.Equal(t, destID, *published[0].MoveTo)
	require.Equal(t, center, published[0].CenterID)
	require.Equal(t, owner, published[0].ActorID)
}

func TestDeleteColumnWithoutMoveToOnNonEmptyColumnConflicts(t *testing.T) {
	env := newTestEnv()
	center := uuid.New()
	owner := uuid.New()
	source := kanban.Column{ID: kanban.ColumnID(uuid.New()), TenantID: kanban.TenantID(center)}
	dest := kanban.Column{ID: kanban.ColumnID(uuid.New()), TenantID: kanban.TenantID(center)}
	env.cols.cols[source.ID] = source
	env.cols.cols[dest.ID] = dest
	tid := kanban.TaskID(uuid.New())
	env.tasksRep.tasks[tid] = kanban.Task{ID: tid, TenantID: kanban.TenantID(center), ColumnID: source.ID, CreatedBy: kanban.ActorID(owner)}

	_, err := env.svc.DeleteColumn(context.Background(), scopeFor(owner, center, true), uuid.UUID(source.ID), nil)
	require.Equal(t, apperror.CodeConflict, appErr(t, err).Code)
}

func TestDeleteColumnLastColumnIsRejected(t *testing.T) {
	env := newTestEnv()
	center := uuid.New()
	owner := uuid.New()
	only := kanban.Column{ID: kanban.ColumnID(uuid.New()), TenantID: kanban.TenantID(center)}
	env.cols.cols[only.ID] = only

	_, err := env.svc.DeleteColumn(context.Background(), scopeFor(owner, center, true), uuid.UUID(only.ID), nil)
	require.Equal(t, apperror.CodeConflict, appErr(t, err).Code)
}

func TestCreateColumnWithoutManageBoardIsForbidden(t *testing.T) {
	env := newTestEnv()
	center := uuid.New()
	member := uuid.New()
	existing := kanban.Column{ID: kanban.ColumnID(uuid.New()), TenantID: kanban.TenantID(center)}
	env.cols.cols[existing.ID] = existing

	_, err := env.svc.CreateColumn(context.Background(), scopeFor(member, center, false), CreateColumnRequest{Name: "New"})
	require.Equal(t, apperror.CodeForbidden, appErr(t, err).Code)
}

func TestCreateColumnRejectsDuplicateName(t *testing.T) {
	env := newTestEnv()
	center := uuid.New()
	owner := uuid.New()
	existing := kanban.Column{ID: kanban.ColumnID(uuid.New()), TenantID: kanban.TenantID(center), Name: "Cần làm"}
	env.cols.cols[existing.ID] = existing

	_, err := env.svc.CreateColumn(context.Background(), scopeFor(owner, center, true), CreateColumnRequest{Name: "Cần làm"})
	require.Equal(t, apperror.CodeValidation, appErr(t, err).Code)
}

func TestHandoverOnDepartureUnassignsAndReassigns(t *testing.T) {
	env := newTestEnv()
	center := uuid.New()
	departed := uuid.New()
	successor := uuid.New()
	col := kanban.Column{ID: kanban.ColumnID(uuid.New()), TenantID: kanban.TenantID(center)}
	env.cols.cols[col.ID] = col

	assignedTask := kanban.TaskID(uuid.New())
	depID := kanban.ActorID(departed)
	env.tasksRep.tasks[assignedTask] = kanban.Task{
		ID: assignedTask, TenantID: kanban.TenantID(center), ColumnID: col.ID,
		CreatedBy: kanban.ActorID(uuid.New()), AssigneeID: &depID,
	}
	createdTask := kanban.TaskID(uuid.New())
	env.tasksRep.tasks[createdTask] = kanban.Task{
		ID: createdTask, TenantID: kanban.TenantID(center), ColumnID: col.ID,
		CreatedBy: kanban.ActorID(departed),
	}

	unassigned, reassigned, err := env.svc.HandoverOnDeparture(context.Background(), center, departed, successor)
	require.NoError(t, err)
	require.Equal(t, 1, unassigned)
	require.Equal(t, 1, reassigned)

	require.Nil(t, env.tasksRep.tasks[assignedTask].AssigneeID, "the departing member's assignment must be cleared")
	require.Equal(t, kanban.ActorID(successor), env.tasksRep.tasks[createdTask].CreatedBy, "creatorship must move to the successor")
}

func TestGetTaskNotFoundTranslatesTo404(t *testing.T) {
	env := newTestEnv()
	center := uuid.New()
	_, err := env.svc.GetTask(context.Background(), scopeFor(uuid.New(), center, true), uuid.New())
	require.Equal(t, apperror.CodeNotFound, appErr(t, err).Code)
}

func TestParseDueOnAcceptsNilAndRejectsBadFormat(t *testing.T) {
	got, err := parseDueOn(nil)
	require.NoError(t, err)
	require.Nil(t, got)

	bad := "not-a-date"
	_, err = parseDueOn(&bad)
	require.Error(t, err)

	good := "2026-03-14"
	got, err = parseDueOn(&good)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, 2026, got.Year())
}
