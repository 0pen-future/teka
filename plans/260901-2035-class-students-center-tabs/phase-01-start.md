---
phase: 1
title: "Nav move + owner guard + entry-point sweep"
status: completed
priority: P1
effort: "5h"
dependencies: []
---

# Phase 1: Nav move + owner guard + entry-point sweep

## Overview

Chuyển entry nav "Lớp & học sinh" sang nhóm "Trung tâm" (owner-only, mobile
vào sheet "Thêm"), tách `StudentsPage` thành component-vỏ guard + content để
non-owner redirect với zero request, và ẩn card "Lớp mới" trên dashboard với
non-owner. Sau phase này member không còn thấy/vào được trang; nội dung trang
chưa đổi.

## Requirements

- Functional: entry chỉ hiện cho owner trong nhóm "Trung tâm"; non-owner vào
  `/students` bị redirect `/`; non-owner không thấy card "Lớp mới".
- Non-functional: redirect **zero request** roster (chứng minh bằng test
  `server.events.on("request:start")` theo mẫu `students-page.test.tsx:161-177`).

## Architecture

- **Nav desktop** (`apps/web/src/layouts/dashboard-layout.tsx`): xoá entry
  `{ label: "Lớp & học sinh", to: "/students", Icon: HvUsersIcon, perm: "students.list" }`
  khỏi nhóm "Dạy học" (dòng ~84); thêm vào nhóm "Trung tâm" bằng conditional
  spread owner-only (bỏ field `perm`), đặt ngay trước "Phân quyền vai trò"
  để cụm owner-only liền nhau (pattern dòng 135-148):
  ```tsx
  ...(isResolved && isOwner
    ? [{ label: "Lớp & học sinh", to: "/students", Icon: HvUsersIcon }]
    : []),
  ```
- **Nav mobile**: thêm `"Lớp & học sinh"` vào `OVERFLOW_LABELS`
  (`dashboard-layout.tsx:166-179`) và `"/students"` vào
  `OVERFLOW_PATH_PREFIXES` (`:186-197`, hiện chỉ có `/students/import`) —
  entry rời bottom bar, vào sheet "Thêm" như mọi entry Trung tâm khác
  (quyết định user 2026-09-01).
- **Guard component-vỏ** (`apps/web/src/features/roster/pages/students-page.tsx`):
  KHÔNG chèn early-return giữa các hook — React Query hooks vẫn subscribe và
  fire trước khi `<Navigate>` có hiệu lực. Sao đúng hình dạng tiền lệ
  `center-permissions-page.tsx:13-21`: export `StudentsPage` mới chỉ là vỏ:
  ```tsx
  export function StudentsPage() {
    const { isOwner, isResolved, isError } = useCenterContext();
    if (!isResolved && !isError) return null;
    if (!isOwner) return <Navigate to="/" replace />;
    return <StudentsPageContent />;
  }
  ```
  Toàn bộ thân hiện tại chuyển nguyên khối sang `StudentsPageContent`
  (cùng file). Content vẫn tự gọi `useCenterContext()` cho `isOwner`/
  `canRunSends` như hiện tại — dọn ở phase 3.
- **Card "Lớp mới"**
  (`apps/web/src/features/dashboard/components/class-overview-cards.tsx:35`):
  card link `/students?class_id=` render cho mọi member có `classes.list` —
  sau guard sẽ redirect vòng về dashboard. Quyết định user: **ẩn card với
  non-owner** (component lấy `isOwner` từ `useCenterContext`). Sửa luôn
  copy empty-state (`:108`) nhắc "Lớp & học sinh" chỉ khi owner.

## Related Code Files

- Modify: `apps/web/src/layouts/dashboard-layout.tsx`
- Modify: `apps/web/src/features/roster/pages/students-page.tsx`
- Modify: `apps/web/src/features/dashboard/components/class-overview-cards.tsx`
- Modify: `apps/web/src/layouts/__tests__/dashboard-layout.test.tsx` — các
  test vỡ đã kiểm kê: `:49-62` (thứ tự "Phụ huynh" trong Dạy học), `:71-77`
  (map nhóm), `:78-84` (danh sách nhóm Trung tâm), `:117-124` (bottom bar
  `toEqual([...])`), `:152-158` (đổi `getByRole` → `findByRole` vì entry giờ
  gate bất đồng bộ theo owner), `:176-181`, `:245-256` (thứ tự Dạy học),
  `:266+` (thứ tự Trung tâm), test sheet "Thêm"
- Modify: `apps/web/src/features/roster/__tests__/students-page.test.tsx`
  (guard test cần `extraRoutes: [{ path: "/", element: ... }]` theo mẫu
  `center-permissions.test.tsx:42`; test member hiện có đổi ngữ nghĩa thành
  test redirect, phần còn lại sign-in owner)
- Modify: `apps/web/src/features/dashboard/__tests__/dashboard-page.test.tsx:223`
  (card "Lớp mới")

## Implementation Steps

1. Sửa `buildNavGroups`: xoá entry khỏi "Dạy học", thêm spread owner-only
   vào "Trung tâm" trước "Phân quyền vai trò".
2. Cập nhật `OVERFLOW_LABELS` + `OVERFLOW_PATH_PREFIXES`.
3. Tách `StudentsPage` = vỏ guard + `StudentsPageContent`.
4. Ẩn card "Lớp mới" với non-owner + sửa copy empty-state.
5. Cập nhật toàn bộ test đã kiểm kê ở Related Code Files; thêm test mới:
   (a) owner thấy entry trong "Trung tâm" đúng thứ tự, không thấy trong
   "Dạy học"; (b) member với `students.list` không thấy entry ở nhóm nào,
   bottom bar không còn tab này; (c) member render `StudentsPage`
   (`memberCenterHandler`) → redirect `/` với zero request roster
   (`server.events`); (d) member không thấy card "Lớp mới", owner thấy.

## Success Criteria

- [x] Test nav (desktop + bottom bar + sheet Thêm) xanh.
- [x] Test guard redirect + zero-request xanh.
- [x] Test dashboard card theo vai trò xanh; `npm run typecheck` xanh.

## Risk Assessment

- **`isError` đá cả owner về `/`** (`use-center-context.ts:33-43`: lỗi
  `/centers/me` ⇒ `isResolved=false, isError=true` ⇒ guard rơi nhánh
  `!isOwner`): hành vi giống hệt tiền lệ center-permissions/audit — chấp
  nhận để nhất quán; không xử lý riêng.
- **Member mất lối tắt roster/gửi báo cáo trên trang này**: mitigations đã
  xác minh — học vụ còn `/reports` + notifications-page; teacher đọc roster
  qua `/records`. Hệ quả ghi danh ghi tại plan.md "Accepted consequences".
- **Rollback**: thuần web, không migration/flag — revert commit phase này;
  phase 1 revert độc lập được với phase 2-3.
