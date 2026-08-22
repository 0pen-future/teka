---
phase: 4
title: "Web friend picker"
status: completed
priority: P1
effort: "1d"
dependencies: [2]
---

# Phase 4: Web friend picker

## Overview

UI map contact ↔ bạn Zalo: card "Zalo" trong trang chi tiết contact, modal
picker chọn từ danh sách bạn bè (search theo tên), unmap được. Chạy song song
được với Phase 3 (chỉ phụ thuộc Phase 2).

## Requirements

- Functional:
  - `contact-detail-page` thêm card Zalo: chưa map → nút `Chọn bạn Zalo` mở
    picker; đã map → hiển thị `zalo_name` + nút `Bỏ liên kết` (confirm dialog
    dùng `confirm-dialog.tsx` sẵn có).
  - Picker modal (`HvModal`): fetch `GET /me/zalo/friends`, search client-side
    theo `display_name` (normalize dấu tiếng Việt như các search hiện có nếu đã
    có util), chọn → `PUT /contacts/:id/zalo-mapping`.
  - Teacher chưa link Zalo → card hiển thị trạng thái + link tới trang profile
    (`Kết nối Zalo trước`); expired → tương tự với copy re-scan.
  - `contacts-page` list: badge nhỏ đã-map (dot/icon + `zalo_name`), không đổi
    layout chính.
- Non-functional: copy tiếng Việt; TanStack Query cho mọi server state; MSW
  handlers + tests theo chuẩn feature hiện có; a11y (focus trap modal có sẵn
  từ HvModal).

## Architecture

- API slice: mở rộng `features/profile/api/zalo-api.ts` thêm `getZaloFriends`
  (schema mới trong `profile/schemas/zalo-schemas.ts`); mapping mutation đặt ở
  `features/roster/api` (contacts là domain roster) — mỗi feature giữ đúng
  domain, picker import qua public index của profile feature.
- Hooks: `useZaloFriends()` (staleTime ngắn, chỉ fetch khi modal mở),
  `useSetContactZaloMapping(contactId)`, `useClearContactZaloMapping(contactId)`
  → invalidate query contact detail + contacts list.
- Component mới: `roster/components/zalo-friend-picker.tsx` (modal, search,
  danh sách avatar + tên, chọn 1); tái dùng pattern `contact-picker.tsx` sẵn có.

## Related Code Files

- Create: `apps/web/src/features/roster/components/zalo-friend-picker.tsx` (+ test)
- Modify: `apps/web/src/features/roster/pages/contact-detail-page.tsx`,
  `contacts-page.tsx`, roster api/hooks/schemas + `__tests__` + MSW handlers
- Modify: `apps/web/src/features/profile/api/zalo-api.ts`,
  `schemas/zalo-schemas.ts`, `hooks/use-zalo.ts` (friends query), index exports

## Implementation Steps

1. Schemas + API: friends response, mapping request; MSW handlers.
2. Hooks với invalidation đúng key.
3. `zalo-friend-picker.tsx` (search + select + loading/error/empty states).
4. Gắn vào contact-detail card + badge contacts-page.
5. Tests: picker chọn/map/unmap, trạng thái chưa-link/expired, search filter.

## Success Criteria

- [x] Map một contact từ picker → card hiển thị tên bạn Zalo; unmap → về trạng
      thái chưa map; list badge cập nhật không cần reload.
- [x] Chưa link Zalo → card dẫn về profile, không gọi friends API.
- [x] `npm test`, `eslint`, `tsc -b --noEmit`, `prettier --check`, build xanh.

## Risk Assessment

- **Trùng tên bạn bè** ("Mẹ Bảo" x2): hiển thị avatar + tên; chấp nhận teacher
  tự phân biệt — không xây disambiguation thêm (ghi nhận, KISS).
- **Friends API chậm/Zalo down:** query chỉ chạy khi mở modal, có error state
  retry; không block trang contact.

## Carry-over từ review phase 2 (mapping backend)

- Contact responses dùng `omitempty` cho `zalo_user_id`/`zalo_name`: khi chưa
  map, key biến mất khỏi JSON (không phải `null`) → zod schema phía web phải
  khai `.optional()`, không phải `.nullable()`.
- Friends API là cú gọi Zalo live mỗi request — đặt `staleTime` cho query
  friends để mở/đóng modal nhiều lần không bắn request liên tiếp từ tài khoản
  Zalo cá nhân.
- Backend đã fallback `display_name` ← tên profile khi bạn không có alias, nên
  picker không cần xử lý dòng tên rỗng.
