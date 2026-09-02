---
phase: 4
title: "Hàng mở rộng 3 khối và điều hướng"
status: pending
priority: P1
effort: "1d"
dependencies: [2, 3]
---

# Phase 4: Hàng mở rộng 3 khối và điều hướng

## Overview
Chuyển `SessionDetailPanel` (tab + card/sheet) thành `SessionExpandRow` hiển thị
đồng thời 3 khối NHẬN XÉT CHUNG · GIÁO ÁN · ĐIỂM BUỔI trong hàng mở rộng của
bảng, với footer gợi ý phím / số ô chưa lưu / Đóng. Bỏ nhánh `HvModal` +
`useMediaQuery` ở page; guard điểm chưa lưu bao cả đổi hàng, lớp, tháng, view;
trạng thái tải/trống/lỗi dùng `HvStateBlock`.

## Requirements
- Functional:
  - `SessionExpandRow` (forwardRef, handle `flush`, `discard`, `requestClose`)
    props: `centerId`, `classId`, `classTitle`, `derived`, `canWrite`,
    `hasScoreComponents`, `onClose`, `onDirtyChange`.
  - Layout: `grid grid-cols-1 min-[900px]:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,1.2fr)]`,
    `border-y-[1.5px] border-mint-200`, nền `cream-50`; mỗi khối `p-4 flex flex-col gap-2`,
    `border-r-[1.5px] border-line-100` (trừ khối cuối; ở mobile bỏ border-r,
    thêm border-b); `h5` 11.5px 800 `ink-400` tracking .3px.
  - Khối 1 NHẬN XÉT CHUNG: giữ logic note draft/save/toast/"Chưa lưu"/"Đã lưu ✓"
    hiện có (label a11y giữ "NHẬN XÉT CHUNG CỦA BUỔI" cho textarea qua
    `aria-label`; tiêu đề hiển thị "NHẬN XÉT CHUNG"); read-only khi `!canWrite`.
  - Khối 2 GIÁO ÁN · BÀI n/N: tiêu đề bài (`font-display` 15px 700), `PlanSummary`,
    dòng cuối `PlanStatusPill` + ngày duyệt nếu `plan.reviewed_at`/tương đương có
    trong `LessonPlan` (kiểm tra schema; không có thì chỉ pill); trống →
    "Chưa có giáo án cho buổi này — soạn ở tab Chương trình & giáo án."; buổi hủy →
    "Buổi hủy — không có giáo án."
  - Khối 3 ĐIỂM BUỔI: tiêu đề "ĐIỂM BUỔI · k ĐẦU ĐIỂM" khi có bộ điểm (k =
    số component) hoặc "ĐIỂM BUỔI"; nội dung `ScoreEntryByStudent` (đã có
    "Mở bảng đầy đủ", footer tiến độ, `+ n học sinh` qua `<details>`) hoặc khối
    điểm chung hiện có (danh sách `HvScoreInput` + "Lưu điểm buổi"); vùng danh
    sách `max-h-[280px] overflow-y-auto`.
  - Footer (`col-span-full`, nền trắng, `border-t line-100`, `px-4 py-2.5`,
    `flex items-center gap-2.5`): "Di chuyển <kbd>↑</kbd> <kbd>↓</kbd> · mở/đóng
    <kbd>Enter</kbd>" (kbd: `bg-cream-200 border line-300 rounded-[6px] px-1.5 text-[11px] font-mono ink-500`,
    ẩn ở mobile), spacer, `<small class="text-sun-600 font-bold">` "n ô chưa lưu"
    khi `dirty > 0`, `HvButton ghost sm` "Đóng" (`aria-label="Đóng chi tiết buổi"`).
  - Page: xóa `HvModal`, `useMediaQuery`, `variant="sheet"`; `selectedSessionId`
    trong state (không URL) như hiện tại; `requestNavigation(next)` với
    `next: { kind: "session", id } | { kind: "class", id } | { kind: "month", month } | { kind: "view", view }`;
    khi `panelDirtyCount > 0` → `UnsavedScoresGuard`; "Lưu và đóng" → `flush()` rồi
    thực hiện; "Bỏ thay đổi" → `discard()` rồi thực hiện. Đổi lớp/tháng đóng
    hàng đang mở.
  - Trạng thái: `sessionsPending` → `HvStateBlock state="loading" title="Đang tải buổi học…"`;
    lỗi → `HvStateBlock state="error" title="Không tải được buổi học."` + nút
    "Thử lại" (`refetch`); trống → `HvStateBlock state="empty" title="Chưa có buổi học nào trong tháng m."`;
    không có lớp → `HvStateBlock state="empty" title="Chưa có lớp đang hoạt động"`
    + `HvButton` "Tạo lớp" (link `/center/classes`) khi `isOwner`.
  - Bàn phím: Escape trong hàng mở rộng → `requestClose`; ↑↓ ở bảng (Phase 3).
