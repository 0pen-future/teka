---
phase: 4
title: "Web headless kanban lib"
status: completed
priority: P1
effort: "1d"
dependencies: []
---

# Phase 4: Web headless kanban lib (`apps/web/src/lib/kanban`)

## Overview

Lõi board phía web, headless hoàn toàn: types branded, reducer thuần, selectors,
`useKanban`, `useKanbanKeyboard`, prop getters, port `KanbanDataSource`, và
`KanbanError`. Không JSX, không CSS, không design system, không TanStack, không
`@/lib/api`. Phụ thuộc duy nhất: `react`.

Phase này độc lập với Phase 1–3 → chạy song song được. File ownership riêng
biệt: chỉ `src/lib/kanban/**` và một block override trong `eslint.config.js`.

## Requirements

### Functional

- Types: `ColumnId`, `TaskId` branded string; `KanbanColumn{id, name, order,
  isDone}`; `KanbanTask{id, columnId, position}` + generic `TTask extends
  KanbanTask`; `KanbanBoard<TTask>`.
- Reducer thuần với discriminated-union action: `board/replaced`,
  `tasks/moved`, `tasks/upserted`, `tasks/removed`, `columns/reordered`,
  `columns/upserted`, `columns/removed`. Reducer là **hàm tiện ích export**
  cho adapter tính optimistic state; hook **không** giữ board trong
  `useReducer` *(Red-team: hai nguồn sự thật board — reducer trong hook và
  query cache — sẽ lệch khi refetch)*.
- Selectors: `selectTasksByColumn` (group + sort theo `position`, tie-break
  `createdAt`), `selectColumnCounts`, `selectAdjacentColumnId(columnId, dir)`.
- `useKanban({ board, dataSource })` trả `{ board, tasksByColumn, moveTask,
  reorderColumn, getColumnProps, getTaskProps, activeTaskId }`. `board` là
  **prop** do app đưa vào (từ cache của app) và được trả lại nguyên; hook chỉ
  giữ state UI phù du (`activeTaskId`, `announcement`).
- `useKanbanKeyboard`: roving tabindex trong cột, ArrowUp/Down di chuyển focus,
  Home/End đầu/cuối cột, `[` / `]` chuyển task sang cột kề.
- Port `KanbanDataSource<TTask>` với 9 method (load board, 4 column command,
  4 task command) — mỗi method là một intent rời rạc (Command).
- `KanbanError` union: `{kind:'not-found', entity}`, `{kind:'conflict'}`,
  `{kind:'forbidden'}`, `{kind:'validation', fields}`, `{kind:'unknown', cause}`.

### Non-functional

- Zero import ngoài `react` — ESLint chặn `@/features/*`, `@/components/*`,
  `@/lib/api*`, `@tanstack/*`, `zod`, `axios`.
- Reducer và selectors test được không cần render.
- Prop getters trả object thuần, merge được handler của app (gọi handler của
  app trước, không nuốt `event.defaultPrevented`).
- Không quyết định layout: hook không biết desktop/mobile.

## Architecture

**Hook-only, không compound component.** Board có nhiều state phái sinh (group
theo cột, đếm, quá hạn) cần tính tập trung qua selectors. Compound component
buộc share qua Context → khó test pure logic và re-render thừa. TanStack Table
đi hướng "core + adapter hook" vì cùng lý do.

**Trade-off:** app phải tự wiring JSX và spread prop getters đúng chỗ; sai một
chỗ là mất a11y. Bù lại app tự do bố cục desktop/mobile mà không nhân bản logic.
Giảm rủi ro bằng README + test presenter ở Phase 5.

**Reducer + Selectors (SRP).** Reducer chỉ biến đổi một `KanbanBoard` thành
`KanbanBoard` mới; mọi phái sinh nằm ở selector thuần → test input→output.
`useKanban` memoize selector theo `board` prop. **Một nguồn sự thật:** lib
không sở hữu board; app sở hữu (Teka: TanStack cache) và dùng `kanbanReducer`
để tính optimistic state (`setQueryData(key, b => kanbanReducer(b, action))`).
Trade-off: lib không "tự chạy" nếu app không đưa board mới vào; bù lại không
bao giờ có hai board lệch nhau *(Red-team)*.

