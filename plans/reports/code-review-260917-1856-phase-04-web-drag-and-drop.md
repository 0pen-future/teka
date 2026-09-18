---
title: "Code review Phase 4 — Web: kéo-thả kanban bằng dnd-kit"
date: 2026-09-17
reviewer: reviewer-phase4
scope: "diff chưa commit trên master @ b790cc0"
verdict: "Approve with fixes"
---

# Code review — Phase 4: Web drag-and-drop (dnd-kit)

## 1. Phạm vi và cách kiểm chứng

- Plan đối chiếu: `plans/260917-1515-task-dnd-rich-text/phase-04-web-drag-and-drop.md`
- Diff: 18 file sửa + 4 file mới (`use-board-dnd.ts`, `task-card-preview.tsx`,
  `task-card-styles.ts`, `use-board-dnd.test.tsx`), ~650 dòng thêm.
- Cây làm việc **đang thay đổi trong lúc review**: lần chạy đầu
  `task-board-page.test.tsx` còn `console.log("DEBUG toasts")` + `status: 400`
  và fail 1 test; sau đó file được sửa thành `status: 422` và xanh trở lại.
  Mọi kết luận dưới đây tính trên ảnh chụp lúc 19:01–19:10 ngày 2026-09-17.
- Gate đã tự chạy lại:

  | Gate | Kết quả |
  |---|---|
  | `npm run typecheck` | pass |
  | `npm run lint` | 0 error, 6 warning `react-hooks/incompatible-library` (có từ trước) |
  | `npx vitest run src/features/tasks src/lib/kanban` | 10 file / 100 test pass |
  | `npm run build` | pass, chunk `task-board-page` 99.36 kB (31.13 kB gz) |
  | `npx eslint --print-config src/lib/kanban/index.ts` | rule `no-restricted-imports` có group `@dnd-kit/*`, severity error |

- Ba thay đổi team-lead báo sau đó (toast `hvToast(kanbanErrorToastMessage(...))`
  trong `moveAndAnnounce`; mock move đổi 400 → 422 kèm test trang mới; tách
  `task-card-styles.ts`) **đều đã nằm trong bản được review** — so lại snapshot
  sau khi nhận tin: diff không đổi một byte. Xem M3 và L7 cho phần đánh giá
  nhánh lỗi, và §2 cho lý do tách file đã được xác minh.
- Đã đối chiếu hành vi dnd-kit với **mã nguồn upstream tại đúng tag**
  (`@dnd-kit/core@6.3.1`, `@dnd-kit/sortable@10.0.0`) thay vì suy đoán:
  `AbstractPointerSensor.ts`, `TouchSensor.ts`, `useDraggable.ts`,
  `useSortable.ts`, `DragOverlay.tsx`, `DndContext.tsx`.

## 2. Đánh giá tổng quan

Kiến trúc adapter bám sát plan và đúng chỗ: dnd-kit chỉ xuất hiện ở
`features/tasks` (hook + 4 component), `src/lib/kanban` không bị đụng và đã có
rào ESLint thật sự hiệu lực. Hợp đồng a11y của lib được giữ nguyên vì
`attributes` của dnd-kit không bị spread và prop-getter `extra` được dùng đúng
cách (lib merge `ref`, `role="option"` được đặt **sau** spread nên không bị
ghi đè). Mọi đường move (menu, `[`/`]`, thả) hội tụ về
`dataSource.moveTask(taskId, columnId, index)` — đây là điểm mạnh nhất của
phase, vì `after_task_id` và optimistic position chỉ tính ở một chỗ.

Hai nhóm vấn đề đáng sửa trước khi đóng phase: **live region của trang che mất
thông báo `[`/`]`** (hồi quy a11y thực tế, dù cơ chế có sẵn từ trước) và
**cờ chặn click sau khi thả được đặt trong `useEffect` thay vì tại thời điểm
thả** như plan quy định. Phần còn lại là hiệu năng nhỏ và độ phủ test.

Việc tách `task-card-styles.ts` là **bắt buộc chứ không phải scope creep**: đã
xác minh `react-refresh/only-export-components` là severity error cho
`features/tasks/components/**` (`eslint --print-config`, `allowConstantExport:
true`), nên `isOverdue` và `taskCardSurfaceClassName` — hai hàm thường mà
`task-card-preview.tsx` cần dùng chung — không thể export từ file component.

