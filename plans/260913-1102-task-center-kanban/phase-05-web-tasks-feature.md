---
phase: 5
title: "Web tasks feature"
status: completed
priority: P1
effort: "1.5d"
dependencies: [3, 4]
---

# Phase 5: Web tasks feature (`apps/web/src/features/tasks`)

## Overview

Feature Teka nối `src/lib/kanban` với API thật: api client + Zod schemas, query
key factory, hooks TanStack Query, adapter `KanbanDataSource` với optimistic
update, presenter desktop/mobile, dialog cấu hình cột, dropdown "Chuyển sang",
form công việc với assignee picker từ directory, route + nav entry + deep-link
guard, MSW handlers, và bộ test quartet.

Đây là phase duy nhất biết cả hai phía: nó là *adapter layer* theo đúng nghĩa.

## Requirements

### Functional

- API client: `tasks-api.ts` (board, CRUD task, move), `task-columns-api.ts`
  (CRUD cột, reorder), `member-directory-api.ts` (hoặc bổ sung vào
  `features/center/api/center-api.ts` nếu directory thuộc domain center).
- Zod schemas mirror DTO của Phase 3; `parseData`/`parseList` theo khuôn
  `src/lib/api/envelope.ts`.
- Query keys: `tasksKeys = { all: ["tasks"], boards: () => [...], board: (params) => [...] }` theo
  khuôn `features/roster/hooks/roster-keys.ts`.
- Adapter `KanbanDataSource`: optimistic update qua
  `queryClient.setQueryData(key, b => kanbanReducer(b, action))`; **mọi** lỗi
  → `invalidateQueries(tasksKeys.boards())` (mọi entry board bất kể params),
  không rollback snapshot; `onSettled` → invalidate; 9 mutation dùng chung
  `scope: { id: "kanban-board" }` để chạy tuần tự *(Red-team: rollback snapshot
  xoá mất optimistic state của mutation khác đang bay)*. Map `ApiError` →
  `KanbanError`.
- Trang `/tasks`: guard `has("tasks.list")` (nav ẩn **và** redirect deep-link).
- Desktop: board scroll ngang khi > 3 cột, cột tối thiểu 230px, cột `is_done`
  nền mint-50 + ✓, thẻ mờ 72% và hiện "xong dd/mm".
- Mobile: ≤ 4 cột dùng `HvSegmented` có đếm; > 4 cột dùng `HvSelect`; render 1
  cột.
- `HvSegmented` "Của tôi / Toàn trung tâm" chỉ render khi `has("tasks.view_all")`,
  mặc định "Của tôi".
- Nút "Cấu hình cột" chỉ render khi `has("tasks.manage_board")`; dialog cho đổi
  tên inline (lưu khi blur), ↑/↓ gọi reorder, toggle `is_done`, xoá qua
  `HvConfirmDialog` + `HvSelect` chọn cột đích khi cột còn việc.
- Form công việc: `HvSelect` cột (mặc định cột position 0 hoặc cột đang mở),
  `HvSelect` "Giao cho" từ directory, hạn, ưu tiên; nút Xoá chỉ hiện khi caller
  là creator/owner và có `tasks.delete`; assignee không phải creator thấy form
  chế độ đọc + menu chuyển cột.
- Live region `aria-live="polite"` nhận `announcement` từ `useKanbanKeyboard`.

### Non-functional

- Không import ngược: `src/lib/kanban` không biết feature này tồn tại.
- MSW handlers phủ đủ happy path; test override cho lỗi/edge.
- Quartet (loading / empty / error / data) cho trang board.

## Architecture

**Adapter (DIP).** `use-tasks-data-source.ts` implement `KanbanDataSource` bằng
TanStack Query mutations. Lib gọi intent; adapter lo transport + cache. Đây là
chỗ duy nhất biết cả `KanbanError` lẫn `ApiError`.

