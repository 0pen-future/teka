---
phase: 1
title: "Contacts"
status: pending
priority: P1
effort: "5h"
dependencies: []
---

# Phase 1: Contacts

## Overview

The `contacts` feature: the người liên hệ (contact person — the parent or
guardian who receives the fee message and pays it). PRD R5 makes this the unit
of reporting and R7 the unit of collection, so it is the first roster entity
built and the one students hang off.

This phase also establishes the feature-package shape the rest of plan 02
copies: model → repository (scoped) → service → dto → handler → routes, with a
two-teacher isolation test.

## Requirements

- CRUD over `contacts`: create, list (paginated, searchable), get, update,
  soft delete.
- `uq_contacts_phone` — unique `(teacher_id, phone)` among non-deleted rows —
  surfaces as 409, driven by the index, not a pre-check.
- Deleting a contact that still has non-deleted students returns 409 naming the
  blocking students.
- After deleting a contact, the same phone can be used again (partial index
  behaviour, schema note (j)).
- Phones normalised to a single canonical form before write.
- Every query scoped by `teacher_id` and `deleted_at IS NULL` (D4).

## Architecture

**Table**, from `docs/schema_design.sql`: `contacts(id, teacher_id, user_id,
full_name, phone, created_at, updated_at, deleted_at)` with
`CONSTRAINT uq_contacts_tid UNIQUE (id, teacher_id)` and
`CREATE UNIQUE INDEX uq_contacts_phone ON contacts(teacher_id, phone) WHERE
deleted_at IS NULL`.

`user_id` stays NULL for all of V1 — parents do not log in, they open a token
link (PRD R5). The column is modelled as `*uuid.UUID` and never written.

**Why phone uniqueness is per-teacher and not global.** The schema comment is
explicit: a parent whose children study with several teachers is several
independent `contacts` rows. Global uniqueness would merge two teachers'
customers into one record and leak data across tenants.

