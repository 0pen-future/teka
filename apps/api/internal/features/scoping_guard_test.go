package features_test

import (
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
	paths, err := filepath.Glob("*/repository.go")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no feature repositories found — glob root moved?")
	}
	for _, path := range paths {
		if filepath.Dir(path) == "centers" {
			continue
		}
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
