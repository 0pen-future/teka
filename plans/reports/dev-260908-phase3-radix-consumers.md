# Phase 3 Report: Radix Select → HvSelect (audit, thanh toán, ghi danh)

Plan: `plans/260908-0832-dropdown-design-system/phase-03-radix-select-consumers.md`
Status: DONE

## Files Modified
- `apps/web/src/features/audit/components/audit-filters.tsx` — 2 `Select` → `HvSelect` (Giáo viên, Nhóm hành động), option lists built with `HvSelectOption[]` (`teacherOptions`, `actionGroupOptions`); guard-free `set()` on change per spec.
- `apps/web/src/features/collections/components/record-payment-dialog.tsx` — `Select` → `HvSelect id="payment-method"`, module-level `methodOptions` built from `methodLabels`; `aria-invalid` wired from `errors.method`.
- `apps/web/src/features/roster/components/enroll-student-dialog.tsx` — `Select` → `HvSelect id="enroll-class"`; `classSelectOptions` built with `meta: \`${formatScheduleSummary(...)} · ${formatMoney(...)}/buổi\``, unchanged `formatScheduleSummary`/`formatMoney` helpers.
- `apps/web/src/features/audit/__tests__/audit-page.test.tsx` — added `mockViewport(1024)` in `beforeEach` (D9); no other changes needed, existing tests already used `combobox`/`option` roles.
- `apps/web/src/features/collections/__tests__/record-payment-dialog.test.tsx` — added `mockViewport(1024)` in `beforeEach`; added new test asserting `getByRole("combobox", { name: "Hình thức" })` exists (proves `FieldLabel htmlFor="payment-method"` wiring) and gets `aria-invalid="true"` after a server field error on `method` (drives `errors.method` through `useApiFormErrors`, since the form's own default value is always a valid method and can't go empty through the UI — this is the only reachable way to make `errors.method` truthy through user interaction).
- `apps/web/src/features/roster/__tests__/enroll-student-dialog.test.tsx` — added `mockViewport(1024)` in `beforeEach`; test unchanged otherwise (already used `combobox`/`option`).

## Tasks Completed
- [x] Audit: both pickers on `HvSelect`, `aria-label`/`sheetTitle` Giáo viên & Nhóm hành động, `searchNoun` per picker, `w-[180px]`, `all` → "Tất cả …", `custom` disabled "Tùy chỉnh" only when `isCustomAction`, change applies filter directly.
- [x] Record payment: `HvSelect id="payment-method"`, `w-full`, `sheetTitle="Hình thức"`, `aria-invalid` from `errors.method`.
- [x] Enroll student: `HvSelect id="enroll-class"`, placeholder "Chọn lớp…", `meta` format exactly as specified, `searchNoun="lớp"`, `sheetTitle="Chọn lớp"`.
- [x] D9 viewport mocks added to all three test files.
- [x] New `aria-invalid`/name test added to `record-payment-dialog.test.tsx`.
- [x] `components/ui/select.tsx` left untouched (still imported by `teaching/components/class-select.tsx`, Phase 2's file — not deleted here per D8).

## Tests Status
- Type check: touched files clean. Two pre-existing errors remain outside ownership (not introduced by this phase, not fixed per scope): `src/features/center/components/member-permissions-dialog.tsx:87` (`member` possibly undefined) and `src/features/roster/__tests__/class-staff-section.test.tsx:10` (`pickOption` unused) — both belong to Phase 4's in-progress files.
- Unit tests: `npx vitest run src/features/audit src/features/collections src/features/roster/__tests__/enroll-student-dialog.test.tsx src/features/roster/__tests__/students-page.test.tsx` → 7 files, 60 tests, all green.
- Lint: `npx eslint` on all 6 touched source+test files → clean, no output.
- `grep -rn "components/ui/select" apps/web/src/features/{audit,collections}` and roster's `enroll-student-dialog.tsx` → 0 matches.

## Issues Encountered
None. No popover/sheet-in-`HvModal` misbehavior observed on the real `record-payment-dialog`/`enroll-student-dialog` consumers (both wrapped in `HvModal`); Phase 1's nested-dialog handling held up unmodified. No file ownership conflicts detected.

## Next Steps
Phase 5 can delete `apps/web/src/components/ui/select.tsx` once Phase 2 and Phase 4 also land (D8 gate). Manual 1024px/375px visual checks from the phase's Success Criteria are explicitly deferred to Phase 5 per the task brief.

Status: DONE
Summary: All 4 Radix Select consumers (audit ×2, payment method, enroll class) now use HvSelect with spec-matching props; 3 test files updated per D9, one new aria-invalid test added; 60 tests green, lint clean, no stray `components/ui/select` imports in owned files.
Concerns/Blockers: none.
