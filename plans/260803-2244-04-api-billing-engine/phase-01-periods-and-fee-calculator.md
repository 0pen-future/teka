---
phase: 1
title: "Billing Periods and Fee Calculator"
status: pending
priority: P1
effort: "6h"
dependencies: []
---

# Phase 1: Billing Periods and Fee Calculator

## Overview

Stands up the `billing` feature package: GORM models mirroring the four billing
tables, the repository layer, `billing_periods` CRUD, and the pure fee
calculator that every later phase reuses. No invoice is written in this phase —
the calculator is a pure function over attendance data so it can be unit-tested
without a database.

## Requirements

- PRD R3: fee = billable sessions in period × enrollment unit price + carried
  debt, computed per student independently.
- One `billing_periods` row per teacher per calendar month, unique on
  `(teacher_id, year, month)` where not soft-deleted
  (`docs/schema_design.sql:271`).
- `period_start` / `period_end` are the first and last calendar day of the
  month, resolved in the teacher's timezone (`teachers.timezone`,
  `docs/schema_design.sql:67`).
- Calculator must be pure: inputs in, numbers out, no I/O, no clock reads.
- Money is `int64` end to end (D5).

## Architecture

New package `apps/api/internal/features/billing`, mirroring the layout of
`apps/api/internal/features/users` (model, repository, service, dto, handler,
routes, tests).

**Models** (`model.go`) mirror the DDL exactly and are never auto-migrated,
matching the comment convention at
`apps/api/internal/features/users/model.go:1-3`:

- `BillingPeriod` → `billing_periods` (`docs/schema_design.sql:256-272`)
- `Invoice` → `invoices` (`docs/schema_design.sql:276-312`) — **no**
  `gorm.DeletedAt` field; the table has no `deleted_at` (schema note (i),
  `docs/schema_design.sql:512`)
- `InvoiceLine` → `invoice_lines` (`docs/schema_design.sql:318-334`) — no
  `deleted_at`, and only `created_at` (no `updated_at`)
- `InvoiceAdjustment` → `invoice_adjustments` (`docs/schema_design.sql:338-353`)
  — has `deleted_at`, so `gorm.DeletedAt` applies

Status constants mirror the CHECK lists:

```go
const (
    PeriodOpen   = "open"
    PeriodClosed = "closed"

    InvoiceDraft         = "draft"
    InvoiceIssued        = "issued"
    InvoicePartiallyPaid = "partially_paid"
    InvoicePaid          = "paid"
    InvoiceVoid          = "void"
)
```

**Calculator** (`calculator.go`) is the single source of fee math (DRY: phases
2, 3 and 4 all call it). Shape:

```go
type AttendanceTally struct {
    EnrollmentID   uuid.UUID
    StudentID      uuid.UUID
    ContactID      uuid.UUID
    StudentName    string
    ContactName    string
    ClassID        uuid.UUID
    ClassName      string
    ClassStartDate time.Time
    UnitPrice      int64
    BillableCount  int
    AbsentCount    int
    PresentCount   int
}

type ComputedLine struct {
    EnrollmentID  uuid.UUID
    ClassName     string
    UnitPrice     int64
    BillableCount int
    AbsentCount   int
    Amount        int64 // BillableCount * UnitPrice — mirrors the DB CHECK
}

type ComputedInvoice struct {
    StudentID       uuid.UUID
    ContactID       uuid.UUID
    StudentName     string
    ContactName     string
    Lines           []ComputedLine
    OpeningBalance  int64
    CurrentCharge   int64 // SUM(Lines[].Amount)
    AdjustmentTotal int64
    TotalDue        int64 // OpeningBalance + CurrentCharge + AdjustmentTotal
}

// Compute groups tallies by student and applies the R3 formula.
func Compute(tallies []AttendanceTally, opening map[uuid.UUID]int64,
    adjustments map[uuid.UUID]int64) []ComputedInvoice
```

