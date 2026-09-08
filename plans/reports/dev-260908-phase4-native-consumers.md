# Phase 4 — Native Select Consumers

Plan: `plans/260908-0832-dropdown-design-system/phase-04-native-select-consumers.md`
Status: DONE

## Files modified

- `apps/web/src/features/center/components/member-permissions-dialog.tsx` —
  role select and per-permission override select swapped to `HvSelect`. Added
  a D7 guard (`if (!member || roleId === (member.role_id ?? "")) return;`) to
  `handleRoleChange` and an inline D7 guard on the override `onValueChange`.
- `apps/web/src/features/roster/components/class-staff-section.tsx` — staff
  member picker swapped to `HvSelect`. Plain state setter, no D7 guard needed
  (no side effect on re-pick).
- `apps/web/src/features/roster/pages/class-settings-page.tsx` — teacher
  handoff picker swapped to `HvSelect`. Added `handleTargetChange` with a D7
  guard so re-picking the armed target does not un-arm the confirm step.
- `apps/web/src/test/pick-option.ts` (new) — shared
  click-combobox-then-click-option test helper. The pattern repeated 11 times
  across the four touched test files (2+2 new D7 tests in
  `center-permissions.test.tsx`, 3 in `center-page.test.tsx`, 4 in
  `class-staff-section.test.tsx`, 3+1 new D7 test in
  `class-settings-handoff.test.tsx`), well past the 2-repeat threshold, so a
  shared helper was the DRY call. Options are always looked up through the
  global `screen` (never `within(dialog)`) since `HvSelect` options portal to
  `document.body` via Radix.
- `apps/web/src/features/center/__tests__/center-permissions.test.tsx` —
  `selectOptions`/`toHaveValue` → `pickOption`/`toHaveTextContent`; added
  `mockViewport(1024)` in `beforeEach`; added a new D7 test ("does not send a
  role PUT when re-picking the currently assigned role").
- `apps/web/src/features/center/__tests__/center-page.test.tsx` — same
  pattern swap across 3 tests exercising the `reports.send` override select;
  added `mockViewport(1024)`.
- `apps/web/src/features/roster/__tests__/class-staff-section.test.tsx` —
  4 occurrences of `selectOptions` → `pickOption`; added `mockViewport(1024)`
  inside the existing `beforeEach`.
- `apps/web/src/features/roster/__tests__/class-settings-handoff.test.tsx` —
  `select`/`selectOptions` → `pickOption`; added `mockViewport(1024)`; added a
  new D7 test ("keeps the arm state when re-picking the already-armed teacher").
- `apps/web/e2e/class-staff-write.spec.ts` — the class-picker locator's role
  changed `button` → `combobox` (`/^Lớp/`); `ensureClassTeacher`'s
  `selectOption({ label })` → click-combobox + `page.getByRole("option", …)`
  (option list portals to `body`, outside the card).
- `apps/web/e2e/secretary-send.spec.ts` — `inputValue()` →
  `getAttribute("data-value")`; `selectOption(target)` → click-combobox +
  click-option through an `OVERRIDE_MODE_LABELS` lookup table
  (`grant`/`inherit`/`deny` → `Cấp riêng`/`Theo vai trò`/`Chặn riêng`);
  assert-then-set idempotency preserved unchanged.

## Tasks completed

- [x] All 4 native `<select>` elements replaced with `HvSelect` per spec
      (options/value/onValueChange/placeholder/disabled/sheetTitle/className
      per component).
- [x] D7 guards added everywhere a select drives a side effect (role assign,
      handoff arm/confirm). Override-mode select's inline guard was already
      in the code I wrote from the start.
- [x] `mockViewport(1024)` added to every touched test file's `beforeEach`.
- [x] Value-reading in tests/e2e switched to `toHaveTextContent` /
      `data-value`, never `toHaveValue` / `inputValue()`.
- [x] Two new D7-guard tests added (role re-pick no-op PUT; handoff re-pick
      keeps arm state).
- [x] Shared `pick-option.ts` test helper created and documented above.
- [x] `class-config-page.test.tsx` — confirmed non-existent, no action taken
      (matches the phase's own caveat).

## Tests status

- Type check: **pass** (`npm run typecheck` clean, 0 errors project-wide).
- Unit tests: **pass** — `npx vitest run src/features/center
  src/features/roster/__tests__/class-staff-section.test.tsx
  src/features/roster/__tests__/class-settings-handoff.test.tsx
  src/features/roster/__tests__/class-config-page.test.tsx` → 8 files, 67
  tests, all green (the nonexistent last path is silently ignored by vitest's
  glob, no error).
- Lint: **pass** — `npx eslint` on every touched file → 0 errors. One
  pre-existing warning in `class-settings-page.tsx` (React Compiler
  incompatible-library skip on `form.watch(...)`, line 123) — unrelated to
  this phase's edits, not touched.
- e2e: **not run** (per task constraint — edited by inspection only).

## Grep verification

- `grep -rn "<select" src` → 0 matches project-wide (phases 2/3 also clean by
  now).
- `grep -rn 'selectOptions|toHaveValue'` scoped to the 4 touched test files →
  0 matches.
- `grep -n 'selectOption|inputValue'` scoped to the 2 touched e2e specs → 0
  matches. (Repo-wide `e2e/` still has one unrelated `inputValue()` in
  `invite-accept.spec.ts`, reading a text-input invite link, not a select —
  outside this phase's scope.)

## Issues encountered

- `npm run typecheck` initially failed with `TS18048: 'member' is possibly
  'undefined'` at `handleRoleChange` in `member-permissions-dialog.tsx`.
  Cause: TypeScript does not carry control-flow narrowing of an outer `const`
  into a nested **function declaration** (only into function
  expressions/arrows defined after the narrowing check) — same reason
  `handleSave` in the same file already re-guards `if (!data || !member)`
  even though it is defined after the top-of-component `if (!data || !member)
  return`. Fixed by adding the same local guard convention to
  `handleRoleChange` (`if (!member || roleId === (member.role_id ?? ""))
  return;`).
- One test collision from the `HvSelect` swap: `HvSelect`'s trigger renders
  its selected option's label in a `<span>`, which collided with
  `center-permissions.test.tsx`'s `getByText("Cấp riêng", { selector: "span"
  })` / `getByText("Chặn riêng", { selector: "span" })` assertions — those
  were meant to find the `HvBadge` showing the *effective source*, but now
  also matched the override select's own trigger label when its current value
  happened to equal the same text. Fixed by scoping the selector to
  `span.rounded-full` (the `HvBadge` base class, absent from the `HvSelect`
  trigger's label span), which disambiguates without weakening the
  assertions.

No other file-ownership conflicts observed; `git status` confirms only files
in this phase's ownership list plus the new `pick-option.ts` were touched.

## Next steps

Phase 4 is complete and self-verified. No dependents blocked by this phase per
the plan. Team lead can proceed to cross-phase integration/verification once
phases 2 and 3 report in.
