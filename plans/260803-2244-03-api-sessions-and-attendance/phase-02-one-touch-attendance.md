---
phase: 2
title: "One-Touch Attendance"
status: pending
priority: P1
effort: "7h"
dependencies: [1]
---

# Phase 2: One-Touch Attendance

## Overview

PRD **R2**: điểm danh 1 chạm — one-touch attendance. Open a session, everyone
is present by default, tick only the absentees, confirm. The acceptance
criterion sets the budget: a 30-student class with 2 absentees costs at most 3
interactions.

The API shape is derived from that budget. One `POST` carrying only the absent
student ids. The server resolves the roster, writes a row for every student
(present ones included), and stamps `attendance_confirmed_at`.

This is the data the entire product is a function of. PRD: *"Mọi giá trị phía
sau đều là hệ quả toán học của dữ liệu điểm danh."*

## Requirements

- `GET /api/v1/sessions/:id/attendance` returns the session's roster with each
  student's current status, so the screen renders in one call.
- `POST /api/v1/sessions/:id/attendance` accepts `{absent_student_ids: [...]}`
  and writes the complete record set.
- The roster is resolved as of the **session date**, not today, using
  `enrollments.ActiveOn` from plan 02 phase 4.
- Confirming sets `attendance_confirmed_at` and moves `status` from `planned`
  to `held`.
- Re-posting to a confirmed session replaces the records in place — rows are
  updated, never soft-deleted.
- `present` and `absent` are both written with `billable = true`; `excused` is
  not writable in V1.
- Cancelled sessions reject attendance with 409.
- Every write teacher-scoped (D4).

## Architecture

**Table**: `attendance_records(id, teacher_id, session_id, student_id,
enrollment_id, status, billable, note, recorded_at, updated_at, deleted_at)`
with `CHECK (status IN ('present','absent','excused'))`,
`billable BOOLEAN NOT NULL DEFAULT true`, composite FKs to `class_sessions`,
`students`, and `enrollments` (all `(x_id, teacher_id)`, all `ON DELETE
CASCADE`), and `uq_attendance_records` — unique `(session_id, student_id)`
where `deleted_at IS NULL`.

**Why present students get rows.** The schema states it directly: *"Lý do không
chỉ lưu người vắng: cần phân biệt 'có mặt' với 'chưa điểm danh'."* Without
present rows, an unconfirmed session and a fully-attended session look
identical, which makes the G4 metric unmeasurable and phase 3's warning feed
impossible. The UI stays one-touch because the server materialises the rows —
the storage model and the interaction model are decoupled on purpose.

**Why `enrollment_id` is stored.** Plan 04 prices a session by reading
`enrollments.unit_price`. Capturing the enrollment at confirmation time means
the price lookup never has to re-derive which enrollment was active on a past
date — and it stays correct even if the student later leaves and re-enrolls at
a different price.

**Data flow — confirm**

```
POST /sessions/:id/attendance {absent_student_ids: [s1, s2], note?}
  -> handler: teacherID from authctx
  -> service.Confirm(ctx, teacherID, sessionID, absentIDs)
       session := sessions.GetByID(scoped)      -> 404 if absent
       if session.status == 'cancelled'          -> 409
       roster := enrollments.ActiveOn(teacherID, session.class_id, session.session_date)
       if any absentID not in roster             -> 422 (naming the ids)
       tx.WithinTx:
         for each enrollment e in roster:
            status := 'absent' if e.student_id in absentIDs else 'present'
            UPSERT attendance_records
              (id = id.New(), teacher_id, session_id, student_id = e.student_id,
               enrollment_id = e.id, status, billable = true)
            ON CONFLICT (session_id, student_id) WHERE deleted_at IS NULL
              DO UPDATE SET status = excluded.status,
                            enrollment_id = excluded.enrollment_id,
                            billable = excluded.billable,
                            updated_at = now()
         soft-delete records for students no longer in the roster
         UPDATE class_sessions
            SET status = 'held', attendance_confirmed_at = now()
          WHERE id = ? AND teacher_id = ?
  -> 200 with the full record set
```

The upsert is what makes re-confirmation safe and idempotent. Record ids stay
stable across edits, `recorded_at` preserves when attendance was first taken
(feeding the G4 "within 24h" metric) and `updated_at` shows the last edit.

**The one legitimate soft delete.** A student removed from the roster since the
last confirmation (they left the class, or were added to the session by
mistake) gets their record soft-deleted. This is the *only* case the schema
permits: *"chỉ dùng khi học sinh bị thêm nhầm vào buổi. Học sinh vắng KHÔNG
phải xoá mềm — dùng status='absent'."* An absent student is never soft-deleted;
soft-deleting a record changes the billable count and therefore the money
already sent to a parent.