Không tìm thấy lỗi bảo mật hay rò rỉ dữ liệu. Id đi vào `asTaskId/asColumnId`
đều sinh từ DOM do chính app đăng ký, và `resolveDrop` vẫn kiểm tra id có tồn
tại trên board trước khi trả target (`positions.ts:101`, `:116`), nên
`over` lạ hoặc cột không tồn tại đều thành `null` → không gọi API. `canMove`
chỉ là rào UI; server vẫn tự kiểm quyền (`POST /tasks/:id/move` yêu cầu owner /
creator / assignee), nên rào client không phải trust boundary.

## 3. Phát hiện

### Critical

Không có.

### High

**H1. Live region của trang che vĩnh viễn thông báo `[`/`]` sau lần kéo đầu tiên**

`apps/web/src/features/tasks/pages/task-board-page.tsx:212-214`

```tsx
<div aria-live="polite" role="status" className="sr-only">
  {announcement || kanban.announcement}
</div>
```

`announcement` là state của trang, `kanban.announcement` là state của lib.
Toán tử `||` nghĩa là **chỉ cần `announcement` khác rỗng một lần, nhánh
`kanban.announcement` không bao giờ hiển thị lại nữa**. Trước phase 4 chỉ menu
"Chuyển cột" đặt `announcement`; sau phase 4 **mọi thao tác kéo-thả** đều đặt
nó (`moveAndAnnounce` ở dòng 79-96 dùng chung cho cả drag và menu). Kịch bản
hỏng: kéo một thẻ → `announcement` = "Đã chuyển …, vị trí 2/3." → bấm `]` trên
thẻ khác → `kanban.announcement` đổi nhưng text render không đổi → screen
reader **không đọc gì**. AC9 yêu cầu "`[`/`]` vẫn di chuyển việc và focus theo"
kèm thông báo, nên đây là vi phạm acceptance criteria trong luồng thực tế.

Test hiện có không bắt được vì nó bấm `]` trên trang vừa mount, lúc
`announcement` còn rỗng (`task-board-page.test.tsx` case "moves a task with the
`]` shortcut").

Đề xuất sửa (giữ nguyên quyết định "trang sở hữu live region"): cho thông báo
của lib chảy qua cùng một state thay vì hai nguồn song song.

```tsx
useEffect(() => {
  if (kanban.announcement) setAnnouncement(kanban.announcement);
}, [kanban.announcement]);
// ...
<div aria-live="polite" role="status" className="sr-only">{announcement}</div>
```

Kèm test: kéo (hoặc dùng menu) một lần, rồi bấm `]` và assert text mới xuất hiện.

Lưu ý phụ cùng chỗ: hai lần thả liên tiếp cho ra **chuỗi giống hệt nhau** thì
`aria-live` không phát lại (nội dung không đổi). Nếu muốn chắc chắn, thêm bộ
đếm vào state và render chuỗi qua `key`, hoặc chấp nhận và ghi nhận.

### Medium

**M1. Cờ chặn click sau khi thả đặt trong `useEffect`, lệch so với plan bước 4**

`apps/web/src/features/tasks/components/task-card.tsx:114-125`

```tsx
const droppedAtRef = useRef(0);
const wasDraggingRef = useRef(false);
useEffect(() => {
  if (wasDraggingRef.current && !isDragging) droppedAtRef.current = Date.now();
  wasDraggingRef.current = isDragging;
}, [isDragging]);
```

Plan bước 4 ghi rõ: "`justDroppedRef` set ở `onDragEnd`/`onDragCancel` (qua
context hoặc prop)". Bản cài đặt suy ra thời điểm thả từ passive effect. Passive
effect được React lên lịch qua scheduler (MessageChannel macrotask), còn
`click` được trình duyệt đẩy ngay sau `mouseup` — trình duyệt thường ưu tiên
task input hơn message, nên **cờ có thể được đặt sau khi `click` đã chạy**.

Thực tế hiện chưa vỡ, vì đã kiểm chứng từ mã nguồn upstream rằng dnd-kit tự
chặn click trong 50 ms:
`AbstractPointerSensor.ts:183` thêm `documentListeners.add(Click, stopPropagation, {capture:true})`
khi activation constraint đạt, và `:153-159` chỉ gỡ document listener sau
`setTimeout(..., 50)`. React 19 gắn listener ở root container, nằm **dưới**
`document` trong đường capture, nên click bị chặn trước khi tới React.

