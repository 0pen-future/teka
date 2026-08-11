---
phase: 3
title: "Delegation Grants UI"
status: pending
priority: P1
effort: "1d"
dependencies: [2]
---

# Phase 3: Delegation Grants UI

## Overview

Web UI (React, `apps/web`) cho tính năng delegation grants của Phase 2: giáo viên cấp quyền giám sát cho manager theo SĐT, xem hai chiều grant (đã cấp / được nhận), và thu hồi. Chỉ phủ 3 endpoint grants — dashboard giám sát của manager (Phase 4) vẫn là UI plan riêng.

## Requirements

- Functional: form cấp grant theo SĐT; danh sách `given` (manager tôi đã cấp) và `received` (GV tôi được quản); thu hồi từ cả hai danh sách với confirm.
- Non-functional: theo đúng feature-folder pattern hiện có của `apps/web`; UI copy phản ánh đúng ràng buộc bảo mật của API (201 không trả tên manager — không được hiển thị/hứa hẹn tên ở bước tạo).

## Architecture

- Feature folder mới `apps/web/src/features/oversight/` theo đúng bố cục các feature hiện có (đối chiếu `apps/web/src/features/profile/`): `api/`, `hooks/`, `components/`, `pages/`, `schemas/`, `__tests__/`, `routes.tsx`, `index.ts`.
- **Data layer**: TanStack Query. `useGrants` (GET `/oversight/grants`), `useCreateGrant`, `useRevokeGrant` — mutation xong invalidate query key `["oversight","grants"]`. Axios client dùng `@/lib/api` như các feature khác.
- **Routing**: `routes.tsx` export `oversightRoutes` (lazy như các feature khác), mount trong `apps/web/src/app/router.tsx` dưới `DashboardLayout` (path `/oversight`). Thêm nav item vào `apps/web/src/layouts/dashboard-layout.tsx` theo pattern nav hiện có.
- **Contract phản chiếu Phase 2** (schemas zod validate response):
  - `POST /oversight/grants` `{manager_phone}` → `201 {id, manager_phone, created_at}` — response chỉ echo phone caller nhập, **không có tên manager** (chống oracle dò danh bạ). UI hiển thị toast thành công với phone, kèm giải thích "tên manager sẽ hiện trong danh sách sau khi cấp".
  - `GET /oversight/grants` → `{given: [{id, manager: {id, full_name, phone}, created_at}], received: [{id, managed: {...}, created_at}]}`.
  - `DELETE /oversight/grants/:id` → `204`; `404` cho mọi case không-thuộc-mình/đã-thu-hồi → UI xử lý như "đã thu hồi", refetch, không hiện lỗi đỏ.
- **Phone validation**: schema zod feature-local mirror rule `vnphone` backend — theo pattern `phoneField` tại `apps/web/src/features/roster/schemas/roster-schemas.ts:12` (nhận `0xxxxxxxxx` và E.164). Backend là chốt chặn cuối; client chỉ chặn sớm cho UX.
- **Error mapping** (dùng `@/lib/forms/use-api-form-errors.ts` như các form hiện có): 404 → "Không tìm thấy tài khoản giáo viên với số này" (message chung — API cố ý không phân biệt không-tồn-tại/không-active); 409 → "Đã cấp quyền cho số này rồi"; 422 → "Không thể tự cấp quyền cho chính mình".

## UI Design

Trang `Quyền giám sát` (`/oversight`):

1. **Section "Manager của tôi"** (`given`): danh sách card/row — tên + phone manager, ngày cấp, nút Thu hồi (confirm dialog radix `AlertDialog` theo pattern `anonymize-student-dialog.tsx`). Empty state: giải thích ngắn cơ chế cấp quyền + CTA mở form cấp.
2. **Nút "Cấp quyền giám sát"**: dialog form 1 field SĐT (react-hook-form + zod resolver, pattern `contact-dialog.tsx`), submit → toast sonner.
3. **Section "Giáo viên tôi quản lý"** (`received`): tên + phone GV được quản, ngày nhận, nút Thu hồi (managed hoặc manager đều thu hồi được — API cho phép cả hai phía).
4. Loading skeleton + error state theo component shared hiện có.

## Related Code Files

- Create: `apps/web/src/features/oversight/{index.ts,routes.tsx}`
- Create: `apps/web/src/features/oversight/api/grants-api.ts`
- Create: `apps/web/src/features/oversight/hooks/use-grants.ts`
- Create: `apps/web/src/features/oversight/schemas/grant-schemas.ts`
- Create: `apps/web/src/features/oversight/pages/grants-page.tsx`
- Create: `apps/web/src/features/oversight/components/{create-grant-dialog.tsx,revoke-grant-dialog.tsx,grant-list.tsx}`
- Create: `apps/web/src/features/oversight/__tests__/` (page + dialogs + schemas, kèm test handlers theo pattern `features/profile/__tests__/zalo-handlers.ts`)
- Modify: `apps/web/src/app/router.tsx` (mount `oversightRoutes`)
- Modify: `apps/web/src/layouts/dashboard-layout.tsx` (nav item)

## Implementation Steps

1. Schemas zod (request + response shapes, `phoneField` mirror `vnphone`).
2. `grants-api.ts` + hooks TanStack Query (list/create/revoke + invalidation).
3. `grants-page.tsx` với hai section + empty/loading/error states.
4. `create-grant-dialog.tsx` (form + error mapping 404/409/422) và `revoke-grant-dialog.tsx` (confirm, 404 → coi như đã thu hồi).
5. Routes + nav; kiểm tra lazy chunk hoạt động.
6. Tests: render hai danh sách từ fixture; create happy path + từng nhánh lỗi; revoke happy + 404-idempotent; validation phone client.
7. `npm run lint && npm run typecheck && npm run test` trong `apps/web`.

## Success Criteria

- [ ] GV cấp grant bằng SĐT; danh sách `given` cập nhật không cần reload
- [ ] Thu hồi từ cả hai section; revoke lần 2 (404) không hiện lỗi — trạng thái hội tụ về "đã thu hồi"
- [ ] Không chỗ nào trong flow tạo grant hiển thị tên manager trước khi grant tồn tại
- [ ] Ba nhánh lỗi 404/409/422 hiện message tiếng Việt đúng ngữ nghĩa
- [ ] Lint + typecheck + vitest pass; route lazy-load đúng pattern

## Risk Assessment

- **UI lộ thông tin hơn API cho phép**: rủi ro chính là copy/hiển thị "hứa" tên manager ở bước tạo — chốt bằng success criterion và test assert nội dung toast.
- **Lệch contract khi Phase 2 đổi DTO**: schemas zod validate response lúc runtime → fail sớm và rõ thay vì render sai; sửa schema là một chỗ.
- **Drift validation phone client/server**: client chỉ là UX, backend `vnphone` là chốt chặn; test giữ một fixture số hợp lệ/không hợp lệ đồng bộ với backend test.
