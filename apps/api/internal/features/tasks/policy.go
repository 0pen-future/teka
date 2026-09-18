package tasks

import (
	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/pkg/kanban"
)

// actorFrom translates a caller's authctx.Scope into the kanban.Actor the
// core's DefaultPolicy reasons about. This package uses kanban.DefaultPolicy
// as-is (see service.go) rather than a bespoke kanban.Policy: the plan's
// object-level rules (owner/creator/assignee for read/write/move,
// owner-or-manage_board for the board) are exactly DefaultPolicy's four
// rules, so there is nothing left to override — everything else (whether
// the caller may hit an endpoint at all) is the routespec capability-key
// tier, enforced by middleware before a request ever reaches this package.
func actorFrom(sc authctx.Scope) kanban.Actor {
	return kanban.Actor{
		ID:      kanban.ActorID(sc.TeacherID),
		IsOwner: sc.IsOwner,
		Perms: map[string]bool{
			kanban.PermManageBoard: sc.Has(authctx.PermTasksManageBoard),
			kanban.PermViewAll:     sc.Has(authctx.PermTasksViewAll),
		},
	}
}
