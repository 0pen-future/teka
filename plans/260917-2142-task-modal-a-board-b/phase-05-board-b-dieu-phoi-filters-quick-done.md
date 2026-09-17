---
phase: 5
title: "Bảng · Phương án B — lọc nhanh, Xong một cú nhấp, cột tô màu/thu gọn, URL state"
status: pending
priority: P1
effort: "2d"
dependencies: [1, 2, 4]
---

# Phase 5: Bảng · Phương án B — "Bảng điều phối cho người quản lý"

## Context Links

- Report mục "Bảng · Phương án B" (dải lọc, chip giáo viên có số, checkbox Xong + Hoàn tác, cột màu theo giai đoạn, thu gọn, ghi nhớ trạng thái)
- Quyết định D3 (lọc + đếm phía server), D4, D5, D7, D9 (bãi bỏ), D11 (theo report 100%) trong [plan.md](./plan.md)
- `apps/web/src/features/tasks/pages/task-board-page.tsx` (sau Phase 4: không còn segmented/scope; `canViewAll`; `moveAndAnnounce`; `handleDrop`)
- `apps/web/src/features/tasks/api/tasks-api.ts:15-22` (`GetBoardParams`), `hooks/use-task-board.ts` + `hooks/tasks-keys.ts` (`tasksKeys.board(params)`, `keepPreviousData`), `__tests__/tasks-handlers.ts:154-156` (MSW board)
- Report demo Bảng B: nhóm chip `aria-label="Bộ lọc nhanh"`, chip có số, "Của tôi" = `assignee === ME`, chip giáo viên đếm `!done`
- `apps/web/src/features/tasks/hooks/use-tasks-data-source.ts` (`toAppColumn` → `KanbanColumn`; `UpdateColumnVariables{columnId,name?,isDone?}`; `afterTaskIdAt`)
- `apps/web/src/features/tasks/hooks/use-board-dnd.ts` (`onMove(taskId, DropTarget{columnId,index})`)
- `apps/web/src/lib/kanban/types.ts:26-47` (`KanbanColumn`, `KanbanBoard<TTask>` — không generic cột), `state.ts`, `selectors.ts`, `use-kanban.ts`, `README.md`
- `apps/web/src/features/tasks/schemas/task-schemas.ts` (`taskColumnSchema`, `CreateColumnInput`, `UpdateColumnInput`)
- `apps/web/src/features/tasks/components/board-settings-modal.tsx` (`ColumnRow`, `DoneSwitch`)
- `apps/web/src/features/tasks/components/task-column.tsx` (`bg-mint-50` khi `isDone`, else `bg-cream-100`)
- URL state mẫu: `apps/web/src/features/collections/pages/*` dùng `useSearchParams` của `react-router`
- Token màu: `apps/web/src/styles/tokens/colors.css` (sky-50 `#eaf5fb`, sun-100 `#fff4d6`, mint-50)

## Overview

Lớp điều phối đặt lên bảng đã sửa ở Phase 4:

1. **Dải lọc nhanh** (chỉ khi `canViewAll`, thay chỗ segmented đã gỡ ở Phase 4): `Tất cả · Của tôi · Quá hạn · Hôm nay · Chưa giao` + chip theo giáo viên (tên + số việc chưa xong). Một chip trạng thái + một chip giáo viên có thể cùng bật. **Lọc và đếm do server làm** (D3): chip đổi → query board đổi (`filter`/`assignee`), số trên chip = `board.counts`.
2. **Checkbox "Xong"** ở mép trái card (hover/focus/touch) → chuyển sang cột done đầu tiên; toast "Hoàn tác" trả về cột/vị trí cũ.
3. **Cột tô màu theo giai đoạn** từ `column.color` (API Phase 1); picker màu trong Cấu hình cột.
4. **Thu gọn cột** → rail dọc (tên + số); ghi vào URL.
5. **Trạng thái trên URL**: `?filter=overdue&assignee=<id>&collapsed=<id>,<id>` (không có `scope`, D4/D11).

## Key Insights

