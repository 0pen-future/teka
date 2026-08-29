---
title: "Secretary as delegated report sender"
description: "Sending reports becomes the secretary's job: a can_send_reports member reads statements/debt center-wide and runs sends from her own Zalo; plain teachers only input attendance/remarks and lose send access; owner unchanged"
status: done
priority: P1
effort: "7.5d"
tags: [api, web, zalo, notifications, authorization]
created: 2026-08-29
blockedBy: []
blocks: []
---

# Secretary as delegated report sender

> **HTML plan:** [plan.html](./plan.html) is the user-facing artifact (phase
> outlines, diagrams, annotated mockups). This file + `phase-*.md` remain the
> canonical execution detail for `/ak:cook`.

Contract source: [brainstorm report](../reports/brainstorm-260828-0849-secretary-report-sender.md)
(contract accepted 2026-08-28; option A of three).

## Overview

Owner invites a secretary as a normal center member (existing invitation flow,
untouched). A new boolean membership permission `can_send_reports` lets her
read billing periods/statements/debt across the whole center and run paced
statement/reminder sends for any member's period **from her own linked Zalo**,
with notification/run/audit rows attributing HER as sender. No write access to
attendance, classes, billing config, contacts, or membership.

**Division of labor (direction change 2026-08-29):** sending reports to
parents is the secretary's responsibility, not the teachers'. Teachers only
provide per-lesson input — điểm danh (attendance) and nhận xét (session
notes/marks, both already shipped in `000009_teaching`, untouched here). A
plain member (no flag) is **blocked from creating sends on every channel**,
including their own periods — an intentional breaking change to today's
self-send behavior. The owner keeps today's send abilities as the fallback
sender. Report content stays statements/reminders; lesson remarks do NOT go
into the sent messages.

## Key decisions

- **D1 — flag, not role:** permission is `center_members.can_send_reports`
  (boolean, default false). No role enum, no JWT change, no RBAC framework
  (brainstorm non-goal, YAGNI). Scope resolution picks it up per request.
- **D2 — owner behavior frozen; flag is member-only:** today `zalo_personal`
  refuses a cross-teacher period with 409
  (`notifications/service.go:94-96,149-151`). The new gate allows
  cross-teacher personal sends only when the caller has `can_send_reports`,
  and the flag can only be granted to an active non-owner member (user
  decision 2026-08-29) — so the owner's 409 is preserved unconditionally.
  Owner send behavior stays exactly as today (fallback sender when no
  secretary exists); the breaking change of D8 applies to plain members only.
- **D3 — read cluster via dedicated read helpers:**
  `ReportsOversight() = IsOwner || CanSendReports` is applied through NEW
  read-only scope helpers (`scopedRead`), never by relaxing the shared
  `scoped()` helpers that also back writes (red-team findings 1-3). Cluster:
  statements list/get/figures, billing periods list/get, collections
  debt/balances reads, contacts roster read, notifications
  list/runs/Zalo-mapping reads. Period ledger reads become period-scoped so
  the period's teacher always sees delegated sends on their own period.
  Dashboard, audit, invitations, and every write path keep their current
  guards.
- **D4 — no standalone statement generation right:** the secretary reaches
  statement refresh only through the BulkSend flow via an internal
  delegated-Generate path; the standalone generate endpoint and statement
  revoke keep their current guards (denial-tested).
- **D5 — denial convention:** denials follow the repo's existing neutral
  not-found tenancy convention (`statements/auth_integration_test.go:141`)
  rather than literal 403s; the brainstorm's "403 paths tested" maps to "denial
  paths tested per convention".
- **D6 — one personal-send run per period:** removing the 409 removes the
  indirect guarantee that only one person sends a period; a new partial unique
  index on `notification_runs(billing_period_id) WHERE status='running'` plus
  a pre-check makes it explicit — no parent ever gets double DMs from two
  concurrent runs.
