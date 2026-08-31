---
title: "Phase 4: CRUD endpoint policy cutover"
status: done
estimate: "4 days"
dependsOn: [2, 3]
---
# Phase 4: CRUD endpoint policy cutover

Waves: classes/contacts/students/enrollments; sessions/schedules/scores/teaching assignments; then billing/payments operations that are truly CRUD.

## Tasks
- [x] Map collection GET=`list`, item GET=`read`, POST=`create`, PATCH/PUT=`edit`, DELETE=`delete`; document exceptions.
- [x] Attach manifest policies with only explicit legacy aliases.
- [x] Preserve center predicates, not-found masking, and class-staff View/Manage/Score as AND checks.
- [x] Remove owner-only checks only for intentional delegation after replacement tests pass.
- [x] Test owner, seeded/custom role, grant/deny, wrong center, hidden object, and unassigned class.
- [x] Prove list does not imply read and read does not imply enumeration.
- [x] Cut repository scoping over from `data.view_center_wide` to per-resource `<resource>.view_all` per wave; parity-test each resource in both directions (no center-wide leak without the key, no narrowed visibility for backfilled legacy holders).
<!-- Updated: Validation Session 1 - per-resource view_all scoping cutover; estimate 2.5d -> 4d -->

- [x] Emit denial metrics by canonical key/reason without sensitive data.

## Acceptance and verification
- [x] No CRUD route relies on UI guards or loses tenant/capability protection.
- [x] Verify `401`, `403`, masked not-found, deny precedence, and legacy compatibility after each wave.
- [x] Run feature integration, registry, API, race, lint, and OpenAPI checks per wave.

## Execution record

- Enforcement lives in one manifest-driven middleware,
  `internal/server/route_policy_enforce.go`: it keys on
  `(method, FullPath)`, fails closed on unclassified routes, applies
  owner-only and permission policies, and logs every denial via slog with
  canonical key, reason, and route (the denial-metrics surface — no payload
  data, only IDs already present in the request context).
- `registerFeatures` builds a single ordered `authChain`
  (`RequireAuth → ResolveScope → enforceRoutePolicy`) and every feature's
  `RegisterRoutes` mounts it as a variadic `auth ...gin.HandlerFunc`; features
  never assemble their own ordering. The old per-route
  `middleware.RequirePermission`/`RequireOwner` helpers were deleted with the
  cutover.
- Enforcement attached to all routes at once rather than per wave: migration
  000018 backfilled the 53-key baseline onto every system role and role-less
  member, so no member could lose access at flip time; the waves structured
  the repo-scoping cutover instead.
- Intentional delegation per the frozen phase-1 inventory (D column):
  students.create/edit/delete owner gates removed from the students service.
  `repo.Update` became a write-scoped `Updates` (masked not-found on
  RowsAffected==0, following AnonymizeAndDelete's precedent) so the widened
  class-staff read scope cannot leak writability.
- view_all scoping cutover: every repository `sc.CenterWide()` branch now
  reads `sc.CenterWideFor(<resource>.view_all)` per the inventory §5 mapping
  (classes+classstaff, contacts, students incl. ContactExists, enrollments
  incl. the picker standing bypass, sessions incl. canGenerate, attendance,
  statements, payments, billing, notifications). The legacy
  ReportsOversight/CanSendReports axis stays frozen (statements phone rule,
  contacts scopedRead, zalo) per the recorded decision — Phase 8 territory.
  `internal/features/scoping_guard_test.go` now bans `.CenterWide()` in
  repositories so the legacy axis cannot creep back.
- Tests: `internal/server/route_policy_enforce_test.go` (per-route unit
  matrix: owner passes all, full-grant member stops at owner-only, exact-key
  requirement per permission route, deny precedence, list≠read, unclassified
  fails closed, denial log shape, envelope) and
  `internal/server/policy_integration_test.go` (full router + Postgres:
  owner/baseline member, expired token 401, removed membership, immediate
  role-change effect, deny list keeps read, wave-1 view_all parity in both
  directions with students as the narrow probe — contacts reads sit on the
  frozen oversight axis, not a view_all key).
