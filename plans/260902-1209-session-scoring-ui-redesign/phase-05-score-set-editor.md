---
phase: 5
title: "Score set editor"
status: completed
priority: P1
effort: "1d"
dependencies: [1, 2]
---

# Phase 5: Score set editor

## Overview

Nâng `ScoreSetEditorModal` theo report §5.1: modal `size="lg"` (720px), hàng
48px với nút 44px, lỗi hiện **dưới đúng hàng**, `HvSegmented` "Từng cột | Dán
danh sách", đếm "6/10 cột", dải xem trước tiêu đề bảng như giáo viên sẽ thấy.
Giữ `react-hook-form` + `watch/setValue` trên `string[]` (D3), không đổi schema.

## Requirements

- Functional:
  - Modal `HvModal size="lg"`; form id `score-set-editor-form` giữ; label "Tên bộ điểm", "Tên cột điểm N" giữ (test cũ).
  - Chế độ "Từng cột": mỗi hàng `Input` 48px + `HvButton variant="ghost" icon={<HvIcon name="arrow-up" />}` ↑ ↓ (disabled đầu/cuối) + Xoá (`icon={<HvIcon name="trash" />}`; **giữ hành vi hiện có**: chỉ render khi `components.length > 1`); lỗi của hàng qua `FieldError` ngay dưới hàng đó (`errors.components?.[i]?.message`); lỗi `components.root` (rỗng/quá 10) → `HvNotice tone="danger"` trên danh sách.
  - Chế độ "Dán danh sách": một `<textarea>` 6 dòng, placeholder 3 dòng mẫu; khi chuyển vào: `components.join("\n")`; khi chuyển ra hoặc submit: parse `split(/[\n,;]+/)`, trim, bỏ rỗng, `slice(0,10)`; nếu bị cắt → `HvNotice tone="warning"` "Chỉ giữ 10 cột đầu". Trùng tên hiện ngay bên dưới textarea qua helper `findDuplicateIndexes` (tách từ `superRefine` để dùng chung).
  - Bộ đếm "n/10 cột" cạnh tiêu đề danh sách; nút "Thêm cột" **vẫn render** ở 10 nhưng disabled kèm helper text "Tối đa 10 cột" (không `title`; không ẩn nút — bố cục không nhảy).
  - Không làm (non-goal, xem plan.md): bấm tiêu đề để focus ô, Enter ở hàng cuối tự thêm hàng.
  - Dải xem trước: hàng `<div aria-label="Xem trước tiêu đề bảng">` với chip `HvBadge variant="neutral"` theo thứ tự hiện tại, cập nhật live; tên rỗng hiển thị "(trống)".
  - Submit: giữ `useApiFormErrors(form, {conflictField:"name"})`, toast giữ.
- Non-functional: hai chế độ là hai view trên cùng `components` (research §3); chỉ một view mutable tại một thời điểm; dưới `sm` bottom sheet, textarea dùng `inputmode` mặc định.

## Architecture

```
features/center/
  lib/score-set-components.ts     parsePastedComponents(text): {names, truncated}; findDuplicateIndexes(names): Set<number>
  schemas/grading.ts              superRefine gọi findDuplicateIndexes (không đổi output)
  components/score-set-editor-modal.tsx   HvModal lg, HvSegmented mode, hai view, preview
  components/score-set-preview-strip.tsx  chip strip (dùng lại ở Phase 6 dialog gán)
```

State bổ sung trong modal: `mode: "rows" | "paste"`, `pasteText: string`,
`truncated: boolean`. Chuyển `rows → paste`: `setPasteText(components.join("\n"))`.
Chuyển `paste → rows` hoặc submit khi `mode === "paste"`: `setComponents(parse(pasteText).names)`.

## Related Code Files

| Action | File | Ghi chú |
|--------|------|---------|
| Create | `apps/web/src/features/center/lib/score-set-components.ts` | hai hàm thuần |
| Create | `apps/web/src/features/center/__tests__/score-set-components.test.ts` | |
| Modify | `apps/web/src/features/center/schemas/grading.ts` | `superRefine` dùng `findDuplicateIndexes`; message/path giữ nguyên |
| Create | `apps/web/src/features/center/components/score-set-preview-strip.tsx` | props `names: string[]`, `size?: "sm" \| "md"` |
| Modify | `apps/web/src/features/center/components/score-set-editor-modal.tsx` | như trên |
| Modify | `apps/web/src/features/center/__tests__/class-config-page.test.tsx` | thêm case dán danh sách, lỗi per-row, đếm, preview, cắt >10 |

## Implementation Steps

1. `score-set-components.ts` + test: parse (`"A\nB, C;D"` → 4; 12 dòng → 10 + `truncated`; dòng trống bị bỏ trước khi cắt), duplicates (case-insensitive, trim).
2. Đổi `superRefine` trong `grading.ts` sang helper; chạy test schema hiện có (nếu có) hoặc test trang.
3. `score-set-preview-strip.tsx`.
4. Modal: `size="lg"`; header có `HvSegmented` (`aria-label="Cách nhập cột điểm"`); view rows với hàng 48px (`grid grid-cols-[1fr_auto_auto_auto] gap-2 items-start`); `FieldError` per row; counter; preview.
5. View paste: textarea + duplicate helper + truncated notice; nút "Áp dụng" không cần — chuyển chế độ hoặc submit là áp dụng.
6. Submit path khi đang ở paste: parse trước `handleSubmit`.
7. Test + lint/typecheck. Kiểm tra tay ở 1080px và 390px.

## Test scenarios

| Case | Assertion |
|------|-----------|
| Mở tạo mới | dialog có `role="radio"` "Từng cột" (checked) và "Dán danh sách"; counter "1/10 cột" |
| Nhập "Miệng" ở cột 1, "miệng " ở cột 2, submit | `FieldError` "Tên cột điểm bị trùng" dưới cột 2 (query trong hàng 2), không có POST |
| Chuyển "Dán danh sách", dán 8 dòng, submit | POST với 8 components đúng thứ tự; toast "Đã tạo bộ điểm X" |
| Dán 12 dòng | notice "Chỉ giữ 10 cột đầu"; chuyển về rows thấy 10 hàng; counter "10/10 cột"; nút Thêm disabled + helper |
| Dán 2 dòng trùng | helper trùng hiện ngay khi gõ (trước submit) |
| Sửa bộ có 3 cột, ↓ ở hàng 1 | preview strip thứ tự đổi; PUT với thứ tự mới |
| Xoá tới 1 hàng | hàng còn lại không có nút Xoá (hành vi cũ) |

## Success Criteria

- [x] Tạo bộ 8 cột bằng một lần dán; lỗi trùng đúng hàng; counter và preview live.
- [x] Không còn `div` lỗi tự viết; `HvModal size="lg"`.
- [x] Test `class-config-page.test.tsx` cũ + mới xanh; lint/typecheck xanh.

## Risk Assessment

- **Zod `superRefine` refactor đổi thông điệp**: giữ nguyên chuỗi "Tên cột điểm bị trùng" và path `[index]`; test cũ bảo vệ.
- **Đồng bộ hai view**: chuyển chế độ khi textarea có lỗi trùng vẫn cho chuyển (lỗi sẽ hiện per-row); không giữ hai nguồn thật cùng lúc.
- **`FieldError` dedupe theo message**: per-row dùng một message nên không ảnh hưởng.
