---
phase: 2
title: "Students and PII Anonymisation"
status: completed
priority: P1
effort: "6h"
dependencies: [1]
---

# Phase 2: Students and PII Anonymisation

## Overview

The `students` feature over PRD R1's **closed field list**: full name, contact,
and an optional display note. Nothing else. Plus the delete action, which
anonymises rather than erases, because financial records must stay readable
while the child's personal data goes away.

This is the phase where the product's legal posture is either implemented or
quietly lost. PRD R1 is unusually direct about it: *"Bất kỳ đề xuất thêm trường
nào cũng phải kèm câu trả lời 'trường này phục vụ tính tiền như thế nào' — nếu
không trả lời được thì không thêm."*

## Requirements

- CRUD over `students` with exactly these writable fields: `full_name`,
  `contact_id`, `display_note`.
- `contact_id` is required and must belong to the same teacher.
- Delete anonymises: scrub `full_name` and `display_note`, stamp
  `anonymized_at`, set `deleted_at`. The row survives so financial FKs hold.
- List and detail are scoped by teacher and exclude soft-deleted rows (D4).
- A test asserts the create/update DTOs expose no field outside the closed list.

## Architecture

**Table**: `students(id, teacher_id, contact_id, user_id, full_name,
display_note, created_at, updated_at, deleted_at, anonymized_at)` with
`FOREIGN KEY (contact_id, teacher_id) REFERENCES contacts(id, teacher_id) ON
DELETE RESTRICT` and `CONSTRAINT uq_students_tid UNIQUE (id, teacher_id)`.

`user_id` is NULL forever in V1 — students are absent from the product by
design (PRD Non-Goals: "Học sinh không có job đủ mạnh trong bài toán này").

**Why `display_note` is in the closed list and not a scope violation.** PRD's
edge cases require distinguishing two siblings in the same class who often
share a surname: *"hai dòng điểm danh riêng biệt; giao diện phải phân biệt
được (thường trùng họ, dễ tick nhầm)"*. The schema comment gives the intended
content: a label like "An lớp 9A". It is a disambiguator for the attendance UI,
so it serves fee calculation directly — mis-ticking the wrong sibling produces
a wrong invoice. Note it is free text and teachers may put anything in it; it
is `VARCHAR(50)` to keep it a label rather than a notes field.

**The composite FK is the tenancy guarantee.** `(contact_id, teacher_id)`
referencing `contacts(id, teacher_id)` means a student cannot point at another
teacher's contact even if a caller supplies that id — the insert simply fails.
The service should still validate and return a clean 422 rather than letting a
foreign-key error become a 500, but the safety property comes from the schema.

**Three different kinds of removal**, per schema note (q). They are not
interchangeable and conflating them is the most likely mistake in this phase:

| | What it means | When | Reversible |
|---|---|---|---|
| `deleted_at` | Hidden from the teacher's lists. Data intact. | Teacher removes a student from their roster | Yes |
| `anonymized_at` | PII scrubbed; row and foreign keys survive | Data-subject erasure request, or as part of delete | No |
| hard `DELETE` | Row gone | Only after retention expires and no invoice references it | No |

V1 implements the first two together in one action and does not offer the
third. `invoices.student_id` is `ON DELETE RESTRICT`, so a hard delete of a
billed student is refused by Postgres — intentionally (schema note (q):
*"chặn còn hơn để sổ sách mất dòng"*).

**Data flow — delete**

```
DELETE /api/v1/students/:id
  -> service.Delete(ctx, teacherID, studentID)
       -> repo.GetByID(scoped)  -> 404 if absent
       -> tx.WithinTx:
            UPDATE students
               SET full_name    = <placeholder>,
                   display_note = NULL,
                   anonymized_at = now(),
                   deleted_at    = now()
             WHERE id = ? AND teacher_id = ?
  -> 204
```

The placeholder replaces the name because `full_name` is `NOT NULL`. Use a
non-identifying constant such as `"Đã xoá"` ("deleted"). Do not encode the
original name, initials, or a hash of it — a reversible or correlatable value
is not erasure.

