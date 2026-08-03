---
title: "02 API Roster Management"
description: "Contacts, students, classes, schedules, and enrollments — the closed-field-list roster that PRD R1 specifies and every fee calculation reads from."
status: completed
priority: P1
effort: "20h"
tags: [api, go, roster, crud, privacy, multi-tenancy]
created: 2026-08-03
blockedBy: [260803-2244-01-api-schema-replacement-and-auth]
blocks: [260803-2244-03-api-sessions-and-attendance]
---

# 02 API Roster Management

## Overview

PRD **R1** ("Quản lý lớp và học sinh") in API form: the teacher's roster.
Five tables, all already created by plan 01's baseline migration —
`contacts`, `students`, `classes`, `class_schedules`, `enrollments`. This plan
writes the Go feature packages for them.

Two design decisions from the PRD carry all the weight and are not negotiable
here:

1. **The phone lives on the contact, not the student.** PRD R1: *"Nếu gắn SĐT
   thẳng vào học sinh, mọi thứ trong Mục 6 liên quan tới gộp thông báo và gộp
   công nợ sẽ phải làm lại từ đầu."* One contact (người liên hệ — the parent or
   guardian who receives messages and pays) has many students; each student
   points at exactly one contact. There is no "family" entity: a family is
   derivable as the set of students sharing a contact.
2. **The unit price lives on the enrollment, not the class.** `classes` carries
   `default_unit_price`; `enrollments.unit_price` is what actually gets billed.
   V1 always inherits the class default and does not allow editing it, but the
   column exists so sibling discounts (PRD Q9, P1) do not require a rewrite.

The field list for students is **closed**. PRD R1 states any proposed new field
must answer "how does this field serve fee calculation" — if it cannot, it is
not added. This is a legal posture (Nghị định 13/2023 on children's personal
data, PRD Q2), not a preference.

## Scope

**In scope**

- `contacts` CRUD, unique phone per teacher, delete blocked while students
  reference the contact.
- `students` CRUD over the closed field list, plus the anonymise action that
  PRD R1's acceptance criterion requires.
- `classes` CRUD and `class_schedules` (weekday, start time, duration,
  effective date range).
- `enrollments`: join a class on a given date, leave a class, one active
  enrollment per student+class.

**Non-goals**

- Session generation from schedules — plan 03 reads `class_schedules` and
  writes `class_sessions`.
