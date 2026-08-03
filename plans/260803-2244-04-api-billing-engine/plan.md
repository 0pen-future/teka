---
title: "04 API Billing Engine"
description: "Billing periods, per-student fee computation, chốt sổ (period close) with hard block on unconfirmed sessions, immutable invoice snapshots, adjustments and void."
status: completed
priority: P1
effort: "22h"
branch: HEAD
tags: [api, go, billing, invoices, money]
created: 2026-08-03
blockedBy: [260803-2244-03-api-sessions-and-attendance]
blocks: [260803-2244-05-api-payments-and-collections]
---

# 04 API Billing Engine

## Overview

Turns confirmed attendance into money. Implements PRD **R3** (per-individual fee
computation) and **R4** (chốt sổ = period close and review). Introduces the
`billing` feature package which owns `billing_periods`, `invoices`,
`invoice_lines`, `invoice_adjustments`.

Core formula (PRD R3, mirrored by the DB CHECK at `docs/schema_design.sql:306`
and `:332`):

```
line.amount      = line.billable_count * line.unit_price
invoice.current_charge   = SUM(line.amount)
invoice.opening_balance  = previous closed period invoice outstanding (carry-over)
invoice.adjustment_total = SUM(active invoice_adjustments.amount)
invoice.total_due        = opening_balance + current_charge + adjustment_total
```

One invoice per **student** per period; one `invoice_lines` row per
**enrollment**. A student in two classes produces two lines on one invoice
(PRD R1 acceptance, `docs/schema_design.sql:314`).

## Scope

In scope:

- `billing_periods` lifecycle: create/list/get, open, close, reopen-blocked.
- Fee preview (the chốt sổ review screen data, R4) computed live, no writes.
- Draft invoice materialisation so per-line review adjustments can attach.
- Close flow: hard block on past-unconfirmed sessions, immutable snapshots,
  `draft` → `issued` (or `void` for empty invoices), period → `closed`.
- Manual review adjustments with mandatory reason (`invoice_adjustments`).
- Post-close attendance edits → next-period adjustments (D7).
- Invoice void with reason.

Non-goals (owned elsewhere or out of V1):

- Payments, allocations, collection board → plan 05.
- Statements, message text, notifications → plan 06.
- The P1 "tiền đang thất thoát" report over `v_unbilled_attendance`
  (`docs/schema_design.sql:474`) — the view exists from the baseline, no
  endpoint in V1.
- Any schema change. Schema is frozen at `docs/schema_design.sql` (D1). No new
  migration files in this plan. Plan 03 introduces `000003` (widening
  `idx_class_sessions_pending` to cover past `planned` sessions, baseline
  untouched); if plans 04-06 ever prove they need one, it starts at `000004`.

## Phases

| # | Phase | Effort | Depends on | Status |
|---|-------|--------|-----------|--------|
| 1 | [Billing periods and fee calculator](./phase-01-periods-and-fee-calculator.md) | 6h | — | Pending |
| 2 | [Preview and draft invoices](./phase-02-preview-and-draft-invoices.md) | 5h | 1 | Pending |
| 3 | [Period close and void](./phase-03-period-close-and-void.md) | 6h | 2 | Pending |
| 4 | [Adjustments and post-close reconciliation](./phase-04-adjustments-and-reconciliation.md) | 5h | 3 | Pending |

## Upstream contracts (plans 02-03)

Billing consumes three sanctioned entry points and reimplements none of them.
Each is a place where a second copy of the rule would silently change money or
block a teacher on something they were never warned about.

| Contract | Owner | Billing consumes it in |
|---|---|---|
| `enrollments.ActiveOn(teacherID, classID, date)` — roster membership, inclusive at both `started_on` and `ended_on` | plan 02 | Phase 1 (invariant assertion), phase 4 (no-line enrollment case) |
| `attendance.CountBillableByEnrollment` — billable/absent/present counts per enrollment over a date window | plan 03 | Phase 1 `TallyAttendance`, phase 4 `LiveBillableCounts` |
| Pending-attendance feed with `from`/`to` filtering | plan 03 phase 3 | Phase 3 close block and the future-session warnings |

Import direction is one-way: `billing` imports `attendance` and `enrollments`;
neither imports `billing`. Phase 4's `BillingReconciler` callback interface is
declared inside the attendance package to keep it that way.

## Key decisions

- **D1** — schema is `docs/schema_design.sql` verbatim, including both views.
  Nothing in this plan adds, drops, or alters a column.
