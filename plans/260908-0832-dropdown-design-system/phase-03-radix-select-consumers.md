---
phase: 3
title: "Radix Select → HvSelect (audit, thanh toán, ghi danh)"
status: completed
priority: P1
effort: "3h"
dependencies: [1]
---

# Phase 3: Radix Select → `HvSelect` (audit, thanh toán, ghi danh)

## Overview

Chuyển 4 dropdown còn dùng shadcn/Radix `Select` (audit ×2, hình thức thanh toán, lớp khi ghi danh) sang `HvSelect`. **Không** xoá `components/ui/select.tsx` ở phase này: `teaching/components/class-select.tsx` (Phase 2) cũng import nó, nên xoá ở đây sẽ vỡ khi Phase 2/3 chạy song song — xoá ở Phase 5 (D8). Hai trong số này nằm trong `HvModal`; hành vi nested đã được chứng minh bằng test ở Phase 1, phase này chỉ xác nhận trên consumer thật.

## Requirements

- Functional:
  - Audit: hai dropdown `w-[180px]`, `aria-label` "Giáo viên" / "Nhóm hành động"; `sheetTitle` cùng chữ; `searchNoun="giáo viên"` và `searchNoun="nhóm hành động"` (Nhóm hành động có 18+ mục → ô lọc **luôn** hiện, note phải là `Không có nhóm hành động nào khớp "q"`); giá trị `all` hiển thị "Tất cả …"; option `custom` disabled "Tùy chỉnh" chỉ khi `isCustomAction`; đổi giá trị áp filter ngay (không cần guard D7: chỉ set filter).
  - Ghi nhận thanh toán: `id="payment-method"` cho `FieldLabel htmlFor` (tên truy cập = "Hình thức"); `aria-invalid` từ `errors.method`; `w-full`; `sheetTitle="Hình thức"`.
  - Ghi danh: `id="enroll-class"`, placeholder "Chọn lớp…", option `label: klass.name`, `meta: \`${formatScheduleSummary(...)} · ${formatMoney(...)}/buổi\``; `searchNoun="lớp"`; `sheetTitle="Chọn lớp"`.
- Non-functional: test hiện có của 3 màn xanh với thay đổi tối thiểu (D9 `mockViewport(1024)` trong `beforeEach`; `record-payment-dialog.test.tsx` và `enroll-student-dialog.test.tsx` có `findByRole("dialog")` trần → phải ở nhánh popover). Thêm **1 test** ở `record-payment-dialog.test.tsx`: `getByRole("combobox", { name: "Hình thức" })` tồn tại và có `aria-invalid="true"` sau khi submit không chọn hình thức (bảo vệ dây `FieldLabel htmlFor` + `errors.method`).

## Architecture

Mỗi consumer map dữ liệu → `HvSelectOption[]` bằng `useMemo` hoặc inline (danh sách nhỏ). Không giữ state thêm. Ví dụ audit:
```tsx
<HvSelect
  aria-label="Giáo viên" sheetTitle="Giáo viên" searchNoun="giáo viên"
  className="w-[180px]"
  options={[{ value: "all", label: "Tất cả giáo viên" }, ...members.map((m) => ({ value: m.id, label: m.full_name }))]}
  value={filters.actor_id ?? "all"}
  onValueChange={(value) => set({ actor_id: value === "all" ? undefined : value })}
/>
```

## Related Code Files

- Modify: `apps/web/src/features/audit/components/audit-filters.tsx`, `apps/web/src/features/collections/components/record-payment-dialog.tsx`, `apps/web/src/features/roster/components/enroll-student-dialog.tsx`, `apps/web/src/features/audit/__tests__/audit-page.test.tsx` (thêm `mockViewport(1024)` nếu cần), `apps/web/src/features/roster/__tests__/enroll-student-dialog.test.tsx` (tương tự), `apps/web/src/features/collections/__tests__/record-payment-dialog.test.tsx` (chỉ nếu đỏ)
- Không xoá ở phase này: `apps/web/src/components/ui/select.tsx` (Phase 5)
- Verify unchanged: `apps/web/e2e/roster.spec.ts` (đã dùng `combobox` "Lớp" + `option`, chạy 1280px → popover)

## Implementation Steps

1. `audit-filters.tsx`: thay 2 `Select` → `HvSelect`; giữ `Input` hành động và date như cũ.
2. `record-payment-dialog.tsx`: thay `Select` trong `Field` → `HvSelect id="payment-method" aria-invalid=…`.
3. `enroll-student-dialog.tsx`: thay `Select` → `HvSelect id="enroll-class"` với `meta`.
4. `grep -rn "components/ui/select" apps/web/src` → chỉ còn `teaching/components/class-select.tsx` (Phase 2) hoặc 0 nếu Phase 2 đã merge; **không** xoá file ở phase này.
5. Thêm test `aria-invalid`/tên "Hình thức" vào `record-payment-dialog.test.tsx`; chạy `npx vitest run src/features/audit src/features/collections src/features/roster/__tests__/enroll-student-dialog.test.tsx`; sửa test theo D9 khi option nằm trong sheet.
6. Tự kiểm tra thủ công popover trong dialog: mở "Ghi nhận thanh toán" ở 1024px, mở Hình thức, dùng ↑↓ Enter; ở 375px mở sheet trong dialog, Esc đóng sheet trước.
7. `npm run lint && npm run typecheck`; commit `refactor(web): audit, payment and enroll pickers on HvSelect`.

## Success Criteria

- [x] `grep -rn "components/ui/select" apps/web/src/features/{audit,collections,roster}` = 0; build xanh.
- [x] `audit-page`, `record-payment-dialog` (kèm test mới `aria-invalid`), `enroll-student-dialog`, `collections-page`, `students-page` test xanh.
- [x] Audit "Nhóm hành động": ô lọc `Tìm nhóm hành động…` luôn hiện; gõ chuỗi lạ → note đúng noun.
- [x] Trong `HvModal` ở 1024px: popover mở, chọn bằng click và bàn phím, focus quay lại trigger, dialog không đóng.
- [x] Trong `HvModal` ở 375px: sheet mở trên form, Esc/chọn đóng sheet, form vẫn mở với giá trị mới.
- [x] Option "Tùy chỉnh" hiển thị mờ, không chọn được, ↑↓ bỏ qua.

## Risk Assessment

- **Popover / sheet trong Dialog modal:** đã chứng minh bằng 2 test nested ở Phase 1 (bước 6). Tín hiệu vỡ trên consumer thật ở bước 6 phase này (click option không chọn, Esc đóng cả form): quay lại `hv-select.tsx`/`hv-modal.tsx` theo phản ứng đã định ở Phase 1 (`modal` Popover / prop `inDialog`; prop additive `onEscapeKeyDown` cho `HvModal`), **không** vá ở consumer và không dùng `stopPropagation`.
- **Test `within(dialog)` của form tìm option:** với `mockViewport(1024)` option nằm trong portal ngoài dialog → dùng `screen.findByRole("option")` (audit-page và enroll-student hiện đã dùng `screen`).
