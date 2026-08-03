---
phase: 1
title: "Session Generation and Lifecycle"
status: pending
priority: P1
effort: "7h"
dependencies: []
---

# Phase 1: Session Generation and Lifecycle

## Overview

Turn `class_schedules` into concrete `class_sessions`, and manage each
session's lifecycle: `planned` → `held`, or `cancelled` with a reason.

Generation is on demand and idempotent: asking for a class's sessions over a
date range materialises any that are missing and returns the whole set. There
is no scheduler to operate and no backfill job to debug.

## Requirements

- `GET /api/v1/classes/:id/sessions?from=&to=` returns the sessions in the
  range, generating missing ones first.
- Generation reads effective `class_schedules` rows (plan 02 phase 3's
  `ListEffectiveSchedules`) and emits one session per matching weekday inside
  the intersection of the requested range, the class's `[start_date, end_date]`,
  and each schedule's `[effective_from, effective_to]`.
- Generating the same range twice creates no duplicates
  (`uq_class_sessions_per_day`).
- A session can be cancelled with a required reason, and un-cancelled back to
  `planned`.
- A session created by mistake can be soft-deleted; that is distinct from
  cancelling.
- Manual one-off sessions can be created for a date with no schedule (a make-up
  class the teacher schedules ad hoc).
- All queries teacher-scoped, all reads exclude soft-deleted rows (D4).

## Architecture

**Table**: `class_sessions(id, teacher_id, class_id, session_date, start_time,
status, cancel_reason, attendance_confirmed_at, created_at, updated_at,
deleted_at)` with `CHECK (status IN ('planned','held','cancelled'))`,
`CHECK (status <> 'cancelled' OR attendance_confirmed_at IS NULL)`,
composite FK `(class_id, teacher_id) → classes(id, teacher_id) ON DELETE
CASCADE`, `CONSTRAINT uq_class_sessions_tid UNIQUE (id, teacher_id)`, and three
indexes:

- `uq_class_sessions_per_day` — unique `(class_id, session_date)` where
  `deleted_at IS NULL`. The idempotency guarantee.
- `idx_class_sessions_pending` — `(teacher_id, session_date)` where
  `status = 'held' AND attendance_confirmed_at IS NULL AND deleted_at IS NULL`.
  Phase 3's index; the schema calls this "truy vấn nóng nhất".
- `idx_class_sessions_class_date` — `(class_id, session_date)`.

**Generation algorithm**

```
generate(teacherID, classID, from, to):
    class    := classes.GetByID(scoped)                 -- 404 if absent
    windowLo := max(from, class.start_date)
    windowHi := min(to, class.end_date or +inf)
    schedules := classes.ListEffectiveSchedules(teacherID, classID, windowLo, windowHi)

    rows := []
    for each schedule s:
        lo := max(windowLo, s.effective_from)
        hi := min(windowHi, s.effective_to or +inf)
        for d := lo .. hi:
            if int(d.Weekday()) == s.weekday:
                rows = append(rows, session{
                    id:           id.New(),
                    teacher_id:   teacherID,
                    class_id:     classID,
                    session_date: d,
                    start_time:   s.start_time,
                    status:       'planned',
                })
    INSERT rows ... ON CONFLICT (class_id, session_date)
                     WHERE deleted_at IS NULL DO NOTHING
```

Three properties worth stating because each has a failure mode that only shows
up in production:

*Clamped to the class dates.* Sessions must never predate `start_date` (ngày
khai giảng). A class opening on the 15th with a Monday schedule must not
generate sessions for the first two Mondays of the month, or every student gets
billed for classes that never happened.

*Clamped to the schedule's effective range.* This is what makes a mid-term
timetable change safe: sessions before the change are justified by the old row,
after by the new one.

*Conflict-tolerant insert.* `ON CONFLICT ... DO NOTHING` against the partial
unique index makes concurrent generation safe without a lock. Two dashboard
loads racing each other is the normal case, not an exotic one. Note the
`ON CONFLICT` clause must name the same predicate as the partial index for
Postgres to match it.

*Soft-deleted sessions do not block regeneration.* The partial index excludes
them, so a deleted session's date is free again. That is intentional: deleting
a mistakenly-created session and regenerating should work. It also means
deleting a session that the schedule still implies will simply be recreated on
the next generation call — which is why cancelling, not deleting, is the right
action for "this class did not happen".

