//go:build integration

package tasks_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/tasks"
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/internal/testutil"
)

const (
	filterToday     = "2026-09-17"
	filterYesterday = "2026-09-16"
)

// taskIDs collects the IDs of a BoardColumnResponse's tasks for order-
// insensitive membership assertions.
func taskIDs(resp tasks.BoardResponse) []uuid.UUID {
	var ids []uuid.UUID
	for _, col := range resp.Columns {
		for _, tk := range col.Tasks {
			ids = append(ids, tk.ID)
		}
	}
	return ids
}

// TestBoardFilterOverdueOnlyReturnsOpenOverdueTasks asserts filter=overdue
// excludes a task already completed and a task not yet due, keeping only an
// open task whose due_on is strictly before the client's today.
func TestBoardFilterOverdueOnlyReturnsOpenOverdueTasks(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	col := seedColumn(t, e.db, scope.CenterID, "To do", 0, false)
	yesterday := filterYesterday
	today := filterToday
	overdueOpen := seedTask(t, e.db, scope.CenterID, col, owner.ID, "overdue open", seedTaskOpts{dueOn: &yesterday})
	seedTask(t, e.db, scope.CenterID, col, owner.ID, "overdue but done", seedTaskOpts{dueOn: &yesterday, completedAt: true})
	seedTask(t, e.db, scope.CenterID, col, owner.ID, "due today", seedTaskOpts{dueOn: &today})
	seedTask(t, e.db, scope.CenterID, col, owner.ID, "no due date", seedTaskOpts{})

	resp, err := e.svc.Board(context.Background(), scope, tasks.BoardQuery{Filter: "overdue", Today: filterToday})
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{overdueOpen}, taskIDs(resp))
}

// TestBoardFilterTodayOnlyReturnsOpenTasksDueToday mirrors the overdue case
// for filter=today: only the open task due exactly on today survives.
func TestBoardFilterTodayOnlyReturnsOpenTasksDueToday(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	col := seedColumn(t, e.db, scope.CenterID, "To do", 0, false)
	today := filterToday
	yesterday := filterYesterday
	dueToday := seedTask(t, e.db, scope.CenterID, col, owner.ID, "due today open", seedTaskOpts{dueOn: &today})
	seedTask(t, e.db, scope.CenterID, col, owner.ID, "due today but done", seedTaskOpts{dueOn: &today, completedAt: true})
	seedTask(t, e.db, scope.CenterID, col, owner.ID, "overdue", seedTaskOpts{dueOn: &yesterday})

	resp, err := e.svc.Board(context.Background(), scope, tasks.BoardQuery{Filter: "today", Today: filterToday})
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{dueToday}, taskIDs(resp))
}

// TestBoardFilterUnassignedOnlyReturnsOpenUnassignedTasks asserts
// filter=unassigned excludes both an assigned task and an already-completed
// unassigned task.
func TestBoardFilterUnassignedOnlyReturnsOpenUnassignedTasks(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	col := seedColumn(t, e.db, scope.CenterID, "To do", 0, false)
	unassignedOpen := seedTask(t, e.db, scope.CenterID, col, owner.ID, "unassigned open", seedTaskOpts{})
	seedTask(t, e.db, scope.CenterID, col, owner.ID, "unassigned done", seedTaskOpts{completedAt: true})
	seedTask(t, e.db, scope.CenterID, col, owner.ID, "assigned", seedTaskOpts{assignee: &owner.ID})

	resp, err := e.svc.Board(context.Background(), scope, tasks.BoardQuery{Filter: "unassigned"})
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{unassignedOpen}, taskIDs(resp))
}

