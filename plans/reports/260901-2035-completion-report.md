# Completion Report: class-students-center-tabs

Plan: `plans/260901-2035-class-students-center-tabs/` (branch `teka/260901-2035`, uncommitted). All 4 phases completed, all success criteria ticked w/ evidence.

## Shipped

1. **Nav move**: "Lớp & học sinh" moved from "Dạy học" group to "Trung tâm" group, owner-only (`isResolved && isOwner` spread, same pattern as "Phân quyền vai trò"). Mobile: entry dropped from bottom bar into "Thêm" overflow sheet.
2. **Owner-only guard**: `StudentsPage` split into shell (guard) + `StudentsPageContent`. Non-owner hitting `/students` directly redirects to `/` with zero roster requests (proven via `server.events` test, same pattern as `center-permissions-page.tsx`).
3. **3-tab restructure** (Lớp học | Học sinh | Chưa ghi danh), underline pattern copied from `permission-matrix.tsx`, state in `?tab=` URL param w/ `{replace:true}`. Tab-inference rule preserves all pre-existing `?class_id=` deep links (no link changes needed at call sites). New "Lớp học" tab: class list + create + per-row settings link, zero roster/session/enrollment requests when active.
4. **Member touchpoint sweep**: dashboard "Lớp mới" card hidden for non-owner; roster-import-page non-owner copy rewritten (dead link removed); class-settings-page back-link now role-aware (owner → `/students?tab=students&class_id=`, non-owner → `/records?class_id=`); grep confirms `/students` only remains in owner-context usage + route defs.
5. **E2e rewrite**: `class-staff-read.spec.ts`, `class-staff-write.spec.ts` member roster assertions moved to `/records?class_id=`; `roster.spec.ts` updated for new tab-default layout.

## User-accepted consequences

- **hoc_vu (class-role) lost the UI path to send class-scoped reports.** Discovered mid-phase-4: the only entry was the roster page's "Gửi báo cáo" button (`ClassSendPeriodsDialog`), now owner-only; `/reports` needs center-wide `can_send_reports` which class-role hoc_vu lacks; notifications-page needs a period id only discoverable via that same dialog. User confirmed 2026-09-01: accept the loss. `ClassSendPeriodsDialog` deleted as dead code, class-scoped variants of `useReportPeriods`/`listReportPeriods` trimmed, the e2e test for this flow removed. **API still allows class-scoped send — UI path only was cut.**
- **Member enrollment surface reduced.** `EnrollExistingStudentDialog` was only reachable from the now owner-only roster page. Remaining path for a member to enroll an existing student into their own class: `/contacts/:id` → student detail → "Ghi danh vào lớp", but reaching `/contacts/:id` needs `contacts.list`, which a member may lack. No new surface added; accepted per owner-only decision (plan.md "Accepted consequences").

## Verification evidence

- `npm run typecheck` — clean.
- `npm run test` (web) — Vitest 68 files, 480 passed / 3 skipped.
- `make lint` / equivalent — 0 errors.
- `git diff --stat apps/api/` — empty; `CATALOG_VERSION` unchanged at 3 (tester + reviewer independently confirmed).
- E2e on fresh isolated `teka-e2e` stack (seed 2026-09-01): 26 passed, all 3 rewritten specs (`class-staff-read`, `class-staff-write`, `roster`) green.
- Gate review: reviewer found 1 Medium (classes-tab swallowed `/classes` load error as empty state) — fixed same session (thread `isError` from `useClassesList`, render proper error state, add unit test). 2 Low findings logged as accepted non-issues, no code change (dashboard card layout shift on `!isOwner`; cosmetic mobile "Thêm" tab highlight on `/students/:id`).

## Out-of-scope issue (noted, not fixed here)

Master's Web CI e2e suite has been red since PR #39 (`b915a50`, 2026-08-31): billing, collections, and 3 statement specs fail on a fresh seed. Last known-green commit: `a0704ed` (2026-08-30). This predates and is unrelated to this plan's branch point — confirmed by running the full e2e suite on master@2efc779 (this plan's base), which shows the same 5 failures plus the old hoc_vu-send test this plan removed. Needs its own bugfix plan.

## Unresolved questions

None.
