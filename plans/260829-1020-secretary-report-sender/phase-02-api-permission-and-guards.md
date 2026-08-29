---
phase: 2
title: "API read oversight & delegated send path"
status: done
priority: P1
effort: "3d"
dependencies: [1]
---

# Phase 2: API read oversight & delegated send path

## Overview

Let a `can_send_reports` holder read periods/statements/debt center-wide and
run `zalo_personal` bulk sends for other teachers' periods from her own Zalo,
with attribution to her. Make send creation exclusive to owner + capability
holder (plan.md D8): plain members are denied on every channel, own periods
included. Keep every write surface and the owner's current behavior untouched.

## Requirements

- Functional: capability holder can list billing periods (with the owning
  teacher's identity), list/get statements, read period figures/debt
  (collections), read contacts' Zalo-mapping status, run statement/reminder
  bulk sends (all channels) for any center member's period, and list
  notifications/runs. Notification + run rows stamp HER `TeacherID`. A
  server-side pre-send preview returns the exact send buckets.
- Functional (D8): a plain member calling BulkSend or ResumeRun — any channel,
  even their own period — gets an explicit 403-style denial; `markSent` and
  ledger reads keep working on existing rows.
- Non-functional: no write access anywhere new (attendance, classes, billing
  config, contacts, membership, imports, dashboard, audit stay as-is); owner
  and plain-member READS behave exactly as before (member send removal is the
  one intentional breaking change); no period can ever have two concurrent
  personal-send runs.

## Architecture

- **Read oversight — dedicated read helpers, never the shared `scoped()`:**
  the existing `scoped()` helpers back BOTH reads and writes, so relaxing them
  would hand the secretary write powers:
  - `statements/repository.go:204-210 scoped()` also backs `Revoke`
    (`:363-386`) — revoking kills a parent's link permanently
  - `billing/repository.go:265-280 scoped()/invoiceScoped()` also back
    close/void/adjustments
  - `contacts/repository.go:62-71 scoped()` also backs `SoftDelete` and
    `setZaloMapping`
  Instead, add a per-feature read-only helper (e.g. `scopedRead`) that filters
  `center_id` always and `teacher_id` unless `sc.ReportsOversight()`, and wire
  it ONLY into: statements List/Get/PeriodFigures
  (`statements/service.go:141-163`), billing periods List/Get, collections
  reads backing debt/balances (`collections/repository.go:50-99` — this
  feature was previously missing from the cluster; Goal 2 "read debt" lives
  here, and the web dialog's data comes from it), contacts roster read (the
  list endpoint only), and `notifications/repository.go ZaloMappings`
  (`:337-350`). Every write keeps the original `scoped()` / `!sc.IsOwner`
  guard. Do NOT touch dashboard (`centers/dashboard.go:57` `if !sc.IsOwner`),
  audit routes, invitations.
- **Ledger reads become period-scoped:** `ListByPeriod`, `LatestRunByPeriod`
  and `RunSnapshot` currently filter `n.teacher_id = sc.TeacherID`
  (`notifications/repository.go:83-88,192-205`), so after a delegated send the
  period's own teacher would see an EMPTY ledger and resend → duplicate DMs.
  Rescope them by period ownership: visible when the period belongs to
  `sc.TeacherID` OR `sc.ReportsOversight()`. This also keeps delegated history
  visible after a revoke (see Phase 1 rollback note). No `on_behalf_of` column
  — the period's `teacher_id` already identifies whose data was sent (YAGNI).
- **Statement generation stays internal (decision D4):** `Generate` is gated by
  `GetPeriodStatus`'s own `!sc.IsOwner` branch
  (`statements/repository.go:223-239`), which ALSO gates the standalone
  `POST /billing-periods/:id/statements/generate` route
  (`statements/routes.go:9-14`). BulkSend must pass for a secretary
  (`notifications/service.go:137` calls Generate), but the standalone endpoint
  must NOT open up. Split the intent: `Generate` takes an internal
  delegated-allowed flag (or a `GetPeriodStatusForSend` variant using
  `ReportsOversight()`); the standalone handler keeps the old owner/own-period
  guard. `Revoke` keeps `scoped()` untouched. Mandatory denial tests for both
  standalone generate and revoke on a foreign period.
- **Send exclusivity gate (D8):** at the TOP of `BulkSend` and `ResumeRun`
  (`notifications/service.go`), before any channel/session work:

  ```go
  if !sc.ReportsOversight() {
      return nil, apperror.Forbidden("sending reports requires the send-reports permission")
  }
  ```

  Applies to every channel — `zalo_personal` and `zalo_manual` alike. This is
  a deliberate deviation from the neutral not-found convention (plan.md D5):
  the caller owns the period and can see it, so an honest 403 beats a lying
  404. `markSent` keeps its current row-scoped guard (a member finishing
  previously created manual rows must not brick), and the ledger/run reads
  stay per the period-scoped rescope above. Existing integration tests where
  a plain teacher self-sends are UPDATED to expect the denial (intentional,
  not weakened); owner-send suites must pass untouched. Check `apperror` for
  a Forbidden constructor first — if none exists, add one following the
  existing `Conflict` pattern rather than overloading 409.
- **Delegated personal send:** `notifications/service.go:149-151` currently
  refuses a cross-teacher period on `zalo_personal` with 409. With the
  exclusivity gate above, callers here are only the owner or a capability
  holder; the cross-teacher branch below now only ever fires for the owner
  (member-only flag, D2), preserving the 409 verbatim. New gate:

  ```go
  crossTeacher := len(gen.Statements) > 0 && gen.Statements[0].TeacherID != sc.TeacherID
  if personal && crossTeacher && !sc.CanSendReports {
      return apperror.Conflict("...existing message...")
  }
  ```

  Owner never holds the flag (member-only, plan.md D2), so the owner's 409 is
  preserved unconditionally. `verifyPersonalSession(ctx, sc.TeacherID)` stays
  on the CALLER — the secretary's own linked Zalo, preserving the
  1-Zalo-per-person invariant (`migrations/000004_zalo_accounts.up.sql`).
- **One personal-send run per period (new invariant):** today "one period, one
  personal sender" is enforced only INDIRECTLY by the 409 this phase removes —
  `HasActiveRun`, `RunManager.Reserve` and the partial unique index
  (`migrations/000006_one_running_run_per_teacher.up.sql`) are all keyed by
  `teacher_id` (`notifications/repository.go:172-175`,
  `run_manager.go:169-186`), so teacher and secretary sending the same period
  concurrently would double-DM every parent. Add migration 000012: partial
  unique index `ON notification_runs (billing_period_id) WHERE
  status = 'running'`, plus a `HasActiveRunForPeriod(centerID, periodID)`
  pre-check before `Reserve` for a clean 409. Integration test: two concurrent
  callers on one period → exactly one run created.
- **Revocation semantics (truthful, not "instant"):** a run is a background
  goroutine holding all items in memory with a minimal scope
  (`run_manager.go:48-54,82-84`); revoking the flag does NOT stop it by
  itself. For cross-teacher runs the run loop re-checks `can_send_reports`
  (one indexed read per item; pace is 3-8s so load is trivial) before each
  `SendDM` and fails the remaining items with an explicit "permission revoked"
  reason. `ResumeRun` (`service.go:413-433`) re-checks `CanSendReports` for a
  cross-teacher run before resuming — its internal `Generate` call must use
  the same delegated path as BulkSend. Recovery for orphaned runs (secretary
  interrupted then revoked/left): the period's teacher — who now SEES the run
  via period-scoped ledger reads — or the flow of a fresh send can fail-out
  queued rows of a non-running cross-teacher run on their own period
  (`FailQueuedInRun` with reason); tests cover "revoke mid-run" and
  "close membership then resume".
- **Pre-send preview endpoint (server-side buckets):** add read-only
  `GET /billing-periods/:id/notifications/preview` returning the three buckets
  the web confirm dialog needs — auto-send (mapped + friend), mapped-but-not-
  friend, unmapped (manual fallback) — computed server-side from the FULL
  contact set intersected with the caller's Zalo friend list (`zalo` feature,
  live fetch `protocol/contacts.go:19-26`), plus `max_run_size`
  (`config.go:114`, currently 50) so the client can warn/block oversized runs.
  Rationale: the current client-side intersection stops at the 100-per-page
  cap (`notifications-page.tsx:92-106` documents the undercount) — tolerable
  for own-teacher rosters, guaranteed wrong center-wide, and a secretary
  confirming a send she cannot verify must not be shown invented numbers.
  Guarded by the same oversight/ownership rule as BulkSend.
