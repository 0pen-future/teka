// Package centers exercises the exempt-path rules: the scope-resolution
// package legitimately builds authctx.Scope and OwnerAnchor by hand and its
// repository is exempt from R1's witness requirement.
package centers

import (
	"context"

	"gorm.io/gorm"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/authctx"
)

type gormRepository struct {
	db *gorm.DB
}

// ResolveScope touches a raw root with a Scope parameter and no witness —
// this would flag in package a, but centers is exempt from R1.
func (r *gormRepository) ResolveScope(ctx context.Context, sc authctx.Scope) error {
	return database.FromContext(ctx, r.db).Where("teacher_id = ?", sc.TeacherID).Find(&struct{}{}).Error
}

// GoodScopeLiteral builds a Scope by hand — this is centers' job, so it is
// exempt from the module-wide R3 literal ban.
func GoodScopeLiteral() authctx.Scope {
	return authctx.Scope{TeacherID: "x", IsOwner: true}
}

// GoodMintOwnerAnchor mints the proof — centers is the one legitimate
// caller.
func GoodMintOwnerAnchor() authctx.OwnerAnchor {
	return authctx.MintOwnerAnchor("x", "y")
}