- Any fee arithmetic. The mid-cycle-join rule ("chỉ tính từ buổi kế tiếp trở
  đi") is *enabled* here by storing `enrollments.started_on` accurately, and
  *enforced* by the billing engine in plan 04. See "Boundary with billing".
- Editing `enrollments.unit_price` (PRD section 4 excludes per-student pricing
  from V1).
- A "family" entity, a second contact per student, or contact login. All are
  P1 (PRD R1 and schema note (r)).
- The scheduled retention/hard-delete job (schema note (q)). This plan builds
  the per-student anonymise action a teacher triggers; the periodic job needs
  the unresolved retention policy from PRD Q2.

## Phases

| # | Phase | Effort | Depends on | Status |
|---|-------|--------|------------|--------|
| 1 | [Contacts](./phase-01-contacts.md) | 5h | — | Done |
| 2 | [Students and PII anonymisation](./phase-02-students-and-anonymisation.md) | 6h | 1 | Done |
| 3 | [Classes and schedules](./phase-03-classes-and-schedules.md) | 5h | — | Done |
| 4 | [Enrollments](./phase-04-enrollments.md) | 4h | 2, 3 | Done |

Phases 1→2 and 3 are independent and may run in parallel; they touch disjoint
directories. Phase 4 needs both.

## Key Decisions

Inherited from plan 01 and restated because every phase here depends on them:

**D3 — UUIDv7 from Go.** The schema declares bare `UUID PRIMARY KEY` with no
default. Every insert in this plan calls `internal/shared/id.New()`.

**D4 — Tenancy at the repository layer.** Every query filters `teacher_id`;
every read on a soft-delete table adds `deleted_at IS NULL`. Copy the `scoped`
helper from `internal/features/teachers/repository.go`. Composite FKs
(`uq_contacts_tid`, `uq_students_tid`, `uq_classes_tid`, `uq_enrollments_tid`)
already make cross-teacher *writes* impossible — a `students` row cannot point
at another teacher's contact because the FK is on `(contact_id, teacher_id)`.
They do nothing about cross-teacher *reads*. Every phase carries a two-teacher
isolation test.

**D5 — Money is `BIGINT` đồng; states are `VARCHAR` + `CHECK` mirrored as Go
constants.** `default_unit_price` and `unit_price` are đồng. No float, no
decimal type, no cents.

**D6 — GORM mirrors the schema; never `AutoMigrate`.**

Plan-specific:

**Soft delete is the default; hard delete is not offered.** All five tables
carry `deleted_at`, and every `UNIQUE` on them is a partial index
(`WHERE deleted_at IS NULL`) precisely so a teacher can delete and recreate a
record. Deleting a contact and re-adding the same phone must work.

**Deleting a contact with students is refused, not cascaded.**
`students.contact_id` is `ON DELETE RESTRICT`. PRD's edge case is explicit:
*"Một con nghỉ hẳn, con kia còn học → không xoá người liên hệ, không xoá lịch
sử công nợ của con đã nghỉ."* The API returns 409 listing the blocking
students rather than silently orphaning them.

**Student deletion anonymises rather than erases.** PRD R1 acceptance:
*"dữ liệu cá nhân bị xoá thật, chỉ giữ lại bản ghi tài chính đã ẩn danh."*
Financial records reference students with `ON DELETE RESTRICT`, so a real
`DELETE` is refused by the database once an invoice exists — by design (schema
note (q)). The delete action therefore scrubs `full_name` and `display_note`
and stamps `anonymized_at`, while `invoices.student_name` (a snapshot taken at
closing time) keeps the books readable. `deleted_at` and `anonymized_at` are
different things and both exist.

## Acceptance Criteria

Traced to PRD R1.

- [x] A teacher creates a class with a start date (ngày khai giảng), a weekly
      schedule, and a default unit price.
- [x] A student can be added to a class at any time, with the join date
      recorded (`enrollments.started_on`).
- [x] One student can be enrolled in several classes belonging to the same
      teacher.
- [x] **R1 acceptance:** the student creation form exposes no field outside
      full name, contact, and (as a disambiguator) display note. Asserted by a
      test over the request DTO's field set, not by inspection.
- [x] **R1 acceptance:** deleting a student scrubs the personal data and keeps
      the financial trail readable through the invoice snapshots.
- [x] Two contacts of the same teacher cannot share a phone; two different
      teachers can each have a contact with the same phone.
- [x] Deleting a contact that still has students returns 409 and names them.
- [x] Deleting a contact whose students are all deleted succeeds, and the same
      phone can be registered again afterwards.
- [x] A student cannot hold two open enrollments in the same class; ending one
      (`ended_on`) allows a new one.
- [x] `enrollments.unit_price` is copied from `classes.default_unit_price` at
      creation and is not writable through the API.
- [x] Leaving a class sets `ended_on` and keeps the enrollment (and therefore
      the debt history) readable.
- [x] Every list and detail endpoint is scoped to the calling teacher;
      requesting another teacher's record id returns 404, not 403.

## Boundary with billing (plan 04)

This plan stores `enrollments.started_on` and `ended_on` faithfully. It does
**not** decide which sessions a student is charged for. PRD R1's criterion
*"Given lớp đã có 10 buổi học, When thêm học sinh mới, Then học sinh chỉ được
tính tiền từ buổi kế tiếp trở đi"* is satisfied downstream: plan 03 creates
`attendance_records` only for enrollments open on the session date, and plan 04
counts only `billable` records inside the period. Getting `started_on` right
here is the necessary condition; the sufficient condition lives in those plans.

Concretely, the contract this plan owes plan 03: for any class and any date,
"the students who should appear on that session's attendance sheet" is exactly
`enrollments WHERE class_id = ? AND started_on <= date AND (ended_on IS NULL OR
ended_on >= date) AND deleted_at IS NULL`. That query shape is implemented once
here, in the enrollments repository, and consumed by plan 03.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Scope creep on the student field list (birth date, school, grade) | High | High — legal exposure under Nghị định 13/2023 | The closed list is asserted by a test over the DTO; PRD R1 requires a fee-calculation justification for any addition |
| A repository forgets the `teacher_id` filter | Medium | High — cross-tenant leak | Copy `scoped()` from plan 01; every phase has a two-teacher isolation test in its success criteria |
| `unit_price` becomes editable "just for support purposes" | Medium | Medium — breaks PRD section 4's single pricing model | The field is absent from create/update DTOs entirely, so exposing it is a visible API change, not a flag flip |
| Anonymisation is implemented as a hard delete and hits the RESTRICT FK | Medium | Medium | Phase 2 spells out the scrub-and-stamp approach and tests the with-invoice case |
| Phone stored inconsistently on contacts, defeating `uq_contacts_phone` | Medium | High — breaks notification and debt grouping (R5, R7) | Reuse the `Normalize` helper plan 01 built for account phones |

## Open Questions

1. Is a contact allowed to have no students (created in advance), or must one
   be created alongside a student? The schema permits a childless contact;
   this plan assumes it is allowed and the UI simply does not encourage it.
2. When a student moves to a different contact (custody change, wrong entry),
   do existing invoices keep the original `contact_id`? The schema says yes —
   `invoices.contact_id` is a snapshot of "contact at closing time" — but the
   product behaviour when a teacher does this mid-period is undefined and
   affects plan 05's payment allocation.
3. Retention policy for anonymised students (PRD Q2, schema note (q)): how long
   before hard delete becomes permissible, and is it automatic or on request?
   Unanswered, and it blocks the scheduled job, not this plan.
4. Should ending an enrollment while the current period is open trigger
   anything, or is it purely a data edit? Assumed purely data here; plan 04
   decides how a mid-period departure is closed out ("chốt sổ tới buổi cuối
   cùng, giữ lại nợ nếu có").

<!-- slug: 02-api-roster-management -->
