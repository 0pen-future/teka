---
title: "Phase 4 kanban: kéo-thả dnd-kit trong feature tasks"
date: 2026-09-17
summary: dnd-kit adapter useBoardDnd + card sortable giữ role=option; reviewer H1/M1/M2/M3 đã sửa; 766 test xanh
---

# Phase 4 kanban: kéo-thả dnd-kit trong feature tasks

## What happened
- Thêm `@dnd-kit/{core,sortable,utilities}` chỉ trong `apps/web/src/features/tasks`; ESLint chặn `@dnd-kit/*` trong `src/lib/kanban` (lib vẫn headless, chỉ import `react`).
- `hooks/use-board-dnd.ts`: sensors cố định (Mouse distance 6px, Touch delay 250ms), collision `closestCorners` khi cho phép đổi cột (desktop) và `closestCenter` khi không (mobile), onDragEnd map `over` → `resolveDrop` của lib rồi gọi `onMove(taskId, target)`. Announcer tiếng Anh của dnd-kit bị tắt; trang giữ live region tiếng Việt.
- `TaskCard` là `useSortable` **và** `role="option"` của lib: không spread `attributes` (sẽ đè `tabIndex`/`role` và thêm `aria-roledescription`), listeners được đưa qua `getTaskProps(extra)` để lib merge với ref/onKeyDown. `TaskColumn` là droppable + `SortableContext`, có spacer để thả xuống đáy cột. `DragOverlay` render `TaskCardPreview`.
- Move API: mock MSW trả `422` (không phải 400) vì `mapApiError` chỉ coi 422 là validation, khớp `apperror.Invalid` phía API. Toast hiện message field đầu tiên; live region hiện `Không chuyển được việc.`; bảng refetch sau khi settle nên card về đúng thứ tự server.

## Decision
- Reviewer H1: `{announcement || kanban.announcement}` che thông báo `[`/`]` sau lần kéo/menu đầu tiên → tách thành hai live region (trang và lib), có test hồi quy "menu move rồi `]` vẫn được đọc".
- Reviewer M1: dấu thời điểm thả chuyển từ `useEffect(isDragging)` sang `useDndMonitor.onDragEnd` (đóng dấu ngay trong mouseup/touchend, trước click). dnd-kit đã chặn click 50ms sau kéo chuột; cửa sổ 300ms chỉ để bọc đường touch.
- Reviewer M2: `prefers-reduced-motion` đọc một lần trong `useBoardDnd` (trả `reducedMotion`), card bỏ transition và overlay bỏ `dropAnimation` theo cùng cờ.
- Reviewer M3: bỏ claim "optimistic rollback" (mutation chỉ invalidate ở onSettled); đổi tên test thành "reloads the server order".
- Reviewer L7: toast cho lỗi không xác định (500, mất mạng) nay nêu rõ hành động: `kanbanErrorToastMessage(error, fallback)` với fallback "Không chuyển được việc, vui lòng thử lại."; phase file dòng 27 cập nhật cho khớp.
- Giữ `canMove = tasks.edit`; ownership do server chặn 403. AC8 (giữ 250ms trên touch) chỉ có test đơn vị, kiểm thủ công/e2e để Phase 6.

## Next steps
- Phase 5: TipTap editor + RichTextView cho mô tả việc.
- Phase 6: e2e kéo-thả (desktop + mobile hold), docs, ship; nhớ backup DB prod trước migration 000023.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