- **D8 — send exclusivity (direction change 2026-08-29, supersedes the
  original Goal 5 for plain members):** creating sends requires
  `ReportsOversight()` (`IsOwner || CanSendReports`) — the same helper D3
  already defines, reused as the send gate (DRY, no second concept). Enforced
  server-side at the top of BulkSend and ResumeRun for ALL channels
  (`zalo_personal`, `zalo_manual`, future ones); a plain member gets an
  explicit 403-style denial, not the neutral not-found — the period is
  visibly their own, so hiding it would lie (conscious deviation from D5,
  which stays the rule for cross-tenant reads). `markSent` and ledger reads
  stay as-is (existing rows remain workable/visible). Web hides send entry
  points for plain members; server remains the authority. User decisions
  2026-08-29: block at API on every channel; owner keeps sending; report
  content unchanged (statements/reminders only — nhận xét is teacher input,
  not message content).
- **D7 — audit via action registry, split routes:** grant/revoke are
  `POST`/`DELETE /centers/me/members/:teacherId/send-reports` with two entries
  in `audit/action.go` — the middleware stores no request body, so distinct
  routes are what makes grant and revoke distinguishable in the audit trail.
  No manual bus events (double-log risk).

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Owner grants/revokes send-reports permission per member (API + UI + audit) | P1 |
| 2 | Capability holder reads periods/statements/debt center-wide | P1 |
| 3 | Capability holder runs statement/reminder sends from her own Zalo for any member's period, attributed to her | P1 |
| 4 | Befriend-first UX: pre-send warning for mapped-but-not-friend parents + friend-request CTA | P2 |
| 5 | Zero behavior change for the owner; plain-member READS unchanged | P1 |
| 6 | Send exclusivity: plain members blocked from creating sends on every channel (API + hidden UI); teachers keep only attendance + nhận xét input | P1 |

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Permission model & scope resolution](./phase-01-start.md) | Done |
| 2 | [API read oversight & delegated send path](./phase-02-api-permission-and-guards.md) | Done |
| 3 | [Web: owner grant/revoke UI](./phase-03-web-owner-grant-revoke.md) | Done |
| 4 | [Web: secretary send experience](./phase-04-web-secretary-send-experience.md) | Done |
| 5 | [E2E, regression & docs](./phase-05-integration-e2e-and-docs.md) | Done |

Dependencies: 1 → 2 → 4; 1 → 3 → 4; 5 after 3+4. Phases 2 and 3 can run in
parallel after 1.

## Success Criteria

- [x] Owner can invite secretary (existing flow) and grant/revoke the
      permission; grant/revoke is audited with owner as actor
- [x] Secretary links her own Zalo via the untouched per-teacher flow
- [x] Secretary lists periods/statements center-wide and runs a paced send;
      messages go out from her Zalo; notification/run rows attribute her
- [x] Secretary cannot mutate attendance/classes/billing/contacts/membership
      (denial paths tested per convention, one test per surface)
- [x] Plain member cannot create a send on ANY channel — own period included —
      via bulk or resume (explicit 403-style denial tested); their reads,
      attendance and nhận xét input are unchanged
- [x] Send entry points (nút "Nhắc nợ", link gửi thông báo trên billing
      review, send CTA on the notifications page) hidden for plain members;
      ledger stays visible
- [x] Owner send behavior unchanged incl. cross-teacher 409
      (regression suites pass untouched)
- [x] No period can run two concurrent personal sends; period's teacher sees
      delegated sends in their own ledger (no double-DM paths)

## Key risks

- Zalo spam heuristics on fresh accounts DM-ing non-friends — mitigated by
  befriend-first UX, server-computed 3-bucket pre-send preview, existing run
  pacing; not testable in e2e (operational risk).
- Over-relaxing a scope function — mitigated by never touching shared
  `scoped()` helpers (dedicated `scopedRead`), Phase 2's explicit `IsOwner`
  branch inventory, and per-surface denial tests incl. statement
  generate/revoke and billing close/void.
