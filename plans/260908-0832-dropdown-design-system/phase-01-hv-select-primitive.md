---
phase: 1
title: "Primitive HvSelect trong hv kit"
status: completed
priority: P1
effort: "7h"
dependencies: []
---

# Phase 1: Primitive `HvSelect` trong hv kit

## Overview

Trích `RecordsClassSelect` (`apps/web/src/features/teaching/components/records-class-select.tsx`) thành primitive dùng chung `HvSelect` trong `components/hv/`, prop-driven, giữ nguyên 100% vỏ và hành vi của dropdown Lớp ở `/records`, bổ sung các trạng thái mà consumer khác cần (placeholder, disabled, aria-invalid, option disabled, cuộn khi dài). Phase này **additive**: chưa consumer nào đổi.

## Requirements

- Functional:
  - Chọn một giá trị `string`; `""` = chưa chọn → hiện `placeholder`.
  - Popover anchored dưới trigger từ `sm`; bottom sheet `HvModal` (size md, tiêu đề `sheetTitle`) dưới `sm`.
  - Ô lọc hiện khi `options.length > searchThreshold` (mặc định 5); lọc substring không phân biệt hoa thường trên `label`; note không khớp; gõ chữ từ option chuyển vào ô lọc (Space giữ lại cho option).
  - Roving focus ↑↓ Home End, bỏ qua option `disabled`; Enter/Space chọn (button native); Esc đóng, focus về trigger.
  - `onValueChange` gọi mọi lần chọn (D7); đóng sau khi chọn.
  - Tên truy cập theo D3 (ba cách, không trộn). `div[role=listbox]` có `aria-label={sheetTitle}` (D12), ô lọc `aria-label={\`Tìm ${searchNoun}\`}`.
  - Trigger hiện `label · meta`; option hiện `label` và `meta` ở hai span **không** separator (D4, giữ nguyên markup chuẩn). Cách đọc giá trị đang chọn: text của trigger + `data-value={value}` (D11).
  - Hoạt động **bên trong `HvModal`** ở cả hai nhánh: popover chọn được bằng click/bàn phím, dialog cha không đóng; sheet (nested dialog) — Esc chỉ đóng sheet, focus về trigger, form cha còn mở.
- Non-functional: chỉ token DS; `motion-reduce` tôn trọng; không dependency mới; **không import từ `@/features/*`** (hv kit là tầng dưới, `docs/frontend-guidelines.md`); test hv kit ≥ 14 case phủ cả hai breakpoint.

## Architecture

```ts
// apps/web/src/components/hv/hv-select.tsx
export interface HvSelectOption {
  value: string;
  label: string;
  /** Chữ phụ bên phải option và sau label trên trigger, ví dụ "28 HS", "Tối Thứ Ba". */
  meta?: string;
  disabled?: boolean;
}

export interface HvSelectProps {
  options: HvSelectOption[];
  value: string;
  onValueChange: (value: string) => void;
  /** Tiêu đề bottom sheet dưới `sm`, ví dụ "Chọn lớp". */
  sheetTitle: string;
  placeholder?: string;            // mặc định "Chọn…"
  groupLabel?: string;             // "Lớp đang dạy" — nhãn uppercase trên danh sách
  searchNoun?: string;             // "lớp" → "Tìm lớp…", `Không có lớp nào khớp "q"`; mặc định "mục"
  searchThreshold?: number;        // mặc định 5; Infinity = không bao giờ hiện ô lọc
  id?: string;                     // cho <label htmlFor>
  labelId?: string;                // aria-labelledby = `${labelId} ${triggerTextId}`
  "aria-label"?: string;
  "aria-invalid"?: boolean;
  disabled?: boolean;
  className?: string;              // ghi đè chiều rộng trigger (w-full, w-[180px]…)
  align?: "start" | "end";         // Popover align, mặc định "start"
}
```

Cấu trúc file (một file, không tách sớm):

1. `triggerClassName`, `optionClassName`, `contentClassName`, `searchBoxClassName`, `groupLabelClassName` — **sao chép nguyên chuỗi class** từ `records-class-select.tsx`, **trừ** hai token chiều rộng `min-w-[230px] max-sm:w-full` bị tách khỏi class cơ sở của trigger (D10: twMerge không gộp `min-w-*` với `w-*`, và `min-width` thắng `width` → consumer `w-[180px]` sẽ bị ép 230px). Records truyền lại hai token này qua `className` ở Phase 2. Cộng thêm:
   - trigger: `disabled:cursor-not-allowed disabled:bg-cream-200 disabled:text-ink-300 aria-invalid:border-coral-400 data-placeholder:font-bold data-placeholder:text-ink-400` (trạng thái lấy từ `components/ui/select.tsx` cũ, đã là token DS).
   - popover content: thêm `max-h-(--radix-popover-content-available-height) overflow-y-auto`.
   - sheet body: bọc `div.max-h-[60dvh].overflow-y-auto`.
   - option disabled: `aria-disabled:pointer-events-none aria-disabled:opacity-50`.
