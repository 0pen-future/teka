---
title: "Phase 5: Web Audit Page"
status: done
priority: P2
effort: "1d"
dependencies: [4]
---

# Phase 5: Web Audit Page

## Overview

Trang web audit owner-only trong `apps/web`: bảng log + filter (actor,
action, khoảng thời gian) + infinite scroll theo cursor. Nav entry ẩn với
member.

## Requirements

- [x] Feature folder mới `src/features/audit` theo cấu trúc feature hiện có
      (schemas / api / hooks / components / pages / __tests__)
- [x] Route owner-only trong `src/app/router.tsx` dưới dashboard layout;
      non-owner vào thẳng URL → redirect (theo pattern owner-gating hiện có
      trong `layouts/dashboard-layout.tsx` / dashboard pages — đối chiếu lúc
      implement)
- [x] Nav entry chỉ hiện khi `isOwner` (khớp cách dashboard-layout gating)
- [x] `useInfiniteQuery` (TanStack) với `getNextPageParam = next_cursor`
- [x] Filter: actor (select từ member list — reuse hook/center member query
      hiện có), action (select nhóm chính + free text), from/to (date picker
      của design system nếu có)
- [x] Hiển thị: thời gian (local), actor_name, action, entity, status badge
      (2xx xanh / 4xx vàng / 5xx đỏ), IP; row expand xem metadata JSON

## Related Code Files

- Create: `apps/web/src/features/audit/schemas/audit-schemas.ts` (zod)
- Create: `apps/web/src/features/audit/api/audit-api.ts` (axios)
- Create: `apps/web/src/features/audit/hooks/use-audit-logs.ts`
- Create: `apps/web/src/features/audit/components/audit-table.tsx`
- Create: `apps/web/src/features/audit/components/audit-filters.tsx`
- Create: `apps/web/src/features/audit/pages/audit-page.tsx`
- Create: `apps/web/src/features/audit/__tests__/audit-page.test.tsx`
- Create: `apps/web/src/features/audit/__tests__/audit-handlers.ts` (MSW)
- Modify: `apps/web/src/app/router.tsx`
- Modify: `apps/web/src/layouts/dashboard-layout.tsx` (nav entry)
- Modify: `apps/web/src/test/msw/handlers.ts` (nếu handlers tập trung)

## Implementation Steps (TDD)

1. Đọc 1 feature tương đồng có list+filter (vd `collections` hoặc `center`)
   để khớp convention schemas/api/hooks/tests trước khi viết.
2. **Test trước** — `audit-page.test.tsx` + MSW handlers:
   - render rows từ page 1
   - scroll/nút "tải thêm" → fetch cursor page 2, append
   - đổi filter action → query mới với param đúng, reset list
   - non-owner: nav không có entry + route redirect
3. Implement schemas → api → hook → components → page → router/nav tới xanh.
4. `npm run lint && npm run typecheck && npm run test` trong `apps/web`.

## Todo

- [x] Đối chiếu feature convention + owner-gating pattern
- [x] Tests + MSW handlers (đỏ)
- [x] schemas/api/hooks/components/page (xanh)
- [x] Router + nav gating
- [x] lint + typecheck + test pass

## Success Criteria

- [x] Acceptance 5 brainstorm: owner thấy trang, member không thấy entry và
      không vào được route
- [x] UI khớp design system hiện có (component tái sử dụng, không style mới
      ngoài hệ thống)

## Risk Assessment

- Volume lớn làm chậm render → chỉ render page đã fetch (infinite query
  mặc định), không virtualize ở V1 (YAGNI, limit 50/page).
- Timezone hiển thị — dùng local timezone browser, format theo util datetime
  hiện có nếu repo đã có helper.

## Ghi chú sau review (260826)

- Review DONE_WITH_CONCERNS → chi tiết + fixes tại
  `plans/reports/review-260826-phase-05-web-audit-page.md`.
- H1: lỗi transient (fetchNextPage/refetch fail) không còn xoá rows đã render —
  error hiển thị inline, có regression test.
- H2: `formatDateTime` có test literal riêng; vitest pin `TZ=Asia/Ho_Chi_Minh`.
- M1–M3 fixed (select "Tùy chỉnh", a11y expand toggle, entity_id/actor_role
  trong panel chi tiết); M4 (date input state) chấp nhận nguyên trạng.
- Empty-state phân biệt "có filter" vs "chưa có hoạt động" theo ghi chú L4 của
  phase 4 (window ngược trả 200 rỗng).
