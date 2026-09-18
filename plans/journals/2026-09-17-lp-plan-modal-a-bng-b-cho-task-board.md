---
title: Lập plan Modal A + Bảng B cho task board
date: 2026-09-17
summary: "Plan 6 phase (api, web, migration 000024) từ report UX ui-redesign-260917-2117; các quyết định D1–D11 đã kiểm chứng trong source."
---

# Lập plan Modal A + Bảng B cho task board

## What happened

- Chuyển hai phương án được chọn trong `plans/reports/ui-redesign-260917-2117-task-board-and-task-modal.html` (Modal · A, Bảng · B) thành plan `plans/260917-2142-task-modal-a-board-b/` với 6 phase: API + migration, kit hv, Modal A, sửa lỗi Bảng A, Bảng B, e2e/docs/ship.
- Scout phát hiện ba khoảng trống so với report: `task_columns` chỉ có `is_done` (không tô được cột "Đang làm"/"Chờ duyệt"), không có endpoint khôi phục việc đã soft-delete (`TaskRepository.Get` loại `deleted_at`), và `isOverdue` dùng `toISOString()` (UTC) nên lệch ngày ở VN.
- `ak plan create` tự sinh timestamp `260917-1449` khác timestamp hook `260917-2142`; đổi tên thư mục rồi `ak plan reindex`.
- Kiểm chứng `MoveAllToColumn` (`task_repository.go:238-251`) không lọc `deleted_at` → rủi ro "restore vào cột đã xoá" đóng, chỉ cần test khoá.

## Decision

- D1 `task_columns.color` enum none/sky/sun/mint; core `pkg/kanban` giữ `Color string` mờ, enum ở adapter binding + DB CHECK (lib trung lập sản phẩm).
- D2 `POST /tasks/:id/restore` gate `tasks.delete` + `CanWriteTask`, audit `task.restore`; không có cửa sổ hoàn tác phía server.
- D3/D11 bộ lọc nhanh tính phía client trên board đã cache, giữ segmented phạm vi và thêm dải lọc khi scope hiệu lực là `center`.
- D6 `HvModal.stickyFooter` opt-in thay vì đổi mặc định md/lg; D7 `KanbanBoard` generic theo cột.

## Next steps

- `/ak:cook plans/260917-2142-task-modal-a-board-b/plan.md`, bắt đầu Phase 1 (API) và Phase 2 (kit) song song.
- Câu hỏi mở cho chủ dự án: lọc client vs server, giữ segmented + dải lọc hay thay hẳn, read-only modal có cho đổi cột.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
