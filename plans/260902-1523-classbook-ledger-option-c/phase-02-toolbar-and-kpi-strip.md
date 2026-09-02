---
phase: 2
title: "Toolbar, chọn lớp, month stepper, dải KPI"
status: pending
priority: P1
effort: "1d"
dependencies: [1]
---

# Phase 2: Toolbar, chọn lớp, month stepper, dải KPI

## Overview
Thay header + tablist pill + 5 thẻ KPI + tab gạch chân bằng toolbar Phương án C
(nút chọn lớp có tìm kiếm, month stepper, HvSegmented, CSV ghost icon) và dải
KPI 4 ô trên hairline.

## Requirements
- Functional:
  - `ClassSelect`: nút `min-h-11`, nền trắng, `shadow-soft-sm`, `rounded-[var(--radius-md)]`,
    nội dung: tên lớp (`font-display` 15px 800) + `<small>` "n HS · lịch"
    (`ink-500`, 12px; mobile chỉ "n HS") + `HvIcon chevron-down`. Bấm mở
    `HvModal size="sm"` tiêu đề "Chọn lớp" chứa `ClassSearchInput` (khi
    `showSearch`), danh sách nút lớp (tên + lịch), `ClassSearchEmptyNote`.
    Chọn → `onSelect(classId)` → đóng. `aria-haspopup="dialog"`, `aria-expanded`.
  - `MonthStepper`: `‹` `Tháng M/YYYY` `›`; nút ghost icon `aria-label="Tháng trước"`
    / `"Tháng sau"` (thêm icon `chevron-left`/`chevron-right` vào `hv-icon.tsx`);
    nhãn `font-display` 15px 800, `tabular-nums`; `onChange(month)`.
  - View switch: `HvSegmented variant="tabs" idBase="classbook-view"`
    options `sessions: "Buổi học"`, `course: "Chương trình & giáo án"`;
    panel tương ứng có `id="classbook-view-panel-{value}"` `role="tabpanel"`.
  - CSV: `HvButton variant="ghost" size="sm"` chỉ icon `file`,
    `aria-label="Tải dữ liệu lớp (CSV)"` (giữ tên a11y để test cũ dùng lại),
    disabled khi không có lớp.
  - `ClassKpiStrip`: `grid grid-cols-2 sm:grid-cols-4` (spec: 4 auto cột gap
    32px, mobile 2×2), `border-b-[1.5px] border-line-200`, `py-3 px-1`,
    mỗi ô: nhãn 11.5px 800 `ink-400` tracking .3px, số `font-display` 20px 800
    `tabular-nums`, `<small>` sub 12px `ink-500` (ẩn sub ở mobile theo mock
    390). Items: SĨ SỐ (n, "tái tục x%"), CHUYÊN CẦN (x%, "a/b lượt"),
    ĐIỂM TB (n.n hoặc "—", "k buổi"), LÃI/LỖ Tm (vnd, "thu … · chi …"; giá
    trị âm `text-coral-600`).
  - URL: `class_id` + `month` cùng nằm trong `useSearchParams`, ghi bằng
    `replace: true`, giữ param còn lại khi đổi một param.
- Non-functional: mọi nút ≥ 44px, focus ring hv, không hard-code màu ngoài token.

## Architecture
- `classbook-page.tsx` giữ state URL: `classId = searchParams.get("class_id") ?? firstActive`,
  `month = parseMonthParam(searchParams.get("month"))`; `setParams(patch)`
  gộp cả hai. `useClassMarks(classId, month)`, `useMonthSessions(classId, month)`.
- KPI nhận `ClassbookTotals` + `activeHeadcount` + `retentionStat` (có sẵn)
  và `monthNumber` cho nhãn "LÃI/LỖ T{m}".
- Đổi lớp/tháng/view đi qua `requestNavigation` (Phase 4) để guard; ở phase
  này gọi thẳng, Phase 4 nối guard.

## Related Code Files
- Create: `apps/web/src/features/teaching/components/class-select.tsx`
- Create: `apps/web/src/features/teaching/components/month-stepper.tsx`
- Create: `apps/web/src/features/teaching/components/class-kpi-strip.tsx`
- Modify: `apps/web/src/components/hv/hv-icon.tsx` (thêm `chevron-left`, `chevron-right`)
- Modify: `apps/web/src/features/teaching/pages/classbook-page.tsx`
- Delete: `apps/web/src/features/teaching/components/class-stat-cards.tsx`
- Read: `apps/web/src/features/roster/components/class-search.tsx`,
  `apps/web/src/features/roster/hooks/use-class-search.ts`,
  `apps/web/src/features/roster/lib/roster-format.ts` (`formatScheduleSummary`),
  `apps/web/src/components/hv/hv-modal.tsx`, `hv-button.tsx`.

## Implementation Steps
1. Thêm 2 icon vào registry `hv-icon.tsx` (Lucide `ChevronLeft`, `ChevronRight`).
2. Viết `month-stepper.tsx` (props `month`, `onChange`, `disabled?`).
3. Viết `class-select.tsx` (props `classes`, `selectedId`, `headcount`,
   `today`, `onSelect`); dùng `useClassSearch(classes)`; danh sách là
   `ul > li > button` với `aria-current` cho lớp đang chọn.
4. Viết `class-kpi-strip.tsx` (props `totals`, `headcount`, `retention`,
   `monthNumber`); nhãn tháng và format `vnd` như `ClassStatCards` cũ.
5. Sửa `classbook-page.tsx`: toolbar `flex flex-wrap items-center gap-2.5`
   (h1 26px display → giữ `text-[26px]`, spacer `flex-1`, stepper,
   segmented, CSV); thay `ClassStatCards` bằng `ClassKpiStrip`; bỏ tablist
   pill lớp và tab gạch chân; xóa `class-stat-cards.tsx`.
6. Cập nhật `exportCsv` dùng `month.label` từ `monthWindow`.

## Success Criteria
- [x] Đổi tháng bằng stepper cập nhật URL `?month=` và bảng/KPI tải lại theo tháng.
- [x] Chọn lớp trong modal đổi `class_id`, giữ `month`.
- [x] KPI đúng 4 ô, sub đúng nội dung, lãi âm màu coral.
- [x] `typecheck` xanh; test classbook-page tạm cập nhật `statCard` → truy vấn nhãn KPI.

## Risk Assessment
- `HvModal` che page trong a11y tree (test lớp guard đang dựa vào
  `hidden: true`) → viết lại test ở Phase 5 theo modal chọn lớp.
- Tên lớp dài làm toolbar wrap → `flex-wrap` + `max-w` trên nút, `truncate`.
