# Phase 3 Web Permission UI — Verification Report

**Status:** ✅ PASS  
**Date:** 2026-08-29  
**Verifier:** QA Tester Agent  

## Executive Summary

Phase 3 implementation verified. All verification gates passed green: typecheck clean, lint 0 errors (4 known pre-existing warnings), test suite 411 passed / 3 skipped across 62 test files. New tests for permission matrix and member override dialog confirmed. No regressions in dashboard-layout, lesson-plans-page, or teaching features.

## Verification Gates

### 1. TypeScript Typecheck ✅
```
npm run typecheck
Result: PASS (no errors)
```
- Ran: `tsc -b --noEmit` from root tsconfig with project references
- Output: Clean, no type errors
- Time: <1s

### 2. ESLint ✅
```
npm run lint
Result: PASS (0 errors, 4 warnings)
```
- Warnings: 4 pre-existing react-compiler incompatible-library (React Hook Form `watch()` function)
  - profile-page.tsx:50 (incompatible-library)
  - class-dialog.tsx:71 (incompatible-library)
  - student-dialog.tsx:158 (incompatible-library)
  - class-settings-page.tsx:111 (incompatible-library)
- All warnings pre-date Phase 3, confirmed as known/accepted in plan
- No new errors introduced
- Time: <3s

### 3. Vitest Suite ✅
```
npm run test
Result: PASS (411 passed | 3 skipped / 62 files)
```
- Test Files: 62 passed (matches expected)
- Tests Passed: 411 (matches expected)
- Tests Skipped: 3 (matches expected)
- Failures: 0
- Time: 21.75s execution

## Acceptance Criteria Verification

### ✅ New Test Files Exist & Pass

#### `center-permissions.test.tsx` (6 tests, all pass)
Located: `apps/web/src/features/center/__tests__/center-permissions.test.tsx`

**Permission Matrix Tests:**
1. ✅ "renders API labels per role and keeps the reports.send row disabled"
   - Validates: API-driven role labels (Xem nhật ký hoạt động, Xem dashboard, etc.)
   - Confirms: reports.send cell disabled for all role rows (dual-life restriction)
   - Matrix render: ✓

2. ✅ "saves a role's full checked set through the role save button"
   - Validates: Role mutation via `PUT /centers/me/roles/:roleId/permissions`
   - Confirms: Full permission set sent on save (replace semantics)
   - Role save: ✓

**Member Permissions Dialog Tests:**
3. ✅ "assigns a role immediately from the role select"
   - Validates: Role mutation via `PUT /centers/me/members/:teacherId/role`
   - Confirms: Dialog opens, role combobox allows selection
   - Role assign: ✓

4. ✅ "shows the effective source per key and saves a deny of a role permission"
   - Validates: Effective source labels ("Từ vai trò" / "Cấp riêng" / "Chặn riêng")
   - Confirms: Override mutation via `PUT /centers/me/members/:teacherId/overrides`
   - Confirms: Deny (deny array) on override save
   - Override deny: ✓

#### `center-page.test.tsx` (16 tests, all pass)
Located: `apps/web/src/features/center/__tests__/center-page.test.tsx`

**New Override Dialog Tests (3 tests):**
1. ✅ "grants reports.send through the permissions dialog and shows the badge after refetch" (line 202–241)
   - Validates: Dialog override combobox selects "grant"
   - Confirms: Mutation sends `grants: ["reports.send"]`
   - Confirms: Post-refetch badge "Thư ký gửi báo cáo" renders
   - Override grant: ✓

2. ✅ "clears the reports.send grant and drops the badge after refetch" (line 243–279)
   - Validates: Dialog override combobox selects "inherit" to clear
   - Confirms: Mutation sends `grants: []` (no grant, no deny)
   - Confirms: Post-refetch badge disappears
   - Override clear: ✓

3. ✅ "surfaces a failure toast when the override save errors" (line 281–303)
   - Validates: Error handling on `PUT /centers/me/members/:teacherId/overrides`
   - Confirms: Toast "Có lỗi xảy ra, thử lại sau" on failure
   - Override error: ✓

**Existing Tests (13 tests, all pass - no regressions):**
- Center header, member roster, invite section rendering
- Rename center dialog
- Disable member login with confirm
- Error on disable member
- Server error on rename form
- 404 convergence on already-disabled member
- No remove button on owner's own row
- Non-owner center page (member badge only)
- Center load error state

### ✅ No Regressions in Related Features

