---
phase: 3
title: "Period Close and Void"
status: pending
priority: P1
effort: "6h"
dependencies: [2]
---

# Phase 3: Period Close and Void

## Overview

Implements **chốt sổ** (period close — literally "closing the books"): the
irreversible action that freezes a month's figures and turns draft invoices into
issued debt. Also implements invoice void, the only correction path available
once an invoice is issued (schema note (i), `docs/schema_design.sql:512`).

The close is the single highest-stakes write in the product. Everything a parent
is later told comes from what this transaction records.

## Requirements

- R4: closing is **hard blocked** when any session inside the period has already
  happened but has no `attendance_confirmed_at`. The response must name the
  offending sessions, not just refuse.
- R4: confirming close locks the period (`billing_periods.status='closed'`,
  `closed_at` set) and issues the invoices.
- Invoice snapshots are immutable after issue: `student_name`, `contact_name`,
  `class_name`, `unit_price`, all counts and amounts.
- PRD §5: a class with no sessions in the period yields no invoice and no
  notification.
- The whole close is one transaction. A failure leaves the period `open` with
  no partially issued invoices.

## Architecture

**Close pipeline**, all inside a single `tx.WithinTx`:

```
1. Load period FOR UPDATE, assert status='open'          -> else 409
2. BlockingSessions(teacher, period)                     -> if any, 409 + list
3. ComputePeriod  (phase 2)                              -> []ComputedInvoice
4. Upsert drafts  (phase 2)                              -> persisted drafts
5. Void empty drafts                                     -> status='void'
6. Issue remaining drafts                                -> status='issued'
7. period.status='closed', closed_at=now()
```

**Blocking check (step 2) reuses plan 03's pending-attendance feed.** Billing
does **not** write its own "unconfirmed sessions in period" query. Plan 03 phase
3 exposes the pending-attendance feed with `from`/`to` date filtering
specifically so this close block and the teacher's dashboard warning agree by
construction:

```
blocking = pendingFeed(teacherID, from: period.period_start,
                                  to:   min(period.period_end, today))
warnings = pendingFeed(teacherID, from: today + 1 day,
                                  to:   period.period_end)
```

`today` is resolved once from `teachers.timezone` at the top of the pipeline.
**[verify the feed's exact package, function name and result type in plan 03
phase 3 before implementing]**

This is not a style preference. A divergent predicate would block closing on
sessions the dashboard never warned about — the teacher would hit a wall they
had no way to see coming, on the one action they cannot undo.

Three properties of the feed that billing depends on and asserts in its own
integration tests. A failure means plan 03 changed, and the fix belongs there:

- **`planned` past sessions are included.** A session still marked `planned`
  after its date has passed is exactly the "you forgot to take attendance" case
  R4 exists to catch. Plan 03's migration `000003` widens
  `idx_class_sessions_pending` (`docs/schema_design.sql:218-220`) to cover it,
  so this is indexed rather than a scan.
- **`cancelled` sessions are excluded.** The CHECK at
  `docs/schema_design.sql:212` forbids a cancelled session from ever carrying
  `attendance_confirmed_at`, so including them would make close permanently
  impossible.
- **Soft-deleted sessions are excluded.**

Sessions dated **after** today but inside the period do **not** block — closing
early is legal. They come back in the `warnings` array, and any attendance later
confirmed on them flows into the next period through the phase 4 adjustment
path.

If the feed's result type lacks a field the `409` payload needs (class name,
session status), map it in billing rather than asking plan 03 to widen its
contract for one consumer.

**Response on block** — `409 CONFLICT` with the offending list, because R4's
acceptance criterion is "chỉ rõ buổi nào" (point at exactly which sessions):

```json
{
  "error": {
    "code": "CONFLICT",
    "message": "period has unconfirmed sessions",
    "details": {
      "unconfirmed_sessions": [
        { "session_id": "...", "class_id": "...", "class_name": "Toán 9A",
          "session_date": "2026-08-12", "status": "held" }
      ]
    }
  }
}
```

`apperror.AppError` currently carries `Fields map[string]string`
(`apps/api/internal/shared/apperror/apperror.go:29-30`), which cannot hold a
list. Rather than widen the shared error type for one caller, the handler
returns this payload directly with `409` via `response.OK(c,
http.StatusConflict, ...)`-style construction, or a small
`response.ErrWithDetails` helper is added. Pick one during implementation and
apply it consistently; do not change `AppError`'s existing contract.