- **Lọc là dữ liệu, không phải view** (D3): `useTaskBoard({ filter, assignee, today })`
  → `tasksKeys.board(params)` khác key mỗi bộ lọc; `keepPreviousData` giữ bảng cũ
  khi đổi chip nên không nháy. `useKanban` nhận board **đã lọc** từ server; dnd,
  `[`/`]`, `afterTaskIdAt` chạy trên chính board đó — không còn hai danh sách,
  **không cần `toFullIndex`** (D9 bãi bỏ). Server `ListColumnPositions` chèn sau
  hàng xóm hiển thị trong cột đầy đủ, nên thả "sau card B" vẫn đúng dù có card ẩn.
- **Mutation lạc quan trên board lọc**: `applyOptimistic` ghi vào cache của key
  đang active (đúng board đang nhìn); `invalidateBoard` onSettled refetch cả bảng
  lẫn `counts`. Việc vừa move khỏi tập lọc (vd. quick-done khi lọc "Quá hạn") sẽ
  biến mất sau refetch — đúng ý người dùng, toast Hoàn tác vẫn hoạt động (move về).
- **Đếm chip = `board.counts`** (server, toàn bộ tập nhìn thấy, chỉ việc chưa
  xong, không đổi theo `filter`) → không cần `hasMore`/tooltip "50+"; `has_more`
  chỉ còn ý nghĩa cho cột > 50 việc như hiện tại.
- **`KanbanBoard` không generic theo cột** → D7: thêm tham số
  `TColumn extends KanbanColumn = KanbanColumn` ở `KanbanBoard`, `KanbanAction`
  (`columns/upserted`), `kanbanReducer`, `UseKanbanOptions`, `selectors`; mặc
  định giữ tương thích, test lib không đổi.
- **Cột thu gọn không là drop target**: `useDroppable({ disabled: collapsed })`;
  không render `SortableContext` cho cột đó; `getColumnProps` vẫn gọi để lib
  giữ roving focus? → Rail có `role="button"` "Mở rộng cột X"; card trong cột
  thu gọn không render nên `[`/`]` đưa việc vào cột thu gọn vẫn hợp lệ (lib chỉ
  cần id cột).
- Không còn `scope`: người `!canViewAll` bỏ qua `filter`/`assignee` trên URL
  (không gửi lên server, không render dải lọc); server vẫn giới hạn bằng
  `Visibility` nên param lạ cũng vô hại.
- Quick-done cần **cột done đầu tiên theo `order`**; nếu không có cột done →
  ẩn checkbox toàn bảng (và ẩn cả chip "Hoàn thành" logic).
- Tương phản: chữ `ink-500 #5b756c` trên `sun-100 #fff4d6` ≈ 4.6:1, trên
  `sky-50 #eaf5fb` ≈ 4.7:1 (ước lượng; **đo lại bằng script** ở bước 12).

## Requirements

