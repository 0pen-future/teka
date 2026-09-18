//go:build integration

package tasks_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/testutil"
)

// TestHandoverUnassignsAndReassignsLiveTasksOnly asserts
// Service.HandoverOnDeparture clears the departing member's assignments,
// moves their authored tasks' creatorship to the successor, and leaves
// soft-deleted tasks untouched either way.
func TestHandoverUnassignsAndReassignsLiveTasksOnly(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	_, departing := testutil.Teacher(t, e.db)
	testutil.JoinCenter(t, e.db, departing.ID, owner.CenterID)
	scope := testutil.ScopeFor(t, e.db, owner.ID)
	col := seedColumn(t, e.db, scope.CenterID, "To do", 0, false)

	assignedTask := seedTask(t, e.db, scope.CenterID, col, owner.ID, "assigned", seedTaskOpts{assignee: &departing.ID})
	createdTask := seedTask(t, e.db, scope.CenterID, col, departing.ID, "created", seedTaskOpts{})
	deletedAssigned := seedTask(t, e.db, scope.CenterID, col, owner.ID, "deleted-assigned", seedTaskOpts{assignee: &departing.ID, deletedAt: true})
	deletedCreated := seedTask(t, e.db, scope.CenterID, col, departing.ID, "deleted-created", seedTaskOpts{deletedAt: true})

	unassigned, reassigned, err := e.svc.HandoverOnDeparture(context.Background(), scope.CenterID, departing.ID, owner.ID)
	require.NoError(t, err)
	require.Equal(t, 1, unassigned)
	require.Equal(t, 1, reassigned)

	var assignedRow struct{ AssigneeID *uuid.UUID }
	require.NoError(t, e.db.Raw("SELECT assignee_id FROM tasks WHERE id = ?", assignedTask).Scan(&assignedRow).Error)
	require.Nil(t, assignedRow.AssigneeID)

	var createdRow struct{ CreatedBy uuid.UUID }
	require.NoError(t, e.db.Raw("SELECT created_by FROM tasks WHERE id = ?", createdTask).Scan(&createdRow).Error)
	require.Equal(t, owner.ID, createdRow.CreatedBy)

	var deletedAssignedRow struct{ AssigneeID *uuid.UUID }
	require.NoError(t, e.db.Raw("SELECT assignee_id FROM tasks WHERE id = ?", deletedAssigned).Scan(&deletedAssignedRow).Error)
	require.NotNil(t, deletedAssignedRow.AssigneeID, "a soft-deleted task's assignment must not be touched by a handover")
	require.Equal(t, departing.ID, *deletedAssignedRow.AssigneeID)

	var deletedCreatedRow struct{ CreatedBy uuid.UUID }
	require.NoError(t, e.db.Raw("SELECT created_by FROM tasks WHERE id = ?", deletedCreated).Scan(&deletedCreatedRow).Error)
	require.Equal(t, departing.ID, deletedCreatedRow.CreatedBy, "a soft-deleted task's creatorship must not be touched by a handover")
}

// TestHandoverIsNoOpWhenDepartingMemberHasNoTasks asserts a departure with
// nothing to hand over reports zero counts rather than erroring.
func TestHandoverIsNoOpWhenDepartingMemberHasNoTasks(t *testing.T) {
	t.Parallel()
	e := newTasksEnv(t)
	_, owner := testutil.Teacher(t, e.db)
	_, departing := testutil.Teacher(t, e.db)
	testutil.JoinCenter(t, e.db, departing.ID, owner.CenterID)
	scope := testutil.ScopeFor(t, e.db, owner.ID)

	unassigned, reassigned, err := e.svc.HandoverOnDeparture(context.Background(), scope.CenterID, departing.ID, owner.ID)
	require.NoError(t, err)
	require.Zero(t, unassigned)
	require.Zero(t, reassigned)
}
