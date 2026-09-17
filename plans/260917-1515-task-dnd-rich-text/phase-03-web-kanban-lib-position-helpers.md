---
phase: 3
title: "Web lib: helper vị trí thuần + [ / ] về đầu cột"
status: completed
priority: P2
effort: "0.5d"
dependencies: []
---

# Phase 3: Web lib — helper vị trí thuần + `[`/`]` về đầu cột

## Overview

Thêm vào `src/lib/kanban` các hàm thuần để chuyển "thả ở chỉ số i trong cột"
thành "đứng sau việc nào" và ngược lại, đồng bộ `[`/`]` với hành vi đầu cột
thật của API, và cập nhật README để ghi rõ DnD là chế độ opt-in ở tầng feature.
Lib vẫn chỉ phụ thuộc `react`.

## Requirements

- Functional:
  - `positions.ts` xuất:
    - `resolveDrop<TTask>(board, { taskId, overId, overType })` → `{ columnId, index, afterTaskId: TaskId | null } | null` — quy đổi kết quả `onDragEnd` (over là task hay cột) thành vị trí đích; `index` tính trên danh sách cột đích **đã loại** task đang kéo; thả lên chính mình hoặc `over` không thuộc board → `null`. Quy tắc khớp hình ảnh của `verticalListSortingStrategy` (= `arrayMove(items, activeIndex, overIndex)`): **cùng cột** → `index = overIndexGốc` (chỉ số của `over` trong mảng *chưa* loại moving; kéo xuống thì card đứng *sau* over, kéo lên thì đứng *trước* over); **cột khác** → `index` = vị trí của `over` trong cột đích (chèn trước over).
    - `afterTaskIdAt(tasks, index, movingId)` → `TaskId | null` (index 0 → `null` = đầu cột; index k → id của phần tử k−1 sau khi loại `movingId`).
    - `positionBetween(prev: number | undefined, next: number | undefined)` → số float dùng cho `kanbanReducer` optimistic: cả hai `undefined` → 0; chỉ có `next` → `next − 1`; chỉ có `prev` → `prev + 1`; cả hai → trung bình.
    - `optimisticPositionFor(tasks, index, movingId)` = `positionBetween` áp lên hàng xóm tại `index` (feature chỉ gọi một chỗ).
  - `useKanbanKeyboard`: `[`/`]` gọi `dataSource.moveTask(task.id, targetColumnId, 0)` (đổi từ `targetTasks.length`) — khớp API "không có `after_task_id` = đầu cột" và optimistic hiện tại của feature. Test hiện có assert `targetTasks.length` phải sửa theo.
  - `index.ts` export 4 hàm + kiểu `DropTarget`.
  - README: mục "Non-goal" đổi thành "Pointer drag-and-drop là chế độ **opt-in do app thêm** (Teka: `features/tasks/hooks/use-board-dnd.ts` với dnd-kit); lib chỉ cung cấp helper vị trí thuần và giữ `[`/`]`". Bảng port: ghi rõ `position` của `moveTask` là *chỉ số đích trong cột* (0 = đầu), adapter tự dịch sang hợp đồng API.
- Non-functional: ESLint boundary không đổi; mọi nhánh của `positions.ts` có test; không thay đổi `kanbanReducer`, `selectors.ts`, `useKanban`.

## Architecture

```
onDragEnd(event) ──► resolveDrop(board, {taskId, overId, overType})
                        │ overType "task"   → cùng cột: index = overIndexGốc (arrayMove semantics:
                        │                       [A,B,C] kéo A lên C → [B,C,A]; kéo C lên A → [C,A,B])
                        │                     cột khác: index = vị trí over trong cột đích (chèn trước over)
                        │ overType "column" → cột = overId; index = tasks.length (cuối cột)
                        ▼
              { columnId, index, afterTaskId }
                        │
                        ├─► dataSource.moveTask(taskId, columnId, index)   (adapter dịch index → after_task_id)
                        └─► kanbanReducer(board, {type:"tasks/moved", position: optimisticPositionFor(...)})
```

Lib không biết dnd-kit: `overType` do adapter gán từ `data.current.type` của dnd-kit.

## Related Code Files

- Create: `apps/web/src/lib/kanban/positions.ts`
- Create: `apps/web/src/lib/kanban/__tests__/positions.test.ts`
- Modify: `apps/web/src/lib/kanban/index.ts` (export)
- Modify: `apps/web/src/lib/kanban/use-kanban-keyboard.ts:245` (`targetTasks.length` → `0`; bỏ biến `targetTasks` nếu không còn dùng)
- Modify: `apps/web/src/lib/kanban/__tests__/use-kanban-keyboard.test.tsx` (assert `moveTask(id, col, 0)`)
- Modify: `apps/web/src/lib/kanban/README.md` (Non-goal → opt-in; bảng port; mục mới "Position helpers")
- Modify: `apps/web/src/lib/kanban/data-source.ts` (doc comment `moveTask`: `position` = chỉ số đích)

## Implementation Steps

1. Viết `positions.ts` với JSDoc ngắn nêu bất biến: "index đếm trên cột đích sau khi loại task đang kéo; `afterTaskId === null` nghĩa là đầu cột".
2. Test bảng cho `afterTaskIdAt` (index 0, giữa, cuối, movingId nằm trước/sau index), `positionBetween` (4 nhánh; gap nhỏ vẫn trả trung bình — renormalize là việc của server; position âm hợp lệ và selector vẫn xếp trước 0), `resolveDrop` với kỳ vọng cụ thể: `[A,B,C]` kéo A lên C → `[B,C,A]` (index 2, after C); kéo C lên A → `[C,A,B]` (index 0, after null); kéo A lên B → `[B,A,C]`; over task cột khác → chèn trước over; over cột rỗng → index 0; over cột có việc → cuối; over chính mình → null; over lạ → null. Dùng `arrayMove` tự viết 3 dòng trong test làm oracle cho case cùng cột.
3. Sửa `use-kanban-keyboard.ts` và test tương ứng; chạy `npx vitest run src/lib/kanban`.
4. Cập nhật README + `data-source.ts` doc; `npm run lint` (eslint boundary) + `npm run typecheck`.

## Success Criteria

- [x] `positions.test.ts` phủ mọi nhánh; `use-kanban-keyboard.test.tsx` xanh với `0`.
- [x] `index.ts` export `resolveDrop`, `afterTaskIdAt`, `positionBetween`, `optimisticPositionFor`, `DropTarget`.
- [x] `npm run lint`, `npm run typecheck`, `npm run test -- src/lib/kanban` xanh; không import mới ngoài `react`.
- [x] README mô tả DnD là chế độ opt-in ở app, `[`/`]` giữ nguyên là đường bàn phím.

## Risk Assessment

- **Đổi `[`/`]` từ cuối sang đầu cột** là thay đổi hành vi nhìn thấy với người dùng bàn phím: v1 API đã luôn đặt đầu cột nên UI trước đây chỉ *hứa* cuối cột rồi refetch về đầu — thay đổi này làm lib nói thật. Ghi vào README.
- **`resolveDrop` với `over` là task đang kéo** (dnd-kit bắn `over === active` khi thả tại chỗ) → trả `null`, adapter bỏ qua, không gọi API. Test rõ case này.
- **Lệch 1 khi kéo xuống cùng cột**: nếu dùng "chèn trước over" cho mọi trường hợp thì optimistic nhảy ngược so với hình ảnh lúc kéo và `after_task_id` sai. Đã chốt quy tắc `arrayMove` ở trên; test kỳ vọng ghi rõ mảng kết quả nên không thể khoá nhầm hành vi sai.
