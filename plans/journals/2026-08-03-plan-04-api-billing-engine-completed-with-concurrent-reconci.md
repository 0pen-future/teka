---
title: Plan 04 API billing engine completed with concurrent-reconciliation fix
date: 2026-08-03
summary: "Billing periods, fee computation, period close, adjustments; fixed a concurrent post-close reconciliation double-count race"
---

# Plan 04 API billing engine completed with concurrent-reconciliation fix

## What happened

Completed all four phases of the API billing engine (billing periods, per-student fee
computation, chốt sổ period close, immutable invoice snapshots, adjustments, and
post-close reconciliation), then ran the mandatory review + test finalize chain.

- Phases 1–4 delivered sequentially: model/calculator → preview/draft → close/void →
  adjustments/reconciliation. Money is int64 VND end-to-end (no floats), invoices are
  immutable once issued (draft-only ON CONFLICT guard + period-open guard), and every
  query is teacher-scoped.
- Independent test pass: billing coverage 67.9%, whole-API gate 73.8% (floor 60%); all
  money-critical behaviours covered with DB-level assertions (double-count guard,
  closed-invoice immutability, source_session_id non-null, concurrent-close
  serialisation, unconfirmed-session blocking, total_due formula, draft idempotency,
  void payment guard + balance-view exclusion).

## Decision

Code review (money/immutability/tenancy) confirmed the core invariants hold for the
sequential path and surfaced items beyond the plan's scope:

- H1 (fixed): concurrent reconciliation of the same student's closed period could
  double-count the carried delta under READ COMMITTED. Added a SELECT ... FOR UPDATE on
  the closed-period invoice row, taken before reading already_adj, so the second
  reconciliation waits for the first to commit and then no-ops. Student iteration is
  sorted to keep lock ordering deadlock-free. New regression test proves the carry nets
  to a single −200,000 under -race, not −400,000.
- L4 (fixed): VoidInvoice now re-validates the reason at the Go layer (void_reason has
  no DB CHECK), matching AddAdjustment.
- M2/M3/L5: product-intent gaps (post-close roster removal, newly-billable on a
  voided/absent invoice, non-durable reconciliation retry). Recorded in adr.md as known
  V1 limitations corrected via manual adjustment; decision deferred to plan 05/06 rather
  than blocking plan 04 delivery.

All deviations are logged in adr.md. Lint clean (0 issues); full short suite and billing
integration suite green under -race.

## Next steps

- Plan 05 (payments & collections): owns the canonical invoice-status derivation that
  billing's local deriveInvoiceStatus stands in for, and the payment-reversal path M2/M3
  reference.
- Revisit M2/M3 auto-credit/charge intent during plan 05/06.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