**Iterating dates, not timestamps.** `session_date` is a `DATE`. Iterate day by
day using `time.Time` values constructed in the teacher's timezone
(`teachers.timezone`, default `Asia/Ho_Chi_Minh`) at midnight, and use
`AddDate(0, 0, 1)` rather than adding 24 hours. Adding a duration across a DST
boundary drifts; Vietnam has no DST today, but the timezone column exists
because that assumption is not permanent.

**Lifecycle transitions**

| From | To | Endpoint | Guard |
|---|---|---|---|
| `planned` | `held` | implicit on attendance confirm (phase 2), or `POST /sessions/:id/hold` | — |
| `planned`/`held` | `cancelled` | `POST /sessions/:id/cancel {reason}` | reason required and non-empty; clears nothing but must reject if `attendance_confirmed_at` is set |
| `cancelled` | `planned` | `POST /sessions/:id/uncancel` | clears `cancel_reason` |
| any | soft-deleted | `DELETE /sessions/:id` | refuse if attendance is confirmed |

Cancelling a session that already has confirmed attendance is refused with 409.
The schema's `CHECK (status <> 'cancelled' OR attendance_confirmed_at IS NULL)`
would reject it anyway; the service check exists to produce a clear message
rather than a constraint-violation 500. If a teacher genuinely needs to cancel
a confirmed session, they clear attendance first — that path makes the money
consequence visible, which is the point.

**Ad-hoc sessions.** `POST /api/v1/classes/:id/sessions {session_date,
start_time?}` creates a single session with no schedule behind it. It hits the
same unique index, so creating one on a date that already has a session returns
409.

## Related Code Files

**Create**

- `apps/api/internal/features/sessions/model.go` — `Session` plus status
  constants
- `apps/api/internal/features/sessions/repository.go`
- `apps/api/internal/features/sessions/generator.go` — the pure date-expansion
  logic, separated so it is unit-testable without a database
- `apps/api/internal/features/sessions/service.go`
- `apps/api/internal/features/sessions/dto.go`
- `apps/api/internal/features/sessions/handler.go`
- `apps/api/internal/features/sessions/routes.go`
- `apps/api/internal/features/sessions/errors.go`
- `apps/api/internal/features/sessions/generator_test.go`
- `apps/api/internal/features/sessions/service_test.go`
- `apps/api/internal/features/sessions/handler_test.go`
- `apps/api/internal/features/sessions/integration_test.go`

**Modify**

- `apps/api/internal/server/router.go` — mount the feature; inject the classes
  service
- `apps/api/internal/testutil/fixtures.go` — `Session(t, db, teacherID,
  classID, date, opts...)`
- `apps/api/seeds/seed.go` — generate sessions for the seeded classes across
  the current and previous month, leaving some unconfirmed so phase 3 has
  something to warn about

## Implementation Steps

1. `model.go`: `Session` mirroring the columns. `SessionDate time.Time` as a
   date, `StartTime *string` in `HH:MM` (matching plan 02 phase 3's choice for
   `class_schedules.start_time` — check what that phase actually did and match
   it), `CancelReason *string`, `AttendanceConfirmedAt *time.Time`, `DeletedAt
   gorm.DeletedAt`. No `default:` tag on `ID` (D3). Status constants
   `StatusPlanned`, `StatusHeld`, `StatusCancelled` (D5). Explicit
   `TableName()`.
2. `generator.go`: a pure function
   `Expand(class ClassWindow, schedules []ScheduleWindow, from, to time.Time,
   loc *time.Location) []time.Time` implementing the algorithm above with no
   database access. Keeping it pure is what makes the boundary cases cheap to
   test — and the boundary cases are where the money is.
3. `generator_test.go`: table-driven cases covering a class starting mid-range;
   a class with an `end_date` inside the range; a schedule whose
   `effective_to` is NULL; two schedules on different weekdays; a schedule
   replaced mid-range by another (no gap, no overlap); an empty result when the
   range predates the class; weekday 0 (Sunday) handled correctly.
