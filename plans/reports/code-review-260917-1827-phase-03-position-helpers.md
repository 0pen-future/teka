# Code review — Phase 3: helper vị trí thuần cho `src/lib/kanban`

- Ngày: 2026-09-17
- Phạm vi: `apps/web/src/lib/kanban/{positions.ts, index.ts, data-source.ts, use-kanban-keyboard.ts, README.md}` + `__tests__/{positions.test.ts, use-kanban-keyboard.test.tsx}`
- Kết luận: **Approve có điều kiện** — không có lỗi chặn; 2 điểm Medium nên xử lý trước khi Phase 4 nối adapter dnd-kit.

## Tóm tắt

Thuật toán `resolveDrop` đúng với ngữ nghĩa `arrayMove` đã chốt trong plan, có
test đối chiếu đủ 6 cặp `(from, to)` trong cột và các case biên cột khác / cột
rỗng / no-op. Đổi `[`/`]` sang `moveTask(id, col, 0)` không gây hồi quy ở
feature vì adapter hiện bỏ hẳn tham số `position` và optimistic luôn đặt 0.
Lint, typecheck, prettier, test đều xanh. Vấn đề còn lại nằm ở độ bền của hợp
đồng public khi Phase 4 gọi: `optimisticPositionFor` không clamp index giống
`afterTaskIdAt`, và `resolveDrop` trả DropTarget cho cú thả không đổi gì.

## Kết quả kiểm tra

| Lệnh (chạy tại `apps/web`)        | Kết quả                                                 |
| --------------------------------- | ------------------------------------------------------- |
| `npm run lint`                    | 0 error, 6 warning `react-hooks/incompatible-library` có sẵn trên master |
| `npm run typecheck`               | sạch                                                    |
| `npx prettier --check src/lib/kanban` | sạch (gồm cả README.md)                             |
| `npx vitest run src/lib/kanban`   | 5 file, 55 test pass                                    |

## Findings

### Medium

**M1 — `optimisticPositionFor` không clamp index, lệch với `afterTaskIdAt`**
`apps/web/src/lib/kanban/positions.ts:81` so với `positions.ts:54`.

`afterTaskIdAt` clamp `Math.min(index, remaining.length)`, nên index vượt độ
dài được hiểu là "cuối cột". `optimisticPositionFor` thì không: với
`index = tasks.length` (đếm trên danh sách **chưa** loại task đang kéo — đúng
con số mà `[`/`]` từng gửi, và là cách một adapter dễ tự tính "thả xuống đáy"),
`remaining[index - 1]` và `remaining[index]` đều `undefined` nên hàm trả `0`.
Hệ quả: `after_task_id` gửi lên là task cuối cột (đúng), còn optimistic đặt
`position = 0` (gần đầu cột) — card nhảy sai chỗ cho tới khi refetch. Hai hàm
lại được JSDoc và README mô tả là "cùng một thông tin diễn đạt hai cách", nên
người dùng API không có lý do nghi ngờ.

Đề xuất:

```ts
const remaining = withoutTask(tasks, movingId);
const slot = Math.min(Math.max(index, 0), remaining.length);
return positionBetween(remaining[slot - 1]?.position, remaining[slot]?.position);
```

và thêm test `optimisticPositionFor(columnA, 99, X) === 31`,
`optimisticPositionFor(columnA, 3, A) === 31` (clamp về đáy),
`optimisticPositionFor(columnA, -1, X) === 9`.

**M2 — `resolveDrop` trả DropTarget cho cú thả không thay đổi gì**
`apps/web/src/lib/kanban/positions.ts:109-119`.

Thả một task lên chính cột nó đang đứng, khi nó vốn đã ở cuối cột, cho
`index = remaining.length` và `afterTaskId` = task ngay trên nó, tức đúng vị trí
hiện tại. `resolveDrop` vẫn trả non-null, nên adapter Phase 4 sẽ bắn một request
`POST /tasks/:id/move` + một optimistic write + một refetch board cho mỗi lần
người dùng thả hụt vào vùng trống của cột. Đã kiểm tra `apps/api/pkg/kanban/service.go`
(`MoveTask`, khoảng dòng 305-335): không publish event, nên đây là lãng phí
round-trip chứ không tạo nhiễu audit.

Đề xuất (chọn một, không cần đổi quy tắc đã chốt): hoặc `resolveDrop` trả `null`
khi `columnId === moving.columnId && afterTaskId` trùng hàng xóm hiện tại của
`moving`; hoặc ghi rõ trong JSDoc/README rằng "DropTarget có thể mô tả đúng vị
trí hiện tại — adapter phải tự bỏ qua" để Phase 4 không quên.

### Low

