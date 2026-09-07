---
phase: 3
title: "RecordsClassSelect: popover listbox + bottom sheet"
status: completed
priority: P1
effort: "6h"
dependencies: [2]
---

# Phase 3: `RecordsClassSelect` — popover listbox + bottom sheet

## Overview

Dựng dropdown lớp đúng mockup A (ghi chú 2, 3, 8): trigger 44px bo 14px hiện `Tên lớp · N HS`, popover trắng viền line-200 bo 16px với ô "Tìm lớp…" khi >5 lớp, nhóm "LỚP ĐANG DẠY", mục 42px có ✓ mint; dưới `sm` cùng list body mở trong `HvModal` bottom sheet. Component mới, độc lập trang, có test riêng. `ClassSelect` của classbook không đụng (D4).

## Requirements

- Functional:
  - Props: `classes: Class[]`, `selectedId: string`, `onSelect(classId)`, `labelId` (id của nhãn "LỚP" để `aria-labelledby`), `id?`.
  - Trigger: `button type="button" aria-haspopup="listbox" aria-expanded aria-controls={listboxId} aria-labelledby="{labelId} {triggerTextId}"` → tên truy cập "Lớp Toán 6A · 28 HS". Nội dung: tên (14.5px/800 ink-900) + `<span>· {n} HS</span>` (12.5px/700 ink-400, `ml-0.5`), icon `ChevronDown` 16px ink-400 xoay 180° khi mở (`transition-transform`, `motion-reduce:transition-none`).
  - Trigger style: `inline-flex min-h-11 min-w-[230px] items-center justify-between gap-2.5 rounded-[14px] border-2 border-line-200 bg-white pl-3.5 pr-2.5 text-[14.5px] font-extrabold text-ink-900 hover:border-mint-300 data-[state=open]:border-mint-400 focus-visible:ring-4 focus-visible:outline-none`; `max-sm:w-full` (mockup `.phone .sel-trig{width:100%}`).
  - Popover (≥`sm`): `Popover.Content` từ `radix-ui` (`import { Popover as PopoverPrimitive } from "radix-ui"`, cùng cách `hv-modal.tsx` dùng `Dialog`), `align="start" sideOffset={6}`, class `z-50 w-max min-w-(--radix-popover-trigger-width) max-w-[320px] rounded-[16px] border-2 border-line-200 bg-white p-1.5 shadow-soft-lg origin-(--radix-popover-content-transform-origin) data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 motion-reduce:animate-none`.
  - Bottom sheet (<`sm`): `HvModal open onOpenChange title="Chọn lớp" size="md"` chứa cùng `ClassOptionList`; đóng sau khi chọn.
  - `ClassOptionList` (nội bộ file):
    - Ô lọc chỉ khi `classes.length > 5` (dùng `useClassSearch(classes)` — giữ ngưỡng và cách lọc): wrapper `mx-0.5 mb-1.5 mt-0.5 flex min-h-[38px] items-center gap-2 rounded-[12px] border-2 border-line-200 bg-cream-50 px-2.5`, icon `Search` 14px ink-400, `input type="search" aria-label="Tìm lớp" placeholder="Tìm lớp…"` `w-full bg-transparent text-[13.5px] font-bold text-ink-700 placeholder:text-ink-400 outline-none` + ẩn nút xoá native (`[&::-webkit-search-cancel-button]:appearance-none`).
    - Nhãn nhóm `div` "Lớp đang dạy": `px-3 pt-1.5 pb-0.5 text-[11px] font-extrabold uppercase tracking-[0.4px] text-ink-400` (không phải heading, `aria-hidden` không cần).
    - `div role="listbox" aria-label="Chọn lớp" id={listboxId}` chứa `button role="option" aria-selected` mỗi lớp: `flex min-h-[42px] w-full items-center gap-2.5 rounded-[11px] px-3 text-left text-[14px] font-bold text-ink-700 hover:bg-cream-100 focus-visible:bg-cream-100 focus-visible:outline-none aria-selected:bg-mint-50 aria-selected:text-mint-700`; icon `Check` 16px `text-mint-600` với `invisible` khi không chọn (giữ chỗ); `<span className="flex-1">{name}</span>`; `<span className="text-[12px] font-bold text-ink-400">{n} HS</span>` (bỏ nếu D1 = frontend-only).
    - Không khớp → `ClassSearchEmptyNote` (đã có, 13px ink-400 bold) trong `px-3 py-2.5`.
  - Bàn phím: mở → focus ô lọc (nếu có) hoặc option đang chọn; trong listbox ↑↓ di chuyển focus giữa option (wrap), Home/End, Enter/Space chọn, Esc đóng (Popover/Dialog xử lý); ↓ từ ô lọc → option đầu; gõ chữ khi đang ở option → chuyển focus về ô lọc (nếu có).
  - Chọn → `onSelect(id)` chỉ khi khác `selectedId`, đóng, focus về trigger.
- Non-functional: không thêm dependency; test không cần CSS; a11y theo APG listbox.

## Architecture

```
RecordsClassSelect
├── useMediaQuery("(min-width: 640px)")  ← lib/hooks/use-media-query.ts (useSyncExternalStore + matchMedia)
├── isSmUp ? Popover.Root/Trigger/Portal/Content : (trigger button + HvModal)
└── ClassOptionList { classes, selectedId, onPick, autoFocus }
      └── useClassSearch(classes) → query/filtered/showSearch/emptyNote
```