// TestBoardFilterMineIncludesTheCallersCompletedTasks asserts filter=mine is
// the one filter that does not imply OpenOnly: the caller's own completed
// task still shows (e.g. in a "Hoàn thành" column), while another teacher's
// task never does.
func TestBoardFilterMineIncludesTheCallersCompletedTasks(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	_, other := testutil.Teacher(t, e.db)
	testutil.JoinCenter(t, e.db, other.ID, owner.CenterID)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	col := seedColumn(t, e.db, scope.CenterID, "To do", 0, false)
	mineOpen := seedTask(t, e.db, scope.CenterID, col, owner.ID, "mine open", seedTaskOpts{assignee: &owner.ID})
	mineDone := seedTask(t, e.db, scope.CenterID, col, owner.ID, "mine done", seedTaskOpts{assignee: &owner.ID, completedAt: true})
	seedTask(t, e.db, scope.CenterID, col, owner.ID, "someone else's", seedTaskOpts{assignee: &other.ID})

	resp, err := e.svc.Board(context.Background(), scope, tasks.BoardQuery{Filter: "mine"})
	require.NoError(t, err)
	require.ElementsMatch(t, []uuid.UUID{mineOpen, mineDone}, taskIDs(resp))
}

// TestBoardFilterAssigneeAndFilterCombineWithAnd asserts assignee=X ANDs
// onto whatever filter is already active rather than replacing it.
func TestBoardFilterAssigneeAndFilterCombineWithAnd(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	_, other := testutil.Teacher(t, e.db)
	testutil.JoinCenter(t, e.db, other.ID, owner.CenterID)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	col := seedColumn(t, e.db, scope.CenterID, "To do", 0, false)
	yesterday := filterYesterday
	wanted := seedTask(t, e.db, scope.CenterID, col, owner.ID, "overdue, owner's", seedTaskOpts{assignee: &owner.ID, dueOn: &yesterday})
	seedTask(t, e.db, scope.CenterID, col, owner.ID, "overdue, other's", seedTaskOpts{assignee: &other.ID, dueOn: &yesterday})
	seedTask(t, e.db, scope.CenterID, col, owner.ID, "not overdue, owner's", seedTaskOpts{assignee: &owner.ID})

	resp, err := e.svc.Board(context.Background(), scope, tasks.BoardQuery{
		Filter: "overdue", Assignee: owner.ID.String(), Today: filterToday,
	})
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{wanted}, taskIDs(resp))
}

// TestBoardCountsAreInvariantAcrossFiltersAndSortByAssignee asserts counts
// summarize the caller's entire visible open set regardless of the filter
// applied to the columns, that by_assignee sorts by count desc then teacher
// id, and that an assignee with zero open tasks never appears.
func TestBoardCountsAreInvariantAcrossFiltersAndSortByAssignee(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	_, busy := testutil.Teacher(t, e.db)
	_, quiet := testutil.Teacher(t, e.db)
	_, idle := testutil.Teacher(t, e.db)
	testutil.JoinCenter(t, e.db, busy.ID, owner.CenterID)
	testutil.JoinCenter(t, e.db, quiet.ID, owner.CenterID)
	testutil.JoinCenter(t, e.db, idle.ID, owner.CenterID)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	col := seedColumn(t, e.db, scope.CenterID, "To do", 0, false)
	yesterday := filterYesterday
	today := filterToday

	seedTask(t, e.db, scope.CenterID, col, owner.ID, "busy 1", seedTaskOpts{assignee: &busy.ID, dueOn: &yesterday})
	seedTask(t, e.db, scope.CenterID, col, owner.ID, "busy 2", seedTaskOpts{assignee: &busy.ID, dueOn: &today})
	seedTask(t, e.db, scope.CenterID, col, owner.ID, "quiet 1", seedTaskOpts{assignee: &quiet.ID})
	seedTask(t, e.db, scope.CenterID, col, owner.ID, "unassigned", seedTaskOpts{})
	seedTask(t, e.db, scope.CenterID, col, owner.ID, "idle's, but done", seedTaskOpts{assignee: &idle.ID, completedAt: true})

	var counts tasks.BoardCountsResponse
	for i, filter := range []string{"all", "mine", "overdue", "unassigned"} {
		resp, err := e.svc.Board(context.Background(), scope, tasks.BoardQuery{Filter: filter, Today: filterToday})
		require.NoError(t, err)
		if i == 0 {
			counts = resp.Counts
			continue
		}
		require.Equal(t, counts, resp.Counts, "counts must not vary with filter=%s", filter)
	}

	require.Equal(t, 4, counts.All, "idle's completed task must not count")
	require.Equal(t, 1, counts.Overdue)
	require.Equal(t, 1, counts.Unassigned)

	require.Len(t, counts.ByAssignee, 2, "idle has zero open tasks and must be omitted")
	require.Equal(t, busy.ID, counts.ByAssignee[0].TeacherID)
	require.Equal(t, 2, counts.ByAssignee[0].Count)
	require.Equal(t, quiet.ID, counts.ByAssignee[1].TeacherID)
	require.Equal(t, 1, counts.ByAssignee[1].Count)
}

