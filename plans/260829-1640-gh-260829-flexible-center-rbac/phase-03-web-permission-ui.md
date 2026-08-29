---
phase: 3
title: "Web permission UI"
status: completed
priority: P1
effort: "1.5d"
dependencies: [2]
---

# Phase 3: Web permission UI

## Overview

Permission management inside the existing "Trung tâm" page
(`apps/web/src/features/center/`): role display + role assignment in the
member list, a permission matrix for roles, and per-member overrides. The
send-reports dialog folds into this UI.

## Requirements

- Functional: owner sees a "Phân quyền" area in the center section — role
  permission matrix (roles × catalog keys, vi labels from API) and, per
  member: role select + override editor. Non-owner sees none of it (API
  already 403s; UI simply doesn't render).
- Non-functional: follow `docs/frontend-guidelines.md` + existing center
  feature patterns (`center-api.ts`, zod schemas, TanStack Query hooks, HV
  design-system components, MSW handlers in tests).

## Architecture

- `GET /centers/me/permissions` → one query hook (`use-center-permissions`)
  feeding both matrix and member editors; mutations invalidate it plus
  `useCenter`.
- Role matrix: table of role rows × permission columns with checkboxes;
  save-per-role via `PUT /centers/me/roles/:roleId/permissions` (replace
  semantics — send full checked set). The `reports.send` cell is DISABLED on
  role rows with a tooltip ("cấp theo từng thành viên") — API rejects it
  during dual-life (see phase 2); unlocked in phase 4.
- Member editor: extend the existing member row/dialog pattern — role
  `<select>` (giáo viên/học vụ/trợ giảng) + override list showing effective
  source (từ role / cấp riêng / chặn riêng). Owner row displays "Chủ trung
  tâm" badge, no editor (implicit superuser, not a role row).
- Replace `SendReportsPermissionDialog` usage: the send-reports toggle becomes
  the `reports.send` key inside the member override editor; legacy dialog and
  its endpoints stop being called by the UI (removal itself is phase 4).
- Labels come from API catalog response — no TS-side label map.
- Member-side effect: `/centers/me` now carries the caller's effective
  permission keys (phase 2) — gate member nav/pages on them
  (`use-center-context.ts`, `dashboard-layout.tsx`) so a member granted e.g.
  `audit.read` or `dashboard.view` actually sees those surfaces. Members with
  NULL/default role render the default "Giáo viên" badge.
- `teaching.review_queue` surfaces (nav hiding is NOT the only guard):
  `pages/lesson-plans-page.tsx` hard-redirects non-owners (deep-link guard,
  see its header comment :20-23) — swap `isOwner` → key check; same for
  `hooks/use-review-queue.ts` `enabled: isOwner` (:22), else the nav badge
  never loads for a grantee. Hide/disable approve/redo/reopen actions in the
  plan-review panel for non-owners (write gate stays `IsOwner`, API 403s).
  Grant recipe note for docs/UI hint: the page assembles its queue from
  member-scoped repo reads, so meaningful center-wide review visibility =
  `teaching.review_queue` + `data.view_center_wide` (financial dashboard still
  gated separately by `dashboard.view`).
  <!-- Updated: Validation Session 1 (2026-08-29) — kongming post-validation
       counsel: review-queue web guards + action-button hiding -->
- E2e: update `apps/web/e2e/secretary-send.spec.ts` IN THIS PHASE to drive the
  new override UI (the legacy dialog stops rendering here — deferring the spec
  to phase 4 would leave e2e red for the whole soak window).

## Related Code Files

- Modify: `apps/web/src/features/center/pages/center-page.tsx`,
  `components/member-list.tsx`, `api/center-api.ts`,
  `schemas/center-schemas.ts`, `hooks/use-center.ts`,
  `apps/web/src/features/teaching/pages/lesson-plans-page.tsx`,
  `apps/web/src/features/teaching/hooks/use-review-queue.ts`,
  plan-review panel component (non-owner action hiding)
- Create: `apps/web/src/features/center/components/permission-matrix.tsx`,
  `components/member-permissions-dialog.tsx`,
  `hooks/use-center-permissions.ts`, tests + MSW handlers under
  `__tests__/`
- Delete (phase 4, not here): `send-reports-permission-dialog.tsx`

## Implementation Steps

1. Schemas + API client + query/mutation hooks for the 4 endpoints.
2. Permission matrix component (roles × keys), optimistic-free save (simple
   invalidate; permission edits are rare).
3. Member role select + overrides dialog; fold send-reports toggle in.
4. Wire into center page under owner-only branch (`"members" in data`).
5. Vitest + MSW: matrix render/save (incl. disabled `reports.send` role cell),
   role change, override grant/deny, member nav gating on effective perms,
   non-owner-without-perms renders nothing, error states.
5b. Update `apps/web/e2e/secretary-send.spec.ts` to the new UI; run the
   isolated e2e stack (compose `teka-e2e`, fresh seed) green.
6. `npm run lint` + `tsc` + vitest green; run app (`make`/compose dev) for a
   manual pass of the flow.

## Success Criteria

- [x] Owner: edits role matrix, changes a member's role, grants/denies an
      override — all persist and re-render from server state.
- [x] Send-reports management fully available through the new UI (old dialog
      unused; file deletion stays in phase 4).
- [x] Non-owner center page unchanged from today.
- [x] Vitest suite green (62 files, 411 passed / 3 skipped); lint + typecheck
      clean; isolated e2e stack green (26/26, secretary-send driven through
      the new override dialog).

## Risk Assessment

- **Matrix UX overwhelms with raw keys** → vi labels + grouped rows from API
  ordering; keys never shown to end users.
- **Stale member list after mutations** → invalidate both permission and
  center queries in every mutation's onSuccess.
- **Design drift** → reuse HvCard/HvButton/HvBadge patterns already in the
  center feature; no new design primitives.
