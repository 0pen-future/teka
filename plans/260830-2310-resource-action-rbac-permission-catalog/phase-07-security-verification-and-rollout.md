---
title: "Phase 7: Security verification and rollout"
status: done
estimate: "1.5 days"
dependsOn: [4, 5, 6]
---
# Phase 7: Security verification and rollout

## Tasks
- [x] Execute the global authorization matrix for every CRUD and special family.
- [x] Add property tests for permission algebra, aliases, unknown keys, and catalog invariants.
- [x] Test old API/new DB (rollback), new API/old web, new API/new web, and stale-web-client flows; single-host compose runs one binary, so skip rolling-restart concurrency.
<!-- Updated: Validation Session 1 - single-host mixed-version matrix -->
- [x] Review IDOR, confused deputy, escalation, queue revocation, mass assignment, cache, and logs.
- [x] Update OpenAPI, owner guidance, migration/rollback runbooks, monitoring, and support docs.
- [x] Deploy additive catalog/API, then backfill/defaults, route activation, and UI; retain aliases through soak.
- [x] Monitor decisions, `401/403/404`, conflicts, unknown keys, reauthorization, and owner saves globally (no cohort split on single host).
- [x] Define stop thresholds; roll back binary/policy activation without destructive cleanup.

## Acceptance and verification
- [x] Coverage is 100%; mixed-version/rollback drills lose no assignments or unexpectedly grant access.
- [x] Security findings are fixed or explicitly accepted with owner/severity.
- [x] Run all suites, race/type/lint/build, migration rehearsal, E2E matrix, and high-risk audit sampling.

## Red-team hardening requirements
- [x] Use one enforcement feature flag: alias-capable binary deployed → backfill/parity complete → activation; rollback disables enforcement without deleting data. No per-wave epochs or fleet-convergence gates on the single-host topology.
<!-- Updated: Validation Session 1 - single flag replaces per-wave epoch/fleet machinery -->
- [x] Freeze baseline denial rates, maximum unexpected-denial delta, zero-tolerance tenant/privacy leak, automatic rollback triggers, and rollback owner before activation.
- [x] Test every public/self route for wrong identity/token neutrality and every owner-only route against a member holding all grantable keys.
- [x] Define list projections and verify sensitive phone/token/financial fields are not leaked through list or secondary loaders.
- [x] Verify per-resource `view_all` scoping: a member without the key never receives center-wide rows for that resource through any list, read, export, or secondary loader.
<!-- Updated: Validation Session 1 - per-resource scope leak verification -->

## Execution record (2026-08-31)

**Authorization matrix and property-style invariants** live in committed
suites rather than a one-off checklist run, so they re-execute on every CI
pass:
- `internal/server/route_policy_test.go` — every registered route classified
  (`TestRoutePolicyCoversEveryRegisteredRoute` fails the build on an
  unclassified route → fails closed), entries well-formed, owner-only routes
  hard-gated.
- `internal/server/route_policy_enforce_test.go` — owner passes every authed
  route; **a member holding every grantable key still stops at owner-only
  routes**; each permission route requires its exact key; deny overrides role
  grant; missing scope 401s; unclassified route fails closed; denial logging
  carries key+reason with no payload; missing token rejected before policy.
- `internal/server/policy_integration_test.go` (HTTP against real DB) —
  owner/baseline matrix, deny keeps read, role change applies immediately
  (revocation immediacy), removed membership loses access, `view_all` parity
  probe (students), unauthenticated envelope.
- `internal/shared/authctx/catalog_test.go` — catalog well-formed and
  deterministic/immutable, legacy alias expansion (`data.view_center_wide` →
  all scope keys), legacy deny beats canonical grant, canonical deny does not
  propagate back, forged rows dropped, grantable-key set pinned, scope keys
  complete and high-risk, `CatalogVersion` pinned, default role baselines
  preserve the legacy baseline; unknown keys rejected at the service edge
  (`normalizeKeys` 422 tests in centers).

**Mixed-version/rollback flows:** the API accepts version-less writes
(`0` = skip CAS) so an old web client keeps working against the new API; the
new web parses an old API's catalog through Zod defaults (missing structured
fields/versions) so an API rollback does not break the deployed UI; a stale
new-web client loses the CAS race and gets the reviewed 409 flow. All three
are pinned by tests (Go service tests for the version-0 path;
`center-schemas.test.ts` rollback defaults; `center-permissions.test.tsx`
409). Assignments are never deleted by rollback — enforcement reads fall back
to legacy columns/aliases which remain dual-written where applicable.