**Roster as of the session date.** Using today's enrollments would put a
student who left last week onto a session they attended (correct records
vanish) and a student who joined yesterday onto a session from last month
(phantom charges). `ActiveOn` with its inclusive boundaries is the only
sanctioned query, and phase 4 of plan 02 already asserted those boundaries.

**Editing a past confirmed session.** Same endpoint, no special mode. PRD user
story 8 makes this routine, not exceptional. The consequence for a *closed*
billing period is plan 04's problem — the schema's
`invoice_adjustments.source_session_id` is the mechanism, and PRD Q5 decides
the policy. This phase's obligation is narrow: record the edit accurately, keep
ids stable, keep `updated_at` truthful.

**Concurrency.** Two devices confirming the same session simultaneously both
run the upsert; the unique index serialises them and the last writer wins. That
is acceptable — both are the same teacher recording the same class — but the
test must prove no duplicate rows result.

**Response size.** A 150-student class returns 150 records. That is fine for
one class, but the endpoint returns one session's roster only; never add a
"confirm all sessions" bulk variant without pagination, or the response grows
without bound.

## Related Code Files

**Create**

- `apps/api/internal/features/attendance/model.go` — `Record` plus status
  constants
- `apps/api/internal/features/attendance/repository.go`
- `apps/api/internal/features/attendance/service.go`
- `apps/api/internal/features/attendance/dto.go`
- `apps/api/internal/features/attendance/handler.go`
- `apps/api/internal/features/attendance/routes.go`
- `apps/api/internal/features/attendance/errors.go`
- `apps/api/internal/features/attendance/service_test.go`
- `apps/api/internal/features/attendance/handler_test.go`
- `apps/api/internal/features/attendance/integration_test.go`

**Modify**

- `apps/api/internal/features/sessions/service.go` — expose the status/confirm
  update the attendance service calls (or accept an interface here; prefer a
  consumer-defined interface in the attendance package, matching the pattern at
  `apps/api/internal/features/auth/service.go:18`)
- `apps/api/internal/server/router.go` — mount the feature, inject sessions and
  enrollments services
- `apps/api/internal/testutil/fixtures.go` — `AttendanceRecord(...)`
- `apps/api/seeds/seed.go` — confirm attendance for past sessions, leaving a
  couple unconfirmed for phase 3

## Implementation Steps

1. `model.go`: `Record` mirroring the columns. `Status string`, `Billable bool`,
   `Note *string`, `RecordedAt`, `UpdatedAt`, `DeletedAt gorm.DeletedAt`. No
   `default:` tag on `ID` (D3). Constants `StatusPresent = "present"`,
   `StatusAbsent = "absent"`, and `StatusExcused = "excused"` with a comment
   saying it is reserved for P1 and not writable in V1 (D5). Explicit
   `TableName()`.
2. Define the consumer interfaces this service needs, in this package:
   `RosterSource` with `ActiveOn(ctx, teacherID, classID uuid.UUID, on
   time.Time) ([]enrollments.Enrollment, error)` and `SessionStore` with
   `GetByID` and `MarkHeldAndConfirmed(ctx, teacherID, sessionID, at)`.
3. `repository.go`: `scoped` helper, then `UpsertMany(ctx, records []Record)
   error`, `ListBySession(teacherID, sessionID)`,
   `SoftDeleteMissing(teacherID, sessionID, keepStudentIDs)`, and
   `CountBillableByEnrollment(teacherID, enrollmentID, from, to)` — the last
   one is plan 04's entry point, added here because this package owns the table.
   `UpsertMany` uses `clause.OnConflict` with `DoUpdates` on the columns listed
   in the flow above; verify the emitted SQL targets `uq_attendance_records`
   including its `WHERE deleted_at IS NULL` predicate, exactly as phase 1
   required for sessions.
4. `errors.go`: `ErrSessionCancelled`, `ErrStudentNotEnrolled`,
   `ErrSessionNotFound`.
5. `service.go`:
   - `Get(teacherID, sessionID)`: resolve the roster via `ActiveOn`, left-join
     existing records, return one row per student with `status` (or null when
     unconfirmed) and the student's `full_name` and `display_note` — the
     display note is what disambiguates same-named siblings on the tick screen
     (PRD edge case).
   - `Confirm(teacherID, sessionID, absentIDs, note)`: the flow above, all
     inside `tx.WithinTx`.
   - Validate that every id in `absent_student_ids` is in the roster and reject
     unknown ones with 422 naming them, rather than silently ignoring — a typo
     that silently no-ops means a student is billed for a session they missed.
   - Deduplicate `absent_student_ids` before processing.
