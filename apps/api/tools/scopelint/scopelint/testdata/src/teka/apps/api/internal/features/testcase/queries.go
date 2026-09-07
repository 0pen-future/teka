package testcase

import (
	"context"

	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/authctx"
)

// BadSiblingFile proves the repository file set is not limited to the file
// that declares the receiver type: this file declares no type of its own,
// only a method on gormRepository (declared in repository.go), and R1 still
// applies to it.
func (r *gormRepository) BadSiblingFile(ctx context.Context, sc authctx.Scope) error { // want `BadSiblingFile builds a query from a raw \*gorm\.DB root`
	return database.FromContext(ctx, r.db).Where("id = ?", sc.TeacherID).Find(&struct{}{}).Error
}

// BadFreeFunction proves the repository file set also runs R1 and R3 on
// receiverless functions declared alongside the repository's methods: it
// reads sc.IsOwner directly and touches a raw root with no witness.
func BadFreeFunction(ctx context.Context, sc authctx.Scope) error { // want `BadFreeFunction builds a query from a raw \*gorm\.DB root`
	if sc.IsOwner { // want "authority fields are resolved by services, not read in repositories"
		return nil
	}
	return database.FromContext(ctx, nil).Where("id = ?", sc.TeacherID).Find(&struct{}{}).Error
}
