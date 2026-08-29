---
phase: 1
title: "Foundation (zero behavior change)"
status: completed
priority: P1
effort: "2d"
dependencies: []
---

# Phase 1: Foundation (zero behavior change)

## Overview

Introduce the RBAC schema, permission catalog, and aggregated scope resolution
while keeping every observable behavior identical. Acceptance = the full
existing test suite passes WITHOUT modification.

## Requirements

- Functional: new tables + backfill; catalog constants; `Scope` gains a
  permission set with `Has()` / `CenterWide()`; existing grant/revoke
  send-reports endpoints write override rows (dual-write with the legacy
  column).
- **Zero-test-edit guard:** `ReportsOversight()` BODY stays exactly
  `IsOwner || CanSendReports` (authctx.go:55-57). During dual-life the resolve
  path computes `CanSendReports = cm.can_send_reports OR Has(reports.send)` —
  the COLUMN stays authoritative; perms only add. Pure perms-only recompute
  would break zero-test-edit: `testutil.Secretary` (fixtures.go:218-223)
  grants via raw-column `GrantSendReports` with NO override row, and the
  centers integration helper `e.scope()` (integration_test.go:77-82) calls the
  REAL `ResolveScope`, so `secretary_integration_test.go:29`
  (`require.True(secScope.CanSendReports)`) would fail. OR-read cannot
  resurrect revoked perms: reopen/close resets BOTH column and override rows
  atomically, and dual-write keeps them in parity. Phase 4 switches to
  perms-only. `testutil.ScopeFor` / `testutil.GrantSendReports`
  (fixtures.go:140-167,202, column-based) stay untouched until phase 4.
- Non-functional: `ResolveScope` stays ONE DB round trip; no caching; scope
  value remains read-only after `SetScope` (stored by value in gin context).

## Architecture

### Migration `000013_center_rbac.up.sql` (append-only; write `.down.sql` too)

```sql
CREATE TABLE center_roles (
    id         UUID PRIMARY KEY,
    center_id  UUID NOT NULL REFERENCES centers(id) ON DELETE CASCADE,
    key        VARCHAR(32)  NOT NULL,  -- 'giao_vien' | 'hoc_vu' | 'tro_giang'
    name       VARCHAR(100) NOT NULL,  -- display name, editable later
    is_system  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (center_id, key)
);

CREATE TABLE center_role_permissions (
    role_id        UUID NOT NULL REFERENCES center_roles(id) ON DELETE CASCADE,
    permission_key VARCHAR(64) NOT NULL,
    PRIMARY KEY (role_id, permission_key)
);

ALTER TABLE center_members ADD COLUMN role_id UUID REFERENCES center_roles(id);

CREATE TABLE center_member_permissions (
    teacher_id     UUID NOT NULL,
    center_id      UUID NOT NULL,
    permission_key VARCHAR(64) NOT NULL,
    allowed        BOOLEAN NOT NULL,   -- TRUE = grant, FALSE = deny
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (teacher_id, center_id, permission_key),
    FOREIGN KEY (teacher_id, center_id)
        REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE
);
```

Backfill (same migration): 3 roles per existing live center; `role_id` = the
center's `giao_vien` role on every live non-owner stint (owner stint, if any,
stays NULL — owner is outside the role system); every live
`can_send_reports=TRUE` stint → `center_member_permissions` grant row for
`reports.send`. Keep `can_send_reports` column (dual life until phase 4).
Comments in Vietnamese matching migration house style; explain invariants, not
provenance.

### Catalog — `internal/shared/authctx/permissions.go` (new file, same package)

Typed constants, registry, and set type. v1 keys (FROZEN at end of phase 1):

| Key | Today's gate it replaces |
|-----|--------------------------|
| `data.view_center_wide` | repo `scoped()` owner-sees-all axis |
| `reports.send` | `can_send_reports` / `ReportsOversight()` |
| `members.manage` | remove member (`centers/service.go:247`) |
| `center.manage` | rename center (`centers/service.go:204`) |
| `invitations.manage` | invitations feature (`invitations/service.go:131`, `routes.go`) |
| `audit.read` | audit feature (`audit/service.go:74`) |
| `imports.run` | imports (`imports/service.go:207`) |
| `dashboard.view` | centers dashboard (`dashboard.go:57`) |
| `teaching.review_queue` | teaching `ReviewQueue` read (`teaching/service.go:239`) — separate key by user decision: grants lesson-plan review visibility without exposing the center-wide financial/attendance dashboard |

NOT keys (owner-only forever): permission management itself, ownership handoff
(`handoff/service.go:100`), write-on-behalf paths in repositories, and teaching
`resolveReviewedClass` (`teaching/service.go:~504`) — verified WRITE gate
(approve/redo/reopen lesson plans); mapping it to `teaching.review_queue` would
let a read-permission holder mutate reviews. It keeps `IsOwner`.
<!-- Updated: Validation Session 1 (2026-08-29) — teaching.review_queue split
     out of dashboard.view; members.manage cite fixed :224→:247 -->

```go
type PermSet map[string]struct{}          // read-only after SetScope

func (s Scope) Has(key string) bool       // true unconditionally when IsOwner
func (s Scope) CenterWide() bool          // IsOwner || Has(PermDataViewCenterWide)
```

Registry test: every constant registered exactly once; unknown DB keys ignored
on read.

### Scope resolution — `centers/repository.go` + `centers/service.go`

Extend the `ResolveScope` raw query (repository.go:160) to aggregate, still one
round trip, e.g. add:

