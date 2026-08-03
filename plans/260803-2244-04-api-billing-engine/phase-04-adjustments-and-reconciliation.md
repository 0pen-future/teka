---
phase: 4
title: "Adjustments and Post-Close Reconciliation"
status: pending
priority: P1
effort: "5h"
dependencies: [3]
---

# Phase 4: Adjustments and Post-Close Reconciliation

## Overview

Two related capabilities, both built on `invoice_adjustments`
(`docs/schema_design.sql:338-353`):

1. **Manual review adjustments** — R4's "sửa được thủ công từng dòng (có ghi
   chú lý do)" (correct each line by hand, with a recorded reason).
2. **Post-close reconciliation (D7)** — PRD R2 lets a teacher fix attendance for
   a session that already happened; PRD R4's second acceptance criterion says
   the system must warn and carry the difference into the next period. This
   phase implements that carry, answering PRD Q5 and resolving schema note (k)
   (`docs/schema_design.sql:533`) in favour of the adjustment branch.

## Requirements

- Every adjustment carries a non-empty `reason` (enforced by the CHECK at
  `docs/schema_design.sql:351`).
- Editing attendance in a **closed** period never mutates the issued invoice.
- The delta lands on the **next open** period's invoice for the same student,
  with `source_session_id` pointing at the edited session
  (`docs/schema_design.sql:344`).
- Repeated edits to the same session must not double count.
- Editing attendance in an **open** period changes nothing here — the next
  draft/close recomputation picks it up naturally (phase 2).
- Cancelling an adjustment creates an opposite-signed adjustment; it does not
  delete (`docs/schema_design.sql:347`).

## Architecture

### Manual adjustments

`POST /invoices/:id/adjustments` with `{ amount, reason }`. `amount` is signed:
negative reduces the bill. Allowed while the invoice is `draft`, `issued`, or
`partially_paid`; refused on `void` and on `paid` (a paid invoice cannot grow a
new balance without re-opening collection — return `409` telling the teacher to
adjust the next period instead).

After insert, recompute in the same transaction:

```
adjustment_total = Σ amount WHERE invoice_id = :id AND deleted_at IS NULL
total_due        = opening_balance + current_charge + adjustment_total
status           = derived from paid_amount vs total_due (plan 05 helper)
```

`DELETE /invoices/:id/adjustments/:adjId` is **not** offered. Cancelling posts
an opposite-signed adjustment via the same endpoint. `deleted_at` on
`invoice_adjustments` exists only for the "typed it wrong, nothing sent yet"
case and is not exposed as an API in V1.

### Post-close reconciliation (D7)

**Trigger.** Attendance mutation lives in plan 03's feature package. The import
direction is one-way: `billing` imports `attendance` (for
`CountBillableByEnrollment` and the pending feed, phases 1 and 3), and
`attendance` must never import `billing`. So the callback interface is declared
in attendance, billing supplies the implementation, and the two are joined in
`registerFeatures` (`apps/api/internal/server/router.go:63`):

```go
// declared in the attendance package
type BillingReconciler interface {
    ReconcileSession(ctx context.Context, teacherID, sessionID uuid.UUID) (Reconciliation, error)
}
```

The attendance service calls it after any committed change to a confirmed
session's records, and surfaces the returned `Reconciliation` (whether an
adjustment was created, for how much, against which period) as a warning in its
own response — that is R4's "hệ thống cảnh báo" (the system warns).

`ReconcileSession` is a no-op returning an empty result when the session's date
falls outside any closed period.

**Delta computation.** Per student affected by the session, at student level
(not line level) because `invoice_adjustments` has no line reference:

```
P            = closed billing_period containing session.session_date
I            = invoice of (P, student), status <> 'void'   -- skip if absent
live_charge  = Σ over I's lines: live_billable_count(line.enrollment_id, P) * line.unit_price
already_adj  = Σ invoice_adjustments.amount
                 WHERE deleted_at IS NULL
                   AND source_session_id IN (sessions of P)
                   AND invoice.student_id = student
delta        = live_charge - I.current_charge - already_adj
```

Three things this gets right and that the tests must pin down:

- `line.unit_price` is the **snapshot** price, so a later price change does not
  retroactively reprice a closed period.
