---
title: "Resource-action RBAC permission catalog"
description: "Code-owned CRUD and special-operation policies assigned by center owners."
status: in-progress
priority: P1
effort: "15-17 engineering days plus rollout soak"
tags: [authorization, rbac, api, web, migration, security]
created: 2026-08-30
blockedBy: []
blocks: [260829-1640-gh-260829-flexible-center-rbac, 260830-0938-gh-260830-class-staff-roles-phone-privacy]
---

# Resource-action RBAC permission catalog

## Outcome

Every center-scoped API operation has a code-owned policy. Resources expose `create`, `list`, `read`, `edit`, and `delete` where meaningful; domain commands use explicit special permissions. Owners assign only grantable catalog entries to roles through an accessible UI. Owner bypass remains, while tenant scope, object visibility, and class capability checks stay independent.

## Accepted contract

- Stable keys use `<resource>.<action>`; server code owns definitions and the database stores assignments only.
- Seed canonical defaults and provide assignment UI; users never create permission definitions.
- Preserve `(role permissions ∪ member grants) − member denies`, owner bypass, and class-staff capabilities.
- Definitions contain key, resource, action, kind, label, description, risk, grantable, deprecated, and order.
- Atomic writes require a catalog/assignment version and reject stale, unknown, deprecated, or non-grantable entries.
- Roll out additively with explicit aliases, symmetric grant/deny backfill, mixed-version tests, soak, then cleanup.
- Catalog keys use only canonical CRUD verbs (`create`, `list`, `read`, `edit`, `delete`), scope keys (`<resource>.view_all`), and named specials; a registry guard forbids any catalog key equal to a class-staff capability string (`attendance.write`, `scores.write`, `sessions.write`, `statement.send`).
- `data.view_center_wide` decomposes into per-resource `<resource>.view_all` scope-kind keys; legacy holders backfill symmetrically (grants and denies) to the full per-resource set.
- Rollout machinery is sized for the real single-host docker-compose production topology: keep stale-web-client protection, catalog/assignment versioning, and rollback; drop multi-instance fleet-convergence gates, per-wave policy epochs, rolling-restart matrices, and cohort monitoring.

## Scope and boundaries

Core resources: classes, contacts, students, enrollments, sessions, schedules, scores, teaching assignments, billing records, and payments. Specials: archive/restore, attendance confirmation, lifecycle transitions, billing close/reopen, payment reversal, statements, import/export, notifications, and audit/report access. Public-token and authenticated-self routes are explicit exemptions. Permission/role administration, staffing/handoff, sensitive review writes, and unsafe financial commands remain non-grantable owner-only operations unless Phase 1 explicitly reclassifies them.

## Phases

| # | Phase | Depends on | Estimate |
|---|---|---|---|
| 1 | [Inventory and compatibility contract](./phase-01-start.md) | — | 1.5d |
| 2 | [Structured catalog and route-policy foundation](./phase-02-structured-catalog-and-route-policy-foundation.md) | 1 | 2d |
| 3 | [Compatibility migration and role defaults](./phase-03-compatibility-migration-and-role-defaults.md) | 1, 2 | 2d |
| 4 | [CRUD endpoint policy cutover](./phase-04-crud-endpoint-policy-cutover.md) | 2, 3 | 4d |
| 5 | [Special-operation policy cutover](./phase-05-special-operation-policy-cutover.md) | 2, 3, 4 | 2.5d |
| 6 | [Owner permission API and responsive UI](./phase-06-owner-permission-api-and-responsive-ui.md) | 2, 3 | 2d |
| 7 | [Security verification and rollout](./phase-07-security-verification-and-rollout.md) | 4, 5, 6 | 1.5d |
| 8 | [Legacy compatibility cleanup](./phase-08-legacy-compatibility-cleanup.md) | 7 + soak | 1d |

## Global verification matrix

For every protected route family automate: unauthenticated `401`; missing permission `403`; owner, allowed role, and member-grant success; member deny overriding role/grant; wrong-center denial; non-leaking hidden/missing-object behavior; and class assignment/capability denial where applicable. Registry coverage fails whenever a route is not deliberately public, self, owner-only, or permission-protected.

## Cross-plan coordination