- **Attribution:** already correct — `service.go:213` stamps `sc.TeacherID` on
  notification rows and `service.go:253-260` on runs; bulk-send's audit row
  records the caller as actor via the middleware path. Add tests asserting the
  secretary is the sender, never the period's teacher, never the owner.
- **Privacy trade-off (documented, accepted):** oversight reads expose contact
  names, phones and `zalo_user_id` center-wide to the capability holder — the
  same visibility the owner already has, and intrinsic to the granted purpose
  (she must send to those parents). The Zalo mapping remains the
  period-owner's consent artifact (`notifications/repository.go:114-123`);
  the delegated path reads it, never rewrites it.

## Related Code Files

- Create: `apps/api/migrations/000012_one_running_run_per_period.up.sql` +
  `.down.sql`
- Modify: `apps/api/internal/features/statements/repository.go` + `service.go`
- Modify: `apps/api/internal/features/billing/repository.go` (read helper),
  `dto.go` (PeriodResponse gains `teacher_id`, `teacher_name` — additive
  public-contract change Phase 4 needs for group-by-teacher; `make api-docs`)
- Modify: `apps/api/internal/features/collections/repository.go` (read helper)
- Modify: `apps/api/internal/features/contacts/repository.go` (read helper,
  list only)
- Modify: `apps/api/internal/features/notifications/service.go`,
  `repository.go`, `run_manager.go`, `handler.go`, `routes.go` (preview route)
