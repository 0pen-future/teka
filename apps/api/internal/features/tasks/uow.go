package tasks

import (
	"context"

	"teka/apps/api/internal/database"
	"teka/apps/api/pkg/kanban"
)

// unitOfWork adapts database.TxManager to kanban.UnitOfWork — same shape,
// forwarded 1-1.
type unitOfWork struct {
	tx database.TxManager
}

// newUnitOfWork builds the kanban.UnitOfWork backed by tx.
func newUnitOfWork(tx database.TxManager) *unitOfWork {
	return &unitOfWork{tx: tx}
}

var _ kanban.UnitOfWork = (*unitOfWork)(nil)

// Within implements kanban.UnitOfWork.
func (u *unitOfWork) Within(ctx context.Context, fn func(ctx context.Context) error) error {
	return u.tx.WithinTx(ctx, fn)
}
