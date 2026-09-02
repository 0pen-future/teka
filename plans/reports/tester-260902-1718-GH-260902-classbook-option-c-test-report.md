# Classbook Option C Design — Test Report

**Date:** 2026-09-02 17:28  
**Branch:** `teka/260902-1241`  
**Test Suite:** vitest (full run)

## Test Results

```
Test Files:  78 passed (78)
     Tests:  582 passed | 3 skipped (585)
  Duration:  23.41s
    Status:  ✅ PASS
```

All tests passed. No regressions detected.

## Linting & Type Check

```
ESLint:      5 warnings (0 errors) — React Hook Form incompatible-library warnings
             (pre-existing, unrelated to this change)
Prettier:    ✅ All matched files use Prettier code style
TypeScript:  ✅ Exit code 0 — no type errors
```

## Acceptance Criteria Coverage

### formatScheduleLabel function

✅ **Single day with time-of-day:** `"Tối Thứ Ba"`  
- Test: `formatScheduleLabel.test.ts:63–64`  
- Covers: weekday 2, start_time 18:00

✅ **Multiple days, same time-of-day:** `"Tối T2-T4-CN"`  
- Test: `formatScheduleLabel.test.ts:73–82`  
- Covers: weekday 1, 3, 0 all at 18:00 → collapsed to short names, Monday-first

✅ **Multiple time-of-day slots:** `"Sáng Thứ Bảy, Tối T3-T5"`  
- Test: `formatScheduleLabel.test.ts:85–94`  
- Covers: weekday 6 at 09:00 + weekday 2,4 at 19:00 → joined with comma

✅ **Empty schedule:** `""`  
- Test: `formatScheduleLabel.test.ts:97–108`  
- Covers: empty array returns empty string

✅ **Closed rows ignored:** Test verifies `effective_to` before today excludes row  
- Test: `formatScheduleLabel.test.ts:97–106`  
- Covers: closed row filtered out before formatting

### Classbook combobox — class selector

✅ **Combobox label shows class name + schedule:** `"Toán 6A · Tối Thứ Ba"`  
- Test: `classbook-page.test.tsx:178–181`  
- Covers: picker.toHaveTextContent("Toán 6A · Tối Thứ Ba")

✅ **Combobox label shows headcount + teacher:** `"1 HS · Cô Lan"`  
- Test: `classbook-page.test.tsx:178–181`  
- Covers: picker.toHaveTextContent("1 HS · Cô Lan")

✅ **Combobox option shows schedule:** `"Toán 6B · Tối Thứ Ba"`  
- Test: `classbook-page.test.tsx:444–447`  
- Covers: option label includes schedule to differentiate same-named classes

✅ **Class switch guards unsaved scores:**  
- Test: `classbook-page.test.tsx:431–461` ("guards switching classes while component scores are unsaved")  
- Test: `classbook-page.test.tsx:463–494` ("keeps the current class when the guard is dismissed")  
- Covers: unsaved-scores guard dialog appears; class switch blocked until user saves or discards

### Month navigation

✅ **"Tháng trước" button works:** Navigates to previous month  
- Test: `classbook-page.test.tsx:155–167` ("steps the month with the stepper")  
- Covers: Tháng 8 → Tháng 7 (Aug → Jul)

✅ **"Tháng sau" button works:** Navigates to next month  
- Test: `classbook-page.test.tsx:155–167` ("steps the month with the stepper")  
- Covers: Tháng 7 → Tháng 8 (Jul → Aug)

✅ **Month buttons also guard unsaved scores:**  
- Test: `classbook-page.test.tsx:496–515` ("guards stepping the month while component scores are unsaved")  
- Covers: month step blocked if unsaved component scores exist

## Files Changed (Read-Only Verification)

All changes verified as existing and reachable by tests:

- ✅ `apps/web/src/features/roster/lib/roster-format.ts` — new `formatScheduleLabel` function
- ✅ `apps/web/src/features/roster/__tests__/roster-format.test.ts` — test coverage
- ✅ `apps/web/src/features/teaching/pages/classbook-page.tsx` — page redesign
- ✅ `apps/web/src/features/teaching/__tests__/classbook-page.test.tsx` — e2e test coverage

## Summary

No test failures, no type errors, no linting blockers. All acceptance criteria for the classbook Option C design are covered by passing tests. The redesign maintains backward compatibility: unsaved-score guards, month navigation, and class selection all work as expected.

## Unresolved Questions

None.

---

**Status:** DONE  
**Summary:** Full test suite passed (78 files, 582 tests). All acceptance criteria for formatScheduleLabel, classbook combobox, and month navigation verified by tests.  
**Concerns/Blockers:** None.
