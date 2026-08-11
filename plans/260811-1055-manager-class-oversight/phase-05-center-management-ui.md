---
phase: 5
title: "Center Management UI"
status: pending
priority: P2
effort: "1.5d"
dependencies: [2]
---

# Phase 5: Center Management UI

## Overview

Web UI (React, `apps/web`) cho feature centers của Phase 2: xem center + thành viên, đổi tên (owner), gia nhập center theo SĐT owner, rời center, remove member (owner). Chỉ phủ 4 endpoint centers — dashboard owner (Phase 4) là UI plan riêng sau khi API chốt.

## Requirements

- Functional: trang "Trung tâm" — thông tin center, danh sách member với badge owner, form join theo SĐT, nút rời/remove với confirm.
- Non-functional: theo feature-folder pattern hiện có của `apps/web`; UI copy phản ánh đúng ràng buộc API (join yêu cầu chưa có dữ liệu — giải thích trước khi user bấm, đừng để 409 mới biết).

## Architecture

- Feature folder mới `apps/web/src/features/center/` theo bố cục hiện có (đối chiếu `apps/web/src/features/profile/`): `api/`, `hooks/`, `components/`, `pages/`, `schemas/`, `__tests__/`, `routes.tsx`, `index.ts`.
- **Data layer**: TanStack Query — `useCenter` (GET `/centers/me`), `useRenameCenter`, `useJoinCenter`, `useRemoveMember` — mutation invalidate `["center","me"]`. Axios qua `@/lib/api`. Join/leave thành công phải invalidate rộng (`queryClient.invalidateQueries()` toàn bộ hoặc reload) — đổi center nghĩa là **mọi** dữ liệu cache (classes, students, dashboard...) thuộc scope cũ.
- **Routing**: `routes.tsx` export lazy routes, mount trong `apps/web/src/app/router.tsx` dưới `DashboardLayout` (path `/center`); nav item trong `apps/web/src/layouts/dashboard-layout.tsx`.
- **Role-gating**: `is_owner` từ `GET /centers/me` — nút rename/remove chỉ render cho owner; backend vẫn là chốt chặn (403).
- **Contract phản chiếu Phase 2** (schemas zod validate response):
  - `GET /centers/me` → `{center: {id, name, is_owner}, members: [{id, full_name, phone, is_owner}]}`.
  - `PATCH /centers/me {name}` → 200; 403 cho member.
  - `POST /centers/join {owner_phone}` → `201 {center_id, joined_at}`; 404/409/422.
  - `DELETE /centers/me/members/:teacherId` → 204; 404 idempotent → UI coi như "đã rời", refetch, không lỗi đỏ.
- **Phone validation**: zod mirror `vnphone` — pattern `phoneField` tại `apps/web/src/features/roster/schemas/roster-schemas.ts:12`. Client chặn sớm cho UX, backend là chốt.
- **Error mapping** (`@/lib/forms/use-api-form-errors.ts`): 404 → "Không tìm thấy chủ trung tâm với số này"; 409 → "Tài khoản của bạn đã có dữ liệu hoặc thành viên — chưa thể gia nhập trung tâm khác"; 422 → "Không thể tự gia nhập trung tâm của chính mình".

## UI Design

Trang `Trung tâm` (`/center`):

1. **Header center**: tên center + badge "Chủ trung tâm"/"Thành viên"; owner có nút sửa tên (dialog form pattern `contact-dialog.tsx`).
2. **Section "Thành viên"**: danh sách tên + phone + badge owner; owner thấy nút Xóa từng member (confirm `AlertDialog` pattern `anonymize-student-dialog.tsx`); member thường thấy nút "Rời trung tâm" cho chính mình (confirm nêu rõ: dữ liệu đã tạo ở lại trung tâm).
3. **Section "Gia nhập trung tâm khác"** (chỉ hiện khi caller là owner center cá nhân một mình): form 1 field SĐT (react-hook-form + zod) + copy giải thích điều kiện (tài khoản chưa có dữ liệu); submit → toast sonner, invalidate toàn bộ cache.
4. Loading skeleton + error state theo component shared hiện có.

## Related Code Files

- Create: `apps/web/src/features/center/{index.ts,routes.tsx}`
- Create: `apps/web/src/features/center/api/center-api.ts`
- Create: `apps/web/src/features/center/hooks/use-center.ts`
- Create: `apps/web/src/features/center/schemas/center-schemas.ts`
- Create: `apps/web/src/features/center/pages/center-page.tsx`
- Create: `apps/web/src/features/center/components/{rename-center-dialog.tsx,join-center-dialog.tsx,remove-member-dialog.tsx,member-list.tsx}`
- Create: `apps/web/src/features/center/__tests__/` (page + dialogs + schemas + handlers theo pattern `features/profile/__tests__/zalo-handlers.ts`)
- Modify: `apps/web/src/app/router.tsx` (mount routes)
- Modify: `apps/web/src/layouts/dashboard-layout.tsx` (nav item)

## Implementation Steps

1. Schemas zod (request/response, `phoneField` mirror `vnphone`).
2. `center-api.ts` + hooks (get/rename/join/remove + invalidation; join invalidate toàn bộ).
3. `center-page.tsx` với header + members + join section, role-gated.
4. Dialogs: rename, join (error mapping 404/409/422), remove/leave (confirm + copy "dữ liệu ở lại", 404 → idempotent).
5. Routes + nav; kiểm tra lazy chunk.
6. Tests: render theo 2 role (owner/member); join happy + từng nhánh lỗi; remove/leave happy + 404-idempotent; validation phone; assert copy cảnh báo "dữ liệu ở lại trung tâm".
7. `npm run lint && npm run typecheck && npm run test` trong `apps/web`.

## Success Criteria

- [ ] Member list đúng theo role; nút owner-only không render cho member (và backend 403 nếu gọi lách)
- [ ] Join thành công → toàn bộ app phản ánh center mới không cần đăng nhập lại (cache invalidate sạch)
- [ ] Rời/remove: confirm nêu rõ dữ liệu ở lại; thao tác lần 2 (404) hội tụ về trạng thái "đã rời", không lỗi đỏ
- [ ] Ba nhánh lỗi join 404/409/422 hiện message tiếng Việt đúng ngữ nghĩa
- [ ] Lint + typecheck + vitest pass; route lazy-load đúng pattern

## Risk Assessment

- **Cache scope cũ sau khi đổi center**: rủi ro chính của UI — user join xong còn thấy classes center cá nhân cũ trong cache. Chốt bằng invalidate toàn bộ + test assert refetch.
- **UI hứa quá quyền** (member thấy nút owner): role-gate từ `is_owner` + test render theo role; backend luôn là chốt.
- **Lệch contract khi Phase 2 đổi DTO**: zod validate runtime → fail sớm; sửa một chỗ.
