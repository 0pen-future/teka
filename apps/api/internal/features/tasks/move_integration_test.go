//go:build integration

package tasks_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/tasks"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/testutil"
)

// minPositionGap mirrors pkg/kanban's floor: no two neighbours in a column
// may ever sit closer than this after a move.
const minPositionGap = 1e-6

// afterTask builds a MoveTaskRequest that lands the moved task directly below
// anchor in col.
func afterTask(col, anchor uuid.UUID) tasks.MoveTaskRequest {
	return tasks.MoveTaskRequest{ColumnID: col, AfterTaskID: tasks.Optional[uuid.UUID]{Set: true, Value: &anchor}}
}

// columnTasks returns the ids and positions of col's tasks as GET board
// reports them, in board order.
func columnTasks(t *testing.T, e *tasksEnv, scope authctx.Scope, col uuid.UUID) ([]uuid.UUID, []float64) {
	t.Helper()
	resp, err := e.svc.Board(context.Background(), scope)
	require.NoError(t, err)
	for _, c := range resp.Columns {
		if c.ID != col {
			continue
		}
		require.False(t, c.HasMore, "the assertion needs the whole column")
		ids := make([]uuid.UUID, len(c.Tasks))
		positions := make([]float64, len(c.Tasks))
		for i, task := range c.Tasks {
			ids[i] = task.ID
			positions[i] = task.Position
		}
		return ids, positions
	}
	t.Fatalf("column %s not on the board", col)
	return nil, nil
}

// TestMoveTaskAfterReordersWithinAColumn asserts a same-column move lands
// directly below after_task_id and GET board reflects the new order.
func TestMoveTaskAfterReordersWithinAColumn(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	col := seedColumn(t, e.db, scope.CenterID, "To do", 0, false)
	a := seedTask(t, e.db, scope.CenterID, col, owner.ID, "a", seedTaskOpts{position: 0})
	b := seedTask(t, e.db, scope.CenterID, col, owner.ID, "b", seedTaskOpts{position: 1})
	c := seedTask(t, e.db, scope.CenterID, col, owner.ID, "c", seedTaskOpts{position: 2})

	moved, err := e.svc.MoveTask(context.Background(), scope, a, afterTask(col, c))
	require.NoError(t, err)
	require.Equal(t, 3.0, moved.Position, "after the last task: one past it")
	ids, _ := columnTasks(t, e, scope, col)
	require.Equal(t, []uuid.UUID{b, c, a}, ids)

	moved, err = e.svc.MoveTask(context.Background(), scope, a, afterTask(col, b))
	require.NoError(t, err)
	require.Equal(t, 1.5, moved.Position, "between b and c, ignoring a's own old slot")
	ids, _ = columnTasks(t, e, scope, col)
	require.Equal(t, []uuid.UUID{b, a, c}, ids)
}

// TestMoveTaskAfterAcrossColumns asserts a cross-column move lands between
// the anchor and its successor and still applies the done-column stamp.
func TestMoveTaskAfterAcrossColumns(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	todo := seedColumn(t, e.db, scope.CenterID, "To do", 0, false)
	done := seedColumn(t, e.db, scope.CenterID, "Done", 1, true)
	first := seedTask(t, e.db, scope.CenterID, done, owner.ID, "first", seedTaskOpts{position: 0})
	second := seedTask(t, e.db, scope.CenterID, done, owner.ID, "second", seedTaskOpts{position: 1})
	moving := seedTask(t, e.db, scope.CenterID, todo, owner.ID, "moving", seedTaskOpts{position: 0})

	moved, err := e.svc.MoveTask(context.Background(), scope, moving, afterTask(done, first))
	require.NoError(t, err)
	require.Equal(t, done, moved.ColumnID)
	require.Equal(t, 0.5, moved.Position)
	require.NotNil(t, moved.CompletedAt)
	ids, _ := columnTasks(t, e, scope, done)
	require.Equal(t, []uuid.UUID{first, moving, second}, ids)
}