**Void-empty rule (step 5).** A drafted invoice is voided rather than issued
when `current_charge = 0 AND opening_balance = 0 AND adjustment_total = 0`.
Rationale: `v_contact_balance` (`docs/schema_design.sql:459`) already excludes
`status='void'`, and plan 06 generates statements only for non-void invoices, so
a voided empty invoice satisfies "lớp chưa có buổi nào trong kỳ → không sinh
phiếu thu, không gửi thông báo" without inventing a new state. The invoice row
survives as evidence the student was considered and correctly charged nothing.
`void_reason` is set to a fixed sentence; `voided_at` is required by the CHECK
at `docs/schema_design.sql:307`.

**Immutability after issue.** Enforced in Go, not by the DB (there is no
trigger; schema note (l), `docs/schema_design.sql:540`). Concretely:

- `UpsertInvoice` refuses to update any invoice whose `status <> 'draft'`
  (phase 2, step 1).
- The repository exposes no method that updates `invoice_lines` on a non-draft
  invoice.
- Only two writers touch an issued invoice, both narrow: `paid_amount`/`status`
  from plan 05, and `adjustment_total`/`total_due` from a same-period
  adjustment. Everything else is frozen.

**Void an issued invoice.** `POST /invoices/:id/void` with a required `reason`.
Guards:

- period may be closed (that is the normal case);
- `status` must be `issued` or `partially_paid`;
- `paid_amount` must be `0`. A paid invoice must have its payment reversed first
  (plan 05) — voiding it would leave allocated money pointing at a void invoice
  and silently change `v_contact_balance`. Return `409` naming the payment.

Void sets `status='void'`, `voided_at=now()`, `void_reason`. It does not delete
anything.

**Reopen is not supported.** `billing_periods.status` has no path back from
`closed` (`docs/schema_design.sql:263`). Corrections after close go through
adjustments (phase 4) or void. State this in the API docs so the frontend does
not offer a reopen button.

## Related Code Files

Create:

- `apps/api/internal/features/billing/close.go` — blocking check, close pipeline, void
- `apps/api/internal/features/billing/close_test.go` — unit tests over a fake repo

Modify:

- `apps/api/internal/features/billing/repository.go` — add `LockPeriod`,
  `IssueDraftInvoices`, `VoidInvoices`, `ClosePeriod`, `GetInvoice`. **No
  session-scanning method** — pending sessions come from plan 03's feed
- `apps/api/internal/features/billing/service.go` — add `Close`, `VoidInvoice`
- `apps/api/internal/features/billing/dto.go` — add `CloseResponse`,
  `UnconfirmedSession`, `VoidInvoiceRequest`
- `apps/api/internal/features/billing/handler.go` — add `close`, `voidInvoice`
- `apps/api/internal/features/billing/routes.go` — add the two routes
- `apps/api/internal/features/billing/integration_test.go` — close scenarios
- `apps/api/internal/shared/response/response.go` — **only if** the
  `ErrWithDetails` helper route is chosen over an inline handler response

Delete: none. No migration files.

## Implementation Steps

1. Locate plan 03 phase 3's pending-attendance feed and confirm its package,
   function name, parameters and result type. Take the attendance service as a
   constructor dependency of the billing service; do not add a session query to
   billing's repository.
2. Add `blockingSessions(ctx, teacherID, period, today)` and
   `futureUnconfirmedSessions(...)` as thin **unexported** wrappers in
   `close.go` that call the feed with the two date windows above and map the
   result into `[]UnconfirmedSession{SessionID, ClassID, ClassName,
   SessionDate, Status}`, ordered by `session_date, class_name`. If the feed
   does not carry `class_name`, resolve it in the mapper.
3. Add `LockPeriod(ctx, teacherID, periodID)` issuing
   `SELECT ... FOR UPDATE` on `billing_periods` so two concurrent close calls
   serialise instead of double-issuing.
4. Write `Close` in `close.go` following the seven pipeline steps. Resolve
   `today` from the teacher's timezone, once, at the top — never call
   `time.Now()` deeper in the pipeline.
5. Implement step 5 (void empty) and step 6 (issue) as two bulk `UPDATE`
   statements filtered on `period_id`, `teacher_id`, and `status='draft'`, so
   the number of statements does not grow with student count.
6. Implement `ClosePeriod` setting `status='closed'`, `closed_at`, `updated_at`,
   guarded by `WHERE status = 'open'`; assert `RowsAffected = 1` and abort the
   tx otherwise.
7. Build `CloseResponse`: `period`, `issued_count`, `voided_count`,
   `total_due`, `warnings.future_unconfirmed_sessions[]`.
