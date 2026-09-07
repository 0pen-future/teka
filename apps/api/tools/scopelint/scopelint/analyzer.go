// Package scopelint enforces tenancy scoping invariants for repository
// methods: a method that accepts a caller's authctx.Scope, Anchor or
// OwnerAnchor and touches the database must visibly bind that scope, must
// never discard scoping already applied to a query, and must resolve
// authority through authctx rather than branch on it directly.
package scopelint

import (
	"go/ast"
	"go/types"
	"path"
	"strings"
	"sync/atomic"
	"unicode"

	"golang.org/x/tools/go/analysis"
)

const (
	authctxPkgPath  = "teka/apps/api/internal/shared/authctx"
	databasePkgPath = "teka/apps/api/internal/database"
	gormPkgPath     = "gorm.io/gorm"
	directivePrefix = "//scopelint:unscoped"
)

// Analyzer is the scopelint go/analysis pass.
var Analyzer = &analysis.Analyzer{
	Name: "scopelint",
	Doc: `scopelint enforces tenancy scoping invariants for repository methods.

R1 "scope witness": in any package under internal/features, the repository
receiver is identified by type shape, not by filename — a named struct type
declaring a field of type *gorm.DB (excluding the type declared in the
centers package, the scope-resolution package itself). Every function in
that package's non-test files — a method on the repository receiver, or a
receiverless function declared alongside one — that accepts an authctx.Scope,
Anchor or OwnerAnchor parameter and touches a raw *gorm.DB root (a call to
database.FromContext, or a read of a *gorm.DB-typed receiver field) must
contain a witness: a call to a shape-matched scoping helper (an unexported
method on that same repository receiver type, returning *gorm.DB, with at
least one Scope/Anchor/OwnerAnchor parameter), or a selector expression
reading the CenterID field of any expression (one pointer level unwrapped)
typed authctx.Scope, Anchor or OwnerAnchor — a parameter, a copy of one, or a
field reached through an embedded struct. A witness proves the scope was
mentioned somewhere in the method, not that every query chain in it applied
it; dataflow through a discarded result or an unused reference is an explicit
non-goal, and row-level center_id/teacher_id predicates remain the backstop
for that residue. A method with no scope-typed parameter is out of scope by
design: it builds a struct the caller already scoped, or looks up a token/id
the caller already gated. A method may opt out with a directive comment
directly above it, "//scopelint:unscoped <reason>"; a directive with no
reason is itself flagged.

R2 "no reset": .Session(&gorm.Session{NewDB: true}) as a literal in the call
arguments, and .Unscoped(), on a *gorm.DB discard whatever scoping the caller
already applied to the chain, and are forbidden in any non-test file. Binding
the gorm.Session value to a variable first is not recognized — the spec
targets the literal form. .Table( is table selection, not a reset, and is
allowed.

R3 "authority stays in authctx": within the R1 file set, reading
Scope.IsOwner, calling Scope.Has, calling Scope.CenterWideFor outside a
function whose name contains a "read" token (split on camelCase and
underscore boundaries, so "alreadyClosed" does not qualify), or calling
StaffRolesFor/StaffRoleCan forks authorization out of authctx and into a
repository. Across every non-test file (excluding packages named centers,
middleware, testutil, authctx or seeds, which legitimately resolve or
construct scope): building an authctx.Scope composite literal with fields,
assigning to a field of a Scope-typed value (one pointer level unwrapped),
building an OwnerAnchor literal, or calling MintOwnerAnchor forges authority
by hand instead of resolving it.`,
	Run: run,
}

// repositoryPackageCount counts, across the analyzer's most recent run
// (possibly parallel across packages), how many packages had a repository
// receiver identified. tree_test.go asserts a floor on it so that analyzing
// nothing looks nothing like analyzing everything cleanly (a renamed or split
// repository file must never silently disable the linter).
var repositoryPackageCount atomic.Int64

// ResetRepositoryPackages clears the counter before a fresh run.
func ResetRepositoryPackages() { repositoryPackageCount.Store(0) }