```sql
COALESCE((SELECT array_agg(rp.permission_key) FROM center_role_permissions rp
          WHERE rp.role_id = cm.role_id), '{}')                    AS role_perms,
COALESCE((SELECT array_agg(mp.permission_key ORDER BY mp.permission_key)
            FILTER (WHERE mp.allowed), '{}') ...                   AS grants,
-- and the deny list symmetrically from center_member_permissions
```

Service combines: `perms = (role_perms ∪ grants) − denies`; sets
`CanSendReports = cm.can_send_reports OR Has(reports.send)` (dual-life OR-read,
see Requirements — column authoritative until phase 4) so all 21
`ReportsOversight()` sites and the `CanSendReports` field consumers
(`request_events.go`, statements, fixtures) are untouched.

### Repo swap

Mechanical: every `if !sc.IsOwner` inside `*/repository.go` `scoped()` helpers
→ `if !sc.CenterWide()`. Behavior identical (nobody holds
`data.view_center_wide` yet). `imports/apply.go` anchor scope keeps
`IsOwner=false` semantics — its anchor must also not hold permissions.

### Dual-write and reopen reset (repository layer, atomic)

`grantSendReports`/`revokeSendReports` (centers service) now upsert/delete the
`reports.send` override row AND the legacy column in the same transaction.

The reopen reset lives today INSIDE repository SQL, not the service:
`centers/repository.go:275` (`OpenMembership` `ON CONFLICT … DO UPDATE SET …
can_send_reports = FALSE`) and `:287` (`CloseMembership`). Adding "reset
`role_id` to the center's `giao_vien` role + DELETE the stint's override rows"
turns each from one statement into several — they MUST run atomically in the
caller's transaction (`database.FromContext`; e.g. invitation-accept at
`invitations/service.go:330,356`). Owner guard: `cli/create_center.go:111`
calls `OpenMembership` for the OWNER — the owner's stint must NOT receive a
role (`role_id` stays NULL) and is filtered out of permission listings; owner
stays outside the role system.

`NULL role_id` is defined behavior, not an error: it means "empty role perms"
(equivalent to `giao_vien` in v1, whose set is empty). Raw-SQL insert paths
(`testutil.JoinCenter` fixtures.go:172-183, `seeds/seed.go:389`) may leave it
NULL; the reopen-reset integration test must therefore drive the REAL service
path, not the fixture.

### Seeding paths (all three, easy to miss)

- `centers` service center-creation path (registration).
- `internal/cli/create_center.go`.
- `seeds/seed.go` (`make seed`).

## Related Code Files

- Create: `apps/api/migrations/000013_center_rbac.up.sql` + `.down.sql`,
  `apps/api/internal/shared/authctx/permissions.go` (+ test)
- Modify: `apps/api/internal/shared/authctx/authctx.go`,
  `apps/api/internal/features/centers/{repository,service,model}.go`,
  `apps/api/internal/cli/create_center.go`, `apps/api/seeds/seed.go`,
  repositories with `IsOwner` scoping: students, classes, sessions,
  attendance, payments, collections, statements (grep-driven list),
  `apps/api/internal/testutil/fixtures.go` (extend, don't break callers)
- Delete: none

## Implementation Steps

1. Write migration pair + backfill; `make test-api` migration tests green.
2. Add catalog constants + PermSet + registry test in `authctx`.
3. Extend `ResolveScope` query + service combination; add `Has`/`CenterWide`;
   recompute `CanSendReports` from perms.
4. Swap repo `IsOwner` → `CenterWide()`; add grep guard test asserting no
   `IsOwner` and no `.Has(` in `*/repository.go`.
5. Dual-write in grant/revoke endpoints; reset role+overrides on membership
   reopen.
6. Seed roles in all 3 creation paths.
7. New integration tests: revocation next-request effect; reopen reset (via
   real service path — see Dual-write section); backfill correctness
   (can_send_reports → override row) as a STEPWISE migration test in
   `migrations_test.go` (insert data at v000012, step to 000013) — plain
   integration tests migrate an empty schema and cannot see the backfill.

## Success Criteria

- [x] `make test-api` green with zero edits to existing tests (total coverage
      76.3%, floor 60%). Only additions landed in `migrations_test.go`, plus
      the self-documented relative step-count bump 8→9 in
      `TestDownFoldsPersonalChannelIntoManual` that every new migration
      requires. Canary tests untouched.
- [x] `grep -rn "sc\.IsOwner\|scope\.IsOwner\|\.Has(" apps/api/internal/features/*/repository.go`
      → 0 outside `centers/repository.go`, now also pinned by the new guard
      test `internal/features/scoping_guard_test.go`.
- [x] Revocation + reopen-reset integration tests pass
      (`centers/rbac_integration_test.go`, 5 tests), plus stepwise backfill
      test `TestCenterRBACBackfill` in `migrations_test.go` — which caught a
      real down-migration bug (missing `DROP TABLE center_role_permissions`).
- [x] Catalog frozen and documented (frozen in
      `authctx/permissions.go`; documented in `docs/api-guidelines.md` Tenancy
      — "Configurable permissions (RBAC)").

## Risk Assessment

- **Backfill misses a creation path** → all three paths listed above; add an
  integration assertion that a fresh center has exactly 3 system roles.
- **Aggregated query regression (N+1 or wrong join)** → keep single Raw query;
  existing `TestResolveScope*` integration tests + new perm assertions.
- **Scope mutation after SetScope** → document read-only rule on PermSet;
  copies share the map, never write to it post-resolve.
- **Down migration** → drops tables + column; safe because phase 1 writes are
  derivable (column still authoritative for `reports.send` until phase 4).
- **Prod backfill** → runs automatically via the pre-deploy `migrate` service
  (`docker-compose.prod.yml`, migrations embedded in binary); `gen_random_uuid()`
  available since migration 000001. No separate runbook needed.