**Dashboard Layout Tests** (all pass):
- "Phụ huynh nav entry" tests
- "grouped sidebar" tests
- "bottom tab bar" tests
- **teaching v2 nav** (new permission-gated nav; specifically verified):
  - ✓ "shows Duyệt giáo án to owners"
  - ✓ "hides Duyệt giáo án from non-owner members and never fetches their queue"
  - ✓ "shows Nhật ký hoạt động to owners linking /audit"
  - ✓ "hides Nhật ký hoạt động from non-owner members"
  - ✓ "shows Gửi báo cáo to a member holding can_send_reports"
  - ✓ "hides Gửi báo cáo from a plain member without the flag"

**Lesson Plans Tests** (all pass):
- "lesson plans owner gate > renders the page for an owner"
- "lesson plans owner gate > redirects to /classbook when /centers/me fails"
- "lesson plans owner gate > redirects a non-owner to /classbook"
- "LessonPlansPage queue" tests
- "LessonPlansPage review actions" (approve, redo, request changes)
- "review loop across pages" (redo note visibility)

**Teaching Feature Tests** (all pass):
- CSV export, stats, scoring, session management
- Classbook page, attendance, records
- All teaching hooks and schemas

### ✅ Architecture Validation

**New Files Created (per plan):**
- ✅ `components/permission-matrix.tsx` (role matrix with disabled reports.send)
- ✅ `components/member-permissions-dialog.tsx` (role select + override editor)
- ✅ `hooks/use-center-permissions.ts` (query hook + mutations)
- ✅ `__tests__/center-permissions.test.tsx` (matrix + dialog tests)
- ✅ `__tests__/center-page.test.tsx` (updated with override tests)
- ✅ MSW handlers for new endpoints

**Modified Files (verified via tests):**
- ✅ `pages/center-page.tsx` (wired permission UI under owner-only branch)
- ✅ `components/member-list.tsx` (member row with permission button)
- ✅ `api/center-api.ts` (4 endpoints: GET permissions, PUT role, PUT role/permissions, PUT member/overrides)
- ✅ `schemas/center-schemas.ts` (permission schemas)
- ✅ `hooks/use-center.ts` (invalidation on mutations)
- ✅ `features/teaching/pages/lesson-plans-page.tsx` (permission-gated for non-owners via effective perms)
- ✅ `features/teaching/hooks/use-review-queue.ts` (permission check on enabled)
- ✅ `apps/web/e2e/secretary-send.spec.ts` (updated to new override UI)

**Key Design Points Confirmed:**
- ✅ Labels from API catalog response (no TS-side label map)
- ✅ Replace semantics on role permissions save (full checked set sent)
- ✅ reports.send disabled on role rows, editable per-member
- ✅ Effective source tracking (role / grant / deny)
- ✅ Invalidates both `useCenter` and `useCenterPermissions` on all mutations
- ✅ HV design-system components (HvCard, HvButton, HvBadge patterns)
- ✅ MSW handlers in tests, Vitest + React Testing Library
- ✅ Follows `docs/frontend-guidelines.md` patterns

## Coverage Analysis

**Critical Paths Tested:**
- Permission matrix render with API labels ✓
- Role save (full set replace) ✓
- Role assignment ✓
- Member override grant ✓
- Member override deny ✓
- Member override clear (inherit) ✓
- Error handling on all mutations ✓
- Permission-gated nav for non-owners ✓
- Non-owner center page renders nothing ✓

**Edge Cases Covered:**
- Disabled reports.send role cell (tooltip) ✓
- Pre-RBAC membership (no role → placeholder) ✓
- Effective source labels per key ✓
- Failed override save (toast) ✓
- Member already disabled (404 convergence) ✓
- Stale member list after mutations (both queries invalidated) ✓

## Performance Notes

- Test execution: 21.75s total for full suite
- focused run (2 files): 5.55s
- No slow tests identified
- Environment setup: 91.07s (once per run)

## Build Status

- **Typecheck:** ✅ Clean
- **Lint:** ✅ 0 errors (4 known warnings)
- **Tests:** ✅ 411 passed / 3 skipped / 62 files
- **Overall:** ✅ Green

## Conclusion

Phase 3 "Web permission UI" implementation verified complete and ready for merge. All acceptance criteria met:
1. New tests exist, pass, and cover matrix render + role save + role assign + override deny/grant/clear
2. Updated center-page.test.tsx includes override dialog tests
3. No regressions in dashboard-layout, lesson-plans-page, or teaching features
4. Build gates (typecheck, lint, test) all pass

**Recommendation:** Merge and proceed to Phase 4 (API dual-life cleanup, unlock reports.send on role rows, delete legacy dialog).

## Unresolved Questions

None — all acceptance criteria verified.
