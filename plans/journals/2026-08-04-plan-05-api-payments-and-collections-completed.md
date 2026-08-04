---
title: Plan 05 API payments and collections completed
date: 2026-08-04
summary: "Payments (D8 allocation, reversal, reallocation) + read-only collection board delivered; fixed a recompute cross-join no-op and unified lock ordering to remove a reallocation deadlock"
---

# Plan 05 API payments and collections completed

## What happened

Delivered all three phases of plan 05 (payments & collections) for the Go API.

- Phase 1 `payments` package: pure D8 two-pass `Allocate` (opening balance across
  all invoices first, then current charge; ordered period_start → earliest class
  start NULLS LAST → invoice id), transactional payment recording with candidate
  invoices locked FOR UPDATE, and `RecalcInvoicePaid` that re-derives
  paid_amount/status from allocations (never increments). Money int64 end to end.
- Phase 2 correction paths: manual reallocation (allocated_by='manual', union
  recompute), reversal (new counter-entry payments row + mirrored allocations +
  reversed_at stamp, nothing deleted), and re-run auto-allocation for a payment's
  unallocated remainder.
- Phase 3 read-only `collections` package: by-contact (default, over
  v_contact_balance) and by-class board views plus a period summary, with the one
  documented D4 exception (soft-deleted contacts with debt stay visible, flagged
  contact_archived).

## Decision

Two real defects were found and fixed during implementation/review, both money-
or availability-critical:

- The recompute SQL used a derived table with no join predicate against invoices,
  so when a reallocation deleted every allocation of an invoice the UPDATE matched
  zero rows and left the invoice at a stale paid_amount/status. Fixed by anchoring
  a single-row target and LEFT JOINing the allocation aggregate so a zeroed
  invoice is always matched.
- Concurrent contact-scoped writes could deadlock: reallocation locked only the
  new target invoices yet recomputed the union of old+new, reversal locked none
  before recompute, recompute looped over a Go map (random order), and the
  candidate query locked by period_start while InvoicesByIDs locked by id. Unified
  every write path onto one lock order (invoice id): reallocation now locks the
  full union, reversal locks the affected invoices, recompute iterates id-sorted,
  and the candidate query orders by id (safe — Allocate re-sorts by D8 before
  allocating). New regression test runs two crossing reallocations under -race and
  asserts no internal/deadlock error with the ledger still balanced.

Also hardened recompute to preserve draft (not only void) status, and recorded
two product-intent limitations (reversal row drops reference_code; unallocated
credit is global per contact) as V1 decisions in adr.md.

Verification: whole-repo unit suite green; payments+collections integration
suites green under -race; make test-api total coverage 71.5% (floor 60%);
make lint-api 0 issues; swagger regenerated; no float types anywhere.

## Next steps

- Plan 06 (statements & notifications) — next in the API chain; unblocked by 05.
- Then the web design-system foundation (260803-2325), plan 07 (teacher app),
  plan 08 (parent statement page).
- Revisit OQ-1 (D8 default) and OQ-2 (overpayment credit) once real usage data
  from the auto/manual allocation mix exists.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