// TestMoveTaskWithoutAfterStillLandsAtTop asserts the original body shape
// (column_id only) keeps placing the task above every task in the column.
func TestMoveTaskWithoutAfterStillLandsAtTop(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	todo := seedColumn(t, e.db, scope.CenterID, "To do", 0, false)
	doing := seedColumn(t, e.db, scope.CenterID, "Doing", 1, false)
	existing := seedTask(t, e.db, scope.CenterID, doing, owner.ID, "existing", seedTaskOpts{position: 5})
	moving := seedTask(t, e.db, scope.CenterID, todo, owner.ID, "moving", seedTaskOpts{position: 0})

	moved, err := e.svc.MoveTask(context.Background(), scope, moving, tasks.MoveTaskRequest{ColumnID: doing})
	require.NoError(t, err)
	require.Equal(t, 4.0, moved.Position)
	ids, _ := columnTasks(t, e, scope, doing)
	require.Equal(t, []uuid.UUID{moving, existing}, ids)
}

// TestMoveTaskRenormalizesAfterRepeatedInsertsBetweenTheSamePair drives the
// float-gap floor: inserting between the same two tasks over and over halves
// the gap each time, so without renormalization positions would collapse.
// The board must keep the exact insertion order and never leave two
// neighbours closer than minPositionGap.
func TestMoveTaskRenormalizesAfterRepeatedInsertsBetweenTheSamePair(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	target := seedColumn(t, e.db, scope.CenterID, "Target", 0, false)
	source := seedColumn(t, e.db, scope.CenterID, "Source", 1, false)
	head := seedTask(t, e.db, scope.CenterID, target, owner.ID, "head", seedTaskOpts{position: 0})
	tail := seedTask(t, e.db, scope.CenterID, target, owner.ID, "tail", seedTaskOpts{position: 1})

	// 45 halvings of a gap of 1 pass far below 1e-6 (2^-20 already does), so
	// the column is renormalized at least twice on the way, while the total
	// stays under the board's per-column cap so the whole column is visible.
	const inserts = 45
	var inserted []uuid.UUID
	for i := 0; i < inserts; i++ {
		id := seedTask(t, e.db, scope.CenterID, source, owner.ID, "x", seedTaskOpts{position: float64(i)})
		_, err := e.svc.MoveTask(context.Background(), scope, id, afterTask(target, head))
		require.NoError(t, err, "insert %d", i)
		inserted = append(inserted, id)
	}

	want := []uuid.UUID{head}
	for i := len(inserted) - 1; i >= 0; i-- {
		want = append(want, inserted[i])
	}
	want = append(want, tail)

	ids, positions := columnTasks(t, e, scope, target)
	require.Equal(t, want, ids, "each insert lands directly below head, pushing earlier inserts down")
	for i := 1; i < len(positions); i++ {
		require.GreaterOrEqual(t, positions[i]-positions[i-1], minPositionGap, "gap between %d and %d", i-1, i)
	}
}

// TestMoveTaskAfterRejectsAnchorsOutsideTheDestination asserts after_task_id
// must be a live task of the destination column: another column, another
// tenant, a soft-deleted task, and an unknown id all map to a 422 on the
// after_task_id field, and the task stays where it was.
func TestMoveTaskAfterRejectsAnchorsOutsideTheDestination(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	_, otherOwner := testutil.Teacher(t, e.db)
	otherScope := testutil.ScopeFor(t, e.db, otherOwner.ID)

	todo := seedColumn(t, e.db, scope.CenterID, "To do", 0, false)
	done := seedColumn(t, e.db, scope.CenterID, "Done", 1, true)
	moving := seedTask(t, e.db, scope.CenterID, todo, owner.ID, "moving", seedTaskOpts{position: 0})
	sibling := seedTask(t, e.db, scope.CenterID, todo, owner.ID, "sibling", seedTaskOpts{position: 1})
	gone := seedTask(t, e.db, scope.CenterID, done, owner.ID, "gone", seedTaskOpts{position: 0, deletedAt: true})
	foreignCol := seedColumn(t, e.db, otherScope.CenterID, "Done", 0, true)
	foreign := seedTask(t, e.db, otherScope.CenterID, foreignCol, otherOwner.ID, "foreign", seedTaskOpts{position: 0})

	for name, anchor := range map[string]uuid.UUID{
		"another column":  sibling,
		"soft-deleted":    gone,
		"another tenant":  foreign,
		"unknown id":      uuid.New(),
		"the moving task": moving,
	} {
		_, err := e.svc.MoveTask(context.Background(), scope, moving, afterTask(done, anchor))
		ae := appErr(t, err)
		require.Equal(t, apperror.CodeValidation, ae.Code, name)
		require.Contains(t, ae.Fields, "after_task_id", name)
	}

	ids, _ := columnTasks(t, e, scope, todo)
	require.Equal(t, []uuid.UUID{moving, sibling}, ids, "rejected moves change nothing")
}

