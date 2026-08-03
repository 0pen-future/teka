---
phase: 1
title: "Payment Recording and Auto-Allocation"
status: pending
priority: P1
effort: "6h"
dependencies: []
---

# Phase 1: Payment Recording and Auto-Allocation

## Overview

Creates the `payments` feature package and the allocation engine — the bridge
between the two axes of PRD R7. A teacher records one amount against one
contact; the engine decides how much of it settles each child's invoice, per D8.

The allocator is a pure function so its rules can be exercised exhaustively
without a database. Every allocation edge case in the PRD lives in its unit
test file.

## Requirements

- `payments`: `amount > 0` (CHECK, `docs/schema_design.sql:364`), `method` in
  `cash | transfer | other` (`:366`), `received_on` required, optional
  `reference_code` (≤50 chars) and `note`.
- Payment belongs to a contact of the calling teacher; the composite FK
  `(contact_id, teacher_id)` (`docs/schema_design.sql:377`) already prevents
  cross-teacher writes, but the service checks first so the client gets a 404
  rather than a 500.
- Allocation per D8: opening-balance portion of every candidate invoice first,
  then current charge; ties by earlier class start date, then invoice id.
- `payment_allocations.amount > 0` (`docs/schema_design.sql:389`) — zero-value
  allocations are skipped, not written.
- Allocation into one invoice never exceeds that invoice's outstanding.
- `invoices.paid_amount` and `status` recomputed in the same transaction.

## Architecture

New package `apps/api/internal/features/payments`, mirroring the layout of
`apps/api/internal/features/users`.

**Models** (`model.go`) — neither table has `deleted_at`, so neither model gets
`gorm.DeletedAt`:

- `Payment` → `payments` (`docs/schema_design.sql:360-379`)
- `PaymentAllocation` → `payment_allocations` (`docs/schema_design.sql:384-398`)

Constants mirroring the CHECK lists:

```go
const (
    MethodCash     = "cash"
    MethodTransfer = "transfer"
    MethodOther    = "other"

    AllocatedAuto   = "auto"
    AllocatedManual = "manual"
)
```

**The allocator** (`allocator.go`) is pure:

```go
type Candidate struct {
    InvoiceID      uuid.UUID
    PeriodStart    time.Time
    EarliestClassStart *time.Time // nil when the invoice has no lines
    OpeningBalance int64
    TotalDue       int64
    PaidAmount     int64
}

type Allocation struct {
    InvoiceID uuid.UUID
    Amount    int64
}

// Allocate distributes amount across candidates per D8 and returns the
// allocations plus whatever could not be placed.
func Allocate(amount int64, candidates []Candidate) (allocs []Allocation, unallocated int64)
```

**Ordering.** Candidates are sorted once by:

1. `PeriodStart` ascending — older periods are older debt.
2. `EarliestClassStart` ascending, `nil` last. An invoice with no lines is pure
   carry-over debt and is fully consumed by pass one anyway, so its position in
   pass two does not matter.
3. `InvoiceID` ascending — makes the result reproducible, which matters because
   the test suite and the teacher both need the same answer twice.

**Two passes over that same order:**

```
outstanding(c)     = c.TotalDue - c.PaidAmount                       (>= 0)
opening_unpaid(c)  = max(0, min(c.OpeningBalance - c.PaidAmount, outstanding(c)))
rest_unpaid(c)     = outstanding(c) - opening_unpaid(c)

pass 1: for c in order: take = min(remaining, opening_unpaid(c))
pass 2: for c in order: take = min(remaining, rest_unpaid(c))
```

Within an invoice, existing `paid_amount` is treated as having settled the
opening balance first. That is what makes `opening_unpaid` shrink correctly
across repeated part-payments, and it means a negative `adjustment_total` can
never produce a negative `rest_unpaid`.

Both passes accumulate into a per-invoice map, so an invoice touched by both
passes yields **one** `payment_allocations` row — required by
`uq_payment_allocations (payment_id, invoice_id)`
(`docs/schema_design.sql:397`).

`remaining` after pass two is returned as `unallocated`.

**Candidate query.** Invoices of the contact eligible for payment:

