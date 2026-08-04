---
title: "05 API Payments and Collections"
description: "Contact-level payment recording, automatic allocation across a family's invoices, reversal, and the two-axis collection board (by contact, by class)."
status: completed
priority: P1
effort: "16h"
branch: HEAD
tags: [api, go, payments, collections, money]
created: 2026-08-03
blockedBy: [260803-2244-04-api-billing-engine]
blocks: [260803-2244-06-api-statements-and-notifications]
---

# 05 API Payments and Collections

## Overview

Implements PRD **R7**, the two-axis model that is the point of the whole data
design: **debt is recorded per student, money is received per contact**
(`docs/schema_design.sql:27`). A parent hands over one amount for however many
children; the system splits it across those children's invoices and can still
answer "who in class 9A has paid?".

Two feature packages:

- `payments` — writes. Records `payments`, computes `payment_allocations`,
  maintains `invoices.paid_amount` and `invoices.status`, handles reversal.
- `collections` — reads. The two board views over `v_contact_balance`
  (`docs/schema_design.sql:459`) and the invoice/line tables.

Splitting write from read keeps file ownership clean and stops a reporting query
from accidentally acquiring write concerns.

## Scope

In scope:

- Record a payment against a contact: `amount > 0`, `method`, `received_on`,
  optional `reference_code` and `note` (`docs/schema_design.sql:360-379`).
- Auto-allocation per D8, writing `payment_allocations` with
  `allocated_by='auto'`.
- Manual reallocation, writing `allocated_by='manual'`.
- Reversal via `reverses_payment_id`, never delete
  (`docs/schema_design.sql:371-374`).
- `invoices.paid_amount` and `status` maintenance:
  `issued` → `partially_paid` → `paid`.
- Collection board: by-contact (default), by-class, summary, unpaid filter.

Non-goals:

- Payment gateway or bank reconciliation. V1 shows a QR and the teacher confirms
  manually (PRD §3 Non-Goals).
- Automatic transaction matching from `reference_code` — that is P1
  (`docs/schema_design.sql:368`).
- Automatic debt reminders after X days — P1.
- Any schema change (D1). No new migration files.

## Phases

| # | Phase | Effort | Depends on | Status |
|---|-------|--------|-----------|--------|
| 1 | [Payment recording and auto-allocation](./phase-01-payment-recording-and-auto-allocation.md) | 6h | — | Completed |
| 2 | [Reallocation, reversal, and invoice status](./phase-02-reallocation-and-reversal.md) | 5h | 1 | Completed |
| 3 | [Collection board views](./phase-03-collection-board.md) | 5h | 2 | Completed |

## Key decisions

- **D8 (answers PRD Q8, adopting the PRD's own proposed default).** Automatic
  allocation pays **oldest debt first**: the carried `opening_balance` portion of
  every candidate invoice before any current charge, ties broken by the earlier
  class start date, then by invoice id for determinism. Rows are written with
  `allocated_by='auto'`; a teacher override rewrites them as `'manual'`. The
  `auto`/`manual` ratio is the measurement that tells us whether the rule
  matches reality (`docs/schema_design.sql:390-391`).
- **D4** — every query is scoped by `teacher_id`. **Documented exception:** the
  collection board joins `contacts` **without** `deleted_at IS NULL`, because a
  soft-deleted contact's debt must stay collectable (PRD §5: "một con nghỉ hẳn,
  con kia còn học → không xoá lịch sử công nợ"). Soft-deleted contacts are
  flagged in the response, not hidden.
- **D5** — money is `BIGINT` VND; allocation is exact integer arithmetic with no
  rounding step, so a split can never lose or invent a đồng.
- **No soft delete on `payments` or `payment_allocations`** (schema note (i),
  `docs/schema_design.sql:512`). A mistaken payment is reversed with a
  counter-entry, the accounting way.
- **`paid_amount` is derived, never incremented.** After any allocation change
  it is recomputed from scratch:
  `Σ allocations of non-reversal payments − Σ allocations of reversal payments`.
  Incremental `+=` drifts the moment one transaction retries.
- **Reversal rows mirror the original's allocations.** The reversal `payments`
  row carries its own `payment_allocations` with the same invoices and amounts.
  This keeps `payment_allocations` reconcilable one-to-one against `payments`
  and makes the recompute formula above sign-symmetric.
- **Underpayment is visible per child.** A family that pays less than the total
  leaves some invoices `partially_paid`; the by-class view shows exactly which
  child is short and by how much (R7 acceptance).
- **Overpayment stays unallocated.** The schema has no credit table, so the
  surplus is simply `payment.amount − Σ allocations` and is surfaced as
  `unallocated_amount`. See OQ-2.

## Acceptance criteria

From PRD R7.

- [x] Recording a payment at contact level allocates across that contact's
      children's invoices without further input.
- [x] A parent with two children who pays in full: both children show "đã đóng"
      (paid) in the by-class view.
- [x] A parent with two children who underpays: the by-class view shows which
      child is short and by how much.
- [x] Marking a payment persists — reopening the board later shows the same
      state with the debt reduced.
- [x] With 150 students, the unpaid group can be filtered in one request.
- [x] The by-contact view is the default: one row per parent, all children
      merged, with total due / paid / outstanding.
- [x] Period totals show collected and outstanding.
- [x] A reversed payment restores the invoices to their prior status and
      `paid_amount`, and both the original and the reversal remain in the
      ledger.
- [x] Allocation never exceeds an invoice's outstanding, and `paid_amount` never
      goes negative (the CHECK at `docs/schema_design.sql:305`).

## Open questions

- **OQ-1.** D8 is the PRD's proposed default and is explicitly flagged in Q8 as
  unvalidated — many teachers may treat the balance as one family debt rather
  than splitting per child. The implementation makes this cheap to revisit: the
  ordering rule is one comparator function, and the `allocated_by` mix tells us
  when it is wrong. No user research has happened yet.
- **OQ-2.** Overpaid amounts remain unallocated with no automatic application to
  a future period's invoices, because triggering that at close would couple plan
  05 back into plan 04. The teacher re-runs auto-allocation manually after the
  next close. Confirm this is acceptable, or accept the coupling.
- **OQ-3.** A manual reallocation hard-deletes and rewrites that payment's
  `payment_allocations` rows. This is a re-derivable link, not a ledger entry —
  the `payments` row (the money fact) is never touched — but it does mean the
  previous split leaves no trace. If auditability of splits matters, the
  alternative is a reversal-and-reallocate flow. Recommend the simple version
  for V1.
- **OQ-4.** Voiding an invoice that has allocations is blocked by plan 04
  (phase 3) and requires reversing the payment first. Confirm teachers find that
  ordering acceptable rather than having the system cascade automatically.

<!-- slug: 05-api-payments-and-collections -->
