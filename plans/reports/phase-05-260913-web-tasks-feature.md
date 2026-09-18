# Báo cáo Phase 5: Web tasks feature

## Tổng quan

Đã implement đầy đủ `apps/web/src/features/tasks` theo phase file, nối
`src/lib/kanban` (Phase 4, không sửa) với API thật (Phase 3, chỉ đọc contract
tại `plans/260913-1102-task-center-kanban/reports/api-contract-v1.md`, không
đọc code Go).

## Files tạo mới

Toàn bộ nằm dưới `apps/web/src/features/tasks/` (đúng danh sách file ownership
của phase, 19 file production + 5 file test, ~3150 dòng):

- `api/tasks-api.ts`, `api/task-columns-api.ts`
- `schemas/task-schemas.ts`
- `hooks/tasks-keys.ts`, `hooks/use-task-board.ts`,
  `hooks/use-tasks-data-source.ts`, `hooks/use-member-directory.ts`
- `lib/map-api-error.ts`
- `pages/task-board-page.tsx`
- `components/board-desktop.tsx`, `components/board-mobile.tsx`,
  `components/task-column.tsx`, `components/task-card.tsx`,
  `components/move-menu.tsx`, `components/task-form-modal.tsx`,
  `components/board-settings-modal.tsx`, `components/delete-column-dialog.tsx`
- `routes.tsx`, `index.ts`
- `__tests__/tasks-handlers.ts`, `__tests__/task-board-page.test.tsx`,
  `__tests__/board-settings-modal.test.tsx`,
  `__tests__/use-tasks-data-source.test.tsx`, `__tests__/board-mobile.test.tsx`

## Files sửa

- `apps/web/src/app/router.tsx` — mount `tasksRoutes`.
- `apps/web/src/layouts/dashboard-layout.tsx` + test — NavEntry "Công việc"
  `/tasks` (`perm: "tasks.list"`), đưa vào `OVERFLOW_LABELS`/
  `OVERFLOW_PATH_PREFIXES` để giữ nguyên 3 tab chính mobile; test thêm case
  "Công việc" xuất hiện/vắng trong sheet "Thêm" theo quyền.
- `apps/web/src/test/msw/handlers.ts` — handler mặc định `GET /tasks/board`,
  `GET /task-columns`, `GET` member directory, cộng 3 task fixture + 3 cột mặc
  định. Sửa thêm lần này: tách interface `DefaultTaskFixture` (`due_on`,
  `completed_at`: `string | null`) để `defaultTasksById: Record<string,
  DefaultTaskFixture>` type-check — bản gốc suy ra type object từ
  `defaultTask1` (có `due_on` là literal string), khiến `defaultTask2` (có
  `due_on: null`) không gán được.
- `apps/web/src/features/center/api/center-api.ts` +
  `schemas/center-schemas.ts` — thêm `getMemberDirectory`.

## Sai lệch so với phase file / contract

1. **Cache shape**: `tasksKeys.board(params)` lưu `BoardQueryData {scope,
   board}` thay vì `BoardResponse` thô, để mutation biết `scope` đang active
   khi build lại board sau optimistic update — không đổi hợp đồng API, chỉ là
   chi tiết cache nội bộ.
2. **`KanbanError` là reject-value, không phải `Error`**: theo đúng kiểu của
   `src/lib/kanban`, mọi mutation `throw`/reject một union object
   (`{kind: "conflict"}`, `{kind: "validation", fields}`, …) làm `TError`
   generic của TanStack Query, không phải `Error` instance. ESLint có 2 rule
   phản đối việc này (`prefer-promise-reject-errors`, `only-throw-error`); đã
   giữ nguyên kiến trúc (vì nó khớp type `KanbanDataSource` của lib đóng băng ở
   Phase 4) và tắt `@typescript-eslint/only-throw-error` có phạm vi file kèm
   comment giải thích, thay vì đổi cách lib được thiết kế để throw lỗi.
3. **409 vs 422 cho xoá cột / reorder** — theo `api-contract-v1.md`: xoá cột
   còn việc mà thiếu `move_to`, hoặc xoá cột cuối cùng, trả **409**
   (`ErrColumnNotEmpty`/`ErrLastColumn`); trùng tên, vượt giới hạn cột, input
   sai, hoán vị không hợp lệ, assignee không phải thành viên đều trả **422 +
   `fields`**. `PUT /task-columns/order` coi cả 409 lẫn 422 là "board đã đổi"
   → invalidate + toast, gộp vào `{kind: "conflict"}` trong
   `use-tasks-data-source.ts`. Đây là điểm hợp đồng cần đội API xác nhận lại
   (xem phần Concerns bên dưới) vì tài liệu contract không nêu rõ ràng 409 áp
   dụng cho reorder hay chỉ cho xoá cột.

