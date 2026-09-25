# Web review: "Sửa lớp học" wizard

Branch `feat/giang-day-menu`, uncommitted web changes. Read-only review.

## Scope
- Files: `apps/web/src/features/roster/{components/class-edit-wizard.tsx, components/schedule-rows-editor.tsx, components/class-dialog.tsx, hooks/use-save-class-wizard.ts, hooks/use-classes.ts, hooks/roster-keys.ts, lib/schedule-diff.ts, lib/roster-format.ts, schemas/roster-schemas.ts, api/classes-api.ts}`, the tests, and `apps/web/e2e/class-list.spec.ts`
- Checks run: `vitest run src/features/roster` passed (246 passed, 3 skipped). `tsc -b --noEmit` is clean. eslint has 0 errors (only the known `incompatible-library` warnings). **prettier --check fails on 3 files.**
- Checked against the backend: `apps/api/internal/features/classes/{dto.go,service.go}` and `classinvites/service.go`.

## Overall
The save order is correct: PUT first, then adds, then closes and deletes, then the invite. Self-paced transitions follow the contract. The diff logic is sound for the (weekday, time, duration) key. The main problems are a behaviour regression (the course is now mandatory), silent price rewrites, dates that are editable but never cross-checked, and error visibility inside the new scroll container.

## High

### H1. Editing a class now requires attaching a course, but create still allows no course
`schemas/roster-schemas.ts:493` (`course_id: z.string().min(1, "Chọn khóa học")`)
- Create mode still offers `NO_COURSE_OPTION` ("Không gắn khóa học", `class-dialog.tsx`). The backend accepts `course_id: ""` on update.
- The failure: an owner opens a legacy class (or one just created with no course) to rename it and gets "Chọn khóa học". The only way to save is to attach a course.
- Attaching a course also fires `pickCourse`, which overwrites `default_unit_price` with the course price (see H2).
- The e2e test had to be patched to work around this. `e2e/class-list.spec.ts:141-143` says "The wizard requires a course; the class was created without one, so any seeded course will do", and it picks `option.nth(1)`, which depends on the seed.
- The old edit dialog never touched the course, so this is a regression and a product decision. Options:
  - (a) Relax the rule to `z.string()` and offer "Không gắn khóa học" like create does.
  - (b) Require a course only when `klass.course` was already set.
  - (c) Keep it and require courses on create too.

### H2. Re-picking the current course silently resets a custom class price
`class-edit-wizard.tsx:292-296` (`pickCourse`)
- `HvSelect.onValueChange` is documented as "Called on every pick, including re-picking the current value" (`hv-select.tsx`).
- The failure: a class with a custom price of 120.000đ on a course whose default is 100.000đ. The user opens the Khóa học dropdown and re-selects the same course, and the price becomes 100.000đ with no notice.
- The old dialog's `rateChanged` warning ("Đơn giá mới chỉ áp cho lượt ghi danh từ nay về sau…") was removed, so nothing flags the price change before save.
- Fix: only copy the price when `nextId !== values.course_id`. Restore the rate-change notice when `values.default_unit_price !== klass.default_unit_price`.

## Medium

### M1. Start and end dates are now editable but never cross-validated
`schemas/roster-schemas.ts:501` and the `superRefine` block
- The wizard edits `start_date` and `end_date` for the first time in edit mode. Nothing checks `end_date >= start_date`, and neither does the backend (`classes.end_date DATE`, no CHECK in `000001_baseline_schema.up.sql:132`; `Service.Update` only parses).
- The failure: end date 2026-01-01 with start date 2026-09-01 is saved and yields a nonsensical phase.
- Fix: add an issue on `end_date` in the `superRefine` when both are set and `end < start`.

### M2. Partial-save errors are invisible: no toast, and the message sits at the bottom of a scroll container
`class-edit-wizard.tsx:401-408` and `:853`
- The `result.partial` branch only calls `form.setError("root")`.
- The root `FieldError` renders at the end of the last section, inside the new fixed-height `sm:overflow-y-auto` scroller.
- The failure: the user clicks "Lưu thay đổi" while viewing section 1, a schedule POST fails after the PUT succeeded, and nothing visible happens. The dialog just stays open.
- The old short dialog did not have this problem. Fix: `hvToast(message, { variant: "danger" })` in the partial branch, and/or render the root error above the sections.

### M3. The teacher picker offers people who already have a pending invitation, so save ends in an "invitation" partial error
`class-edit-wizard.tsx:254-260` (and `findFree("teacher")` at `:350`)
- `teacherCandidates` excludes active staff and the caller, but not pending invitees.
- `classinvites/service.go:107-112` returns 409 "người này đã có lời mời đang chờ cho lớp".
- The failure: "Tìm giáo viên rảnh" can auto-pick a pending invitee. The class fields save, then the invite 409s and the dialog stays open with "Đã lưu lớp nhưng chưa gửi được lời mời…". Every retry repeats the 409 until the user clears the teacher.
- Fix: also exclude teachers with a pending invite for this class (the invitations list is already cached by `classInvitationsKeys`), or treat 409 at the invitation stage as "already invited".

### M4. The Lớp trước / Lớp sau selects show "— Không —" for a link outside the first 100 classes
`class-edit-wizard.tsx:195, 245-253`
- `useClassesList({ status: "all", per_page: 100 })` is capped server-side at 100 (`pagination.go maxPerPage`). It also only lists classes the caller can read.
- `HvSelect` shows the placeholder when `value` is not in `options` (`hv-select.tsx:286,318`).
- The failure: in a center with more than 100 classes, or for a teacher who cannot read the linked class, an existing parent or next link displays as "— Không —". Save keeps it because the value is unchanged, but the UI misstates the data.
- The same happens for an **archived course**: `listCourseOptions` is `status=active`, so an attached archived course shows "— Chọn khóa học —".
- Fix: always include the current `klass.parent_class_id` / `klass.next_class_id` / `klass.course` as synthetic options, labelled from the class response or a detail fetch.