**Data flow into the calculator.** Counts come from plan 03, not from a query
written here. `attendance.CountBillableByEnrollment(ctx, teacherID, from, to)`
is the sanctioned entry point; it returns, per enrollment in the date window,
`billable_count`, `absent_count` and `present_count`. Billing calls it with
`from = period.period_start`, `to = period.period_end` and joins the result to
the pricing and naming data billing itself needs for the snapshot:

```
enrollments e   -- unit_price
students    s   -- student_name snapshot
contacts    c   -- contact_name snapshot
classes     cl  -- class_name, start_date
```

`TallyAttendance(ctx, teacherID, periodID)` is therefore an **assembler**, not
an aggregator: one call into attendance, one metadata query billing owns, zipped
on `enrollment_id` into `[]AttendanceTally`.

Why the aggregate is not rewritten here: the rules that decide whether a session
counts — `status='held'`, `attendance_confirmed_at IS NOT NULL`, `deleted_at IS
NULL` on both records and sessions, cancelled excluded — are plan 03's rules. A
second copy inside billing would drift, and the direction of that drift is
money.

Four properties billing depends on and asserts in its own integration test. A
failure means plan 03's contract changed, not that billing needs its own filter:

1. Cancelled sessions never contribute, so a teacher-cancelled session bills
   nobody (PRD §5 edge). The CHECK at `docs/schema_design.sql:212` already
   forbids a confirmed cancelled session.
2. Unconfirmed sessions never contribute, matching the schema comment at
   `docs/schema_design.sql:203`.
3. Soft-deleted attendance records and sessions never contribute (schema note
   (j), `docs/schema_design.sql:526`).
4. Mid-period joiners need **no** `started_on` filter: plan 03 only creates
   `attendance_records` for enrollments active on the session date, so rows
   before `enrollments.started_on` do not exist.

**Roster and enrollment dates.** Billing never writes its own
`started_on <= d AND (ended_on IS NULL OR ended_on >= d)` predicate. Where it
genuinely needs "who was on this class's roster on date D" — property 4's
assertion here, and the no-line enrollment case in phase 4 — it calls the plan
02/03 sanctioned query `enrollments.ActiveOn(ctx, teacherID, classID, date)`,
which is inclusive at both the `started_on` and `ended_on` boundaries.
**[verify the exact package path and signature once plan 02/03 land]**

**Import direction.** `billing` imports `attendance` and `enrollments`; neither
imports `billing`. Phase 4's `BillingReconciler` interface is declared *inside*
the attendance package precisely so that stays true, which is what keeps this
direct dependency cycle-free.

**Opening balance** is the carry-over defined by R3. For student `S` and period
`P`, it is the outstanding of `S`'s invoice in the most recent **closed** period
strictly before `P`:

```
opening_balance(S, P) = COALESCE(prev.total_due - prev.paid_amount, 0)
  where prev = invoice of S in the closed billing_period with the greatest
               period_end < P.period_start, and prev.status <> 'void'
```

This does not double count: `prev.total_due` already contains its own
`opening_balance`, so taking its outstanding rolls the whole history forward
exactly once. Negative results (over-payment) clamp to 0 in V1; unallocated
credit is handled in plan 05.

## Related Code Files

Create:

- `apps/api/internal/features/billing/model.go` — GORM models + status constants
- `apps/api/internal/features/billing/calculator.go` — pure fee math
- `apps/api/internal/features/billing/calculator_test.go` — table-driven unit tests
- `apps/api/internal/features/billing/repository.go` — `Repository` interface + GORM impl
- `apps/api/internal/features/billing/service.go` — period lifecycle service
- `apps/api/internal/features/billing/service_test.go` — service tests over a fake repo
- `apps/api/internal/features/billing/dto.go` — request/response types
- `apps/api/internal/features/billing/handler.go` — Gin handlers for periods
- `apps/api/internal/features/billing/routes.go` — `RegisterRoutes`
- `apps/api/internal/features/billing/integration_test.go` — DB-backed tests

