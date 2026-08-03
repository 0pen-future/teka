---
phase: 2
title: "Preview and Draft Invoices"
status: pending
priority: P1
effort: "5h"
dependencies: [1]
---

# Phase 2: Preview and Draft Invoices

## Overview

Builds the chốt sổ review screen data (PRD R4: "một màn hình duy nhất hiển thị
toàn bộ học sinh × số buổi × thành tiền" — one screen showing every student ×
session count × amount). Two endpoints on the same calculator:

- **Preview** — pure read, nothing persisted. Answers "what would this period
  cost if I closed it now?".
- **Draft** — persists the computed result as `invoices` with `status='draft'`
  plus `invoice_lines`, so the teacher can attach review adjustments (phase 4)
  before issuing.

Draft materialisation exists because `invoice_adjustments.invoice_id` is a
required FK (`docs/schema_design.sql:350`): a manual correction cannot be stored
until an invoice row exists. `draft` is the state the schema reserves for this
(`docs/schema_design.sql:295`).

## Requirements

- R4: one payload listing every student in the period with their per-class
  session counts and amounts, plus carried debt and grand total.
- Preview writes nothing. Draft is idempotent — running it twice produces the
  same rows, not duplicates.
- Recomputation never destroys an existing `invoice_adjustments` row.
- Recomputation never hard-deletes an `invoice_lines` row (schema note (i),
  `docs/schema_design.sql:512`) — a line that lost all its attendance is
  zeroed, not removed.
- Draft is only allowed while the period is `open`.

## Architecture

**Preview is the same code path as draft, minus persistence.** One service
method computes, two callers use the result:

```
ComputePeriod(ctx, teacherID, periodID) ([]ComputedInvoice, error)
  ├── repo.TallyAttendance          -> []AttendanceTally
  ├── repo.PreviousClosedPeriod     -> prev period (may be nil)
  ├── repo.OpeningBalances          -> map[studentID]int64
  ├── repo.AdjustmentTotals         -> map[invoiceID]int64 keyed back to student
  └── billing.Compute(...)          -> []ComputedInvoice   (phase 1)
```

`Preview` serialises the result. `Draft` persists it. No second implementation
of the formula exists (DRY).

