---
phase: 4
title: "Web: kéo-thả bằng dnd-kit (adapter opt-in ở feature tasks)"
status: completed
priority: P1
effort: "1.5d"
dependencies: [1, 3]
---

# Phase 4: Web — kéo-thả bằng dnd-kit (adapter opt-in ở feature tasks)

## Overview

Thêm `@dnd-kit/core` + `@dnd-kit/sortable` + `@dnd-kit/utilities` vào feature
`tasks`, bọc bảng trong `DndContext`, cho phép kéo việc trong cột và giữa các
cột (desktop) hoặc trong cột (mobile), gọi `POST /tasks/:id/move` với
`after_task_id`, optimistic qua `kanbanReducer`, giữ nguyên hợp đồng a11y và
`[`/`]`/menu "Chuyển" của lib.

## Requirements

- Functional:
  - Desktop (≥768px): kéo card trong cột để sắp xếp; kéo sang cột khác thả vào giữa hai card hoặc vào vùng trống cuối cột; `DragOverlay` hiển thị bản sao card; cột đích highlight khi hover.
  - Mobile: chỉ sắp xếp trong cột đang xem (`BoardMobile` hiển thị 1 cột); kích hoạt bằng nhấn giữ 250ms, tolerance 5px; chuyển cột vẫn qua menu "Chuyển".
  - Sensor: `MouseSensor{distance:6}` + `TouchSensor{delay:250,tolerance:5}` đăng ký **ở mọi viewport** (không dùng `PointerSensor` cùng `TouchSensor`: trên cảm ứng `pointerdown` bắn trước `touchstart` và dnd-kit chỉ cho một sensor chiếm cử chỉ, nên `TouchSensor` sẽ không bao giờ chạy). `isDesktop` chỉ quyết định có cho thả sang cột khác, không chọn sensor → tablet cảm ứng ≥768px vẫn nhấn giữ. Card `touch-action: manipulation` (không `pan-y`, tránh trình duyệt cướp cử chỉ sau khi delay đã qua).
  - Chỉ người có `tasks.edit` (đã map thành `canMove`) mới kéo được; `useSortable({ disabled: !canMove })`.
  - Thả tại chỗ hoặc ngoài vùng hợp lệ → không gọi API. Thả hợp lệ → `moveTaskMutation({ taskId, columnId, afterTaskId, position })`; optimistic đặt `position = optimisticPositionFor(...)`; lỗi → toast theo loại lỗi (`kanbanErrorToastMessage`: 422 hiện message field, lỗi không xác định → "Không chuyển được việc, vui lòng thử lại."), live region "Không chuyển được việc.", rồi invalidate (mẫu sẵn có).
  - Thông báo a11y: sau khi thả thành công đặt `announcement` "Đã chuyển "{title}" sang cột {name}, vị trí {k}/{n}." vào live region hiện có; làm **rỗng** live region + `screenReaderInstructions` của dnd-kit (`accessibility={{ announcements: {...trả undefined}, screenReaderInstructions: { draggable: "" } }}`). dnd-kit vẫn render node `LiveRegion` + `HiddenText` (không gỡ được), chỉ không có nội dung → test page phải chọn live region của trang theo text/`id`, không assert "chỉ 1 vùng aria-live".
  - Card giữ `role="option"`, `tabIndex` roving, `aria-selected`: **không spread `attributes` của dnd-kit** lên card (dnd-kit destructure với giá trị mặc định nên `role: undefined` vẫn ra `role="button"`, kèm `aria-roledescription`, `aria-describedby`, `aria-pressed`). Card chỉ cần `setNodeRef` + `listeners` + `style`. Truyền chúng qua cơ chế `extra` sẵn có của lib: `getTaskProps(task.id, { ref: setNodeRef, ...listeners })` — lib tự merge ref (`mergeRefs` là hàm private, feature không gọi được; và `taskProps` đã chứa `ref` nên đặt `ref=` riêng sẽ bị ghi đè).
  - Không dùng `KeyboardSensor` (Space/Enter trên option đã có nghĩa "mở việc"; `[`/`]` là đường bàn phím).
  - `prefers-reduced-motion` → tắt transition của sortable.
- Non-functional:
  - dnd-kit chỉ được import trong `features/tasks/**` (thêm rule `no-restricted-imports` cho `src/lib/kanban/**`: `@dnd-kit/*` với thông điệp).
  - Không đổi `src/lib/kanban` ngoài phase 3.
  - Bundle: +~19 KB gz; không cần lazy vì route bảng đã lazy sẵn.

## Architecture

