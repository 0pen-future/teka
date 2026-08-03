package database

import (
	"context"

	"gorm.io/gorm"
)

type txKey struct{}

// GormTxManager implements TxManager with GORM transactions carried in ctx.
type GormTxManager struct {
	db *gorm.DB
}

// NewTxManager wraps db in a TxManager.
func NewTxManager(db *gorm.DB) *GormTxManager {
	return &GormTxManager{db: db}
}

// WithinTx runs fn inside a transaction, committing on nil and rolling back
// on error or panic. Nested calls join the ambient transaction.
func (m *GormTxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok && tx != nil {
		return fn(ctx)
	}
	return m.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(context.WithValue(ctx, txKey{}, tx))
	})
}

// FromContext returns the ambient transaction handle, or fallback when the
// call is not inside WithinTx. Repositories use this so the same methods work
// inside and outside transactions.
func FromContext(ctx context.Context, fallback *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok && tx != nil {
		return tx
	}
	return fallback.WithContext(ctx)
}