**Students with no attendance but carried debt.** `TallyAttendance` returns
nothing for them, so `ComputePeriod` additionally pulls the previous closed
period's students with `outstanding > 0` and emits a `ComputedInvoice` with no
lines, `current_charge = 0`, `opening_balance = outstanding`. This is what keeps
a student who quit mid-period on the books (PRD §5: "học sinh nghỉ hẳn giữa chu
kỳ → giữ lại nợ nếu có").

**Draft upsert semantics.** All inside one transaction:

1. For each `ComputedInvoice`, upsert `invoices` on the natural key
   `uq_invoices (period_id, student_id)` (`docs/schema_design.sql:303`):
   - insert with `status='draft'` when absent;
   - update `opening_balance`, `current_charge`, `total_due`, and the name
     snapshots when present **and** `status='draft'`;
   - **skip and fail** with a conflict when present and `status <> 'draft'` —
     an issued invoice is immutable (D7).
2. For each `ComputedLine`, upsert `invoice_lines` on
   `uq_invoice_line (invoice_id, enrollment_id)`
   (`docs/schema_design.sql:331`), writing `billable_count`, `absent_count`,
   `unit_price`, `amount`, `class_name`.
3. Zero out any existing line on those invoices whose `enrollment_id` is not in
   the computed set: `billable_count = 0, absent_count = 0, amount = 0`. The row
   survives so the invoice total always reconciles against its detail.
4. Re-read `adjustment_total` from `invoice_adjustments` (`deleted_at IS NULL`)
   and recompute `total_due = opening_balance + current_charge +
   adjustment_total` so the CHECK at `docs/schema_design.sql:306` holds.

Snapshots written on insert and refreshed on draft update: `student_name`,
`contact_name` (`docs/schema_design.sql:287-288`), `class_name`
(`docs/schema_design.sql:323`), `unit_price` (`docs/schema_design.sql:326`).
After issue they are frozen — that is the point of the columns (they must stay
readable after a retention job removes the PII rows; schema note (q),
`docs/schema_design.sql:564`).

**Contact snapshot.** `invoices.contact_id` is the contact *at close time*
(`docs/schema_design.sql:281`). Draft refresh follows the student's current
contact; once issued it never moves.

## Related Code Files

Create:

- `apps/api/internal/features/billing/preview.go` — `ComputePeriod` + draft upsert
- `apps/api/internal/features/billing/preview_test.go` — service-level tests

Modify:

- `apps/api/internal/features/billing/repository.go` — add `AdjustmentTotals`,
  `CarriedDebtStudents`, `UpsertInvoice`, `UpsertInvoiceLine`,
  `ZeroUnmatchedLines`, `ListInvoices`, `GetInvoiceWithLines`
- `apps/api/internal/features/billing/service.go` — add `Preview`, `Draft`
- `apps/api/internal/features/billing/dto.go` — add `PreviewResponse`,
  `PreviewInvoice`, `PreviewLine`, `PreviewTotals`
- `apps/api/internal/features/billing/handler.go` — add `preview`, `draft`
- `apps/api/internal/features/billing/routes.go` — add the two routes
- `apps/api/internal/features/billing/integration_test.go` — add draft tests

Delete: none. No migration files.

## Implementation Steps

1. Add `AdjustmentTotals(ctx, teacherID, periodID) (map[uuid.UUID]int64, error)`
   to the repository, keyed by `invoice_id`, summing
   `invoice_adjustments.amount WHERE deleted_at IS NULL`.
2. Add `CarriedDebtStudents(ctx, teacherID, prevPeriodID)` returning
   `(studentID, contactID, studentName, contactName, outstanding)` for invoices
   of the previous closed period with `status <> 'void'` and
   `total_due - paid_amount > 0`.
3. Write `ComputePeriod` in `preview.go` following the data flow above. Merge
   the carried-debt-only students into the tally-derived set before calling
   `Compute`; a student present in both keeps their lines and gains the opening
   balance.
4. Add DTOs. `PreviewInvoice` carries `student_id`, `student_name`,
   `contact_id`, `contact_name`, `lines[]`, `opening_balance`,
   `current_charge`, `adjustment_total`, `total_due`, and `invoice_id`
   (null until drafted). `PreviewLine` carries `enrollment_id`, `class_id`,
   `class_name`, `billable_count`, `absent_count`, `present_count`,
   `unit_price`, `amount`. `PreviewTotals` carries `student_count`,
   `total_opening`, `total_charge`, `total_adjustment`, `total_due`. All money
   fields are `int64`.
5. Add `GET /billing-periods/:id/preview` → `Service.Preview`. Sort invoices by
   `student_name` then `student_id` for stable rendering. Return `409` if the
   period does not belong to the caller — actually `404`, per the phase 1
   no-existence-leak rule.
6. Add `POST /billing-periods/:id/draft` → `Service.Draft`. Guard:
   `period.status` must be `open`, else `apperror.Conflict("period is closed")`.
   Run the whole upsert inside `tx.WithinTx`
   (`apps/api/internal/database/tx_manager.go:11`).
7. Implement the four upsert steps from Architecture. Use explicit
   `ON CONFLICT ... DO UPDATE` SQL (GORM `clause.OnConflict`) keyed on the
   documented unique constraints; do not rely on read-then-write, which races
   with a second tab.
8. Return the refreshed draft as a `PreviewResponse` so the client does not need
   a second round trip.
9. Extend `integration_test.go`:
   - draft twice → same invoice ids, same line ids, no duplicates;
   - draft, add an adjustment directly in SQL, draft again → adjustment row
     still present and `adjustment_total` reflected in `total_due`;
   - draft, soft-delete an attendance record, draft again → line zeroed, row
     still present, invoice total reconciles with `SUM(lines.amount)`;
   - draft against a closed period → `409`;
   - draft when an issued invoice already exists → `409`, nothing mutated.
10. Run `go test ./apps/api/internal/features/billing/...`.

## Success Criteria

- [ ] `GET /api/v1/billing-periods/:id/preview` returns every student with at
      least one billable session in the period, plus every student carrying debt
      from the previous closed period.
- [ ] A student enrolled in two classes appears once with two lines (R1).
- [ ] Preview writes zero rows (asserted by comparing table counts before and
      after in the integration test).
- [ ] `POST /api/v1/billing-periods/:id/draft` is idempotent.
- [ ] Draft never deletes an `invoice_lines` or `invoice_adjustments` row.
- [ ] `total_due = opening_balance + current_charge + adjustment_total` holds on
      every drafted row (asserted by a query, not just by the Go struct).
- [ ] Draft on a `closed` period returns `409` with no writes.

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Concurrent draft from two tabs creates duplicate invoices | Medium | High | Upsert via `ON CONFLICT` on `uq_invoices` / `uq_invoice_line`, not read-then-write; whole operation in one tx |
| Recompute wipes a manual adjustment | Medium | High | Adjustments are never written by the draft path; `adjustment_total` is re-derived by summing them (step 1). Integration test asserts survival |
| Zeroed lines confuse the review screen | Low | Medium | Preview omits lines where `billable_count = 0` and `absent_count = 0` from the rendered list while keeping them in the DB |
| Draft on a large teacher (150 students, 3 classes) is slow | Low | Medium | One aggregate query plus batched upserts; measure in the integration test and fail over 2s. Indexes `idx_invoices_contact_period` (`docs/schema_design.sql:309`) and `idx_invoice_lines_invoice` (`:334`) already exist |
| Contact reassigned between draft and close changes `contact_id` silently | Low | Medium | Documented behaviour: draft follows the live contact, issued invoices freeze it. Preview response includes `contact_name` so the change is visible before close |

**Rollback.** Draft rows are `status='draft'` and never leave the system as
statements or notifications (plan 06 filters on `status <> 'draft'`). To back
out, void them (`status='void'` with a reason) or leave them — a subsequent
draft overwrites their figures. Reverting the code removes both endpoints
without touching phase 1.
