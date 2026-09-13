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

// TestColumnFromAnotherCenterIsNotFound asserts a column id that resolves to
// a real row in a different tenant is indistinguishable from a missing one.
func TestColumnFromAnotherCenterIsNotFound(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, ownerA := testutil.Teacher(t, e.db)
	_, ownerB := testutil.Teacher(t, e.db)
	scopeA := testutil.ScopeFor(t, e.db, ownerA.ID)
	scopeB := testutil.ScopeFor(t, e.db, ownerB.ID)
	colInA := seedColumn(t, e.db, scopeA.CenterID, "A's column", 0, false)

	name := "renamed"
	_, err := e.svc.UpdateColumn(context.Background(), scopeB, colInA, tasks.UpdateColumnRequest{Name: &name})
	require.Equal(t, apperror.CodeNotFound, appErr(t, err).Code)
}

// TestTaskFromAnotherCenterIsNotFound mirrors the column case for tasks.
func TestTaskFromAnotherCenterIsNotFound(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, ownerA := testutil.Teacher(t, e.db)
	_, ownerB := testutil.Teacher(t, e.db)
	scopeA := testutil.ScopeFor(t, e.db, ownerA.ID)
	scopeB := testutil.ScopeFor(t, e.db, ownerB.ID)
	colInA := seedColumn(t, e.db, scopeA.CenterID, "A's column", 0, false)
	taskInA := seedTask(t, e.db, scopeA.CenterID, colInA, ownerA.ID, "A's task", seedTaskOpts{})

	_, err := e.svc.GetTask(context.Background(), scopeB, taskInA)
	require.Equal(t, apperror.CodeNotFound, appErr(t, err).Code)
}

// TestMemberCheckerOnlyRecognizesSameCenterLiveMembership asserts a teacher
// who belongs to a different center cannot be assigned a task, and a
// departed (left_at set) member of the same center cannot either.
func TestMemberCheckerOnlyRecognizesSameCenterLiveMembership(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	_, outsider := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	col := seedColumn(t, e.db, scope.CenterID, "To do", 0, false)

	outsiderID := outsider.ID
	_, err := e.svc.CreateTask(context.Background(), scope, tasks.CreateTaskRequest{
		Title: "x", ColumnID: &col, AssigneeID: &outsiderID,
	})
	ae := appErr(t, err)
	require.Equal(t, apperror.CodeValidation, ae.Code)
	require.Contains(t, ae.Fields, "assignee_id")

	// Join, then leave: a closed membership must not count as live either.
	testutil.JoinCenter(t, e.db, outsider.ID, scope.CenterID)
	require.NoError(t, e.db.Exec(
		"UPDATE center_members SET left_at = now() WHERE teacher_id = ? AND center_id = ? AND left_at IS NULL",
		outsider.ID, scope.CenterID).Error)

	_, err = e.svc.CreateTask(context.Background(), scope, tasks.CreateTaskRequest{
		Title: "y", ColumnID: &col, AssigneeID: &outsiderID,
	})
	ae = appErr(t, err)
	require.Equal(t, apperror.CodeValidation, ae.Code)
}

// TestCompositeForeignKeysRejectCrossTenantAssigneeAtTheDatabaseLayer proves
// translateDBError's fk_tasks_assignee_center branch is actually reachable:
// a raw insert naming an assignee who is a live member of a different
// center must be rejected by the database itself, not merely by the
// application-level memberChecker pre-check.
func TestCompositeForeignKeysRejectCrossTenantAssigneeAtTheDatabaseLayer(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, ownerA := testutil.Teacher(t, e.db)
	_, ownerB := testutil.Teacher(t, e.db)
	scopeA := testutil.ScopeFor(t, e.db, ownerA.ID)
	colInA := seedColumn(t, e.db, scopeA.CenterID, "A's column", 0, false)

	err := e.db.Exec(
		`INSERT INTO tasks (id, center_id, column_id, created_by, assignee_id, title, priority)
		 VALUES (gen_random_uuid(), ?, ?, ?, ?, 'cross-tenant assignee', 'none')`,
		scopeA.CenterID, colInA, ownerA.ID, ownerB.ID).Error
	require.Error(t, err, "the composite FK on (assignee_id, center_id) must reject an assignee from another center")
}
