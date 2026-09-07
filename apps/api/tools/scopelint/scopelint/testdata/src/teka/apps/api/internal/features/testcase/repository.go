package testcase

import (
	"context"

	"gorm.io/gorm"
	"teka/apps/api/internal/database"
	"teka/apps/api/internal/shared/authctx"
)

type gormRepository struct {
	db *gorm.DB
}

// readScoped is a root scoping helper: it calls database.FromContext (a raw
// root) and references sc.CenterID directly, so it passes R1 on its own
// account.
func (r *gormRepository) readScoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
	return database.FromContext(ctx, r.db).Where("center_id = ?", sc.CenterID)
}

// centerScoped is a narrowing helper: it takes an existing *gorm.DB and
// never touches a raw root itself, so R1 does not apply to it at all.
func (r *gormRepository) centerScoped(q *gorm.DB, sc authctx.Scope, col string) *gorm.DB {
	return q.Where(col+" = ?", sc.CenterID)
}

// GoodHelper witnesses R1 through a call to a shape-matched root helper.
func (r *gormRepository) GoodHelper(ctx context.Context, sc authctx.Scope) error {
	return r.readScoped(ctx, sc).Find(&struct{}{}).Error
}

// GoodDirectCenterID witnesses R1 through a direct reference to its scope
// parameter's CenterID field, with no helper call at all.
func (r *gormRepository) GoodDirectCenterID(ctx context.Context, sc authctx.Scope) error {
	return database.FromContext(ctx, r.db).Where("center_id = ?", sc.CenterID).Find(&struct{}{}).Error
}

// GoodNarrowing witnesses R1 through a call to the narrowing helper
// centerScoped, which itself takes the already-built *gorm.DB.
func (r *gormRepository) GoodNarrowing(ctx context.Context, sc authctx.Scope) error {
	q := database.FromContext(ctx, r.db).Table("items")
	q = r.centerScoped(q, sc, "center_id")
	return q.Find(&struct{}{}).Error
}

// GoodRawSQL witnesses R1 through a direct reference to its Anchor
// parameter's CenterID field inside a raw SQL string's arguments.
func (r *gormRepository) GoodRawSQL(ctx context.Context, a authctx.Anchor) error {
	return database.FromContext(ctx, r.db).Raw("SELECT * FROM items WHERE center_id = ?", a.CenterID).Find(&struct{}{}).Error
}

//scopelint:unscoped test fixture exercising the directive escape hatch
func (r *gormRepository) GoodDirective(ctx context.Context, sc authctx.Scope) error {
	return database.FromContext(ctx, r.db).Where("id = ?", sc.TeacherID).Find(&struct{}{}).Error
}

// GoodNoScopeParam has no Scope/Anchor/OwnerAnchor parameter, so it sits
// outside R1 by design regardless of the raw root it touches.
func (r *gormRepository) GoodNoScopeParam(ctx context.Context, id string) error {
	return database.FromContext(ctx, r.db).Where("id = ?", id).Find(&struct{}{}).Error
}

// BadNoScope touches a raw root, takes a Scope parameter, and neither calls
// a helper nor references CenterID.
func (r *gormRepository) BadNoScope(ctx context.Context, sc authctx.Scope, id string) error { // want `BadNoScope builds a query from a raw \*gorm\.DB root`
	return database.FromContext(ctx, r.db).Where("id = ?", id).Find(&struct{}{}).Error
}

// BadTeacherOnly references its scope parameter but never its CenterID
// field, so the raw root is still unwitnessed.
func (r *gormRepository) BadTeacherOnly(ctx context.Context, sc authctx.Scope) error { // want `BadTeacherOnly builds a query from a raw \*gorm\.DB root`
	return database.FromContext(ctx, r.db).Where("teacher_id = ?", sc.TeacherID).Find(&struct{}{}).Error
}

// badRootHelper matches the helper shape (unexported, returns *gorm.DB, has
// a Scope parameter) but never references CenterID itself, so R1 flags it
// on its own account even though other methods may call it as a witness.
func (r *gormRepository) badRootHelper(ctx context.Context, sc authctx.Scope) *gorm.DB { // want `badRootHelper builds a query from a raw \*gorm\.DB root`
	return database.FromContext(ctx, r.db).Where("teacher_id = ?", sc.TeacherID)
}

//scopelint:unscoped
func (r *gormRepository) BadBareDirective(ctx context.Context, sc authctx.Scope) error { // want `directive on BadBareDirective needs a one-sentence reason` `BadBareDirective builds a query from a raw \*gorm\.DB root`
	return database.FromContext(ctx, r.db).Where("id = ?", sc.TeacherID).Find(&struct{}{}).Error
}

