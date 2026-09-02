---
title: "Classbook — close the gap to design Option C"
status: completed
updated: 2026-09-02
created: 2026-09-02
branch: teka/260902-1241
design: https://claude.ai/code/artifact/51b376d6-ceb6-4b7a-b19a-8c04185c9e86 (section "Phương án C")
---

# Classbook — close the gap to design Option C

## Outcome

`/classbook` matches the "Sổ lớp mở rộng tại chỗ" mock 100%. The ledger,
expand row, KPI strip, view switch and CSV button already match (commit
f28316c + the uncommitted toolbar pass). Remaining deltas:

1. Class picker shows **`Toán 8 · Tối Thứ Ba`** in bold display font, both in
   the trigger and in every option (user ask). The mock's secondary line
   `14 HS · Cô Lan` was tried and then dropped on user review (2026-09-02):
   the trigger carries class information only.
2. Trigger reads like the mock's `.sel`: white, soft shadow, no input border.
3. Month stepper reads like the mock's `.stepper`: one white card, two 36px
   chevron squares, label in between.
4. CÓ MẶT cell: `13/14` and the bar sit on one line (mock), not stacked.

## Constraints

- Keep hooks, URL params, unsaved-scores guard, a11y roles (combobox/listbox,
  tablist) untouched.
- Roster feature stays the owner of schedule formatting; teaching imports
  from `features/roster` index only.
- No new API.

## Non-goals

- View switch stays the bordered mint-active group the user approved in the
  previous pass (mock shows the older cream well).
- No change to expand row, KPI strip, ledger columns, Điểm danh/Hồ sơ pickers.
- `formatScheduleSummary` ("T3 — 18:00") keeps its current callers.
- The picker stays a plain Radix Select without a search box, as decided in
  plan `260902-1639` (user asked for a dropdown instead of the modal). Radix
  typeahead is prefix-only, so "9b" no longer finds "Anh 9B"; Điểm danh and
  Hồ sơ keep their searchable pickers. Revisit if a center outgrows ~15 classes.

## Acceptance criteria

- [x] `formatScheduleLabel` → "Tối Thứ Ba" (one day), "Tối T2-T4-T6" (several
      days, one slot), "Sáng T7, Tối T3-T5" (several slots, start-time order), "" (no schedule);
      Sáng < 12:00 ≤ Chiều < 18:00 ≤ Tối. Unit-tested.
- [x] Combobox "Chọn lớp" text is `Toán 6A · Tối Thứ Ba`; options show the
      same label; no headcount/teacher line.
- [x] Picking a class still routes through `requestNavigation` (guard tests
      pass unchanged apart from the label assertion).
- [x] "Tháng trước" / "Tháng sau" buttons keep their names; stepper is one
      white shadow card.
- [x] CÓ MẶT: number + bar inline; `aria-label="Có mặt N%"` unchanged.
- [x] `make test-web`, `make lint-web` green; no weakened tests.

## Files

| Change | File |
|--------|------|
| add `formatScheduleLabel` | `apps/web/src/features/roster/lib/roster-format.ts`, `__tests__/roster-format.test.ts` |
| export label + `useClassStaff` + `ClassStaff` | `apps/web/src/features/roster/index.ts` |
| bold label, secondary line, `.sel` look | `apps/web/src/features/teaching/components/class-select.tsx` |
| headcount + giáo viên props | `apps/web/src/features/teaching/pages/classbook-page.tsx` |
| `.stepper` card | `apps/web/src/features/teaching/components/month-stepper.tsx` |
| inline attendance bar | `apps/web/src/features/teaching/components/sessions-table.tsx` |
| label assertions | `apps/web/src/features/teaching/__tests__/classbook-page.test.tsx` |

## Risks / rollback

- One extra query per selected class (`/classes/:id/staff`); it is small and
  cached per class. Rollback: drop the two props and the hook call.
- Long multi-slot labels truncate in the trigger (`line-clamp-1`); the full
  label stays readable in the option list.

## Verification (2026-09-02)

- `make test-web`: 78 files, 583 passed, 3 skipped.
- `make lint-web`: exit 0 (eslint 0 errors / 5 pre-existing warnings, prettier, tsc).
- Reports: `plans/reports/tester-260902-1718-GH-260902-classbook-option-c-test-report.md`,
  `plans/reports/code-reviewer-260902-1718-GH-260902-classbook-option-c-review.md`.
- Review follow-ups applied: trigger `aria-label` names the open class again
  ("Chọn lớp — đang xem Toán 6A · Tối Thứ Ba"); attendance bar stays visible on
  phones (`w-12 sm:w-16`) so its `aria-label` survives; `mondayFirst` hoisted;
  unparseable start time covered by a test; stepper arrows use `--radius-sm`
  and `--ease-out`.
- Not verified in a real browser (no dev stack in this session): popper list
  with 10+ classes, trigger truncation on narrow phones.
- Follow-up (2026-09-02, user review on production): page title on its own
  row; controls row `sm:flex-nowrap` with the picker shrinking/truncating and
  stepper + view group `shrink-0`; dropdown list pinned to DS colours (white,
  cream hover, mint-50 checked) instead of shadcn popover tokens, which turned
  dark in the user's browser; `N HS · giáo viên` line removed with its
  `useClassStaff` wiring.
