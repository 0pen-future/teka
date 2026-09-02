---
title: "Review — Classbook Option C completion pass"
plan: plans/260902-1718-GH-260902-classbook-option-c-complete/plan.md
reviewer: code-reviewer
date: 2026-09-02
verdict: DONE_WITH_CONCERNS
---

# Review — Classbook Option C completion pass

## Scope

Working-tree diff vs HEAD, restricted to this pass:

- `apps/web/src/features/roster/lib/roster-format.ts` (+41)
- `apps/web/src/features/roster/index.ts` (+12/-5)
- `apps/web/src/features/roster/__tests__/roster-format.test.ts` (+53)
- `apps/web/src/features/teaching/components/class-select.tsx` (rewritten, 160 lines touched)
- `apps/web/src/features/teaching/components/month-stepper.tsx` (rewritten)
- `apps/web/src/features/teaching/components/sessions-table.tsx` (attendance cell)
- `apps/web/src/features/teaching/pages/classbook-page.tsx` (+28)
- `apps/web/src/features/teaching/__tests__/classbook-page.test.tsx` (+56)

## Gates

| Check | Result |
|-------|--------|
| `make lint-web` (eslint + prettier + tsc -b) | exit 0 — 0 errors, 5 pre-existing `react-hooks/incompatible-library` warnings, all in files untouched by this pass |
| `vitest run src/features/roster/__tests__/roster-format.test.ts src/features/teaching/__tests__/classbook-page.test.tsx` | 2 files, 27 tests passed |
| `vitest run src/features/teaching src/features/roster src/features/attendance` | 35 files, 274 passed / 3 skipped |

## Acceptance criteria

| Criterion | Status |
|-----------|--------|
| `formatScheduleLabel` shapes + Sáng/Chiều/Tối bands, unit-tested | Met, with one ordering deviation (L1) |
| Combobox text starts with `Toán 6A · Tối Thứ Ba`, options match, trigger shows `1 HS · Cô Lan` | Met |
| Class pick still routes through `requestNavigation` | Met, and now covered by a new "Ở lại" test that proves the controlled Select does not keep a rejected pick |
| "Tháng trước"/"Tháng sau" names kept, one white shadow card | Met |
| CÓ MẶT number + bar inline, `aria-label="Có mặt N%"` unchanged | Met on `sm+`; the bar (and its label) is `hidden` below `sm` — see M2 |
| `make test-web` / `make lint-web` green, no weakened tests | Lint green; the three suites above are green; full `make test-web` not run |

## Findings

### H1 — The class picker's search was removed, and the plan did not ask for it

`apps/web/src/features/teaching/components/class-select.tsx:1-93`

The plan scopes this file to "bold label, secondary line, `.sel` look". The file was
instead rewritten from an `HvModal` picker (`useClassSearch` + `ClassSearchInput` +
`ClassSearchEmptyNote`, `aria-haspopup="dialog"`) to a Radix `Select`. That drops the
case-insensitive substring search that `useClassSearch`
(`apps/web/src/features/roster/hooks/use-class-search.ts:19-21`) reveals once a center
has more than 5 classes. The component's new doc comment states Radix typeahead
"replaces a search box", but typeahead is prefix-only against `textValue`
(`"Toán 6A · Tối Thứ Ba"`), so a teacher can no longer find "Anh 9B" by typing "9b".
`SelectContent` also renders every class with no cap.

The removed search is an earlier product decision still in force on the two sibling
screens (`features/attendance/pages/sessions-page.tsx:65`,
`features/teaching/pages/records-page.tsx:36`), so `/classbook` now diverges from them.

Fix: confirm the removal with the user. If a dropdown is wanted, keep the filter — e.g.
render the trigger as the `.sel` card but keep the modal list underneath, or move to a
command/combobox pattern that accepts a query. If the removal stands, record it in the
plan's non-goals so the divergence from the other two pickers is deliberate.

### M1 — `aria-label` on the trigger hides which class is selected from screen readers

`apps/web/src/features/teaching/components/class-select.tsx:60`

`aria-label="Chọn lớp"` overrides the trigger's content as its accessible name. Radix's
trigger carries no `aria-labelledby`, so the value is only in the text content, which the
label now shadows. The previous button announced `Chọn lớp — đang xem Toán 6A`
(HEAD `class-select.tsx`), so this is a regression for AT users.

Fix:

```tsx
aria-label={selected ? `Chọn lớp — đang xem ${classLabel(selected, today)}` : "Chọn lớp"}
```

and revert the test matchers to `screen.getByRole("combobox", { name: /^Chọn lớp/ })`
(`classbook-page.test.tsx:178, 189, 442, 452, 466, 480`).

### M2 — The attendance bar and its `aria-label` disappear below `sm`

`apps/web/src/features/teaching/components/sessions-table.tsx:242`