```
TaskBoardPage
 └─ useBoardDnd({ board, tasksByColumn, canMove, isDesktop, onMove })   ← hook adapter duy nhất biết dnd-kit
      │ sensors: MouseSensor{distance:6} + TouchSensor{delay:250,tolerance:5} (mọi viewport; không PointerSensor)
      │ collisionDetection: closestCorners (cross-column) | closestCenter (mobile in-column)
      │ state: activeTaskId, overColumnId
      │ onDragEnd → target = resolveDrop(board, {taskId, overId, overType})
      │           → target && onMove(target)
      ▼
 <DndContext {...dnd.contextProps}>
    BoardDesktop / BoardMobile (nhận thêm `dnd` slot)
      └─ TaskColumn: useDroppable({ id: column.id, data:{type:"column"} }) + SortableContext(items = ids, strategy = verticalListSortingStrategy)
           └─ TaskCard: const { setNodeRef, listeners, transform, transition, isDragging } =
                 useSortable({ id: task.id, data:{type:"task", columnId}, disabled: !canMove })
                 const taskProps = getTaskProps(task.id, { ref: setNodeRef, ...listeners })   // lib merge ref
                 <div style={transform} data-dragging={isDragging} {...taskProps}>   // KHÔNG spread attributes của dnd-kit
    <DragOverlay>{activeTask ? <TaskCardPreview task /> : null}</DragOverlay>
 </DndContext>
```

Data source:

```ts
// use-tasks-data-source.ts
moveTaskMutation.mutationFn: ({ taskId, columnId, afterTaskId }) =>
  moveTaskApi(taskId, { column_id: columnId, after_task_id: afterTaskId ?? null })
onMutate: applyOptimistic({ type: "tasks/moved", taskId, columnId, position })
// Menu "Chuyển" và [ / ] đi cùng đường với index = 0 → afterTaskId = null,
// position = positionBetween(undefined, firstInColumn?.position) — card thực sự nổi lên đầu
// ngay cả khi cột đã có position âm (thay cho 0 cố định hiện nay).
dataSource.moveTask: (taskId, columnId, index) => {
  const tasks = tasksByColumn.get(columnId) ?? [];
  return moveTaskMutation.mutateAsync({ taskId, columnId,
    afterTaskId: afterTaskIdAt(tasks, index, taskId),
    position: optimisticPositionFor(tasks, index, taskId) });
}
```

Mẫu thiết kế: **Adapter** (`use-board-dnd.ts` cô lập dnd-kit; đổi lib sau này = đổi 1 hook + 2 component), **prop-getter với `extra`** (lib gộp ref/listeners của adapter vào props của nó, lib thắng về a11y). Cặp sensor cố định, chỉ phạm vi thả (`isDesktop`) là Strategy.

## Related Code Files