- **D5** — money is `BIGINT` VND. No floats anywhere, including DTOs (JSON
  numbers are `int64`). States are `VARCHAR` + CHECK mirrored as Go constants.
  All fee math lives in Go services and is unit-tested; no DB triggers
  (`docs/schema_design.sql:540`).
- **D4** — every repository query is scoped by `teacher_id`; reads on
  soft-delete tables add `deleted_at IS NULL`. The four financial tables have no
  `deleted_at` by design (`docs/schema_design.sql:512`); corrections use
  `status='void'`, reversal, or a counter-signed adjustment.
- **D7** — editing attendance of a **closed** period never mutates an issued
  invoice. It writes `invoice_adjustments` against the **next open** period's
  invoice with `source_session_id` set. Resolves PRD Q5, consistent with schema
  note (k) at `docs/schema_design.sql:533`.
- **D3** — UUIDv7 generated in Go via the shared util from plan 01.
- **Billable ≠ attended (V1).** `attendance_records.billable` defaults `true`
  and the schema comment at `docs/schema_design.sql:234` states that in V1 both
  `present` and `absent` are charged; `excused` is P1. Money therefore follows
  `billable = true`, not `status = 'present'`. `absent_count` is carried
  separately for display only. See open question OQ-1 — PRD R3 wording says
  "số buổi có mặt" (sessions attended) and contradicts this.
- **Review edits are invoice-level adjustments, not line edits.** R4 asks for
  per-line manual correction with a reason. `invoice_adjustments`
  (`docs/schema_design.sql:338`) has `invoice_id` + `reason` but **no**
  `invoice_line_id`. Rather than invent a column, a per-line correction is
  stored as an invoice-level adjustment whose `reason` names the line. This
  keeps recompute-at-close safe (adjustments survive recomputation, direct line
  edits would not). Trade-off recorded in OQ-2.

## Acceptance criteria

Pulled from PRD R3 and R4.

- [x] R3: three students in one class with different attended counts produce
      three different amounts, each equal to `billable_count * unit_price`.
- [x] R3: a student owing 500,000 from the previous period sees that amount in
      `opening_balance`, rendered separately from `current_charge`.
- [x] R3: recomputing after an attendance change inside an **open** period
      yields updated figures with no manual step.
- [x] R4: closing a period that still has a past session without
      `attendance_confirmed_at` returns `409` and lists the offending sessions
      (id, class name, date, status).
- [x] R4: after close, editing attendance inside that period returns a warning
      and creates an `invoice_adjustments` row on the next open period's
      invoice with `source_session_id` set; the closed invoice is byte-for-byte
      unchanged.
- [x] R1: a student enrolled in two classes of the same teacher gets one
      invoice with two `invoice_lines`.
- [x] R1: a student added mid-period is charged only from the first session
      they have an attendance record for.
- [x] Edge (PRD §5): a student who quits mid-period is still invoiced up to
      their last session and keeps any carried debt.
- [x] Edge (PRD §5): a class with zero sessions in the period produces no
      invoice line; a student with no sessions anywhere and no carried debt
      produces no issued invoice.
- [x] Edge (PRD §5): a session with `status='cancelled'` bills nobody.
- [x] Every write path runs inside one transaction; a failure mid-close leaves
      the period `open` and no partial invoices.

## Open questions

- **OQ-1 (blocking for correctness, cheap to answer).** PRD R3 says fees are
  "số buổi **có mặt** × đơn giá" (attended sessions × unit price) but
  `docs/schema_design.sql:234` says V1 charges both `present` and `absent`.
  These give different numbers the moment anyone is absent. This plan follows
  the schema (`billable = true`) because D1 makes the schema authoritative, and
  because charging absences is the common Vietnamese tutoring convention for
  trả-sau classes. **Needs a product confirmation before first real close.** If
  R3 wins instead, the only change is the calculator predicate — one function,
  one test file.
- **OQ-2.** `invoice_adjustments` cannot reference a specific line. Per-line
  review attribution therefore lives in free-text `reason`. Acceptable for V1
  (teacher reads it, parent never sees it); revisit if the web UI needs to
  render "which line was corrected" structurally.
- **OQ-3.** Double-count risk: a teacher may both fix an attendance record and
  add a compensating adjustment for the same fact. Mitigated by showing
  `current_charge` and `adjustment_total` separately in preview and by listing
  every adjustment with its reason, but not prevented. Confirm this is
  acceptable rather than adding a guard.
- **OQ-4.** Closing a period early (e.g. on the 28th with sessions scheduled on
  the 30th) is allowed with a warning. Those later sessions bill into the next
  period via the D7 adjustment path. Confirm teachers expect that rather than a
  hard block on future sessions.

<!-- slug: 04-api-billing-engine -->
