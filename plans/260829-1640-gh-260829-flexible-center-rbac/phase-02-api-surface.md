---
phase: 2
title: "API surface"
status: completed
priority: P1
effort: "1.5d"
dependencies: [1]
---

# Phase 2: API surface

## Overview

Swap the capability-gate `IsOwner` sites to `Has(perm)` (behavior-identical:
only the owner holds anything, via bypass) and expose owner-only permission
management endpoints with audit events.

## Requirements

- Functional: capability gates use catalog keys; owner-only CRUD over role
  permissions, member role assignment, member overrides; audit events for
  every permission mutation.
- Non-functional: permission management endpoints hard-gated on
  `scope.IsOwner` (NOT a catalog key); response envelope + error conventions
  per `docs/api-guidelines.md`; swag annotations → `make api-docs`.

## Architecture

### Gate swap (behavior-identical)

Per the phase-1 catalog table: replace `if !sc.IsOwner` at capability sites
with `if !sc.Has(authctx.Perm…)` in: centers service (rename→`center.manage`,
removeMember→`members.manage`), dashboard (`dashboard.view`), audit
(`audit.read`), imports (`imports.run`), invitations
(`invitations.manage`), teaching `ReviewQueue` read only
(`teaching/service.go:239` → `teaching.review_queue`, its own key by user
decision).
<!-- Updated: Validation Session 1 (2026-08-29) — ReviewQueue swap targets
     teaching.review_queue instead of dashboard.view -->
Keep `IsOwner` at: handoff,
permission management, any write-on-behalf branch, and teaching
`resolveReviewedClass` (`~:504` — verified write gate for
approve/redo/reopen; see phase 1 catalog). `Has()` returns true for owner, so
no test changes expected beyond new coverage.

### Endpoints (extend `centers` feature — no new feature package)

Mounted in `centers/routes.go` under the existing group:

| Method/Path | Purpose |
|-------------|---------|
| `GET /centers/me/permissions` | Catalog (keys + vi labels), roles with their permission sets, members with role + overrides. Owner-only. |
| `PUT /centers/me/roles/:roleId/permissions` | Replace a role's permission set (validated against registry). |
| `PUT /centers/me/members/:teacherId/role` | Assign member's role (must belong to caller's center). |
| `PUT /centers/me/members/:teacherId/overrides` | Replace member's grant/deny override list. |

Rules: reject unknown permission keys (422 validation error); reject targeting
the owner (mirrors existing "grant refuses the owner" precedent); role/member
must resolve inside `scope.CenterID` (never trust path segment for tenancy —
path id is the target, tenancy comes from scope). PUT-replace semantics keep
the API small (no per-key PATCH endpoints).

**Dual-life constraint (until phase 4 drops the column):**

- `PUT …/roles/:roleId/permissions` REJECTS `reports.send` (422) —
  `reports.send` is assignable only per-member while the legacy
  `can_send_reports` column lives, because a role-held grant cannot be
  mirrored into a per-member column and would poison the phase-4 parity gate.
- `PUT …/members/:teacherId/overrides` dual-writes the legacy column whenever
  the override set adds/removes `reports.send` (same transaction), exactly
  like the legacy grant/revoke endpoints do in phase 1.
- Phase 4 lifts the role-matrix restriction after the column drop.

**`/centers/me` effective permissions:** add the caller's effective permission
key array to BOTH response shapes (owner and member) so the web app can gate
nav/pages for members. Note for phase 4: the JSON contract field
`can_send_reports` (centers/dto.go) survives the column drop — it becomes
computed from perms; only the DB column dies.

### Audit events

Publish on the shared event bus (`internal/shared/events`, pattern:
`invitations/service.go:124`) for: role-permission set changed, member role
changed, member overrides changed. Audit subscriber persists them like
existing actions (see `audit/model.go` Action). Include actor, target,
before/after key sets.

### Vietnamese labels

Catalog key → vi display label map lives next to the constants (single source
for API response; web renders what API returns, no duplicated label map in
TS).

## Related Code Files

- Modify: `apps/api/internal/features/centers/{routes,handler,service,repository,dto}.go`,
  `apps/api/internal/features/{audit,imports,invitations,teaching}/service.go`,
  `apps/api/internal/features/centers/dashboard.go`,
  `apps/api/internal/shared/authctx/permissions.go` (labels),
  audit subscriber for new actions
- Create: integration tests `centers/permissions_integration_test.go`
- Docs: `docs/api-guidelines.md` Tenancy section — add the two invariants:
  owner bypass, `CenterWide()`-only in repositories; permission mgmt owner-only.

## Implementation Steps

1. Swap capability gates to `Has()`; verify suite green unchanged.
2. DTOs + validation (registry-checked keys) + repository queries.
3. Handlers + routes, owner-only guard, swag annotations, `make api-docs`.
4. Audit events + subscriber actions.
5. Integration tests: non-owner 403 on all 4 endpoints; owner happy paths;
   unknown key 422; owner-as-target rejected; cross-center target 404;
   grant→next-request effect through a real gated endpoint; `reports.send` on
   role matrix 422; overrides with `reports.send` keep column in parity;
   `/centers/me` returns effective perms for a granted member.
6. Update `docs/api-guidelines.md`.

## Success Criteria

- [x] All 4 endpoints owner-only, validated, documented in generated swagger
      (`make api-docs` regenerated; owner-only pinned by
      `TestPermissionManagementIsOwnerOnly`).
- [x] A member granted `audit.read` via role or override can read audit log on
      the next request; revocation flips it back
      (`TestAuditReadGrantRevokeNextRequest` drives the real
      `audit.Service.List` gate in both directions).
- [x] Audit log records every permission mutation with actor + diff
      (middleware actions-map rows for the 3 PUT routes + service events
      `centers.{RolePermissionsChanged,MemberRoleChanged,MemberOverridesChanged}`
      with before/after sets, published after commit).
- [x] No remaining capability-gate `IsOwner` outside handoff/permission-mgmt/
      write-on-behalf (grep review 2026-08-29: remaining sites are
      handoff/service.go:100, teaching/service.go:506 write gate,
      centers Me() shape switch, SetSendReports, requirePermissionAdmin —
      all in the allowed list).

## Risk Assessment

- **Escalation via mgmt endpoints** → hard `IsOwner` gate + owner-target
  rejection tests.
- **Gate swap flips a write path** → phase-1 verification note on teaching
  sites; review each swapped site's handler for mutations before choosing key.
- **Swagger drift** → `make api-docs` is generated; never hand-edit.