6. `dto.go`: `ConfirmRequest{AbsentStudentIDs []uuid.UUID
   \`json:"absent_student_ids"\`, Note string \`binding:"omitempty,max=500"\`}`.
   An empty array is valid and means everyone was present — it must not be
   confused with a missing field, so do not mark it `required`.
   `AttendanceRowResponse{StudentID, StudentName, DisplayNote, EnrollmentID,
   Status, Billable, Note}`. `AttendanceResponse{SessionID, SessionDate,
   Status, AttendanceConfirmedAt, Rows []AttendanceRowResponse}`.
7. `handler.go` / `routes.go`: `GET /sessions/:id/attendance` and
   `POST /sessions/:id/attendance`, both behind `requireAuth`, both starting
   from `authctx.TeacherID(c)`.
8. Mount in `internal/server/router.go`; extend seeds so past sessions are
   confirmed with a realistic scattering of absences.
9. Tests:
   - `service_test.go`: an absent id outside the roster → 422; duplicate
     absent ids collapse to one record; confirming a cancelled session → 409;
     an empty absent list marks everyone present.
   - `integration_test.go`:
     - 30 enrolled students, 2 absent: exactly 30 records, 28 `present`, 2
       `absent`, all `billable = true`, and the operation is one HTTP call.
     - Re-confirm with a different absentee set: still 30 records, the same
       record ids, `updated_at` advanced, `recorded_at` unchanged.
     - A student who joined the day after the session gets no record.
     - A student who left the day before gets no record.
     - A student whose `started_on` equals the session date gets a record.
     - Two concurrent confirms produce exactly 30 records.
     - A student removed from the roster between confirmations has their record
       soft-deleted, while an absent student never does.
     - Confirming sets `status = 'held'` and `attendance_confirmed_at`.
     - Teacher isolation: teacher A cannot read or confirm teacher B's session.
10. `make api-docs`, `make test-api`, `make lint-api`.

## Success Criteria

- [ ] **R2 acceptance:** 30 students, 2 absent, recorded in one API call
      carrying two ids — 3 interactions total in the UI
- [ ] Every student in the roster gets a record, present ones included
- [ ] `present` and `absent` are both `billable = true`
- [ ] `excused` cannot be written through the API
- [ ] The roster is resolved as of the session date, not today
- [ ] Boundary cases hold: `started_on` equal to the session date is included;
      `ended_on` equal to it is included; a day later or earlier is excluded
- [ ] **R2 acceptance:** a session confirmed three days ago can be re-confirmed
      with a different absentee set, and the records reflect it
- [ ] Record ids are stable across re-confirmation; `recorded_at` is preserved
      and `updated_at` advances
- [ ] Absent students are never soft-deleted; only students removed from the
      roster are
- [ ] Confirming sets `status = 'held'` and `attendance_confirmed_at`
- [ ] Confirming a cancelled session returns 409
- [ ] An absent id outside the roster returns 422 naming it
- [ ] Two concurrent confirmations leave exactly one record per student
- [ ] Teacher A cannot read or write teacher B's session attendance
- [ ] `make test-api` and `make lint-api` pass

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Only absentee rows written, present students omitted | Medium | High — G4 unmeasurable, billing wrong, phase 3 impossible | Full-record-set assertion is the first success criterion; the schema's reasoning is quoted in the model's doc comment |
| Roster taken from current enrollments rather than the session date | Medium | High — phantom charges and missing records on edited past sessions | `ActiveOn` is the only sanctioned query; four boundary tests |
| Absent students soft-deleted instead of marked `absent` | Medium | High — billable count drifts, money already reported becomes wrong | Explicit criterion; the schema's warning comment is reproduced in the repository |
| Unknown absent id silently ignored | Medium | Medium — a student is billed for a session they missed | 422 naming the ids, asserted by test |
| Upsert conflict target misses the partial index | Medium | High — re-confirmation errors instead of updating | Emitted SQL verified at step 3, same discipline as phase 1 |
| Re-confirmation recreates rows with new ids, breaking any downstream reference | Low | Medium | Upsert rather than delete-and-insert; stable-id assertion |
| Confirming a 150-student class times out | Low | Medium | One bulk upsert, not a per-student round trip; seed a 150-student class and assert a bounded query count |
| Empty `absent_student_ids` treated as a validation error | Medium | Medium — "everyone present" is the common case and must work | Field is not `required`; explicit test |