- `already_adj` is what prevents double counting on a second edit: the issued
  invoice never moves, so without this term every subsequent edit would re-post
  the full difference. `already_adj` is found through `source_session_id` →
  session → date → period, which is why `source_session_id` must always be set
  on reconciliation rows.
- An enrollment with attendance in `P` but no line on `I` (a class the student
  joined and had backfilled) contributes `live_count × enrollments.unit_price`.
  Membership on the session date is confirmed through
  `enrollments.ActiveOn(ctx, teacherID, classID, session.session_date)`, never a
  hand-written `started_on`/`ended_on` comparison. Rare; covered by a test.

If `delta = 0`, write nothing and report no adjustment.

**Target invoice.** The adjustment needs an `invoice_id`, and the next period's
invoices may not exist yet. `ensureAdjustmentTarget` resolves it:

1. Target period = the earliest `billing_periods` row for the teacher with
   `status='open'` and `period_start > P.period_end`.
2. If none, ensure the period for the current calendar month (teacher timezone)
   when it is after `P`; if that month is already closed, ensure the following
   month. Reuses `EnsurePeriod` from phase 1, which is idempotent.
3. Ensure a `draft` invoice exists for `(target period, student)` via the
   phase 2 upsert, with `opening_balance = 0`, `current_charge = 0`, name
   snapshots taken live. The next draft/close recomputation fills in the real
   figures and preserves the adjustment (phase 2, step 4).

**Reason text** is generated, not free-form, so the teacher can read the trail:
`"Điều chỉnh do sửa điểm danh buổi {date} lớp {class_name} của kỳ {MM/YYYY}"`
(adjustment from an attendance correction on {date}, class {class_name}, period
{MM/YYYY}).

**Interaction with statements.** Plan 06 renders the parent link from live data
and shows carried adjustments separately, so a parent opening an old link after
a correction sees updated numbers (R5 acceptance) even though the invoice itself
never changed.

## Related Code Files

Create:

- `apps/api/internal/features/billing/adjustment.go` — manual adjustments +
  `ReconcileSession`
- `apps/api/internal/features/billing/adjustment_test.go` — delta math unit tests

Modify:

- `apps/api/internal/features/billing/repository.go` — add
  `CreateAdjustment`, `AdjustmentsBySourcePeriod`, `PeriodContainingDate`,
  `NextOpenPeriod`, `LiveBillableCounts`, `RecalcInvoiceTotals`
- `apps/api/internal/features/billing/service.go` — add `AddAdjustment`,
  `ReconcileSession`
- `apps/api/internal/features/billing/dto.go` — add `AdjustmentRequest`,
  `AdjustmentResponse`, `ReconciliationResponse`
- `apps/api/internal/features/billing/handler.go` — add `addAdjustment`,
  `listAdjustments`
- `apps/api/internal/features/billing/routes.go` — add the two routes
- `apps/api/internal/features/billing/integration_test.go` — reconciliation cases
- `apps/api/internal/features/attendance/service.go` — declare
  `BillingReconciler`, call it after a committed change, return the warning
  **[UNVERIFIED path — plan 03 owns this package and has not landed yet;
  confirm the file and the mutation entry points before editing]**
- `apps/api/internal/features/attendance/dto.go` — add the warning field
  **[UNVERIFIED, same caveat]**
- `apps/api/internal/server/router.go` — pass the billing service as the
  attendance service's reconciler

Delete: none. No migration files.

## Implementation Steps

1. Add `CreateAdjustment`, `RecalcInvoiceTotals(ctx, teacherID, invoiceID)` and
   `AdjustmentsBySourcePeriod(ctx, teacherID, studentID, periodID)` to the
   repository. `RecalcInvoiceTotals` re-derives `adjustment_total` and
   `total_due` in one `UPDATE ... FROM (SELECT sum ...)` so the CHECK at
   `docs/schema_design.sql:306` can never be violated by a partial write.
2. Implement `AddAdjustment` with the status guards from Architecture, inside
   `tx.WithinTx`. Validate `reason` (`binding:"required,min=3,max=500"`) and
   reject `amount = 0`.
3. Add `GET /invoices/:id/adjustments` returning the audit trail ordered by
   `created_at`, filtered `deleted_at IS NULL`.
4. Implement `PeriodContainingDate` and `NextOpenPeriod` in the repository.
5. Implement `ensureAdjustmentTarget` per the three-step resolution, reusing
   `EnsurePeriod` (phase 1) and the invoice upsert (phase 2).