**Why duplicate phones must be prevented at all.** Two contacts with the same
number inside one teacher's roster split a family in two: the parent gets two
Zalo messages and two totals (breaking R5's "một tin nhắn, một tổng"), and the
debt view double-counts (breaking R7). The index is the enforcement; the API
just translates its violation.

**Data flow — delete**

```
DELETE /api/v1/contacts/:id
  -> handler: teacherID from authctx (never from the request)
  -> service.Delete(ctx, teacherID, contactID)
       -> repo.Get(scoped)             -> not found -> 404
       -> repo.CountStudents(scoped)   -> > 0 -> 409 with the student names
       -> repo.SoftDelete(scoped)
  -> 204
```

The count-then-delete sequence is racy in principle (a student could be created
between the two statements). It is acceptable here because the FK
`students.contact_id ... ON DELETE RESTRICT` is only about hard deletes — a
soft delete is an UPDATE and the database will not stop it. The counting exists
to give a helpful error, not to guarantee integrity; the worst case is a
soft-deleted contact with a live student, which the student list surfaces and
an undelete fixes. Do not wrap it in a heavier lock for a single-teacher
workload.

**Listing.** `GET /api/v1/contacts?query=&page=&per_page=`. `query` matches
`full_name` or `phone` case-insensitively, following the pattern at
`apps/api/internal/features/users/repository.go:78-97` (that file is deleted by
plan 01 phase 3 — read it from git history if needed, or copy the equivalent
from the teachers repository). Pagination via
`internal/shared/pagination.Parse`.

Each listed contact includes `student_count`, because the teacher's mental
model is families, not phone numbers. Compute it with a grouped subquery in one
statement — a per-row count is an N+1 that shows up immediately at the PRD's
150-student scale.

## Related Code Files

**Create**

- `apps/api/internal/features/contacts/model.go`
- `apps/api/internal/features/contacts/repository.go`
- `apps/api/internal/features/contacts/service.go`
- `apps/api/internal/features/contacts/dto.go`
- `apps/api/internal/features/contacts/handler.go`
- `apps/api/internal/features/contacts/routes.go`
- `apps/api/internal/features/contacts/errors.go`
- `apps/api/internal/features/contacts/service_test.go`
- `apps/api/internal/features/contacts/handler_test.go`
- `apps/api/internal/features/contacts/integration_test.go`

**Modify**

- `apps/api/internal/server/router.go` — mount the feature in
  `registerFeatures`
- `apps/api/internal/testutil/fixtures.go` — add a `Contact(t, db, teacherID,
  opts...)` fixture
- `apps/api/seeds/seed.go` — seed a handful of contacts for the seeded teacher

## Implementation Steps

1. `model.go`: `Contact` struct mirroring the columns. `ID uuid.UUID` with
   **no** `default:` tag (D3). `UserID *uuid.UUID`. `DeletedAt gorm.DeletedAt`.
   Add `func (Contact) TableName() string { return "contacts" }` — GORM's
   pluraliser happens to agree here, but be explicit so a later rename cannot
   silently break it.
2. `repository.go`: copy the `scoped(ctx, teacherID)` helper from
   `internal/features/teachers/repository.go` and its comment. Implement
   `Create`, `GetByID(teacherID, id)`, `List(teacherID, filter, pagination)`,
   `Update`, `SoftDelete`, `CountActiveStudents(teacherID, contactID)`,
   `ListStudentNames(teacherID, contactID)`. Add `translateError` mapping
   `gorm.ErrDuplicatedKey` → `ErrDuplicatePhone`.
3. `List` returns student counts in the same query: left-join a grouped
   subquery over `students` filtered on `deleted_at IS NULL`, select
   `contacts.*, COALESCE(sc.n, 0) AS student_count`. Scan into a row struct,
   not the model.
4. `errors.go`: `ErrNotFound`, `ErrDuplicatePhone`, `ErrHasStudents`.
5. `dto.go`: `CreateRequest{FullName string
   \`binding:"required,min=1,max=100"\`, Phone string
   \`binding:"required,vnphone"\`}` (100 and the phone rule mirror
   `VARCHAR(100)` / `VARCHAR(20)`), `UpdateRequest` with the same two fields,
   `ContactResponse{ID, FullName, Phone, StudentCount, CreatedAt}`, and
   `FromModel`.
6. `service.go`: `Create` normalises the phone with the helper plan 01 added to
   `internal/shared/validation`, sets `ID: id.New()` and `TeacherID`, and maps
   `ErrDuplicatePhone` → `apperror.Conflict`. `Delete` follows the flow under
   **Architecture** and returns a 409 whose message lists up to five blocking
   student names plus a count if there are more.
7. `handler.go`: five handlers, each opening with `authctx.TeacherID(c)`.
   Swagger annotations in the existing style.
8. `routes.go`: `rg.Group("/contacts", requireAuth)` with POST ``, GET ``,
   GET `/:id`, PUT `/:id`, DELETE `/:id`.
9. Mount in `internal/server/router.go`.
10. Add the `Contact` fixture and seed data.
11. Tests:
    - `service_test.go` (fake repository): create normalises the phone;
      duplicate → conflict; delete with students → `ErrHasStudents`.
    - `handler_test.go`: unauthenticated → 401; validation failures → 422.
    - `integration_test.go` (real Postgres): the same phone under two different
      teachers both succeed; the same phone twice under one teacher → 409;
      soft delete then recreate the same phone → 201; teacher A requesting
      teacher B's contact id → 404.
12. `make api-docs`, `make test-api`, `make lint-api`.

## Success Criteria

- [ ] Create, list, get, update, soft-delete all work end to end against a real
      Postgres
- [ ] Duplicate phone within one teacher returns 409; the same phone under a
      second teacher returns 201
- [ ] Deleting a contact with live students returns 409 naming them
- [ ] Soft-deleting a contact frees its phone for reuse
- [ ] `0912345678` and `+84912345678` are treated as the same number
- [ ] Teacher A gets 404 (not 403) for teacher B's contact id
- [ ] Listing 150 contacts issues a bounded number of queries — no per-row
      student count
- [ ] `make test-api` and `make lint-api` pass

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| N+1 on `student_count` at 150-contact scale | Medium | Medium — the roster screen is a daily path | Grouped subquery in `List`; the integration test seeds 150 contacts and asserts the query count |
| Pre-check SELECT used for phone uniqueness instead of the index | Medium | Medium — duplicate contacts under concurrency, breaking R5/R7 grouping | Step 2 mandates `translateError`; a test creates the same phone twice |
| 403 instead of 404 for another teacher's id, confirming the id exists | Medium | Low | Repository scoping makes it a natural 404; asserted in the isolation test |
| Contact soft-deleted while students remain, orphaning them in the UI | Low | Low | Guarded by the 409 path; residual case is visible and reversible |
| Phone normalisation helper missing because plan 01 named it differently | Medium | Low | Grep `internal/shared/validation` before writing; reuse rather than re-implement (DRY) |