// RepositoryPackages reports the counter's current value.
func RepositoryPackages() int { return int(repositoryPackageCount.Load()) }

func run(pass *analysis.Pass) (any, error) {
	pkgBase := path.Base(pass.Pkg.Path())
	literalExempt := isExemptBase(pkgBase)

	repoRecv := findRepositoryReceiver(pass)
	if repoRecv != nil {
		repositoryPackageCount.Add(1)
	}
	var repoFiles map[*ast.File]bool
	if repoRecv != nil && pkgBase != "centers" {
		repoFiles = collectRepoFiles(pass, repoRecv)
	}

	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}
		checkNoReset(pass, file)
		if !literalExempt {
			checkLiteralConstruction(pass, file)
		}
		if !repoFiles[file] {
			continue
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			checkRepoAuthority(pass, fn)
			checkWitness(pass, fn, repoRecv)
		}
	}
	return nil, nil
}

func isExemptBase(base string) bool {
	switch base {
	case "centers", "middleware", "testutil", "authctx", "seeds":
		return true
	}
	return false
}

// isFeaturesPkg reports whether pkgPath sits under an internal/features tree
// — the only place R1 and the repository-scoped part of R3 apply.
func isFeaturesPkg(pkgPath string) bool {
	return strings.Contains(pkgPath, "/internal/features/") || strings.HasSuffix(pkgPath, "/internal/features")
}

// findRepositoryReceiver identifies the repository receiver by type shape,
// not by filename: the named struct type, declared at package scope in a
// package under internal/features, that has a field of type *gorm.DB. It
// returns nil if the package is not under internal/features or declares no
// such type. Package-scope names are visited in sorted order so the result
// is deterministic if more than one type happens to match.
func findRepositoryReceiver(pass *analysis.Pass) *types.TypeName {
	if !isFeaturesPkg(pass.Pkg.Path()) {
		return nil
	}
	scope := pass.Pkg.Scope()
	for _, name := range scope.Names() {
		tn, ok := scope.Lookup(name).(*types.TypeName)
		if !ok {
			continue
		}
		named, ok := tn.Type().(*types.Named)
		if !ok {
			continue
		}
		st, ok := named.Underlying().(*types.Struct)
		if !ok {
			continue
		}
		for i := 0; i < st.NumFields(); i++ {
			if isGormDBPtr(st.Field(i).Type()) {
				return tn
			}
		}
	}
	return nil
}

// collectRepoFiles returns every non-test file in the package that either
// declares repoRecv itself or declares at least one method on it — the file
// set R1 and the repository-scoped part of R3 analyse. A file with no method
// on repoRecv but sitting alongside one (a free-function helper file) is not
// included by this alone; the rules apply to receiverless functions inside an
// included file, but that does not change which files are included.
func collectRepoFiles(pass *analysis.Pass, repoRecv *types.TypeName) map[*ast.File]bool {
	files := map[*ast.File]bool{}
	declPos := repoRecv.Pos()
	for _, file := range pass.Files {
		filename := pass.Fset.Position(file.Pos()).Filename
		if strings.HasSuffix(filename, "_test.go") {
			continue
		}
		if file.Pos() <= declPos && declPos <= file.End() {
			files[file] = true
			continue
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil {
				continue
			}
			fnObj, _ := pass.TypesInfo.Defs[fn.Name].(*types.Func)
			if fnObj == nil {
				continue
			}
			if recvTypeName(fnObj) == repoRecv {
				files[file] = true
				break
			}
		}
	}
	return files
}

// recvTypeName returns the named type a method's receiver is declared on,
// unwrapping one level of pointer.
func recvTypeName(fn *types.Func) *types.TypeName {
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return nil
	}
	t := sig.Recv().Type()
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	named, ok := t.(*types.Named)
	if !ok {
		return nil
	}
	return named.Obj()
}

// isGormDBPtr reports whether t is *gorm.DB.
func isGormDBPtr(t types.Type) bool {
	if t == nil {
		return false
	}
	p, ok := t.(*types.Pointer)
	if !ok {
		return false
	}
	return isNamedType(p.Elem(), gormPkgPath, "DB")
}