Nghĩa là `droppedAtRef` hiện chỉ là lưới đỡ cho `click` tổng hợp đến sau 50 ms
trên cảm ứng — đúng mục đích — nhưng là lưới đỡ **không xác định về thời
điểm**. Đề xuất: cho `useBoardDnd` (nơi đã sở hữu `onDragEnd`/`onDragCancel`)
trả thêm một ref/thời điểm thả và truyền xuống card, hoặc đơn giản là đọc
`isDragging` trong một ref cập nhật ngay trong render thay vì effect. Nếu quyết
định giữ nguyên, hãy ghi lý do vào comment (dnd-kit đã che 50 ms, đây chỉ là
lưới cho touch) để người sau không tưởng là bảo vệ chính.

**M2. `useMediaQuery` gọi trong từng `TaskCard`, nằm trên đường re-render 60 fps**

`apps/web/src/features/tasks/components/task-card.tsx:106`

```tsx
const reducedMotion = useMediaQuery("(prefers-reduced-motion: reduce)");
```

`use-media-query.ts` gọi `window.matchMedia(query)` **mỗi lần `getSnapshot`**,
tức mỗi render, cộng một `addEventListener` cho mỗi card. Trong khi kéo,
`DndContext` cập nhật translate theo từng pointer move nên toàn bộ cây
`SortableContext` render lại; với trần 50 việc/cột × 4 cột là ~200 lần
`matchMedia` mỗi khung hình, và 200 listener `change` thường trực.

Đây không phải lỗi đúng/sai, nhưng là hệ số nhân tránh được rẻ tiền: đọc
`reducedMotion` một lần ở `TaskBoardPage` (hoặc trong `useBoardDnd`) và truyền
xuống qua prop / context. Bonus: cùng giá trị đó dùng được cho D1 dưới đây.

**M3. Optimistic move không có rollback thật, comment mô tả sai**

`apps/web/src/features/tasks/pages/task-board-page.tsx:89-94`

```tsx
// The optimistic order is already rolled back by the data source;
```

`moveTaskMutation` chỉ có `onMutate` (áp optimistic) và
`onSettled: invalidateBoard` — không có `onError` khôi phục snapshot
(`use-tasks-data-source.ts:363-387`). Cơ chế phục hồi là refetch, đúng như
`tasks-keys.ts` ghi ("no optimistic snapshot/rollback"). Nếu refetch cũng
hỏng (mất mạng), thẻ **ở lại vị trí sai** trong khi toast báo lỗi.

Hành vi này có từ trước và là quyết định kiến trúc đã ghi, nên không đề nghị
đổi; nhưng comment mới thêm nói sai bản chất, nên sửa thành "data source
invalidate board để refetch dựng lại thứ tự thật". Test
"rolls the card back and toasts when the server rejects a move" cũng nên đổi
tên cho khớp (nó chứng minh refetch, không phải rollback).

### Low

**L1. Bỏ `dropAnimation` khi `prefers-reduced-motion`**

`board-desktop.tsx:66`, `board-mobile.tsx:104`: `<DragOverlay>` dùng
`dropAnimation` mặc định (~250 ms). Phase file **không** yêu cầu tắt nó (chỉ
yêu cầu "tắt transition của sortable", đã làm ở `task-card.tsx:130`), nên đây
là ghi nhận chứ không phải thiếu sót so với plan. Vẫn nên làm vì WCAG 2.3.3:
`DragOverlay` nhận `dropAnimation: DropAnimation | null` (xác nhận ở
`DragOverlay.tsx:23`), nên `dropAnimation={reducedMotion ? null : undefined}`
là đủ — và dùng lại đúng giá trị đã hoisted ở M2.

**L2. `board-mobile.test.tsx` chạy `DndContext` với sensor mặc định**

`__tests__/board-mobile.test.tsx:30-34` truyền `dnd: { contextProps: {} }`.
`DndContext.tsx:143` mặc định `sensors = defaultSensors` = PointerSensor +
KeyboardSensor. Nghĩa là test này dựng **cấu hình sensor khác hẳn production**
(có KeyboardSensor → `listeners` chứa `onKeyDown`, vô tình bị `TaskCard` ghi đè
ngay sau spread). Test vẫn xanh nhưng không chứng minh gì về wiring thật. Đề
xuất: dùng `contextProps` thật từ `useBoardDnd` hoặc ít nhất truyền `sensors: []`.

