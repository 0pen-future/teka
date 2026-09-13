# Báo cáo hoàn thành Phase 4 — Web headless kanban lib

**Plan:** `plans/260913-1102-task-center-kanban/`
**Phase file:** `plans/260913-1102-task-center-kanban/phase-04-web-headless-kanban-lib.md`
**Trạng thái:** completed (với 1 ghi chú ngoài phạm vi sở hữu file, xem mục
"Gates" bên dưới).

## Tóm tắt

Đã dựng xong `apps/web/src/lib/kanban` — core kanban headless, phụ thuộc duy
nhất là `react`: branded types, reducer thuần, 3 selector, port
`KanbanDataSource` (9 method), `KanbanError` + guard, 2 hook
(`useKanban`, `useKanbanKeyboard`), `index.ts` (public surface), `README.md`,
và 4 file test (34 test, toàn bộ pass). Đã thêm 1 block override
`no-restricted-imports` vào `apps/web/eslint.config.js` theo đúng precedent đã
có trong file (dòng 38–61 trước khi sửa).

## Bước 1 — Verify a11y trước khi code (bắt buộc theo phase)

Đọc trước khi viết bất kỳ prop getter nào:

- W3C WAI-ARIA APG — Listbox pattern:
  https://www.w3.org/WAI/ARIA/apg/patterns/listbox/
- W3C WAI-ARIA APG — Grid pattern (đọc để loại trừ):
  https://www.w3.org/WAI/ARIA/apg/patterns/grid/
- react-aria / React Spectrum — ví dụ drag-and-drop (tìm bản kanban tham
  chiếu): các hook DnD của thư viện nhắm tới drag đầy đủ qua bàn phím
  (pick-up bằng Enter, di chuyển bằng Tab/Arrow, drop bằng Enter, hủy bằng
  Escape) trên nền `role="button"`, không phải cặp `grid`/`gridcell`.

**Kết luận:** đề xuất ban đầu trong plan đứng vững, không cần sửa role. Mỗi
cột `role="listbox"` (+ `aria-label`), mỗi thẻ `role="option"`, roving
tabindex theo model "selection follows focus" của listbox (không dùng grid vì
grid được thiết kế cho dữ liệu dạng bảng 2 chiều đều hàng-cột, không hợp với
các cột kanban cao thấp khác nhau). `[`/`]` chuyển task sang cột kề — đây là
lựa chọn đơn giản hơn so với chế độ DnD-qua-bàn-phím đầy đủ của react-aria (drag
đầy đủ là non-goal tường minh của phase), tương tự kỹ thuật WCAG G219
("nút di chuyển lên/xuống" thay vì kéo-thả). Toàn bộ nội dung và URL đã ghi
vào đầu `apps/web/src/lib/kanban/README.md`.

## Bề mặt API xuất ra (cho tác giả Phase 5)

### `types.ts`
- `ColumnId`, `TaskId`: branded `string & { readonly __brand: ... }`.
- `asColumnId(id: string): ColumnId`, `asTaskId(id: string): TaskId` — điểm
  cast duy nhất, adapter dùng khi parse response.
- `KanbanColumn { id: ColumnId; name: string; order: number; isDone: boolean }`
- `KanbanTask { id: TaskId; columnId: ColumnId; position: number; createdAt?: string }`
- `KanbanBoard<TTask extends KanbanTask> { columns: KanbanColumn[]; tasks: TTask[] }`

### `state.ts`
- `KanbanAction<TTask>`: union 7 nhánh — `board/replaced`, `tasks/moved`,
  `tasks/upserted`, `tasks/removed`, `columns/reordered`, `columns/upserted`,
  `columns/removed`.
- `kanbanReducer<TTask>(board, action): KanbanBoard<TTask>` — **hàm thuần,
  export riêng, không dùng trong `useReducer` ở bất kỳ hook nào.** App tự gọi
  hàm này để tính optimistic state cho query cache (ví dụ trong README).
  `switch` exhaustive, nhánh `default` ép kiểu `never` để TS báo lỗi khi thêm
  action mới mà quên xử lý.

### `selectors.ts`
- `selectTasksByColumn<TTask>(board): Map<ColumnId, TTask[]>` — group + sort
  theo `position`, tie-break `createdAt`; không mutate `board.tasks`; bao gồm
  cả cột rỗng (mảng `[]`).
- `selectColumnCounts<TTask>(board): Map<ColumnId, number>`.
- `type AdjacentDirection = "previous" | "next"`;
  `selectAdjacentColumnId<TTask>(board, columnId, direction): ColumnId | undefined`
  — `undefined` ở biên board hoặc cột không tồn tại.

### `errors.ts`
- `KanbanErrorEntity = "column" | "task"`.
- `KanbanError` union 5 kind: `not-found` (kèm `entity`), `conflict`,
  `forbidden`, `validation` (kèm `fields: Record<string,string>`), `unknown`
  (kèm `cause: unknown`).