// TestBoardCountsAllIgnoresThe50PerColumnCap asserts counts.all reflects the
// caller's full visible open set even when a single column's task list is
// capped by has_more.
func TestBoardCountsAllIgnoresThe50PerColumnCap(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	col := seedColumn(t, e.db, scope.CenterID, "To do", 0, false)
	for i := 0; i < 51; i++ {
		seedTask(t, e.db, scope.CenterID, col, owner.ID, "t", seedTaskOpts{position: float64(i)})
	}

	resp, err := e.svc.Board(context.Background(), scope, tasks.BoardQuery{})
	require.NoError(t, err)
	require.True(t, resp.Columns[0].HasMore)
	require.Len(t, resp.Columns[0].Tasks, 50)
	require.Equal(t, 51, resp.Counts.All)
}

// TestBoardCountsForNonViewAllMemberOnlyCoverOwnTasks asserts an ordinary
// member without tasks.view_all gets counts scoped to Visibility's degraded
// "mine" set, not the whole center — assignee=<someone else> must not leak
// another teacher's count.
func TestBoardCountsForNonViewAllMemberOnlyCoverOwnTasks(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	_, member := testutil.Teacher(t, e.db)
	testutil.JoinCenter(t, e.db, member.ID, owner.CenterID)
	ownerScope := testutil.ScopeFor(t, e.db, owner.ID)
	col := seedColumn(t, e.db, ownerScope.CenterID, "To do", 0, false)
	seedTask(t, e.db, ownerScope.CenterID, col, owner.ID, "owner's", seedTaskOpts{assignee: &owner.ID})
	memberTask := seedTask(t, e.db, ownerScope.CenterID, col, member.ID, "member's", seedTaskOpts{assignee: &member.ID})

	memberScope := testutil.ScopeFor(t, e.db, member.ID)
	resp, err := e.svc.Board(context.Background(), memberScope, tasks.BoardQuery{Assignee: owner.ID.String()})
	require.NoError(t, err)
	require.Equal(t, 1, resp.Counts.All, "counts stay scoped to the member's own visibility regardless of assignee=owner")
	require.Empty(t, taskIDs(resp), "assignee=owner ANDs onto a mine-degraded visibility that never includes owner's rows, so it matches nothing")

	resp, err = e.svc.Board(context.Background(), memberScope, tasks.BoardQuery{Assignee: member.ID.String()})
	require.NoError(t, err)
	require.Equal(t, []uuid.UUID{memberTask}, taskIDs(resp), "assignee=self still matches the member's own visible task")

	grantPerm(t, e.db, member.ID, ownerScope.CenterID, authctx.PermTasksViewAll)
	memberScope = testutil.ScopeFor(t, e.db, member.ID)
	resp, err = e.svc.Board(context.Background(), memberScope, tasks.BoardQuery{})
	require.NoError(t, err)
	require.Equal(t, 2, resp.Counts.All, "view_all widens counts to the whole center")
}
