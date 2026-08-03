package database

import "context"

// TxManager runs a function inside a database transaction. Repositories read
// the transaction handle from ctx, so services stay storage-agnostic. The
// GORM-backed implementation lands with the first multi-write feature.
type TxManager interface {
	// WithinTx begins a transaction, runs fn, and commits — or rolls back if
	// fn returns an error or panics.
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}
