---
phase: 4
title: "Web owner config page"
status: done
priority: P1
effort: "1d"
dependencies: [2]
---

# Phase 4: Web owner config page

## Overview

Trang cấu hình lớp học owner-only trong feature `center`: CRUD bộ điểm +
gán bộ cho lớp, cùng entry sidebar mới trong nhóm "Trung tâm".

## Requirements

- Functional:
  - Entry sidebar "Cấu hình lớp học" trong nhóm Trung tâm, gate
    `isResolved && isOwner` (pattern "Phân quyền vai trò",
    `dashboard-layout.tsx:114`).
  - Route `/center/class-config` lazy trong `center/routes.tsx`.
  - Trang: danh sách bộ điểm (tên + components theo position), tạo/sửa
    (modal), xóa (confirm); bảng lớp với bộ đang gán + hành động gán/gỡ.
  - Lớp đã có điểm: disable gán/gỡ + tooltip/ghi chú lý do; map 409 từ API
    thành thông báo tiếng Việt.
- Non-functional: hv-* design system (hv-button, hv-card, hv-modal,
  status-pill); API qua `src/lib/api/`; Vitest + MSW offline.

## Architecture

- Feature `center` mở rộng — không tạo web feature mới (decision 6):
  - `api/grading.ts` — client các endpoint phase 2.
  - `schemas/grading.ts` — zod schemas (theo cách schemas/ hiện có).
  - `hooks/use-score-sets.ts` — TanStack Query (list/mutations, invalidate
    theo key).
  - `components/score-set-editor-modal.tsx`, `assign-score-set-dialog.tsx`.
  - `pages/class-config-page.tsx`.
- Danh sách lớp: tái dùng hook/API lớp hiện có (soi feature `roster`/
  classbook dùng gì để list classes) thay vì endpoint mới.
- Guard trang: như `center-permissions-page` gate owner (soi cách nó chặn
  non-owner — redirect hay ẩn) và làm giống.

## Related Code Files

- Modify: `apps/web/src/layouts/dashboard-layout.tsx` (entry + OVERFLOW_LABELS)
- Modify: `apps/web/src/features/center/routes.tsx`
- Create: `apps/web/src/features/center/{api/grading.ts,schemas/grading.ts,hooks/use-score-sets.ts}`
- Create: `apps/web/src/features/center/components/{score-set-editor-modal.tsx,assign-score-set-dialog.tsx}`
- Create: `apps/web/src/features/center/pages/class-config-page.tsx`
- Create: `apps/web/src/features/center/__tests__/class-config-page.test.tsx`

## Implementation Steps

1. Soi `center-permissions-page.tsx` + `permission-matrix.tsx` để khớp
   guard, layout, MSW test setup.
2. API layer + zod + hooks.
3. Page + modals; editor validate tên thành phần trùng ngay client (server
   vẫn là nguồn chuẩn).
4. Sidebar entry + OVERFLOW_LABELS (bottom-bar <md).
5. Tests: render owner thấy entry/page, member không thấy entry; CRUD flow
   qua MSW; 409 khi gán lớp có điểm hiển thị message.
6. `npm run typecheck && npm run test` trong `apps/web`.

## Success Criteria

- [ ] AC1: entry owner-only, member không thấy (test).
- [ ] AC2: CRUD bộ điểm với validate tên thành phần không trùng.
- [ ] AC5 phía UI: lớp có điểm → không cho gán, thông báo rõ.
- [ ] typecheck + vitest pass.

## Risk Assessment

- Bảng lớp list toàn trung tâm cho owner: đã verify
  `classes/repository.go` widen qua `sc.CenterWide()` — owner thấy
  center-wide, tái dùng API hiện có an toàn.
- Sidebar Trung tâm đã 6 entries; thêm 1 vẫn ổn desktop, mobile đi vào
  overflow sheet (bước 4).