// TestConcurrentMovesAfterTheSameAnchorNeverCollide runs two moves into the
// same slot at once. The per-column advisory lock serializes them, so both
// land below the anchor at distinct positions and the board stays a strict
// order; without the lock both would compute the same midpoint.
func TestConcurrentMovesAfterTheSameAnchorNeverCollide(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	target := seedColumn(t, e.db, scope.CenterID, "Target", 0, false)
	source := seedColumn(t, e.db, scope.CenterID, "Source", 1, false)
	head := seedTask(t, e.db, scope.CenterID, target, owner.ID, "head", seedTaskOpts{position: 0})
	tail := seedTask(t, e.db, scope.CenterID, target, owner.ID, "tail", seedTaskOpts{position: 1})
	movers := []uuid.UUID{
		seedTask(t, e.db, scope.CenterID, source, owner.ID, "m1", seedTaskOpts{position: 0}),
		seedTask(t, e.db, scope.CenterID, source, owner.ID, "m2", seedTaskOpts{position: 1}),
	}

	errs := make([]error, len(movers))
	var wg sync.WaitGroup
	for i, id := range movers {
		wg.Add(1)
		go func(i int, id uuid.UUID) {
			defer wg.Done()
			_, errs[i] = e.svc.MoveTask(context.Background(), scope, id, afterTask(target, head))
		}(i, id)
	}
	wg.Wait()
	for i, err := range errs {
		require.NoError(t, err, "mover %d", i)
	}

	ids, positions := columnTasks(t, e, scope, target)
	require.Len(t, ids, 4)
	require.Equal(t, head, ids[0])
	require.Equal(t, tail, ids[3])
	require.ElementsMatch(t, movers, ids[1:3])
	for i := 1; i < len(positions); i++ {
		require.Greater(t, positions[i], positions[i-1], "positions must be strictly increasing")
	}
}

// TestConcurrentTopMoveSurvivesRenormalization runs a top move (no
// after_task_id) against a move that renormalizes the same column. Both
// placements take the column lock, so whichever commits second sees the
// other's rows: the top move stays first either way. Without the lock the
// renormalization's per-row updates could overwrite the top move's position
// and silently undo it.
func TestConcurrentTopMoveSurvivesRenormalization(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	target := seedColumn(t, e.db, scope.CenterID, "Target", 0, false)
	source := seedColumn(t, e.db, scope.CenterID, "Source", 1, false)
	// head and tail sit closer than the floor allows bisecting, so the
	// after-move below must renormalize the column.
	head := seedTask(t, e.db, scope.CenterID, target, owner.ID, "head", seedTaskOpts{position: 0})
	tail := seedTask(t, e.db, scope.CenterID, target, owner.ID, "tail", seedTaskOpts{position: minPositionGap})
	rising := seedTask(t, e.db, scope.CenterID, target, owner.ID, "rising", seedTaskOpts{position: 5})
	between := seedTask(t, e.db, scope.CenterID, source, owner.ID, "between", seedTaskOpts{position: 0})

	var wg sync.WaitGroup
	errs := make([]error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, errs[0] = e.svc.MoveTask(context.Background(), scope, rising, tasks.MoveTaskRequest{ColumnID: target})
	}()
	go func() {
		defer wg.Done()
		_, errs[1] = e.svc.MoveTask(context.Background(), scope, between, afterTask(target, head))
	}()
	wg.Wait()
	require.NoError(t, errs[0], "top move")
	require.NoError(t, errs[1], "after move")

	ids, positions := columnTasks(t, e, scope, target)
	require.Equal(t, []uuid.UUID{rising, head, between, tail}, ids, "the top move is first and the after move sits below head")
	for i := 1; i < len(positions); i++ {
		require.GreaterOrEqual(t, positions[i]-positions[i-1], minPositionGap, "gap between %d and %d", i-1, i)
	}
}