- Revocation is not instantaneous for an in-flight run — the run loop
  re-checks the flag per item (seconds at the configured pace); documented
  truthfully instead of claimed instant.
- Privacy trade-off (accepted): a capability holder reads contact names,
  phones and Zalo mappings center-wide — same visibility the owner has,
  intrinsic to the granted purpose; mappings stay read-only for her.
- Rollout of D8 is a behavior REMOVAL for existing teachers: after deploy,
  a teacher who used to self-send sees no send entry until the owner grants
  someone the flag (owner can always send as fallback). Needs a release note
  and the docs update in Phase 5; existing tests that exercised member
  self-send are updated intentionally, not weakened.

## Red Team Review

| Session | Reviewers | Findings | Outcome |
|---------|-----------|----------|---------|
| 2026-08-29 | Assumption Destroyer, Security Adversary, Failure Mode Analyst (Full tier) | 15 after dedup: 5 Critical, 8 High, 2 Medium — all evidence-backed (file:line verified) | All 15 accepted and applied 2026-08-29 (user: "apply all"). Key changes: dedicated read-only scope helpers instead of relaxing shared `scoped()` (D3); per-period run lock migration 000012 (D6); split grant/revoke routes + action-registry audit (D7); flag reset on membership close/reopen; period-scoped ledger reads; server-side pre-send preview endpoint; delegated-Generate internal path (D4); e2e seed cross-teacher data + idempotent spec; citation fixes (`:teacherId`, handler line numbers, no shadcn switch, `personalReady` location). User decisions: flag is member-only (no owner self-grant); validation interview skipped. Effort 5.5d → 7d. |

**Direction change (2026-08-29, post-red-team):** user redirected the model to
"only the secretary sends; teachers only input điểm danh + nhận xét". Decisions
recorded in D8 (4 answers: block at API, all channels, owner keeps sending,
report content unchanged). Original Goal 5's "zero behavior change for plain
members" narrowed to reads only; Goal 6 added. The red-team rows above reviewed
the pre-change plan; the D8 delta (send gate, UI hiding, test updates) has NOT
been re-red-teamed.

**Consistency sweep (2026-08-29):** all six plan files re-checked after
applying findings — member-only flag consistent across D2/Phase 1 guard/
Phase 3 UI/Phase 4 nav; route shape `POST|DELETE …/:teacherId/send-reports`
consistent across Phase 1/Phase 3/D7; migration numbering 000011 (Phase 1) /
000012 (Phase 2) sequential after 000010; preview endpoint produced in
Phase 2, consumed in Phase 4; `PeriodResponse` teacher fields produced in
Phase 2, consumed in Phase 4 grouping; e2e audit assertions match D7 action
names; efforts sum to the frontmatter total (1.5+2.5+0.5+1.5+1 = 7d). Zero
unresolved contradictions.

**Consistency sweep #2 (2026-08-29, after the D8 direction change):** all six
files re-checked. "Purely additive" / "zero behavior change for plain members"
claims removed or narrowed to reads everywhere (D2, Goal 5, Phase 2
requirements/criteria, Phase 4 criteria); Phase 1's "all existing tests pass
unchanged" stands (Phase 1 alone stays additive — the gate lands in Phase 2);
`ReportsOversight()` is both the read and the send gate (one helper, D3+D8);
web gating mirrors it as `canRunSends`; Phase 5 covers the plain-teacher
negative path + release note; efforts now 1.5+3+0.5+1.5+1 = 7.5d matching
frontmatter. Zero unresolved contradictions.

## Open questions

None — the brainstorm's two unresolved questions are decided here: grant/revoke
UI lives on the `/center` members list (Phase 3); secretary gets a dedicated
read-only period list page reusing the existing send page, not the owner
oversight views (Phase 4). The red team's open questions (owner self-grant,
revoke-mid-run semantics, collections scope, contact privacy) are decided in
D2/D3 and the risk list above.

<!-- slug: secretary-report-sender -->
