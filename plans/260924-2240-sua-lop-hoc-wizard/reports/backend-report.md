# Backend Report — Sửa lớp học wizard (API side)

Scope: `apps/api` support for the wizard's room/study_mode/next_class_id
fields, the availability endpoint, and the two server-side validations the
coordinator added mid-task (end_date vs start_date, duration_min bounds).

## Files changed

- `apps/api/migrations/000035_class_room_study_mode.up.sql` /
  `.down.sql` — new migration, additive only, never edited after creation.
- `apps/api/internal/features/classes/model.go` — `StudyMode*` constants,
  `Class.Room`, `Class.StudyMode` (with `gorm:"default:scheduled"`),
  `Class.NextClassID *uuid.UUID` (`gorm:"-"`, computed).
- `apps/api/internal/features/classes/dto.go` — `room`/`study_mode` on
  create/update/response DTOs, `next_class_id` on update/response,
  `AvailabilitySlot`/`AvailabilityResponse` types, the `duration_min`
  `max=600` binding tag, the `validateDateRange` helper used by create/update.
- `apps/api/internal/features/classes/service.go` — self-paced schedule
  guard (`CodeSelfPacedNoSchedule`), `resolveNextClassLink`, cycle checks via
  `ParentCreatesCycle`, `Availability`/`computeBusy`/`overlaps` logic,
  `validateDateRange` calls in `CreateAnchored` and `Update`.
- `apps/api/internal/features/classes/repository.go` — `LinkNextClass`,
  `AvailabilityClasses`, `ActiveMemberDirectory`,
  `teacherAccountStatusActive` local constant (avoids an import cycle with
  `centers`).
- `apps/api/internal/features/classes/handler.go` /
  `apps/api/internal/features/classes/routes.go` — `GET
  /classes/availability` wiring.
- `apps/api/internal/features/classes/errors.go` — `ErrSelfPacedNoSchedule` /
  `CodeSelfPacedNoSchedule`.
- `apps/api/internal/shared/routespec/routespec.go` — availability route
  policy entry (`PermClassesList`, no audit).
- `apps/api/internal/features/classes/service_test.go`,
  `handler_test.go`, `integration_test.go` — unit, HTTP-level, and
  integration coverage (new integration tests listed below).
- `apps/api/seeds/seed.go` — updated to the current DTO shape where it
  builds classes.
- `apps/api/docs/docs.go`, `swagger.json`, `swagger.yaml` — regenerated via
  `make api-docs`; not hand-edited.

### Deviations from file ownership (flagged for coordinator review)

Two files outside my listed ownership needed a one-line, mechanically
necessary fix; both are pre-existing hand-pinned regression guards that
break on *any* additive change of their kind, not specifically because of a
design decision I made:

- `apps/api/internal/server/route_policy_snapshot_test.go` — added the
  `GET /api/v1/classes/availability` entry
  (`Kind: PolicyPermission, Key: "classes.list"`) to the hand-maintained
  215-entry snapshot. Without this the whole server package fails to build
  its test suite the moment any new route is registered in `routespec.go`;
  the snapshot's own purpose is to force an explicit, reviewed update like
  this one.
- `apps/api/migrations/migrations_test.go` — bumped
  `TestDownFoldsPersonalChannelIntoManual`'s hardcoded rollback step count
  from 30 to 31 (comment updated to match), because adding migration
  `000035` shifts every migration count above it by one. No behavioral
  assertion in that test changed — it still rolls back to the same anchor
  migration (`000005_zalo_personal_mapping`).

Both are one-line, additive-only fixes required to keep `make test-api-unit`
/ `make test-api` green; nothing else in either file was touched.

## API contract — verified against `plan.md`

- **Migration 000035**: `classes.room VARCHAR(50) NOT NULL DEFAULT ''`,
  `classes.study_mode VARCHAR(12) NOT NULL DEFAULT 'scheduled'` with
  `CHECK (study_mode IN ('scheduled','self_paced'))`. Confirmed via the
  migration file and via `TestSelfPacedCreateWithZeroSchedulesRoundTrips`
  round-tripping both defaults through a real DB.

- **`ClassResponse`** now carries:
  ```json
  {
    "...": "existing fields unchanged",
    "room": "P101",
    "study_mode": "scheduled",
    "next_class_id": "uuid-or-null"
  }
  ```
  `next_class_id` is the earliest-created live child whose
  `parent_class_id` points at this class; null when none. Verified by
  `TestNextClassLinkAgainstRealDB`.

- **`CreateClassRequest`** accepts optional `room` (string, ≤50 chars) and
  `study_mode` (`scheduled`|`self_paced`, defaults to `scheduled`):
  - `scheduled` requires `schedules` with ≥1 entry, otherwise 422 with a
    field error on `schedules`.
  - `self_paced` must send zero schedules; a non-empty `schedules` list is
    rejected 422 on `schedules`.
  Verified by `TestSelfPacedCreateWithZeroSchedulesRoundTrips` (self-paced
  create with zero schedules succeeds; a scheduled class with zero
  schedules is refused) and existing unit tests for the reverse case
  (self-paced with schedules).

