---
phase: 6
title: "Score set list and assign dialog"
status: completed
priority: P2
effort: "1d"
dependencies: [1, 2, 5]
---

# Phase 6: Score set list and assign dialog

## Overview

Trang cấu hình lớp (report §5.2–5.3, trừ phần cần API): danh sách bộ điểm
thành thẻ `HvCard` với chip cột (dùng `ScoreSetPreviewStrip` của Phase 5) và
hành động Sửa/Xoá; dialog gán bộ điểm thay `<select>` bằng radio card Radix
hiển thị cột ngay khi chọn, `HvNotice` báo trước quy tắc lớp đã có điểm (D4);
bảng lớp có bản thẻ dưới `md` bằng hai markup (research §5). Bảng lớp **không**
hiển thị bộ điểm đang gán của từng lớp: API chỉ có `GET /classes/:id/score-components`
theo từng lớp (N+1 request), không có endpoint batch → non-goal, follow-up API.

## Requirements

- Functional:
  - **Danh sách bộ điểm**: `<ul>` thẻ `HvCard variant="raised" padding="md"`; tiêu đề tên bộ, chip cột theo thứ tự, `HvBadge neutral` "N cột"; hành động `HvButton ghost size="sm"` "Sửa" và "Xoá" (Xoá → `HvConfirmDialog` từ Phase 2). Không hiển thị "Đang dùng ở N lớp" (D5). Rỗng → `HvStateBlock state="empty"` với action "Tạo bộ điểm".
  - **Dialog gán**: `HvModal size="lg"`; mở thì hiện `HvNotice tone="info"` "Lớp đã ghi nhận điểm sẽ không đổi hoặc xoá được bộ điểm."; `RadioGroup.Root` với mỗi `RadioGroup.Item` là thẻ (tên + preview strip), `aria-label="{tên}, {n} cột"`; bộ đang gán (so sánh theo tên + cột, vì API không trả `source_set_id`) được gợi ý "Đang dùng" bằng `HvBadge info` — chỉ là gợi ý; nút Gán disabled khi chưa chọn; "Xoá gán" `HvButton variant="danger"` chỉ khi lớp đang có cột; 409 → khoá như cũ, `HvNotice tone="warning"` với `CONFLICT_MESSAGE` giữ nguyên, thêm classId vào `lockedClassIds` (state trang, `Set<string>`), mở lại dialog cùng lớp trong phiên → khoá ngay.
  - **Bảng lớp**: giữ hai cột như hiện tại (tên lớp, hành động). ≥`md` là `<table>` (`th scope="col"`, `<caption class="sr-only">`); <`md` là `<ul>` thẻ: tên lớp + nút "Gán bộ điểm" 44px block. Hai markup, một hàm render hàng. Không thêm cột bộ điểm/số cột (cần API batch).
  - Nút "Gán bộ điểm" khi chưa có bộ nào: disabled + helper text (đã làm ở Phase 2, giữ).
- Non-functional: giữ mọi copy toast; dưới `sm` dialog là bottom sheet, radio card cuộn dọc.

## Architecture

```
features/center/
  pages/class-config-page.tsx          ScoreSetsSection (cards) + ClassAssignmentSection (two markups) + lockedClassIds
  components/score-set-card.tsx        thẻ bộ điểm
  components/assign-score-set-dialog.tsx  radio cards + notice + locked prop
  components/class-score-set-table.tsx  table + card list (shared row model)
```

`AssignScoreSetDialog` props thêm: `locked?: boolean` (từ `lockedClassIds`),
`onLocked: () => void` (gọi khi nhận 409). `currentSetGuess = sets.find(s => sameColumns(s.components, current.map(c => c.name)))`
tính từ `GET /classes/:id/score-components` mà dialog đã fetch sẵn cho một lớp. Endpoint gán là
`POST /classes/:id/score-set` (không phải PUT). Nút Đóng `variant="ghost"`, Xoá gán `danger`, Gán `primary`.

## Related Code Files

