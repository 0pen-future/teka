package scopelint_test

import (
	"fmt"
	"path/filepath"
	"sort"
	"testing"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"

	"teka/apps/api/tools/scopelint/scopelint"
)

// minRepositoryPackages is the floor TestNoDiagnosticsOnRepositoryTree
// asserts on the number of packages in which the analyzer identified a
// repository receiver. 21 packages match the real tree today; the floor
// sits a few below that so an incidental repository removal does not break
// the test, while a rename or split of a whole package's repository file —
// which used to disable R1/R3 for that package silently — still fails it.
const minRepositoryPackages = 18

// TestNoDiagnosticsOnRepositoryTree is scopelint's self-enforcement: it
// loads and analyses the real internal and cmd trees exactly as `go test
// ./...` would, so a missing witness ships as a failing test — under
// test-api-unit, test-api and CI alike — rather than depending on a
// Makefile target an agent can step around. It also asserts a floor on how
// many repository receivers were found at all, so a rename or split that
// stops the analyzer from finding a package's repository fails loudly
// instead of passing by analysing nothing.
func TestNoDiagnosticsOnRepositoryTree(t *testing.T) {
	moduleRoot, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}

	cfg := &packages.Config{
		Mode: packages.LoadAllSyntax,
		Dir:  moduleRoot,
	}
	pkgs, err := packages.Load(cfg, "teka/apps/api/internal/...", "teka/apps/api/cmd/...")
	if err != nil {
		t.Fatalf("packages.Load: %v", err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		t.Fatal("packages.Load reported package errors, see above")
	}

	scopelint.ResetRepositoryPackages()
	graph, err := checker.Analyze([]*analysis.Analyzer{scopelint.Analyzer}, pkgs, nil)
	if err != nil {
		t.Fatalf("checker.Analyze: %v", err)
	}

	var diags []string
	for act := range graph.All() {
		if act.Err != nil {
			t.Errorf("%s: %v", act.Package, act.Err)
			continue
		}
		for _, d := range act.Diagnostics {
			pos := act.Package.Fset.Position(d.Pos)
			diags = append(diags, fmt.Sprintf("%s: %s", pos, d.Message))
		}
	}
	if len(diags) > 0 {
		sort.Strings(diags)
		t.Errorf("scopelint found %d diagnostic(s) on internal/...:\n%s", len(diags), joinLines(diags))
	}

	if got := scopelint.RepositoryPackages(); got < minRepositoryPackages {
		t.Errorf("scopelint identified a repository receiver in only %d package(s), want at least %d — "+
			"a repository.go rename, split, or an emptied *gorm.DB field would silently disable R1/R3 for that package",
			got, minRepositoryPackages)
	}
}

func joinLines(lines []string) string {
	out := ""
	for _, line := range lines {
		out += line + "\n"
	}
	return out
}