// unwrapPtr strips one level of pointer indirection from t, if present.
func unwrapPtr(t types.Type) types.Type {
	if p, ok := t.(*types.Pointer); ok {
		return p.Elem()
	}
	return t
}

// isNamedType reports whether t is the named type pkgPath.name.
func isNamedType(t types.Type, pkgPath, name string) bool {
	named, ok := t.(*types.Named)
	if !ok {
		return false
	}
	obj := named.Obj()
	return obj.Pkg() != nil && obj.Pkg().Path() == pkgPath && obj.Name() == name
}

// scopeKindOf returns "Scope", "Anchor" or "OwnerAnchor" if t is one of
// authctx's three scope-carrying types, else "".
func scopeKindOf(t types.Type) string {
	named, ok := t.(*types.Named)
	if !ok {
		return ""
	}
	obj := named.Obj()
	if obj.Pkg() == nil || obj.Pkg().Path() != authctxPkgPath {
		return ""
	}
	switch obj.Name() {
	case "Scope", "Anchor", "OwnerAnchor":
		return obj.Name()
	}
	return ""
}

// isHelperFunc reports whether fn matches the scoping-helper shape: an
// unexported method on expectRecv — the same receiver type as the method
// being checked — returning a single *gorm.DB with at least one
// Scope/Anchor/OwnerAnchor parameter.
func isHelperFunc(fn *types.Func, expectRecv *types.TypeName) bool {
	if fn == nil || ast.IsExported(fn.Name()) {
		return false
	}
	if expectRecv == nil || recvTypeName(fn) != expectRecv {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
		return false
	}
	if sig.Results().Len() != 1 || !isGormDBPtr(sig.Results().At(0).Type()) {
		return false
	}
	for i := 0; i < sig.Params().Len(); i++ {
		if scopeKindOf(sig.Params().At(i).Type()) != "" {
			return true
		}
	}
	return false
}

// isPkgFunc reports whether fn is the free function pkgPath.name (not a
// method).
func isPkgFunc(fn *types.Func, pkgPath, name string) bool {
	if fn == nil || fn.Name() != name {
		return false
	}
	sig, ok := fn.Type().(*types.Signature)
	if !ok || sig.Recv() != nil {
		return false
	}
	return fn.Pkg() != nil && fn.Pkg().Path() == pkgPath
}

// isMethodOn reports whether fn is method name declared on pkgPath.typeName.
func isMethodOn(fn *types.Func, pkgPath, typeName, name string) bool {
	if fn == nil || fn.Name() != name {
		return false
	}
	recv := recvTypeName(fn)
	if recv == nil {
		return false
	}
	return recv.Pkg() != nil && recv.Pkg().Path() == pkgPath && recv.Name() == typeName
}

// resolveSelObj resolves the object a selector expression refers to,
// covering both value selections (method/field access, recorded in
// TypesInfo.Selections) and package-qualified identifiers (recorded in
// TypesInfo.Uses).
func resolveSelObj(pass *analysis.Pass, sel *ast.SelectorExpr) types.Object {
	if info, ok := pass.TypesInfo.Selections[sel]; ok {
		return info.Obj()
	}
	return pass.TypesInfo.Uses[sel.Sel]
}

// directiveReason returns the reason text of a //scopelint:unscoped
// directive in fn's doc comment, and whether the directive is present at
// all. An empty reason with present=true means the directive is malformed.
func directiveReason(fn *ast.FuncDecl) (reason string, present bool) {
	if fn.Doc == nil {
		return "", false
	}
	for _, c := range fn.Doc.List {
		text := strings.TrimSpace(c.Text)
		if !strings.HasPrefix(text, directivePrefix) {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(text, directivePrefix)), true
	}
	return "", false
}