- `isKanbanError(value: unknown): value is KanbanError` — type guard, không
  map từ HTTP status (việc đó thuộc adapter).

### `data-source.ts`
- `KanbanDataSource<TTask extends KanbanTask>` — 9 method, **cú pháp
  function-property** (`loadBoard: () => Promise<...>`, không phải method
  shorthand) để tránh implicit `this` và tránh
  `@typescript-eslint/unbound-method` khi test spy trực tiếp trên method:
  `loadBoard`, `createColumn`, `updateColumn`, `reorderColumns`,
  `deleteColumn`, `createTask`, `updateTask`, `moveTask`, `deleteTask`.
  Mọi method trả `Promise`, reject bằng (hoặc được adapter map thành)
  `KanbanError`.

### `use-kanban.ts` — hook chính
- `useKanban<TTask>({ board, dataSource }): UseKanbanResult<TTask>` với
  `UseKanbanResult = { board, tasksByColumn, moveTask, reorderColumn,
  getColumnProps, getTaskProps, activeTaskId, announcement }`.
- `board` là **prop**, trả lại y nguyên (cùng reference) — hook không copy,
  không `useReducer`. Có test chứng minh (`use-kanban.test.tsx`): gọi
  `moveTask` xong, `board` không đổi cho tới khi prop `board` tự đổi.
- `moveTask(taskId, columnId, position)` gọi thẳng `dataSource.moveTask`.
- `reorderColumn(columnId, direction)` hoán đổi cột với hàng xóm rồi gọi
  `dataSource.reorderColumns`; no-op ở biên hoặc cột không tồn tại.
- `getColumnProps(columnId, extra?)` → `{ role: "listbox", "aria-label",
  onKeyDown, ...extra }`; `getTaskProps(taskId, extra?)` → `{ role: "option",
  tabIndex, "aria-selected", ref, onKeyDown?, ...extra }`.

### `use-kanban-keyboard.ts` — roving tabindex + phím tắt (dùng nội bộ bởi `useKanban`, cũng export để dùng độc lập)
- `useKanbanKeyboard<TTask>({ board, dataSource: Pick<KanbanDataSource<TTask>, "moveTask"> }): UseKanbanKeyboardResult`
  (`UseKanbanKeyboardResult = { activeTaskId, getColumnProps, getTaskProps,
  announcement }`).
- ArrowUp/ArrowDown: di chuyển focus trong cột; Home/End: nhảy đầu/cuối cột;
  `[`/`]`: gọi `dataSource.moveTask` sang cột kề, no-op ở biên board.
- Logic bàn phím thật nằm ở `onKeyDown` **cấp cột** (event bubbling từ thẻ
  đang focus lên cột); `onKeyDown` ở `getTaskProps` chỉ là passthrough cho
  `extra` của app — tránh bắn 2 lần nếu app gắn cả hai.
- Sau khi `[`/`]` resolve, hook **không tự set focus ngay** (vì `board` app
  truyền vào lúc đó vẫn là board cũ) — dùng `pendingMoveFocus` ref + một
  `useEffect([board, focusTask])` chỉ gọi focus thật khi `board` mới đã phản
  ánh đúng task nằm ở cột đích. Đây là chỗ khó nhất của phase — có test riêng
  (`"keeps focus on the moved task once the app feeds back the updated
  board"`) xác nhận focus không bị mất khi DOM remount sang cột mới.

### `index.ts`
Export toàn bộ type/hàm public liệt kê trên; không export hàm/kiểu nội bộ
(`callAllHandlers`, `mergeRefs`, `resolveFocusedTaskId`, v.v. không lộ ra
ngoài).

## Sai khác so với đặc tả chữ nghĩa của phase file (đã cân nhắc, có lý do)

