---
phase: 3
title: "Classes and Schedules"
status: completed
priority: P1
effort: "5h"
dependencies: []
---

# Phase 3: Classes and Schedules

## Overview

`classes` and `class_schedules`: the class with its opening date (ngày khai
giảng), its default per-session price, and its fixed weekly timetable. PRD R1's
first bullet: *"Tạo lớp với ngày khai giảng, lịch cố định trong tuần, đơn
giá/buổi."*

The schedule rows are what plan 03 turns into concrete `class_sessions`. Their
`effective_from` / `effective_to` range exists so a class can change its
weekly slot mid-term without corrupting the sessions already generated under
the old timetable.

This phase is independent of phases 1–2 and can run in parallel with them.

## Requirements

- CRUD over `classes`: name, start date, optional end date,
  `default_unit_price` (BIGINT đồng, D5), status `active`/`archived`.
- Schedule rows managed as a sub-resource of a class: weekday (0=Sunday…6),
  start time, duration in minutes, effective date range.
- Archiving a class is the normal "stop using it" action; soft delete is
  reserved for a class created by mistake.
- All queries scoped by `teacher_id`, all reads exclude soft-deleted rows (D4).
- No overlap validation between schedule rows in V1 — PRD P2 lists
  "cảnh báo trùng giờ/trùng phòng" as a future consideration.

## Architecture

**Tables**

`classes(id, teacher_id, name, start_date, end_date, default_unit_price,
status, created_at, updated_at, deleted_at)` with
`CHECK (default_unit_price >= 0)`,
`CHECK (status IN ('active','archived'))`,
`CONSTRAINT uq_classes_tid UNIQUE (id, teacher_id)`, and
`idx_classes_teacher ON classes(teacher_id) WHERE deleted_at IS NULL AND status
= 'active'`.

`class_schedules(id, teacher_id, class_id, weekday, start_time, duration_min,
effective_from, effective_to, created_at, updated_at, deleted_at)` with
`CHECK (weekday BETWEEN 0 AND 6)`, `CHECK (duration_min > 0)`,
`duration_min DEFAULT 90`, and the composite FK
`(class_id, teacher_id) → classes(id, teacher_id) ON DELETE CASCADE`.

Note `class_schedules` has **no** `uq_..._tid` constraint — nothing references
a schedule row, so no composite FK target is needed.

**Weekday encoding.** `SMALLINT` with 0 = Chủ nhật (Sunday), matching Go's
`time.Weekday` where `time.Sunday == 0`. Convert with a direct cast and say so
in a comment; an off-by-one here generates every session on the wrong day, and
the bug looks like a timezone problem for a long time before anyone finds it.

**`status` vs `deleted_at`.** Archiving keeps a class visible in history,
reports, and past invoices — it is the action a teacher takes when a term ends.
Soft delete hides it entirely and is for mistakes. The partial index
`idx_classes_teacher` covers exactly `deleted_at IS NULL AND status = 'active'`,
which is the default list query, so the default list should match that
predicate to use the index.

**Effective ranges.** A schedule row applies to dates in
`[effective_from, effective_to]`, with `NULL` meaning open-ended. Changing a
class's weekly slot is modelled as closing the current row
(`effective_to = <last day of the old timetable>`) and inserting a new one —
not as an in-place edit. That is what keeps already-generated sessions
explicable: a session on a Tuesday in March is justified by the row that was
effective in March, even after the class moves to Thursdays in April.

The API should make this the easy path: `PUT /classes/:id/schedules` accepts
the full desired set of schedule rows and diffs against the stored ones, or
schedules are managed individually with an explicit "close from date" action.
Prefer the individual-resource form — it is simpler, and the "replace the whole
set" form makes it too easy to accidentally rewrite history.

**Deleting a class.** `class_schedules`, `class_sessions`, and `enrollments`
all cascade from `classes` on hard delete, but this API only soft-deletes, so
nothing cascades. A soft-deleted class leaves live enrollments and sessions
pointing at it. Guard it the same way phase 1 guards contacts: refuse with 409
when open enrollments exist, and direct the teacher to archive instead.

**Data flow — create class with schedules**

```
POST /api/v1/classes {name, start_date, default_unit_price, schedules[]}
  -> service.Create
       -> tx.WithinTx:
            INSERT classes (id = id.New(), teacher_id, ...)
            for each schedule: INSERT class_schedules
                               (id = id.New(), teacher_id, class_id,
                                effective_from defaulting to class.start_date)
  -> 201 ClassResponse with schedules embedded
```

Accepting schedules in the create payload matters: a class with no timetable
generates no sessions, so a two-step create leaves a broken class if the second
call fails. One transaction, one round trip.

**Price units.** `default_unit_price` is BIGINT đồng (D5). The JSON field is a
plain integer. Never accept or emit a decimal — 150000 is one hundred fifty
thousand đồng, and any float representation of money in this system is a bug.

## Related Code Files

**Create**

- `apps/api/internal/features/classes/model.go` — `Class` and `Schedule`
- `apps/api/internal/features/classes/repository.go`
- `apps/api/internal/features/classes/service.go`
- `apps/api/internal/features/classes/dto.go`
- `apps/api/internal/features/classes/handler.go`
- `apps/api/internal/features/classes/routes.go`
- `apps/api/internal/features/classes/errors.go`
- `apps/api/internal/features/classes/service_test.go`
- `apps/api/internal/features/classes/handler_test.go`
- `apps/api/internal/features/classes/integration_test.go`

**Modify**

- `apps/api/internal/server/router.go` — mount the feature
- `apps/api/internal/testutil/fixtures.go` — `Class(t, db, teacherID, opts...)`
  and `Schedule(t, db, class, weekday, startTime)`