// checkWitness implements R1 for a single function in the repository file
// set — a method on repoRecv, or a receiverless function declared alongside
// one.
func checkWitness(pass *analysis.Pass, fn *ast.FuncDecl, repoRecv *types.TypeName) {
	hasScopeParam := false
	if fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			if scopeKindOf(pass.TypesInfo.TypeOf(field.Type)) != "" {
				hasScopeParam = true
				break
			}
		}
	}
	if !hasScopeParam {
		return
	}

	rawRoot, witness := walkForWitness(pass, fn.Body, repoRecv)

	reason, hasDirective := directiveReason(fn)
	if hasDirective && reason == "" {
		pass.Reportf(fn.Pos(), "//scopelint:unscoped directive on %s needs a one-sentence reason", fn.Name.Name)
	}
	if hasDirective && reason != "" {
		return
	}
	if !rawRoot || witness {
		return
	}
	pass.Reportf(fn.Pos(),
		"%s builds a query from a raw *gorm.DB root without calling a scoping helper or referencing its scope parameter's CenterID field; scope it, or add //scopelint:unscoped <reason>",
		fn.Name.Name)
}

// walkForWitness reports whether body touches a raw *gorm.DB root (a call to
// database.FromContext, or a read of a *gorm.DB receiver field) and whether
// it contains a witness: a call to a helper shape-matched against repoRecv,
// or a selector reading CenterID off any expression whose type — one
// pointer level unwrapped — is authctx.Scope, Anchor or OwnerAnchor. The
// witness does not have to name the function's own scope parameter: a copy
// (s := sc; s.CenterID) and an embedded field (oa.Anchor.CenterID) both
// count: a witness proves the scope was mentioned, not that dataflow reached
// the query (see the package doc).
func walkForWitness(pass *analysis.Pass, body ast.Node, repoRecv *types.TypeName) (rawRoot, witness bool) {
	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			sel, ok := node.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if fnObj, ok := resolveSelObj(pass, sel).(*types.Func); ok {
				if isPkgFunc(fnObj, databasePkgPath, "FromContext") {
					rawRoot = true
				}
				if isHelperFunc(fnObj, repoRecv) {
					witness = true
				}
			}
		case *ast.SelectorExpr:
			if info, ok := pass.TypesInfo.Selections[node]; ok && info.Kind() == types.FieldVal {
				if isGormDBPtr(info.Type()) {
					rawRoot = true
				}
			}
			if node.Sel.Name == "CenterID" && scopeKindOf(unwrapPtr(pass.TypesInfo.TypeOf(node.X))) != "" {
				witness = true
			}
		}
		return true
	})
	return
}

// hasReadToken reports whether name, split on camelCase and underscore
// boundaries, contains a token equal to "read" (case-insensitive) — the
// read-name rule for CenterWideFor. A substring match would wrongly accept
// "alreadyClosed" or "spreadRows"; a token match does not.
func hasReadToken(name string) bool {
	for _, tok := range splitCamelTokens(name) {
		if strings.EqualFold(tok, "read") {
			return true
		}
	}
	return false
}

// splitCamelTokens splits name into tokens at underscore boundaries and at
// each lowercase-to-uppercase transition (readScoped -> read, Scoped;
// GetPeriodRead -> Get, Period, Read).
func splitCamelTokens(name string) []string {
	var tokens []string
	var cur []rune
	for _, r := range name {
		if r == '_' {
			if len(cur) > 0 {
				tokens = append(tokens, string(cur))
				cur = nil
			}
			continue
		}
		if len(cur) > 0 && unicode.IsUpper(r) && !unicode.IsUpper(cur[len(cur)-1]) {
			tokens = append(tokens, string(cur))
			cur = nil
		}
		cur = append(cur, r)
	}
	if len(cur) > 0 {
		tokens = append(tokens, string(cur))
	}
	return tokens
}