Functional
- Filter: `filter ∈ all|mine|overdue|today|unassigned` (mặc định `all`), `assignee` = teacher_id hoặc rỗng; gửi thẳng lên `GET /tasks/board` cùng `today=localIsoDate(new Date())` (AND phía server, Phase 1). Nhóm chip `role="group" aria-label="Bộ lọc nhanh"` (đúng report); chip đang bật `aria-pressed`; mỗi chip trạng thái hiện số từ `counts` (`Tất cả {all} · Của tôi {mine} · Quá hạn {overdue} · Hôm nay {today} · Chưa giao {unassigned}`). Nút "Xoá lọc" khi có lọc. Dải cuộn ngang trên mobile (`overflow-x-auto`, không wrap).
- Chip giáo viên: từ `counts.by_assignee` (server đã sắp count desc) join `members` để lấy tên (id không có trong `members` → bỏ qua); dạng `Tên · N`; tối đa 8 chip hiển thị, còn lại trong `HvSelect` "Giáo viên khác…".
- Dải lọc chỉ render khi `canViewAll`; người không có `view_all` giữ phụ đề Bảng A (Phase 4), không dải lọc, không gửi `filter`/`assignee`.
- Khi bộ lọc đang bật và cột không có việc: dòng "Không có việc khớp lọc" (khác "cột rỗng" có nút "Thêm việc"); điều kiện = `filter !== "all" || assignee !== ""`.
- Đổi chip khi đang có mutation lạc quan chưa settle: chấp nhận (scope `KANBAN_MUTATION_SCOPE` tuần tự hoá; key mới refetch sau).
- Quick-done: `role="checkbox"` `aria-label="Đánh dấu xong {title}"`, `aria-checked=false`; bọc `CardControlBarrier`; ẩn khi `!canMove`, không có cột done, hoặc `task.completedAt !== null`. Sau khi move thành công: toast `Đã chuyển "X" sang {doneCol}` với `action: Hoàn tác` (6000ms) → `moveTask(taskId, prevColumnId, prevIndex)`; live region cả hai chiều.
- Column color: `taskColumnSchema.color: z.enum(["none","sky","sun","mint"])`; `AppColumn.color`; `TaskColumn` nền theo map `{none: bg-cream-100, sky: bg-sky-50, sun: bg-sun-100, mint: bg-mint-50}`; header cột có chấm màu (nếu ≠ none). Bỏ quy tắc `isDone → bg-mint-50` cứng (migration đã backfill mint).
- Settings modal: mỗi `ColumnRow` thêm `role="radiogroup"` "Màu cột {name}" 4 swatch (`aria-label` "Không màu / Xanh trời / Vàng nắng / Xanh mint") → `updateColumnMutation({columnId, color})`; tạo cột mới nhận `color` (mặc định none; nếu `is_done` bật → mint gợi ý).
- Collapse: nút "Thu gọn cột X" trong header cột (desktop only); rail rộng 44px, chữ dọc (`writing-mode: vertical-rl`), tint theo màu, `aria-expanded=false`; click → mở. Lưu vào `collapsed` param.
- URL: `useSearchParams`; đọc với zod (`boardUrlStateSchema` = `{ filter, assignee, collapsed }`), ghi bằng `setSearchParams(next, { replace: true })`; param mặc định bị lược bỏ để URL sạch. Không giữ `collapsed` trên mobile (mobile hiển thị từng cột).

Non-functional
- Không có selector lọc phía client; chỉ `useMemo` cho danh sách chip giáo viên (`counts.by_assignee` × `members`).
- Không thêm dependency. Test unit cho URL schema, chip giáo viên (join/sort/cap 8); test component cho dải lọc (request có `filter`/`assignee`/`today`, số trên chip), quick-done + hoàn tác (MSW), collapse.

## Architecture

```
url: useBoardUrlState() ─► { filter, assignee, collapsed, set… }   (features/tasks/hooks/use-board-url-state.ts)
page ─► useTaskBoard({ filter, assignee, today }) (chỉ gửi filter/assignee khi canViewAll)
     ─► kanban.board (= board lọc từ server) ─► BoardDesktop/BoardMobile   (dnd, [ ], afterTaskIdAt trên board này)
     ─► board.counts ─► BoardFilterBar chips (số) + assigneeChips(counts.by_assignee, members)   (lib/board-filters.ts)
      └─ handleDrop: target.index ─► moveAndAnnounce (không ánh xạ index)
      └─ handleQuickDone(taskId): prev = {columnId, index}; moveAndAnnounce(taskId, doneColumnId, 0); toast Hoàn tác → moveTask(prev)
components/board-filter-bar.tsx (HvChip) · components/quick-done-checkbox.tsx · components/collapsed-column-rail.tsx
lib/kanban: KanbanBoard<TTask, TColumn = KanbanColumn> (D7)
```

## Related Code Files

Tạo mới
- `hooks/use-board-url-state.ts`, `lib/board-filters.ts` (chỉ `assigneeChips`, `isFiltering`), `components/board-filter-bar.tsx`, `components/quick-done-checkbox.tsx`, `components/collapsed-column-rail.tsx`, `components/column-color-picker.tsx`
- `__tests__/board-filters.test.ts`, `__tests__/use-board-url-state.test.tsx`, `__tests__/board-filter-bar.test.tsx`, `__tests__/quick-done.test.tsx`

