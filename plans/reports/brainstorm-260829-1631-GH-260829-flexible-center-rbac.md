# Brainstorm — Flexible center-level RBAC

Date: 2026-08-29. Status: ACCEPTED by user (design B, học vụ defaults = plain member, permission mgmt owner-only). Advisory: kongming counsel received (approach confirmed with amendments).

## Contract

- **Outcome:** Center-level authorization refactor. 4 roles: chủ trung tâm (owner), giáo viên, học vụ, trợ giảng. Owner configures permissions per role AND per user via UI menu in center section. API + DB change accordingly. Concrete per-role permission sets not yet product-defined → system must be configurable, defaults reproduce today's behavior.
- **Constraints:** Keep fresh-from-DB scope resolution (no JWT caching); stint-based membership semantics (reopen resets grants); append-only migrations; tenancy `scoped()` pattern per docs/api-guidelines.md; owner never lockout-able.
- **Non-goals:** No change to `user_accounts.role` (app-surface routing only); no custom-role CRUD in v1; no policy engine (Casbin/OPA); no per-domain fine-grained view keys; no write-on-behalf expansion; permission-management itself stays owner-only (NOT a configurable key — one-hop escalation risk).
- **Acceptance:**
  - Phase 1 merges with zero existing-test modifications (behavior-identical).
  - Owner can change a role's permission set and one member's override via UI without deploy.
  - Revocation takes effect on next request (integration test).
  - `IsOwner` gone from repository data-scoping (`grep IsOwner internal/features/*/repository.go` ≈ 0, replaced by `Scope.CenterWide()`).

## Current state (verified)

- Binary authz: `Scope{TeacherID, CenterID, IsOwner, CanSendReports}` resolved per request by `middleware.ResolveScope`. ~76 non-test `IsOwner` sites: (a) repo data-scoping owner-sees-all vs member-sees-own; (b) ~26 capability gates (invitations, center mgmt, audit, imports, handoff, dashboard). 21 `ReportsOversight()` sites.
- `center_members` stints + `can_send_reports` bool (migration 000011) = first ad-hoc permission flag.
- Owner = `centers.owner_id`, implicit superuser, may have no live stint (LEFT JOIN in centers/repository.go).

## Options

- **A. Static role enum + hardcoded checks** — fails "owner configures" requirement. Rejected.
- **B. Code-defined permission catalog + per-center role rows + role→permission mapping + per-stint overrides** — RECOMMENDED. Vocabulary in code (typed constants, ~10 coarse keys derived from existing gates), assignments in DB.
- **C. Policy engine (Casbin/OPA)** — flat single-tenant-level model doesn't need it; would fight the fresh-resolution invariant. Rejected (YAGNI).

## Chosen design (B + amendments)

1. **Catalog in code** (~10 keys from existing gates): `reports.send`, `invitations.manage`, `members.manage`, `center.manage`, `audit.read`, `imports.run`, `dashboard.view`, `data.view_center_wide`, + billing/payments gate reductions. Unknown DB keys ignored on read; typed Go constants; registration test.
2. **Data-scoping axis = one key `data.view_center_wide`, never string-checked in repos.** Precompute `Scope.CenterWide() = IsOwner || has(key)`; repos use only that boolean. Forbid `Has()` in `*/repository.go` (guideline + grep test). Write-on-behalf stays `IsOwner`-only.
3. **Tables:** `center_roles(id, center_id, key, name, is_system)` seeded 4/center; `center_role_permissions(role_id, permission_key)`; `center_members.role_id`; `member_permission_overrides(stint FK, permission_key, allowed bool)`. Resolution: owner bypass → else `(role ∪ grants) − denies`. Overrides + role reset on stint reopen (stint FK gives orphaning; app resets role to default like can_send_reports today).
4. **Owner stays implicit superuser** outside role tables (`centers.owner_id`). `Has()` returns true unconditionally for owner. UI may display "chủ trung tâm" as a role without it being a row-assignment.
5. **Roles are per-center rows, no CRUD endpoints v1** (`is_system=true` protects seeds; future 5th/custom role = INSERT not migration).
6. **`ResolveScope`** grows to one aggregated query (roles + perms + overrides, array_agg). No caching.
7. **Defaults (user decision 2026-08-29):** giáo viên/học vụ/trợ giảng ALL start = current member behavior (no extra grants); owner grants more via UI later; owner = bypass.

## Phases

1. **Foundation, zero behavior change:** migrations + backfill (4 roles per existing center, default role_permissions, role_id on live stints, live `can_send_reports=TRUE` → `reports.send` grant-override rows); catalog constants; aggregated ResolveScope; `Has()`/`CenterWide()`; `ReportsOversight()` reimplemented on top; repo `!IsOwner` → `!CenterWide()` swap; existing grant endpoints write override rows. Seed roles in ALL center-creation paths: centers service, cli/create_center.go, seeds/seed.go. Freeze catalog at end of phase.
2. **API surface:** ~26 capability gates → `Has(perm)`; owner-only endpoints (list catalog+roles, PUT role permissions, PUT member role, PUT member overrides); audit events via event bus; differentiate 4 default sets.
3. **Web:** center section — role select in member list + permission matrix; fold send-reports dialog in.
4. **Cleanup:** drop `can_send_reports` column + legacy endpoints (new migration, after soak).

## Risks (ranked)

1. Escalation via configurable meta-permission → permission mgmt owner-only, decided in writing.
2. String checks creeping into repos → CenterWide()-only rule + grep test.
3. Fresh-resolution regression → single round-trip, revocation integration test.
4. Backfill/seeding misses CLI path.
5. Stint-reopen reset semantics need integration test.
6. `can_send_reports` dual life until phase 4; column drop irreversible in append-only chain — verify first.
7. Scope stored by value in gin context — permission set must be read-only after SetScope (document).

## Resolved decisions (user, 2026-08-29)

1. Design B accepted → proceed to plan.
2. Học vụ default = plain member (NOT center-wide); owner grants `data.view_center_wide` via UI when needed.
3. Permission management stays owner-only in v1; not a catalog key.

## Unresolved questions

1. Any per-role write-on-behalf need now? (assumed no — owner-only)
2. Custom-role CRUD permanently out or just deferred? (assumed deferred)
