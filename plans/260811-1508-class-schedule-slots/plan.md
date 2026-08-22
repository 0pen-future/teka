# Multi-slot timetable for "Tạo lớp mới" + "Cài đặt lớp"

Status: implemented, review fixes applied — pending commit · Single phase ·
Source: Claude Design project
"Sổ Lớp prototype interactive" (`So Lop - Prototype.dc.html`, `modalClass` +
`classCfg`).

## Outcome

Both class-timetable surfaces match the updated prototype: a class holds one or
more **khung giờ** (slots), each slot = one HH:MM start time + a multi-select
set of weekdays, with "+ Thêm khung giờ khác", per-slot "Xóa" (only when >1
slot), per-slot summary ("N buổi/tuần" / "Chưa chọn ngày") and a header summary
("· N buổi/tuần").

## Constraints

- API contracts unchanged: `POST /classes` (atomic `schedules[]`),
  `PUT /classes/:id`, schedule add/close/delete fan-out per the existing
  close-and-replace contract in `schedule-diff.ts`.
- Keep react-hook-form + zod + Hv component patterns.
- Keep `duration_min` editable in the create dialog (one shared field — the
  prototype omits it, dropping it would regress behavior) and keep `end_date`.
- Keep 44px touch targets on chips (deliberate a11y deviation from the
  prototype's 36px pills; pill shape adopted).

## Non-goals

- No API/server changes; no per-slot duration editing in "Cài đặt lớp".
- No change to how sessions are generated or billed. Consequence (review
  finding): the generator materializes at most one session per class per date
  (`uq_class_sessions_per_day`) and matches rows by weekday only, so the forms
  must reject a weekday spanning two khung giờ — see acceptance criterion 5.
  Supporting "same weekday, two times" for real would need backend work
  (time-aware index + generator) and stays out of scope.
- Other prototype screens (Chốt sổ, Gửi thông báo, wizard…) out of scope.

## Acceptance criteria

1. Create dialog: name → "Lịch học trong tuần · N buổi/tuần" → slot cards →
   "+ Thêm khung giờ khác" → đơn giá | khai giảng (+ thời lượng | ngày kết
   thúc); submitting builds `schedules` = slots × days (deduped weekday+time
   pairs), `effective_from` = start_date.
2. Settings screen: same slot editor prefilled by grouping active schedule rows
   by start time; save diffs on (weekday, time) pairs — unchanged pairs
   survive, removed/retimed pairs close (or delete if not yet effective), new
   pairs add with preserved/most-common duration.
3. Per-slot validation "Mỗi khung giờ cần ít nhất một ngày trong tuần" and an
   invalid/empty start time both surface on the slot card (message +
   aria-invalid); rate, name validations unchanged.
4. Unit tests (schedule-diff, roster-schemas, class-settings-page,
   students-page) and e2e roster spec updated and green; lint + typecheck +
   build clean.
5. A weekday listed in two khung giờ is rejected client-side ("Ngày này đã có
   ở khung giờ khác — mỗi ngày chỉ một khung giờ") — the backend would accept
   the rows but silently generate only one session per date (under-billing).
6. Multi-time classes render correctly at both schedule-summary sites
   (dashboard class cards, enroll-class picker) via `formatScheduleSummary`,
   which also excludes closed rows.

## Files

- `apps/web/src/features/roster/schemas/roster-schemas.ts` — slot schema; slots
  in dialog/settings input schemas; `toClassCreateInput` flatMap+dedupe.
- `apps/web/src/features/roster/lib/schedule-diff.ts` — `deriveScheduleSlots`
  (replaces `deriveScheduleForm`), pair-based `diffSchedules`.
- `apps/web/src/features/roster/components/schedule-slots-editor.tsx` — new
  shared slot editor (uses `WeekdayChipsMulti`).
- `apps/web/src/features/roster/components/weekday-chips.tsx` — pill styling;
  drop now-unused single-select variant.
- `apps/web/src/features/roster/components/class-dialog.tsx`,
  `pages/class-settings-page.tsx` — swap fields for the editor.
- `apps/web/src/features/roster/lib/roster-format.ts` + `index.ts` —
  `formatScheduleSummary`; consumers
  `features/dashboard/components/class-overview-cards.tsx` and
  `components/enroll-student-dialog.tsx`.
- Tests: `__tests__/schedule-diff.test.ts`, `__tests__/roster-schemas.test.ts`
  (new), `__tests__/class-settings-page.test.tsx`,
  `apps/web/e2e/roster.spec.ts` (default slot has no preselected day now →
  click T2).

## Risk / rollback

UI-only, additive data shapes; rollback = revert the commit. Partial-save
ordering (adds before closes/deletes) is preserved by keeping the existing
save fan-out in `class-settings-page.tsx`.
