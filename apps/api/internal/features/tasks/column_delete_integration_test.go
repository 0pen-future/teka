//go:build integration

package tasks_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/tasks"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/events"
	"teka/apps/api/internal/testutil"
)

// appErr asserts err is an *apperror.AppError and returns it for further
// field/status inspection.
func appErr(t *testing.T, err error) *apperror.AppError {
	t.Helper()
	ae, ok := err.(*apperror.AppError)
	require.True(t, ok, "expected *apperror.AppError, got %T (%v)", err, err)
	return ae
}

// TestDeleteLastColumnIsRejected asserts the board's last remaining column
// can never be deleted, matching kanban.ErrLastColumn's 409.
func TestDeleteLastColumnIsRejected(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	only := seedColumn(t, e.db, scope.CenterID, "only", 0, false)

	_, err := e.svc.DeleteColumn(context.Background(), scope, only, nil)
	require.Equal(t, apperror.CodeConflict, appErr(t, err).Code)
}

// TestDeleteNonEmptyColumnWithoutMoveToConflicts asserts a non-empty column
// cannot be dropped unless the caller names a destination for its tasks.
func TestDeleteNonEmptyColumnWithoutMoveToConflicts(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	source := seedColumn(t, e.db, scope.CenterID, "source", 0, false)
	seedColumn(t, e.db, scope.CenterID, "dest", 1, false)
	seedTask(t, e.db, scope.CenterID, source, owner.ID, "t", seedTaskOpts{})

	_, err := e.svc.DeleteColumn(context.Background(), scope, source, nil)
	require.Equal(t, apperror.CodeConflict, appErr(t, err).Code)
}

// TestDeleteColumnMoveToItselfIsValidationError asserts move_to naming the
// column being deleted is rejected as a 422 validation failure on the
// move_to field, rather than silently accepted.
func TestDeleteColumnMoveToItselfIsValidationError(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	source := seedColumn(t, e.db, scope.CenterID, "source", 0, false)
	seedColumn(t, e.db, scope.CenterID, "dest", 1, false)
	seedTask(t, e.db, scope.CenterID, source, owner.ID, "t", seedTaskOpts{})

	_, err := e.svc.DeleteColumn(context.Background(), scope, source, &source)
	ae := appErr(t, err)
	require.Equal(t, apperror.CodeValidation, ae.Code)
	require.Contains(t, ae.Fields, "move_to")
}

// TestDeleteColumnMovesTasksAndStampsCompletedAt asserts a successful delete
// re-points every task into move_to, reports the moved count, publishes a
// tasks.ColumnDeleted event, and sets completed_at when (only when) the
// destination is a "done" column.
func TestDeleteColumnMovesTasksAndStampsCompletedAt(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	source := seedColumn(t, e.db, scope.CenterID, "source", 0, false)
	done := seedColumn(t, e.db, scope.CenterID, "done", 1, true)
	first := seedTask(t, e.db, scope.CenterID, source, owner.ID, "a", seedTaskOpts{})
	second := seedTask(t, e.db, scope.CenterID, source, owner.ID, "b", seedTaskOpts{})

	var published []tasks.ColumnDeleted
	e.bus.Subscribe("test", 0, func(_ context.Context, evt events.Event) {
		if cd, ok := evt.(tasks.ColumnDeleted); ok {
			published = append(published, cd)
		}
	})

	resp, err := e.svc.DeleteColumn(context.Background(), scope, source, &done)
	require.NoError(t, err)
	require.Equal(t, 2, resp.MovedCount)

	for _, taskID := range []uuid.UUID{first, second} {
		var row struct {
			ColumnID    uuid.UUID
			CompletedAt *string
		}
		require.NoError(t, e.db.Raw(
			"SELECT column_id, completed_at FROM tasks WHERE id = ?", taskID).Scan(&row).Error)
		require.Equal(t, done, row.ColumnID)
		require.NotNil(t, row.CompletedAt, "moving into a done column must stamp completed_at")
	}

	var colCount int64
	require.NoError(t, e.db.Raw("SELECT count(*) FROM task_columns WHERE id = ?", source).Scan(&colCount).Error)
	require.Zero(t, colCount, "the source column itself must be gone")

	require.Len(t, published, 1)
	require.Equal(t, source, published[0].ColumnID)
	require.NotNil(t, published[0].MoveTo)
	require.Equal(t, done, *published[0].MoveTo)
	require.Equal(t, 2, published[0].MovedCount)
}

// TestDeleteColumnMoveClearsCompletedAtForNonDoneDestination asserts moving
// tasks into a non-done column clears any prior completed_at instead of
// leaving a stale timestamp behind.
func TestDeleteColumnMoveClearsCompletedAtForNonDoneDestination(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	source := seedColumn(t, e.db, scope.CenterID, "source", 0, true)
	notDone := seedColumn(t, e.db, scope.CenterID, "not done", 1, false)
	taskID := seedTask(t, e.db, scope.CenterID, source, owner.ID, "a", seedTaskOpts{})
	require.NoError(t, e.db.Exec("UPDATE tasks SET completed_at = now() WHERE id = ?", taskID).Error)

	_, err := e.svc.DeleteColumn(context.Background(), scope, source, &notDone)
	require.NoError(t, err)

	var row struct{ CompletedAt *string }
	require.NoError(t, e.db.Raw("SELECT completed_at FROM tasks WHERE id = ?", taskID).Scan(&row).Error)
	require.Nil(t, row.CompletedAt)
}

// TestCreateColumnRejectsDuplicateNameCaseInsensitively asserts the
// uq_task_columns_name backstop is reachable through the real repository,
// not just theoretically correct.
func TestCreateColumnRejectsDuplicateNameCaseInsensitively(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	seedColumn(t, e.db, scope.CenterID, "Cần làm", 0, false)

	_, err := e.svc.CreateColumn(context.Background(), scope, tasks.CreateColumnRequest{Name: "cần làm"})
	require.Equal(t, apperror.CodeValidation, appErr(t, err).Code)
}

// TestCreateColumnEnforcesTenantCap asserts the ninth column on a center
// already holding the maximum is rejected — the CreateColumn use-case's
// own pre-check, backstopped by columnRepository.Create's advisory-locked
// re-check.
func TestCreateColumnEnforcesTenantCap(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	for i := 0; i < 8; i++ {
		seedColumn(t, e.db, scope.CenterID, uuid.NewString(), i, false)
	}

	_, err := e.svc.CreateColumn(context.Background(), scope, tasks.CreateColumnRequest{Name: "one too many"})
	require.Equal(t, apperror.CodeValidation, appErr(t, err).Code)
}
