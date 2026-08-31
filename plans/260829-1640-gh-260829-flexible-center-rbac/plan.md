---
title: "Flexible Center RBAC"
description: "Refactor center-level authorization: code-defined permission catalog, per-center roles (giáo viên, học vụ, trợ giảng + implicit owner), per-member overrides, owner-configurable via UI"
status: in-progress
priority: P1
effort: "5.5d"
tags: [api, web, db, security, authz]
created: 2026-08-29
blockedBy: [260830-2310-resource-action-rbac-permission-catalog]
---

# Flexible Center RBAC

## Overview

Replace the binary `IsOwner`/`CanSendReports` authorization with a configurable
RBAC: a code-defined permission catalog (vocabulary in Go, assignments in DB),
per-center role rows seeded on center creation, and per-member grant/deny
overrides. The center owner configures permissions per role and per member from
the existing "Trung tâm" UI section. Owner stays an implicit superuser outside
the role tables. Defaults reproduce today's behavior exactly (all non-owner
roles = current member behavior), so phase 1 is behavior-identical.

Accepted brainstorm contract + kongming counsel:
`plans/reports/brainstorm-260829-1631-GH-260829-flexible-center-rbac.md`.

## Decisions (accepted 2026-08-29)

- Design B: catalog in code (typed constants), per-center `center_roles` rows
  (3 configurable seeds: giáo viên, học vụ, trợ giảng; "chủ trung tâm" is a
  display-only label for the implicit `centers.owner_id` superuser — never a
  role row).
- Data-scoping axis = single key `data.view_center_wide`, exposed to
  repositories ONLY as `Scope.CenterWide()`; `Has()` is forbidden in
  `*/repository.go`.
- Overrides = grant+deny per member, app-reset on membership reopen (same
  pattern as `can_send_reports` today — `center_members` PK is
  `(teacher_id, center_id)`, rows are reused on rejoin).
- Permission management + ownership handoff stay owner-only forever; NOT
  catalog keys (one-hop escalation risk).
- Defaults: giáo viên/học vụ/trợ giảng all start = current member behavior
  (empty permission sets); owner grants via UI.
- No custom-role CRUD, no policy engine, no JWT caching, no per-domain view
  keys in v1.

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Owner configures permissions per role and per member via UI, no deploy | P1 |
| 2 | Phase 1 lands with zero behavior change and zero existing-test edits | P1 |
| 3 | Revocation/grant takes effect next request (fresh-from-DB invariant kept) | P1 |
| 4 | Repo data-scoping unified behind `Scope.CenterWide()` (no `IsOwner` left in repositories) | P2 |

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Phase 1: Foundation (zero behavior change)](./phase-01-foundation.md) | Completed |
| 2 | [Phase 2: API surface](./phase-02-api-surface.md) | Completed |
| 3 | [Phase 3: Web permission UI](./phase-03-web-permission-ui.md) | Completed |
| 4 | [Phase 4: Cleanup](./phase-04-cleanup.md) | Deployed; e2e follow-up pending |

Dependencies: strictly sequential 1 → 2 → 3 → 4. Phase 4 additionally waits
for phase 3 soak (deployed + verified in use).

**2026-08-31 — Phase 4 implemented under resource-action-rbac phase 8**
(commits 3d6a3cc..602a4cc, branch `teka/260831-0016`): migration 000019
drops `can_send_reports`, down restores from override rows; role-based
`reports.send` live; legacy endpoints/dialog/dual-write removed; role-matrix
restriction lifted. Suites green (build/vet/unit/integration); prod parity
snapshot 0 drift.

**2026-08-31 — Deployed ~11:46.** Migration 000019 applied in prod
(`schema_migrations` version=20 incl. 000020); `can_send_reports` column
verified dropped; pre-migration backup taken and verified. Remaining: run
the isolated e2e (`teka-e2e`) secretary/send-reports Playwright specs — not
exercised this pass (see phase-04 follow-up).

## Success Criteria

- [x] Phase 1 merges with full existing test suite green and unmodified.
- [x] `grep -rn "sc\.IsOwner\|scope\.IsOwner" apps/api/internal/features/*/repository.go` → 0
      hits outside `centers/repository.go` (scope-resolution home; SQL alias
      `is_owner` and `ScopeRow` legitimately live there). Reverified this
      session: 0 hits repo-wide except `centers/repository.go`.
- [x] `grep -rn "\.Has(" apps/api/internal/features/*/repository.go` → 0 hits (guard test).
      Reverified this session: 0 hits; `CenterWide()` used instead across 10
      repository files.
- [x] Integration test: revoke permission → next request 403; grant → next request 200.
      `centers/permissions_integration_test.go` (`TestAuditReadGrantRevokeNextRequest`).
- [x] Membership reopen resets role to default and clears overrides (integration test).
      `centers/rbac_integration_test.go`.
