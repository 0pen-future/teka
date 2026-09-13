package kanban

import (
	"os/exec"
	"strings"
	"testing"
)

// forbiddenImportSubstrings are the boundary violations this package must
// never depend on, directly or transitively: any host application internals,
// or the two frameworks (Gin, GORM) the plan explicitly keeps out of
// pkg/kanban. Everything else (stdlib plus github.com/google/uuid) is
// allowed.
var forbiddenImportSubstrings = []string{
	"teka/apps/api/internal",
	"github.com/gin-gonic",
	"gorm.io",
}

// TestImportBoundary fails if pkg/kanban ever gains a forbidden dependency,
// directly or transitively. `go list -deps .` is the ground truth for the
// resolved dependency graph (unlike grepping import statements, it also
// catches transitive pulls through an otherwise-innocent-looking package).
//
// This was verified to actually catch a violation: a temporary import of
// "teka/apps/api/internal/database" was added to this package and confirmed
// to fail this test, then reverted, before this test was considered done.
func TestImportBoundary(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatalf("go list -deps .: %v", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		for _, forbidden := range forbiddenImportSubstrings {
			if strings.Contains(line, forbidden) {
				t.Errorf("pkg/kanban depends on forbidden package %q", line)
			}
		}
	}
}
