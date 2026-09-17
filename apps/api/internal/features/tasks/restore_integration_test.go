//go:build integration

package tasks_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/tasks"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/testutil"
)

// TestRestoreTaskRevivesItWithItsOriginalPlacement asserts a soft-deleted
// task comes back with the column, position, and completed_at it had right
// before the delete, and reappears on the board.
func TestRestoreTaskRevivesItWithItsOriginalPlacement(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	col := seedColumn(t, e.db, scope.CenterID, "To do", 0, false)
	taskID := seedTask(t, e.db, scope.CenterID, col, owner.ID, "come back", seedTaskOpts{position: 3})

	require.NoError(t, e.svc.DeleteTask(context.Background(), scope, taskID))

	board, err := e.svc.Board(context.Background(), scope, tasks.BoardQuery{})
	require.NoError(t, err)
	require.Empty(t, board.Columns[0].Tasks, "a soft-deleted task must not be visible before restore")

	resp, err := e.svc.RestoreTask(context.Background(), scope, taskID)
	require.NoError(t, err)
	require.Equal(t, taskID, resp.ID)
	require.Equal(t, col, resp.ColumnID)
	require.Equal(t, float64(3), resp.Position)
	require.Nil(t, resp.CompletedAt)

	board, err = e.svc.Board(context.Background(), scope, tasks.BoardQuery{})
	require.NoError(t, err)
	require.Len(t, board.Columns[0].Tasks, 1)
	require.Equal(t, taskID, board.Columns[0].Tasks[0].ID)
}

// TestRestoreTaskTwiceIsNotFoundOnTheSecondCall asserts restore is not
// idempotent past the first call: once live, the task no longer matches
// GetDeleted's "deleted_at IS NOT NULL" predicate.
func TestRestoreTaskTwiceIsNotFoundOnTheSecondCall(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	col := seedColumn(t, e.db, scope.CenterID, "To do", 0, false)
	taskID := seedTask(t, e.db, scope.CenterID, col, owner.ID, "t", seedTaskOpts{})
	require.NoError(t, e.svc.DeleteTask(context.Background(), scope, taskID))

	_, err := e.svc.RestoreTask(context.Background(), scope, taskID)
	require.NoError(t, err)

	_, err = e.svc.RestoreTask(context.Background(), scope, taskID)
	require.Equal(t, apperror.CodeNotFound, appErr(t, err).Code)
}

// TestRestoreTaskOnALiveTaskIsNotFound asserts a task that was never deleted
// cannot be "restored" — GetDeleted only matches soft-deleted rows.
func TestRestoreTaskOnALiveTaskIsNotFound(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	col := seedColumn(t, e.db, scope.CenterID, "To do", 0, false)
	taskID := seedTask(t, e.db, scope.CenterID, col, owner.ID, "still alive", seedTaskOpts{})

	_, err := e.svc.RestoreTask(context.Background(), scope, taskID)
	require.Equal(t, apperror.CodeNotFound, appErr(t, err).Code)
}
