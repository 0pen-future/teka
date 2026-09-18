---
title: "Phase 3 kanban: helper vị trí thuần trong lib web, [ / ] lên đầu cột"
date: 2026-09-17
summary: "positions.ts (resolveDrop/afterTaskIdAt/positionBetween/optimisticPositionFor) theo arrayMove; [ ] về đầu cột; reviewer M1 clamp và M2 lọc no-op đã sửa"
---

# Phase 3 kanban: helper vị trí thuần trong lib web, [ / ] lên đầu cột

## What happened
- Thêm `apps/web/src/lib/kanban/positions.ts`: `resolveDrop` quy đổi kết quả drag (over task/cột) thành `{columnId, index, afterTaskId}` theo đúng hình ảnh `arrayMove` của sortable list; `afterTaskIdAt`, `positionBetween`, `optimisticPositionFor` để adapter dịch sang `after_task_id` của API và tính position optimistic. Lib vẫn chỉ phụ thuộc `react`.
- `[`/`]` trong `useKanbanKeyboard` đổi từ cuối cột sang `moveTask(id, col, 0)` cho khớp API (không có `after_task_id` = đầu cột) và optimistic hiện tại của feature.
- README lib: DnD là chế độ opt-in ở app, bảng port ghi `position` là chỉ số đích, mục mới "Position helpers"; `docs/architecture.md` bổ sung một cụm từ.
- Test `positions.test.ts` dùng oracle `arrayMove` 3 dòng duyệt mọi cặp (from, to) cùng cột; board test cố tình để `board.tasks` lệch thứ tự position để chứng minh helper đi qua danh sách đã sort.

## Decision
- Reviewer M1: `optimisticPositionFor` clamp `index` như `afterTaskIdAt` để hai helper luôn mô tả cùng một slot (tránh optimistic nhảy lên đầu cột khi caller truyền index đếm trên danh sách chưa loại task).
- Reviewer M2: lọc drop no-op ngay trong `resolveDrop` (trả `null` khi task thả về đúng chỗ đang đứng, ví dụ task cuối cột thả lên chính cột) thay vì đẩy cho adapter — `null` đã có nghĩa "bỏ qua" nên hợp đồng gọn hơn.
- Comment cũ "API always places at the top" và việc nuốt `position` trong `use-tasks-data-source.ts` để Phase 4 sửa cùng adapter.

## Next steps
- Phase 4: `use-board-dnd.ts` với dnd-kit, adapter dịch `index` → `after_task_id`, cập nhật test feature `tasks`.
- Master đang đi trước origin nhiều commit, chưa push (cần user duyệt).

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