- Modify: `apps/api/internal/features/audit/action.go` (preview route is GET —
  no entry needed; verify bulk route entries unchanged)
- Modify: `apps/api/internal/features/*/auth_integration_test.go` (statements,
  notifications, billing, contacts, collections)
- Modify: `apps/api/internal/testutil/` — secretary fixture helper

## Implementation Steps

1. Inventory every repo scope function branching on `sc.IsOwner`
   (`grep -rn "IsOwner" apps/api/internal/features/`); classify each as
   reports-read (new `scopedRead`) or keep-as-is. Record the classification
   table in the PR description.
2. Add `testutil` helper (secretary fixture) reusing `Teacher` + membership +
   flag update.
3. Add `scopedRead` helpers + wire the read cluster (statements, billing,
   collections, contacts list, ZaloMappings); extend `PeriodResponse` with
   teacher identity; run statements/billing/collections integration tests.
4. Rescope ledger reads (`ListByPeriod`, `LatestRunByPeriod`, `RunSnapshot`)
   by period ownership; regression-test the teacher's view of a delegated send.
5. Migration 000012 (per-period running index) + `HasActiveRunForPeriod`
   pre-check; concurrent-send integration test.
6. Add the `ReportsOversight()` exclusivity gate to BulkSend + ResumeRun
   (Forbidden constructor if missing); soften the 409 gate to the
   `CanSendReports` condition (owner-only in practice); internal
   delegated-Generate path; per-item permission re-check in the run loop;
   `ResumeRun` re-check + orphaned-run fail-out. Update plain-member
   self-send tests to expect denial.
7. Preview endpoint (buckets + max_run_size) + tests.
8. Write the authorization matrix tests (see Todo).
9. `make test-api` (integration, Docker) green.

## Todo

- [x] Scope-branch inventory documented; only the read cluster gets
      `scopedRead`; shared `scoped()` untouched everywhere
- [x] Secretary: periods list (with teacher identity), statements list/get,
      figures, collections debt, notifications list return center-wide data
- [x] Secretary + `zalo_personal` on another teacher's period: run created,
      run/notification rows stamp secretary's TeacherID, messages via
      fakeZaloSender from her session
- [x] Period's teacher sees the delegated rows in their period ledger
- [x] Two concurrent senders on one period → exactly one run (index + 409)
- [x] Revoke mid-run → remaining items fail with explicit reason; resume after
      revoke denied; orphaned queued rows can be failed out
- [x] Preview endpoint returns correct buckets past 100 contacts and exposes
      max_run_size
- [x] Secretary send on period whose contacts are unmapped → rows fall back
      manual (existing behavior)
- [x] Plain member (no flag): reads still own-scoped / neutral not-found
      (regression); BulkSend + ResumeRun denied with 403 on their OWN period,
      every channel (new); `markSent` on an existing row still works
- [x] Owner: cross-teacher `zalo_personal` still 409; own sends unchanged
      (regression)
- [x] Secretary write attempts denied — attendance mutation, class mutation,
      billing close/void/adjustment, contacts write, member remove, invitation
      create, standalone statement generate, statement revoke — one test per
      surface, per existing convention (neutral not-found / owner-only guard)
- [x] Secretary without linked Zalo → `verifyPersonalSession` refusal
      unchanged

## Success Criteria

- [x] Full authorization matrix green under `make test-api`
- [x] Owner suites pass untouched; plain-member READ suites pass untouched;
      the only intentional diffs are member self-send tests now expecting
      denial (D8)

## Risk Assessment

- Biggest risk is over-relaxing a scope function — mitigated by never touching
  the shared `scoped()` helpers, the explicit inventory step, and one denial
  test per write surface including statement generate/revoke.
- Migration 000012 can fail on a live DB that already has two running runs for
  one period — impossible today (the 409 blocks cross-teacher personal sends),
  but the up migration should still be written to tolerate/clean terminal
  states like `interrupted`.
- Rollback: revoking the flag closes NEW sends immediately; an in-flight run
  stops at the next item via the per-item re-check (seconds, not instant —
  stated truthfully); delegated history stays visible via period-scoped reads.
- D8 is a behavior removal: a center with no secretary has only the owner as
  sender after deploy. Deliberate product decision (2026-08-29) — needs the
  Phase 5 release note, and support playbook is "owner grants the flag".