2. `useOptionSearch(options, threshold)` — port `useClassSearch` sang `HvSelectOption`, trả `{ query, setQuery, filtered, showSearch, emptyNote }`; note dùng `searchNoun`. Markup note không khớp (`<p>` với class hiện tại của `ClassSearchEmptyNote`) **viết inline** trong `hv-select.tsx` — không import `ClassSearchEmptyNote`/`useClassSearch` từ `@/features/roster`; hai file đó giữ nguyên cho `sessions-page`/`students-page`.
3. `OptionList` — port `ClassOptionList`: ô lọc, nhãn nhóm, `div[role=listbox aria-label={sheetTitle}]` với `button[role=option]`; option render `<span className="flex-1">{label}</span>` + (meta ? `<span className="text-[12px] font-bold text-ink-400">{meta}</span>`) — đúng markup chuẩn, không thêm ` · `; `focusOption` chỉ duyệt option **không** `aria-disabled`; option disabled render `disabled` + `aria-disabled` + `tabIndex=-1`.
4. `HvSelect` — port thân `RecordsClassSelect`: `useMediaQuery("(min-width: 640px)")`, nhánh sheet / nhánh Popover, `focusInitial`, `restoreTriggerFocus`, `pick`. Trigger:
   ```tsx
   <button
     type="button" id={id} role="combobox"
     aria-haspopup="listbox" aria-expanded={open}
     aria-controls={listboxId}
     data-value={value}
     aria-labelledby={labelId ? `${labelId} ${triggerTextId}` : undefined}
     aria-label={ariaLabel} aria-invalid={ariaInvalid || undefined}
     disabled={disabled} data-state={open ? "open" : "closed"}
     data-placeholder={selected ? undefined : ""}
     className={cn(triggerClassName, className)}
   >
     <span id={triggerTextId} className="min-w-0 truncate">
       {selected ? selected.label : placeholder}
       {selected?.meta ? <span className="ml-0.5 text-[12.5px] font-bold text-ink-400"> · {selected.meta}</span> : null}
     </span>
     <ChevronDown … rotate-180 khi open />
   </button>
   ```
   Khi `disabled`, `onClick` không mở (button native đã chặn); nhánh Popover đặt `open` chỉ qua `onOpenChange`.
5. Export `HvSelect`, `HvSelectOption`, `HvSelectProps` từ `components/hv/index.ts`.

Dòng dữ liệu: consumer giữ state/URL/form của mình → truyền `value` + `options` → `HvSelect` không giữ state ngoài `open` và `query`.

## Related Code Files

- Create: `apps/web/src/components/hv/hv-select.tsx`, `apps/web/src/components/hv/__tests__/hv-select.test.tsx`
- Modify: `apps/web/src/components/hv/index.ts`
- Read (nguồn port, chưa xoá ở phase này): `apps/web/src/features/teaching/components/records-class-select.tsx`, `apps/web/src/features/teaching/__tests__/records-class-select.test.tsx`, `apps/web/src/features/roster/components/class-search.tsx` (chỉ chép markup note, không import), `apps/web/src/components/ui/select.tsx` (chỉ lấy class trạng thái disabled/invalid), `apps/web/src/components/hv/hv-modal.tsx`, `apps/web/src/lib/hooks/use-media-query.ts`, `apps/web/src/test/viewport.ts`
- Modify **có điều kiện** (chỉ khi test nested ở bước 6 đỏ): `apps/web/src/components/hv/hv-modal.tsx` — thêm prop additive `onEscapeKeyDown?: (e: KeyboardEvent) => void` forward xuống `Dialog.Content` (HvModalProps hiện không có prop này và không spread rest).

## Implementation Steps

