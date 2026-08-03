---
phase: 3
title: "Collection Board Views"
status: pending
priority: P1
effort: "5h"
dependencies: [2]
---

# Phase 3: Collection Board Views

## Overview

PRD R7's two views over one set of data. The teacher uses them in two different
physical situations, which is why both exist:

- **Xem theo người liên hệ** (by contact) — one row per parent, all children
  merged, total due / paid / outstanding. Used while chasing money. **This is
  the default.**
- **Xem theo lớp** (by class) — one row per student in a class, with unpaid /
  partial / paid status and the exact shortfall. Used while standing in front of
  the class.

Read-only. New package `collections` so no reporting query shares a file with a
writer.

## Requirements

- By-contact view is backed by `v_contact_balance`
  (`docs/schema_design.sql:459-470`), which already merges children and excludes
  void invoices. Do not reimplement that aggregation.
- By-class view shows per-student status and, for an underpaying family, the
  per-child shortfall (R7 acceptance).
- Fast filter for the unpaid group at 150 students (R7 acceptance).
- Period totals: collected and outstanding.
- Both views are scoped to one `billing_period`.

## Architecture

New package `apps/api/internal/features/collections` with only
`model.go`, `repository.go`, `service.go`, `dto.go`, `handler.go`, `routes.go`
and tests. No writes, no transaction manager.

### By-contact view (default)

```sql
SELECT vcb.contact_id, c.full_name, c.phone, c.deleted_at,
       vcb.student_count, vcb.total_due, vcb.total_paid, vcb.outstanding
FROM v_contact_balance vcb
JOIN contacts c ON c.id = vcb.contact_id AND c.teacher_id = vcb.teacher_id
WHERE vcb.teacher_id = :teacher_id
  AND vcb.period_id  = :period_id
  [AND vcb.outstanding > 0]        -- status=unpaid filter
ORDER BY ...
```

**The `contacts` join deliberately omits `deleted_at IS NULL`.** This is the one
documented exception to D4. A parent whose last child has stopped attending is
soft-deleted from the roster, but their unpaid debt must remain visible and
collectable (PRD §5: "một con nghỉ hẳn, con kia còn học → không xoá lịch sử công
nợ"). The response carries `contact_archived: true` so the UI can grey the row
rather than hide it. Every other query in this package keeps the D4 filter.

Derived per-row `payment_status`:

```
outstanding <= 0             -> "paid"
total_paid  == 0             -> "unpaid"
otherwise                    -> "partial"
```

`v_contact_balance` sums across the whole period, so a parent with one child
paid and one unpaid reads `partial` — which is the honest family-level answer
and matches how the teacher will speak to them.

Each contact row also carries `invoices[]`: per child, `invoice_id`,
`student_name`, `total_due`, `paid_amount`, `outstanding`. The teacher's next
question after "partial" is always "which child", and the client must get that
answer from the server rather than reimplementing the D8 split.

Sorting whitelist: `outstanding` (default descending — biggest debt first, which
is what the teacher wants), `full_name`, `total_due`. Paginated with
`pagination.Parse` and the `listSorts` pattern at
`apps/api/internal/features/users/handler.go:17-21`.

### By-class view

```sql
SELECT i.id AS invoice_id, i.student_id, i.student_name,
       i.contact_id, i.contact_name,
       il.class_name, il.billable_count, il.absent_count, il.amount,
       i.opening_balance, i.total_due, i.paid_amount,
       (i.total_due - i.paid_amount) AS outstanding
FROM invoice_lines il
JOIN invoices i     ON i.id = il.invoice_id AND i.teacher_id = il.teacher_id
JOIN enrollments e  ON e.id = il.enrollment_id
WHERE i.teacher_id = :teacher_id
  AND i.period_id  = :period_id
  AND i.status <> 'void'
  AND e.class_id   = :class_id
  [AND i.total_due > i.paid_amount]   -- status=unpaid filter
ORDER BY i.student_name, i.student_id
```

Two things this view must be honest about, both flowing from the two-axis model:

- `total_due`, `paid_amount` and `outstanding` are **invoice-level** (per
  student, all their classes), while `class_name`/`billable_count`/`amount` are
  **line-level** (this class only). The DTO names them apart —
  `line_amount` vs `invoice_total_due` — so the frontend cannot accidentally
  present a per-class figure as the amount owed.
- The shortfall shown per child is `invoice.outstanding`. For a family that
  underpaid, the D8 allocator has already decided which child absorbed the
  shortfall, so this number is exactly what R7's acceptance criterion asks to be
  visible.

Uses `idx_invoice_lines_invoice` (`docs/schema_design.sql:334`) and
`idx_invoices_unpaid` (`:310-312`) for the unpaid filter.

### Summary

```
GET /billing-periods/:id/collections/summary
```

Returns, for non-void invoices in the period: `student_count`, `contact_count`,
`total_due`, `total_paid`, `total_outstanding`, `paid_contact_count`,
`unpaid_contact_count`, `partial_contact_count`, plus `unallocated_credit` —
the sum over the period's contacts of
`payment.amount − Σ allocations` for their non-reversed payments (OQ-2's
visibility hook).

## Related Code Files

Create:

- `apps/api/internal/features/collections/repository.go`
- `apps/api/internal/features/collections/service.go`
- `apps/api/internal/features/collections/dto.go`
- `apps/api/internal/features/collections/handler.go`
- `apps/api/internal/features/collections/routes.go`
- `apps/api/internal/features/collections/integration_test.go`

Modify:

- `apps/api/internal/server/router.go` — register in `registerFeatures`
  (`apps/api/internal/server/router.go:63-73`)

Delete: none. No migration files.

## Implementation Steps

1. Create `repository.go` with three read methods: `ContactBalances`,
   `ClassCollections`, `PeriodSummary`. Each takes `teacherID`, `periodID`, a
   filter struct, and `pagination.Params`. Use `Raw`/`Scan` into result structs
   rather than GORM model scanning — `v_contact_balance` is a view with no
   model, and raw SQL keeps the D4 exception explicit and reviewable.
2. Add a `Filter` struct: `Status string` (`""`, `unpaid`, `partial`, `paid`),
   `ClassID *uuid.UUID`, `Query string` (substring on contact or student name,
   `ILIKE`, matching the pattern at
   `apps/api/internal/features/users/repository.go:80-83`).
3. Create `service.go` with `Service{repo}`. Validate that the period belongs to
   the caller (`apperror.NotFound("billing period")`), that `view` is one of
   `contact`/`class`, and that `class_id` is present when `view=class`
   (`apperror.Invalid`).
4. Create `dto.go` with `ContactBalanceRow`, `ClassCollectionRow`,
   `SummaryResponse`. Name line-level and invoice-level money fields distinctly
   per Architecture. All money `int64`.
5. Create `handler.go` / `routes.go`, behind `requireAuth`:
   - `GET /billing-periods/:id/collections?view=contact|class&class_id=&status=&q=`
     — `view` defaults to `contact` (R7: by-contact is the default view)
   - `GET /billing-periods/:id/collections/summary`
   Return the list through `response.List` with `params.Meta(total)`, matching
   `apps/api/internal/features/users/handler.go:86`.
6. Register in `registerFeatures`.
7. Create `integration_test.go`. Seed one teacher, one closed period, and:
   - contact A with two children in two classes, paid in full;
   - contact B with two children, underpaid so one child is `partially_paid`;
   - contact C with one child, unpaid;
   - contact D, soft-deleted, with an outstanding invoice;
   - one voided invoice.
   Assert:
   - default view returns one row per contact with merged children (R7);
   - contact A's two children both read `paid` in the by-class view (R7
     acceptance);
   - contact B's by-class rows show which child is short and by how much (R7
     acceptance);
   - `status=unpaid` returns exactly B, C and D;
   - contact D appears with `contact_archived: true`;
   - the voided invoice appears in neither view and in no total;
   - summary totals equal the sum of the row values in the contact view;
   - a period id belonging to another teacher returns `404`.