- **`UpdateClassRequest`** is patch semantics (nil = keep stored value):
  - `room *string`: ≤50 chars; an explicit `""` clears it.
  - `study_mode *string`.
  - `next_class_id *string`:
    - `""` unlinks every live child of this class.
    - a UUID links that class as the single next class — it must be a live
      class in the same center, sets that class's `parent_class_id` to
      this class's id, and unlinks any other class that previously pointed
      here.
    - self-links and cycles (direct or transitive, via
      `ParentCreatesCycle`) are rejected 422 with a field error on
      `next_class_id`.
  Verified by `TestNextClassLinkAgainstRealDB`: unknown id → 422
  `next_class_id`; foreign-center class → 422 `next_class_id`; valid link
  round-trips through `Get`; a reverse link that would close a two-class
  loop → 422 `next_class_id`; explicit `""` clears and persists.

- **`POST /classes/:id/schedules`** on a `self_paced` class returns 422
  with code `SELF_PACED_NO_SCHEDULE` (`errors.go`:
  `ErrSelfPacedNoSchedule` / `CodeSelfPacedNoSchedule`). Verified against
  the real DB in `TestSelfPacedCreateWithZeroSchedulesRoundTrips`, not just
  the in-memory fake.

- **`GET /classes/availability?slot=<wd>-<HH:MM>-<dur>&slot=…&exclude_class_id=<uuid>`**:
  - Permission `PermClassesList`, no audit entry — matches the
    `route_policy_snapshot_test.go` entry added above.
  - Response shape:
    ```json
    { "rooms": [{"name": "P101", "free": false}],
      "teachers": [{"teacher_id": "uuid", "name": "...", "free": false}] }
    ```
  - Rooms are the distinct non-empty rooms of the center's live classes;
    teachers are the center's active members.
  - Busy rule: a live class other than `exclude_class_id` has a schedule
    active today or later, on the same weekday, whose
    `[start, start+duration)` overlaps the requested slot — that class's
    room, its `teacher_id`, and its active `class_staff` members are all
    marked busy.
  Verified end-to-end against the real batched queries
  (`AvailabilityClasses`, `ActiveMemberDirectory`) by
  `TestAvailabilityAgainstRealDB`: an overlapping schedule marks its room
  and owning teacher busy; a non-overlapping room/slot stays free;
  `exclude_class_id` frees the excluded class's own room (the "editing my
  own class" case).

## Extra-scope validations (coordinator request)

Both were added to match the web wizard's validation and are covered by
unit tests:

1. **`end_date` earlier than `start_date`** — `validateDateRange` in
   `dto.go`, called from both `CreateAnchored` and `Update` in
   `service.go`. Returns 422 with a field error keyed `end_date` when
   `end_date < start_date`. Equal dates are allowed (a same-day class is
   valid). Unit-tested directly in `service_test.go` against both create
   and update paths (service-level, not binding-tag-only, since it depends
   on comparing two request fields).

2. **`duration_min` bounds 1..600** — enforced via the existing Gin
   binding tag `binding:"required,min=1,max=600"` on
   `ScheduleRequest.DurationMin` and `UpdateScheduleRequest.DurationMin` in
   `dto.go` (the `min=1` half pre-existed; `max=600` was the missing half
   added this task). Because Gin binding tags are only enforced at
   `ShouldBindJSON`, this is covered at the HTTP layer in `handler_test.go`
   (0, 601, and boundary values 1/600), not via a direct service-level
   unit test. `make api-docs` was re-run afterward and the regenerated
   `docs/swagger.json`/`swagger.yaml` now show
   `"duration_min": {"type": "integer", "maximum": 600, "minimum": 1}` on
   both schedule schemas.

## Test results

- `go build ./...` (run from `apps/api`) — OK.
- `go vet ./...` and `go vet -tags integration ./...` — OK.
- `make test-api-unit` (repo root, no Docker) — all packages `ok`.
- `make test-api` run serially (`GOFLAGS=-p=1 make test-api`, per stored
  project guidance that these integration tests must not run with
  parallel package workers) — all 50 packages `ok`, including
  `teka/apps/api/internal/features/classes` and
  `teka/apps/api/migrations`. Total coverage **78.6%** against the
  configured 60% floor.
- `make api-docs` — regenerated; `duration_min` bound confirmed in the
  output.

New integration tests added this task, all passing individually and in the
full suite run above:
- `TestSelfPacedCreateWithZeroSchedulesRoundTrips`
- `TestNextClassLinkAgainstRealDB`
- `TestAvailabilityAgainstRealDB`

All integration tests use `testutil.StartPostgres(t)` (self-contained
testcontainers, Ryuk-reaped automatically at session end); no manual
Docker Compose stack was needed and none was started. No processes were
left running and no production `teka-*` containers were touched at any
point.

## Concerns for the coordinator

- The two out-of-ownership one-line fixes
  (`route_policy_snapshot_test.go`, `migrations_test.go`) are necessary for
  the build to pass at all; please acknowledge or reassign review of those
  two lines specifically if that matters for this plan's sign-off process.
- Nothing was committed; all changes are in the working tree only.
