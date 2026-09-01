---
phase: 4
title: "Web — bộ chọn buổi 3 thẻ + lịch tháng"
status: completed
priority: P1
effort: "1d"
dependencies: [2]
---

# Phase 4: Web — bộ chọn buổi 3 thẻ + lịch tháng

## Overview

Thay bộ lọc date-range hiện tại bằng mô hình 3 thẻ BUỔI TRƯỚC · HÔM NAY ·
KẾ TIẾP với mũi tên ‹ › lùi/tiến từng buổi (artboard "Chọn lớp + ngày" và
"Desktop hai cột"); "Mở lịch tháng" là modal lối tắt khi cần nhảy xa. Badge
trạng thái thẻ lấy từ `attendance_summary` (Phase 2).

## Requirements

- Functional:
  - [x] Giữ class pill tabs; chọn lớp xong hiện bộ chọn buổi ngay trên bảng điểm danh.
  - [x] 3 thẻ hiển thị buổi liền trước / buổi neo (mặc định = buổi hôm nay, hoặc buổi sắp tới gần nhất nếu hôm nay không có buổi) / buổi kế tiếp; mũi tên ‹ › dịch neo từng buổi theo dãy buổi của lớp.
  - [x] Trạng thái thẻ: "Chưa điểm danh" coral, "Đang xem/Đã điểm danh" mint, "Sắp tới" xám, "Đã huỷ" muted; badge đếm "n đúng giờ · n muộn · n vắng" khi đã confirm.
  - [x] Mobile: 3 thẻ ngang + mũi tên hai đầu, phía trên bảng. Desktop (lg+): thẻ dọc ở cột trái (giữ two-pane layout hiện có), mũi tên đầu cột.
  - [x] "Mở lịch tháng": modal month grid — ngày có buổi hiện dot màu theo trạng thái (coral = chưa điểm danh, mint = đã, xám = sắp tới, muted = huỷ); chạm ngày → neo về buổi đó; chuyển tháng ‹ ›.
  - [x] Giữ section "Cần điểm danh" (unconfirmed past) như một lối vào nhanh.
- Non-functional:
  - [x] Không thêm dependency lịch bên thứ ba — month grid tự dựng bằng tokens hiện có (Tailwind + date util thuần).
  - [x] Modal dùng `hv-modal`; keyboard navigation cho grid (arrow keys tối thiểu ở mức focusable buttons); axe lint pass.

## Architecture

- Data: `listClassSessions(classId, {from, to})` với cửa sổ quanh neo
  (ví dụ ±45 ngày) qua TanStack Query; prev/next tính client-side trên dãy đã
  sort. Modal lịch tháng query đúng tháng đang xem (from=đầu tháng,
  to=cuối tháng) — cache theo key `[classId, month]`.
- Neo (anchor session id) là state URL-friendly: giữ route
  `/sessions/:id/attendance` hiện có làm nguồn chân lý — 3 thẻ chỉ là
  navigation đổi `:id`, không thêm state store mới.
- Vì endpoint materialize buổi tương lai từ schedule, thẻ "KẾ TIẾP" luôn có dữ
  liệu khi lớp còn schedule hiệu lực; lớp không có buổi nào → empty state như
  hiện tại.

## Related Code Files

- Modify: `apps/web/src/features/attendance/pages/sessions-page.tsx`
- Create: `apps/web/src/features/attendance/components/session-trio-picker.tsx` (3 thẻ + mũi tên, biến thể ngang/dọc responsive)
- Create: `apps/web/src/features/attendance/components/month-calendar-modal.tsx`
- Modify: `apps/web/src/features/attendance/components/session-list-item.tsx` (badge summary) hoặc thay bằng thẻ mới
- Modify: `apps/web/src/features/attendance/schemas/attendance-schemas.ts` (thêm `attendance_summary`), `hooks/use-sessions.ts` (query window theo neo/tháng)
- Modify: `apps/web/src/features/attendance/__tests__/` + MSW handlers

## Implementation Steps

1. Thêm `attendance_summary` vào zod schema + MSW.
2. Dựng `session-trio-picker` (mobile ngang / desktop dọc qua breakpoint lg) + logic neo/prev/next.
3. Gắn vào sessions-page thay date-range input; giữ two-pane desktop + full-screen panel mobile.
4. Dựng `month-calendar-modal` (grid 7×6, dot trạng thái, chuyển tháng, chọn ngày→navigate).
5. Vitest: neo mặc định (hôm nay / sắp tới gần nhất), mũi tên ở biên (không có buổi trước → disable), badge màu theo trạng thái, modal chọn ngày điều hướng đúng.

## Success Criteria

- [x] `npm run typecheck` + `npm run test` xanh.
- [x] 390px và 1440px khớp bố cục artboard tương ứng; không tràn ngang.
- [x] Điểm danh buổi bất kỳ trong quá khứ/tương lai chỉ bằng ‹ › hoặc lịch tháng.

## Risk Assessment

- **Buổi thưa (lớp 1 buổi/tuần)** — cửa sổ ±45 ngày đủ chứa prev/next; nếu neo
  ở biên cửa sổ, nới from/to theo hướng đó (query key theo window).
- **Đổi hành vi lọc ngày** — date-range filter cũ bị thay; giữ "Cần điểm danh"
  để không mất lối vào các buổi quá hạn ngoài cửa sổ 3 thẻ.
- **Tự dựng month grid** — phạm vi nhỏ (hiển thị + chọn ngày), rẻ hơn và nhất
  quán design system hơn so với thêm thư viện lịch.
