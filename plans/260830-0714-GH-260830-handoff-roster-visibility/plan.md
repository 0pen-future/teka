# Handoff roster visibility

Status: done — implemented and verified 2026-08-30 (`make test-api` green, coverage 75.5% / floor 60%)

## Outcome

A member teacher who received a class via handoff sees that class's roster
(students list, classbook, attendance sheet) even though the enrollment and
student rows were created by the owner.

## Root cause (verified in code)

- Handoff moves `classes.teacher_id`, `class_schedules.teacher_id`, and future
  planned sessions only (`features/handoff/service.go`,
  `classes/repository.go ReassignTeacher`).
- Enrollments/students reads scope non-center-wide members to
  `teacher_id = self` (creator), so the new teacher's roster queries return 0
  rows: `enrollments/repository.go scoped`, `attendance/repository.go
  StudentNames`.

## Constraints / decisions (user-accepted 2026-08-30)

- Option (A): widen READ paths by class assignment; no ownership moves.
- Contacts stay own-rows-only: the class teacher sees student names, not the
  parent contact list. Enrollment rows expose `student_name` only — OK.
- Write paths (enrollment create/end/delete) stay creator/owner-scoped.

## Non-goals

- No change to students feature list (`GET /students?class_id=` keeps narrow
  scope; roster surfaces use enrollments).
- No handoff data migration.

## Changes

1. `apps/api/internal/features/enrollments/repository.go` — add `readScoped`
   (scoped OR "enrollment's class is currently assigned to caller"); use it in
   `GetByID`, `List`, `ActiveOn`. Writes keep `scoped`.
2. `apps/api/internal/features/attendance/repository.go` — `StudentNames`
   widens name resolution to students enrolled in a class assigned to caller.
3. Integration tests: assigned member sees roster + attendance names; peer
   without the class still sees nothing; member cannot end owner's enrollment.
4. `docs/api-guidelines.md` — document the class-teacher read widening.

## Acceptance criteria

- Member with `classes.teacher_id = self` (post-handoff): `GET
  /enrollments?class_id=` returns roster; attendance GET returns roster with
  names; marks validation accepts roster students.
- Unassigned peer still gets 0 rows / 404 (no peer-to-peer leak).
- `make test-api` green.
