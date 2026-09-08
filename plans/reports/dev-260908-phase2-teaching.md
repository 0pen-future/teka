# Phase 2 report — Teaching class pickers on HvSelect

Plan: `plans/260908-0832-dropdown-design-system/phase-02-teaching-consumers.md`
Status: **DONE**

## Files modified

- `apps/web/src/features/teaching/components/records-toolbar.tsx` — swapped `RecordsClassSelect` import/usage for `HvSelect` per the Architecture snippet (labelId, sheetTitle "Chọn lớp", groupLabel "Lớp đang dạy", searchNoun "lớp", placeholder "Chọn lớp", options with `meta: \`${k.student_count} HS\``, className `min-w-[230px] max-sm:w-full`).
- `apps/web/src/features/teaching/components/class-select.tsx` — rewritten from Radix `Select` to `HvSelect`. Kept `classLabel` for the `aria-label` string only; options now `{ value, label: klass.name, meta: formatScheduleLabel(...) || undefined }`; `sheetTitle`/`searchNoun`/`placeholder` all "Chọn lớp"/"lớp"; `className="w-full sm:w-fit sm:max-w-[520px]"`; kept the `if (classId !== selected?.id) onSelect(classId)` guard (D7). Removed the `ui/select` import and the stale accent-variable comments.
- `apps/web/src/features/teaching/__tests__/records-toolbar.test.tsx` — 2 locators `button`→`combobox`.
- `apps/web/src/features/teaching/__tests__/records-pages.test.tsx` — 5 locators `button`→`combobox` (incl. the 375px sheet test, which keeps its `dialog` assertion unchanged).
- `apps/web/src/features/teaching/__tests__/classbook-page.test.tsx` — line ~444: replaced the single `toHaveTextContent("Toán 6B · Tối Thứ Ba")` on the option with two regex assertions (`/Toán 6B/`, `/Tối Thứ Ba/`), matching D4's no-separator option markup. Nothing else in this file needed touching — it already used `role="combobox"` throughout (Radix Select already exposed that role) and none of its `dialog` queries collide with the new sheet branch (D9's file list for mandatory `mockViewport` doesn't include this file, confirmed empirically: full suite green with no `mockViewport` added here).
- `apps/web/e2e/records-search.spec.ts` — 3 locators `button`→`combobox`.
- `apps/web/e2e/class-staff-read.spec.ts` — 1 locator `button`→`combobox` (line 39, inside `assertStaffReadJourney`).

## Files deleted

- `apps/web/src/features/teaching/components/records-class-select.tsx`
- `apps/web/src/features/teaching/__tests__/records-class-select.test.tsx`

## Tasks completed

- [x] `records-toolbar.tsx` on `HvSelect`, old import gone.
- [x] `records-class-select.tsx` + its test deleted.
- [x] `class-select.tsx` rewritten on `HvSelect`, `ui/select` import gone, guard kept.
- [x] All `button`→`combobox` locator updates (2 unit + 5 unit + 3 e2e + 1 e2e = 11 sites); `class-staff-write.spec.ts:35` left untouched for Phase 4.
- [x] `classbook-page.test.tsx:444` split into two regex assertions.

## Tests status

- `npx vitest run src/features/teaching`: **17 files / 144 tests passed.**
- `npx eslint` on all touched files: clean, no output.
- `npm run typecheck`: 5 errors, **all in `src/features/center/**`** (Phase 4's ownership — `center-permissions.test.tsx` unused imports + missing `@/test/pick-option` module, `member-permissions-dialog.tsx:87` possibly-undefined). None in files I own; not fixed, listed for the Phase 4 agent.

## Verification greps (all match required output)

- `grep -rn RecordsClassSelect src` → 0 results.
- `grep -rn "ui/select" src/features/teaching` → 0 results.
- `grep -rn '"button", { name: /\^Lớp' src e2e` → only `e2e/class-staff-write.spec.ts:35` (Phase 4's).
- `grep -rn '" · "' src/features/teaching/__tests__` → 0 results (no separator asserted on any option).

## Issues encountered

None inside my file ownership. The pre-existing typecheck errors in `apps/web/src/features/center/**` are outside scope (Phase 4) and were left for that agent.

## Next steps

Phase 2 is complete and self-contained; nothing blocks Phase 3/4/5. Phase 5's full-suite/build/e2e pass will need Phase 4's typecheck errors resolved first, but that's outside this phase.

Status: DONE
Summary: Teaching's two class pickers (records toolbar, classbook) now run on `HvSelect`; old Radix-based `records-class-select.tsx` deleted; all 11 `button`→`combobox` locator sites updated except Phase 4's line; 144 teaching unit tests green, lint clean, typecheck clean in-scope (5 pre-existing errors live in Phase 4's `features/center` files).
Concerns/Blockers: none.