**Một nguồn sự thật + invalidate-on-error.** Cache TanStack là board duy nhất
(plan D11). Optimistic = `setQueryData` bằng `kanbanReducer` của lib; lỗi bất
kỳ (404/409 hay mạng/5xx) = `invalidateQueries` để lấy sự thật từ server. Không
snapshot/rollback: với hai mutation bay song song, rollback về snapshot của
mutation A xoá luôn optimistic state của B. `scope: { id: "kanban-board" }`
trên cả 9 mutation làm chúng chạy tuần tự nên optimistic state chồng nhau
đúng thứ tự. Trade-off: lỗi mạng cũng gây một refetch (thay vì rollback tức
thì); chấp nhận vì đơn giản hơn và không có nhánh lỗi thứ hai để sai. Đây là
**pattern mới** trong repo — `teaching/hooks/use-teaching-mutations.ts` là
nơi duy nhất dùng `onMutate` hiện nay và **không** optimistic, chỉ là điểm
so sánh, không phải khuôn.

**Presenter (Strategy).** `board-desktop.tsx` và `board-mobile.tsx` nhận cùng
một `UseKanbanResult` từ một instance `useKanban` ở page. Presenter "dumb" tuyệt
đối: không gọi mutation, không giữ state ngoài `activeColumnId` của mobile.
Trade-off: page phải chọn presenter theo breakpoint; bù lại 0 dòng logic move bị
nhân đôi. Test: cùng fixture state, hai presenter, hai assertion set.

**Permission gating 2 lớp.** `useCenterContext().has(key)` ẩn UI; API vẫn là
authority. Deep-link guard là bắt buộc — ẩn nav không phải authorization
(`docs/frontend-guidelines.md`).

**Testability.** Lib đã test logic thuần ở Phase 4. Phase này test cái lib không
biết: mapping API, optimistic + invalidate, gating theo `has()`, hai presenter.

## Related Code Files

**Create** (dưới `apps/web/src/features/tasks/`)

- `api/tasks-api.ts`, `api/task-columns-api.ts`
- `schemas/task-schemas.ts`
- `hooks/tasks-keys.ts`, `hooks/use-task-board.ts`,
  `hooks/use-tasks-data-source.ts`, `hooks/use-member-directory.ts`
- `lib/map-api-error.ts` (`ApiError` → `KanbanError`)
- `pages/task-board-page.tsx`
- `components/board-desktop.tsx`, `components/board-mobile.tsx`,
  `components/task-column.tsx`, `components/task-card.tsx`,
  `components/move-menu.tsx`, `components/task-form-modal.tsx`,
  `components/board-settings-modal.tsx`, `components/delete-column-dialog.tsx`
- `routes.tsx`, `index.ts`
- `__tests__/tasks-handlers.ts`, `__tests__/task-board-page.test.tsx`,
  `__tests__/board-settings-modal.test.tsx`,
  `__tests__/use-tasks-data-source.test.tsx`,
  `__tests__/board-mobile.test.tsx`

**Modify**

- `apps/web/src/app/router.tsx` — mount `tasksRoutes` (khuôn import tại `:5-17`).
- `apps/web/src/layouts/dashboard-layout.tsx` — NavEntry "Công việc" `/tasks`
  với `perm: "tasks.list"`; **thêm** `"Công việc"` vào `OVERFLOW_LABELS`
  (`:170`) và `/tasks` vào `OVERFLOW_PATH_PREFIXES` (`:191`) để mobile giữ
  đúng 3 tab chính + "Thêm" (`primaryTabs` `:540`); desktop sidebar hiển thị
  bình thường *(Red-team: không đưa vào overflow → test 3 tab chính đỏ; đưa
  vào là quyết định UX, ghi ở handoff)*.
- `apps/web/src/layouts/__tests__/dashboard-layout.test.tsx` — giữ assertion
  3 tab (`:121`); thêm case "Công việc" nằm trong sheet "Thêm" khi có
  `tasks.list` và vắng khi không có.