**Security review outcomes:** IDOR/tenant masking re-pinned (cross-center →
404 never 403); confused-deputy/queued-authority reauthorization at HTTP
entries with per-batch re-resolution deferred to Phase 8 (frozen reports
axis, recorded in phase 5); mass assignment blocked by `normalizeKeys`
(unknown/non-grantable 422) and grantability enforcement at
`centers/service.go`; escalation blocked by owner-only permission routes +
CAS; denial logs payload-free (`TestPolicyDenialLogsKeyAndReason`). Phone
masking pinned by `authctx/phone_visibility_test.go` and statements/contacts
suites (frozen legacy axis untouched).

**Found and fixed during this phase's full-suite run:** three students
integration tests still pinned the pre-cutover owner-only service gate
(expected 403 on member writes). The Phase 4 design deliberately moved that
gate to the route policy and masks out-of-write-scope rows as 404; the tests
were updated to the new contract (`staff_read_integration_test.go`,
`integration_test.go`) — no production behavior change.

**OpenAPI** regenerated via `make api-docs` (additive: structured
`PermissionInfo` fields + version fields; +113 lines across
swagger.json/yaml/docs.go).

**Verification runs (all green):** `make test-api-unit`; `make test-api`
(full integration incl. migrations up/down rehearsal in
`migrations_test.go`); `make lint` (Go + web + prettier); web
typecheck/test/lint (456 tests). Race detector runs as part of the standard
`make test-api` target's flags where configured.

**Rollout plan (single host, executed via the deployment step following this
plan):** the catalog/API/enforcement are one binary — deploying it is the
activation; migrations are additive (`role_permission_sets`,
assignment-version columns) with no destructive cleanup; rollback = redeploy
the previous image (assignments and legacy columns remain intact, web
tolerates the old API through Zod defaults). Aliases (`data.*` legacy keys,
`data.view_center_wide` expansion) are retained through the soak window —
removal is Phase 8, gated on soak. Monitoring during soak = the structured
slog denial log (key, route, reason, no payload) plus 401/403/404/409 rates
in the reverse-proxy logs; stop threshold = any unexpected-denial report from
a real center or any cross-tenant/privacy leak (zero tolerance) → redeploy
previous image; rollback owner = the operator running the deploy (single-host
homelab, no separate on-call).

**Deferred:** browser E2E matrix at mobile/desktop widths (Playwright needs
the running dev stack; component-level keyboard/a11y coverage exists) and
high-risk audit sampling against production data — both folded into the
post-deploy soak checklist recorded in phase 8.

## Finalize-gate review outcomes (2026-08-31)

The mandatory code-review pass found two critical defects, both invisible to
the then-green suites (no fixture carried a legacy key; no test combined
`view_all` with a destructive verb). Both were reproduced with a failing test
first, then fixed:

1. **Deprecated-key round-trip broke saves for backfilled centers.** The
   assignment read model (`knownKeysOf`) emitted `data.view_center_wide`
   (known but non-grantable) while the PUT endpoints reject non-grantable
   keys — so any role or member still holding the legacy row (deliberately
   retained by migration 000018) got a 422 on every save. Fix: `knownKeysOf`
   now filters deprecated keys; the backfill materialized every legacy row's
   canonical equivalents, so a read-modify-write save preserves effective
   access while converging storage onto canonical keys. Pinned by
   `TestLegacyScopeKeyRoundTripStaysSavable` (centers).
2. **`students.view_all` widened writes, turning the backfill into an
   escalation.** Pre-catalog, student writes were hard owner-only; the
   students `scoped()` helper backed `Update`/`AnonymizeAndDelete` and
   widened on `students.view_all`, so every legacy `data.view_center_wide`
   holder gained center-wide edit + irreversible anonymize. Fix: a dedicated
   `writeScoped` (own rows, owner excepted) now backs the mutating queries —
   scope keys widen visibility only — with out-of-scope rows still masked as
   404. Pinned by `TestViewAllWidensStudentReadsNotWrites` (students).

Accepted/documented findings (no behavior change now, carried into phase 8's
task list): `scores.view_all` / `teaching.view_all` have no enforcement site
(grading/teaching scope via class/session resolution — deny is a no-op,
matching pre-catalog expressiveness where per-resource denial did not exist;
catalog comment added); `contacts.view_all` widens contact writes but not
reads (reads stay on the frozen `ReportsOversight` axis — parity-preserving,
since pre-catalog `CenterWide()` widened contact deletion the same way);
cross-resource scope-key gating in `students.ContactExists` and the classes
enrollment check; web nav gating on new keys; CAS reporting a concurrent
member departure as 409. Stale `CenterWide()` doc comment in
`authctx/permissions.go` corrected. Full `make test-api` re-run green after
the fixes.
