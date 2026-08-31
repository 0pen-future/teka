---
title: Resource-action RBAC permission catalog shipped (phases 1-7)
date: 2026-08-30
summary: "Catalog v2 with per-resource view_all scoping, route-policy registry, CAS versioning, migration 000018 backfill; two critical review findings fixed via TDD; phase 8 blocked on soak"
---

# Resource-action RBAC permission catalog shipped (phases 1-7)

## What happened

Implemented the full resource-action RBAC permission catalog plan
(`plans/260830-2310-resource-action-rbac-permission-catalog/`) via
/ak:cook --tdd --auto. Phases 1-7 done; committed as `fad8b88`
(87 files, +5149) on `teka/260831-0016`.

Core delivery: structured PermDef catalog (`internal/shared/authctx/catalog.go`,
CatalogVersion 2), legacy `data.*` alias expansion in BuildPermSet, per-resource
`<resource>.view_all` scope keys behind `Scope.CenterWideFor`, fail-closed
route-policy registry (`internal/server/route_policy.go`) with a coverage test
that fails the build on any unclassified route, CAS versioning on permission
assignments (catalog_version + assignment_version, 0 = skip, 409 on stale), and
migration `000018_resource_action_catalog_backfill` (system-role baselines plus
legacy `data.view_center_wide` expanded to the 12 canonical view_all keys,
ON CONFLICT DO NOTHING, ledgered, legacy rows retained through soak). Web got the
matrix UI, rollback-tolerant Zod defaults, and the 409 conflict flow. OpenAPI
regenerated (+113 lines).

## Failures worth remembering

The mandatory finalize code review caught two critical defects that every green
suite had missed, because no fixture carried a legacy key and no test combined
`view_all` with a destructive verb:

1. **Backfilled centers could not save permissions.** `knownKeysOf` emitted the
   deprecated `data.view_center_wide` in the assignment read model while the PUT
   endpoints 422 non-grantable keys — every read-modify-write save of a
   backfilled role/member failed. Fix: filter deprecated keys from the read
   model; the backfill already materialized canonical equivalents, so saves
   converge storage. Pinned by `TestLegacyScopeKeyRoundTripStaysSavable`.
2. **The backfill was a privilege escalation for student writes.** Students'
   `scoped()` widened on `students.view_all` and also backed Update and
   AnonymizeAndDelete, so every legacy center-wide viewer silently gained
   center-wide edit plus irreversible anonymize (pre-catalog these were hard
   owner-only). Fix: dedicated `writeScoped` (own rows + owner) for mutations —
   scope keys widen visibility only, out-of-scope writes mask as 404. Pinned by
   `TestViewAllWidensStudentReadsNotWrites`.

Both were reproduced with a failing test first. Full `make test-api` green after
the fixes (coverage 75.9%, floor 60%).

## Decision

Scope keys (`*.view_all`) are visibility-only by contract; write widening is
never implied. Deprecated keys stay effective via alias expansion but never
surface in read models. `scores.view_all` / `teaching.view_all` remain reserved
no-ops (grading/teaching scope via class resolution) — wire-or-retire recorded
in phase 8.

## Next steps

- Deploy per `docs/deployment.md` (single host: build images, compose -p teka up,
  migrate job applies 000018 before API start; rollback = previous image tags).
- Phase 8 (legacy cleanup: alias removal, legacy-row deletion, frozen reports
  axis retirement) is blocked on the production soak window plus the carried-in
  review items (contacts.view_all read/write asymmetry, cross-resource scope
  gating, web nav gating, CAS 409-vs-404).

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