`useMediaQuery` trả `false` trong Vitest (mock `matchMedia` ở `test/setup.ts` cho `matches:false`) → **mặc định test chạy nhánh bottom sheet**; test popover ghi đè `window.matchMedia` trả `matches: true` cho query `min-width: 640px`. Ghi rõ điều này trong test helper `mockViewport(width)` để Phase 5/6 tái dùng.

## Related Code Files

- Create: `apps/web/src/lib/hooks/use-media-query.ts` (+ export nếu `lib/hooks` có barrel; hiện chỉ có `use-no-index.ts`, import trực tiếp theo đường dẫn)
- Create: `apps/web/src/lib/hooks/__tests__/use-media-query.test.ts`
- Create: `apps/web/src/features/teaching/components/records-class-select.tsx`
- Create: `apps/web/src/features/teaching/__tests__/records-class-select.test.tsx`
- Create: `apps/web/src/test/viewport.ts` (helper `mockViewport(width: number)` đặt `matchMedia` theo `min-width`/`max-width` trong query)
- Read only: `apps/web/src/components/hv/hv-modal.tsx`, `components/ui/select.tsx` (mượn class animation), `features/roster/hooks/use-class-search.ts`, `features/roster/components/class-search.tsx`, `features/teaching/components/class-select.tsx` (không sửa)

## Implementation Steps

1. `use-media-query.ts`: `useMediaQuery(query: string): boolean` với `useSyncExternalStore(subscribe(matchMedia.addEventListener("change")), getSnapshot = matchMedia(query).matches)`; guard `typeof window === "undefined"` → false.
2. `test/viewport.ts`: `mockViewport(width)` — parse `(min-width: Npx)` / `(max-width: Npx)` để trả `matches` đúng, gọi listener khi đổi; dùng `vi.stubGlobal` hoặc gán `window.matchMedia`.
3. Viết `records-class-select.tsx` theo Requirements; tách `ClassOptionList` + `useListboxKeys` nhỏ trong cùng file (rule `react-refresh/only-export-components`: chỉ export component `RecordsClassSelect`).
4. Test:
   - render tên + `· 28 HS` trên trigger; `aria-expanded` đổi khi mở; `role="listbox"` xuất hiện; option đang chọn `aria-selected=true`.
   - ≤5 lớp không có `Tìm lớp`; 7 lớp có; gõ "toán" lọc; gõ "zzz" hiện `Không có lớp nào khớp "zzz"`.
   - click option gọi `onSelect(id)` một lần, đóng; click option đang chọn không gọi.
   - ↓↓ Enter chọn lớp thứ 3; Esc đóng và focus về trigger.
   - `mockViewport(375)` → mở ra `role="dialog"` có tiêu đề "Chọn lớp"; `mockViewport(1280)` → không có dialog.
5. `npm run lint && npm run typecheck && npx vitest run src/features/teaching src/lib`.

## Todo

- [x] `useMediaQuery` + test
- [x] `mockViewport` helper
- [x] `RecordsClassSelect` (trigger, popover, sheet, list body, phím)
- [x] Test component xanh, lint/typecheck xanh

## Success Criteria

- [x] Trigger/popover/mục khớp mockup theo từng giá trị: 44/230/14/16/42/11px, line-200→mint-300/400, mint-50/700, ✓ mint-600, `N HS` 12px ink-400.
- [x] `classes.length > 5` ⇔ có ô "Tìm lớp…"; note không khớp đúng chuỗi của `useClassSearch`.
- [x] Bàn phím ↑↓ Home End Enter Space Esc hoạt động; focus trả về trigger sau khi chọn/đóng.
- [x] <640px mở bottom sheet `HvModal`; ≥640px mở popover.
- [x] Không file nào ngoài danh sách trên bị sửa; `class-select.tsx` diff rỗng.

## Risk Assessment

- **Popover `min-w-(--radix-popover-trigger-width)`** cần Radix Popover đặt CSS var — có sẵn trong `radix-ui` Popover (`--radix-popover-trigger-width`). Tín hiệu vỡ: popover hẹp hơn trigger → dùng `style={{ minWidth: triggerRef.current?.offsetWidth }}`.
- **Focus trap trong HvModal** + autofocus ô lọc: Dialog tự focus phần tử đầu tiên; đặt `autoFocus` trên ô lọc/option đang chọn qua `onOpenAutoFocus` để không nhảy vào nút đóng.
- **`aria-labelledby` nối 2 id**: nếu Testing Library tính tên khác kỳ vọng, đổi sang `aria-label={\`Lớp — đang xem ${name}\`}` như classbook (không ảnh hưởng thị giác).

## Kết quả

Commit `bff2b68`. Lệch ghi nhận: thêm prop tuỳ chọn `onCloseAutoFocus` vào `HvModal` để bottom sheet trả focus về trigger (Dialog modal không có `Dialog.Trigger` nên `triggerRef` null); Radix `Popover.Content` mang `role="dialog"` nên test popover chỉ khẳng định không có dialog tên "Chọn lớp". Kích thước/token kiểm bằng class Tailwind, không đo pixel. Sau review (commit `f1d3515`): Space không còn bị đẩy vào ô lọc khi >5 lớp nên option kích hoạt native ở mọi kích thước danh sách.