**L3. Độ phủ AC9 còn thiếu hai assert**

`task-board-page.test.tsx` case "keeps the lib's option semantics…" kiểm
`aria-roledescription` + `aria-describedby` + roving tabindex, nhưng **không**
kiểm `aria-pressed` (AC9 liệt kê) và không chứng minh "live region của trang là
nơi duy nhất có text" (hiện chỉ `getByText`). Thêm:

```ts
expect(option).not.toHaveAttribute("aria-pressed");
// và: mọi node [aria-live] khác node của trang phải rỗng
```

**L4. Handler MSW stateless chưa mô phỏng lỗi `after_task_id`**

`src/test/msw/handlers.ts:1145-1157` chỉ echo `position: after.position + 1`,
không kiểm `after_task_id` có thuộc cột đích không, và có thể tạo position
trùng với một task sẵn có. Handler stateful (`__tests__/tasks-handlers.ts:298-330`)
đã đúng (kể cả 422). Chấp nhận được cho mock stateless, nhưng comment hiện nói
"chỉ quyết định position là top hay ngay dưới" — nên nói thêm rằng nó **không**
kiểm tra hợp lệ, để test nào cần nhánh lỗi biết phải dùng handler stateful.

**L7. Copy toast khi move lỗi lệch so với phase file**

`task-board-page.tsx:89-94` đảo hai chuỗi so với Requirements dòng 27 của phase
file ("lỗi → toast *"Không chuyển được việc."* + invalidate"): toast nhận
`kanbanErrorToastMessage(error)` còn "Không chuyển được việc." đi vào live
region. Với lỗi có `fields` (422) thì bản cài đặt **tốt hơn** plan vì người
dùng sáng mắt thấy đúng nguyên nhân. Nhưng với `kind: "unknown"` (500, mất
mạng — nhánh phổ biến nhất) `kanbanErrorToastMessage` trả
`"Đã xảy ra lỗi, vui lòng thử lại."` (`map-api-error.ts`), tức toast **không
cho biết thao tác nào hỏng** trong khi người dùng vừa kéo một thẻ.

Đề xuất giữ hướng đi hiện tại nhưng ghép ngữ cảnh vào, ví dụ
`hvToast(\`Không chuyển được việc. ${kanbanErrorToastMessage(error)}\`, …)`
hoặc chỉ dùng thông điệp cụ thể khi `kind === "validation"`. Nếu giữ nguyên,
nên ghi vào plan rằng Requirements dòng 27 đã được thay đổi có chủ đích.

**L5. Kéo thẻ không thuộc quyền sở hữu giờ dễ chạm hơn**

Client mở kéo theo `tasks.edit` (`canMove={canEdit}`), còn server chỉ cho owner
center / người tạo / người được giao. Một người có `tasks.edit` nhưng không sở
hữu việc sẽ kéo được, thấy thẻ nhảy, rồi bị 403 → toast "Bạn không có quyền…"
và thẻ quay lại sau refetch. Hành vi này có từ trước với menu "Chuyển cột",
nhưng kéo-thả làm nó dễ gặp hơn nhiều. Không đề nghị đổi trong phase này; ghi
nhận để phase 6 (e2e) hoặc một phase sau quyết định có siết `canMove` theo
ownership hay không.

**L6. `useMemo(contextProps)` không bao giờ trúng**

`use-board-dnd.ts:135-147` memo hoá `contextProps`, nhưng `onMove` là
`handleDrop` — một closure mới mỗi render của `TaskBoardPage`
(`task-board-page.tsx:98`), và repo **không** bật React Compiler (kiểm
`vite.config.ts`: chỉ `@vitejs/plugin-react`). Vậy `onDragEnd` đổi mỗi render →
memo tan mỗi render. Không gây lỗi (DndContext giữ handler qua ref nội bộ), chỉ
là memo trang trí. Sửa: bọc `handleDrop`/`moveAndAnnounce` bằng `useCallback`,
hoặc bỏ `useMemo` và ghi chú lý do.

### Nit

- `docs/frontend-guidelines.md:54`: dòng `inputmode="decimal"` bị mất 2 space
  thụt đầu dòng — thay đổi không liên quan tới phase 4 lọt vào diff. Render
  markdown không đổi và `prettier --check` vẫn xanh, nhưng nên hoàn nguyên để
  diff sạch.