- Modify: `apps/web/package.json` (`@dnd-kit/core@^6.3.1`, `@dnd-kit/sortable@^10.0.0`, `@dnd-kit/utilities@^3.2.2`)
- Modify: `apps/web/eslint.config.js` (cấm `@dnd-kit/*` trong `src/lib/kanban/**`)
- Create: `apps/web/src/features/tasks/hooks/use-board-dnd.ts` (adapter; export `BoardDndHandle` = `{ contextProps, activeTask, overColumnId }`)
- Create: `apps/web/src/features/tasks/components/task-card-preview.tsx` (bản sao tĩnh cho `DragOverlay`, không prop-getter)
- Modify: `apps/web/src/features/tasks/components/task-card.tsx` (`useSortable`; nhận **hàm** `getTaskProps` thay vì props đã tính, gọi với `extra = { ref: setNodeRef, ...listeners }`; style transform; `data-dragging`; `touch-action: manipulation`; cờ `justDroppedRef` để `onOpen` bỏ qua `click` compat đến muộn sau `touchend`; nút menu chặn `onMouseDown` + `onTouchStart` stopPropagation)
- Modify: `apps/web/src/features/tasks/components/task-column.tsx` (`useDroppable` + `SortableContext`; truyền `getTaskProps` xuống card thay vì `taskProps={getTaskProps(task.id)}`; nới kiểu prop thành `(taskId, extra?) => …`; vùng trống cuối cột `min-h` để thả; highlight `data-over`)
- Modify: `apps/web/src/features/tasks/components/board-desktop.tsx`, `board-mobile.tsx` (nhận `dnd` slot, bọc `DndContext`, `DragOverlay`; kiểu `getTaskProps` nới thành `(taskId, extra?) => …` ở `board-desktop.tsx:17`, `board-mobile.tsx:23`)
- Modify: `apps/web/src/features/tasks/pages/task-board-page.tsx` (gọi `useBoardDnd`, `handleDrop`, announcement; sửa comment "no drag-and-drop in v1")
- Modify: `apps/web/src/features/tasks/hooks/use-tasks-data-source.ts` (`moveTaskMutation` biến `{taskId, columnId, afterTaskId, position}`; `dataSource.moveTask` dịch index; bỏ comment "always places at top")
- Modify: `apps/web/src/features/tasks/api/tasks-api.ts` (`moveTask(id, body: { column_id; after_task_id: string | null })`)
- Modify: `apps/web/src/features/tasks/schemas/task-schemas.ts` (`MoveTaskRequest`)
- Modify: `apps/web/src/test/msw/handlers.ts:1144` và `__tests__/tasks-handlers.ts:288` (đọc `after_task_id`; stateful handler chèn đúng vị trí và trả `position` midpoint)
- Create: `apps/web/src/features/tasks/__tests__/use-board-dnd.test.tsx` (gọi trực tiếp `onDragEnd` của hook với event giả — không mô phỏng gesture)
- Modify: `apps/web/src/features/tasks/__tests__/task-board-page.test.tsx` (a11y: card vẫn `role="option"`, không `role="button"`, không `aria-roledescription`/`aria-describedby`/`aria-pressed`, chỉ 1 `tabIndex=0`/cột; live region của trang chọn theo text — `getByRole("status")` hiện tại ở dòng ~133 sẽ gặp 2 node sau khi thêm dnd-kit; `canMove=false` → không listener; `]` rồi focus card đích vẫn chạy — chứng minh ref của lib không bị mất)
- Modify: `apps/web/src/features/tasks/__tests__/use-tasks-data-source.test.tsx` (`moveTask(id, col, 2)` → body `after_task_id` = id phần tử 1; index 0 → `null`; optimistic position; case cũ `moveTask(…, 0)` dòng ~170 và handler riêng dòng ~141 cập nhật kỳ vọng)
- Modify: `apps/web/src/lib/kanban/__tests__/use-kanban-keyboard.test.tsx` (đã đổi ở phase 3 — chỉ xác nhận xanh sau khi data source đổi)
- Modify: `docs/frontend-guidelines.md` (đoạn "Drag-and-drop: dnd-kit chỉ ở feature, lib giữ a11y; thứ tự spread")

## Implementation Steps

1. `npm i @dnd-kit/core@6.3.1 @dnd-kit/sortable@10.0.0 @dnd-kit/utilities@3.2.2` (range `^` trong package.json, lockfile pin exact). Thêm rule eslint cho lib.
2. API/schema/data source: đổi `moveTask` request; `moveTaskMutation` nhận `afterTaskId` + `position`; `dataSource.moveTask` dịch index bằng helper phase 3; menu "Chuyển"/`[`/`]` đi qua cùng đường với `index = 0`. Cập nhật MSW (hai handler) để chèn theo `after_task_id`. Chạy test data source.
3. Viết `use-board-dnd.ts`: `useSensors(useSensor(MouseSensor, {activationConstraint:{distance:6}}), useSensor(TouchSensor, {activationConstraint:{delay:250, tolerance:5}}))` cố định; `onDragStart` set `activeTaskId`; `onDragOver` set `overColumnId` (highlight; trên mobile nếu `over` thuộc cột khác → bỏ qua); `onDragEnd` → `resolveDrop` → `onMove(target)`; `onDragCancel` reset. Trả `contextProps` (`sensors`, `collisionDetection`, `accessibility`, 4 handler) để presenter spread.
4. `task-card.tsx`: `useSortable` với `disabled: !canMove` (khi disabled `listeners` là `undefined`); **bỏ qua `attributes`**; `style = { transform: CSS.Translate.toString(transform), transition: reducedMotion ? undefined : transition, touchAction: "manipulation" }`; `const taskProps = getTaskProps(task.id, { ref: setNodeRef, ...listeners })` rồi `<div style={style} data-dragging={isDragging} {...taskProps}>`; `justDroppedRef` set ở `onDragEnd`/`onDragCancel` (qua context hoặc prop) và xoá sau 300 ms, `onOpen` bỏ qua khi cờ bật. Nút menu: `onMouseDown`/`onTouchStart` stopPropagation (hoặc `setActivatorNodeRef` lên tay nắm riêng nếu xung đột còn).
5. `task-column.tsx`: `useDroppable({ id: column.id, data: { type: "column" } })`; `SortableContext` items = ids của cột; vùng thả cuối cột (`div` `min-h-12`) là con của droppable; highlight qua `data-over={overColumnId === column.id}`.
6. Presenters + page: bọc `DndContext`; `DragOverlay` render `TaskCardPreview`; `handleDrop(target)` gọi `dataSource.moveTask(taskId, target.columnId, target.index)` rồi đặt announcement "vị trí k/n"; giữ `handleMoveTask` cho menu.
7. Tests: `use-board-dnd.test.tsx` (renderHook: `onDragEnd({ active:{id}, over:{id, data:{current:{type}}} })` → `onMove` đúng target; over = active → không gọi; mobile cross-column → không gọi); page test kiểm DOM/a11y như trên; data source test kiểm body.
8. Kiểm tay trên Chrome desktop + Chrome Android (DevTools touch) + tablet ≥768px cảm ứng: nhấn giữ 250ms mới kéo, cuộn dọc bình thường khi chạm nhanh, thả xong không mở modal.
9. Gate: `npm run lint`, `npm run typecheck`, `npm run test`; `npm run build` để xem kích thước chunk.

