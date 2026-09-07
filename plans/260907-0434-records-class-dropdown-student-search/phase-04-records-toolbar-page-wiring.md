---
phase: 4
title: "RecordsToolbar + nối vào trang"
status: completed
priority: P1
effort: "4h"
dependencies: [1, 3]
---

# Phase 4: `RecordsToolbar` + nối vào trang

## Overview

Thay khối pill (`records-page.tsx` dòng 118–142) bằng thanh bộ lọc trắng đúng mockup A (ghi chú 1, 4, 5, 7): trường LỚP với `RecordsClassSelect`, trường TÌM HỌC SINH với ô tìm (icon, kbd `/`, nút ×), bộ đếm live ở desktop; dưới `md` xếp dọc, bộ đếm + nút "CSV" chuyển xuống dưới bảng, nút CSV trên header ẩn. Header, CSV export, bảng (vẫn 6 cột) giữ nguyên.

## Requirements

- Functional:
  - `RecordsToolbar` props: `classes`, `selectedClassId`, `onSelectClass`, `query`, `onQueryChange`, `matched`, `total`, `searchRef` (để phím `/` focus), `compact` (true khi <768).
  - Khung: `flex flex-wrap items-end gap-3 rounded-[20px] bg-white px-4 py-3.5 shadow-soft-sm` (`compact` → `p-3`).
  - Field: `flex flex-col gap-[5px]`; nhãn `text-[11.5px] font-extrabold uppercase tracking-[0.35px] text-ink-400`. Nhãn "Lớp" là `<span id>` → `aria-labelledby` cho trigger; nhãn "Tìm học sinh" là `<label htmlFor>` ô tìm. Field lớp: `compact ? "w-full" : ""`. Field tìm: `flex-1 basis-[260px]` (`.field.grow`), ô tìm `w-full`.
  - Ô tìm (`.search`): `relative flex min-h-11 items-center gap-2 rounded-[14px] border-2 border-line-200 bg-white pl-3 pr-2 focus-within:border-mint-400 transition-colors`; icon `Search` 16px ink-400 (`shrink-0`); `input type="search" id aria-label="Tìm học sinh" placeholder="Gõ tên học sinh…" autoComplete="off" enterKeyHint="search"` class `min-w-0 flex-1 bg-transparent text-[14.5px] font-bold text-ink-900 placeholder:font-semibold placeholder:text-ink-400 outline-none focus:shadow-none [&::-webkit-search-cancel-button]:appearance-none`; lọc ngay `onChange` (không Enter).
  - Khi `query === ""`: `<kbd aria-hidden>` "/" `rounded-[6px] border-[1.5px] border-line-200 bg-cream-50 px-1.5 py-1 font-mono text-[11px] font-bold leading-none text-ink-400`. Khi có query: `button aria-label="Xoá tìm kiếm"` `inline-flex h-8 w-8 items-center justify-center rounded-[10px] bg-cream-200 text-ink-500 hover:bg-cream-300 hover:text-ink-900 focus-visible:ring-4 focus-visible:outline-none` + icon `X` 14px; bấm → `onQueryChange("")` và focus lại ô tìm.
  - Bộ đếm `<span role="status" aria-live="polite" class="whitespace-nowrap text-[13px] font-extrabold text-ink-500">`: query ? `<b class="text-ink-900">{matched}</b> / {total} học sinh` : `<b class="text-ink-900">{total}</b> học sinh`. Desktop: field thứ 3 trong toolbar với nhãn rỗng (`aria-hidden`, `&nbsp;`) và hộp `flex min-h-11 items-center`. Compact: **không** render trong toolbar; trang render dưới bảng (một bộ đếm duy nhất trong DOM).
  - Phím `/`: `useEffect` trong trang lắng nghe `keydown` trên `document`; bỏ qua khi `event.key !== "/"`, có modifier, hoặc `activeElement` là input/textarea/select/contenteditable; ngược lại `preventDefault()` + `searchRef.current?.focus()`.
  - Trang: `selectClass` (đã sửa ở Phase 1: `replace: true`, xoá `q` khi đổi lớp — D2) truyền vào `onChange` của `RecordsClassSelect`; ô tìm hiển thị trống ngay sau khi đổi lớp vì `query` đọc từ URL. Bảng nhận `filteredRows` (từ Phase 1) và `query`. Header CSV: `compact` → không render nút trên header; dưới bảng render hàng `flex items-center justify-between px-1` gồm bộ đếm + nút ghost "CSV" (`min-h-10`, cùng class nút CSV, icon `Download` 16px) gọi `exportCsv`. Nút CSV header đổi glyph "⬇" thành icon `Download` 16px (mockup dùng SVG; guideline không dùng emoji/glyph) — nhãn "Tải danh sách (CSV)" giữ nguyên.
  - Bỏ import `ClassSearchInput`, `ClassSearchEmptyNote`, `useClassSearch`, `cn` nếu không còn dùng trong trang (barrel roster giữ nguyên export).