- `resolveDrop` trả `DropTarget.afterTaskId` nhưng `handleDrop` chỉ dùng
  `target.index`, rồi `dataSource.moveTask` tính lại `afterTaskIdAt` từ cache.
  Đây là hệ quả cố ý của quyết định "một đường duy nhất", nhưng đáng một comment
  ở `handleDrop` nói rõ vì sao `afterTaskId` bị bỏ.
- Plan bước 3 nói `onDragOver` "trên mobile nếu `over` thuộc cột khác → bỏ qua";
  bản cài đặt không có nhánh đó. Vô hại vì `BoardMobile` chỉ render một cột nên
  không tồn tại droppable của cột khác trong `DndContext` đó.
- `task-card.tsx`: `style` đặt **sau** spread `getTaskProps(...)`. Đúng cho hiện
  tại (lib không trả `style`), nhưng nếu lib thêm `style` sau này sẽ bị nuốt im
  lặng. Một dòng comment là đủ.
- `role="listbox"` của cột vẫn chứa `<p>Chưa có việc</p>` không phải `option`
  (có từ trước). Spacer mới thì đã `aria-hidden`, đúng.

## 4. Kiểm tra không hồi quy (mục b/c của checklist)

| Hạng mục | Kết luận | Bằng chứng |
|---|---|---|
| Menu "Chuyển cột" | Giữ nguyên, thêm chặn `mousedown`/`touchstart` | `task-card.tsx:160-166`; test "rolls the card back…" đi qua menu |
| Phím `[`/`]` | Còn chạy, nhưng thông báo bị che sau lần kéo đầu (H1) | `use-kanban-keyboard.ts` không đổi; test `]` xanh |
| Mở card bằng click/Enter/Space | Giữ nguyên; click bị chặn 50 ms bởi dnd-kit + 300 ms bởi cờ card | `task-card.tsx:123-128, 146-152` |
| `canMove=false` | Không kéo được: `useDraggable` trả `listeners: undefined` khi `disabled` (xác nhận `useDraggable.ts:114`); menu disabled | test "does not start a drag for a caller without tasks.edit" |
| Board settings | Không đụng tới | `board-settings-modal.tsx` không có trong diff |
| Focus / roving tabindex sau khi thả | Ref của lib được merge, không bị `setNodeRef` ghi đè (`use-kanban-keyboard.ts:246-252` `mergeRefs`); `getTaskProps` đặt `role`/`tabIndex` sau `...extra` | test `]` + focus vẫn xanh |
| Exports `src/lib/kanban/index.ts` | Không đổi | `git diff` không chạm `src/lib/kanban` |
| Prop `TaskCard`/`TaskColumn`/`Board*` | Đổi (thêm `dnd`, `isDropTarget`, `getTaskProps` thành hàm) — **nội bộ feature**, `features/tasks/index.ts` chỉ export type schema + 3 hook nên không phá hợp đồng liên feature | `features/tasks/index.ts` |
| Body `POST /tasks/:id/move` | Khớp server: `after_task_id` Optional/nullable, vắng hoặc `null` = đỉnh cột (`apps/api/.../service.go:282-292`, `dto.go:178`) | `task-schemas.ts:100-107` |

Hai điểm đã kiểm chứng thêm và **không** phải lỗi:

- `touch-action: manipulation` là khuyến nghị chính thức của dnd-kit khi
  TouchSensor dùng `delay` constraint; `TouchSensor.ts` kế thừa
  `AbstractPointerSensor`, vốn hủy drag nếu vượt `tolerance` trong lúc chờ delay
  (`AbstractPointerSensor.ts:136-142`), nên cuộn nhanh vẫn cuộn.
- Áp `transform`/`transition` vô điều kiện trên card nguồn là đúng khi có
  `DragOverlay`: `useSortable.ts:115` tính `shouldDisplaceDragSource =
  !useDragOverlay && isDragging`, và `getTransition()` (`:219-243`) tự trả
  `undefined` ở các nhánh cần thiết.

## 5. Đối chiếu Success Criteria

