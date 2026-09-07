package features_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Repositories scope data through Scope.CenterWideFor(<resource>.view_all)
// only. Branching on IsOwner would fork the data-scoping axis away from the
// permission system, calling Has() would let an arbitrary capability key widen
// reads, and the legacy CenterWide() would collapse the per-resource axis back
// into one center-wide switch — all regressions this guard catches at
// compile-test time. centers/repository.go is the scope-resolution home: its
// SQL computes is_owner and its ScopeRow carries it, so it is exempt.
func TestRepositoriesScopeThroughCenterWideOnly(t *testing.T) {
	for _, path := range repositoryPaths(t) {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(src), "\n") {
			// StaffRolesFor/StaffRoleCan are banned too: the capability
			// map is resolved in services, and repositories only bind the
			// resulting role slice — a repo consulting the map would fork
			// write authorization away from its one home.
			for _, banned := range []string{"sc.IsOwner", "scope.IsOwner", ".Has(", ".CenterWide()", "StaffRolesFor", "StaffRoleCan"} {
				if strings.Contains(line, banned) {
					t.Errorf("%s:%d: %q is forbidden in repositories — scope data via sc.CenterWideFor(<resource>.view_all)", path, i+1, banned)
				}
			}
		}
	}
}

// A <resource>.view_all key is a visibility key: it widens reads and nothing
// else. Writes widen through Scope.WriteWide (the owner alone). The two are
// told apart syntactically: every CenterWideFor call in a repository must sit
// inside a function whose name says it is a read (readScoped, scopedRead,
// readNarrow, GetPeriodRead, …), so a write helper that consults the
// visibility key fails here before it ships. The name convention is the
// contract; a write-named function that wants center reach calls WriteWide.
func TestRepositoriesWidenWritesThroughWriteWideOnly(t *testing.T) {
	fset := token.NewFileSet()
	for _, path := range repositoryPaths(t) {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			isRead := strings.Contains(strings.ToLower(fn.Name.Name), "read")
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "CenterWideFor" {
					return true
				}
				if !isRead {
					pos := fset.Position(call.Pos())
					t.Errorf("%s:%d: CenterWideFor inside %s — a visibility key may only widen a read-named function; widen writes with sc.WriteWide()",
						path, pos.Line, fn.Name.Name)
				}
				return true
			})
		}
	}
}

// repositoryPaths lists every non-test feature file that carries repository
// code: repository.go itself plus any sibling declaring methods on the
// *gormRepository receiver (sessions/pending.go, for one), so a query moved
// out of repository.go stays under both guards.
func repositoryPaths(t *testing.T) []string {
	t.Helper()
	paths, err := filepath.Glob("*/*.go")
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, path := range paths {
		if filepath.Dir(path) == "centers" || strings.HasSuffix(path, "_test.go") {
			continue
		}
		if filepath.Base(path) != "repository.go" {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(src), "*gormRepository)") {
				continue
			}
		}
		out = append(out, path)
	}
	if len(out) == 0 {
		t.Fatal("no feature repositories found — glob root moved?")
	}
	return out
}

// A Scope is resolved from the database for the caller and nowhere else. A
// service that wants to act on another teacher's rows names them with an
// Anchor (Scope.AnchorTo / Scope.Self); building a Scope by hand, mutating its
// authority fields, or minting an OwnerAnchor without the centers proof would
// all reopen the "fake Scope with borrowed rights" hole this guard closes.
// Scope resolution lives in centers and middleware; test fixtures, testutil
// and the dev seeder (which resolves from the same membership SQL) are exempt.
// The walk starts at the module root so cmd/ and seeds/ are covered, not just
// internal/. The check walks the AST so a literal split over several lines,
// nested inside a slice or map literal, or built through an aliased import
// cannot slip past it. What it cannot see is a copied Scope whose TeacherID
// is reassigned — that stays a review item until the type-based guard lands.
func TestScopeLiteralsOnlyWhereResolved(t *testing.T) {
	const authctxPath = "teka/apps/api/internal/shared/authctx"
	exempt := func(path string) bool {
		return strings.HasPrefix(path, "../../internal/features/centers/") ||
			strings.HasPrefix(path, "../../internal/middleware/") ||
			strings.HasPrefix(path, "../../internal/testutil/") ||
			strings.HasPrefix(path, "../../internal/shared/authctx/") ||
			strings.HasPrefix(path, "../../seeds/")
	}
	mintAllowed := func(path string) bool {
		return strings.HasPrefix(path, "../../internal/features/centers/") ||
			strings.HasPrefix(path, "../../internal/shared/authctx/")
	}

	fset := token.NewFileSet()
	walked := 0
	err := filepath.WalkDir("../..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		walked++
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		pkgName := ""
		for _, imp := range file.Imports {
			if strings.Trim(imp.Path.Value, `"`) != authctxPath {
				continue
			}
			pkgName = "authctx"
			if imp.Name != nil {
				pkgName = imp.Name.Name
			}
		}
		if pkgName == "" {
			return nil
		}
		if pkgName != "authctx" {
			t.Errorf("%s: import authctx under its own name; an alias hides Scope construction from readers", path)
		}
		isAuthctx := func(expr ast.Expr, name string) bool {
			sel, ok := expr.(*ast.SelectorExpr)
			if !ok || sel.Sel.Name != name {
				return false
			}
			ident, ok := sel.X.(*ast.Ident)
			return ok && ident.Name == pkgName
		}
		// elemType is the element type a slice/array/map literal implies for
		// its untyped inner literals ([]authctx.Scope{{...}}), which carry a
		// nil Type of their own.
		elemType := func(expr ast.Expr) ast.Expr {
			switch typ := expr.(type) {
			case *ast.ArrayType:
				return typ.Elt
			case *ast.MapType:
				return typ.Value
			}
			return nil
		}
		checkLit := func(typ ast.Expr, elts []ast.Expr, pos token.Pos) {
			if isAuthctx(typ, "Scope") && len(elts) > 0 {
				t.Errorf("%s: a Scope with fields is built by hand; name the rows with sc.AnchorTo(teacherID) / sc.Self() instead", fset.Position(pos))
			}
			if isAuthctx(typ, "OwnerAnchor") {
				t.Errorf("%s: an OwnerAnchor is a proof; ask centers.Service.ResolveOwnerAnchor for one", fset.Position(pos))
			}
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CallExpr:
				if !mintAllowed(path) && isAuthctx(node.Fun, "MintOwnerAnchor") {
					t.Errorf("%s: MintOwnerAnchor is minted by centers.Service.ResolveOwnerAnchor only", fset.Position(node.Pos()))
				}
			case *ast.CompositeLit:
				if exempt(path) {
					return true
				}
				checkLit(node.Type, node.Elts, node.Pos())
				if elem := elemType(node.Type); elem != nil {
					for _, elt := range node.Elts {
						if kv, ok := elt.(*ast.KeyValueExpr); ok {
							elt = kv.Value
						}
						if inner, ok := elt.(*ast.CompositeLit); ok && inner.Type == nil {
							checkLit(elem, inner.Elts, inner.Pos())
						}
					}
				}
			case *ast.AssignStmt:
				if exempt(path) {
					return true
				}
				for _, lhs := range node.Lhs {
					sel, ok := lhs.(*ast.SelectorExpr)
					if !ok {
						continue
					}
					switch sel.Sel.Name {
					case "IsOwner", "Perms", "CanSendReports":
						t.Errorf("%s: Scope authority fields are resolved, never assigned", fset.Position(node.Pos()))
					}
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if walked == 0 {
		t.Fatal("no Go files walked — module root moved?")
	}
}