8. Add `POST /billing-periods/:id/close` and `POST /invoices/:id/void`. Both
   behind `requireAuth`, both teacher-scoped.
9. Implement `VoidInvoice` with the three guards from Architecture. Reason is
   `binding:"required,min=3,max=500"`; the DB CHECK at
   `docs/schema_design.sql:351` rejects blank reasons on adjustments and the
   same discipline applies here by convention.
10. Extend `integration_test.go` with, at minimum:
    - close with a past unconfirmed `held` session → `409`, list contains that
      session, period still `open`, zero invoices issued;
    - close with a past unconfirmed `planned` session → same;
    - **agreement check:** for a period seeded with a mix of confirmed,
      unconfirmed, cancelled and future sessions, the set returned by plan 03's
      pending feed over the period window equals the set in the close `409`
      payload. This is the assertion that prevents the two predicates from
      drifting apart;
    - close with a `cancelled` session in the period → succeeds, that session
      bills nobody;
    - close with a future unconfirmed session → succeeds, warning present;
    - happy path: three students with different session counts → three invoices
      with three different `total_due` values (R3 acceptance);
    - student in two classes → one invoice, two lines (R1 acceptance);
    - class with zero sessions → no line; a student with no sessions and no
      carried debt → invoice voided, not issued;
    - student who quit mid-period with carried debt → invoice issued with
      `current_charge` from their last sessions and non-zero `opening_balance`;
    - after close, `POST .../draft` returns `409` and no row changes;
    - concurrent close (two goroutines) → exactly one succeeds, the other gets
      `409`, and `SELECT count(*) FROM invoices` matches the single-run count;
    - void an issued invoice → excluded from `v_contact_balance`;
    - void an invoice with `paid_amount > 0` → `409`.
11. Run `go test ./apps/api/internal/features/billing/... -race`.

## Success Criteria

- [ ] R4: close is blocked by any past unconfirmed session and the response
      names each one (id, class name, date, status).
- [ ] R4: successful close sets `billing_periods.status='closed'` and
      `closed_at`, and every draft invoice becomes `issued` or `void`.
- [ ] R3: three students with different session counts get three different
      amounts, each equal to `Σ billable_count × unit_price` plus carried debt.
- [ ] R3: carried debt appears in `opening_balance`, visibly separate from
      `current_charge`.
- [ ] A class with zero sessions in the period produces no invoice line.
- [ ] Close is atomic: an injected failure after step 5 leaves the period `open`
      with no `issued` invoices.
- [ ] Two concurrent closes issue each invoice exactly once.
- [ ] An issued invoice cannot be updated through any exported repository
      method other than the payment and adjustment paths.
- [ ] Voided invoices disappear from `v_contact_balance`.

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Double close issues invoices twice | Medium | Critical | `SELECT ... FOR UPDATE` on the period plus `WHERE status='open'` guard with `RowsAffected` assertion; race test in step 10 |
| Blocking predicate misses `planned` past sessions, money silently unbilled | Medium | High | Covered by plan 03's feed and its `000003` index widening; dedicated integration case here |
| Blocking predicate includes `cancelled`, making close impossible | Low | Critical | Excluded by the feed; dedicated integration case and the CHECK at `docs/schema_design.sql:212` cited in the code |
| Close blocks on sessions the dashboard never warned about | Medium | High | Both read the same plan 03 feed; the agreement assertion in step 10 fails the build if they diverge |
| Teacher closes early, later sessions never billed | Medium | High | Warnings array in the response plus the phase 4 reconciliation path; also visible in `v_unbilled_attendance` (`docs/schema_design.sql:474`) |
| Timezone makes "today" the wrong day near midnight, blocking or allowing wrongly | Medium | Medium | `today` resolved once from `teachers.timezone`; `session_date` is a `DATE` so no instant comparison happens |
| Widening `apperror.AppError` for the session list breaks other features | Low | Medium | Do not widen it; return the detail payload from the handler or add a separate `response` helper (Architecture) |
| Void of a partially paid invoice orphans allocations | Medium | High | `paid_amount = 0` guard; teacher must reverse the payment first (plan 05) |

**Rollback.** Close is irreversible by design and there is no `reopen`. If a
close is wrong, the recovery path is: reverse any payments (plan 05), void the
affected invoices with a reason, and record the corrected figures as adjustments
on the next period (phase 4). Reverting the *code* after a close has run leaves
the data consistent — the closed period simply becomes read-only.