```sql
SELECT ... FROM invoices i
JOIN billing_periods bp ON bp.id = i.period_id AND bp.teacher_id = i.teacher_id
LEFT JOIN LATERAL (
    SELECT min(cl.start_date) AS earliest_class_start
    FROM invoice_lines il
    JOIN enrollments e ON e.id = il.enrollment_id
    JOIN classes    cl ON cl.id = e.class_id
    WHERE il.invoice_id = i.id
) lc ON true
WHERE i.teacher_id = :teacher_id
  AND i.contact_id = :contact_id
  AND i.status IN ('issued', 'partially_paid')
  AND i.total_due > i.paid_amount
ORDER BY bp.period_start, lc.earliest_class_start NULLS LAST, i.id
```

`draft` invoices are excluded — money cannot settle a bill that has not been
issued. `void` and `paid` are excluded by the same predicate.

**`paid_amount` recompute** (`recalcInvoicePaid`), run for every invoice touched,
inside the same transaction:

```sql
UPDATE invoices i SET
  paid_amount = COALESCE(x.paid, 0),
  status = CASE
      WHEN i.status = 'void' THEN i.status
      WHEN COALESCE(x.paid,0) >= i.total_due AND i.total_due > 0 THEN 'paid'
      WHEN COALESCE(x.paid,0) > 0 THEN 'partially_paid'
      ELSE 'issued'
  END,
  updated_at = now()
FROM (
  SELECT pa.invoice_id,
         SUM(CASE WHEN p.reverses_payment_id IS NULL THEN pa.amount ELSE -pa.amount END) AS paid
  FROM payment_allocations pa
  JOIN payments p ON p.id = pa.payment_id
  WHERE pa.invoice_id = :invoice_id
  GROUP BY pa.invoice_id
) x
WHERE i.id = :invoice_id AND i.teacher_id = :teacher_id
```

Three properties: it is idempotent, it never touches a `void` or `draft`
invoice's status, and it can never drift because it recomputes rather than
increments. The `CHECK (paid_amount >= 0)` at `docs/schema_design.sql:305` is
the backstop if the sign logic is ever wrong.

## Related Code Files

Create:

- `apps/api/internal/features/payments/model.go`
- `apps/api/internal/features/payments/allocator.go` — pure `Allocate`
- `apps/api/internal/features/payments/allocator_test.go`
- `apps/api/internal/features/payments/repository.go`
- `apps/api/internal/features/payments/service.go`
- `apps/api/internal/features/payments/service_test.go`
- `apps/api/internal/features/payments/dto.go`
- `apps/api/internal/features/payments/handler.go`
- `apps/api/internal/features/payments/routes.go`
- `apps/api/internal/features/payments/integration_test.go`

Modify:

- `apps/api/internal/server/router.go` — register the feature in
  `registerFeatures` (`apps/api/internal/server/router.go:63-73`)

Delete: none. No migration files.

## Implementation Steps

1. Create `model.go` with `Payment` and `PaymentAllocation`, `TableName()` on
   each, no `gorm.DeletedAt`, plus the constants above.
2. Create `allocator.go` with `Allocate` exactly as specified. Keep it free of
   `uuid` generation, clock reads, and errors — it returns values only.
3. Create `allocator_test.go`, table-driven, covering:
   - exact payment clears one invoice;
   - exact payment clears two children's invoices;
   - underpayment fills the older period first and leaves the newer
     `partially_paid`;
   - underpayment within one period splits by earlier class start date;
   - opening balance across two invoices is settled before either current
     charge (the core D8 assertion);
   - an invoice with `nil EarliestClassStart` sorts last in pass two;
   - overpayment returns the surplus as `unallocated`;
   - an invoice already partially paid has its `opening_unpaid` reduced
     correctly;
   - a negative `adjustment_total` invoice never yields a negative allocation;
   - zero-amount allocations are omitted entirely;
   - `Σ allocations + unallocated == amount` for every case (the invariant that
     catches any arithmetic slip).
4. Create `repository.go` with `Repository` interface + `gormRepository` using
   `database.FromContext(ctx, r.db)`
   (`apps/api/internal/features/users/repository.go:50`). Methods:
   `CreatePayment`, `GetPayment`, `ListPayments`, `CandidateInvoices`,
   `InsertAllocations`, `RecalcInvoicePaid`, `ContactExists`. All take
   `teacherID`.
5. Create `service.go` with `Service{repo, tx}`. Implement
   `Record(ctx, teacherID, req)`:
   1. validate the contact belongs to the teacher → `apperror.NotFound("contact")`;
   2. inside `tx.WithinTx`: insert the `payments` row (UUIDv7 from the shared
      util, D3); load candidates; call `Allocate`; insert allocations with
      `allocated_by='auto'`; `RecalcInvoicePaid` for each touched invoice;
   3. return the payment with its allocations and `unallocated_amount`.