**Prop Getters (OCP).** `getColumnProps(columnId)` trả `{role, 'aria-label',
onKeyDown}`; `getTaskProps(taskId)` trả `{role, tabIndex, 'aria-selected',
onKeyDown}`. App compose thêm handler qua tham số thứ hai
(`getTaskProps(id, { onClick })`).

**Port + Command (DIP, ISP).** `KanbanDataSource` là interface app implement ở
Phase 5. Lib gọi `dataSource.moveTask(...)` và **chỉ thế**; cập nhật board
(optimistic hay sau response) là việc của adapter, và board mới quay lại lib
qua prop. Lib **không** biết HTTP, không biết cache, không rollback.

**a11y — chưa chốt, phải verify trước.** Đề xuất hiện tại: mỗi cột
`role="listbox"`, mỗi thẻ `role="option"`, roving tabindex, `[`/`]` chuyển cột,
`aria-live="polite"` do app cung cấp nội dung. Đề xuất này đến từ suy luận, chưa
đối chiếu source thật của react-aria kanban example. **Bước 1 của phase là đọc
source đó (hoặc W3C APG Listbox) rồi mới chốt roles.** Xem Risk.

## Related Code Files

**Create** (dưới `apps/web/src/lib/kanban/`)

- `types.ts` — branded IDs, `KanbanColumn`, `KanbanTask`, `KanbanBoard`.
- `state.ts` — `KanbanAction`, `kanbanReducer`.
- `selectors.ts` — 3 selector thuần.
- `errors.ts` — `KanbanError`, type guard `isKanbanError`.
- `data-source.ts` — interface `KanbanDataSource`.
- `use-kanban.ts` — hook chính + prop getters.
- `use-kanban-keyboard.ts` — roving tabindex + phím tắt.
- `index.ts` — public surface (chỉ export cái app cần).
- `README.md` — ports, prop getters, hợp đồng a11y, luật biên giới, cách tách.
- `__tests__/state.test.ts`, `__tests__/selectors.test.ts`,
  `__tests__/use-kanban.test.tsx`, `__tests__/use-kanban-keyboard.test.tsx`.

**Modify**

- `apps/web/eslint.config.js` — thêm block override cho
  `src/lib/kanban/**/*.{ts,tsx}` theo precedent `no-restricted-imports` tại
  `:38-61`.

## Implementation Steps

1. **Verify a11y trước khi code.** Đọc source react-aria kanban example hoặc
   W3C APG Listbox pattern; ghi kết luận (roles, key map, live-region policy)
   vào đầu `README.md` kèm URL. Nếu kết luận khác đề xuất, sửa đề xuất, không
   sửa kết luận.
2. `types.ts`: branded ID bằng intersection `string & { readonly __brand }`;
   helper `asColumnId`/`asTaskId` để adapter convert một chỗ.
3. `state.ts`: discriminated union + `kanbanReducer` với `switch` exhaustive
   (dùng `never` check ở `default` để TS bắt thiếu case).
4. `selectors.ts`: `selectTasksByColumn` trả `Map<ColumnId, TTask[]>`, sort
   `position` rồi `createdAt`. Tránh mutate input.
5. `errors.ts`: union + guard. Lib **không** map từ HTTP — đó là việc adapter.
6. `data-source.ts`: interface 9 method, mọi method trả `Promise`, ném
   `KanbanError`.
7. `use-kanban.ts`: **không** `useReducer` cho board. `useMemo` selectors theo
   `board` prop; `useState` chỉ cho `activeTaskId`; action wrapper
   (`moveTask`, `reorderColumn`) gọi `dataSource` và trả `Promise` — không tự
   đổi board. Prop getters nhận `extra` để merge handler.
8. `use-kanban-keyboard.ts`: quản lý `activeTaskId` per column; xử lý
   ArrowUp/Down/Home/End và `[`/`]`; sau khi move, giữ focus theo thẻ (không
   nhảy về đầu cột). Không tự render live region — trả `announcement` string để
   app đặt vào region của mình.
9. `index.ts`: export types, reducer, selectors, 2 hook, `KanbanError`. Không
   export nội bộ.
10. `eslint.config.js`: thêm override chặn `@/features/*`, `@/components/*`,
    `@/lib/api`, `@/lib/api/*`, `@tanstack/*`, `zod`, `axios`, kèm message giải
    thích lý do biên giới.
