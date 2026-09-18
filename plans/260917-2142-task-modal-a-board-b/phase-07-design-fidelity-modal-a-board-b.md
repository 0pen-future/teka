---
phase: 7
title: "Khớp 100% UI với report: Modal A + Bảng B"
status: done
priority: P1
effort: "1d"
dependencies: [3, 4, 5, 6]
---

# Phase 7: Khớp 100% UI với report (Modal A + Bảng B)

## Context Links

- Report: `plans/reports/ui-redesign-260917-2117-task-board-and-task-modal.html` (mục Modal · A, Bảng · B, `#trienkhai`)
- Mã: `apps/web/src/features/tasks/components/{task-form-modal,task-card,task-card-styles,quick-done-checkbox,actions-menu,board-filter-bar,assignee-avatar,task-column,collapsed-column-rail,board-desktop}.tsx`, `pages/task-board-page.tsx`, `lib/column-colors.ts`
- Test: `__tests__/{task-form-modal,task-board-page}.test.tsx`; e2e `e2e/tasks-board*.spec.ts`

## Overview

Sau Phase 3–6 UI đã đúng hành vi nhưng còn lệch report ở bố cục, kích thước,
màu và copy. Phase này đóng mọi chênh lệch còn lại, giữ nguyên contract API,
hook và lib kanban.

## Chênh lệch cần đóng

### Modal A

| # | Report | Hiện tại | Sửa |
|---|--------|----------|-----|
| M1 | Eyebrow: pill cột (sky; mint + check khi cột xong) + `<người tạo> tạo dd/MM/yyyy` | chuỗi `Cột · Người · Tạo dd/MM` | `description` thành node eyebrow, ngày đủ năm |
| M2 | Hàng `Cột | Người phụ trách` (1 cột ≤520px), rồi `Hạn` + chip nhanh ngay dưới | Cột full, rồi `Người phụ trách | Hạn` | Đổi bố cục |
| M3 | Chip hạn nhanh: min-h 32, viền 1.5px line-200, 12.5px ink-500, hover mint | HvChip sm (pressed mint) | Nút local, không pressed |
| M4 | Độ ưu tiên: ô 44px, viền 2px, radius 14, chấm 9px, tint theo mức khi chọn | HvChip radio mint | Nút radio local |
| M5 | Footer: `Xoá` chữ đỏ + icon thùng rác · spacer · pill `Chưa lưu` sun · `Huỷ` · `Lưu`, gap 10, border-top | ghost-sm coral + HvBadge warning | Nút text-danger + pill sun |
| M6 | Hộp xác nhận **inline** cuối modal (`role=alertdialog`): xoá "Công việc sẽ rời khỏi bảng. Bạn có 6 giây để hoàn tác." Giữ lại/Xoá; bỏ "Bỏ thay đổi chưa lưu?" + "Tiêu đề, mô tả hoặc thuộc tính bạn vừa sửa sẽ không được giữ." Tiếp tục sửa/Bỏ thay đổi (primary) | HvConfirmDialog lồng, copy khác | Panel inline thay HvConfirmDialog |
| M7 | Toast: "Đã lưu công việc"; `Đã xoá "<tiêu đề>"` + Hoàn tác → "Đã khôi phục công việc"; "Vui lòng nhập tiêu đề"; "Tiêu đề tối đa 200 ký tự"; "Mô tả vượt 2.000 ký tự, hãy rút gọn" | "Đã tạo/cập nhật công việc", "Đã xoá công việc", không toast lỗi | Đổi copy, thêm toast lỗi validate |

### Bảng B

| # | Report | Hiện tại | Sửa |
|---|--------|----------|-----|
| B1 | Card: tiêu đề → avatar → ⋯; padding-left 18px, không viền trái; chấm coral trước tiêu đề khi quá hạn; tiêu đề 14.5px/1.3 gạch ngang khi xong; mô tả 13px; hover nhấc + shadow-md | avatar trước tiêu đề, viền trái 4px, 13.5px | Sửa `TaskCardBody`, `taskCardSurfaceClassName` |
| B2 | Nút Xong: 26px tròn nổi mép trái, chỉ khi chưa xong, ẩn tới hover/focus, `aria-label`/`title` "Đánh dấu hoàn thành" | checkbox 40px trong hàng, luôn hiện | Viết lại `quick-done-checkbox` thành nút |
| B3 | Chip ưu tiên có chấm; chip hạn có icon (alert/clock/cal/check) | HvBadge không chấm/icon | `dot` + icon lucide |
| B4 | Menu: "Chuyển tới", icon di chuyển từng mục, `<hr>`, "Mở chi tiết" | "Chuyển sang", không icon, không mở chi tiết | Sửa `ActionsMenu` (thêm `onOpen`) |
| B5 | Toast sau chuyển qua menu: `Đã chuyển "<tiêu đề>" sang <cột>` + Hoàn tác → "Đã hoàn tác"; Xong nhanh dùng cùng toast | Menu không toast; Xong: "Đã đánh dấu xong" | Gộp trong `moveWithUndo` ở page |
| B6 | Dải lọc: một `role=group`, nút `aria-pressed`, min-h 36, 13px, pressed = nền ink-900 chữ trắng; "Quá hạn" đỏ khi >0 & chưa bật; sep 1.5×22 line-300; chip giáo viên có avatar 20px; không có "Xoá lọc"/select "Khác…" | 2 radiogroup HvChip, Xoá lọc, overflow select | Viết lại `BoardFilterBar` |
| B7 | Avatar: 28px (26 trong card, 20 trong chip), chữ trắng 800 11px trên mint-500/sky-400/sun-500/ink-400 | 24px pastel chữ ink-900 | Sửa `AssigneeAvatar` (prop `size`) |
| B8 | Cột 272px, padding 8, min-h 360, nền cream-200 (xong: mint-50), không viền; over = ring mint-400 + mint-100; header 15px + count 11.5px; nút 36px, thứ tự thu gọn → thêm; **không** chấm màu | 280px, viền, chấm màu, thêm → thu gọn | Sửa `TaskColumn`, `COLUMN_TINT.none` |
| B9 | Ô trống: viền line-300, min-h 120, 13px, "Không có việc khớp bộ lọc", nút mint "Thêm việc" có icon +; over → viền mint-400 nền mint-50 | line-200, 96px, 12px, "khớp lọc" | Sửa copy + class |
| B10 | Rail thu gọn 56px, padding 8/4, chevron + tên dọc + count, không chấm | 44px có chấm | Sửa rail |
| B11 | Header: "Cấu hình cột" min-h 44; wrapper fade hai mép, gap 12 | size sm, fade phải | Sửa page + `BoardDesktop` |

## Validation

- `npx vitest run src/features/tasks src/lib/kanban` (sửa locator theo copy/role mới)
- `make lint-web test-web`
- `make e2e-isolated E2E_ARGS="tasks-board"` (sửa locator: Bỏ thay đổi chưa lưu?, Xoá lọc → bấm lại chip, alertdialog)

## Risk / Rollback

- Đổi role checkbox → button và radiogroup → aria-pressed là thay đổi a11y có chủ đích theo report; e2e/unit cập nhật cùng commit.
- Rollback: revert commit của phase; không có thay đổi API/schema.