Modify:

- `apps/api/internal/server/router.go` — wire the feature inside
  `registerFeatures` (`apps/api/internal/server/router.go:63-73`)

Delete: none.

No migration files. Schema is frozen (D1).

## Implementation Steps

1. Create `model.go`. One struct per table with `TableName()` returning the
   snake_case table name. Omit `DeletedAt` on `Invoice` and `InvoiceLine`;
   include `gorm.DeletedAt` on `BillingPeriod` and `InvoiceAdjustment`. Add the
   status constants listed above. Copy the package-doc comment style from
   `apps/api/internal/features/users/model.go:1-3`.
2. Create `calculator.go` with `Compute` as specified. Group tallies by
   `StudentID`; inside each group build one `ComputedLine` per
   `EnrollmentID`. Set `Amount = int64(BillableCount) * UnitPrice`,
   `CurrentCharge = Σ Amount`, `TotalDue = OpeningBalance + CurrentCharge +
   AdjustmentTotal`. Sort lines by `ClassStartDate` then `ClassName` so output
   is deterministic.
3. Create `calculator_test.go` covering: single line; two lines for one student
   (R1 acceptance); zero billable count; carried `opening_balance` only with no
   lines; negative `adjustment_total` producing a smaller `total_due`; a large
   value case (150 students × 30,000,000 VND) asserting no int overflow. Assert
   the two DB CHECK identities hold for every computed struct — the test is the
   Go-side mirror of `docs/schema_design.sql:306` and `:332`.
4. Create `repository.go` with a `Repository` interface following the shape at
   `apps/api/internal/features/users/repository.go:23-30`, and a
   `gormRepository` that reads the transaction handle via
   `database.FromContext(ctx, r.db)` exactly as
   `apps/api/internal/features/users/repository.go:50` does. Methods for this
   phase: `CreatePeriod`, `GetPeriod`, `GetPeriodByYearMonth`, `ListPeriods`,
   `PreviousClosedPeriod`, `TallyAttendance(ctx, teacherID, periodID)`,
   `OpeningBalances(ctx, teacherID, prevPeriodID, studentIDs)`. Every method
   takes `teacherID` and adds it to the `WHERE` (D4).
   `TallyAttendance` takes the attendance service as a constructor dependency
   and calls `CountBillableByEnrollment` for the counts; the only SQL it writes
   is the enrollment/student/contact/class metadata join. Do not re-derive
   counts from `attendance_records` here.
5. Add `ErrPeriodNotFound` and `ErrDuplicatePeriod` sentinels, translated from
   `gorm.ErrDuplicatedKey` the same way as
   `apps/api/internal/features/users/repository.go:117-125`.
6. Create `service.go` with `Service` holding `repo Repository` and
   `tx database.TxManager` (`apps/api/internal/database/tx_manager.go:8`).
   Implement:
   - `EnsurePeriod(ctx, teacherID, year, month)` — idempotent create-or-get.
     Computes `period_start` / `period_end` from the teacher's timezone. Maps
     the duplicate error onto the existing row rather than a 409, so concurrent
     callers converge.
   - `ListPeriods(ctx, teacherID, pagination.Params)`.
   - `GetPeriod(ctx, teacherID, periodID)` → `apperror.NotFound("billing period")`.
   Map repository errors through `apperror` exactly as
   `apps/api/internal/features/users/service.go:63-71`.
7. Create `dto.go`: `PeriodResponse` (id, year, month, period_start,
   period_end, status, closed_at), `EnsurePeriodRequest` (year
   `binding:"required,min=2020,max=2100"`, month
   `binding:"required,min=1,max=12"`), plus `FromPeriodModel` /
   `FromPeriodModels` converters following
   `apps/api/internal/features/users/dto.go`.
