# Phase 1 Test Report: HvSelect Primitive

**Date:** 2026-09-08  
**Tester:** QA Lead  
**Status:** PASSED

## Summary

HvSelect primitive passes all validation gates. 20 unit tests covering both breakpoints (popover at 1024px, sheet below sm), nested modal scenarios, disabled options, search filtering, placeholder handling, and edge cases all pass consistently. Build, typecheck, and lint clean. No new dependencies introduced.

## Test Execution Results

### 1. Unit Tests — HvSelect Component

**Command:** `npx vitest run src/components/hv`

```
Test Files  10 passed (10)
Tests       83 passed (83)
Duration    4.62s
```

**HvSelect test file:** `src/components/hv/__tests__/hv-select.test.tsx`
- **Tests:** 20 passed
- **Coverage:** Popover mode (sm and up), sheet mode (below sm), disabled states, placeholder, search filtering, accessibility names, nested modal scenarios

### 2. Full Test Suite

**Command:** `npm run test`

```
Test Files  86 passed (86)
Tests       659 passed | 3 skipped (662)
Duration    23.44s
```

**Status:** All existing tests continue to pass. No regressions detected.

### 3. Flakiness Check

**Command:** `npx vitest run src/components/hv/__tests__/hv-select.test.tsx` (3 consecutive runs)

```
Run 1: 20 passed (5.68s)
Run 2: 20 passed (5.23s)
Run 3: 20 passed (6.05s)
```

**Result:** No flakiness detected. Tests are deterministic.

### 4. Type Checking

**Command:** `npm run typecheck`

```
Status: CLEAN (no errors)
```

### 5. Linting

**Command:** `npm run lint`

```
Checked files:
  ✓ src/components/hv/hv-select.tsx — clean
  ✓ src/components/hv/__tests__/hv-select.test.tsx — clean
  ✓ src/components/hv/index.ts — clean

Pre-existing warnings in other files: 5 (React Hook Form incompatibility)
New issues: 0
```

### 6. Build

**Command:** `npm run build`

```
Status: SUCCESS
Bundle includes HvSelect in hv-*.js asset
Gzip size: within normal range
```

## Test Coverage Analysis

### Existing Tests (19 cases in hv-select.test.tsx)

#### Popover Mode (1024px+)
1. ✓ Names trigger after label, selected option, and meta
2. ✓ Opens listbox with correct aria-label (sheetTitle)
3. ✓ Renders option label and meta as adjacent spans (no separator)
4. ✓ Omits group label and meta when not provided
5. ✓ Filters past threshold using searchNoun in empty note
6. ✓ Defaults searchNoun to "mục" when undefined
7. ✓ Never shows filter when searchThreshold is Infinity
8. ✓ Reports picked value and closes (even on re-pick)
9. ✓ Navigates with arrow keys, picks with Enter, dismisses with Escape
10. ✓ Hands typing from option to filter
11. ✓ Activates focused option with Space
12. ✓ Shows placeholder while nothing selected
13. ✓ Does not open while disabled
14. ✓ Reflects aria-invalid on trigger
15. ✓ Skips disabled options with arrow keys, refuses to pick them
16. ✓ Takes accessible name from aria-label
17. ✓ Takes accessible name from label with htmlFor
18. ✓ Picks inside HvModal without closing the modal (popover case)

#### Sheet Mode (<640px)
19. ✓ Opens list in sheet, closes on pick, returns focus
20. ✓ Stacks sheet above HvModal, Escape only closes sheet

### Additional Probes (Edge Cases — All Passed)

1. **Nested HvSelect in HvModal at viewport 1024** — Keyboard-only interaction, ArrowDown and Enter navigation, focus restoration, modal stays open ✓

2. **Disabled options with search** — Filter excludes disabled options from navigation, ArrowDown from search lands on first enabled option ✓

3. **Stale value handling** — Trigger shows placeholder when value not in options, data-placeholder attribute set, no crash ✓

4. **Re-render with new options while open** — List updates to show new options when array changes during open state ✓

## Architectural Verification

### No Feature Layer Imports
✓ Verified: `grep -r "@/features" src/components/hv/` returns 0 results

### Class Token Preservation
✓ Trigger: matches records-class-select.tsx baseline  
✓ Options: matches records-class-select.tsx baseline  
✓ Popover content: includes max-h and overflow-y-auto tokens  
✓ Sheet body: wrapped in max-h-[60dvh] overflow-y-auto container  
✓ Disabled option state: aria-disabled + opacity-50 + pointer-events-none  

### Accessibility Compliance
- ✓ Combobox role with aria-haspopup="listbox"
- ✓ aria-controls points to listbox id
- ✓ aria-expanded reflects state
- ✓ aria-invalid reflects prop (when set)
- ✓ aria-selected on options (button role)
- ✓ aria-disabled on disabled options
- ✓ Roving focus with tabIndex management
- ✓ aria-label on search input: "Tìm {searchNoun}"
- ✓ aria-label on listbox: sheetTitle
- ✓ Trigger accessible name via labelId, aria-label, or label element

### Keyboard Navigation
- ✓ Enter: open from trigger, pick from option
- ✓ Space: pick from option (native button)
- ✓ ArrowUp/ArrowDown: navigate, skip disabled, wrap
- ✓ Home/End: navigate to first/last enabled
- ✓ Escape: close listbox, restore focus to trigger
- ✓ Printable key from option: hand to search input
- ✓ ArrowDown from search: focus first enabled option

### Responsive Behavior
- ✓ 1024px+: Popover anchored below trigger (Radix)
- ✓ <640px: HvModal bottom sheet with nested dialog

### Modal Nesting
- ✓ Popover inside HvModal: focus trap and onOpenChange work independently
- ✓ Sheet inside HvModal: Escape closes sheet only, focus returns to trigger, parent modal stays open

## Critical Path Coverage

| Path | Coverage |
|------|----------|
| Empty state (value="") | Placeholder shown, data-placeholder set |
| Selected state | Trigger shows "label · meta", aria-selected on option |
| Search active | Filter appears, focuses search, navigates filtered list |
| Search inactive | No filter, navigates all options |
| Disabled option | Skipped in navigation, click ignored, style applied |
| Disabled component | Button disabled attr set, onClick prevented |
| Aria-invalid | Border style applied, aria-invalid attr forwarded |
| Nested in modal (popover) | Popover renders, modal stays open, focus restored |
| Nested in modal (sheet) | Sheet in dialog, Esc only closes sheet |

## Build & Dependencies

- ✓ No new npm dependencies added
- ✓ Uses existing: `radix-ui`, `lucide-react`, `@/lib/hooks/use-media-query`, `@/lib/utils`
- ✓ Motion-reduce respected (data-open:animate-in → motion-reduce:animate-none)
- ✓ DS tokens only (no hardcoded colors/sizes)

## Known Observations

1. **Filter on search is case-insensitive substring match** — as designed
2. **First enabled option gets tabIndex=0 when nothing selected** — allows keyboard navigation without click
3. **Selected option gets tabIndex=0 when visible** — restores focus to selection on reopen
4. **ArrowDown from search focuses first option in filtered list** — not first in DOM order, by design
5. **Nested popover uses Radix's built-in modal stacking** — no additional z-index management needed for 1024px case

## Unresolved Questions

None. All success criteria from the phase spec are met.

---

**Next Steps:** Phase 1 complete and ready for consumer onboarding in Phase 2 (teaching records integration).