This plan supersedes only pending legacy-authorization cleanup in `260829-1640-gh-260829-flexible-center-rbac` and gates only Phase 5 cleanup in `260830-0938-gh-260830-class-staff-roles-phone-privacy`. Completed work remains valid; class capabilities must not be removed. Reserve the next migration number from master at implementation time.

## Success criteria

- [x] Every route has an intentional policy classification and coverage fails closed.
- [x] Existing centers retain equivalent effective access after backfill, including denies.
- [x] Owners manage assignments on desktop/mobile but cannot invent keys or grant locked operations.
- [x] Stale clients cannot erase unseen permissions.
- [x] Every special has a dedicated policy or explicit owner-only/exempt classification.
- [x] Tenant, object, and class constraints remain independent.
- [x] Mixed-version deploy, rollback, audit, and cleanup gates are tested and documented.

## Progress

**2026-08-31 — Phases 1–7 done; Phase 8 blocked on soak.** Catalog
(`CatalogVersion 2`, 73 non-deprecated entries), route-policy registry with
fail-closed coverage, additive migrations + symmetric legacy backfill
(incl. `data.view_center_wide` → per-resource `view_all`), CRUD and
special-operation cutover, owner permission API + responsive grouped UI with
CAS/409 flow and high-risk confirmation, and the security-verification
matrix are all implemented and green: `make test-api` full integration
suite passes repo-wide (coverage 75.9%, floor 60%), `make test-api-unit`,
`make lint`, web typecheck + 456 Vitest tests. OpenAPI regenerated
additively. Deployment (which starts the soak window) follows via the
deployment runbook; Phase 8 legacy retirement stays blocked until soak
completes — see `phase-08-legacy-compatibility-cleanup.md`. The frozen
reports axis (`ReportsOversight`/`CanSendReports`) is untouched by design
and retires in Phase 8.

## Risks

Atomic replacement can erase unknown keys, grant-only expansion breaks deny precedence, broad aliases can grant destructive commands, middleware can obscure lost repository scope, and high-risk UI labels can hide impact. Version writes, map grants and denies explicitly, retain service tests, and require descriptions plus affected-member confirmation. Decomposing `data.view_center_wide` rewrites repository scoping per resource: any resource missed during cutover either leaks center-wide data or silently narrows a legacy holder's visibility — parity tests must cover both directions per resource.

## Unresolved questions

None blocking. Phase 1 freezes exact keys and owner-only classifications before enforcement changes.

## Validation Log

### Session 1 — 2026-08-30
**Trigger:** `/ak:plan validate` (scoped) after `--advise` kongming counsel: GO_WITH_CONCERNS with three product forks requiring user decisions before Phase 1 freezes keys.

### Verification Results
- Claims checked: 10 (scoped; kongming advisory pass had already verified core anchors with file:line evidence)
- Verified: 10 | Failed: 0 | Unverified: 0
- Tier: Standard (scoped by user direction; red-team hardening already embedded in phases 2, 3, 5, 6, 7)
- Failures: none. Evidence: 9 coarse keys and `BuildPermSet = (role ∪ grants) − denies` at `apps/api/internal/shared/authctx/permissions.go:92-108`, owner bypass at `permissions.go:122`, `data.view_center_wide` as single data-scoping axis at `permissions.go:13-16`, class capability strings at `class_staff.go:67-73`, 22 `routes.go` feature files, latest migration `000017` (plan hardcodes no number), web surfaces under `apps/web/src/features/center`.

**Questions asked:** 6 (4 + 2 clarification/confirmation)

#### Questions & Answers

1. **[Scope]** Right-size rollout machinery for single-host docker-compose production (drop multi-instance fleet steps)?
   - Options: Cut fleet machinery, keep stale-client protection + version/CAS + rollback (Recommended) | Keep full machinery | Keep simple flag, cut the rest
   - **Answer:** Cut fleet machinery (confirmed on simplified re-ask after initial question was unclear)
   - **Rationale:** Production is one host; two API binaries never run concurrently. Stale browser tabs remain the only real mixed-version surface. Saves ~2 days without reducing safety. Partially reverses accepted red-team items in phases 3/7 — reversal decided by user, not agent.