Invoices remain readable because `invoices.student_name` is a snapshot captured
when the period closed (schema section 6: *"khi job retention xoá cứng
students/contacts, sổ sách tài chính vẫn phải đọc được"*). This is why R1's
acceptance criterion is satisfiable at all.

Open enrollments are ended (`ended_on = today`) in the same transaction if any
are still open — a deleted student must stop appearing on future attendance
sheets. Their historical `attendance_records` are untouched: deleting them
would change the billable count and therefore the money already reported to a
parent (schema warning on `attendance_records.deleted_at`).

**Listing.** `GET /api/v1/students?query=&contact_id=&class_id=&page=`.
`class_id` filters through open enrollments and is what the attendance screen
uses. Each row carries the contact's name and phone so the roster screen does
not need a second call.

## Related Code Files

**Create**

- `apps/api/internal/features/students/model.go`
- `apps/api/internal/features/students/repository.go`
- `apps/api/internal/features/students/service.go`
- `apps/api/internal/features/students/dto.go`
- `apps/api/internal/features/students/handler.go`
- `apps/api/internal/features/students/routes.go`
- `apps/api/internal/features/students/errors.go`
- `apps/api/internal/features/students/service_test.go`
- `apps/api/internal/features/students/handler_test.go`
- `apps/api/internal/features/students/integration_test.go`
- `apps/api/internal/features/students/dto_fields_test.go` — reflection test
  pinning the closed field list

**Modify**

- `apps/api/internal/server/router.go` — mount the feature
- `apps/api/internal/testutil/fixtures.go` — `Student(t, db, teacherID,
  contactID, opts...)`
- `apps/api/seeds/seed.go` — seed students against the seeded contacts

## Implementation Steps

1. `model.go`: `Student` mirroring the columns, including
   `AnonymizedAt *time.Time` and `DeletedAt gorm.DeletedAt`. No `default:` tag
   on `ID` (D3). Explicit `TableName()`.
2. `repository.go`: the `scoped` helper, then `Create`, `GetByID`, `List`,
   `Update`, `AnonymizeAndDelete`. `List` joins `contacts` for the contact name
   and phone and optionally joins `enrollments` when `class_id` is present —
   join, do not loop.
3. `AnonymizeAndDelete(ctx, teacherID, studentID, placeholder string)` issues
   the single scoped `UPDATE` shown above. It runs on the context transaction
   so the enrollment closure joins it.
4. `dto.go`: `CreateRequest{FullName string
   \`binding:"required,min=1,max=100"\`, ContactID uuid.UUID
   \`binding:"required"\`, DisplayNote string \`binding:"omitempty,max=50"\`}`.
   `UpdateRequest` carries the same three. `StudentResponse{ID, FullName,
   DisplayNote, ContactID, ContactName, ContactPhone, CreatedAt}`.
5. Write `dto_fields_test.go`: reflect over `CreateRequest` and
   `UpdateRequest` and assert the exact set of JSON field names. The test's
   comment states the invariant — the schema and PRD limit student data to
   what fee calculation needs — so that a future contributor who adds a field
   and sees a red test understands it is a requirement, not a stale fixture.
6. `service.go`:
   - `Create`: verify the contact exists under this teacher (clean 422 if not),
     set `ID: id.New()` and `TeacherID`, insert.
   - `Update`: same contact check when `contact_id` changes.
   - `Delete`: load, then in one transaction end open enrollments and call
     `AnonymizeAndDelete`.
   The enrollment closure needs the enrollments repository, which phase 4
   builds. Sequence phase 4 before wiring this, or define a small consumer
   interface here (`EndOpenEnrollments(ctx, teacherID, studentID, on time.Time)
   error`) and inject it — the interface approach keeps the phases independent
   and matches the consumer-defined-interface pattern already used at
   `apps/api/internal/features/auth/service.go:18`.
7. `handler.go` and `routes.go`: `rg.Group("/students", requireAuth)` with the
   five routes. Every handler starts from `authctx.TeacherID(c)`.
8. Mount, add the fixture, extend the seeds.
9. Tests:
   - `service_test.go`: create with a contact belonging to another teacher →
     422; delete scrubs the name and stamps both timestamps.
   - `integration_test.go`: create a student, close a period so an invoice
     exists (or insert an invoice row directly via fixture), delete the
     student, then assert `students.full_name` is the placeholder,
     `anonymized_at` is set, and `invoices.student_name` still holds the
     original name; assert a hard `DELETE` on that student is refused by the
     database; assert teacher isolation on get/list.
   - Assert deleting a student leaves their `attendance_records` intact.
10. `make api-docs`, `make test-api`, `make lint-api`.

## Success Criteria

- [x] Create/list/get/update/delete work against a real Postgres
- [x] `dto_fields_test.go` passes and fails if any field is added
- [x] A `contact_id` belonging to another teacher returns 422, never 500
- [x] Deleting a student sets `anonymized_at` and `deleted_at`, replaces
      `full_name` with a non-identifying placeholder, and nulls `display_note`
- [x] After deletion, an existing invoice still shows the original
      `student_name`
- [x] After deletion, the student's `attendance_records` still exist and are
      unchanged
- [x] After deletion, any open enrollment has `ended_on` set
- [x] A hard `DELETE` on a billed student is refused by the database
- [x] Teacher A gets 404 for teacher B's student id
- [x] Listing students filtered by `class_id` issues a bounded query count
- [x] `make test-api` and `make lint-api` pass

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Field list quietly grows (birth date, school, grade, notes) | High | High — legal exposure under Nghị định 13/2023, PRD Q2 | `dto_fields_test.go` fails loudly and explains the invariant in its comment |
| Delete implemented as a hard `DELETE`, hitting the RESTRICT FK in production | Medium | High — teacher cannot remove a student at all | The scrub-and-stamp flow is spelled out; a test asserts the hard delete is refused |
| Anonymised name is reversible (initials, hash, "Nguyen V.") | Medium | High — not erasure, so the legal claim is false | Constant placeholder mandated at step 3; asserted by exact-match test |
| `attendance_records` deleted along with the student, changing billed counts | Medium | High — wrong money already sent to a parent | Explicit success criterion; the schema's own warning comment is quoted in the code's doc comment |
| Circular dependency between students and enrollments packages | Medium | Low | Consumer-defined interface at step 6, mirroring the existing auth/users pattern |
| Deleted student keeps appearing on future attendance sheets | Medium | Medium | Open enrollments closed in the same transaction; plan 03 selects only enrollments open on the session date |
