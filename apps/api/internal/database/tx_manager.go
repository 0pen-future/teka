package database

import "context"

// TxManager runs a function inside a database transaction. Repositories read
// the transaction handle from ctx, so services stay storage-agnostic; see
// GormTxManager for the GORM-backed implementation.
type TxManager interface {
	// WithinTx begins a transaction, runs fn, and commits — or rolls back if
	// fn returns an error or panics.
	WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}