2. **[Architecture]** Catalog granularity: expand 9 coarse keys to ~50-60 resource-action keys?
   - Options: Full CRUD as contracted (Recommended) | Merge list+read into view | Merge create+edit+delete into manage
   - **Answer:** Full CRUD as contracted
   - **Rationale:** Keeps "list does not imply read" and per-verb delegation — the core value of the plan. Backfill design already targets this granularity.

3. **[Assumptions]** Namespace collision between catalog keys and class-staff capability strings (`attendance.write` etc.)?
   - Options: Catalog uses canonical verbs only + registry guard test (Recommended) | Prefix capabilities as `class.*` | Prefix catalog as `perm.*`
   - **Answer:** Catalog uses canonical verbs + guard test
   - **Rationale:** Zero stored-data migration; a registry test bans any catalog key equal to a capability string, keeping the two vocabularies mechanically disjoint.

4. **[Architecture]** Where does `data.view_center_wide` live in the new catalog?
   - Options: Keep as single scope-kind key (Recommended) | Decompose into per-resource list scopes | Let Phase 1 decide
   - **Answer:** Decompose into per-resource `<resource>.view_all` scope keys (confirmed on explicit re-ask including the +2-4 day repository-scoping cost)
   - **Rationale:** User wants per-resource visibility control despite the cost. Legacy `data.view_center_wide` grants/denies backfill symmetrically to the full per-resource set so effective access is unchanged at cutover.

#### Confirmed Decisions
- Rollout machinery: single-host right-size — keep stale-client + version/CAS + rollback; drop fleet-convergence, per-wave epochs, rolling-restart matrix, cohort monitoring.
- Catalog granularity: full CRUD verbs as contracted; Phase 1 may still omit meaningless actions per resource.
- Namespace: canonical verbs + scope keys + specials only; registry guard test bans capability-string collisions.
- Scope key: `data.view_center_wide` → per-resource `<resource>.view_all` (kind=scope), symmetric backfill, repository scoping cutover per resource.

#### Action Items
- [x] Update contract, risks, effort (15-17d), and Phase 4 estimate in `plan.md`.
- [x] Propagate decisions 1, 3, 4 into phases 1, 2, 3, 4, 6, 7, 8.

#### Impact on Phases
- Phase 1: inventory per-resource scoped visibility; freeze `view_all` keys and the namespace convention.
- Phase 2: add scope kind to catalog; registry guard test against capability strings.
- Phase 3: backfill `data.view_center_wide` → per-resource `view_all` grants and denies; simplify writer-compatibility step to single-host deploy ordering.
- Phase 4: repository scoping cutover honoring per-resource `view_all` (estimate 2.5d → 4d).
- Phase 6: UI groups per-resource scope keys with high-risk labeling.
- Phase 7: replace per-wave epoch/fleet gates with a single enforcement flag; drop rolling-restart and cohort monitoring; verify per-resource scope leak-freedom.
- Phase 8: retire legacy `data.view_center_wide` rows only after parity soak.

### Post-Validation Kongming Review
Final advisory pass on the propagated plan returned GO for `/ak:cook` with three additive patches, all applied:
- Pre-cutover revocation gap: while a resource's repository still honors legacy `data.view_center_wide` (Phase 3 → its Phase 4 wave), new-API scope-key writes dual-write the legacy row symmetrically so a UI revocation is never ineffective (phase-06).
- 1:N alias semantics: legacy grant → all N canonical grants, legacy deny → all N canonical denies, and a single-canonical deny never propagates back through the legacy key (phase-02).
- Phase 1: multi-resource aggregate endpoints get one explicit key (never composed from multiple `view_all`), and access fixtures include the class-staff assignment dimension (active + ended) since plan 260830-0938 moved attendance/grading/sessions/enrollments to assignment-based scoping this week.
Top implementation watch item: Phase 1 freezing a wrong per-resource visibility baseline for the resources plan 0938 just changed.

### Whole-Plan Consistency Sweep
Swept all plan files for `fleet`, `rolling restart`, `epoch`, `cohort`, `view_center_wide`, and stale estimates after propagation. One stale line found and fixed (phase-08 fleet-convergence wording → single-host). Remaining `view_center_wide` mentions are intentional decomposition/backfill/cleanup references; phase-03's "locked epoch/CAS transaction" is a database write-gate epoch, unrelated to the removed fleet policy epochs. No unresolved contradictions.

<!-- slug: resource-action-rbac-permission-catalog -->