1. **`announcement` trong return của `useKanban`.** Danh sách return-shape ở
   dòng 37–40 của phase file liệt kê `{ board, tasksByColumn, moveTask,
   reorderColumn, getColumnProps, getTaskProps, activeTaskId }` — không có
   `announcement`. Nhưng phần kiến trúc (dòng 40: "hook chỉ giữ state UI phù
   du (`activeTaskId`, `announcement`)") và message giao việc gốc của
   team-lead đều liệt kê `announcement`. Đã chọn **giữ `announcement`** trong
   return của `useKanban` (không chỉ ở `useKanbanKeyboard`) vì app cần giá
   trị này để đổ vào live region của chính nó (bước 8 của Implementation
   Steps: "trả `announcement` string để app đặt vào region của mình") — nếu
   chỉ có ở `useKanbanKeyboard`, app dùng `useKanban` (API chính) sẽ không có
   đường lấy announcement nào khác ngoài tự gọi thêm
   `useKanbanKeyboard` song song, dẫn tới 2 instance state focus riêng biệt
   (bug tiềm ẩn).
2. **`ref` bắt buộc trong `getTaskProps`.** Không có trong danh sách prop
   getter tường minh của phase file, nhưng là yêu cầu a11y bắt buộc: đổi
   `tabIndex` không tự di chuyển focus DOM thật. Không có `ref` để lib tự gọi
   `.focus()` thì roving tabindex sẽ đúng về mặt thuộc tính nhưng sai về mặt
   hành vi bàn phím thật (Tab/Arrow sẽ không đưa focus tới đúng ô).
3. **Cú pháp function-property cho `KanbanDataSource`** (thay vì method
   shorthand) — quyết định kỹ thuật để tránh
   `@typescript-eslint/unbound-method` khi test spy trực tiếp method; không
   đổi hợp đồng gọi (`dataSource.moveTask(...)` vẫn gọi y hệt).
4. **Nội dung `announcement` là tiếng Anh tối giản** (`"Moved task to
   Doing."`, `"Could not move task."`), không phải tiếng Việt — theo đúng
   yêu cầu giao việc: lib không được chứa chuỗi UI tiếng Việt đặc thù app;
   nếu cần bản dịch, app tự format lại chuỗi ở tầng feature.

Không có sai khác nào về hình dạng `KanbanAction`, `KanbanDataSource` (9
method), `KanbanError`, hay hành vi reducer/selector so với đặc tả.

## Gates đã chạy

| Lệnh | Kết quả |
|---|---|
| `npx eslint src/lib/kanban` | Sạch, 0 lỗi |
| `npx vitest run src/lib/kanban` | 34/34 pass |
| `npx tsc -b --noEmit` (toàn `apps/web`) | Sạch, 0 lỗi |
| `grep -rn "@/features\|@/components\|@/lib/api\|@tanstack" src/lib/kanban` | 0 hit |
| Kiểm chứng ESLint boundary (thêm tạm `import { HvButton } from "@/components/hv"` vào `types.ts`, chạy `npx eslint`, thấy lỗi đúng message, rồi revert) | Đúng như kỳ vọng — lỗi xuất hiện, đã revert sạch |
| `make lint-web` (root) | **Xanh** (chỉ có 5 warning React Compiler tiền tồn tại, không liên quan tới lib, không có lỗi) |
| `make test-web` (root) | **Đỏ** — 693 pass / 1 fail / 3 skip. Test fail duy nhất: `src/features/center/__tests__/center-schemas.test.ts > buildCatalogTabs > pins one tab per business resource and the exact admin fold`, lệch vì danh sách tab thực tế có thêm nhãn `"tasks"` không nằm trong danh sách kỳ vọng cứng của test. Nguyên nhân: `apps/web/src/test/msw/handlers.ts` đã bị sửa (+40 dòng) **trước khi phase 4 bắt đầu** — nằm ngoài file ownership của phase này (`src/lib/kanban/**`, `eslint.config.js`). Đã xác nhận: file/test này và mọi file trong `src/features/center` không bị phase 4 đụng tới; đây là việc của phase/agent khác trong cùng plan (rất có thể phase wiring catalog task-center) chưa cập nhật fixture kỳ vọng của test. **Không tự sửa** theo đúng nguyên tắc file ownership. |

Tóm lại: mọi thứ **trong phạm vi sở hữu file của phase 4** đều xanh tuyệt
đối. `make test-web` đỏ do 1 test ở module khác, không phải do
`src/lib/kanban`.

## Success Criteria (đối chiếu)

Đã tick tất cả 7 mục trong phase file, kèm ghi chú rõ ràng ở mục
`make test-web` về test-fail không liên quan nêu trên.

## Vấn đề chưa giải quyết / cần chuyển tiếp

- **Cần một agent khác (chủ của `src/test/msw/handlers.ts` /
  `src/features/center`) cập nhật lại kỳ vọng của
  `center-schemas.test.ts`** để `make test-web` xanh toàn repo. Đây không
  phải việc của phase 4 và tôi không sửa để tránh vi phạm ranh giới file.
- Không có câu hỏi mở nào khác về hợp đồng API của lib — Phase 5 có thể bắt
  đầu implement `KanbanDataSource` adapter và JSX presenter dựa trên
  `index.ts` + `README.md` đã có.

Status: DONE_WITH_CONCERNS
Summary: Đã hoàn thành toàn bộ `apps/web/src/lib/kanban` (types, reducer thuần, selectors, port, errors, 2 hook, index, README, 4 file test — 34/34 pass) và block ESLint boundary; verify a11y theo W3C APG Listbox trước khi code như yêu cầu; `npx tsc -b`, `npx eslint src/lib/kanban`, và `make lint-web` đều sạch.
Concerns/Blockers: `make test-web` toàn repo đỏ vì 1 test không liên quan trong `src/features/center/__tests__/center-schemas.test.ts` (lệch do `src/test/msw/handlers.ts` bị sửa bởi việc khác ngoài phạm vi sở hữu file của phase 4) — cần agent sở hữu file đó sửa, không phải blocker của phase 4.
