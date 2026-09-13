package kanban

// Permission keys DefaultPolicy looks up in Actor.Perms. The core does not
// know or care what a host application calls these capabilities elsewhere;
// an adapter that uses different key names must translate into these, or
// implement its own Policy.
const (
	PermManageBoard = "tasks.manage_board"
	PermViewAll     = "tasks.view_all"
)

// DefaultPolicy implements the four object-level rules from the plan brief:
//
//   - manage board (create/rename/reorder/delete columns): owner OR
//     PermManageBoard.
//   - read a task: owner OR PermViewAll OR creator OR assignee.
//   - write (edit/delete) a task: owner OR creator. Being the assignee alone
//     is not enough.
//   - move a task between columns: owner OR creator OR assignee.
//
// This is the single place these rules are expressed; TaskRepository never
// re-derives a read rule from Actor, it only receives the resulting
// Visibility.
type DefaultPolicy struct{}

var _ Policy = DefaultPolicy{}

// CanManageBoard implements Policy.
func (DefaultPolicy) CanManageBoard(actor Actor, _ TenantID) bool {
	return actor.IsOwner || actor.Perms[PermManageBoard]
}

// CanReadTask implements Policy.
func (DefaultPolicy) CanReadTask(actor Actor, task Task) bool {
	if actor.IsOwner || actor.Perms[PermViewAll] {
		return true
	}
	return isCreator(actor, task) || isAssignee(actor, task)
}

// CanWriteTask implements Policy.
func (DefaultPolicy) CanWriteTask(actor Actor, task Task) bool {
	return actor.IsOwner || isCreator(actor, task)
}

// CanMoveTask implements Policy.
func (DefaultPolicy) CanMoveTask(actor Actor, task Task) bool {
	return actor.IsOwner || isCreator(actor, task) || isAssignee(actor, task)
}

// Visibility implements Policy.
func (DefaultPolicy) Visibility(actor Actor, _ TenantID) Visibility {
	if actor.IsOwner || actor.Perms[PermViewAll] {
		return Visibility{All: true}
	}
	return Visibility{Participant: actor.ID}
}

func isCreator(actor Actor, task Task) bool {
	return actor.ID == task.CreatedBy
}

func isAssignee(actor Actor, task Task) bool {
	return task.AssigneeID != nil && *task.AssigneeID == actor.ID
}
