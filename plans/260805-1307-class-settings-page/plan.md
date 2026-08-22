# Class Settings Page ("Cài đặt lớp")

Status: implemented — verified (108/108 vitest, lint 0 errors, tsc clean, build OK);
code-reviewed, 3 High findings fixed per user decision, Mediums deferred (see Follow-ups)

## Brainstorm contract

**Outcome**: Teacher on "Lớp & học sinh" selects a class tab, clicks "⚙ Cài đặt lớp",
lands on a dedicated settings screen (prototype `classCfg`, `So Lop - Prototype.dc.html:214-255`)
and edits: tên lớp, lịch trong tuần (day chips), giờ học, đơn giá/buổi. Save applies
from the next session; closed periods untouched.

**Constraints**:
- 100% design-system fields/tokens per prototype `classCfg` screen — no extra fields.
- No backend changes; reuse existing contracts: `PUT /classes/:id` (name/price),
  `POST/DELETE /classes/:id/schedules` (timetable diff).
- Follow codebase patterns: zod + react-hook-form + Field/Input, HvCard/HvButton,
  TanStack Query hooks, route.lazy chunks.

**Non-goals**:
- No archive/delete/start-date/end-date/duration editing on this screen
  (live in `ClassDialog` / `ScheduleEditor`).
- No changes to enrollment `unit_price` back-propagation (server copies at enroll time).

**Acceptance criteria**:
1. Tab row on `/students` shows right-aligned "⚙ Cài đặt lớp" only when a real class
   tab is active (not "Tất cả"/"Chưa ghi danh"); click → `/classes/:id/settings`.
2. Settings screen renders: back link "← Lớp & học sinh", title "Cài đặt lớp — {tên}",
   subtitle "Thay đổi áp dụng từ buổi kế tiếp — các kỳ đã chốt không đổi.", 3 stat
   cards (HỌC SINH = active enrollments, BUỔI KỲ {MM} = held/total non-cancelled
   sessions this month, ĐƠN GIÁ HIỆN TẠI), form card with exactly: Tên lớp,
   Lịch trong tuần (multi day-chips), Giờ học (time), Đơn giá / buổi (đ, step 5000),
   conditional rate-change warning, Hủy + "Lưu thay đổi".
3. Validation per prototype: tên bắt buộc, ≥1 ngày/tuần, đơn giá > 0.
4. Rate-change warning (sun-100) shows iff price differs from saved value.
5. Save = `PUT /classes/:id` (when name/price changed) + schedule diff
   (adds first with `effective_from` = today; rows already in effect are
   closed via `PUT …/schedules/:sid` with `effective_to` = yesterday per the
   API's close-and-replace contract; only never-effective rows are deleted;
   duration preserved from replaced row, else class's common duration, else
   90) → toast "Đã lưu {tên} — áp dụng từ buổi kế tiếp" → navigate back to
   `/students?class_id={id}`. (Amended after code review H2/H3.)
6. Vitest for the page + pure diff helper; lint/type/build/test clean repo-wide.

## Scout evidence

- `students-page.tsx` — tab row exists; `ml-auto` slot currently holds "Quản lý lớp" link (kept).
- `classes-api.ts` / `use-classes.ts` — update class + add/delete schedule hooks exist.
- `schedule-editor.tsx` — established semantics: schedule add/delete only regenerates
  sessions ≥ `effective_from`; held/billed sessions untouched.
- `roster-schemas.ts` — `classUpdateInputSchema` requires name/start_date/end_date/price.
- `weekday-chips.tsx` — single-select chips; needs a multi-select variant.
- `money-input.tsx` — đ input, default step 5000 (matches design).
- Stats sources: `useEnrollmentsList({class_id})` (roster), `listClassSessions`
  (attendance feature) for current-month sessions.

## Steps

1. `roster-schemas.ts`: add `classSettingsInputSchema` (name, days ≥1, start_time HH:MM,
   default_unit_price > 0).
2. `weekday-chips.tsx`: add `WeekdayChipsMulti` (toggle set, reuse `chipOrder`).
3. `lib/schedule-diff.ts`: pure `diffSchedules(activeSchedules, days, time, today)` →
   `{toDelete: id[], toAdd: ScheduleInput[]}` + unit test.
4. `pages/class-settings-page.tsx`: screen per design; save orchestration
   (update class → deletes → adds, sequential `mutateAsync`), hvToast, navigate.
5. `routes.tsx`: add `classes/:id/settings` lazy route.
6. `students-page.tsx`: add config button in tab row (visible when class tab active).
7. Tests: page render/validation/save via MSW `roster-handlers.ts` pattern.
8. Verify: vitest roster suite → lint → typecheck → full build.

## Risk

- Classes with multiple different times per week collapse into one "Giờ học" field
  (inherent to design's field set); on save all selected days unify to that time.
  Documented in code comment.
- ~~Deleting a schedule row is already an exposed operation~~ Superseded by review
  H2: rows already in effect are now *closed* (`effective_to`), matching the API's
  close-and-replace contract; only never-effective rows are hard-deleted.

## Code review outcome (2026-08-05)

Reviewer: DONE_WITH_CONCERNS. User decision: fix the 3 High findings only.

**Fixed (High):**
- H1 — stat range narrowed to [month start, today]; `GET /classes/:id/sessions`
  materializes sessions, so a full-month range would freeze future sessions at the
  old timetable before save could change them.
- H2 — replaced DELETE-based diff with close-and-replace: `PUT …/schedules/:sid`
  sets `effective_to` = yesterday for rows already in effect (past sessions stay
  explicable); DELETE only for `effective_from` ≥ today. `useUpdateSchedule`
  reshaped to `(classId)` + `mutationFn({scheduleId, input})` (had no callers).
- H3 — save order is adds → closes → deletes so an interrupted save can never
  leave a class with zero timetable rows; partial failure sets a distinct root
  error ("Chỉ lưu được một phần thay đổi…"); form reset gated per `klass.id` so
  the invalidation-driven refetch cannot wipe that error. Also guarded the ⚙ pill
  to real class tabs only (`classes.some(...)`, AC1 gap).

**Follow-ups (Medium/Low, deferred by user decision):**
- M1 — name-only save on a multi-time timetable silently unifies times; gate the
  schedule diff on `dirtyFields.days || dirtyFields.start_time` + UI warning.
- M2 — a replaced row's future `effective_to` is not carried to its replacement.
- M3 (rest) — background refetch reset race for in-progress edits generally.
- M4 — `classSettingsInputSchema` price `min(1)` blocks renaming an existing 0đ
  class (API/`ClassDialog` allow 0).
- M5 — use `form.formState.isSubmitting` instead of ORed `isPending` flags.
- M6 — `WeekdayChipsMulti` duplicates chip markup/styles of `WeekdayChips`.
- L1 — `today()` (UTC) vs `currentMonth()` (local) disagree 00:00–07:00 VN time.
- L2 — rate warning flashes one frame before the first form reset.
- L4 — HỌC SINH stat caps at `per_page: 100` enrollments.