- Non-functional: giữ `beforeunload` khi dirty; toast text không đổi
  ("Đã lưu nhận xét buổi … — …", "Đã lưu điểm n học sinh — buổi …").

## Architecture
- Đổi tên file `session-detail-panel.tsx` → `session-expand-row.tsx`
  (`git mv`), bỏ `DetailTab`, `HvSegmented`, `variant`, guard nội bộ đổi tab
  (`PendingAction` chỉ còn `close`, hoặc bỏ hẳn và để page guard qua
  `requestClose` → `onRequestClose`). Chọn: giữ `requestClose` gọi
  `onClose` qua guard của page (page đã có `pendingSelection`), xóa
  `UnsavedScoresGuard` khỏi row để chỉ còn một modal guard trên trang.
- Bảng nhận `renderExpanded={(row) => <SessionExpandRow key={row.session.id} ref={panelRef} … />}`.
- `hasScoreComponents` tính một lần ở page (`useClassScoreComponents`) và
  dùng cho cả chip CHẤM ĐIỂM (Phase 1 hook) lẫn khối điểm.

## Related Code Files
- Rename+Modify: `apps/web/src/features/teaching/components/session-detail-panel.tsx` → `session-expand-row.tsx`
- Modify: `apps/web/src/features/teaching/pages/classbook-page.tsx`
- Modify: `apps/web/src/features/teaching/components/sessions-table.tsx` (nối `renderExpanded`)
- Delete: `apps/web/src/lib/hooks/use-media-query.ts`, `apps/web/src/lib/hooks/__tests__/use-media-query.test.ts`
- Read: `apps/web/src/features/teaching/components/score-entry-by-student.tsx`,
  `score-table-modal.tsx`, `unsaved-scores-guard.tsx`, `plan-summary.tsx`,
  `apps/web/src/features/teaching/lib/teaching-store.ts` (`LessonPlan` fields).

## Implementation Steps
1. `git mv session-detail-panel.tsx session-expand-row.tsx`; đổi tên export
   `SessionExpandRow`, `SessionExpandRowHandle`; xóa tab/segmented/variant.
2. Dựng 3 khối + footer theo class ở trên; giữ nguyên các hook mutation.
3. Page: gỡ `HvModal`/`useMediaQuery`; viết `requestNavigation`, nối vào
   `ClassSelect.onSelect`, `MonthStepper.onChange`, `HvSegmented.onValueChange`,
   `SessionsTable.onSelect/onClose`.
4. Thay các nhánh loading/empty bằng `HvStateBlock`; thêm nhánh error và
   không có lớp.
5. Xóa hook `use-media-query` + test; `grep` không còn tham chiếu.
6. Chạy `npm run typecheck`, `npm run test -- classbook` (sẽ đỏ ở selector,
   Phase 5 sửa).

## Success Criteria
- [x] Bấm hàng → hàng mở rộng xuất hiện ngay dưới, 3 khối cùng hiện; bấm hàng khác → đổi (qua guard nếu dirty).
- [x] Footer hiện "n ô chưa lưu" đúng số và "Đóng" hoạt động.
- [x] Không còn `HvModal` trong `classbook-page.tsx`; `useMediaQuery` không còn trong repo.
- [x] Đổi lớp/tháng/view khi dirty mở guard; "Bỏ thay đổi" không gửi PUT.

## Risk Assessment
- Mất focus khi hàng mở rộng unmount (đổi buổi) → sau khi đổi, focus nút
  hàng mới chọn (`requestAnimationFrame`).
- `ScoreEntryByStudent` gọi `onDirtyChange` với count; giữ `useCallback` ổn
  định để tránh vòng lặp effect.