`className="hidden w-16 sm:block"` sets `display:none` on phones, which removes the bar
from the accessibility tree together with `aria-label="Có mặt N%"` and the under-70%
`missing` colour cue. The `13/14` text stays, so the loss is the percentage and the
low-attendance signal, exactly on the viewport where the ledger is hardest to scan. The
test suite cannot catch this: jsdom applies no Tailwind CSS, so `hidden` is inert in
tests.

Fix: either keep the bar at a smaller width on phones, or add an `sr-only` span carrying
`Có mặt N%` so the figure survives the visual hide.

### L1 — Multi-slot label order does not match the plan's example

`apps/web/src/features/roster/lib/roster-format.ts:82-93`

The plan's criterion writes `"Tối T3-T5, Sáng T7"`, but `deriveScheduleSlots`
(`schedule-diff.ts:73`) sorts slots by start time ascending, so the code produces
`"Sáng Thứ Bảy, Tối T3-T5"`. The new test at `roster-format.test.ts:88-98` encodes the
implementation's order. Chronological order is the better behaviour; fix the plan text
rather than the code.

### L2 — `mondayFirst` duplicated

`roster-format.ts:53` and `roster-format.ts:78` define the same closure. Hoist one
module-level `const mondayFirst = (weekday: number) => (weekday === 0 ? 7 : weekday);`.

### L3 — Untested unreachable branch in `formatDayPart`

`roster-format.ts:65` returns `""` when the hour will not parse, and
`formatScheduleLabel:90` then drops the day part. `scheduleSchema.start_time` is
`z.string()` so malformed data is conceivable, but nothing exercises the branch. Either
add a case to the new describe block or drop the guard.

### L4 — Month stepper diverges from local DS habits in three small ways

`apps/web/src/features/teaching/components/month-stepper.tsx:11-12`

- 36px tap targets (`h-9 w-9`) against the repo's `min-h-11` habit; the plan mandates the
  36px squares, so this is informational only.
- `duration-[var(--dur-fast)]` without the `ease-[var(--ease-out)]` that every other
  usage pairs it with (`hv-button.tsx:13`, `hv-segmented.tsx:44`, `hv-card.tsx:23`).
- `rounded-[12px]` is literally `var(--radius-sm)`; only one other file hardcodes it.

### L5 — Headcount can pair with the wrong class for one frame

`classbook-page.tsx:152,338`. `enrollmentsPage` keeps the previous page while switching
class, so the trigger can briefly render the new class name beside the previous class's
`N HS`. Pre-existing for the SĨ SỐ KPI, now also visible in the picker. Not worth fixing
unless it shows in practice.

## Verified, not issues

- **Unsaved-scores guard**: `onSelect` still calls `requestNavigation`
  (`classbook-page.tsx:341`). The Select is fully controlled (`value={selected?.id ?? ""}`),
  Radix treats `""` as "show placeholder", and the new "Ở lại" test proves a rejected pick
  does not stick and re-triggers the guard.
- **KPI SĨ SỐ** value is the same expression, now via the extracted `headcount` const.
- **`formatScheduleSummary`** keeps its four callers (dashboard cards, classes tab,
  enroll dialog); only `class-select` stopped using it, as the plan intends.
- **Public contracts**: `features/roster/index.ts` additions are purely additive;
  `ClassSelect`'s `headcount` went required → optional (a loosening) and the only caller
  is `classbook-page.tsx`.
- **`useClassStaff`**: `enabled: Boolean(classId)`, keyed per class, 30s `staleTime` — one
  cached request per class, no waterfall, no refetch loop. The server gate on
  `GET /classes/:id/staff` (`classstaff/service.go:35-39`) is owner-or-stint-or-center-wide,
  the same visibility that puts the class in the classbook list, so a 404 storm is not
  expected; if it did 404, `data` is undefined and the name simply does not render.
- **Teacher lookup** (`role_key === "giao_vien" && ended_at === null`, first match) matches
  `class-staff-section.tsx:83` (`activeByRole[giao_vien]?.[0]`, `isActive` = `ended_at === null`).
- **`formatScheduleLabel` edges**: weekday 0 sorts last and spells "Chủ Nhật";
  `"HH:MM:SS"` is normalised by `toHhmm` inside `deriveScheduleSlots`; closed rows are
  excluded; no schedule → `""`. All four are covered by the new tests.
- **react-refresh**: no non-component export was added to either component file
  (`classLabel`, `labelClassName`, `arrowClassName` stay module-private).
- The `viewOptions` icons in `classbook-page.tsx:56-62` type-check against
  `HvSegmentedOption.icon` and the `table` / `file` icon names exist.

## Unresolved questions

1. Was dropping the class search (H1) a user decision, or a side effect of chasing the
   mock's `.sel` look? It is the only blocking question.
2. Should the plan's `"Tối T3-T5, Sáng T7"` example be corrected to chronological order
   (L1), or is the day-part order the intended reading?
3. Is hiding the attendance bar on phones (M2) part of the approved mock, or an
   unreviewed addition to the "one line" requirement?
