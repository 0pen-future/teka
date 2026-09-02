---
title: Classbook toolbar và modal Bảng điểm theo design system
date: 2026-09-02
summary: "Dropdown chọn lớp thay modal, HvSegmented thành nhóm nút có viền, modal Bảng điểm đồng bộ header band; review bắt class ink-600 không tồn tại"
---

# Classbook toolbar và modal Bảng điểm theo design system

## What happened

Ba lỗi UX người dùng báo trên `/classbook` (plan
`plans/260902-1639-GH-260902-classbook-toolbar-and-score-table-ds/plan.md`):

- Modal "Bảng điểm" lệch design system: bảng không đường kẻ, workspace cố định
  90dvh trống, ĐTB dùng dấu chấm, nút "Đóng" tô nền cạnh tranh với nút chính.
  Sửa trong `score-table-modal.tsx`: header band `bg-cream-200` 12px extrabold
  uppercase ink-500 (cùng chuỗi với roster table), hairline `border-line-100`
  đặt trên ô vì bảng dùng `border-separate`, "Đóng" thành ghost.
  `HvModal size="xl"` đổi từ `sm:h-[90dvh]` sang `sm:max-h-[90dvh]` (co theo nội dung).
- Chọn lớp mở popup modal. Viết lại `class-select.tsx` trên `ui/select`
  (Radix Select, `position="popper"`), bỏ ô tìm kiếm (typeahead của Radix đủ),
  trigger một dòng, mỗi option kèm lịch học. Guard điểm chưa lưu vẫn đi qua
  `requestNavigation`; Select controlled nên lựa chọn bị từ chối không dính lại.
- "Buổi học" / "Chương trình & giáo án" không giống nút. `HvSegmented` đổi từ
  dải nền nhạt sang nhóm nút viền `border-2 border-line-200 bg-white`, item
  active tô mint-400 chữ trắng, thêm icon. Hover chỉ áp cho item idle bằng
  `data-[state=inactive]` / `data-[state=unchecked]`, tránh xung đột thứ tự
  variant hover của Tailwind với nền active.

## Review findings và cách xử lý

- `text-ink-600` không tồn tại: bridge `@theme` trong `globals.css` chỉ map ink
  900/700/500/400/300. Tailwind v4 không sinh utility, item inactive rơi về
  ink-700 của body, ngược mục tiêu. Sửa thành `text-ink-500`. Hai chỗ dùng sẵn
  ngoài scope (`attendance-page.tsx:366`, `blocking-sessions-panel.tsx:39`)
  chưa sửa.
- `formatAverage` trùng từng ký tự với `formatLedgerScore`; giờ re-export.
- Thêm test nhánh "Ở lại" khi đổi lớp: trigger giữ "Toán 6A" và chọn lại lần
  hai vẫn bật guard.
- Test kit chỉ khẳng định chuỗi class literal, nên không bắt được token chết.
  jsdom không thay thế kiểm tra thị giác.

## Verification

`make lint-web` exit 0 (5 warning react-hooks có sẵn). `make test-web`: 78
file, 578 pass, 3 skip. Chưa xem trên trình duyệt thật dropdown popper với lớp
có 10+ mục (lần đầu repo dùng `position="popper"`).

## Decision

- ĐTB dùng dấu phẩy ở mọi nơi `formatAverage` được gọi (bảng + chip panel).
- Restyle `HvSegmented` áp cho cả hai variant, nên mode switch của score-set
  editor cũng đổi theo.

## Next steps

- Người dùng quyết định commit (chưa commit).
- Cân nhắc tách header band dùng chung (hiện lặp ở 4 bảng) và dọn hai
  `text-ink-600` còn lại.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