6. Implement `LiveBillableCounts(ctx, teacherID, enrollmentIDs, period)` as a
   filtered call to `attendance.CountBillableByEnrollment` over the period
   window — the same entry point phase 1 uses. Billing writes no second
   counting query, so `live_charge` and `current_charge` can never be computed
   by two different rules.
7. Implement `ReconcileSession`: resolve the session, find its closed period,
   collect affected students from the session's attendance records, compute the
   delta per student, and for non-zero deltas create the target invoice and the
   adjustment row with `source_session_id` set. One transaction for the whole
   call.
8. Write `adjustment_test.go` as pure unit tests on the delta function:
   first edit produces the full difference; second edit on the same session
   produces only the incremental difference; an edit that reverts the first
   produces an equal and opposite delta summing to zero; an unchanged session
   produces zero; a student with no invoice in the closed period is skipped.
9. Declare `BillingReconciler` in the attendance package and call it after each
   committed attendance mutation. **Verify plan 03's actual file layout and
   mutation entry points first** — the paths above are projected, not observed.
   Wire the concrete implementation in `registerFeatures`.
10. Extend `integration_test.go`:
    - close a period, flip one student from `present` to `absent` (still
      billable in V1 per OQ-1) → no delta; flip `billable` to false → negative
      delta on the next period's invoice with `source_session_id` set;
    - assert the closed invoice's every column is byte-identical before and
      after (snapshot the row, compare);
    - edit twice → two adjustment rows, sum equals the total difference, no
      double count;
    - edit when no next period exists → the next period and a draft invoice are
      created, then the next close recomputes and keeps the adjustment;
    - manual adjustment then close-time recompute → adjustment survives and is
      inside `total_due`;
    - adjustment on a `void` invoice → `409`.
11. Run `go test ./apps/api/internal/features/billing/... ./apps/api/internal/features/attendance/... -race`, then the full suite since a shared contract changed.

## Success Criteria

- [ ] R4: editing attendance of a closed period returns a warning and creates an
      `invoice_adjustments` row on the next open period's invoice with
      `source_session_id` set.
- [ ] The closed period's invoice and lines are provably unchanged after the
      edit (row-level comparison in the test).
- [ ] Repeated edits to the same session never double count.
- [ ] Every adjustment has a non-empty reason; the DB CHECK is never hit as a
      500 because Go validates first.
- [ ] Cancelling an adjustment is done by posting the opposite amount; no
      delete endpoint exists.
- [ ] `total_due = opening_balance + current_charge + adjustment_total` holds
      after every adjustment write.
- [ ] Reconciliation is a no-op for sessions in open periods and for periods
      with no invoice for that student.

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Double counting on repeated edits — parent billed twice for one correction | High | Critical | `already_adj` term derived through `source_session_id`; dedicated unit and integration tests (steps 8, 10). This is the single most likely money bug in the plan |
| `source_session_id` left null on a reconciliation row, breaking `already_adj` forever after | Medium | Critical | Always set in the one code path that creates these rows; integration test asserts non-null |
| Teacher fixes attendance *and* posts a compensating adjustment → double reduction | Medium | High | Not prevented (OQ-3 in plan.md). Preview and the invoice detail show `current_charge` and each adjustment separately so it is visible |
| Plan 03's attendance package differs from the projected paths | High | Medium | Paths tagged `[UNVERIFIED]`; step 9 requires re-verification before editing. The interface lives in attendance, so only one call site changes |
| Import cycle between billing and attendance | Medium | Medium | One-way dependency: billing imports attendance; the callback interface is declared in attendance and wired in `router.go`, so attendance never imports billing |
| Adjustment lands on a period the teacher has not looked at yet | Medium | Low | Target period auto-created as `open`; the next preview shows the adjustment with its generated reason |
| Unit price changed after close, reconciliation reprices history | Low | High | Delta uses `invoice_lines.unit_price` (the snapshot), never the live enrollment price, for enrollments that have a line |

**Rollback.** Adjustment rows are additive and reversible by posting the
opposite amount, so no destructive rollback is needed. If the reconciliation
hook proves wrong, disable it by passing a no-op `BillingReconciler` in
`registerFeatures` — one line, no data change. Adjustments already written stay
valid and are simply included in the next close.