Sửa
- `apps/web/src/lib/kanban/{types.ts,state.ts,selectors.ts,use-kanban.ts,README.md}` (generic cột)
- `schemas/task-schemas.ts` (`color`), `hooks/use-tasks-data-source.ts` (`AppColumn`, `toAppColumn`, `UpdateColumnVariables.color`, `CreateColumnVariables.color`, optimistic `columns/upserted` giữ color; nhận `boardParams` để `useTaskBoard`/`invalidateBoard` theo key), `api/tasks-api.ts` (`GetBoardParams` thêm `filter?`, `assignee?`), `hooks/tasks-keys.ts`/`use-task-board.ts` (key gồm filter/assignee/today)
- `components/task-column.tsx` (tint, collapse button, quick-done wiring, "Không có việc khớp lọc" qua prop `filtering`), `board-desktop.tsx` (rail), `board-mobile.tsx` (không collapse), `task-card.tsx` (`onQuickDone?`), `board-settings-modal.tsx` (picker)
- `pages/task-board-page.tsx` (URL state → board params, filter bar khi `canViewAll`, handleQuickDone)
- `__tests__/tasks-handlers.ts` (cột có `color`, PATCH nhận color; board handler áp `filter`/`assignee`/`today` lên fixture và trả `counts` — theo đúng quy tắc Phase 1), `__tests__/task-board-page.test.tsx`, `board-settings-modal.test.tsx`, `use-tasks-data-source.test.tsx`

## Implementation Steps

