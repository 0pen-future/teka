---
phase: 1
title: "Permission model & scope resolution"
status: done
priority: P1
effort: "1.5d"
dependencies: []
---

# Phase 1: Permission model & scope resolution

## Overview

Add the `can_send_reports` membership permission (DB + authctx.Scope), the
owner-only grant/revoke endpoints, and their audit actions. Pure additive
backend foundation — no send-path behavior changes yet.

## Requirements

- Functional: owner can grant/revoke `can_send_reports` for an active non-owner
  member; flag is resolved into request scope on every request; `/centers/me`
  exposes the flag (owner sees it per member; a member sees their own). Owner
  can NEVER hold the flag (member-only — user decision resolving the D2
  ambiguity).
- Non-functional: backward compatible — flag defaults `false`, no existing
  behavior changes; scope resolution stays a single query; grant and revoke
  produce distinguishable audit rows.

## Architecture

- **Storage:** boolean column on `center_members` (composite PK
  `(teacher_id, center_id)`, active row = `left_at IS NULL` — see
  `apps/api/migrations/000007_centers.up.sql:36-78`). A boolean flag, not a
  role enum: brainstorm non-goal rules out a general role framework (YAGNI),
  and V1 issues only `teachers` accounts, so no JWT/role change.
- **Membership lifecycle (permission must NOT survive a rejoin):**
  `OpenMembership` is an upsert `ON CONFLICT (teacher_id, center_id) DO UPDATE
  SET left_at = NULL, joined_at = now()` (`centers/repository.go:255-266`) —
  it reuses the old row, so a flag granted in a previous stint would silently
  resurrect on re-invite. Add `can_send_reports = FALSE` to the `DO UPDATE SET`
  list, and also reset it in `CloseMembership` (`repository.go:268-280`) as
  defence in depth. Test: grant → close → open → ResolveScope returns false.
- **Scope:** extend `authctx.Scope` (`apps/api/internal/shared/authctx/authctx.go:38-43`)
  with `CanSendReports bool` and helper
  `func (s Scope) ReportsOversight() bool { return s.IsOwner || s.CanSendReports }`.
  The helper serves double duty (plan.md D3 + D8): read paths in later phases
  branch on `ReportsOversight()`, and Phase 2 reuses the SAME helper as the
  send-creation gate (only owner or secretary may send, any channel). The
  cross-teacher personal-send 409 branches on `CanSendReports` only (owner
  behavior unchanged, see plan.md decision D2).
- **Resolution:** `centers/repository.go:153-169 ResolveScope` gains
  `LEFT JOIN center_members cm ON cm.teacher_id = t.id AND cm.center_id = t.center_id AND cm.left_at IS NULL`
  selecting `COALESCE(cm.can_send_reports, false)`. The same SQL exists in two
  more copies that must stay in sync: `apps/api/internal/testutil/fixtures.go:140-158
  ScopeFor` and `apps/api/seeds/seed.go:169-187 scopeFor` — update both.
  Other places construct `Scope` literals directly (≈19 sites, mostly tests and
  derived scopes like `periodScope`); they zero-value `CanSendReports=false`,
  which is the safe default — no change needed, but note it in the PR.
- **Grant/revoke — two routes, no body:**
  `POST /centers/me/members/:teacherId/send-reports` (grant) and
  `DELETE /centers/me/members/:teacherId/send-reports` (revoke), in the
  existing centers feature module, owner-only like removeMember
  (`centers/handler.go:99`, `centers/routes.go:10` — note the existing wildcard
  is `:teacherId`; gin panics on conflicting wildcard names for the same
  segment, so the new routes MUST reuse `:teacherId` exactly). Two verb routes
  instead of a PATCH-with-body: no `*bool` binding pitfalls (a bare bool would
  silently treat `{}` as revoke), and the audit action string distinguishes
  grant from revoke for free (see Audit). Target must be an active, non-owner
  member; otherwise the neutral not-found error per tenancy convention — this
  also enforces the "owner can never hold the flag" decision.