11. Tests: reducer (mỗi action + action lạ không đổi state), selectors (sort
    ổn định, cột rỗng), `useKanban` bằng RTL (`renderHook`) với fake
    `KanbanDataSource`, keyboard hook (arrow/Home/End/`[`/`]` + roving tabindex
    đúng một `tabIndex=0` mỗi cột).
12. `README.md`: bảng port, ví dụ adapter 20 dòng, hợp đồng a11y đã verify ở
    bước 1, luật biên giới, quy trình tách sang repo khác.
13. Gates: `make test-web`, `make lint-web` (ESLint phải fail khi cố tình thêm
    import cấm — kiểm chứng một lần rồi revert).

## Success Criteria

- [x] `make test-web` xanh cho phần lib: `npx vitest run src/lib/kanban` →
      34/34 pass, không cần MSW (không có HTTP). **Lưu ý:** `make test-web`
      full-repo hiện đỏ vì 1 test không liên quan
      (`src/features/center/__tests__/center-schemas.test.ts`, lệch do
      `src/test/msw/handlers.ts` đã bị sửa bởi việc khác ngoài phạm vi sở hữu
      file của phase này) — xem báo cáo hoàn thành để biết chi tiết.
- [x] `make lint-web` xanh; thêm `import { HvButton } from "@/components/hv"`
      vào file trong lib → ESLint error → AC-LIB (kiểm chứng rồi revert).
- [x] `grep -rn "@/features\|@/components\|@/lib/api\|@tanstack" src/lib/kanban`
      → 0 hit.
- [x] Reducer test phủ mọi case của union; TS báo lỗi khi thêm action mới mà
      quên case (kiểm chứng bằng `never` guard).
- [x] `useKanban` test: đổi `board` prop → `tasksByColumn` đổi theo; gọi
      `moveTask` → fake `dataSource` nhận đúng args và board trả ra **không**
      đổi cho tới khi prop đổi (chứng minh hook không giữ bản sao).
- [x] Keyboard test chứng minh: đúng một thẻ `tabIndex=0` mỗi cột; `]` gọi
      `dataSource.moveTask` với cột kề bên phải; ở cột cuối `]` không gọi gì.
- [x] `README.md` ghi rõ hợp đồng a11y kèm URL nguồn đã đọc ở bước 1.

## Risk Assessment

| Rủi ro | Mức | Mitigation |
|---|---|---|
| Pattern ARIA kanban chưa verify → roles sai, screen reader đọc lệch | Cao × Cao | Bước 1 bắt buộc đọc source thật trước khi code. **Tín hiệu vỡ:** source cho thấy react-aria dùng `grid`/`gridcell` thay vì `listbox`/`option`, hoặc dùng nút hành động ẩn thay phím tắt. **Ứng phó:** đổi sang pattern đã verify, cập nhật prop getters + test cùng lúc; nếu chi phí vượt nửa ngày, giữ `listbox` nhưng ghi hạn chế vào README và mở follow-up. |
| Prop getters nuốt handler của app | Trung × Trung | Getter nhận `extra` handler, gọi trước handler nội bộ, tôn trọng `defaultPrevented`. Test có case này. |
| Selector không memoize → re-render toàn board mỗi keystroke | Trung × Trung | `useMemo` theo reference `board`; reducer trả state mới chỉ khi thực sự đổi. |
| Ai đó "tiện tay" thêm `useReducer` vào hook để board tự cập nhật → hai nguồn sự thật | Trung × Cao | README ghi luật "board là prop"; test ở Success Criteria chứng minh hook không giữ bản sao. |
| Branded ID gây ma sát ở biên adapter | Cao × Thấp | 2 helper convert tập trung; adapter gọi một lần khi parse response. |
| ESLint override quá rộng chặn cả import nội bộ lib | Thấp × Trung | Pattern chỉ liệt kê alias/package cấm, không chặn relative import trong cùng thư mục. Test bằng cách chạy lint sau bước 10. |

**Giả định có thể sai:** hook-only đủ, không cần compound component. *Tín hiệu
vỡ:* Phase 5 phải truyền >5 prop xuyên 3 tầng để một thẻ hoạt động đúng. *Ứng
phó:* thêm một Context provider **tuỳ chọn** ở lib bọc quanh cùng hook, giữ hook
là API chính — không viết lại thành compound component.