- `apps/web/src/test/msw/handlers.ts` — handler mặc định cho board, cột,
  directory.
- `apps/web/src/features/center/api/center-api.ts` +
  `schemas/center-schemas.ts` — hàm `getMemberDirectory` nếu chọn đặt directory
  ở domain center.

## Implementation Steps

1. Đọc `features/roster/hooks/roster-keys.ts` và `hooks/use-students.ts` làm
   khuôn key factory + mutation invalidation; đọc `features/teaching/hooks/
   use-teaching-mutations.ts` chỉ để biết cách viết `onMutate` trong repo —
   optimistic-by-reducer là pattern mới, không có khuôn sẵn.
2. `schemas/task-schemas.ts`: Zod cho `TaskColumn`, `Task`, `BoardResponse`,
   `MemberDirectoryEntry`. Field `omitempty` phía Go dùng `.optional()`.
3. `api/*.ts`: hàm async đặt tên, dùng `apiClient.get<unknown>` +
   `parseData`/`parseList`. Không bắt lỗi ở đây — interceptor đã sinh `ApiError`.
4. `lib/map-api-error.ts`: 404 → `not-found`, 409 → `conflict`, 403 →
   `forbidden`, 422 → `validation` kèm `err.fields`, còn lại → `unknown`.
5. `hooks/tasks-keys.ts`: `all`, `boards()` (prefix mọi board), `board(params)`,
   `directory()`.
6. `hooks/use-task-board.ts`: `useQuery` với `placeholderData: keepPreviousData`.
7. `hooks/use-tasks-data-source.ts`: 9 mutation, cùng `scope: { id:
   "kanban-board" }`; mỗi mutation có `onMutate` (`cancelQueries` +
   `setQueryData(key, b => b && kanbanReducer(b, action))`, **không**
   snapshot), `onError` (`invalidateQueries(tasksKeys.boards())`), `onSettled`
   (invalidate cùng key). Trả object implement `KanbanDataSource`, ổn định
   qua `useMemo`.
8. `pages/task-board-page.tsx`: guard `has("tasks.list")` (redirect khi
   `isResolved && !has`), gọi `useTaskBoard` + `useKanban`, chọn presenter theo
   breakpoint, render live region, header với đếm việc mở/quá hạn.
9. `components/board-desktop.tsx` + `task-column.tsx` + `task-card.tsx`: spread
   `getColumnProps`/`getTaskProps`; token theo `src/styles/tokens/colors.css`
   (cột done nền mint-50, thẻ của tôi viền trái mint-400).
10. `components/board-mobile.tsx`: `HvSegmented` khi ≤ 4 cột, `HvSelect` khi
    nhiều hơn; render đúng 1 `task-column`.
11. `components/move-menu.tsx`: Radix DropdownMenu liệt kê cột theo position,
    loại cột hiện tại; dùng chung desktop + mobile.
12. `components/task-form-modal.tsx`: `HvModal` + react-hook-form + Zod; hiển
    thị lỗi field từ `KanbanError.validation`; chế độ đọc cho assignee.
13. `components/board-settings-modal.tsx` + `delete-column-dialog.tsx`: đếm
    n/8, "Thêm cột" disabled khi đủ 8, xoá disabled khi còn 1 cột, 409 trùng
    tên hiện dưới ô, 409 reorder → toast "Bảng đã thay đổi, tải lại" + refetch.
14. `routes.tsx` + mount vào `app/router.tsx`; NavEntry vào
    `layouts/dashboard-layout.tsx`.
15. MSW: handler mặc định trong `src/test/msw/handlers.ts`; override lỗi trong
    `__tests__/tasks-handlers.ts`.
16. Tests: quartet cho board page; gating (`has` false → không thấy nút cấu
    hình cột, không thấy segmented view_all); optimistic move hiện ngay rồi
    server trả 409 → board refetch về sự thật (không phải snapshot); hai move
    liên tiếp → request thứ hai chỉ gửi sau khi request đầu xong; xoá cột có
    việc bắt buộc chọn đích; mobile ≤4 vs >4 cột; form ở chế độ đọc cho
    assignee; "Công việc" trong sheet "Thêm" ở mobile.
