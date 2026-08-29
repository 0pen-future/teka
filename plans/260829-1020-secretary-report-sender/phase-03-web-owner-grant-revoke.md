---
phase: 3
title: "Web: owner grant/revoke UI"
status: done
priority: P2
effort: "0.5d"
dependencies: [1]
---

# Phase 3: Web: owner grant/revoke UI

## Overview

Give the owner a per-member toggle on the center page to grant/revoke the
"gửi báo cáo" permission, with a badge showing who holds it.

## Requirements

- Functional: owner sees a toggle per active non-owner member on `/center`;
  toggling asks for confirmation, calls the PATCH endpoint, refreshes the
  roster; members holding the flag show a badge.
- Non-functional: member view of `/center` unchanged; optimistic-free (simple
  invalidate — roster is small); Vietnamese copy consistent with existing
  center page tone.

## Architecture

- Extend `centerSchema` shapes (`apps/web/src/features/center/schemas/center-schemas.ts:4-51`):
  `CenterMember` gains `can_send_reports: boolean`; the member (`CenterMeMember`)
  shape gains own `can_send_reports` (needed by Phase 4 gating).
- New API fns `grantSendReports(teacherId)` / `revokeSendReports(teacherId)`
  in `features/center/api/` mapping to Phase 1's
  `POST`/`DELETE /centers/me/members/:teacherId/send-reports`; one TanStack
  mutation taking the desired state, invalidating the `/centers/me` query key
  (pattern: existing remove-member mutation).
- UI in `member-list.tsx:18-49`: alongside the existing `is_owner` badge and
  remove button, render an HvBadge "Thư ký gửi báo cáo" when flagged and a
  toggle built from HvButton states (no switch component exists in the
  generated shadcn/ui set — do not add one; follow the hv design system),
  gated the same way as `canRemove` (owner-only, non-owner rows). Confirm via `HvModal` (pattern:
  `remove-member-dialog.tsx`), copy explaining what the permission allows:
  đọc bảng kê/công nợ toàn trung tâm + gửi báo cáo bằng Zalo cá nhân của
  chính thành viên đó. Copy must also state the exclusivity model (plan.md
  D8): giáo viên thường không tự gửi báo cáo — chỉ người giữ quyền này (và
  chủ trung tâm) gửi được, nên hãy đảm bảo luôn có ít nhất một người giữ
  quyền khi trung tâm muốn thư ký phụ trách việc gửi.

## Related Code Files

- Modify: `apps/web/src/features/center/schemas/center-schemas.ts`
- Modify: `apps/web/src/features/center/api/` (center API module)
- Modify: `apps/web/src/features/center/components/member-list.tsx`
- Create: `apps/web/src/features/center/components/send-reports-permission-dialog.tsx`
- Modify: center feature unit tests (vitest + MSW)

## Implementation Steps

1. Extend schemas + API fns + mutation hook.
2. Build the confirm dialog component; wire into `member-list.tsx` for owner.
3. Unit tests: toggle visible only for owner and non-owner rows; grant fires
   POST, revoke fires DELETE, both refresh the roster; badge renders from
   data.

## Todo

- [x] Schemas + API + mutation
- [x] Dialog + member-list wiring + badge
- [x] Vitest/MSW coverage for grant, revoke, member-view unchanged

## Success Criteria

- [x] Owner toggles the permission end-to-end against the real API (manual
      check on dev stack); roster reflects state after refresh

## Risk Assessment

- Low. Schema change is additive; parse failures guarded by zod defaults —
  coordinate deploy order (API first) since zod will reject a missing required
  field: mark `can_send_reports` with `.default(false)` to tolerate an older
  API during rollout.
