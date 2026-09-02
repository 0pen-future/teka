---
title: "Classbook toolbar & score-table modal design-system pass"
status: completed
created: 2026-09-02
branch: teka/260902-1241
---

# Classbook toolbar & score-table modal — design-system pass

## Outcome

Three user-reported gaps on `/classbook` (Quản lý lớp học):

1. Modal "Bảng điểm" looks foreign to the design system: borderless table, fixed
   90dvh empty workspace, dot-decimal averages, sky-filled "Đóng" competing with
   the primary action.
2. Picking a class opens a modal popup; it should be a plain dropdown under the
   control.
3. The "Buổi học" / "Chương trình & giáo án" switch does not read as clickable.

## Constraints

- Keep `useScoreDraft`, the unsaved-scores guard, and URL params untouched.
- Keep a11y contracts: tablist/tab semantics for the view switch, combobox +
  listbox for the class picker, columnheader/rowheader in the table.
- No new kit primitives; reuse `ui/select` (already DS-styled) and restyle
  `HvSegmented` in place.

## Non-goals

- No ledger/table layout change on the page itself, no HvTable extraction.
- No changes to the Điểm danh or Hồ sơ class pickers (they use pills).
- No search input inside the dropdown (Radix Select typeahead covers it).

## Acceptance criteria

- [x] Score table: header band cream-200 / 12px extrabold uppercase ink-500,
      sticky; body rows `border-b border-line-100`; TB and panel chip render
      "7,5"; `HvModal size="xl"` sizes to content up to 90dvh; footer is
      ghost "Đóng" + primary "Lưu điểm".
- [x] Class picker: `role="combobox"` named "Chọn lớp", opens a listbox under
      the trigger (popper), options show name + schedule; choosing a class
      routes through `requestNavigation` so the dirty guard still fires.
- [x] View switch: bordered white group, active item mint-400 filled with white
      text, each option carries an icon; tab semantics unchanged.
- [x] `make test-web`, `make lint-web` green; touched tests updated, none
      weakened.

## Phases

| # | Scope | Files |
|---|-------|-------|
| 1 | Score table modal DS pass | `components/hv/hv-modal.tsx`, `features/teaching/components/score-table-modal.tsx`, `features/teaching/hooks/use-row-cells.ts`, tests |
| 2 | Class dropdown | `features/teaching/components/class-select.tsx`, `pages/classbook-page.tsx`, `__tests__/classbook-page.test.tsx` |
| 3 | Segmented affordance | `components/hv/hv-segmented.tsx`, `pages/classbook-page.tsx` (icons), kit test |

## Decisions taken without the user (call out in delivery)

- Dropdown trigger is one line (class name); headcount already lives in the KPI
  strip, schedule moves into each option.
- Averages switch to comma decimals everywhere `formatAverage` is used (table +
  panel chip) to match `formatLedgerScore`.
- `HvSegmented` restyle applies to both variants so the score-set editor's
  mode switch stays consistent with the classbook tabs.

## Verification (2026-09-02)

- `make lint-web` exit 0 (eslint 0 errors, 5 pre-existing warnings; prettier and
  `tsc -b` clean). `make test-web`: 78 files, 578 passed, 3 skipped.
- Code review found no blockers. Fixed from review: `text-ink-600` (no such
  token; item fell back to body ink-700) → `text-ink-500`; `formatAverage` now
  re-exports `formatLedgerScore`; added the "Ở lại" branch of the class-switch
  guard test. Left as-is: two pre-existing `text-ink-600` uses outside this
  scope (`attendance-page.tsx`, `blocking-sessions-panel.tsx`); header-band
  string duplicated across four tables (candidate for a shared primitive later).
- Not verified in a real browser: the popper dropdown with 10+ classes
  (first `position="popper"` use in the repo; jsdom does not lay out).
