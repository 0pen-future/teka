# Code review — HvSelect consumer migration (Phase 2–4)

Reviewer: `code-reviewer` subagent (static review; sandbox blocks node_modules so
green claims were reproduced by the lead). Diff: working tree vs `ba5c9df`,
9 components, 10 test files, 4 e2e specs, 1 new helper, 2 deletions.

## Verdict

Migration is faithful: all 10 inventory positions render `HvSelect`, the four
D7 guards exist, D3 naming strategy is applied without mixing, `sheetTitle` is
required by the type so TypeScript enforces it everywhere. No Critical finding
(no trust boundary, input handling or authorization path touched).

## Code-verifiable success criteria

| Criterion | Result |
|---|---|
| Phase 2 greps (`RecordsClassSelect`, `ui/select` in teaching, `"button", { name: /^Lớp`) | 0 hits |
| Classbook option separator assertion split into two regexes | pass |
| Phase 3 greps (`ui/select` in audit/collections/roster) | 0 hits |
| Audit action-group search always visible with own noun (20 groups > 5) | pass |
| New `aria-invalid` test on payment method | pass, real path (422 → `useApiFormErrors` → `setError`) |
| Phase 4 greps (`<select`, `selectOptions`, e2e `selectOption(`) | 0 hits (criterion narrowed, see phase-04) |
| Handoff test has positive option count | pass |
| Re-pick does not PUT role / does not un-arm | pass |
| `components/ui/select.tsx` still present (deleted in Phase 5) | pass |
| No `@/features` import under `components/hv`; no edits under `components/ui` | pass |
| Consumer prop contracts unchanged | pass |

## Findings and resolution

| # | Sev | Finding | Resolution |
|---|---|---|---|
| H1 | High | 4 files over prettier print width; `format:check` is a required CI step | Fixed: `prettier --write`, `format:check` clean |
| M2 | Med | `class-select.tsx` dropped `min-w-0`; long class name could stop shrinking in the `sm:flex-nowrap` classbook header | Fixed: `w-full min-w-0 sm:w-fit sm:max-w-[520px]` |
| M3 | Med | permissions test anchored on `span.rounded-full` (HvBadge utility class) | Fixed: `getByText(…, { ignore: '[role="combobox"], [role="combobox"] *' })` |
| M4 | Med | `classbook-page.test.tsx`, `students-page.test.tsx` drive HvSelect without `mockViewport` → sheet branch only | Fixed: `mockViewport(1024)` in both `beforeEach`; `hidden: true` query at classbook guard still needed (HvModal guard) |
| L5 | Low | audit teacher picker has no `placeholder` | Rejected: `teacherOptions` always contains `{ value: "all" }` and `value` falls back to `"all"`, so the trigger never hits the placeholder; "Tất cả giáo viên" would be wrong while an actor filter is live |
| L6 | Low | stale comment about "hidden native options" in audit test | Fixed |
| L7 | Low | `!member` guard in `handleRoleChange` looks dead | Rejected: removing it fails `tsc` (`TS18048 'member' is possibly 'undefined'`) — hoisted function declarations do not inherit const narrowing; same pattern as `handleSave` |
| L8 | Low | three files import `@/components/hv` twice (value + `import type`) | Fixed: inline `type` import |
| L9 | Low | handoff picker relies on parent `*:w-full` instead of D10 `className` | Fixed: `className="w-full"` |
| L10 | Low | un-arm path (change target) untested | Fixed: new test "un-arms the confirm step when the target changes" |
| P1 | — | no consumer-level test that audit "Tùy chỉnh" renders disabled | Fixed: new test in `audit-page.test.tsx` |
| P2 | — | override re-pick guard untested | Fixed: new D7 test in `center-permissions.test.tsx` |

## Post-fix verification (lead)

- `npm run test`: 85 files, 660 passed, 3 skipped.
- `npm run lint`: 0 errors (5 pre-existing warnings in unrelated files).
- `npm run format:check`: clean. `tsc -b --noEmit`: clean. `npm run build`: OK.
- Tester report: `test-260908-phase2-4-consumers.md` (no flakiness across 3 runs per touched file).

## Deferred to Phase 5

Visual criteria (popover chrome, sheet on 375px, disabled option styling),
Playwright specs `records-search`, `class-staff-read`, `class-staff-write`,
`secretary-send`, `roster` on the isolated `teka-e2e` stack.

## Bổ sung muộn từ reviewer (nhận sau khi báo cáo đã chốt)

- **E2e:** `playwright.config.ts` không đặt viewport → các spec chạy 1280 (popover), riêng `records-search.spec.ts` ghim 375 và assert đúng sheet. `class-staff-read`, `class-staff-write`, `secretary-send` lấy option qua `page` (portal ra `body`), khớp `exact: true` vẫn đúng vì option bàn giao/override không có meta và icon ✓ là `aria-hidden`. `data-value` phát vô điều kiện nên assert-then-set ở `secretary-send.spec.ts:45` giữ idempotent; `ensureClassTeacher` trong `afterEach` đã sửa đồng bộ. → Xác nhận thực nghiệm ở Phase 5: 8/8 pass, chạy lại 4/4.
- **Helper `src/test/pick-option.ts`:** lookup trigger đồng bộ trên container, option qua `screen.findByRole` nên độc lập nhánh popover/sheet. Chấp nhận.
- **Test `aria-invalid` ở `record-payment-dialog.test.tsx:91`:** đi qua 422 giả → `useApiFormErrors` → `setError` → prop `aria-invalid`, đúng đường production (zod client không sinh lỗi cho `method` vì mặc định `"cash"`). Lệch câu chữ plan theo hướng tốt hơn. Chấp nhận.
- Trạng thái cuối: DONE_WITH_CONCERNS — H1 (prettier) đã sửa bằng `prettier --write` và `npm run format:check` xanh; M2–M4 đã xử lý (xem bảng trên).