6. Create `dto.go`: `RecordPaymentRequest` (`contact_id` required uuid,
   `amount` required `gt=0`, `method` `oneof=cash transfer other`,
   `received_on` required date, `reference_code` `omitempty,max=50`, `note`
   `omitempty,max=1000`), `PaymentResponse`, `AllocationResponse`. All money
   fields `int64`.
   `AllocationResponse` carries `invoice_id`, `student_id`, `student_name`,
   `period_id`, `amount`, `allocated_by`, plus the target invoice's `total_due`,
   `paid_amount` and `outstanding` **after** the allocation. Every read response
   that returns a payment includes this breakdown. The reason is a hard contract
   requirement from web plan 08: the frontend must never recompute the D8
   oldest-debt-first rule client-side, so the server has to say plainly which
   child each đồng landed on. Two implementations of an allocation rule is how a
   parent gets told two different numbers.
7. Create `handler.go` / `routes.go`, all behind `requireAuth`:
   - `POST /payments` → record
   - `GET  /payments` → paginated list, filters `contact_id`, `period_id`,
     `received_on` range; default sort `-received_on`
   - `GET  /payments/:id` → one payment with its allocations
   Follow the `listSorts` whitelist pattern at
   `apps/api/internal/features/users/handler.go:17-21`.
8. Register in `registerFeatures` (`apps/api/internal/server/router.go:63`)
   using the existing `txMgr` at line 66.
9. Create `integration_test.go` over the real schema:
   - contact with two children, both invoiced, exact payment → both `paid`,
     two allocation rows, `paid_amount` matches `total_due`;
   - underpayment → older/earlier-class invoice fully paid, other
     `partially_paid` with the correct shortfall;
   - overpayment → `unallocated_amount` returned, no allocation exceeds an
     invoice's outstanding, `Σ allocations ≤ payment.amount`;
   - a draft invoice is never allocated to;
   - a void invoice is never allocated to;
   - payment against another teacher's contact → `404`, nothing written;
   - two concurrent payments for the same contact → no allocation drives
     `paid_amount` above `total_due` (`-race`, with row locking on the
     candidate invoices).
10. Run `go test ./apps/api/internal/features/payments/... -race`.

## Success Criteria

- [ ] `POST /api/v1/payments` records the payment and allocates in one
      transaction; a forced failure after insert leaves zero rows.
- [ ] Allocation follows D8: opening balance across all invoices before any
      current charge, ties by earlier class start date.
- [ ] `Σ allocations + unallocated == payment.amount` for every recorded payment.
- [ ] No allocation exceeds its invoice's outstanding.
- [ ] `invoices.status` moves `issued` → `partially_paid` → `paid` correctly and
      never leaves `void` or `draft`.
- [ ] `paid_amount` is recomputed, never incremented (verified by running the
      recompute twice and getting the same value).
- [ ] `Allocate` unit tests cover all eleven cases in step 3.
- [ ] Every read response containing a payment also returns the per-invoice
      allocation breakdown with post-allocation `outstanding`, so no client ever
      needs to reimplement the D8 rule.

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Concurrent payments overpay an invoice | Medium | High | Candidate invoices selected `FOR UPDATE` inside the tx so two payments for one contact serialise; race test in step 9 |
| Allocation loses or invents đồng | Low | Critical | Integer-only arithmetic, no division, no rounding; the `Σ allocations + unallocated == amount` invariant asserted in every unit test |
| `paid_amount` drifts from allocations | Medium | Critical | Always recomputed from `payment_allocations`, never incremented; idempotency asserted |
| D8 ordering is wrong for real teachers | High | Medium | Rule isolated in one comparator; `allocated_by` mix measures it (OQ-1). Changing it is a one-function change plus its test |
| `uq_payment_allocations` violated when an invoice is touched by both passes | Medium | Medium | Passes accumulate into a per-invoice map before any insert (Architecture) |
| Cross-teacher contact accepted, composite FK raises a 500 | Low | Medium | Service checks contact ownership first and returns `404` |

**Rollback.** Payments and allocations are append-only; nothing here mutates
prior data except the derived `paid_amount`/`status`, which can be rebuilt for
every invoice by re-running `RecalcInvoicePaid`. Reverting the code leaves the
recorded payments intact and the invoices correct.
