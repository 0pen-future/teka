// Package database is a minimal stand-in for teka/apps/api/internal/database
// (the real home of FromContext — not internal/shared/database), stubbed at
// its real import path so the analyzer's type-based checks resolve against
// it exactly as they do on the real package.
package database

import (
	"context"

	"gorm.io/gorm"
)

// FromContext mirrors the real FromContext: it returns the *gorm.DB bound to
// ctx, or fallback if none is bound. A call to it is a repository's raw
// query root.
func FromContext(ctx context.Context, fallback *gorm.DB) *gorm.DB {
	return fallback
}
