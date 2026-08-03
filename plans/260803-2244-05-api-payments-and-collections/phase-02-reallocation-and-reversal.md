---
phase: 2
title: "Reallocation, Reversal, and Invoice Status"
status: pending
priority: P1
effort: "5h"
dependencies: [1]
---

# Phase 2: Reallocation, Reversal, and Invoice Status

## Overview

The correction paths. D8 gives teachers an automatic split but explicitly
promises an override ("Giáo viên phải override được" — PRD Q8), and a
mis-recorded payment must be undoable without deleting a ledger row (schema note
(i), `docs/schema_design.sql:512`).

Three capabilities: manual reallocation, payment reversal, and re-running
auto-allocation for a payment that still has an unallocated remainder.

## Requirements

- Manual reallocation replaces a payment's split with teacher-supplied amounts
  and marks the rows `allocated_by='manual'`
  (`docs/schema_design.sql:392-393`).
- Reversal creates a **new** `payments` row with `reverses_payment_id` set to
  the original and stamps `reversed_at` on the original
  (`docs/schema_design.sql:371-374`). Nothing is deleted.
- After any of these, `invoices.paid_amount` and `status` are recomputed for
  every affected invoice.
- `paid_amount` never goes negative (CHECK, `docs/schema_design.sql:305`).
- A reversed payment cannot be reallocated or reversed again.

## Architecture

### Manual reallocation

`PUT /payments/:id/allocations` with `{ allocations: [{ invoice_id, amount }] }`.

Validation, all before any write:

- payment exists, belongs to the teacher, is not a reversal
  (`reverses_payment_id IS NULL`) and is not reversed (`reversed_at IS NULL`);
- every `invoice_id` belongs to the same teacher **and the same contact** as the
  payment — a parent's money cannot settle another family's bill;
- every invoice has `status IN ('issued','partially_paid','paid')`; `draft` and
  `void` are rejected;
- every `amount > 0`;
- `Σ amounts ≤ payment.amount`;
- for each invoice, `amount ≤ total_due − (paid_amount − existing allocation
  from this payment)`, i.e. the new amount may not push that invoice past its
  total.

Write, in one transaction: delete this payment's existing
`payment_allocations` rows, insert the new set with `allocated_by='manual'`,
then `RecalcInvoicePaid` for the union of old and new invoice ids — the union
matters, otherwise an invoice dropped from the split keeps a stale
`paid_amount`.

The hard delete of allocation rows is deliberate and is the one place this plan
removes a financial-table row. The justification: an allocation is a *link*
between two preserved facts (the payment and the invoice), not a fact itself;
the money record is untouched; and the alternative — reversing and re-recording
the payment — would misrepresent a split correction as a refund. Recorded as
OQ-3 in `plan.md`.

### Reversal

`POST /payments/:id/reverse` with `{ reason }` (stored in the reversal row's
`note`; there is no dedicated reason column).

In one transaction:

1. Lock and re-read the original; reject if `reversed_at IS NOT NULL` or
   `reverses_payment_id IS NOT NULL`.
2. Insert a new `payments` row: same `contact_id`, same `amount` (the CHECK at
   `docs/schema_design.sql:364` requires `amount > 0`, so the sign lives in
   `reverses_payment_id`, not the number), same `method`, `received_on` = today
   in the teacher's timezone, `reverses_payment_id` = original id, `note` =
   reason.
3. Mirror the original's allocations onto the reversal row: same invoices, same
   amounts, same `allocated_by`. This keeps `payment_allocations` reconcilable
   against `payments` one-to-one and makes the recompute formula symmetric.
4. Stamp `reversed_at = now()` on the original.
5. `RecalcInvoicePaid` for every affected invoice.

The recompute formula from phase 1 already handles the sign:

```
paid = Σ(alloc where payments.reverses_payment_id IS NULL)
     − Σ(alloc where payments.reverses_payment_id IS NOT NULL)
```

so a full reversal returns every touched invoice to exactly its prior
`paid_amount` and status. That is the property the tests assert directly.

Partial reversal is **not** offered in V1 (YAGNI): reverse in full and record a
new, correct payment.

### Re-running auto-allocation

`POST /payments/:id/allocations/auto` re-runs phase 1's `Allocate` for the
payment's **unallocated remainder** only. Existing allocations are left alone;
candidates exclude invoices this payment has already fully covered. This is the
manual answer to overpayment (OQ-2): after the next period closes, the teacher
presses it and the surplus lands on the new invoices.

Guard: refuse when the payment is reversed or is itself a reversal.

## Related Code Files

Create:

- `apps/api/internal/features/payments/reversal.go` — reversal + reallocation service methods
- `apps/api/internal/features/payments/reversal_test.go`

Modify:

- `apps/api/internal/features/payments/repository.go` — add
  `LockPayment`, `DeleteAllocations`, `AllocationsByPayment`,
  `MarkReversed`, `InvoicesByIDs`
- `apps/api/internal/features/payments/service.go` — add `Reallocate`,
  `Reverse`, `AutoAllocateRemainder`