| Tiêu chí | Trạng thái | Bằng chứng / khoảng trống |
|---|---|---|
| AC7 — kéo trong cột và sang cột khác, API nhận đúng `after_task_id`, thứ tự sau refetch khớp | **Đạt ở mức unit/integration** | `use-board-dnd.test.tsx` (4 case map drop → target); `use-tasks-data-source.test.tsx` "translates a target index…" và "places the task between two neighbours…" assert cả body gửi lẫn thứ tự sau `loadBoard()` qua handler stateful. Xác nhận end-to-end thuộc phase 6. |
| AC8 — mobile nhấn giữ để kéo trong cột; cột khác không nhận thả; menu vẫn chạy | **Đạt một phần** | Rào `allowCrossColumn` có test ("refuses a cross-column drop…"); `collisionDetection` đổi theo `allowCrossColumn` có test. **Chưa có bằng chứng** cho "nhấn giữ 250 ms" (jsdom không mô phỏng được) và plan bước 8 (kiểm tay Chrome Android / tablet ≥768px) chưa có kết quả nào được ghi lại. |
| AC9 — card `role="option"`, không `role="button"`, 1 `tabIndex=0`/cột, không `aria-roledescription`/`aria-describedby`/`aria-pressed`; live region dnd-kit rỗng, live region trang là nơi duy nhất có text; `[`/`]` vẫn chạy | **Chưa đạt trọn** | Phần DOM đạt (test "keeps the lib's option semantics…"), trừ `aria-pressed` chưa assert (L3) và tính duy nhất của live region chưa assert (L3). Phần `[`/`]` **vi phạm trong luồng thực tế** vì H1. |
| `canMove=false` → không kéo được, vẫn mở được bằng click/Enter | **Đạt** | test "does not start a drag for a caller without tasks.edit"; `useDraggable.ts:114` |
| Thả tại chỗ không gọi API | **Đạt** | test "ignores a drop back onto the dragged task itself and a drop outside any target"; `positions.ts:133-135` |
| `npm run lint` chặn `@dnd-kit` trong `src/lib/kanban` | **Đạt** | `eslint --print-config src/lib/kanban/index.ts` → group `@dnd-kit/*`, severity 2 |
| Non-functional: bundle | **Đạt** | chunk `task-board-page` 31.13 kB gz, route đã lazy. Không có số nền trước thay đổi để trừ ra, nên chưa xác nhận được con số "+~19 KB gz" của plan. |

## 6. Hành động đề xuất (theo thứ tự)

1. **H1** — gộp `kanban.announcement` vào `setAnnouncement` (hoặc chọn thông báo
   mới nhất), thêm test: kéo một lần rồi bấm `]` phải đọc được thông báo mới.
2. **M1** — chuyển mốc "vừa thả" sang `onDragEnd`/`onDragCancel` của
   `useBoardDnd`; nếu giữ effect thì ghi rõ trong comment rằng dnd-kit đã che
   50 ms và đây chỉ là lưới cho `click` tổng hợp trên cảm ứng.
3. **M2 + L1** — hoisting `prefers-reduced-motion` lên trang, truyền xuống card
   và dùng luôn cho `dropAnimation={reducedMotion ? null : undefined}`.
4. **M3** — sửa comment và tên test cho đúng bản chất (invalidate + refetch,
   không phải rollback).
5. **L3** — bổ sung assert `aria-pressed` và tính duy nhất của live region.
6. **L7** — quyết định giữ hay ghép ngữ cảnh vào copy toast, rồi đồng bộ với
   Requirements dòng 27 của phase file.
7. **L2, L6, nit `docs/frontend-guidelines.md:54`** — dọn khi tiện.
7. Ghi lại kết quả **kiểm tay bước 8** (Chrome desktop, Chrome Android, tablet
   cảm ứng ≥768px) vào plan/journal trước khi đóng phase — đây là bằng chứng
   duy nhất hiện còn thiếu cho AC8.

## 7. Kết luận

**Approve with fixes.** Không có lỗi Critical, kiến trúc và hợp đồng công khai
đều đúng, gate xanh. Cần sửa H1 trước khi đóng phase vì nó vi phạm AC9 trong
luồng sử dụng thật; M1–M3 nên sửa cùng lượt vì rẻ và cùng chạm các file đã mở.

## 8. Câu hỏi còn treo

1. Có chấp nhận đóng AC8 khi chưa có bằng chứng kiểm tay trên thiết bị cảm ứng
   thật, hay để phase 6 (e2e với `Input.dispatchTouchEvent`) làm gate?
2. `canMove` nên tiếp tục bám `tasks.edit` (L5) hay siết theo ownership để người
   dùng không kéo được thứ chắc chắn bị 403?
3. Ai đang sửa `apps/web/src/features/tasks/__tests__/task-board-page.test.tsx`
   song song? Cần chốt trạng thái file trước khi commit phase.