- Non-functional: một `useMediaQuery("(min-width: 768px)")` ở trang → `compact = !wide` truyền xuống toolbar và bảng (Phase 5) để jsdom không render trùng nội dung.

## Architecture

```
RecordsPage
├── useSearchParams: class_id, q
├── wide = useMediaQuery("(min-width: 768px)")
├── header (h1, p, [CSV nếu wide])
├── <RecordsToolbar compact={!wide} …>
│     ├── Field "Lớp"  → <RecordsClassSelect>
│     ├── Field "Tìm học sinh" → SearchBox (icon, input, kbd | ×)
│     └── [Field đếm nếu wide]
├── loading / empty-class / <StudentRecordsTable rows={filteredRows} query …>
└── [hàng đếm + "CSV" nếu !wide]
```

## Related Code Files

- Create: `apps/web/src/features/teaching/components/records-toolbar.tsx`
- Create: `apps/web/src/features/teaching/__tests__/records-toolbar.test.tsx`
- Modify: `apps/web/src/features/teaching/pages/records-page.tsx`
- Read only: `apps/web/src/components/hv/hv-button.tsx` (không dùng — nút ghost giữ đúng class nút CSV hiện có để đồng nhất)

## Implementation Steps

1. Viết `records-toolbar.tsx` (component `RecordsToolbar`; `SearchBox`, `Field`, `ResultCount` là hàm nội bộ không export).
2. Sửa `records-page.tsx`: thêm `wide`, `searchRef`, effect phím `/`; thay khối pill; đổi `rows` → `filteredRows` cho bảng; thêm hàng đếm + CSV compact; CSV header icon.
3. Test toolbar: nhãn hiện "Lớp"/"Tìm học sinh"; kbd hiện khi rỗng, × hiện khi có query và gọi `onQueryChange("")`; bộ đếm "3 / 28 học sinh" / "28 học sinh"; `compact` không render bộ đếm.
4. Chạy `records-pages.test.tsx` với assertion **không sửa** → phải xanh. Lưu ý: mock `matchMedia` mặc định trả `matches:false` nên `wide=false` (compact); để 3 case cũ chạy ở nhánh desktop như trước, thêm `mockViewport(1280)` vào `beforeEach` của file test — chỉ bổ sung setup, không đổi assertion.
5. `npm run lint && npm run typecheck && npx vitest run src/features/teaching`.

## Todo

- [x] `RecordsToolbar` + test
- [x] `records-page.tsx` nối toolbar, `/`, compact CSV/đếm
- [x] `records-pages.test.tsx` thêm `mockViewport(1280)` trong `beforeEach`, test cũ xanh không đổi assertion
- [x] Lint/typecheck xanh

## Success Criteria

- [x] Không còn `role="tab"` trên `/records`; toolbar đúng token/kích thước mockup ở 1280px và 375px.
- [x] Gõ vào ô tìm → URL `?q=` cập nhật (replace), bảng lọc ngay; × xoá và focus lại; `/` focus ô tìm, không ăn phím khi đang gõ ở input khác.
- [x] Đổi lớp xoá `q` (ô tìm trống, bộ đếm `N / N`); `class_id` trên URL đổi, chỉ một entry history.
- [x] Ở compact: header không có nút CSV, dưới bảng có bộ đếm + nút "CSV" xuất cùng file; chỉ một `role="status"` trong DOM.
- [x] Test cũ trong `records-pages.test.tsx` xanh với assertion nguyên vẹn.

## Risk Assessment

- **Bộ đếm `role="status"` bên trong toolbar trước khi có dữ liệu** đọc "0 học sinh" gây nhiễu → chỉ đổi nội dung sau khi enrollments load (`total` = `rows.length` khi `!sessionsPending`), trước đó render rỗng.
- **`type="search"` hiện nút xoá native trên Safari/Chrome** trùng nút × → đã ẩn bằng `::-webkit-search-cancel-button`; kiểm tra Firefox (không có nút native).
- **Phím `/` xung đột** với ô lọc lớp trong popover: điều kiện bỏ qua khi `activeElement` là input đã bao trường hợp này.

## Kết quả

Commit `c6f89d7`. `ResultCount` export cùng file toolbar để trang dùng lại ở compact (react-refresh rule tắt). Lúc đang tải có 2 vùng `role=status` (bộ đếm rỗng + sr-only của skeleton); trạng thái đã tải chỉ còn một. Điều chỉnh ở Phase 6 (commit `6776e0a`): chọn lại đúng lớp đang chọn vẫn ghi `class_id` lên URL (giữ hành vi pill cũ) nhưng chỉ xoá `q` khi lớp thực sự đổi. Sau review (commit `f1d3515`): phím `/` nhường cho picker đang mở; bộ đếm và skeleton chờ cả query enrollments.