**L1 — nhánh `?? []` không thể xảy ra và nếu xảy ra thì sai im lặng**
`apps/web/src/lib/kanban/positions.ts:129`.

`selectTasksByColumn` (`selectors.ts:31-35`) tạo entry cho cả columnId "stale"
không nằm trong `board.columns`, mà `over` luôn lấy từ `board.tasks`, nên
`tasksByColumn.get(over.columnId)` không bao giờ `undefined`. Nhánh fallback vì
vậy là code chết và không có test — vênh với tiêu chí "mọi nhánh của
`positions.ts` có test". Nếu giả định đó vỡ trong tương lai, `findIndex` trả
`-1` và hàm trả `{ index: -1 }` thay vì `null`. Đề xuất đổi thành guard thật:

```ts
const columnTasks = tasksByColumn.get(over.columnId);
if (!columnTasks) return null;
```

**L2 — cast brand thay vì dùng `asColumnId`**
`apps/web/src/lib/kanban/positions.ts:110,116`. `types.ts` nói rõ `ColumnId`
"construct only via `asColumnId`" nhằm gom cast về một chỗ; ở đây dùng
`overId as ColumnId` hai lần. Đổi sang `asColumnId(overId)` cho nhất quán.

**L3 — README khẳng định một file chưa tồn tại**
`apps/web/src/lib/kanban/README.md:58`: "Teka wires dnd-kit on top in
`src/features/tasks/hooks/use-board-dnd.ts`". File đó chưa có (Phase 4 mới tạo);
`ls apps/web/src/features/tasks/hooks/` hiện chỉ có 4 file khác. Plan có yêu cầu
câu này, nên chỉ cần hạ thì: "sẽ nối dnd-kit ở …" cho tới khi Phase 4 land.