- [x] Owner edits a role's permission set and one member's override via UI; non-owner never sees permission management.
      Phase 3 reviewer + tester confirmed (`center-permissions.test.tsx`,
      `center-page.test.tsx`).
- [x] `/centers/me` returns the caller's effective permission keys (both owner
      and member shapes); member web nav/pages gate on them, so a granted
      permission is usable end-to-end without deploy.
      `centers/dto.go` Perms field + `use-center-context.ts` `has()`; grantee-path
      tests added post-review (dashboard-layout, audit-page, lesson-plans).
- [x] Dual-life constraint holds until phase 4: `reports.send` is assignable
      ONLY as a per-member override (excluded from the role matrix), so the
      legacy column stays in parity.
      `permissions_integration_test.go:120-122` (role-matrix 422),
      `:205-227` (dual-write parity both directions); UI disables the
      `reports.send` role cell (`permission-matrix.tsx`).
- [x] `can_send_reports` column and legacy send-reports endpoints removed (phase 4).
      Migration 000019 + code removal, commit 3d6a3cc; deployed 2026-08-31
      ~11:46, column drop verified in prod.
- [x] Audit events recorded for role assignment, role-permission edit, override edit.
      `centers/events.go` + `service.go` (`RolePermissionsChanged`,
      `MemberRoleChanged`, `MemberOverridesChanged`); `audit/subscriber.go`
      persists them.

## Open Questions

- None blocking. Deferred (recorded in brainstorm report): per-role
  write-on-behalf (assumed no), custom-role CRUD (deferred, `is_system` guard
  in place).

## Validation Log

### Session 1 — 2026-08-29 (Standard tier)

**Verification pass** (~15 claims checked against source):

- Passed: `ReportsOversight()` body + 21 call sites (`authctx.go:55-57`);
  rename gate `centers/service.go:204`; reopen-reset SQL lives in repository
  (`centers/repository.go:275,287` — OpenMembership/CloseMembership);
  owner stint via `cli/create_center.go:111`; notifications mid-run column
  probe (`notifications/repository.go:~328`, `run_manager.go:343`);
  `seeds/seed.go:263` scopeFor reads raw column; fixtures `:140/:172/:202`
  column-based; e2e `secretary-send.spec.ts` exists; migration next number
  000013; `center_members` PK `(teacher_id, center_id)` row-reuse model.
- Failed: 1 — phase-01 catalog cited `members.manage` →
  `centers/service.go:224`, but :224 is the send-reports gate; RemoveMember
  gate is ~:247. **Fixed in phase-01.**
- Nit fixed: `ReportsOversight` cite `:50-55` → `:55-57`.

**Interview decisions** (4 questions):

1. Dual-life `reports.send` restriction (role matrix 422 until phase 4) —
   **accepted** as designed; no change.
2. Member UX for granted perms — **gate member nav/pages on effective perms**
   from `/centers/me`; already in phase 2 (+response shape) and phase 3.
3. Teaching `ReviewQueue` read — **tách key riêng `teaching.review_queue`**
   (NOT bundled into `dashboard.view`): review visibility must be grantable
   without exposing the center-wide financial/attendance dashboard.
   **Propagated:** phase-01 catalog (+1 key, now 9), phase-02 gate swap.
4. Sensitive keys (`audit.read`, `imports.run`, `members.manage`,
   `center.manage`) — **configurable in v1**, full key set; owner-only
   boundary stays at permission mgmt/handoff/write-on-behalf only.

**Consistency sweep:** grep over plan dir for `dashboard.view`,
`teaching.review_queue`, `:239`, `:224` — all references consistent after
propagation; phase 3 nav-gating example keys unaffected; brainstorm report's
"~10 keys" approximation still holds (9 exact). No contradictions between
phases; dependency chain 1→2→3→4 unchanged.

**Post-validation kongming counsel (GO, evidence verified against source):**

1. Phase-1 conflict FIXED: pure perms-recompute of `CanSendReports` would fail
   `centers/secretary_integration_test.go:29` (fixture `Secretary` grants via
   raw column, helper `e.scope()` calls real `ResolveScope`). Phase 1 now
   specifies dual-life OR-read `cm.can_send_reports OR Has(reports.send)`;
   phase 4 switches to perms-only.
2. Phase-3 gap FIXED: `teaching.review_queue` needs `lesson-plans-page.tsx`
   deep-link redirect + `use-review-queue.ts` `enabled: isOwner` swapped to
   the key, plus non-owner action-button hiding; grant recipe = key +
   `data.view_center_wide` for center-wide visibility.
3. Canary for phase 1: `secretary_integration_test.go` +
   `send_reports_integration_test.go:84,95` (reopen reset must clear override
   rows atomically or OR-read resurrects revoked perms).

<!-- slug: gh-260829-flexible-center-rbac -->
