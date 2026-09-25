---
title: "Wizard Sửa lớp học theo So Lop v5 (UI + backend)"
status: completed
created: 2026-09-24
branch: feat/giang-day-menu
---

# Wizard Sửa lớp học

## Outcome
The "Sửa lớp học" dialog matches the v5 prototype wizard (`cls.wiz` in
`handoff/04-logic-component.js`). It has a left section nav with six sections that
scroll into view: Thông tin lớp học · Liên kết lớp · Hình thức học · Lịch học hàng
tuần · Nguồn lực dự kiến · Thông tin bổ sung. Every field is saved through the API.
On 2026-09-24 the user chose to add backend support for room, study mode and next class.

## Design source
The template markup for the wizard sits past the 256KB MCP cut, and handoff files
`01`/`02` are missing. The layout is therefore rebuilt from the `wiz` render values
(labels, hints, option lists, inline styles for the nav, chips and radios) and from
the DS conventions of the screens that are readable.

## API contract (fixed; web and API both code against it)
- Migration `000035_class_room_study_mode`:
  - `classes.room VARCHAR(50) NOT NULL DEFAULT ''`
  - `classes.study_mode VARCHAR(12) NOT NULL DEFAULT 'scheduled'`, CHECK in (`scheduled`, `self_paced`).
- `ClassResponse` adds `room`, `study_mode` and `next_class_id`. `next_class_id` is uuid or null: the
  earliest-created live child whose `parent_class_id` is this class.
- `CreateClassRequest` accepts optional `room` and `study_mode`:
  - `scheduled` requires `schedules` (≥1).
  - `self_paced` must send no schedules; otherwise 422 on `schedules`.
- `UpdateClassRequest` is a patch (nil keeps the current value):
  - `room`: ≤50 characters; "" clears it.
  - `study_mode`.
  - `next_class_id`:
    - "" unlinks every live child.
    - A uuid links that live class, which must be in the same center, as the single child. It sets that class's
      `parent_class_id` and unlinks the other children.
    - Self-links and cycles are rejected with 422 on `next_class_id`.
- `POST /classes/:id/schedules` on a `self_paced` class returns 422 `SELF_PACED_NO_SCHEDULE`.
- `GET /classes/availability?slot=<wd>-<HH:MM>-<dur>&slot=…&exclude_class_id=<uuid>`:
  - Permission PermClassesList, no audit.
  - Response: `{ rooms: [{name, free}], teachers: [{teacher_id, name, free}] }`.
  - Rooms are the distinct non-empty rooms of live classes in the center.
  - Teachers are the active center members.
  - A slot is busy when a live class other than the excluded one has a schedule active today or
    later, on the same weekday, whose [start, start+dur) overlaps it. The busy class's room is then
    busy, and so are its `teacher_id` and its active `class_staff` members.

## Web
- `ClassDialog mode="edit"` renders the wizard (`class-edit-wizard.tsx`). Create mode keeps its form.
- Schedule rows are (weekday, time, duration), as in the prototype. The diff closes, deletes and
  adds rows on that triple. Switching to self-paced closes every active row after the PUT.
- If a "Giáo viên dự kiến" is picked, a class invitation is sent after the save, using the existing
  owner-only endpoint.

## Acceptance
- [x] API: migration up/down; unit and integration tests for the new fields, availability and the next link.
- [x] Web: the wizard renders all six sections with the prototype labels and hints; save sends every field.
- [x] vitest roster, typecheck, lint; `make test-api-unit`; classes API integration tests.
- [x] Swagger regenerated with `make api-docs`.
