package migrations_test

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"testing"

	"teka/apps/api/internal/shared/authctx"
	"teka/apps/api/migrations"
)

// The 000018 backfill embeds two key lists that must never drift from the
// code-owned catalog: the default operational baseline every system role
// receives, and the per-resource view_all set the legacy
// data.view_center_wide rows expand into. The SQL cannot import Go, so the
// lists are literal — this test pins them to authctx and to the checksum the
// migration records in its ledger row.

const backfillUpFile = "000018_resource_action_catalog_backfill.up.sql"

var keyLiteral = regexp.MustCompile(`'([a-z_]+\.[a-z_]+)'`)

// markedBlocks returns the content of every block delimited by
// "-- teka:<name>-begin" / "-- teka:<name>-end" in src.
func markedBlocks(t *testing.T, src, name string) []string {
	t.Helper()
	begin := "-- teka:" + name + "-begin"
	end := "-- teka:" + name + "-end"
	var blocks []string
	rest := src
	for {
		i := strings.Index(rest, begin)
		if i < 0 {
			break
		}
		rest = rest[i+len(begin):]
		j := strings.Index(rest, end)
		if j < 0 {
			t.Fatalf("marker %q opened but never closed", name)
		}
		blocks = append(blocks, rest[:j])
		rest = rest[j+len(end):]
	}
	if len(blocks) == 0 {
		t.Fatalf("no %q block found in %s", name, backfillUpFile)
	}
	return blocks
}

func keysIn(block string) []string {
	var keys []string
	for _, m := range keyLiteral.FindAllStringSubmatch(block, -1) {
		keys = append(keys, m[1])
	}
	return keys
}

func activeScopeKeys() []string {
	var keys []string
	for _, d := range authctx.PermDefs() {
		if d.Kind == authctx.PermKindScope && !d.Deprecated {
			keys = append(keys, d.Key)
		}
	}
	return keys
}

// mappingChecksum is the canonical fingerprint of the SQL-embedded mapping:
// the default baseline and the scope-expansion targets, both in catalog
// order. The migration writes it into rbac_backfill_ledger so an operator can
// tell which catalog generation a database was backfilled under.
func mappingChecksum() string {
	payload := strings.Join(authctx.DefaultRoleKeys(), "\n") +
		"\n|\n" + strings.Join(activeScopeKeys(), "\n")
	sum := sha256.Sum256([]byte(payload))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func TestBackfillSQLMatchesCatalog(t *testing.T) {
	raw, err := migrations.FS.ReadFile(backfillUpFile)
	if err != nil {
		t.Fatalf("read %s: %v", backfillUpFile, err)
	}
	src := string(raw)

	defaults := authctx.DefaultRoleKeys()
	for i, block := range markedBlocks(t, src, "default-keys") {
		got := keysIn(block)
		if len(got) != len(defaults) {
			t.Fatalf("default-keys block %d holds %d keys, catalog baseline holds %d",
				i, len(got), len(defaults))
		}
		for j, key := range defaults {
			if got[j] != key {
				t.Errorf("default-keys block %d position %d: SQL has %q, catalog has %q",
					i, j, got[j], key)
			}
		}
	}

	scope := activeScopeKeys()
	for i, block := range markedBlocks(t, src, "scope-keys") {
		got := keysIn(block)
		if len(got) != len(scope) {
			t.Fatalf("scope-keys block %d holds %d keys, catalog holds %d",
				i, len(got), len(scope))
		}
		for j, key := range scope {
			if got[j] != key {
				t.Errorf("scope-keys block %d position %d: SQL has %q, catalog has %q",
					i, j, got[j], key)
			}
		}
	}

	// The single legacy key the expansion reads must be the deprecated one.
	for _, block := range markedBlocks(t, src, "legacy-scope-key") {
		got := keysIn(block)
		if len(got) != 1 || got[0] != authctx.PermDataViewCenterWide {
			t.Errorf("legacy-scope-key block must reference exactly %q, got %v",
				authctx.PermDataViewCenterWide, got)
		}
	}

	if want := mappingChecksum(); !strings.Contains(src, "'"+want+"'") {
		t.Errorf("ledger checksum literal missing or stale; the SQL must record %s", want)
	}
}