| Action | File | Ghi chú |
|--------|------|---------|
| Create | `apps/web/src/features/center/components/score-set-card.tsx` | |
| Create | `apps/web/src/features/center/components/class-score-set-table.tsx` | hai markup |
| Modify | `apps/web/src/features/center/components/assign-score-set-dialog.tsx` | select → RadioGroup card; notice; locked |
| Modify | `apps/web/src/features/center/pages/class-config-page.tsx` | dùng ba component; `lockedClassIds` |
| Modify | `apps/web/src/features/center/__tests__/class-config-page.test.tsx` | assign: `selectOptions` → `user.click(getByRole("radio", {name: /Giữa kỳ/}))`; case 409 giữ; case mở lại đã khoá; card list rỗng/đầy; bảng có `columnheader` scope |

## Implementation Steps

1. `score-set-card.tsx` + thay `<li>` cũ trong `ScoreSetsSection`; giữ hành động Sửa/Xoá và dialog xoá của Phase 2.
2. `assign-score-set-dialog.tsx`: `HvNotice` info khi mở; `RadioGroup` từ `radix-ui`; preview strip mỗi thẻ; "Đang dùng" gợi ý; `locked` prop; 409 → `onLocked`.
3. `class-config-page.tsx`: `lockedClassIds` state; truyền `locked`/`onLocked`.
4. `class-score-set-table.tsx`: `rows: {classId, className}`; table với `hidden md:block`, list với `md:hidden`; cả hai cùng `data-testid` riêng để test chọn markup.
5. Test: sửa case assign (radio thay combobox), thêm case khoá lại, thẻ rỗng; chạy vitest center + lint/typecheck.
6. Kiểm tra tay 1080px và 390px: thẻ, dialog radio card, bảng → thẻ.

## Test scenarios

| Case | Assertion |
|------|-----------|
| Có 2 bộ điểm | 2 thẻ; mỗi thẻ có chip từng cột và badge "N cột" |
| Không có bộ | `HvStateBlock` empty với nút "Tạo bộ điểm"; nút "Gán bộ điểm" trong bảng disabled + helper |
| Mở dialog gán | `role="note"` info hiện; `role="radio"` cho từng bộ; chọn bộ → preview strip trong thẻ đã chọn `aria-checked="true"`; Gán → `POST /classes/:id/score-set` đúng id; toast giữ |
| Lớp đã có cột trùng bộ X | thẻ X có badge "Đang dùng" |
| 409 | `getByRole("alert")` (HvNotice `tone="warning" role="alert"`) chứa `CONFLICT_MESSAGE`; nút Gán/Xoá gán disabled; đóng, mở lại cùng lớp → vẫn khoá không cần gọi API |
| Bảng | `within(getByTestId("class-score-set-table"))` có `columnheader` ×2; `getByTestId("class-score-set-cards")` có `listitem` = số lớp |

## Success Criteria

- [x] Dialog gán cho thấy cột trước khi bấm Gán; `<select>` không còn.
- [x] Không còn `components.join(", ")` để hiển thị; mọi chỗ dùng `ScoreSetPreviewStrip`.
- [x] Trang cấu hình lớp không gọi thêm request nào so với hiện tại khi tải (không N+1 `score-components`).
- [x] Test center xanh, lint/typecheck xanh; ảnh chụp 390px kèm PR. — *`class-config-390-cards.png`, `score-set-editor-390-rows.png`, `class-config-1280-with-sets.png`, `assign-dialog-1280.png`, `score-set-editor-1280-paste.png` trong `plans/reports/screenshots-260902-scoring-ui/`; 390px không cuộn ngang, nút 44px, chip cột xuống dòng.*

## Risk Assessment

- **"Đang dùng" gợi ý sai khi hai bộ cùng tên cột**: chỉ là gợi ý, ghi rõ "trùng cột với" nếu có nhiều hơn một bộ khớp → không gắn badge.
- **`lockedClassIds` lệch thực tế** (điểm bị xoá ở nơi khác): chỉ sống trong phiên trang; reload là hết; ghi trong Open questions cho follow-up API `has_scores`.
- **Hai markup nhân đôi assertion trong jsdom**: test scope theo `data-testid` như research khuyến nghị.