- `apps/api/internal/features/payments/dto.go` — add `ReallocateRequest`,
  `ReverseRequest`, extend `PaymentResponse` with `reversed_at`,
  `reverses_payment_id`, `unallocated_amount`
- `apps/api/internal/features/payments/handler.go` — three handlers
- `apps/api/internal/features/payments/routes.go` — three routes
- `apps/api/internal/features/payments/integration_test.go` — reversal cases

Delete: none. No migration files.

## Implementation Steps

1. Add the repository methods listed above. `LockPayment` uses
   `SELECT ... FOR UPDATE`. `DeleteAllocations(ctx, teacherID, paymentID)`
   is the only delete in the package — name it explicitly and comment the
   invariant it preserves.
2. Implement `Reallocate` with the full validation list before the transaction
   opens, returning `apperror.Invalid` with per-field messages
   (`apps/api/internal/shared/apperror/apperror.go:55`) so the client can point
   at the offending row.
3. Implement `Reverse` following the five steps above.
4. Implement `AutoAllocateRemainder`: compute
   `remainder = payment.amount − Σ existing allocations`, return `409` when it
   is zero, otherwise reuse `Allocate` and insert with `allocated_by='auto'`,
   merging into any existing row for the same invoice via
   `ON CONFLICT (payment_id, invoice_id) DO UPDATE SET amount = amount +
   EXCLUDED.amount`.
5. Add the DTOs. `ReallocateRequest.Allocations` uses
   `binding:"required,min=1,dive"` with `amount` `gt=0`.
6. Add routes behind `requireAuth`:
   - `PUT  /payments/:id/allocations`
   - `POST /payments/:id/allocations/auto`
   - `POST /payments/:id/reverse`
7. Write `reversal_test.go` against a fake repository for the validation
   branches: reallocating a reversed payment, allocating to another contact's
   invoice, `Σ amounts` exceeding the payment, an amount exceeding an invoice's
   room, a draft invoice target, double reversal.
8. Extend `integration_test.go`:
   - record a payment, snapshot every affected invoice's `paid_amount`/`status`,
     reverse it, assert every one returned to its snapshot value;
   - assert the original row still exists with `reversed_at` set and the
     reversal row exists with `reverses_payment_id` set;
   - reverse twice → second attempt `409`, no new rows;
   - reallocate a two-child split from 70/30 to 100/0 → the emptied invoice
     drops back to `issued`, the other becomes `paid`, both rows are
     `allocated_by='manual'`;
   - reallocate to an invoice of a different contact → `422`, nothing written;
   - overpay, close the next period, run auto-allocate-remainder → surplus
     lands on the new invoice and `unallocated_amount` becomes 0;
   - after every scenario, assert the global invariant
     `Σ over invoices(paid_amount) == Σ(non-reversal allocations) − Σ(reversal
     allocations)`.
9. Run `go test ./apps/api/internal/features/payments/... -race`.

## Success Criteria

- [ ] R7: a reversed payment restores every affected invoice to its exact prior
      `paid_amount` and `status`.
- [ ] Both the original and the reversal remain queryable; no `payments` row is
      ever deleted.
- [ ] Manual reallocation writes `allocated_by='manual'` and recomputes the
      union of old and new invoices.
- [ ] Reallocation cannot move money to another contact's invoice, cannot exceed
      the payment, and cannot push an invoice past `total_due`.
- [ ] `paid_amount >= 0` holds after every operation, including double
      reversal attempts.
- [ ] The ledger invariant in step 8 holds after every integration scenario.
- [ ] Reversing a payment then voiding its invoice succeeds (unblocking plan 04
      phase 3's `paid_amount = 0` guard).

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Reallocation forgets to recompute an invoice it removed money from | High | Critical | Recompute over the **union** of old and new invoice ids, stated in Architecture and asserted by the 70/30 → 100/0 test |
| Double reversal drives `paid_amount` negative | Medium | High | `reversed_at` guard under `FOR UPDATE`; the CHECK at `docs/schema_design.sql:305` is the backstop; explicit test |
| Mirrored reversal allocations double-count instead of cancelling | Medium | Critical | Recompute formula subtracts allocations of reversal payments; snapshot-and-restore test proves it exactly |
| Hard-deleted allocation rows lose audit history | Medium | Low | Accepted for V1 (OQ-3); the `payments` row and the new split are both preserved |
| Reallocation races with a second payment for the same contact | Low | High | `LockPayment` plus `FOR UPDATE` on the target invoices inside the same tx |
| Teacher reverses a payment for an invoice in a closed period | Medium | Low | Allowed and correct — reversal touches `paid_amount`/`status` only, never the frozen snapshot columns |

**Rollback.** Every operation here is expressible as another operation: a wrong
reversal is corrected by recording a fresh payment; a wrong reallocation by
reallocating again. Reverting the code leaves all rows valid, and
`RecalcInvoicePaid` can rebuild `paid_amount` for the whole database from
`payment_allocations` alone.
