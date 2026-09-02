---
phase: 3
title: "Bảng sổ lớp 8 cột với chip trạng thái"
status: pending
priority: P1
effort: "1d"
dependencies: [1]
---

# Phase 3: Bảng sổ lớp 8 cột với chip trạng thái

## Overview
Viết lại `sessions-table.tsx` thành bảng `<table>` đúng Phương án C: 8 cột,
chip trạng thái nhận xét/chấm điểm, hàng hủy/dự kiến theo ngôn ngữ trạng thái,
mini bar có mặt, ẩn cột trên mobile với cột VIỆC gộp, hàng chọn có nền mint và
sub-label "đang mở". Hàng mở rộng (Phase 4) chèn ngay sau hàng đang chọn.

## Requirements
- Functional:
  - Cột: BUỔI · BÀI HỌC · GIÁO ÁN · CÓ MẶT · ĐTB · DOANH THU · NHẬN XÉT · CHẤM ĐIỂM
    (+ VIỆC chỉ mobile). `th` 11.5px 800 `ink-400` tracking .3px, `border-b-[1.5px] line-200`,
    `whitespace-nowrap`; `td` `py-[9px] px-3 border-b line-100`, 13.5px.
  - Bảng: nền trắng, `rounded-[var(--radius-lg)]`, `shadow-soft-md`,
    `overflow-hidden` bọc trong `overflow-x-auto`; `min-w-[960px]` ở ≥640px.
  - Hàng held: BUỔI `font-extrabold ink-900` "Th 3, 01/09" (dùng
    `formatSessionDate`); BÀI HỌC "Bài n · {tiêu đề}" (tiêu đề từ
    `curriculum.lessons[lessonIndex]`, thiếu → "Bài n"); GIÁO ÁN `PlanStatusPill`;
    CÓ MẶT "13/14" `tabular-nums` + `ProgressBar` 64px inline (`ml-2`, màu mint;
    coral khi < 70%); ĐTB "7,6" (`toFixed(1)` với dấu phẩy) hoặc "—" khi
    `average === null`; DOANH THU `vnd(net)` (`text-coral-600` khi âm);
    NHẬN XÉT chip "Đã có" (mint) / "Chưa có" (cream/ink-400); CHẤM ĐIỂM chip
    "n/N" (mint khi n ≥ N và N > 0; sun khi 0 < n < N hoặc n = 0 với N > 0).
  - Hàng cancelled: ngày bình thường, BÀI HỌC = `cancel_reason` (hoặc "Nghỉ"),
    GIÁO ÁN pill "Buổi hủy" (coral, `StatusPill`/span cùng token), các cột còn
    lại "·" `text-ink-300`; nền ngày coral nhạt (`bg-coral-50` nếu token có,
    không thì bỏ, ghi nhận).
  - Hàng planned: BÀI HỌC "Bài n · tiêu đề", GIÁO ÁN `PlanStatusPill`
    (Chờ duyệt / Chưa soạn / Đã duyệt), CÓ MẶT "{sĩ số} dự kiến" `ink-500`,
    còn lại "·".
  - Hàng đang chọn: `bg-mint-50` toàn hàng, dưới ngày `<small>` "đang mở"
    (11.5px 700 `ink-400`), nút có `aria-expanded="true"` `aria-controls={expandRowId}`.
  - Mobile ≤639px (`sm:` breakpoint): ẩn GIÁO ÁN, ĐTB, DOANH THU, NHẬN XÉT,
    CHẤM ĐIỂM (`hidden sm:table-cell`); BUỔI hiện "dd/mm" + sub "Bài n" hoặc
    "Nghỉ"; cột VIỆC (`sm:hidden`) chip: "Xong" (mint) khi có note và đủ điểm,
    "Nhận xét" (sun) khi thiếu note, "Chấm điểm" (sun) khi có note nhưng thiếu
    điểm, "Hủy" (coral) cho cancelled, "Dự kiến" (cream) cho planned.
  - Mỗi hàng là `<tr>` có `onClick` → `onSelect(id)`; ô BUỔI chứa
    `<button type="button">` tên a11y = nhãn ngày (test dùng
    `getByRole("button", { name: /Th 4, 05\/08/ })`), `tabIndex` theo roving
    (hàng chọn = 0, còn lại -1; chưa chọn → hàng đầu = 0).
  - Bàn phím trên `tbody` `onKeyDown`: ArrowDown/ArrowUp focus nút hàng
    kế; Enter/Space trên nút toggle (`onSelect` hoặc `onClose`); Escape đóng.
  - Footer ghi chú doanh thu giữ nguyên câu hiện có ("Doanh thu buổi = học phí
    của học sinh có mặt − 300.000đ chi phí buổi"), đặt dưới bảng 12px `ink-400`.
- Non-functional: mọi số `tabular-nums`; không hard-code màu; test đơn vị cho
  hàm chip nằm ở Phase 1 (`sessionWorkStatus`), test render ở Phase 5.

## Architecture
- `sessions-table.tsx` props mới: `rows: SessionDerived[]`, `classId`,
  `curriculum`, `lessonPlans`, `notes: TeachingState["sessionNotes"]`,
  `scoredCounts: Record<string, number>`, `selectedId`, `onSelect`,
  `onClose`, `expandRowId`, `renderExpanded: (row) => ReactNode` (Phase 4
  truyền `SessionExpandRow`). Bảng tự chèn `<tr id={expandRowId}>` với
  `<td colSpan={9}>` ngay sau hàng chọn.
- Chip nhỏ dùng chung: `session-status-chip.tsx` (`tone: "mint" | "sun" | "coral" | "muted"`,
  `label`), 12px 800, `rounded-full px-2.5 py-[3px]`; giữ trong
  `features/teaching/components` (không lên hv theo non-goal).
- Nguồn tiêu đề bài: `useClassTeaching(classId).curriculum?.lessons`.

## Related Code Files
- Modify (rewrite): `apps/web/src/features/teaching/components/sessions-table.tsx`
- Create: `apps/web/src/features/teaching/components/session-status-chip.tsx`
- Read: `apps/web/src/components/hv/progress-bar.tsx`, `status-pill.tsx`,
  `apps/web/src/features/teaching/components/plan-status-pill.tsx`,
  `apps/web/src/lib/utils.ts` (`formatSessionDate`),
  `apps/web/src/features/teaching/components/student-sessions-table.tsx` (không sửa, chỉ tham khảo kiểu bảng).

## Implementation Steps
1. Viết `session-status-chip.tsx`.
2. Viết lại `sessions-table.tsx` theo props trên; tách `SessionRow`
   (held/cancelled/planned cùng một component, nhánh theo `session.status`).
3. Thêm hàng mở rộng placeholder (`renderExpanded`) và roving tabindex +
   keyboard handler.
4. Nối vào `classbook-page.tsx` (tạm `renderExpanded` trả `SessionDetailPanel`
   cũ để trang chạy được trước Phase 4).
5. Kiểm tra thủ công ở 1280 và 390 (Playwright screenshot tùy chọn) rằng
   không có cuộn ngang trang ở mobile.

## Success Criteria
- [x] Bảng render đúng 3 loại hàng theo fixture test (05/08 held, 08/08 hủy "Nghỉ lễ", 19/08 dự kiến "1 dự kiến").
- [x] Chip Nhận xét/Chấm điểm đổi ngay sau khi lưu (dữ liệu từ cache mutation).
- [x] Mobile không cuộn ngang; cột VIỆC hiện đúng nhãn.
- [x] Nút hàng có `aria-expanded`/`aria-controls`; ↑↓ Enter Esc hoạt động.

## Risk Assessment
- Click cả hàng vs nút bên trong gây double toggle → `stopPropagation` ở
  nút, hoặc chỉ gắn `onClick` ở `tr` và để nút không có handler riêng (chọn
  cách 2, nút vẫn nhận Enter/Space qua sự kiện click gốc của button).
- `PlanStatusPill` status "none" hiển thị "Chưa soạn"; kiểm tra label hiện tại
  trước khi test.
