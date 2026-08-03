---
phase: 4
title: "Enrollments"
status: completed
priority: P1
effort: "4h"
dependencies: [2, 3]
---

# Phase 4: Enrollments

## Overview

`enrollments` — the join between a student and a class, and the row that
carries the **unit price actually billed**. PRD R1 calls this the most
important architectural decision after splitting the contact from the student:
*"Đơn giá nằm ở bản ghi ghi danh, không nằm ở lớp."*

It is also where mid-cycle joining is recorded. `started_on` may be any date,
including one in the middle of a month with ten sessions already taught, and
`ended_on` lets a student leave while their debt history stays intact.

## Requirements

- Enroll a student in a class on a given date; `unit_price` is copied from
  `classes.default_unit_price` at creation and is **not** accepted from the
  request (PRD section 4: V1 has one pricing model).
- One open enrollment per student+class, enforced by `uq_enrollments_active`.
- End an enrollment by setting `ended_on`; the row survives.
- Re-enrolling a student who previously left the same class is allowed.
- `ended_on >= started_on`, enforced by the schema `CHECK` and validated for a
  clean 422.
- Expose the "students active on date D in class C" query that plan 03 needs.
- All queries scoped by `teacher_id` (D4).

## Architecture

**Table**: `enrollments(id, teacher_id, student_id, class_id, started_on,
ended_on, unit_price, created_at, updated_at, deleted_at)` with
`CHECK (unit_price >= 0)`, `CHECK (ended_on IS NULL OR ended_on >=
started_on)`, composite FKs to both `students(id, teacher_id)` and
`classes(id, teacher_id)` (both `ON DELETE CASCADE`),
`CONSTRAINT uq_enrollments_tid UNIQUE (id, teacher_id)`, and
`CREATE UNIQUE INDEX uq_enrollments_active ON enrollments(student_id, class_id)
WHERE ended_on IS NULL AND deleted_at IS NULL`.

**Why the price is copied, not referenced.** If invoices read
`classes.default_unit_price` live, raising the class price would retroactively
change what every past student owed. Copying at enrollment time freezes each
student's rate. V1 never varies the copy, so the values are always identical —
which makes it tempting to skip the column and read the class. Don't: PRD Q9
(sibling discounts) and the schema's own note (r) both land on this column, and
the invoice snapshot chain (`invoice_lines.unit_price`) already assumes it.

`unit_price` is absent from both the create and update DTOs. Its value is
resolved server-side by reading the class. This is the enforcement of "V1 không
cho sửa" — not a validation rule that could be relaxed by accident, but a field
that has no path from the request into the database.

**The active-enrollment index.** `uq_enrollments_active` is partial on
`ended_on IS NULL AND deleted_at IS NULL`, so a student may have many closed
enrollments in one class and at most one open. Duplicate enrollment attempts
surface as a unique violation translated to 409 — again, index-driven, not
pre-check-driven.

**Mid-cycle join.** `started_on` is stored exactly as given. It is not snapped
to a month boundary, not defaulted to the class start date, not adjusted to the
next session. Plan 03 uses it to decide who appears on a session's attendance
sheet; plan 04 counts only attended sessions inside the period. PRD R1's
criterion *"học sinh chỉ được tính tiền từ buổi kế tiếp trở đi"* is the
composition of those two behaviours — this phase's only job is to record the
date honestly.

**The query plan 03 consumes.** Implemented once here and exported through the
repository interface:

```go
// ActiveOn returns the enrollments that should appear on a class's attendance
// sheet for a given date: open on that date and not soft-deleted.
func (r *gormRepository) ActiveOn(ctx context.Context, teacherID, classID uuid.UUID, on time.Time) ([]Enrollment, error)
//   WHERE teacher_id = ? AND class_id = ?
//     AND started_on <= ? AND (ended_on IS NULL OR ended_on >= ?)
//     AND deleted_at IS NULL
```

The boundary conditions are inclusive at both ends. A student whose
`started_on` is exactly the session date attends that session; a student whose
`ended_on` is exactly the session date attends their last one. Both choices are
deliberate and must be asserted by test, because an exclusive boundary silently
loses one session of revenue per student per departure.

**Data flow — enroll**

```
POST /api/v1/enrollments {student_id, class_id, started_on}
  -> service.Create(ctx, teacherID, req)
       -> classes.GetByID(scoped)   -> 422 if absent  (also yields default_unit_price)
       -> students.GetByID(scoped)  -> 422 if absent
       -> INSERT enrollments (id = id.New(), teacher_id,
                              unit_price = class.default_unit_price)
          -> unique violation -> 409 "already enrolled"
  -> 201
```

The two lookups exist to produce clean 422s and to read the price; the
composite FKs are what actually prevent cross-teacher stitching.

