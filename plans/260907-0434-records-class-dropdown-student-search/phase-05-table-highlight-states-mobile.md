---
phase: 5
title: "Bảng: tô đậm, trạng thái rỗng/tải, hàng mobile"
status: todo
priority: P1
effort: "4h"
dependencies: [1, 4]
---

# Phase 5: Bảng — tô đậm, trạng thái rỗng/tải, hàng mobile

## Overview

Hoàn thiện `StudentRecordsTable` theo mockup A (ghi chú 5, 6, 9 và trạng thái "Đang tải"): bọc `<mark>` phần tên khớp, trạng thái rỗng-do-tìm bên trong thẻ bảng với nút "Xoá tìm kiếm", 5 hàng skeleton shimmer thay dòng chữ tải, và bố cục hàng 2 dòng khi `compact`. Desktop 6 cột, màu điểm/xu hướng, nút "Xem hồ sơ" giữ nguyên.

## Requirements

- Functional:
  - Props mới (tuỳ chọn, giữ tương thích): `query?: string`, `onClearSearch?: () => void`, `loading?: boolean`, `loadingLabel?: string` (ví dụ `Đang tải dữ liệu tháng 8…`), `compact?: boolean`.
  - Tô đậm: tên render qua `HighlightedName({ name, query })` dùng `findFoldedMatch`; phần khớp `<mark className="rounded-[4px] bg-sun-200 px-px text-ink-900">`; không khớp/`query` rỗng → text thường.
  - Rỗng do tìm (`rows.length === 0 && query`): vẫn có header (desktop) rồi khối `px-5 py-[30px] text-center text-[13.5px] text-ink-400` gồm `<b className="mb-1.5 block font-display text-[15px] text-ink-700">Không tìm thấy học sinh nào khớp “{query}”</b>`, dòng `Thử gõ ít ký tự hơn hoặc kiểm tra lại họ tên.`, nút ghost `mt-2.5` (cùng class nút CSV, `min-h-11`) icon `X` 14px + "Xoá tìm kiếm" → `onClearSearch`. Trạng thái rỗng của lớp không có học sinh (`Lớp chưa có học sinh đang học.`) vẫn do trang render như cũ khi `rows.length === 0 && !query`.
  - Tải (`loading`): header (desktop) + container `max-h-[520px]` với 5 hàng cùng grid; mỗi ô là `div` `h-3.5 rounded-[6px] bg-[linear-gradient(90deg,var(--color-cream-200),var(--color-cream-300),var(--color-cream-200))] bg-[length:200%_100%] animate-[hv-shimmer_1.2s_linear_infinite] motion-reduce:animate-none` với width `120+i*17`, 30, 34, 70, 24px (ô cuối trống), `aria-hidden`; kèm `<p role="status" className="sr-only">{loadingLabel}</p>`. Keyframe `hv-shimmer { to { background-position: -200% 0 } }` thêm vào `components/hv/hv-animations.css`.
  - `compact` (<768): không render header; container `max-h-[520px]` giữ; hàng `grid grid-cols-[1fr_auto] gap-x-2 gap-y-0.5 px-3.5 py-2.5 border-b border-line-100 hover:bg-cream-100`: ô 1 (col 1, row 1) tên (14px/800 ink-900, có mark); ô 2 (col 1, row 2) dòng phụ `text-[12px] text-ink-500`: `TB <b className={cn("font-extrabold", averageColor(row))}>{avg}</b> · {arrow} {trend.label} · vắng {absences}`; nút (col 2, row 1–2, `self-center justify-self-end`) văn bản "Xem", **`aria-label="Xem hồ sơ"`** để tên truy cập không đổi; cùng class nút "Xem hồ sơ" hiện tại.
  - Desktop: markup hiện tại giữ nguyên từng class; chỉ thay text tên bằng `HighlightedName`.
- Non-functional: nhánh compact/desktop chọn bằng prop (JS), không bằng class ẩn/hiện, để jsdom không có nội dung trùng (giữ `within(row).getByText("8.8")` của test cũ).

## Architecture

```
StudentRecordsTable { rows, onOpen, query, onClearSearch, loading, loadingLabel, compact }
├── loading  → Header? + SkeletonRows(5)
├── rows=[] && query → Header? + SearchEmptyState
└── rows     → Header? + rows.map(compact ? CompactRow : DesktopRow)
HighlightedName(name, query) → findFoldedMatch → [before, <mark>, after]
```

