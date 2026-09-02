---
phase: 2
title: "Consistency pass on existing scoring screens"
status: completed
priority: P1
effort: "0.5d"
dependencies: [1]
---

# Phase 2: Consistency pass on existing scoring screens

## Overview

Áp primitive từ Phase 1 vào bốn file sẽ **còn tồn tại** sau plan: panel chi
tiết buổi (phần khung, tab, ô điểm chung), trang cấu hình lớp, dialog gán, modal
soạn bộ điểm. Không đổi bố cục. `component-score-grid.tsx` **không** đụng ở
phase này vì Phase 3 xoá nó (tránh làm hai lần); `save-button-styles.ts` và
`parseScoreInput` cũ cũng chỉ xoá ở Phase 3 khi caller cuối cùng biến mất.

## Requirements

- Functional: hành vi người dùng không đổi, ngoại trừ (a) ô điểm chung chấp nhận dấu phẩy và không còn spinner, (b) chữ vô nghĩa trong ô điểm chung báo lỗi và chặn nút lưu thay vì bị bỏ qua lặng lẽ. Ô điểm chung **để rỗng vẫn bị bỏ qua như hiện tại** (không gửi `null`, không xoá điểm) — `MarkEntryInput.score` là tri-state và xoá điểm chung không nằm trong scope.
- Non-functional: mọi nút/ô ≥ 44px; test hiện có được cập nhật theo markup mới, không bị xoá.

## Related Code Files

| Action | File | Việc cần làm |
|--------|------|--------------|
| Modify | `apps/web/src/features/teaching/components/session-detail-panel.tsx` | ✕ đóng → `HvButton variant="ghost" size="sm" icon={<HvIcon name="x" />} aria-label="Đóng chi tiết buổi"`; tabs thô → `HvSegmented variant="tabs" idBase="session-detail"` và vùng nội dung bọc `<div role="tabpanel" id="session-detail-panel-{tab}" aria-labelledby="session-detail-tab-{tab}">`; ô điểm chung `type="number"` → `HvScoreInput` (parse từ `@/components/hv`; `""`/`null` → bỏ qua như cũ; `"invalid"` → `invalidIds`, nút lưu disabled); nút "Lưu điểm buổi"/"Lưu nhận xét" → `HvButton variant="primary" size="sm"`; "Đang tải danh sách học sinh…" → `HvStateBlock state="loading" compact`; bỏ import `save-button-styles` và `parseScoreInput` cũ khỏi file này (grid vẫn dùng chúng tới Phase 3) |
| Modify | `apps/web/src/features/center/pages/class-config-page.tsx` | h1 thô 26px → `text-[length:var(--text-xl)] font-bold` (`HvPageHeader` chưa tồn tại, để plan quét tổng làm); "Đang tải…" ×2 → `HvStateBlock`; list rỗng → `HvStateBlock state="empty"` (giữ copy); `th` thêm `scope="col"`; nút "Gán bộ điểm" bỏ `title`, thêm helper "Tạo ít nhất một bộ điểm trước" dưới tiêu đề mục khi `sets.length === 0`; xoá inline hai bước → `HvConfirmDialog tone="danger"` |
| Modify | `apps/web/src/features/center/components/assign-score-set-dialog.tsx` | "Đang tải…" → `HvStateBlock compact`; thông điệp 409 → `HvNotice tone="warning" role="alert"` (giữ `role="alert"` hiện có ở dòng 132); nút Đóng/Xóa gán/Gán → `HvButton` (ghost / danger / primary) |
| Modify | `apps/web/src/features/center/components/score-set-editor-modal.tsx` | nút ↑/↓/Xóa → `HvButton variant="ghost" size="sm" icon={<HvIcon name="arrow-up" />}` … với `aria-label` giữ nguyên; giữ điều kiện chỉ render Xóa khi `components.length > 1`; nút "+ Thêm cột điểm" → `HvButton variant="secondary" block`; lỗi `components.root` → `HvNotice tone="danger"` |
| Modify | `apps/web/src/features/teaching/__tests__/classbook-page.test.tsx` | nếu có query theo nút đóng/tab thì đổi sang `getByRole("tab", {name})` / `getByRole("button", {name: "Đóng chi tiết buổi"})` |
| Modify | `apps/web/src/features/center/__tests__/class-config-page.test.tsx` | xoá bộ điểm: hai bước inline → `getByRole("dialog")` + nút "Xác nhận xóa"; 409 vẫn assert `getByRole("alert")` (dòng ~150); danh sách rỗng → `HvStateBlock`; `columnheader` có `scope="col"`; không còn `title` trên nút Gán |

## Implementation Steps

1. `session-detail-panel.tsx`: nút đóng, tabs + tabpanel, nút lưu, state block.
2. Ô điểm chung: `HvScoreInput`; giữ `Record<studentId,string>`; commit `"invalid"` → `invalidIds`; payload giữ logic cũ (chỉ push khi là số).
3. `class-config-page.tsx`: HvStateBlock loading/empty/error ở hai section; `th scope`; helper text thay tooltip; `HvConfirmDialog` với state `deletingSet`.
4. `assign-score-set-dialog.tsx` + `score-set-editor-modal.tsx`: chỉ đổi nút và khối thông báo; `<select>` và danh sách giữ nguyên.
5. Cập nhật test; chạy `npx vitest run src/features/teaching src/features/center src/components/hv`, rồi `npm run lint && npm run typecheck`.

## Test scenarios

| File test | Case mới/đổi |
|-----------|--------------|
| classbook-page.test (panel) | tab "Điểm buổi" chọn qua `getByRole("tab", {name})`; vùng nội dung có `role="tabpanel"`; ô điểm chung gõ "7,5" → PUT marks với `7.5`; gõ "abc" → `aria-invalid`, nút "Lưu điểm buổi" disabled; ô rỗng → không có entry trong payload |
| class-config-page.test | xoá qua dialog; rỗng → `HvStateBlock`; `columnheader` scope; 409 → `role="alert"` với `CONFLICT_MESSAGE` |

## Success Criteria

- [x] `grep -n 'type="number"' apps/web/src/features/teaching/components/session-detail-panel.tsx` rỗng.
- [x] Không còn chuỗi `Đang tải…` tự viết trong ba file `session-detail-panel.tsx`, `class-config-page.tsx`, `assign-score-set-dialog.tsx`; chuỗi loading chỉ xuất hiện làm `title` của `HvStateBlock` (grep `'Đang tải'` vẫn khớp các prop `title=` đó). Các file khác ngoài scope.
- [x] Không còn `title=` làm tooltip cho nút disabled trong bốn file trên.
- [x] Test `features/teaching`, `features/center`, `components/hv` xanh; lint/typecheck xanh.
- [x] Ảnh chụp tay ở 1080px: bố cục hai màn không đổi ngoài kích thước nút/ô. — *đã chụp qua stack e2e cô lập: `scores-panel-1080.png`, `class-config-1080.png` trong `plans/reports/screenshots-260902-scoring-ui/`; body 1080/1080, không container cuộn ngang.*

## Risk Assessment

- **Tabs**: `HvSegmented variant="tabs"` có roving tabindex; phải có `tabpanel` liên kết (đã yêu cầu) để không thành mẫu ARIA dở dang.
- **`HvButton.icon` là `ReactNode`** (`hv-button.tsx:69`): truyền chuỗi `"x"` vẫn typecheck xanh nhưng hiện chữ; luôn dùng `<HvIcon name=… />`.
- **Test 409** dựa vào `role="alert"`; `HvNotice` phải nhận prop `role` (Phase 1).