1. **Lib generic cột** — `types.ts`: `KanbanBoard<TTask extends KanbanTask, TColumn extends KanbanColumn = KanbanColumn>`; lan sang `state.ts` (`KanbanAction`, `kanbanReducer`), `selectors.ts`, `use-kanban.ts` (`UseKanbanOptions`, trả `board: KanbanBoard<TTask, TColumn>`). Chạy `npm run test -- lib/kanban` — không sửa test lib. Cập nhật README mục Types.
2. **Schema + AppColumn** — `taskColumnSchema.color` enum; `AppColumn extends KanbanColumn { color: ColumnColor }` trong `use-tasks-data-source.ts`; `toAppColumn` map; `useKanban<AppTask, AppColumn>`; `UpdateColumnVariables.color?`, `CreateColumnVariables.color?` → payload; optimistic upsert giữ `color`. MSW cột có `color`.
3. **Tint + picker** — `task-column.tsx`: `COLUMN_TINT: Record<ColumnColor, string>`; chấm màu header. `column-color-picker.tsx` (radiogroup 4 swatch 40px, `aria-checked`); nhúng vào `ColumnRow` trong settings modal + form thêm cột. Test settings: chọn swatch → mutation gọi `{columnId, color:"sun"}`.
4. **URL state** — `use-board-url-state.ts`: schema zod `{ filter: z.enum(["all","mine","overdue","today","unassigned"]).default("all"), assignee: z.string().uuid().catch("").default(""), collapsed: z.string().default("") }`; parse `searchParams`; `set(partial)` xoá key có giá trị mặc định; `collapsed` là `Set<ColumnId>` (split `,`). Test với `MemoryRouter` initialEntries (giá trị lạ → mặc định; `assignee` không phải uuid → "").
5. **Board params từ URL** — `tasks-api.ts`: `GetBoardParams = { today: string; filter?: BoardFilterKey; assignee?: string }`, `getBoard` chỉ gắn key có giá trị (không gửi `filter=all`, `assignee=`). Page: `boardParams = { today: localIsoDate(now), ...(canViewAll ? { filter: url.filter, assignee: url.assignee || undefined } : {}) }` → `useTasksDataSource({ boardParams })` → `useTaskBoard(boardParams)` (key `tasksKeys.board(boardParams)`, giữ `keepPreviousData`); `invalidateBoard` dùng prefix `tasksKeys.boards()` để refetch mọi biến thể.
6. **Chip giáo viên** — `lib/board-filters.ts`: `assigneeChips(counts.by_assignee, members)` → `{ id, name, count }[]` (giữ thứ tự server, bỏ id không có trong `members`, tie-break tên), `isFiltering({ filter, assignee })`. Test: join/sort/cap, id lạ bị bỏ, rỗng.
7. **Filter bar** — `board-filter-bar.tsx`: props `{ filter, assignee, counts, teachers: {id,name,count}[], onChange }`; `div role="group" aria-label="Bộ lọc nhanh"`; `HvChip` cho 5 trạng thái với `count` từ `counts` (chip "Quá hạn" `dot="danger"`, "Hôm nay" `dot="warning"`), chip giáo viên `count`; sau 8 chip → `HvSelect` "Giáo viên khác…"; "Xoá lọc" ghost khi `isFiltering`. Render trong page chỉ khi `canViewAll`, dưới phụ đề `Toàn trung tâm · N việc` (Phase 4). Đổi chip → `url.set({ filter })` / `url.set({ assignee })` (toggle: bấm chip đang bật → về mặc định).
8. **Board lọc là board** — không có visible map: `useKanban` và `useBoardDnd` dùng `kanban.board` như hiện tại; `handleDrop` giữ nguyên `target.index`. `TaskColumn` nhận `filtering: boolean` để hiện "Không có việc khớp lọc" thay drop zone "Thêm việc" khi cột rỗng do lọc (vẫn là drop target).
9. **Quick-done** — `quick-done-checkbox.tsx` (button `role="checkbox"`, `CheckIcon`, 40px, `opacity-0 group-hover… [@media(hover:none)]:opacity-100`); `task-card.tsx` prop `onQuickDone?: () => void`, đặt ở mép trái hàng đầu trong `CardControlBarrier`. Page `handleQuickDone(taskId)`: tìm `prevIndex` trong `kanban.tasksByColumn`, `doneColumnId` = cột `isDone` có `order` nhỏ nhất; `moveAndAnnounce(..., 0)`; toast với `action` Hoàn tác → `moveAndAnnounce(taskId, prev.columnId, prev.index)`. Truyền `onQuickDone` chỉ khi `canEdit && doneColumnId && !task.completedAt`.
10. **Collapse** — `collapsed-column-rail.tsx` (44px, vertical text, `aria-expanded=false`, `aria-label="Mở rộng cột X (N việc)"`); `task-column.tsx` nút "Thu gọn cột X" (`ChevronsLeft` icon, `size-10`, chỉ desktop); `board-desktop.tsx` chọn rail/cột theo `collapsed.has(id)`; `useDroppable disabled` khi collapsed. `[`/`]` vào cột thu gọn: announce vẫn chạy (lib), card không hiển thị — chấp nhận, ghi docs.
11. **Tests** — `tasks-handlers.ts`: board handler đọc `filter`/`assignee`/`today`, lọc fixture theo bảng Phase 1, trả `counts` (không đổi theo filter). `task-board-page.test.tsx`: dải lọc chỉ hiện khi `canViewAll` (thay chỗ 2 test segmented đã xoá ở Phase 4); chọn "Quá hạn" → request board có `filter=overdue&today=…` và card không quá hạn biến mất; chip giáo viên hiện `Tên · N` đúng `counts.by_assignee` (kể cả khi fixture cột > 50); chọn chip giáo viên → request có `assignee=`; "Xoá lọc" → URL sạch; người `!canViewAll` có `?filter=overdue` trên URL → request **không** có `filter`; quick-done → MSW move → toast Hoàn tác → click → move về; collapse → URL có `collapsed=`. `quick-done.test.tsx` (a11y role/label). MSW `POST /tasks/:id/move` giữ nguyên.
12. **Tương phản** — script Node nhỏ trong scratchpad tính WCAG ratio cho `#5b756c` trên `#eaf5fb`, `#fff4d6`, mint-50, cream-100 và `#0f1f1a`(ink-900) trên cùng nền; nếu < 4.5 dùng ink-600 cho chữ phụ trên nền đó. Ghi kết quả vào Validation Log của phase.
13. `make lint-web test-web`.

