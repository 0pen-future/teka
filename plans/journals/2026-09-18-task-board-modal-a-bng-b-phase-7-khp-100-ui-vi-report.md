---
title: "Task board Modal A + Bảng B: phase 7 khớp 100% UI với report"
date: 2026-09-18
summary: "Đóng gap M1-M7/B1-B11 theo report ui-redesign-260917-2117; sửa 7 phát hiện review; unit 195 pass, e2e tasks-board 8/8"
---

# Task board Modal A + Bảng B: phase 7 khớp 100% UI với report

## What happened

- Phase 7 của plan `plans/260917-2142-task-modal-a-board-b` đưa màn "Công việc" khớp 100% với Modal A và Bảng B trong `plans/reports/ui-redesign-260917-2117-task-board-and-task-modal.html`: card (tiêu đề → avatar → ⋯, chấm coral khi quá hạn, gạch ngang khi xong), nút Xong nhanh 26px nhô mép trái, chip lọc `aria-pressed` với avatar cho giáo viên, cột tint theo `columnTintClassName`, rail thu gọn 56px, fade hai mép bảng, modal với panel xác nhận inline (`alertdialog`), eyebrow "Cột · Người tạo ngày", toast copy đúng report.
- Lần chạy e2e đầu 3/8 đỏ. Nguyên nhân: list cột có `overflow-y-auto` cắt phần nút Xong nhanh nhô 15px khỏi card nên Playwright click trúng cột ("intercepts pointer events"); spec `tasks-board` click `getByText(taskTitle)` trùng với toast `Đã chuyển "<title>" sang …` (strict-mode violation); dnd spec đỏ dây chuyền vì dispatch spec timeout trước bước dọn dữ liệu. Sửa: bỏ `overflow-y-auto` (khớp `.col-list` report), click card qua `card(column(...))`.
- Review subagent tìm 1 critical: panel xác nhận phủ footer nhưng Tab vẫn rơi vào "Lưu" đang bị che → có thể lưu thay vì xoá. Sửa bằng `inert` trên `<form>` và footer khi `confirm !== null` (React 19 truyền `inert` thẳng). Ba lỗi vừa: toast mô tả đè lỗi HTML > 20.000 ký tự, `maxLength={200}` chặn toast "Tiêu đề tối đa 200 ký tự", chip "Bỏ hạn" pressed mặc định. Ba lỗi nhỏ: fade `cream-50` vs nền `cream-100`, tên file `quick-done-checkbox.tsx`, thứ tự rail.

## Decision

- Chip hạn nhanh là nút hành động thuần, không `aria-pressed` (report `.quick button` chỉ có hover; "Bỏ hạn" không phải trạng thái).
- Tiêu đề cắt tại 200 ký tự ngay khi nhập/dán kèm toast, giống report dòng 1002, thay vì `maxLength` im lặng. Hằng `TITLE_MAX_LENGTH`/`TITLE_TOO_LONG_MESSAGE` dùng chung schema + modal.
- Tương phản chữ trắng trên avatar (≈2.2–2.9:1) giữ theo spec report; ghi nhận là quyết định thiết kế chờ user.

## Evidence

- `npx vitest run src/features/tasks src/lib/kanban`: 17 file, 195 test pass (3 test mới cho inert, cắt tiêu đề, chip không pressed).
- `make lint-web test-web`: 0 lỗi, 865 pass / 3 skipped. `npm run typecheck` xanh.
- `make e2e-isolated E2E_ARGS="tasks-board"`: 8/8 sau sửa, stack `teka-e2e` đã dọn.
- Review: `plans/reports/code-review-260918-1416-phase-07-design-fidelity-modal-a-board-b.md`.

## Next steps

- Commit phase 7 (chưa commit, 24+ file thay đổi) và push + PR khi user duyệt (SSH cesc1802).
- Nếu muốn, chốt lại tương phản avatar với người thiết kế.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
