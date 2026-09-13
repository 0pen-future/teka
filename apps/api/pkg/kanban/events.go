package kanban

// ColumnDeleted is published after DeleteColumn's UnitOfWork.Within returns
// nil. It is the only v1 event because it is the only fact a host's own
// audit middleware cannot already observe: MoveTo travels as a request body
// field, not a URL, so a request-log based audit trail never sees it. Other
// write operations are expected to be audited by the host from its own
// request/response layer.
type ColumnDeleted struct {
	ColumnID   ColumnID
	MoveTo     *ColumnID
	MovedCount int
}