8. Add a performance assertion: seed 150 students across 5 classes under one
   teacher and assert the default view with `status=unpaid` returns within 500ms
   (the R7 acceptance criterion is a filtering interaction, so it must feel
   instant).
9. Run `go test ./apps/api/internal/features/collections/...`.

## Success Criteria

- [ ] R7: by-contact is the default view; one row per parent with all children
      merged and total due / paid / outstanding.
- [ ] R7: a family that paid in full shows both children as paid in the by-class
      view.
- [ ] R7: a family that underpaid shows the shortfall against the specific
      child.
- [ ] R7: the unpaid group is filterable in one request and returns in under
      500ms at 150 students.
- [ ] Summary reports collected and outstanding, and reconciles with the row
      totals.
- [ ] Void invoices are absent from both views and from all totals.
- [ ] Soft-deleted contacts with debt remain visible and flagged.
- [ ] Line-level and invoice-level money fields are distinctly named in the DTO.

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Frontend renders `invoice_total_due` as the class amount, showing a parent the wrong number | Medium | High | Distinct DTO field names (`line_amount` vs `invoice_total_due`); documented in the OpenAPI annotation |
| The D4 exception is copied into a query where it is wrong, leaking archived roster data | Medium | Medium | Exception confined to the single `contacts` join in `ContactBalances`, commented in place; every other join keeps the filter |
| A student in two classes is double-counted in the by-class view | Medium | Medium | The by-class query is filtered by `e.class_id`, so each class sees exactly one line; invoice-level totals are labelled as such |
| Board is slow at 150 students | Low | Medium | Aggregation happens in `v_contact_balance`; `idx_invoices_unpaid` (`docs/schema_design.sql:310`) covers the filter; performance assertion in step 8 |
| `v_contact_balance` omits contacts whose only invoice is void, so a genuinely-zero family disappears | Low | Low | Correct behaviour — a void invoice is not debt. Documented so it is not "fixed" later |
| Reporting query drifts from the writer's status logic | Low | Medium | Status is derived from `outstanding`/`total_paid` in one place per view, not duplicated per column |

**Rollback.** Read-only package. Remove the two routes and the package
directory; no data is affected.