Trang (`records-page.tsx`): nhánh `sessionsPending && selectedClassId` render `<StudentRecordsTable loading loadingLabel=… rows={[]} …/>` thay `<p>`; nhánh `rows.length === 0` (chưa có học sinh) giữ khối cũ; ngược lại render bảng với `filteredRows`, `query`, `onClearSearch={() => setQuery("")}`, `compact={!wide}`.

## Related Code Files

- Modify: `apps/web/src/features/teaching/components/student-records-table.tsx`
- Modify: `apps/web/src/components/hv/hv-animations.css` (keyframe `hv-shimmer`)
- Modify: `apps/web/src/features/teaching/pages/records-page.tsx` (nhánh loading/empty/table)
- Create: `apps/web/src/features/teaching/__tests__/student-records-table.test.tsx`
- Read only: `apps/web/src/styles/tokens/*.css` (xác nhận `--color-sun-200`, `--color-cream-300` tồn tại), `apps/web/src/components/ui/skeleton.tsx` (không dùng: mockup là shimmer gradient, không phải `animate-pulse`)

## Implementation Steps

1. Thêm keyframe `hv-shimmer` vào `hv-animations.css` (kiểm tra file đã được import toàn cục qua `styles/*.css`).
2. Tách `DesktopRow`, `CompactRow`, `SkeletonRows`, `SearchEmptyState`, `HighlightedName` nội bộ trong `student-records-table.tsx` (không export thêm — rule react-refresh).
3. Cập nhật props + nhánh render; giữ nguyên `gridClassName`, `averageColor`, `trendColor`.
4. Sửa trang theo mục Architecture.
5. Test bảng: mark bọc đúng "Nguyễn" khi query "nguyen" (kiểm tra `container.querySelector("mark").textContent`); rỗng có tiêu đề + nút gọi `onClearSearch`; loading có `role="status"` với nhãn và 5 hàng `aria-hidden`; compact: không có "NGÀY SINH", có "TB 8.8 · ↗ Tăng · vắng 0" và nút tên truy cập "Xem hồ sơ" hiển thị "Xem".
6. `npx vitest run src/features/teaching` (test cũ `records-pages.test.tsx` xanh, kể cả `within(row).getByText("0")` và `getByText("—")`).

## Todo

- [ ] Keyframe shimmer
- [ ] `student-records-table.tsx`: highlight, empty, loading, compact
- [ ] `records-page.tsx`: nhánh loading/empty/table + `onClearSearch`
- [ ] Test bảng + test cũ xanh

## Success Criteria

- [ ] Desktop: DOM/class của header và hàng không đổi ngoài `<mark>`; chụp màn hình 1280px trước/sau trùng nhau khi `q` rỗng.
- [ ] Rỗng do tìm: đúng 3 phần tử (tiêu đề Baloo 15px ink-700, gợi ý 13.5px ink-400, nút ghost) trong thẻ bảng; lớp rỗng vẫn dùng khối cũ.
- [ ] Loading: 5 hàng shimmer + status sr-only; `prefers-reduced-motion` tắt animation.
- [ ] Compact: hàng 2 dòng đúng nội dung `TB 8.6 · ↗ Tăng · vắng 0`, nút "Xem" (aria "Xem hồ sơ"), không tràn ngang ở 375px.

## Risk Assessment

- **`<mark>` mặc định của trình duyệt** (nền vàng, màu đen) đè token → class đã ghi đè `bg-sun-200 text-ink-900`; kiểm tra Firefox áp `color` đúng.
- **Test cũ `parentElement` của tên** dùng `screen.findByText("Nguyễn Văn An")` → phần tử tên vẫn là `div` chứa text thuần khi `query` rỗng (không bọc `span` thừa) để `.parentElement` vẫn là hàng.
- **`sessionsPending` dài** làm skeleton xuất hiện nháy khi đổi lớp → chấp nhận theo mockup; nếu phản hồi khó chịu, giữ dữ liệu cũ bằng `keepPreviousData` (đã có ở `useClassesList`, cân nhắc cho `useEnrollmentsList`) ở phase sau, không thuộc scope này.