## Todo

- [ ] Lib generic cột (types/state/selectors/use-kanban/README), test lib xanh
- [ ] Schema `color`, `AppColumn`, mutations color, MSW
- [ ] Tint cột + `ColumnColorPicker` trong settings (+ test)
- [ ] `useBoardUrlState` `{ filter, assignee, collapsed }` (+ test)
- [ ] `GetBoardParams` filter/assignee, query key, MSW board áp lọc + `counts`
- [ ] `board-filters.ts` `assigneeChips`/`isFiltering` (+ test)
- [ ] `BoardFilterBar` (`Bộ lọc nhanh`, HvChip có số, chip giáo viên) hiện khi `canViewAll`
- [ ] `TaskColumn.filtering` → "Không có việc khớp lọc"
- [ ] `QuickDoneCheckbox` + Hoàn tác (+ test)
- [ ] Collapse rail + URL (+ test)
- [ ] Đo tương phản; `make lint-web test-web`

## Success Criteria

- Người có `tasks.view_all` thấy phụ đề `Toàn trung tâm · N việc` + dải lọc `Bộ lọc nhanh`; người không có: không dải lọc, không gửi `filter`/`assignee`.
- Lọc "Quá hạn" + chip giáo viên A → request `filter=overdue&assignee=A&today=…`, bảng chỉ còn card quá hạn của A; số trên chip không đổi khi đổi lọc; kéo card trong cột đang lọc thả sau card hiển thị liền trước (server chèn đúng, kiểm bằng e2e Phase 6).
- Reload `?filter=mine` với người có `view_all` → chip "Của tôi" `aria-pressed=true` và bảng chỉ có việc `assignee = tôi` (kể cả việc đã xong ở cột Hoàn thành).
- Checkbox Xong → card sang cột done, toast Hoàn tác trả về đúng cột + đúng index.
- Đổi màu cột trong Cấu hình cột → nền cột đổi ngay (optimistic) và giữ sau refetch.
- Reload URL có `filter=overdue&assignee=<id>&collapsed=<id>` → trạng thái khôi phục.
- Tương phản ≥ 4.5:1 cho chữ phụ trên mọi tint.

## Risk Assessment

- **Generic lib lan rộng** → nếu quá 1 buổi, fallback D7: `color?` optional trong `KanbanColumn` với JSDoc "opaque to lib" (ghi rõ trong README).
- **Mutation lạc quan trên board lọc**: thẻ vừa quick-done khi đang lọc "Quá hạn" hiện ở cột Hoàn thành cho tới khi refetch loại nó; thông báo "vị trí i/n" đếm theo việc hiển thị. Chấp nhận, ghi docs (D9 bãi bỏ).
- **Đổi chip nhanh liên tiếp** tạo nhiều key; `keepPreviousData` + `staleTime` mặc định giữ UI ổn; không prefetch.
- **Hoàn tác sau khi board refetch đổi thứ tự** → `prev.index` có thể lệch; chấp nhận (đưa về gần vị trí cũ), toast 6s.
- **`invalidateBoard` chỉ invalidate key hiện tại** → phải dùng prefix `tasksKeys.boards()` (kiểm `tasks-keys.ts`), nếu không chip số cũ sau mutation.

## Security Considerations

- Quick-done/Hoàn tác đi qua `POST /tasks/:id/move` — server vẫn kiểm `CanMoveTask`; UI ẩn checkbox khi `!canEdit` chỉ là tiện lợi.
- URL param được parse bằng zod, giá trị lạ về mặc định; `assignee` được gửi lên server nhưng server AND với `Visibility` (Phase 1 test i) — người không `view_all` không xem được việc người khác qua URL, và web cũng không gửi param cho họ.

## Next Steps

Phase 6: e2e cho lọc/quick-done/collapse, a11y check, docs, gates, ship.
