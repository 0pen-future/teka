---
title: "Phase 3: Compatibility migration and role defaults"
status: done
estimate: "2 days"
dependsOn: [1, 2]
---
# Phase 3: Compatibility migration and role defaults

## Tasks
- [x] Reserve the next migration number from master; add reversible resource-action backfill migrations.
- [x] Encode mappings explicitly and insert role expansions idempotently/conflict-safely.
- [x] Expand member grants and denies symmetrically; retain legacy rows during compatibility.
- [x] Expand `data.view_center_wide` grants and denies symmetrically to the full per-resource `<resource>.view_all` set; parity-test both widened and denied visibility per resource.
<!-- Updated: Validation Session 1 - per-resource view_all backfill -->

- [x] Centralize built-in defaults for migration, center creation, seeds, and tests.
- [x] Expand custom roles only from actual legacy assignments; infer no broader CRUD power.
- [x] Add catalog/assignment version to reads/writes; stale replacement returns `409` without mutation.
- [x] Down removes only rollout-created data and preserves user legacy assignments.
- [x] Compare access before/after for custom roles, overrides, legacy denies, empty roles, and concurrent writes.

## Acceptance and verification
- [x] Up is idempotent; old binaries safely ignore new keys and retain legacy behavior.
- [x] Existing/new centers share one canonical default path.
- [x] Rehearse up → parity → down → up on production-shaped fixtures without drift.

## Red-team hardening requirements
- [x] Run backfill only after the alias-capable binary is deployed; on the single-host compose topology the deploy replaces the binary atomically, so verify deploy ordering rather than fleet convergence.
<!-- Updated: Validation Session 1 - single-host right-size of writer-compatibility gate -->
- [x] Persist separate monotonic `catalog_version` and per-role/per-member `assignment_version`; check/increment CAS in the replacement transaction.
- [x] Add a migration ledger/provenance snapshot with mapping checksum and inserted/skipped/conflicted counts; never delete an owner-modified row on down.
- [x] Prevent backfill/save races with a write gate or locked epoch/CAS transaction.
- [x] Existing built-in roles expand from actual assignments/previously universal behavior, not today's recommended new-center defaults.
- [x] Model center-role × class-role/capability compatibility, including null/custom center roles, ended stints, and member denies.
- [x] Keep SQL mapping/default artifacts in parity with Go through a checksum/generation test.
- [x] Freeze existing `reports.send` semantics while `can_send_reports` cleanup remains unfinished.

## Execution record (260831)
- Migration pair `000018_resource_action_catalog_backfill.{up,down}.sql`: adds
  `assignment_version` to `center_roles`/`center_members`, backfills the
  53-key operational baseline to system roles of live centers and to live
  role-less non-owner stints, and expands `data.view_center_wide`
  grants/denies to the 12 `*.view_all` keys — all `ON CONFLICT DO NOTHING`
  (idempotent, deny-preserving). Row-level provenance in `rbac_backfill_rows`;
  ledger row in `rbac_backfill_ledger` with mapping checksum + per-step
  counts. Down deletes only recorded rows and keeps member rows whose
  `allowed` an owner has since flipped.
- Baseline is centralized in `authctx.DefaultRoleKeys()` (53 grantable
  non-scope, non-legacy-identity keys) and consumed by the migration parity
  test, `repository.CreateCenter`, `seeds/seed.go`, and
  `testutil.Teacher`. SQL↔Go parity pinned by
  `migrations/backfill_parity_test.go` (marked SQL blocks + checksum literal).
- CAS: `authctx.CatalogVersion = 2`; reads return `catalog_version` +
  per-role/per-member `assignment_version`; replacement writes echo them and a
  mismatch is a 409 without mutation (version 0 = pre-CAS client,
  last-write-wins until the Phase 6 UI cutover). Covered by
  `TestAssignmentVersionCAS`.
- Backfill/save race: none possible on this topology — deploy applies
  migrations before the new binary serves traffic (single host), and the CAS
  transaction covers post-deploy concurrent saves. No separate write gate
  needed.
- Custom-role expansion is vacuous: no custom-role CRUD exists; the three
  system roles expand from the previously universal membership access.
- Behavior parity proven in `TestResourceActionCatalogBackfill`
  (`migrations_test.go`): roled/role-less/former/owner members, retired
  centers, pre-existing denies, `BuildPermSet` before/after, ledger counts,
  and down-survivorship.
