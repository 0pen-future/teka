---
phase: 2
title: "Teaching: records + classbook dùng HvSelect"
status: completed
priority: P1
effort: "4h"
dependencies: [1]
---

# Phase 2: Teaching — records + classbook dùng `HvSelect`

## Overview

Thay `RecordsClassSelect` bằng `HvSelect` ngay trong `records-toolbar.tsx` (xoá file cũ + test cũ), và đổi ruột `ClassSelect` của classbook từ Radix Select sang `HvSelect` (giữ file làm wrapper tính nhãn `Tên · lịch`). Cập nhật locator `button` → `combobox` ở test/e2e records (kiểm kê bằng grep) và assertion text option của classbook. `class-select.tsx` vẫn import `components/ui/select` cho tới khi phase này merge — file đó chỉ xoá ở Phase 5.

## Requirements

- Functional:
  - `/records`: **bắt buộc** truyền `placeholder="Chọn lớp"` (mặc định của `HvSelect` là "Chọn…"; `selectedClassId` có thể là `""`); trigger tên truy cập vẫn `Lớp {tên} · {N} HS`; nhóm "Lớp đang dạy"; lọc "Tìm lớp…" khi >5; chọn lại cùng lớp vẫn gọi `onSelectClass` (D7); sheet "Chọn lớp" dưới `sm`.
  - Classbook: trigger `aria-label` giữ `Chọn lớp — đang xem {label}` / `Chọn lớp`; trigger text `Toán 8 · Tối Thứ Ba`; option hiện `Tên` và meta lịch ở hai span **không** ` · ` (D4); không gọi `onSelect` khi chọn lại (guard ở wrapper, đã có).
- Non-functional: URL/`q` behaviour của records không đổi; `records-pages.test.tsx` sửa role ở 5 dòng (grep), giữ assertion sheet của test 375px; `classbook-page.test.tsx` sửa **1 assertion** text option (dòng 444 `toHaveTextContent("Toán 6B · Tối Thứ Ba")` → hai assertion `toHaveTextContent(/Toán 6B/)` + `toHaveTextContent(/Tối Thứ Ba/)`, vì option chuẩn không có separator); phần còn lại của file (combobox/listbox/option) giữ nguyên.

## Architecture

`records-toolbar.tsx`:
```tsx
<HvSelect
  labelId={classLabelId}
  sheetTitle="Chọn lớp"
  groupLabel="Lớp đang dạy"
  searchNoun="lớp"
  placeholder="Chọn lớp"
  options={classes.map((k) => ({ value: k.id, label: k.name, meta: `${k.student_count} HS` }))}
  value={selectedClassId}
  onValueChange={onSelectClass}
  className="min-w-[230px] max-sm:w-full"
/>
```
`class-select.tsx` giữ `classLabel` nhưng tách: `label: klass.name`, `meta: formatScheduleLabel(...)`; `aria-label` như cũ; `className="w-full sm:w-fit sm:max-w-[520px]"`; guard `if (classId !== selected?.id) onSelect(classId)`. Bỏ toàn bộ import `@/components/ui/select` và các ghi chú về accent variables (không còn đúng).

## Related Code Files

- Modify: `apps/web/src/features/teaching/components/records-toolbar.tsx`, `apps/web/src/features/teaching/components/class-select.tsx`; test/e2e đổi locator `button` → `combobox` — kiểm kê bằng `grep -rn '"button", { name: /\^Lớp' apps/web/src apps/web/e2e` (số dòng tại thời điểm lập plan, chạy lại grep khi làm): `__tests__/records-toolbar.test.tsx` ×2 (43, 96); `__tests__/records-pages.test.tsx` ×5 (302, 321, 329, 340, 365 — dòng 365 nằm trong test `mockViewport(375)`, chỉ đổi role, **giữ** assertion sheet); `e2e/records-search.spec.ts` ×3 (28, 32, 33 — spec chạy 375px nên mở **sheet**, option tìm bằng `page.getByRole("option")`); `e2e/class-staff-read.spec.ts` ×1 (39, trong `assertStaffReadJourney`); `e2e/class-staff-write.spec.ts` ×1 (35); `__tests__/classbook-page.test.tsx` dòng 444 (text option, xem Requirements)
- Delete: `apps/web/src/features/teaching/components/records-class-select.tsx`, `apps/web/src/features/teaching/__tests__/records-class-select.test.tsx`
- Verify unchanged: `apps/web/src/features/teaching/pages/records-page.tsx` (hotkey `/` vẫn bail khi focus trong `[role=listbox]`/`[role=dialog]` — markup giữ nguyên), `classbook-page.tsx`

## Implementation Steps

1. Sửa `records-toolbar.tsx` dùng `HvSelect`; xoá import `RecordsClassSelect`.
2. Xoá `records-class-select.tsx` và test của nó (case đã sống ở `hv-select.test.tsx` từ Phase 1).
3. Viết lại `class-select.tsx` trên `HvSelect`; xoá import `ui/select`.
4. Chạy grep ở Related Code Files; đổi từng dòng `getByRole("button", { name: /^Lớp/ })` → `getByRole("combobox", { name: /^Lớp/ })` (2 unit test, 3 e2e); chạy lại grep → 0. Sửa `classbook-page.test.tsx:444` thành hai regex assertion.
5. `npx vitest run src/features/teaching` → xanh; `npm run lint && npm run typecheck`.
6. Commit `refactor(web): teaching class pickers on HvSelect`.

## Success Criteria

- [x] `grep -rn RecordsClassSelect apps/web/src` = 0; `grep -rn "ui/select" apps/web/src/features/teaching` = 0.
- [x] `records-toolbar`, `records-pages`, `student-records-table`, `classbook-page` test xanh; `grep -rn '"button", { name: /\^Lớp' apps/web/src apps/web/e2e` = 0; `grep -rn '" · "' apps/web/src/features/teaching/__tests__` không còn assert separator trên **option**.
- [x] Trên `/records` ở 1024px: mở popover, ✓ mint ở lớp đang chọn, `· 28 HS` bên phải; ở 375px: sheet "Chọn lớp".
- [x] Classbook ở 1024px: trigger `Toán 8 · Tối Thứ Ba` (meta mờ), popover cùng vỏ với records.

## Risk Assessment

- **Classbook test dòng 444–446 assert `toHaveTextContent("Toán 6B · Tối Thứ Ba")` trên option:** option chuẩn không có ` · ` → assertion **đỏ chắc chắn**, sửa theo Requirements (không thêm separator vào option để chiều test). Dòng 451 dùng `hidden: true` vì Radix Select đặt `aria-hidden`: với Popover không còn aria-hidden, `hidden: true` vẫn tìm được → giữ.
- **Classbook e2e (`class-staff-read` dòng 59) đọc `đang xem {STAFF_CLASS}`:** `aria-label` giữ nguyên nên xanh.
- **Trigger classbook trước đây `border-0 shadow-soft-sm` (không viền):** chấp nhận đổi sang viền 2px theo DS — đây chính là mục tiêu plan; ghi vào QA Phase 5.