1. Tạo `hv-select.tsx` theo mục Architecture; port từng khối từ `records-class-select.tsx`, đổi `Class` → `HvSelectOption`, `klass.name` → `label`, `student_count` → `meta`.
2. Port `useClassSearch` → `useOptionSearch` (private trong file); thay chữ "lớp" bằng `searchNoun`.
3. Thêm 4 trạng thái mới (placeholder, disabled, aria-invalid, option disabled) và max-height.
4. Export từ `index.ts`.
5. Viết `__tests__/hv-select.test.tsx`: port 9 case của `records-class-select.test.tsx` (đổi `getByRole("button")` → `getByRole("combobox")`, dữ liệu → `HvSelectOption`; hai case đang assert `listbox` tên "Chọn lớp" (dòng 63, 171) đổi sang tên = `sheetTitle` truyền vào), thêm case: placeholder khi `value=""`; `disabled` không mở; `aria-invalid` phản ánh thuộc tính; option disabled bị bỏ qua khi ↑↓ và không chọn được; tên truy cập với `aria-label`; tên truy cập với `<label htmlFor>` (đúng bằng nhãn, không ghép giá trị); `searchThreshold=Infinity` không hiện ô lọc dù 7 mục; `emptyNote` dùng `searchNoun`; trigger hiện `label · meta` và `data-value`, option có textContent = `label` + `meta` nối liền (không ` · `); `aria-controls` trỏ đúng id listbox khi mở.
6. Thêm 2 case **nested trong `HvModal`** (render `<HvModal open title="Form"><HvSelect … /></HvModal>`): (a) `mockViewport(1024)` trong `beforeEach` — mở popover, click option → `onValueChange` gọi, `HvModal` vẫn `open`, focus về trigger; (b) viewport mặc định (sheet) — mở sheet → có 2 `dialog`, Esc → còn 1 `dialog` (form), focus về trigger, `onOpenChange` của form **không** bị gọi với `false`. Nếu (b) đỏ vì Esc đóng cả hai: **không** dùng `stopPropagation` (Radix lắng nghe Esc bằng capture listener trên `document`); thực hiện bước additive: thêm prop `onEscapeKeyDown` cho `HvModal`, sheet gọi `e.preventDefault()` sau khi tự đóng — ghi lại vào plan.md (Files + Risks) và bỏ `HvModal` khỏi "Không đụng".
7. Chạy `cd apps/web && npx vitest run src/components/hv` và `npm run lint && npm run typecheck`.

## Success Criteria

- [x] `HvSelect` render đúng markup: `button[role=combobox][aria-haspopup=listbox][aria-controls]` → `div[role=listbox][aria-label=sheetTitle]` → `button[role=option]`; test so **tập token class** (`className.split(/\s+/)` → Set) của trigger khi truyền `className="min-w-[230px] max-sm:w-full"`, và của option/content, **bằng** tập token trong `records-class-select.tsx` (không snapshot file, không so thứ tự chuỗi).
- [x] 19+ test hv-select xanh ở cả `mockViewport(1024)` (popover) và mặc định (sheet), gồm 2 case nested trong `HvModal`.
- [x] `grep -rn "@/features" apps/web/src/components/hv` = 0.
- [x] Nếu phải sửa `hv-modal.tsx`: chỉ thêm prop optional, test `hv-modal` hiện có xanh không sửa.
- [x] `records-class-select.tsx` chưa bị đụng; toàn bộ test hiện có vẫn xanh (`npm run test`).
- [x] lint + typecheck xanh.

## Risk Assessment

- **Port lệch chuẩn (class khác 1 token):** mitigations: test so sánh chuỗi class; Phase 5 chụp màn hình `/records` trước/sau. Tín hiệu vỡ: ảnh khác pixel ở trigger/option → sửa class, không sửa mockup.
- **`data-placeholder` không có variant Tailwind:** dùng `data-placeholder:` (Tailwind v4 hỗ trợ `data-*` tuỳ ý). Tín hiệu vỡ: build không sinh class → đổi sang `cn(!selected && "font-bold text-ink-400")`.
- **Option disabled + roving focus:** query selector `'[role="option"]:not([aria-disabled="true"])'`. Tín hiệu vỡ: ↑↓ dừng ở option disabled → sửa selector.
- **`aria-invalid` với `role=combobox` bị eslint jsx-a11y bắt:** combobox cho phép `aria-invalid`; nếu rule báo, truyền qua spread `{...a11y}` như chuẩn hiện tại.
- **Nested dialog (sheet trong `HvModal`) chưa có tiền lệ trong repo:** chứng minh bằng test ở bước 6 trước khi Phase 3/4 dùng. Tín hiệu vỡ: Esc đóng cả form, hoặc overlay sheet nằm dưới form (cả hai `z-50`, sheet mount sau nên đè lên — kiểm bằng thứ tự DOM trong test). Phản ứng đã định: prop additive `onEscapeKeyDown` cho `HvModal`; nếu overlay sai thứ tự → sheet dùng `Dialog.Portal` không `container` để luôn mount sau.
- **Popover trong `HvModal` (1024px):** Radix Select đã chạy ở record-payment/enroll, kỳ vọng đúng sẵn. Tín hiệu vỡ: case 6(a) đỏ → thêm `modal` cho `PopoverPrimitive.Root`; nếu vẫn vỡ, prop `inDialog` ép sheet ở mọi breakpoint — quyết định tại Phase 1, không đẩy sang consumer.