## Kết quả gate

Chạy lần lượt tại `apps/web/`:

```
npm run test        # → 93 test file pass, 726 tests pass, 3 skip (pre-existing)
npx prettier --check .   # → All matched files use Prettier code style!
npm run typecheck    # → tsc -b --noEmit, sạch
npx eslint .          # → 7 problems (1 error, 6 warnings)
```

Lỗi eslint duy nhất còn lại:

```
apps/web/src/features/center/__tests__/center-permissions.test.tsx:200:14
  error  @typescript-eslint/non-nullable-type-assertion-style
```

File này nằm ngoài `apps/web/src/features/tasks/**` (phạm vi cấm sửa của
phase này), lỗi có sẵn từ trước khi phase bắt đầu (không do tasks feature gây
ra) — đã xác nhận qua `git status`/`git diff` đầu phiên: file này nằm trong
danh sách "modified" trước khi tôi bắt đầu làm việc, không phải file tôi tạo.
Do đó `make lint-web` (chuỗi `npm run lint && npm run format:check && npm run
typecheck`) dừng ở bước `lint` và báo đỏ toàn cục — nhưng nguyên nhân duy nhất
nằm ngoài phạm vi sở hữu file của phase 5.

6 warning còn lại đều là `Compilation Skipped: Use of incompatible library`
(React Compiler) do `form.watch(...)` của react-hook-form — 1 trong số đó ở
`task-form-modal.tsx:252` (file của phase này), 5 còn lại ở
`score-set-editor-modal.tsx`, `profile-page.tsx`, `class-dialog.tsx`,
`student-dialog.tsx`, `class-settings-page.tsx` (không thuộc phase này, cùng
pattern đã tồn tại từ trước). Đây là warning, không phải error, và
`npm run lint` không set `--max-warnings 0` (không phải nguyên nhân gate đỏ).

**Xác nhận cô lập**: chạy `npx eslint apps/web/src/features/tasks` riêng cho
thư mục của phase → 0 error, 1 warning (React Compiler, đã nêu trên).

## Quan ngại / cần đội API xác nhận

1. **409 cho `PUT /task-columns/order`**: contract v1 nêu rõ 409 cho
   `ErrColumnNotEmpty`/`ErrLastColumn` (xoá cột) và 422 cho hoán vị không hợp
   lệ (reorder), nhưng không loại trừ khả năng backend cũng trả 409 cho
   reorder trong một số trường hợp cạnh tranh (ví dụ cột bị xoá bởi request
   khác ngay trước khi reorder tới). Vì cả hai trường hợp UI xử lý giống nhau
   ("bảng đã đổi" → invalidate + toast), đây không phải blocker, nhưng nếu
   thực tế reorder không bao giờ trả 409, dòng gộp `mapped.kind === "validation"
   ? {kind:"conflict"} : mapped` trong `use-tasks-data-source.ts` có thể đơn
   giản hoá lại sau — không cần đổi hành vi ngay.
2. **`center-permissions.test.tsx:200:14`** chặn `make lint-web` xanh toàn
   cục — không thuộc phạm vi phase 5, cần phase/owner khác (hoặc final
   cleanup) sửa 1 dòng (`as X` → `X!`).

## Success Criteria (đối chiếu)

- [x] Board render đúng N cột theo position; thẻ nhóm đúng cột.
- [x] Không có `tasks.view_all`: ẩn segmented "Toàn trung tâm"; có quyền:
      chuyển được, board đổi dữ liệu.
- [x] Không có `tasks.manage_board`: ẩn "Cấu hình cột"; `/tasks` trực tiếp vẫn
      xem được board.
- [x] Không có `tasks.list`: nav ẩn và `/tasks` redirect (`<Navigate to="/" />`).
- [x] Assignee không phải creator thấy form chế độ đọc + menu chuyển cột,
      không thấy nút Xoá.
- [x] Xoá cột còn việc: dialog bắt chọn cột đích, 1 request kèm `move_to`.
- [x] Cột `is_done`: nền mint-50 + ✓; thẻ "xong dd/mm".
- [x] Test chứng minh optimistic move áp ngay qua `kanbanReducer`, lỗi (409/
      404/mạng) dẫn tới refetch board.
- [x] Test chứng minh 9 mutation chạy tuần tự (`scope` chung, xem
      `use-tasks-data-source.test.tsx`).
- [x] `dashboard-layout.test.tsx`: 3 tab chính giữ nguyên; "Công việc" trong
      sheet "Thêm".
- [x] `make test-web` xanh; `src/lib/kanban` không bị sửa. `make lint-web`
      đỏ do 1 lỗi có sẵn ngoài phạm vi phase (xem trên).