// checkRepoAuthority implements the repository-scoped part of R3 for a
// single function declared in the repository file set — a method or a
// receiverless function.
func checkRepoAuthority(pass *analysis.Pass, fn *ast.FuncDecl) {
	isRead := hasReadToken(fn.Name.Name)
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.SelectorExpr:
			if node.Sel.Name == "IsOwner" && scopeKindOf(pass.TypesInfo.TypeOf(node.X)) == "Scope" {
				pass.Reportf(node.Pos(), "Scope.IsOwner authority fields are resolved by services, not read in repositories — scope data via CenterWideFor(<resource>.view_all) instead")
			}
		case *ast.CallExpr:
			sel, ok := node.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			fnObj, ok := resolveSelObj(pass, sel).(*types.Func)
			if !ok {
				return true
			}
			switch {
			case isMethodOn(fnObj, authctxPkgPath, "Scope", "Has"):
				pass.Reportf(node.Pos(), "Scope.Has is forbidden in repositories — scope data via CenterWideFor(<resource>.view_all) instead")
			case isMethodOn(fnObj, authctxPkgPath, "Scope", "CenterWideFor") && !isRead:
				pass.Reportf(node.Pos(), "CenterWideFor may only widen a read-named function; widen writes with Scope.WriteWide() instead")
			case isPkgFunc(fnObj, authctxPkgPath, "StaffRolesFor"):
				pass.Reportf(node.Pos(), "StaffRolesFor is forbidden in repositories — the capability map is resolved in services")
			case isPkgFunc(fnObj, authctxPkgPath, "StaffRoleCan"):
				pass.Reportf(node.Pos(), "StaffRoleCan is forbidden in repositories — the capability map is resolved in services")
			}
		}
		return true
	})
}

// checkNoReset implements R2 for a single non-test file.
func checkNoReset(pass *analysis.Pass, file *ast.File) {
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if !isGormDBPtr(pass.TypesInfo.TypeOf(sel.X)) {
			return true
		}
		switch sel.Sel.Name {
		case "Unscoped":
			pass.Reportf(call.Pos(), "Unscoped resets scoping already applied to this *gorm.DB chain")
		case "Session":
			if sessionResetsNewDB(pass, call) {
				pass.Reportf(call.Pos(), "Session(&gorm.Session{NewDB: true}) discards scoping already applied to this *gorm.DB chain")
			}
		}
		return true
	})
}

// sessionResetsNewDB reports whether call's arguments include a
// gorm.Session{NewDB: true} literal.
func sessionResetsNewDB(pass *analysis.Pass, call *ast.CallExpr) bool {
	for _, arg := range call.Args {
		expr := arg
		if unary, ok := expr.(*ast.UnaryExpr); ok {
			expr = unary.X
		}
		lit, ok := expr.(*ast.CompositeLit)
		if !ok || !isNamedType(pass.TypesInfo.TypeOf(lit), gormPkgPath, "Session") {
			continue
		}
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok || key.Name != "NewDB" {
				continue
			}
			if v, ok := kv.Value.(*ast.Ident); ok && v.Name == "true" {
				return true
			}
		}
	}
	return false
}

// checkLiteralConstruction implements the module-wide part of R3 for a
// single non-test, non-exempt file: hand-built Scope/OwnerAnchor literals,
// direct field assignment on a Scope value, and MintOwnerAnchor calls.
func checkLiteralConstruction(pass *analysis.Pass, file *ast.File) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CompositeLit:
			switch scopeKindOf(pass.TypesInfo.TypeOf(node)) {
			case "Scope":
				if len(node.Elts) > 0 {
					pass.Reportf(node.Pos(), "an authctx.Scope is built by hand with fields set; resolve it instead of constructing one")
				}
			case "OwnerAnchor":
				pass.Reportf(node.Pos(), "an authctx.OwnerAnchor is a proof; obtain one from centers instead of building it")
			}
		case *ast.AssignStmt:
			for _, lhs := range node.Lhs {
				sel, ok := lhs.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				if scopeKindOf(unwrapPtr(pass.TypesInfo.TypeOf(sel.X))) == "Scope" {
					pass.Reportf(node.Pos(), "a field of an authctx.Scope value is assigned directly; resolve a new Scope instead of mutating one")
				}
			}
		case *ast.CallExpr:
			sel, ok := node.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if fnObj, ok := resolveSelObj(pass, sel).(*types.Func); ok && isPkgFunc(fnObj, authctxPkgPath, "MintOwnerAnchor") {
				pass.Reportf(node.Pos(), "MintOwnerAnchor is minted by centers only")
			}
		}
		return true
	})
}