8. Create `handler.go` and `routes.go`. Routes, all behind `requireAuth`:
   - `POST   /billing-periods` → ensure period
   - `GET    /billing-periods` → paginated list, default sort `-period_start`
   - `GET    /billing-periods/:id` → one period
   Use `pagination.Parse` with a `listSorts` whitelist mirroring
   `apps/api/internal/features/users/handler.go:17-21`. Resolve the caller's
   teacher id from `authctx.From(c)`; every service call passes it.
9. Wire the feature in `registerFeatures`
   (`apps/api/internal/server/router.go:63`), constructing
   `billing.NewService(billing.NewRepository(db), txMgr)` and
   `billing.RegisterRoutes(v1, billing.NewHandler(svc), requireAuth)`. `txMgr`
   already exists at `apps/api/internal/server/router.go:66`.
10. Create `integration_test.go` using the existing helpers in
    `apps/api/internal/testutil/`. Seed: one teacher, one contact, one student,
    one class with two sessions, confirmed attendance. Assert `TallyAttendance`
    returns the expected counts and that a cancelled session, a soft-deleted
    attendance record, and an unconfirmed session are all excluded. These are
    contract assertions against `attendance.CountBillableByEnrollment`, not
    tests of billing's own SQL — label them so, because a failure means plan 03
    changed. Add the mid-period-joiner assertion from Architecture property 4,
    verifying roster membership through `enrollments.ActiveOn` rather than a
    hand-written date comparison.
11. Run `go test ./apps/api/internal/features/billing/...`, then `go vet ./...`
    and the repo lint target.

## Success Criteria

- [ ] `billing` package compiles and is registered in `registerFeatures`.
- [ ] `POST /api/v1/billing-periods` is idempotent for the same
      `(teacher, year, month)` and returns the same period id.
- [ ] `GET /api/v1/billing-periods/:id` for another teacher's period returns
      `404`, not `403` (no cross-teacher existence leak).
- [ ] `Compute` unit tests pass, including the two CHECK-identity assertions.
- [ ] Integration test proves cancelled, unconfirmed, and soft-deleted rows are
      excluded from the tally.
- [ ] Billing contains no aggregate over `attendance_records` and no
      `started_on`/`ended_on` comparison; `grep` for both returns nothing in the
      `billing` package.
- [ ] No float type appears anywhere in the package.
- [ ] No `deleted_at` reference for `invoices` or `invoice_lines`.

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Calculator disagrees with DB CHECK, insert fails at close | Medium | High | Unit tests assert both CHECK identities on every computed struct (step 3) |
| Timezone boundary puts a session in the wrong month | Medium | High | `period_start`/`period_end` derived from `teachers.timezone`; `session_date` is a `DATE` so the comparison is date-only with no UTC drift. Integration test with a 1st-of-month and 31st-of-month session |
| Billing re-derives attendance counts and drifts from plan 03, inflating fees | Medium | High | Counts come only from `attendance.CountBillableByEnrollment`; billing writes no aggregate over `attendance_records`; the four contract properties are asserted in step 10 |
| Plan 03's counting contract changes shape and silently changes money | Medium | High | Contract assertions are labelled as such (step 10) so a failure points at plan 03; the counting rules exist in exactly one place, so a deliberate change lands everywhere at once |
| GORM silently adds its own `deleted_at` clause to `invoices` | Low | High | Models for `invoices`/`invoice_lines` omit `gorm.DeletedAt`; a test asserts a raw-SQL round trip of a row that would otherwise be filtered |
| Attendance rows exist before `enrollments.started_on` (plan 03 regression) | Low | High | Explicit integration assertion (step 10); a failure means fixing plan 03, not adding a filter here |

**Rollback.** This phase adds a new package and three read-only endpoints.
Revert by removing the `billing` lines from `registerFeatures` and deleting the
package directory. No data is written, so no data migration is needed.
