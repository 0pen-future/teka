---
title: "Phase 2: Structured catalog and route-policy foundation"
status: done
estimate: "2 days"
dependsOn: [1]
---
# Phase 2: Structured catalog and route-policy foundation

Primary surfaces: `apps/api/internal/shared/authctx/permissions.go`, `class_staff.go`, shared HTTP/auth middleware, center permission API, and tests.

## Tasks
- [x] Add immutable structured definitions with deterministic ordering and compatibility helpers.
- [x] Preserve owner bypass and `(role ∪ grants) − denies`; prove deny precedence for canonical/aliased keys.
- [x] Add typed public, self, owner-only, and permission policies plus `RequirePermission`.
- [x] Retain center resolution, object visibility, and class capability checks after permission evaluation.
- [x] Add explicit aliases; unknown keys remain ineffective and observable.
- [x] Reject unknown, deprecated-for-assignment, and non-grantable keys with field errors.
- [x] Extend catalog output additively so current `key`/`label` clients parse it.
- [x] Test unclassified/duplicate routes, invalid/duplicate keys, and missing special metadata.
- [x] Add `scope` kind for `<resource>.view_all` keys; add a registry guard test failing on any catalog key equal to a class-staff capability string.
<!-- Updated: Validation Session 1 - scope kind and capability-collision guard -->


## Acceptance and verification
- [x] Enforcement is unchanged before cutover; serialization is deterministic and validation fails closed.
- [x] An unclassified route deterministically fails tests.
- [x] Run authctx, middleware, permission API, registry, race, lint, and type checks.

## Red-team hardening requirements
- [x] Hard owner-only routes use `OwnerOnly`, never a grantable policy; effective-set construction drops non-grantable rows. Test forged raw-DB rows.
- [x] Canonicalize role grants, member grants, and denies across each alias equivalence class before set algebra; any equivalent deny wins.
- [x] Specify 1:N alias semantics for `data.view_center_wide` → per-resource `view_all`: a legacy grant resolves to all N canonical grants, a legacy deny to all N canonical denies, and a deny of one canonical key never propagates back through the legacy key to affect the other N−1 resources.
<!-- Updated: Validation Session 1 (post-validation kongming review) - 1:N alias resolution semantics -->

- [x] One route-registration helper both installs correctly ordered auth/scope/policy middleware and records metadata; compare registry bidirectionally with `engine.Routes()`.
  <!-- Delivered in Phase 4: registerFeatures builds one ordered authChain (RequireAuth → ResolveScope → enforceRoutePolicy) and every feature RegisterRoutes mounts it variadically; internal/server/route_policy_test.go keeps the bidirectional registry↔engine.Routes() comparison. -->
- [x] Require auth → fresh membership/scope → permission ordering; test expired auth, removed membership, refreshed role, and deny.
  <!-- internal/server/policy_integration_test.go: expired token 401, removed membership non-2xx next request, role replacement immediate 403, member deny beats role grant. -->
- [x] Define an authorized service/command boundary for CRUD so internal callers cannot bypass HTTP middleware; add direct-call denial tests.
