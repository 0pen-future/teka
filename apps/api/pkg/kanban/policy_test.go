package kanban

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

// TestDefaultPolicyTaskMatrix checks the three object-level task rules
// against every actor role the plan brief distinguishes: owner, creator,
// assignee, and an unrelated stranger. A viewer with PermViewAll but none of
// those relationships is checked separately (read-only elevation).
func TestDefaultPolicyTaskMatrix(t *testing.T) {
	t.Parallel()
	creator := ActorID(uuid.New())
	assignee := ActorID(uuid.New())
	owner := ActorID(uuid.New())
	stranger := ActorID(uuid.New())
	task := Task{CreatedBy: creator, AssigneeID: &assignee}

	actors := map[string]Actor{
		"owner":    {ID: owner, IsOwner: true},
		"creator":  {ID: creator},
		"assignee": {ID: assignee},
		"stranger": {ID: stranger},
	}
	wantRead := map[string]bool{"owner": true, "creator": true, "assignee": true, "stranger": false}
	wantWrite := map[string]bool{"owner": true, "creator": true, "assignee": false, "stranger": false}
	wantMove := map[string]bool{"owner": true, "creator": true, "assignee": true, "stranger": false}

	pol := DefaultPolicy{}
	for name, actor := range actors {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, wantRead[name], pol.CanReadTask(actor, task), "read")
			require.Equal(t, wantWrite[name], pol.CanWriteTask(actor, task), "write")
			require.Equal(t, wantMove[name], pol.CanMoveTask(actor, task), "move")
		})
	}
}

func TestDefaultPolicyViewAllReadsWithoutRelationship(t *testing.T) {
	t.Parallel()
	task := Task{CreatedBy: ActorID(uuid.New())}
	viewer := Actor{ID: ActorID(uuid.New()), Perms: map[string]bool{PermViewAll: true}}
	require.True(t, DefaultPolicy{}.CanReadTask(viewer, task))
	require.False(t, DefaultPolicy{}.CanWriteTask(viewer, task), "view_all does not imply write")
	require.False(t, DefaultPolicy{}.CanMoveTask(viewer, task), "view_all does not imply move")
}

func TestDefaultPolicyManageBoard(t *testing.T) {
	t.Parallel()
	tenant := TenantID(uuid.New())
	pol := DefaultPolicy{}
	require.True(t, pol.CanManageBoard(Actor{IsOwner: true}, tenant))
	require.True(t, pol.CanManageBoard(Actor{Perms: map[string]bool{PermManageBoard: true}}, tenant))
	require.False(t, pol.CanManageBoard(Actor{}, tenant))
	require.False(t, pol.CanManageBoard(Actor{Perms: map[string]bool{}}, tenant))
}

func TestDefaultPolicyVisibility(t *testing.T) {
	t.Parallel()
	tenant := TenantID(uuid.New())
	pol := DefaultPolicy{}

	require.True(t, pol.Visibility(Actor{IsOwner: true}, tenant).All)
	require.True(t, pol.Visibility(Actor{Perms: map[string]bool{PermViewAll: true}}, tenant).All)

	actor := Actor{ID: ActorID(uuid.New())}
	vis := pol.Visibility(actor, tenant)
	require.False(t, vis.All)
	require.Equal(t, actor.ID, vis.Participant)
}