**L4 — xuống dòng phá thụt lề bullet trong README**
`apps/web/src/lib/kanban/README.md:40-41`: đoạn ``dataSource.moveTask(taskId,``
ngắt dòng và dòng sau bắt đầu ở cột 0 (`columnId, 0)`...`). Markdown vẫn render
đúng nhờ lazy continuation và prettier không phàn nàn, nhưng đọc source thì
trông như hết bullet. Nên gộp một dòng hoặc thụt 2 space.

**L5 — comment cũ ở feature sẽ sai sau Phase 4 (ngoài scope Phase 3)**
`apps/web/src/features/tasks/hooks/use-tasks-data-source.ts:362-366` vẫn viết
"The API always places a moved task at the top of its destination column
regardless of the `position` the caller requests"; sau Phase 1 điều này chỉ đúng
khi không gửi `after_task_id`. Cùng file, dòng 422, adapter còn nuốt hẳn tham số
thứ ba: `moveTask: (taskId, columnId) => moveTaskMutation.mutateAsync(...)`.
Ghi lại để Phase 4 sửa đồng bộ, không phải lỗi của Phase 3.

## Đối chiếu Success Criteria

- `positions.test.ts` phủ nhánh: **đạt trừ một nhánh chết** (L1). 55 test xanh.
- `use-kanban-keyboard.test.tsx` assert `0`: đạt (`__tests__/use-kanban-keyboard.test.tsx:100`).
- `index.ts` export 4 hàm + `DropTarget`: đạt (`index.ts:20-21`, có thêm `DropEvent` — hợp lý vì là tham số public của `resolveDrop`).
- Lint/typecheck/test xanh, không import mới ngoài `react`: đạt (`positions.ts` chỉ import `./selectors`, `./types`).
- README mô tả DnD opt-in ở app, giữ `[`/`]` là đường bàn phím: đạt.
- Bảng port ghi `position` là chỉ số đích: đạt (`README.md:126-131`), khớp doc mới ở `data-source.ts:26-32`.
- Mục "Position helpers": đạt (`README.md:178-219`), snippet `onDragEnd` dùng đúng API đang tồn tại (`resolveDrop`, `selectTasksByColumn`, `optimisticPositionFor`, `asTaskId` — tất cả đều được export ở `index.ts`).
- Không có plan ID / phase number trong code: đạt.

## Không hồi quy (kiểm chứng)

- Không caller nào ngoài lib truyền `position` khác `0`: `task-board-page.tsx:101`
  gọi `kanban.moveTask(taskId, columnId, 0)`; `use-tasks-data-source.ts:422` bỏ
  tham số; `__tests__/use-tasks-data-source.test.tsx:170` truyền `0`. Không test
  nào ở `features/tasks` assert `targetTasks.length`.
- Optimistic của feature vốn đã ghi `position: 0` cho mọi move
  (`use-tasks-data-source.ts:366`), nên `[`/`]` giờ mới khớp cả ba: announcement,
  optimistic và board refetch.
- Hợp đồng public `KanbanDataSource.moveTask` giữ nguyên chữ ký, chỉ thêm JSDoc.
  Export mới là re-export thuần từ module không side-effect nên không ảnh hưởng
  tree-shaking; ESLint boundary không đổi.

## Đúng đắn thuật toán (đã soi)

- `arrayMove` trong test (`positions.test.ts:42-46`) dùng đúng idiom
  `next.splice(to, 0, ...next.splice(from, 1))`: splice trong chạy trước, nên
  `to` được tính trên mảng đã bỏ phần tử — giống hệt `arrayMove` của dnd-kit.
  Oracle hợp lệ.
- Cùng cột: `index = overIndexGốc` đúng bằng vị trí của task đang kéo trong mảng
  kết quả `arrayMove`, đã kiểm cả kéo xuống (`A→C` ⇒ index 2, after `C`) lẫn kéo
  lên (`C→A` ⇒ index 0, after `null`).
- Dùng danh sách đã sort: `makeBoard()` cố tình xếp `board.tasks` lệch thứ tự
  position (`C,A,B` và `Y,X`) và test cột B kỳ vọng chèn trước `X` ⇒ chứng minh
  helper đi qua `selectTasksByColumn`, không theo thứ tự mảng thô.
- Cột đích rỗng ⇒ index 0 / after null; thả lên chính cột mình ⇒ không đếm chính
  mình (index 2, after `C`); thả lên chính mình và id lạ ⇒ `null`. Tất cả có test.
- `afterTaskIdAt` với index vượt độ dài ⇒ clamp về cuối, có test cả trường hợp
  cột chỉ còn đúng 1 phần tử là chính task đang kéo (`⇒ null`).
- README nói "đầu cột là nơi move không kèm hàng xóm rơi vào phía server" —
  khớp `apps/api/internal/features/tasks/service.go:282-289`.

## Đề xuất hành động (theo thứ tự)

1. Clamp index trong `optimisticPositionFor` + test biên (M1).
2. Quyết định cách xử lý drop no-op: chặn trong `resolveDrop` hoặc ghi vào hợp
   đồng để Phase 4 chặn ở adapter (M2).
3. Đổi `?? []` thành guard trả `null` và thêm test (L1); dùng `asColumnId` (L2).
4. Sửa thì của câu README về `use-board-dnd.ts` và thụt lề dòng 41 (L3, L4).
5. Ghi L5 vào phase 4 để sửa comment + truyền `position` xuống adapter.

## Câu hỏi còn mở

- M2: muốn `resolveDrop` tự lọc no-op (lib "biết" vị trí hiện tại) hay giữ lib
  thuần hình học và để adapter so sánh? Ảnh hưởng tới số request Phase 4.

## Xử lý findings (controller)

| Finding | Quyết định | Thay đổi |
| --- | --- | --- |
| M1 clamp `optimisticPositionFor` | Sửa | `positions.ts`: clamp `index` về `[0, remaining.length]` trước khi lấy hàng xóm, JSDoc nêu rõ đồng bộ với `afterTaskIdAt`; test mới "clamps index like afterTaskIdAt" (index 3 với C đang kéo → 21, index 99 → 31, index −1 → 9). |
| M2 drop no-op lên chính cột | Sửa ở lib | `resolveDrop` trả `null` khi cột đích trùng cột hiện tại và `index` bằng vị trí hiện tại của task (ví dụ task cuối cột thả lên chính cột). JSDoc + README ghi rõ "null = bỏ qua cả optimistic lẫn request". Test mới "returns null when the last task of a column is dropped on that column". |
| L `?? []` code chết / index −1 | Sửa | `resolveDrop` viết lại một luồng: tính `columnId`/`index`, rồi lấy `columnTasks` một lần; nhánh over-task dùng `indexOf(over)` trên danh sách cột của chính `over` nên không thể ra −1. |
| L `overId as ColumnId` | Sửa | Dùng `asColumnId(overId)`. |
| L README nói `use-board-dnd.ts` đã tồn tại | Sửa | Viết lại thành "lớp dnd-kit của Teka là hook `use-board-dnd` của app dưới `src/features/tasks/hooks/`" (Phase 4 tạo). |
| L README ngắt dòng cột 0 | Sửa | Ngắt lại dòng bullet `[`/`]`. |
| L comment cũ + nuốt `position` ở `use-tasks-data-source.ts` | Hoãn Phase 4 | Đúng phạm vi Phase 4 (adapter dịch `index` → `after_task_id`); phase-04 đã liệt kê file này trong "Modify". |

Sau khi sửa: `npx vitest run src/lib/kanban` 5 file / 57 test pass; `npm run lint` 0 error (6 warning có sẵn); `npm run typecheck` sạch; `npx prettier --check src/lib/kanban` sạch.
