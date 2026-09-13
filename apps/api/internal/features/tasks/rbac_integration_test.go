//go:build integration

package tasks_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/tasks"
	"teka/apps/api/internal/shared/apperror"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/testutil"
)

// TestOrdinaryMemberCannotManageBoardWithoutCapability asserts a live,
// role-less member cannot create, update, reorder, or delete columns unless
// explicitly granted tasks.manage_board — the default RBAC backfill
// deliberately withholds it (opt-in, high risk).
func TestOrdinaryMemberCannotManageBoardWithoutCapability(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	_, member := testutil.Teacher(t, e.db)
	testutil.JoinCenter(t, e.db, member.ID, owner.CenterID)
	ownerScope := testutil.ScopeFor(t, e.db, owner.ID)
	memberScope := testutil.ScopeFor(t, e.db, member.ID)

	col := seedColumn(t, e.db, ownerScope.CenterID, "only", 0, false)

	_, err := e.svc.CreateColumn(context.Background(), memberScope, tasks.CreateColumnRequest{Name: "new"})
	require.Equal(t, apperror.CodeForbidden, appErr(t, err).Code)

	name := "renamed"
	_, err = e.svc.UpdateColumn(context.Background(), memberScope, col, tasks.UpdateColumnRequest{Name: &name})
	require.Equal(t, apperror.CodeForbidden, appErr(t, err).Code)

	_, err = e.svc.ReorderColumns(context.Background(), memberScope, tasks.ReorderColumnsRequest{IDs: []uuid.UUID{col}})
	require.Equal(t, apperror.CodeForbidden, appErr(t, err).Code)

	seedColumn(t, e.db, ownerScope.CenterID, "spare", 1, false)
	_, err = e.svc.DeleteColumn(context.Background(), memberScope, col, nil)
	require.Equal(t, apperror.CodeForbidden, appErr(t, err).Code)
}

// TestMemberGrantedManageBoardCanManageColumns asserts the opt-in grant
// actually unlocks column management for a non-owner.
func TestMemberGrantedManageBoardCanManageColumns(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	_, member := testutil.Teacher(t, e.db)
	testutil.JoinCenter(t, e.db, member.ID, owner.CenterID)
	ownerScope := testutil.ScopeFor(t, e.db, owner.ID)
	grantPerm(t, e.db, member.ID, ownerScope.CenterID, authctx.PermTasksManageBoard)
	memberScope := testutil.ScopeFor(t, e.db, member.ID)

	_, err := e.svc.CreateColumn(context.Background(), memberScope, tasks.CreateColumnRequest{Name: "new"})
	require.NoError(t, err)
}

// TestCreatorCanEditAndDeleteOwnTask asserts CanWriteTask's creator branch.
func TestCreatorCanEditAndDeleteOwnTask(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	_, creator := testutil.Teacher(t, e.db)
	testutil.JoinCenter(t, e.db, creator.ID, owner.CenterID)
	ownerScope := testutil.ScopeFor(t, e.db, owner.ID)
	creatorScope := testutil.ScopeFor(t, e.db, creator.ID)
	col := seedColumn(t, e.db, ownerScope.CenterID, "To do", 0, false)
	taskID := seedTask(t, e.db, ownerScope.CenterID, col, creator.ID, "mine", seedTaskOpts{})

	title := "renamed"
	_, err := e.svc.UpdateTask(context.Background(), creatorScope, taskID, tasks.UpdateTaskRequest{Title: &title})
	require.NoError(t, err)

	require.NoError(t, e.svc.DeleteTask(context.Background(), creatorScope, taskID))
}

// TestNonCreatorNonOwnerAssigneeCannotEditOrDeleteButCanMove asserts
// CanWriteTask requires creator/owner (assignee alone is insufficient) while
// CanMoveTask accepts the assignee.
func TestNonCreatorNonOwnerAssigneeCannotEditOrDeleteButCanMove(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	_, creator := testutil.Teacher(t, e.db)
	_, assignee := testutil.Teacher(t, e.db)
	testutil.JoinCenter(t, e.db, creator.ID, owner.CenterID)
	testutil.JoinCenter(t, e.db, assignee.ID, owner.CenterID)
	ownerScope := testutil.ScopeFor(t, e.db, owner.ID)
	assigneeScope := testutil.ScopeFor(t, e.db, assignee.ID)
	col := seedColumn(t, e.db, ownerScope.CenterID, "a", 0, false)
	target := seedColumn(t, e.db, ownerScope.CenterID, "b", 1, false)
	taskID := seedTask(t, e.db, ownerScope.CenterID, col, creator.ID, "t", seedTaskOpts{assignee: &assignee.ID})

	title := "hijacked"
	_, err := e.svc.UpdateTask(context.Background(), assigneeScope, taskID, tasks.UpdateTaskRequest{Title: &title})
	require.Equal(t, apperror.CodeForbidden, appErr(t, err).Code)

	err = e.svc.DeleteTask(context.Background(), assigneeScope, taskID)
	require.Equal(t, apperror.CodeForbidden, appErr(t, err).Code)

	_, err = e.svc.MoveTask(context.Background(), assigneeScope, taskID, tasks.MoveTaskRequest{ColumnID: target})
	require.NoError(t, err, "an assignee who is neither creator nor owner may still move the task")
}

// TestUnrelatedMemberCannotReadTask asserts CanReadTask denies a member who
// is neither owner, tasks.view_all holder, creator, nor assignee.
func TestUnrelatedMemberCannotReadTask(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	_, creator := testutil.Teacher(t, e.db)
	_, bystander := testutil.Teacher(t, e.db)
	testutil.JoinCenter(t, e.db, creator.ID, owner.CenterID)
	testutil.JoinCenter(t, e.db, bystander.ID, owner.CenterID)
	ownerScope := testutil.ScopeFor(t, e.db, owner.ID)
	bystanderScope := testutil.ScopeFor(t, e.db, bystander.ID)
	col := seedColumn(t, e.db, ownerScope.CenterID, "a", 0, false)
	taskID := seedTask(t, e.db, ownerScope.CenterID, col, creator.ID, "t", seedTaskOpts{})

	_, err := e.svc.GetTask(context.Background(), bystanderScope, taskID)
	require.Equal(t, apperror.CodeForbidden, appErr(t, err).Code)
}

// TestBoardScopeDegradesToMineWithoutViewAll asserts an ordinary member sees
// only their own participant rows, while tasks.view_all widens them to the
// whole center — exercised end to end against the real repositories.
func TestBoardScopeDegradesToMineWithoutViewAll(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	_, member := testutil.Teacher(t, e.db)
	testutil.JoinCenter(t, e.db, member.ID, owner.CenterID)
	ownerScope := testutil.ScopeFor(t, e.db, owner.ID)
	col := seedColumn(t, e.db, ownerScope.CenterID, "a", 0, false)
	seedTask(t, e.db, ownerScope.CenterID, col, owner.ID, "owner's task", seedTaskOpts{})

	memberScope := testutil.ScopeFor(t, e.db, member.ID)
	resp, err := e.svc.Board(context.Background(), memberScope)
	require.NoError(t, err)
	require.Equal(t, "mine", resp.Scope)
	require.Empty(t, resp.Columns[0].Tasks)

	grantPerm(t, e.db, member.ID, ownerScope.CenterID, authctx.PermTasksViewAll)
	memberScope = testutil.ScopeFor(t, e.db, member.ID)
	resp, err = e.svc.Board(context.Background(), memberScope)
	require.NoError(t, err)
	require.Equal(t, "center", resp.Scope)
	require.Len(t, resp.Columns[0].Tasks, 1)
}
