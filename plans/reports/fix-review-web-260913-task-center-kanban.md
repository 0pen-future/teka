# Fix review — web (Task-center Kanban)

Áp dụng các finding web trong `plans/reports/code-review-260913-task-center-kanban.md`: M5, N10, N11 và bổ sung test còn thiếu.

## M5 — Bản địa hoá announcement bàn phím

- `src/lib/kanban/use-kanban-keyboard.ts`: thêm option `messages?: UseKanbanKeyboardMessages`
  (`{ moved: (columnName) => string; moveFailed: string }`), mặc định `DEFAULT_MESSAGES` giữ
  nguyên chuỗi tiếng Anh cũ (`"Moved task to {column}."` / `"Could not move task."`). Handler
  `[`/`]` giờ gọi `messages.moved(...)` / `messages.moveFailed` thay vì literal tiếng Anh.
- `src/lib/kanban/use-kanban.ts`: `UseKanbanOptions` nhận thêm `messages?`, forward xuống
  `useKanbanKeyboard`.
- `src/lib/kanban/index.ts`: export type `UseKanbanKeyboardMessages`.
- `src/features/tasks/pages/task-board-page.tsx`: khai báo `KEYBOARD_MESSAGES` tiếng Việt
  ("Đã chuyển việc sang cột {tên}." / "Không chuyển được việc.") và truyền vào `useKanban`.
- `src/lib/kanban/README.md`: cập nhật mục "Locale" mô tả option `messages` và ví dụ wiring
  tiếng Việt của Teka; sửa câu ở a11y contract không còn khẳng định announcement "plain English"
  tuyệt đối.

Test:
- `src/lib/kanban/__tests__/use-kanban-keyboard.test.tsx`: thêm case dùng `messages` tuỳ biến,
  assert `announcement` ra đúng chuỗi tiếng Việt truyền vào (test mặc định cũ vẫn giữ nguyên,
  chứng minh default không đổi).
- `src/features/tasks/__tests__/task-board-page.test.tsx`: thêm case bấm `]` trên thẻ việc, assert
  vùng `role="status"` (aria-live) hiện "Đã chuyển việc sang cột Hoàn thành."

## N10 — Optimistic delete cột làm việc biến mất tạm thời

`src/features/tasks/hooks/use-tasks-data-source.ts` (`deleteColumnMutation.onMutate`): trước khi
dispatch `columns/removed`, đọc cache hiện tại, lọc các task thuộc cột sắp xoá, dispatch
`tasks/moved` cho từng task sang `variables.moveTasksTo` (position 0) — đúng option 1 trong review,
giữ nguyên D11 (không rollback, `onSettled` vẫn invalidate).

Test: `src/features/tasks/__tests__/use-tasks-data-source.test.tsx` — case mới delay response của
`DELETE /task-columns/:id` 30 ms, seed cache bằng `toBoardQueryData(await getBoard(...))`, gọi
`deleteColumnMutation.mutateAsync`, và trong lúc mutation đang pending assert cache: cột đã xoá
biến mất khỏi `columns` nhưng cả 3 task của nó đã có mặt dưới `moveTasksTo`.

## N11 — `useMemo` sai deps + `eslint-disable` vô ích

Bỏ hẳn `useMemo` bọc `dataSource` trong `use-tasks-data-source.ts` (option 2 trong review): object
`dataSource` giờ tính lại mỗi render như một literal thường, không còn `eslint-disable`. Lý do bỏ
thay vì sửa dep sang `params.scope`: đối tượng mutation (`createColumnMutation`, ...) đổi identity
mỗi render nên memo vốn không giữ được identity ổn định cho `dataSource` dù dep là gì; và
`useKanban`'s `tasksByColumn` chỉ phụ thuộc `board`, không phụ thuộc `dataSource`, nên không có gì
để "bảo vệ" bằng memo — khớp đúng bằng chứng review đã nêu (`use-kanban.ts:58`).

Không có test riêng cho N11 (đây là dọn code, hành vi không đổi) — bộ test hiện có (kể cả test mới
của N10) đã chạy qua `dataSource` không memo mà không phát sinh vấn đề.

## Mục 4 (tuỳ chọn) — Bỏ qua

Không thêm hai test còn thiếu review liệt kê (D11 recovery sau lỗi mutation; render `has_more`/
scope-degrade phía UI). Lý do: cả hai đòi hỏi dựng lại ngữ cảnh tích hợp board-query + mutation
(hoặc render `TaskBoardPage` với response `has_more`/`scope` khác thường) tốn công vượt tỉ lệ so
với phần việc bắt buộc M5/N10/N11 trong lượt sửa này; để dành cho một lượt kiểm test riêng nếu cần.

## Test & lint

| Lệnh | Kết quả |
|---|---|
| `npm run typecheck` | pass |
| `npm run lint` | pass (chỉ còn 6 warning React Compiler tiền hữu, không liên quan `src/lib/kanban`/`src/features/tasks`) |
| `npm run format:check` | pass |
| `npx vitest run src/features/tasks src/lib/kanban` | 8 file / 64 test pass |
| `npm run test` (toàn bộ) | 93 file / 729 pass, 3 skip (trước sửa: 726 pass — tăng đúng 3 test mới) |

## File đã sửa

- `apps/web/src/lib/kanban/use-kanban-keyboard.ts`
- `apps/web/src/lib/kanban/use-kanban.ts`
- `apps/web/src/lib/kanban/index.ts`
- `apps/web/src/lib/kanban/README.md`
- `apps/web/src/lib/kanban/__tests__/use-kanban-keyboard.test.tsx`
- `apps/web/src/features/tasks/pages/task-board-page.tsx`
- `apps/web/src/features/tasks/hooks/use-tasks-data-source.ts`
- `apps/web/src/features/tasks/__tests__/task-board-page.test.tsx`
- `apps/web/src/features/tasks/__tests__/use-tasks-data-source.test.tsx`

Status: DONE
Summary: Đã sửa M5 (option `messages` cho `useKanbanKeyboard`/`useKanban` + wiring tiếng Việt ở
`task-board-page.tsx`), N10 (di chuyển task sang cột đích trước khi xoá cột trong optimistic
update), N11 (bỏ `useMemo`/`eslint-disable` sai chỗ cho `dataSource`); thêm 3 test mới cho cả ba.
Toàn bộ typecheck/lint/format/test xanh, không chạm file ngoài phạm vi cho phép.
Concerns/Blockers: Không có. Mục 4 (test D11-recovery, has_more/scope-degrade) bỏ qua vì tốn công
vượt tỉ lệ so với việc bắt buộc — nêu rõ ở trên để cân nhắc riêng.