- **Audit:** the repo convention for mutating routes is an entry in the action
  registry, NOT a bus event — `audit/action.go:14-30` ("when adding a mutating
  route, add its entry here"); the middleware auto-publishes request completion
  and the subscriber persists actor from request context
  (`middleware/request_events.go:88-133`, `audit/subscriber.go:226-256`; no
  request body is stored, which is why split routes are needed to tell grant
  from revoke). Add two entries:
  `POST /api/v1/centers/me/members/:teacherId/send-reports` →
  `center.member.send_reports_grant` and the DELETE twin →
  `center.member.send_reports_revoke` (naming after the existing
  `center.member.remove` pattern). Do NOT publish a manual event — that path is
  reserved for auth/invitation events and would double-log.

## Related Code Files

- Create: `apps/api/migrations/000011_center_member_send_reports.up.sql`,
  `apps/api/migrations/000011_center_member_send_reports.down.sql`
- Modify: `apps/api/internal/shared/authctx/authctx.go`
- Modify: `apps/api/internal/features/centers/repository.go` (ResolveScope,
  OpenMembership, CloseMembership, permission update), `service.go`,
  `handler.go`, `routes.go`, `dto.go`
- Modify: `apps/api/internal/features/audit/action.go` (two registry entries)
- Modify: `apps/api/internal/testutil/fixtures.go` (ScopeFor query),
  `apps/api/seeds/seed.go` (scopeFor query)
- Modify: `apps/api/internal/features/centers/*_test.go` +
  `*_integration_test.go`

## Implementation Steps

1. Write migration pair 000011: `ALTER TABLE center_members ADD COLUMN
   can_send_reports BOOLEAN NOT NULL DEFAULT FALSE;` / drop column. Run
   `migrations_test.go`.
2. Extend `authctx.Scope` + `ReportsOversight()` helper; update ResolveScope
   query and scan, plus the testutil and seeds copies of that query.
3. Reset the flag in `OpenMembership`'s `DO UPDATE SET` and in
   `CloseMembership`.
4. Add `SetSendReports(centerID, teacherID, enabled)` repo+service:
   `UPDATE center_members SET can_send_reports = $3 WHERE teacher_id = $2 AND
   center_id = $1 AND left_at IS NULL AND teacher_id <> (owner)` guarded by
   IsOwner; return not-found when no row updated.
5. Add POST/DELETE handlers + routes behind `requireAuth, resolveScope`; swag
   annotations; regenerate docs via `make api-docs` (never hand-edit
   `apps/api/docs/`).
6. Register both routes in `audit/action.go`; integration test asserts two
   distinguishable audit rows (grant then revoke) with the owner as actor.
7. Extend `/centers/me` DTOs: owner member rows and member self shape gain
   `can_send_reports`.

## Todo

- [x] Migration 000011 up/down + migrations test green
- [x] Scope carries CanSendReports; ResolveScope integration test (flag on/off,
      left member resolves false)
- [x] Rejoin does not resurrect the flag: grant → close → open → false
- [x] POST/DELETE endpoints: owner 204; member caller → 403 Forbidden
      (matches the removeMember/rename owner-only convention, overriding this
      todo's original "neutral not-found" wording); target = owner, left
      member, or stranger → neutral 404
- [x] `/centers/me` exposes flag in both shapes
- [x] Audit rows distinguish grant vs revoke, owner as actor
- [x] testutil `ScopeFor` + seeds `scopeFor` return the flag
- [x] `make test-api-unit` green; centers integration tests green

## Success Criteria

- [x] Owner can toggle the permission via API; member sees own flag in
      `/centers/me`
- [x] All existing tests pass unchanged (default-false compatibility)

## Risk Assessment

- ResolveScope is on every request's hot path — keep the join LEFT and
  covered by the existing `uq_center_members_active` partial index; verify
  query plan doesn't regress.
- Rollback: the down migration drops the column and is schema-safe, but it is
  one-way for DATA — once delegated sends have happened, notification/run rows
  carry the secretary's `teacher_id` on another teacher's period. Phase 2's
  period-scoped ledger reads keep those rows visible to the period's teacher
  even after a revoke/rollback, so no invisible-history resend hazard remains.
