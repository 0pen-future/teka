---
phase: 3
title: "Web — bảng điểm danh 4 cột (phương án A)"
status: completed
priority: P1
effort: "1.5d"
dependencies: [1]
---

# Phase 3: Web — bảng điểm danh 4 cột (phương án A)

## Overview

Dựng lại màn điểm danh theo artboard "A · Bảng điểm danh": table với cột
HỌC SINH + 4 cột trạng thái có tiêu đề màu (Đúng giờ mint · Muộn sun ·
Vắng coral · Lý do sky), mỗi ô là nút tròn icon 1 chạm, mặc định cả lớp Đúng
giờ. Giữ toàn bộ hành vi hiện có: role gating, unsaved-changes blocker, các
state đã xác nhận / đã huỷ / kỳ chốt sổ.

## Requirements

- Functional:
  - [x] State chuyển từ `Set<string>` (absent IDs) sang `Map<studentId, {status, note?}>`; chỉ lưu exception (default present không cần entry).
  - [x] Mỗi hàng: 4 nút tròn thẳng cột; ô đang chọn tô đặc màu trạng thái + bóng nhấn (`box-shadow 0 3px 0` như hv-button); nền hàng nhuộm nhạt theo trạng thái (sun-50/coral-50/sky-50 tương ứng, present giữ trắng).
  - [x] Chip đếm phía trên bảng: "Đúng giờ n · Muộn n · Vắng n · Có lý do n" (ẩn chip = 0, trừ Đúng giờ).
  - [x] Chọn "Có lý do" mở nhập note nhanh (inline hoặc HvModal) — hiển thị subtitle dưới tên ("Vắng có phép — mẹ báo ốm"); note optional.
  - [x] Confirm bar: "XÁC NHẬN · n VẮNG · n MUỘN" (chỉ hiện phần ≠ 0; mặc định "XÁC NHẬN"); giữ các biến thể ĐÃ XÁC NHẬN ✓ / LƯU VÀ TẠO ĐIỀU CHỈNH.
  - [x] Gửi `marks` theo contract Phase 1; buổi đã confirm load lại đúng 4 trạng thái vào Map để sửa.
- Non-functional:
  - [x] A11y: mỗi hàng là `role="radiogroup"` (aria-label = tên học sinh), mỗi ô `role="radio"` + `aria-checked`; touch target ≥ 44px; hợp lệ với axe lint hiện có.
  - [x] Mobile 390px không tràn ngang: grid `minmax(0,1fr) 44px×4`, tên học sinh truncate.

## Architecture

- Giữ pattern one-touch + single POST hiện tại; không per-row network call.
- Grid CSS (không `<table>`) như artboard: header row sticky nếu lớp dài.
- Đổi zod: `AttendanceRow.status` thêm `"late"`; `ConfirmAttendanceInput` →
  `{marks: [...], note?}`; cập nhật `attendance-api.ts` và
  `use-sessions.ts` (giữ nguyên invalidation keys, thêm invalidation cho
  session list vì summary counts đổi).
- Dirty check của useBlocker so sánh Map hiện tại với baseline từ server.
- Màu lấy từ tokens có sẵn: mint-400 `#5cc9a7`, sun-400 `#ffc83d`,
  coral-400 `#ff7a66`, sky-300 `#7fc8e8` — không thêm token mới nếu các sắc
  nhạt (50/100) đã tồn tại trong `colors.css`; nếu thiếu, thêm vào tokens chứ
  không hardcode hex trong component.

## Related Code Files

- Modify: `apps/web/src/features/attendance/pages/attendance-page.tsx`
- Rewrite: `apps/web/src/features/attendance/components/attendance-row.tsx` → grid row 4 nút (cân nhắc đổi tên `attendance-status-row.tsx` nếu props đổi hẳn)
- Create: `apps/web/src/features/attendance/components/attendance-table-header.tsx` (header 4 cột màu)
- Create: `apps/web/src/features/attendance/components/status-count-chips.tsx`
- Modify: `apps/web/src/features/attendance/components/confirm-attendance-bar.tsx`
- Modify: `apps/web/src/features/attendance/schemas/attendance-schemas.ts`, `api/attendance-api.ts`, `hooks/use-sessions.ts`
- Modify: `apps/web/src/features/attendance/__tests__/attendance-page.test.tsx` + MSW handlers
- Modify (nếu thiếu sắc nhạt): `apps/web/src/styles/tokens/colors.css`

## Implementation Steps

1. Cập nhật schemas + api layer + MSW mocks theo contract mới.
2. Refactor state attendance-page sang Map; load baseline từ GET (status 4 giá trị).
3. Dựng header 4 cột + row grid + chip đếm; wiring chọn trạng thái (chạm ô đã chọn ≠ present → quay về present).
4. Note flow cho excused (và cho absent nếu người dùng muốn ghi lý do sau).
5. Confirm bar động; giữ role gating, blocker, closed-period.
6. Vitest: default all-present 1 chạm confirm; chọn từng trạng thái; reload buổi đã confirm; a11y roles; dirty blocker; closed-period.

## Success Criteria

- [x] `npm run typecheck` + `npm run test` xanh.
- [x] Khớp artboard A trên 390px và hàng hiển thị đúng màu/icon từng trạng thái.
- [x] Buổi bình thường (không ai vắng/muộn) vẫn chỉ 1 chạm Xác nhận.

## Risk Assessment

- **Lớp đông (30+) trên mobile** — hàng 54px, header sticky; nếu cuộn nặng thì
  đơn giản hoá shadow, không virtualize (YAGNI ở quy mô lớp học).
- **Radio semantics vs nút toggle** — dùng radiogroup chuẩn để tránh tranh cãi
  a11y; test bằng axe lint có sẵn.
- **MSW/e2e lệch contract** — MSW handlers cập nhật cùng PR; e2e xử lý ở Phase 5.