**Ending an enrollment.** `POST /api/v1/enrollments/:id/end {ended_on}` rather
than a general `PUT`, because it is a distinct business action ("nghỉ hẳn giữa
chu kỳ") and the only mutation V1 allows. `ended_on` defaults to today when
omitted. Ending is idempotent in effect but should return 409 if already ended,
so a double-submit does not silently move the departure date.

**Cross-feature dependency.** Phase 2's student deletion needs
`EndOpenEnrollments(ctx, teacherID, studentID, on)`. Implement it here and let
the students service consume it through the small interface phase 2 defined.

## Related Code Files

**Create**

- `apps/api/internal/features/enrollments/model.go`
- `apps/api/internal/features/enrollments/repository.go`
- `apps/api/internal/features/enrollments/service.go`
- `apps/api/internal/features/enrollments/dto.go`
- `apps/api/internal/features/enrollments/handler.go`
- `apps/api/internal/features/enrollments/routes.go`
- `apps/api/internal/features/enrollments/errors.go`
- `apps/api/internal/features/enrollments/service_test.go`
- `apps/api/internal/features/enrollments/handler_test.go`
- `apps/api/internal/features/enrollments/integration_test.go`

**Modify**

- `apps/api/internal/server/router.go` — mount the feature and inject the
  enrollments service into the students service
- `apps/api/internal/features/students/service.go` — wire the real
  `EndOpenEnrollments` implementation into the interface phase 2 declared
- `apps/api/internal/testutil/fixtures.go` — `Enrollment(t, db, teacherID,
  studentID, classID, startedOn)`
- `apps/api/seeds/seed.go` — enroll the seeded students, including one who
  joins mid-month and one who has already left

## Implementation Steps

1. `model.go`: `Enrollment` mirroring the columns. `StartedOn time.Time`,
   `EndedOn *time.Time` as dates, `UnitPrice int64` (D5), `DeletedAt
   gorm.DeletedAt`, no `default:` tag on `ID` (D3), explicit `TableName()`.
2. `repository.go`: `scoped` helper, then `Create`, `GetByID`,
   `List(teacherID, filter{StudentID, ClassID, ActiveOnly}, pagination)`,
   `End(teacherID, id, endedOn)`, `ActiveOn(teacherID, classID, on)`,
   `EndOpenEnrollments(teacherID, studentID, on)`, and `SoftDelete`.
   `translateError` maps `gorm.ErrDuplicatedKey` → `ErrAlreadyEnrolled`.
3. Write `ActiveOn` exactly as documented above, with the inclusive boundaries
   spelled out in its doc comment.
4. `errors.go`: `ErrNotFound`, `ErrAlreadyEnrolled`, `ErrAlreadyEnded`,
   `ErrClassNotFound`, `ErrStudentNotFound`.
5. `dto.go`: `CreateRequest{StudentID uuid.UUID \`binding:"required"\`,
   ClassID uuid.UUID \`binding:"required"\`, StartedOn *Date
   \`binding:"omitempty"\`}` — `unit_price` is deliberately absent, and a
   comment says why. `EndRequest{EndedOn *Date}`.
   `EnrollmentResponse{ID, StudentID, StudentName, ClassID, ClassName,
   StartedOn, EndedOn, UnitPrice, CreatedAt}`.
6. `service.go`: `Create` follows the flow above, defaulting `started_on` to
   today. `End` loads the enrollment, returns `ErrAlreadyEnded` when `ended_on`
   is already set, validates `ended_on >= started_on` for a clean 422, and
   updates.
7. `handler.go` / `routes.go`: `rg.Group("/enrollments", requireAuth)` with
   POST ``, GET `` (filterable by `student_id`, `class_id`, `active`),
   GET `/:id`, POST `/:id/end`, DELETE `/:id`.
8. Mount the feature and inject it into the students service in
   `internal/server/router.go`. Watch the construction order — the students
   service now takes an enrollments dependency.
9. Extend the seeds: one student who joined on the class start date, one who
   joined mid-month, one who has already left. This is the data plan 03 and
   plan 04 develop against, and the mid-month case is the product's whole
   reason for existing.
10. Tests:
    - `service_test.go`: `unit_price` is taken from the class and ignores any
      attempt to supply one; enrolling twice → 409; ending twice → 409;
      `ended_on` before `started_on` → 422.
    - `integration_test.go`: `ActiveOn` includes a student whose `started_on`
      equals the queried date and one whose `ended_on` equals it, and excludes
      one starting the day after; re-enrolling after ending succeeds and the
      old row survives; changing `classes.default_unit_price` afterwards does
      not change any existing `enrollments.unit_price`; deleting a student ends
      their open enrollments; teacher isolation on get/list.
11. `make api-docs`, `make test-api`, `make lint-api`.

## Success Criteria

- [x] Enrolling copies `unit_price` from `classes.default_unit_price`
- [x] No request path can set `unit_price` — the field is absent from the DTOs
- [x] Raising a class's default price leaves existing enrollments untouched
- [x] A second open enrollment in the same class returns 409, driven by
      `uq_enrollments_active`
- [x] Ending an enrollment sets `ended_on`; the row and its history remain
      readable
- [x] Ending an already-ended enrollment returns 409 without moving the date
- [x] Re-enrolling after leaving succeeds and the previous row is preserved
- [x] `ended_on` earlier than `started_on` returns 422
- [x] `ActiveOn` is inclusive at both boundaries, asserted by test
- [x] Deleting a student (phase 2) ends their open enrollments
- [x] Teacher A gets 404 for teacher B's enrollment id
- [x] Seeds include a mid-month joiner and a departed student
- [x] `make test-api` and `make lint-api` pass

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| `ActiveOn` boundary made exclusive, dropping one billable session per student | Medium | High — under-billing, the exact failure the product exists to prevent | Boundaries stated in the doc comment and asserted by three explicit test cases |
| `unit_price` read live from the class instead of copied | Medium | High — a price change silently rewrites history | The column is populated at insert; a test changes the class price and asserts no enrollment moves |
| `unit_price` exposed in the DTO "for flexibility" | Medium | Medium — breaks PRD section 4's single pricing model | Absent from the DTO with an explanatory comment; adding it is a visible API change |
| `started_on` normalised to a month boundary or the class start date | Low | High — destroys the mid-cycle case the product is built for | Stored verbatim; the seed data and an integration test both cover a mid-month join |
| Circular dependency between students and enrollments at wiring time | Medium | Low | Consumer-defined interface from phase 2; construction order handled in `router.go` at step 8 |
| Date/time zone drift turns `started_on` into the previous day | Medium | Medium | `DATE` columns carry no zone; parse request dates as plain dates, never as timestamps, and assert a round trip |
