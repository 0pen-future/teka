# Code review — Phase 1 `HvSelect` primitive (2026-09-08)

Plan: `plans/260908-0832-dropdown-design-system/` · Branch: `feat/dropdown-design-system`
Reviewer: `code-reviewer` subagent (report relayed by lead; the subagent could not write files).

## Scope
- New: `apps/web/src/components/hv/hv-select.tsx`, `apps/web/src/components/hv/__tests__/hv-select.test.tsx`
- Modified: `apps/web/src/components/hv/index.ts` (+2 export lines)

## Independently verified by reviewer
- Class token sets: trigger (plus `min-w-[230px] max-sm:w-full` via `className`) is a strict superset of `records-class-select.tsx` with exactly 6 state tokens added; option +2 `aria-disabled:*`; content +2 max-height tokens.
- `npx vitest run src/components/hv` 83 green; `npm run test` 659 passed / 3 skipped; typecheck, eslint (new files), build clean; `grep "@/features" src/components/hv` = 0; `records-class-select.tsx`, `hv-modal.tsx` untouched.
- `aria-haspopup="listbox"` / `aria-controls` win over Radix Trigger defaults (Slot merge order). `OptionList` unmounts on close so the filter query never lingers.

## Findings and resolution
| # | Sev | Finding | Resolution |
|---|-----|---------|------------|
| H1 | High | No automated class token-set test (success criterion #1); records file disappears in Phase 2 so nothing would pin the DS tokens | **Fixed**: test "keeps the class token set of the records class dropdown" with inline expected sets for trigger/option/content |
| M2 | Medium | Nested-in-`HvModal` popover test covered mouse only; keyboard path (focus trap pull-back) unproven | **Fixed**: test now waits for option focus, `ArrowDown`, `Enter` |
| M3 | Medium | `focusInitial` last fallback targeted a possibly disabled option (focus() no-op) → empty list in sheet left focus behind the overlay (reachable: enroll dialog with no classes on mobile) | **Fixed**: last fallback is the `role="listbox"` div; new sheet test with `options=[]` |
| M4 | Medium | Default placeholder "Chọn…" + `data-placeholder:*` typography differ from old `/records` empty state | **Plan updated**: Phase 2 must pass `placeholder="Chọn lớp"`; Phase 5 QA screenshots the empty state and confirms generated CSS for the new variants |
| L1 | Low | `aria-controls` emitted while closed = dangling IDREF (axe `aria-valid-attr-value`) | Open — D2 was a user decision; surfaced at the Phase 1 gate |
| L2 | Low | `align` prop untested / no consumer needs it | Test added ("aligns the popover to the trigger end"); keep-or-drop surfaced at gate |
| L3 | Low | Duplicate option values break `key` | Accepted: all consumers key by server ids |
| L4 | Low | No `aria-describedby` prop; `record-payment-dialog` error text is not wired to the trigger today either (parity, not regression) | Surfaced at gate |
| L5 | Low | Coverage skew 18 popover / 2 sheet cases | Sheet cases now 3; shared `OptionList` covers the rest |
| L6 | Low | Breakpoint change while open remounts the surface | Same as original; accepted |

## Side effects
None: no consumer imports `HvSelect` yet; existing exports intact; 659 existing tests green.

Status after fixes: 6/6 Phase 1 success criteria met (H1 closed); 23 tests in `hv-select.test.tsx`.