### M5. Every class-detail page view fetches 100 classes
`class-edit-wizard.tsx:195`
- `class-detail-page.tsx:131-138` mounts `<ClassDialog mode="edit" open={searchParams.get("edit")==="1"}>` permanently for writers.
- Every other query in the wizard is gated on `open`, but `useClassesList` has no `enabled` option, so `GET /classes?status=all&per_page=100` fires on every class-detail visit, even though the dialog is never opened.
- Fix: add an `enabled` parameter to `useClassesList` (or a dedicated link-options query) and gate it on `open`.

### M6. Save errors reimplement the shared error helper
`class-edit-wizard.tsx:370-392` (`showSaveError`)
- This is a parallel reimplementation of `useApiFormErrors` (`lib/forms/use-api-form-errors.ts`): the same fields→setError→root fallback, plus a toast and a hand-rolled CLASS_CODE_TAKEN branch.
- `useApiFormErrors(form, { conflictField: "code" })` already routes a 409 to `code`.
- The old dialog used the helper. Fix: use the helper and add the toast around it.

### M7. prettier fails on three changed files
`use-save-class-wizard.ts`, `roster-schemas.ts` (e.g. the one-line `patch` arrow at ~:403 and the long `ctx.addIssue` at ~:530), and `__tests__/schedule-diff.test.ts`. `make lint-web` runs prettier, so it will fail. Run `npx prettier --write` on them.

## Low

- **L1. Retrying right after a partial error can diff against a stale class.** `use-save-class-wizard.ts:39-72`: the mutations' `onSuccess` invalidations are fire-and-forget (`use-classes.ts:88,121,132,142`), so `klass` in the save closure is the pre-save detail until the refetch lands. An immediate retry after a partial failure re-POSTs rows that were already added. The result is two active rows on one weekday; the backend has no overlap guard in `AddSchedule`. The old hook had the same pattern; the window is small.
- **L2. A stale detail cache can silently revert concurrent edits.** `class-edit-wizard.tsx:227-238`: the form resets once per open from whatever detail is cached, and never re-syncs when the background refetch returns newer data. Save then compares the stale form values against the fresh `klass`, sends them because they "differ", and reverts concurrent edits to the note, tags, room or links. The pattern is inherited, but the surface is much wider now. Consider resetting when `klass.updated_at` changes while the form is not dirty.
- **L3. `today()` is a UTC date.** `class-edit-wizard.tsx:31` uses `new Date().toISOString().slice(0,10)`. Between 00:00 and 07:00 in Vietnam this is yesterday's date, so new rows get `effective_from` = yesterday and closes get the day before. This is repo-wide, not new (`class-dialog.tsx:25`, etc.), but the wizard is the biggest writer of effective dates.
- **L4. Duration has no upper bound.** `schedule-rows-editor.tsx:75-88` and `roster-schemas.ts`: the backend field is `int16`. A value like 40000 fails binding on POST after the PUT already applied, which gives a generic partial error. A duration of 1500 or more shows a wrapped end time via `addMinutes` `% 1440`. Add `.max(600)` or similar.
- **L5. Row accessibility gaps.** `schedule-rows-editor.tsx`:
  - A duplicate-weekday error does not mark the weekday `HvSelect` `aria-invalid`.
  - The per-row `role="alert"` message is not tied to its inputs via `aria-describedby`.
  - A non-integer duration ("1.5") shows zod's default English int message.
- **L6. Section nav does not move focus.** `class-edit-wizard.tsx:272-279` (`jumpTo`) scrolls but leaves focus on the nav button, so keyboard and screen-reader users cannot follow. Focus the section heading (`tabIndex={-1}`) after scrolling. An invalid submit also does not jump to the failing section for controlled fields (course, slots), which RHF cannot focus.
- **L7. Duplicated helpers.** `mondayFirst` is duplicated (`schedule-diff.ts:89` and `roster-format.ts:55`), and so is `dayBefore` (lib and test).
- **L8. The mock does not enforce the self-paced rule.** The msw `POST /classes/:id/schedules` never returns 422 for a `self_paced` class, so the "PUT before POST on self_paced → scheduled" ordering is untested. The self-paced test also only covers scheduled → self_paced, and does not assert request order.
- **L9. Minimum price changed.** `default_unit_price` minimum went from 1 (old dialog: "Nhập đơn giá mỗi buổi") to 0. The backend allows 0, so if this follows the prototype it is fine. Noted as a behaviour change.

## Verified OK
- Save order: PUT before adds, so switching self_paced → scheduled is accepted before POST. Adds before closes and deletes. Closes use `PUT …/schedules/:sid`, which the backend does not block on self_paced.
- Diff: rows that never took effect (`effective_from >= today`) are deleted; others are closed at yesterday. Closed rows are ignored. Rows with a null duration are skipped. Duplicate-weekday input is blocked by the schema.
- Patch fields: `code` blank is not sent (the backend keeps it anyway). `""` clears room, note and links. `next_class_id` is sent only when it changed. `start_date` falls back to the stored date for self-paced.
- Non-owners cannot invite: the select and the find button are disabled, so `teacher_id` stays `""`.
- Dead code: `diffSchedules`, `classSettingsInputSchema` and `useSaveClassSettings` are fully removed. `deriveScheduleSlots`, `emptySlot` and `weeklySessionCount` are still used by create and roster-format.

## Unresolved questions
1. H1: should the course be mandatory on edit (prototype rule) even though create allows no course? This needs a user decision.
2. L9: is a price of 0 intended for the wizard?
