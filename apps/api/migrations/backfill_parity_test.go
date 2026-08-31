package migrations_test

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
	"testing"

	"teka/apps/api/migrations"
)

// The 000018 backfill embeds the key lists of the catalog generation it
// shipped under (catalog v2). Migrations are immutable while the catalog keeps
// evolving — v3 retired data.view_center_wide and the scores/teaching scope
// keys — so the expectations here are frozen literals of that v2 generation,
// not derivations from the live catalog. The test guards exactly one
// invariant: nobody edits the shipped SQL. Catalog drift is the live tests'
// job, not this file's.

const backfillUpFile = "000018_resource_action_catalog_backfill.up.sql"

// The default operational baseline of catalog v2, in catalog order — identical
// in v3, but frozen here because the SQL can never follow a future change.
var frozenDefaultKeys = []string{
	"classes.create", "classes.list", "classes.read", "classes.edit",
	"classes.delete", "classes.archive",
	"schedules.create", "schedules.edit", "schedules.delete",
	"contacts.create", "contacts.list", "contacts.read", "contacts.edit",
	"contacts.delete", "contacts.link_zalo",
	"students.create", "students.list", "students.read", "students.edit",
	"students.delete",
	"enrollments.create", "enrollments.list", "enrollments.read",
	"enrollments.delete", "enrollments.end",
	"sessions.create", "sessions.list", "sessions.read", "sessions.delete",
	"sessions.lifecycle",
	"attendance.read", "attendance.confirm",
	"scores.read", "scores.edit",
	"teaching.read", "teaching.edit",
	"billing.create", "billing.list", "billing.read", "billing.draft",
	"billing.close", "billing.void_invoice", "billing.adjust_invoice",
	"payments.create", "payments.list", "payments.read", "payments.allocate",
	"payments.reverse",
	"statements.list", "statements.read", "statements.generate",
	"statements.revoke",
	"notifications.mark_sent",
}

// The scope-expansion targets of catalog v2, in catalog order — including
// scores.view_all and teaching.view_all, which v3 retired (their rows were
// deleted again by migration 000020).
var frozenScopeKeys = []string{
	"classes.view_all", "contacts.view_all", "students.view_all",
	"enrollments.view_all", "sessions.view_all", "attendance.view_all",
	"scores.view_all", "teaching.view_all", "billing.view_all",
	"payments.view_all", "statements.view_all", "notifications.view_all",
}

// The pre-catalog center-wide axis the expansion read from; retired in v3.
const frozenLegacyScopeKey = "data.view_center_wide"

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

// mappingChecksum is the canonical fingerprint of the SQL-embedded mapping:
// the default baseline and the scope-expansion targets, both in catalog
// order. The migration wrote it into rbac_backfill_ledger so an operator can
// tell which catalog generation a database was backfilled under.
func mappingChecksum() string {
	payload := strings.Join(frozenDefaultKeys, "\n") +
		"\n|\n" + strings.Join(frozenScopeKeys, "\n")
	sum := sha256.Sum256([]byte(payload))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func TestBackfillSQLMatchesFrozenCatalogV2(t *testing.T) {
	raw, err := migrations.FS.ReadFile(backfillUpFile)
	if err != nil {
		t.Fatalf("read %s: %v", backfillUpFile, err)
	}
	src := string(raw)

	for i, block := range markedBlocks(t, src, "default-keys") {
		got := keysIn(block)
		if len(got) != len(frozenDefaultKeys) {
			t.Fatalf("default-keys block %d holds %d keys, frozen baseline holds %d",
				i, len(got), len(frozenDefaultKeys))
		}
		for j, key := range frozenDefaultKeys {
			if got[j] != key {
				t.Errorf("default-keys block %d position %d: SQL has %q, frozen list has %q",
					i, j, got[j], key)
			}
		}
	}

	for i, block := range markedBlocks(t, src, "scope-keys") {
		got := keysIn(block)
		if len(got) != len(frozenScopeKeys) {
			t.Fatalf("scope-keys block %d holds %d keys, frozen list holds %d",
				i, len(got), len(frozenScopeKeys))
		}
		for j, key := range frozenScopeKeys {
			if got[j] != key {
				t.Errorf("scope-keys block %d position %d: SQL has %q, frozen list has %q",
					i, j, got[j], key)
			}
		}
	}

	for _, block := range markedBlocks(t, src, "legacy-scope-key") {
		got := keysIn(block)
		if len(got) != 1 || got[0] != frozenLegacyScopeKey {
			t.Errorf("legacy-scope-key block must reference exactly %q, got %v",
				frozenLegacyScopeKey, got)
		}
	}

	if want := mappingChecksum(); !strings.Contains(src, "'"+want+"'") {
		t.Errorf("ledger checksum literal missing or stale; the SQL must record %s", want)
	}
}