- `apps/api/seeds/seed.go` — seed two classes with weekly schedules

## Implementation Steps

1. `model.go`: `Class` and `Schedule` structs. `StartDate`/`EndDate` as
   `time.Time` / `*time.Time` mapped to `date`; `StartTime` needs care — Postgres
   `TIME` has no date part. Map it to a `string` in `HH:MM` form or a dedicated
   type, and state the choice in a comment. A `time.Time` carrying a zero date
   works with pgx but reads badly and invites timezone confusion; prefer the
   string with a validation rule. `DurationMin int16`. Status constants
   `StatusActive`/`StatusArchived` (D5). Explicit `TableName()` on both.
2. `repository.go`: `scoped` helper, then `CreateWithSchedules`, `GetByID`
   (schedules preloaded), `List(teacherID, filter, pagination)`, `Update`,
   `Archive`, `SoftDelete`, `CountOpenEnrollments`, plus schedule-level
   `AddSchedule`, `UpdateSchedule`, `CloseSchedule(id, effectiveTo)`,
   `SoftDeleteSchedule`, and `ListEffectiveSchedules(teacherID, classID, from,
   to date)`. The last one is the contract plan 03 consumes for session
   generation — it returns schedule rows whose effective range intersects the
   requested window.
3. `errors.go`: `ErrNotFound`, `ErrHasOpenEnrollments`,
   `ErrScheduleNotFound`.
4. `dto.go`: `CreateClassRequest{Name \`binding:"required,min=1,max=100"\`,
   StartDate \`binding:"required"\`, EndDate omitempty,
   DefaultUnitPrice int64 \`binding:"required,min=0"\`,
   Schedules []ScheduleRequest \`binding:"required,min=1,dive"\`}`.
   `ScheduleRequest{Weekday int \`binding:"min=0,max=6"\`, StartTime string
   \`binding:"required,hhmm"\`, DurationMin int \`binding:"required,min=1"\`,
   EffectiveFrom omitempty, EffectiveTo omitempty}`. Add the `hhmm` validator
   to `internal/shared/validation/validation.go` if plan 01 did not.
   Note `binding:"required"` on an int64 rejects 0 — if a free class is
   legitimate, use `min=0` with a pointer or omit `required`; decide and test
   it, because the CHECK allows `>= 0`.
5. `service.go`: `Create` runs the two inserts in `tx.WithinTx`, defaulting
   each schedule's `effective_from` to the class `start_date` when absent.
   `Delete` refuses with `ErrHasOpenEnrollments` → 409 when open enrollments
   exist, suggesting archive. `Archive` flips status.
6. `handler.go` / `routes.go`: `rg.Group("/classes", requireAuth)` with POST
   ``, GET ``, GET `/:id`, PUT `/:id`, POST `/:id/archive`, DELETE `/:id`, and
   the nested `POST /:id/schedules`, `PUT /:id/schedules/:scheduleID`,
   `DELETE /:id/schedules/:scheduleID`.
7. Mount, add fixtures, extend seeds with two classes on different weekdays and
   different opening dates — the PRD's core scenario is classes that start on
   different days, so the seed data should exercise it.
8. Tests:
   - `service_test.go`: create with zero schedules → 422; negative price → 422;
     delete with open enrollments → 409.
   - `integration_test.go`: create rolls back the class when a schedule insert
     fails; `ListEffectiveSchedules` returns only rows overlapping the window,
     including open-ended `effective_to`; weekday 0 round-trips as Sunday;
     `default_unit_price` of 150000 stays exactly 150000; teacher isolation on
     get/list; archived classes are excluded from the default list but
     retrievable by id.
9. `make api-docs`, `make test-api`, `make lint-api`.

## Success Criteria

- [x] Creating a class with schedules is atomic — a failing schedule insert
      leaves no class row
- [x] A class cannot be created with an empty schedule set
- [x] `default_unit_price` round-trips exactly as an integer number of đồng
- [x] Weekday 0 means Sunday and matches `time.Sunday`, asserted by test
- [x] `ListEffectiveSchedules` returns rows whose `[effective_from,
      effective_to]` overlaps the requested window, treating NULL as open-ended
- [x] Closing a schedule (`effective_to`) and adding a replacement leaves the
      old row intact and queryable for past dates
- [x] The default class list matches the `idx_classes_teacher` predicate
      (`deleted_at IS NULL AND status = 'active'`)
- [x] Archiving keeps the class retrievable by id
- [x] Deleting a class with open enrollments returns 409 and suggests archiving
- [x] Teacher A gets 404 for teacher B's class id
- [x] `make test-api` and `make lint-api` pass

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Weekday off-by-one (Monday=0 assumed) | Medium | High — every generated session lands on the wrong day, discovered only after a month of use | Explicit `time.Weekday` cast with a comment; a test asserts Sunday round-trips as 0 |
| Postgres `TIME` mapped to `time.Time` and dragged through a timezone conversion | Medium | High — session start times drift by hours | Map to a validated `HH:MM` string; asserted by round-trip test |
| `default_unit_price` handled as a float somewhere in the stack | Low | High — money errors are the product's core failure mode | BIGINT in the model, `int64` in the DTO, integer in JSON; asserted by an exact-value test |
| Schedules edited in place, making already-generated sessions unexplainable | Medium | Medium | Effective ranges are the documented mechanism; the close-and-replace flow is the API's easy path |
| `binding:"required"` on price rejects a legitimately free class | Low | Low | Called out at step 4 as a decision to make and test |
| Soft-deleted class leaves orphan enrollments and sessions | Medium | Medium | 409 guard on delete; archive is the promoted action |