17. Gates: `make test-web`, `make lint-web`.

## Success Criteria

- [x] Board render đúng N cột theo position; thẻ nhóm đúng cột → AC2.
- [x] Không có `tasks.view_all`: không thấy segmented "Toàn trung tâm"; có
      khoá: chuyển được và board đổi dữ liệu → AC2.
- [x] Không có `tasks.manage_board`: nút "Cấu hình cột" không render; vào
      `/tasks` trực tiếp vẫn xem được board → AC4.
- [x] Không có `tasks.list`: nav ẩn **và** `/tasks` redirect → AC4 (deep-link).
- [x] Assignee thấy form chế độ đọc + menu chuyển cột; không thấy nút Xoá → AC3.
- [x] Xoá cột còn việc: dialog bắt chọn cột đích, gọi đúng một request kèm
      `move_to` → AC5.
- [x] Cột `is_done` hiển thị nền mint-50 + ✓; thẻ hiện "xong dd/mm" → AC6.
- [x] Test chứng minh optimistic move áp ngay qua `kanbanReducer`, và mọi lỗi
      (409/404/mạng) đều dẫn tới refetch board — không có snapshot.
- [x] Test chứng minh 9 mutation chạy tuần tự (scope chung).
- [x] `dashboard-layout.test.tsx`: 3 tab chính giữ nguyên; "Công việc" trong
      sheet "Thêm".
- [x] `make test-web` xanh; `src/lib/kanban` không bị sửa. `make lint-web`
      dừng ở 1 lỗi có sẵn, ngoài phạm vi phase, trong
      `center-permissions.test.tsx` (xem báo cáo).

## Risk Assessment

| Rủi ro | Mức | Mitigation |
|---|---|---|
| Rollback snapshot xoá optimistic state của mutation khác đang bay | Trung × Cao | Không snapshot; lỗi → invalidate; `scope` chung serialize 9 mutation (D11). |
| `onSettled` invalidate liên tục gây nhấp nháy board | Trung × Trung | `placeholderData: keepPreviousData` + chỉ invalidate `tasksKeys.boards()` (mọi params của board), không invalidate `all` (directory không refetch). |
| "Công việc" nằm sau "Thêm" ở mobile → khó tìm | Trung × Thấp | Quyết định UX có chủ đích để không phá 3 tab chính; nêu ở handoff, đổi được bằng một dòng nếu user muốn thay "Thu tiền". |
| Đổi tên cột lưu khi blur gây race với reorder đang bay | Trung × Trung | Mỗi command là mutation riêng; dialog disable hàng đang có mutation pending. Không có CAS ở v1 (D10) → last-write-wins là hành vi đã chấp nhận. |
| Directory endpoint thuộc domain center hay tasks | Thấp × Thấp | Đặt ở `features/center/api` vì nó là dữ liệu thành viên và khoá là `members.list`; `features/tasks` import qua `features/center/index.ts`. |
| Board 8 cột × 50 việc chậm trên mobile cũ | Trung × Trung | Mobile render 1 cột; selector memoize; nếu vẫn chậm, virtualize cột — chỉ khi đo được, không làm trước. |
| MSW `onUnhandledRequest: "error"` làm test fail vì thiếu handler directory | Cao × Thấp | Thêm handler directory vào `src/test/msw/handlers.ts` ở bước 15, không chỉ trong file test của feature. |

**Giả định có thể sai:** một instance `useKanban` ở page phục vụ được cả hai
presenter. *Tín hiệu vỡ:* mobile cần state riêng vượt quá `activeColumnId` (ví
dụ thứ tự cột khác desktop). *Ứng phó:* nâng state đó lên page và truyền xuống,
không tạo instance `useKanban` thứ hai (sẽ tách nguồn sự thật).