// BadReset resets whatever scoping the chain already applied even though it
// also references CenterID (R1 passes, R2 fails).
func (r *gormRepository) BadReset(ctx context.Context, sc authctx.Scope) error {
	return database.FromContext(ctx, r.db).Session(&gorm.Session{NewDB: true}).Where("id = ?", sc.CenterID).Find(&struct{}{}).Error // want "Session\\(&gorm\\.Session\\{NewDB: true\\}\\) discards scoping"
}

// BadUnscoped removes scoping already applied to the chain.
func (r *gormRepository) BadUnscoped(ctx context.Context, sc authctx.Scope) error {
	return database.FromContext(ctx, r.db).Unscoped().Where("id = ?", sc.CenterID).Find(&struct{}{}).Error // want "Unscoped resets scoping"
}

// GoodTable selects a table — not a reset — and passes both R1 and R2.
func (r *gormRepository) GoodTable(ctx context.Context, sc authctx.Scope) error {
	return database.FromContext(ctx, r.db).Table("items").Where("center_id = ?", sc.CenterID).Find(&struct{}{}).Error
}

// BadIsOwner reads the authority field directly instead of going through
// CenterWideFor.
func (r *gormRepository) BadIsOwner(sc authctx.Scope) bool {
	return sc.IsOwner // want "authority fields are resolved by services, not read in repositories"
}

// BadHas calls the permission-set check directly instead of scoping through
// CenterWideFor.
func (r *gormRepository) BadHas(sc authctx.Scope) bool {
	return sc.Has("x") // want "Scope.Has is forbidden in repositories"
}

// badWrite's name does not say "read", so CenterWideFor is forbidden here —
// a write widens through WriteWide, never a visibility key.
func (r *gormRepository) badWrite(sc authctx.Scope) bool {
	return sc.CenterWideFor("x") // want "CenterWideFor may only widen a read-named function"
}

// readSomething's name says "read", so CenterWideFor is allowed.
func (r *gormRepository) readSomething(sc authctx.Scope) bool {
	return sc.CenterWideFor("x")
}

// BadStaffRoles consults the capability-to-role map directly, forking
// write authorization away from services.
func (r *gormRepository) BadStaffRoles(sc authctx.Scope) []string {
	return authctx.StaffRolesFor("x") // want "StaffRolesFor is forbidden in repositories"
}

// BadStaffRoleCan consults the capability-to-role map directly, forking
// write authorization away from services.
func (r *gormRepository) BadStaffRoleCan(sc authctx.Scope) bool {
	return authctx.StaffRoleCan("x", "y") // want "StaffRoleCan is forbidden in repositories"
}

// alreadyClosed proves the read-name rule matches whole camelCase tokens,
// not substrings: "already" contains "read" as a substring but has no
// "read" token, so CenterWideFor is forbidden here exactly as in badWrite.
func (r *gormRepository) alreadyClosed(sc authctx.Scope) bool {
	return sc.CenterWideFor("x") // want "CenterWideFor may only widen a read-named function"
}

// GoodCopiedScope witnesses R1 through a selector on a copy of its scope
// parameter, not the parameter itself — s := sc; s.CenterID.
func (r *gormRepository) GoodCopiedScope(ctx context.Context, sc authctx.Scope) error {
	s := sc
	return database.FromContext(ctx, r.db).Where("center_id = ?", s.CenterID).Find(&struct{}{}).Error
}

// GoodEmbeddedAnchor witnesses R1 through a selector reaching CenterID via
// OwnerAnchor's embedded Anchor field — oa.Anchor.CenterID.
func (r *gormRepository) GoodEmbeddedAnchor(ctx context.Context, oa authctx.OwnerAnchor) error {
	return database.FromContext(ctx, r.db).Where("center_id = ?", oa.Anchor.CenterID).Find(&struct{}{}).Error
}

// otherRepo exists only to prove isHelperFunc requires the same receiver
// type as the method being checked: its unexported, *gorm.DB-returning,
// Scope-parameter method matches the helper shape but not the receiver.
type otherRepo struct {
	db *gorm.DB
}

func (o *otherRepo) unrelatedScoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
	return database.FromContext(ctx, o.db).Where("center_id = ?", sc.CenterID)
}

// BadUnrelatedHelper calls a helper matching the scoping-helper shape but
// declared on a different receiver type, so it must not count as a witness.
func (r *gormRepository) BadUnrelatedHelper(ctx context.Context, sc authctx.Scope) error { // want `BadUnrelatedHelper builds a query from a raw \*gorm\.DB root`
	o := &otherRepo{db: r.db}
	_ = o.unrelatedScoped(ctx, sc)
	return database.FromContext(ctx, r.db).Where("id = ?", sc.TeacherID).Find(&struct{}{}).Error
}