4. `repository.go`: `scoped` helper, then `BulkInsertIgnoreConflicts`,
   `ListByClassAndRange`, `GetByID`, `UpdateStatus`, `SoftDelete`. The bulk
   insert uses GORM's `clause.OnConflict{Columns: ..., DoNothing: true}` — verify
   the generated SQL targets the partial index; if GORM will not emit the
   `WHERE deleted_at IS NULL` predicate, fall back to raw SQL. Do not silently
   accept an `ON CONFLICT DO NOTHING` that matches no index, because it becomes
   a runtime error rather than a no-op.
5. `errors.go`: `ErrNotFound`, `ErrSessionExists`, `ErrAttendanceConfirmed`,
   `ErrReasonRequired`.
6. `service.go`:
   - `ListRange(teacherID, classID, from, to)`: load the class, call the
     generator, bulk-insert, then read back and return. Cap the range (e.g. 400
     days) and return 422 beyond it — an unbounded range is an accidental
     denial of service against your own database.
   - `Cancel(teacherID, sessionID, reason)`: require a non-empty trimmed
     reason; refuse when `attendance_confirmed_at` is set.
   - `Uncancel`, `Hold`, `Delete` (refuse when confirmed), `CreateAdHoc`.
7. `dto.go`: `SessionResponse{ID, ClassID, ClassName, SessionDate, StartTime,
   Status, CancelReason, AttendanceConfirmedAt, StudentCount}`.
   `CancelRequest{Reason string \`binding:"required,min=1,max=500"\`}`.
   `CreateSessionRequest{SessionDate \`binding:"required"\`, StartTime
   omitempty}`.
8. `handler.go` / `routes.go`: `GET /classes/:id/sessions`,
   `POST /classes/:id/sessions`, and a `/sessions/:id` group with GET, DELETE,
   `POST /cancel`, `POST /uncancel`, `POST /hold`. Every handler starts from
   `authctx.TeacherID(c)`.
9. Mount in `internal/server/router.go` and extend the seeds.
10. `integration_test.go`: generating twice over the same range yields the same
    session count; two concurrent generation calls (goroutines) yield exactly
    one row per date; a soft-deleted session is regenerated on the next call; a
    cancelled session is not regenerated over (its row still occupies the date);
    cancelling a confirmed session returns 409; an ad-hoc session on an existing
    date returns 409; teacher isolation on get and list.
11. `make api-docs`, `make test-api`, `make lint-api`.

## Success Criteria

- [ ] `GET /classes/:id/sessions?from=&to=` generates and returns the range
- [ ] Generating the same range twice produces no duplicate rows
- [ ] Two concurrent generation requests produce exactly one row per date
- [ ] No session is generated before the class `start_date` or after `end_date`
- [ ] No session is generated outside a schedule's effective range
- [ ] A schedule change mid-range produces sessions on the old weekday before
      the change and the new weekday after, with no gap or overlap
- [ ] Cancelling requires a reason and stores it
- [ ] Cancelling a session with confirmed attendance returns 409
- [ ] Deleting a session with confirmed attendance returns 409
- [ ] A cancelled session keeps occupying its date and is not regenerated
- [ ] An unreasonably large date range returns 422
- [ ] `generator_test.go` covers all boundary cases with no database
- [ ] Teacher A gets 404 for teacher B's session id
- [ ] `make test-api` and `make lint-api` pass

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| `ON CONFLICT` clause does not match the partial index, so concurrent generation errors instead of no-ops | Medium | High — dashboard fails under normal double-load | Step 4 requires verifying the emitted SQL; concurrent-generation test would fail loudly |
| Sessions generated before `start_date` | Medium | High — students billed for classes that never happened | Clamping is in the pure generator with a dedicated test case |
| Date iteration by adding 24h drifts across a DST boundary | Low | High — wrong session dates, wrong roster | `AddDate(0,0,1)` mandated; dates built in the teacher's timezone |
| Weekday convention mismatch with `class_schedules` (0 = Sunday) | Medium | High — every session on the wrong day | Direct `time.Weekday` cast; generator test asserts Sunday |
| Unbounded date range exhausts memory or the connection | Low | Medium | Range cap with a 422 at step 6 |
| Deleting a session silently regenerates it, confusing the teacher | Medium | Low | Documented; cancelling is the promoted action and is covered by a test |
| `start_time` representation diverges from `class_schedules` | Medium | Low | Step 1 requires matching plan 02 phase 3's actual choice rather than assuming |