## Success Criteria

- [x] AC7: kéo card trong cột và sang cột khác trên desktop → API nhận `after_task_id` đúng, thứ tự sau refetch khớp thứ tự đã thả (MSW stateful + e2e phase 6).
- [x] AC8: mobile kéo trong cột bằng nhấn giữ; cột khác không nhận thả; menu "Chuyển" vẫn hoạt động.
- [x] AC9: DOM sau khi thêm DnD: mọi card `role="option"`, không `role="button"`, 1 `tabIndex=0` mỗi cột, không `aria-roledescription`/`aria-describedby`/`aria-pressed`; live region dnd-kit rỗng, live region của trang là nơi duy nhất có text; `[`/`]` vẫn di chuyển việc và focus theo (test lib + page).
- [x] `canMove=false` → không kéo được (không `listeners`), card vẫn mở được bằng click/Enter.
- [x] Thả tại chỗ không gọi API (test hook).
- [x] `npm run lint` chặn `@dnd-kit` trong `src/lib/kanban`.

## Risk Assessment

- **Click vs drag**: `MouseSensor` `distance: 6` để click mở modal không bị nuốt; nút menu "Chuyển" chặn `mousedown`/`touchstart`. dnd-kit chỉ chặn `click` trong 50 ms sau khi thả; `click` compat trên cảm ứng có thể đến muộn hơn → `justDroppedRef`. Tín hiệu vỡ: test page "click mở modal" đỏ hoặc e2e mobile thấy dialog sau khi thả → tăng distance/cửa sổ cờ, không đổi kiến trúc.
- **Không dùng `PointerSensor` cùng `TouchSensor`**: dnd-kit chỉ cho một sensor chiếm cử chỉ, pointer events bắn trước touch → nhấn giữ chết. CDP `Input.dispatchTouchEvent` cũng sinh pointer events nên e2e mobile (phase 6) là gate cho quyết định sensor này.
- **Cuộn ngang bảng desktop khi kéo tới mép**: dnd-kit auto-scroll mặc định bật; kiểm tay; nếu giật → `autoScroll={{ threshold: { x: 0.2, y: 0.2 } }}`.
- **dnd-kit thêm `aria-describedby`/`aria-roledescription`/`aria-pressed` qua `attributes`**: không spread `attributes` nên card không nhận gì; node instructions/live region rỗng vẫn tồn tại trong DOM (không gỡ được) → chấp nhận, test chọn live region của trang theo text.
- **Optimistic của "Chuyển"/`[`/`]`** giờ là `positionBetween(undefined, first?.position)` (có thể âm); `selectors.ts` sort số nên hợp lệ. `deleteColumnMutation.onMutate` vẫn gán `position: 0` cho việc mồ côi (không qua `moveTask`) → có thể nhấp nháy đứng sau việc vừa lên đầu cho tới refetch; để nguyên, `onSettled` sửa lại.
- **Optimistic position lệch với server** (server midpoint khác client midpoint): vô hại vì `onSettled` invalidate và thứ tự tương đối giống nhau; test chỉ assert thứ tự, không assert giá trị position.
- **Cột chạm cap 50 việc (`has_more: true`)**: thả xuống dưới thẻ thứ 50 gửi `after_task_id` = thẻ 50; server đặt midpoint giữa thẻ 50 và việc ẩn kế tiếp → việc nằm **trên** phần bị ẩn, đúng với những gì người dùng thấy. Chấp nhận, không thêm API; nếu muốn "cuối cột thật" cần endpoint/`position: "bottom"` riêng — ngoài phạm vi plan này (ghi nhận từ review Phase 1).
- **dnd-kit legacy không cập nhật từ 2024-12** (rủi ro chấp nhận ở D1): mọi import gom trong `use-board-dnd.ts`, `task-card.tsx`, `task-column.tsx`; nếu vỡ với React major kế tiếp: thay adapter, lib + API không đổi.
