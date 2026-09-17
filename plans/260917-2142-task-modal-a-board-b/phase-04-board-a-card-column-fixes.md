---
phase: 4
title: "Bảng · sửa lỗi phương án A — card, cột, header"
status: pending
priority: P1
effort: "1d"
dependencies: [1, 2]
---

# Phase 4: Bảng · sửa lỗi phương án A (card, cột, header)

## Context Links

- Report mục "Bảng · Phương án A" (menu ⋯, badge ưu tiên, nhãn hạn, avatar, tương phản, drop zone rỗng, tóm tắt header, cuộn ngang) — Bảng B kế thừa toàn bộ
- Quyết định D8, D10, D11 trong [plan.md](./plan.md)
- `apps/web/src/features/tasks/components/task-card.tsx` (`TaskCardBody`, barrier `role="presentation"` quanh `MoveMenu`)
- `apps/web/src/features/tasks/components/move-menu.tsx` (trigger `aria-label="Chuyển cột"`, chữ "Chuyển")
- `apps/web/src/features/tasks/components/task-card-styles.ts` (`isOverdue` dùng UTC; `taskCardSurfaceClassName`)
- `apps/web/src/features/tasks/components/task-column.tsx` (header cột, nút "+" `size-6`, "Chưa có việc", spacer `min-h-12`)
- `apps/web/src/features/tasks/components/board-desktop.tsx` (cuộn ngang), `board-mobile.tsx`
- `apps/web/src/features/tasks/pages/task-board-page.tsx` (header trang, HvSegmented phạm vi — **bỏ** theo D11, "Cấu hình cột"), `__tests__/task-board-page.test.tsx:85-107` (2 test segmented cần thay)
- `apps/web/src/features/tasks/api/tasks-api.ts:15-22` (`getBoard(params)` gửi `scope` — bỏ), `schemas/task-schemas.ts` (`boardResponseSchema` thêm `counts`), `__tests__/tasks-handlers.ts:154-156` (MSW đọc `scope` → trả `counts`)
- e2e: `apps/web/e2e/tasks-board.spec.ts:43` (`getByRole("button", { name: "Chuyển cột" })`), `apps/web/e2e/helpers/board.ts` (`topCardTitles` đọc `<p>` đầu)
- Tests: `apps/web/src/features/tasks/__tests__/{task-board-page,board-mobile,use-board-dnd}.test.tsx`

## Overview

Sửa 8 lỗi/mùi trên bảng hiện tại mà không đổi hợp đồng lib. Kết thúc phase:
bảng "sạch" để Phase 5 xếp thêm lớp điều phối lên trên. Phase này cũng **gỡ
segmented phạm vi** (D11) và đọc `counts` từ API (Phase 1) cho phụ đề, nên phụ
thuộc Phase 1.

## Key Insights

- `MoveMenu` là radix DropdownMenu; đổi thành menu "Thao tác" chỉ cần đổi
  trigger (icon ⋯ 40px, `aria-label="Thao tác"`) và thêm mục "Mở" (gọi `onOpen`)
  + "Xoá" tuỳ chọn? **Không** — report chỉ yêu cầu chuyển cột nằm trong ⋯;
  giữ menu là danh sách cột (submenu không cần), đổi tên/hình.
- Barrier `role="presentation"` (`onClick`/`onMouseDown`/`onTouchStart`
  stopPropagation) là mẫu đúng cho mọi control lồng trong card kéo được —
  tái dùng cho checkbox Xong ở Phase 5 → tách thành `CardControlBarrier`.
- `topCardTitles` e2e đọc `<p>` đầu tiên trong option → tiêu đề phải vẫn là
  `<p>` đầu; hàng đầu card (avatar + tiêu đề + ⋯) dùng `div` bao ngoài nhưng
  tiêu đề vẫn là `<p>` con đầu tiên xuất hiện trong DOM.
- Hover-only control cần 3 điều kiện hiện: `group-hover`, `group-focus-within`,
  `[@media(hover:none)]` (touch luôn hiện).
- Segmented phạm vi là control chết với người có `view_all`: server luôn echo
  `scope: "center"` (`service.go:87-91`) nên chọn "Của tôi" bị bật ngược sau
  refetch. Gỡ ở phase này; Phase 5 đặt dải lọc vào chỗ trống.
- Phụ đề đếm từ `counts` server (đếm trên toàn bộ tập nhìn thấy) thay vì
  `kanban.board.tasks` (≤ 50/cột) — không cần `summarizeBoard` phía client.
- `isOverdue` sai giờ địa phương: `new Date().toISOString()` là UTC, sau 17:00
  giờ VN đã sang ngày mới → việc hạn hôm nay bị báo quá hạn từ 07:00 sáng ngày
  hạn? Ngược lại: từ 17:00 hôm trước UTC = 00:00 hôm sau VN… kiểm bằng test
  `vi.setSystemTime("2026-09-17T18:30:00+07:00")` với `due_on = 2026-09-17` → phải **không** quá hạn.

## Requirements

Functional
1. **Menu ⋯ "Thao tác"**: nút `size-10` icon `MoreHorizontal`, `aria-label="Thao tác"`,
   góc phải trên card; `opacity-0 group-hover:opacity-100 group-focus-within:opacity-100 [@media(hover:none)]:opacity-100`;
   nội dung menu: heading "Chuyển sang" + các cột khác (giữ `menuitem` tên cột).
2. **Badge ưu tiên**: không render khi `priority === "none"`; low/medium/high giữ.
3. **Nhãn hạn** (`dueState`): `Quá hạn N ngày` (danger), `Hôm nay` (warning),
   `Ngày mai` (neutral), `Hạn dd/MM` (neutral); done → `Xong dd/MM` (success) như cũ.
4. **Avatar chữ cái** 24px thay tên assignee (`aria-label="Phụ trách: {tên}"`,
   `title`); màu nền từ hash tên → 4 tint (mint/sky/sun/coral-100) với chữ ink-900.
5. **Tương phản**: mọi `text-ink-400` trong card/cột (assignee, đếm, "Chưa có việc",
   counter) → `text-ink-500`.
6. **Nút "+"** `size-10` (touch target), vẫn `aria-label="Thêm việc vào X"`.
7. **Cột rỗng**: thay "Chưa có việc" bằng vùng `border-2 border-dashed border-line-200 rounded-[12px] min-h-[96px]`
   chứa nút chữ "Thêm việc" (gọi `onCreateTask`, chỉ khi `canCreate`) — vẫn là drop target.
8. **Header trang**: **bỏ HvSegmented** phạm vi (D11) và state `requestedScope`;
   `getBoard` không gửi `scope`, gửi `today=localIsoDate(new Date())` (để `counts`
   đúng ngày địa phương). Dưới `h1` thêm `p` tóm tắt từ `board.counts`:
   `canViewAll` → `Toàn trung tâm · {all} việc`; ngược lại → `{all} việc · {overdue} quá hạn · {today} hôm nay`
   (ẩn số 0 trừ "N việc"); "Cấu hình cột" → `variant="ghost"` + icon `Columns2`.
9. **Cuộn ngang desktop**: container `scroll-snap-type: x proximity`, mỗi cột
   `scroll-snap-align: start`; fade phải bằng pseudo-element khi còn cuộn được
   (`data-can-scroll-right`, cập nhật qua `onScroll` + `ResizeObserver`).

Non-functional
- Không đổi props lib, không đổi API. Tất cả test hiện có xanh sau cập nhật locator.

## Architecture

```
lib/due-state.ts (Phase 3) + dueState(task, now): { kind: "overdue"|"today"|"tomorrow"|"upcoming"|"none", days, label, variant }
task-card.tsx
  ├─ header row: <Avatar/> <p title> <CardControlBarrier><ActionsMenu/></CardControlBarrier>
  ├─ preview, badges (priority≠none, due/done)
components/card-control-barrier.tsx (tách từ barrier hiện có)
components/actions-menu.tsx (đổi tên từ move-menu.tsx; giữ export MoveMenu alias? → không, đổi hẳn và sửa import)
components/assignee-avatar.tsx
pages/task-board-page.tsx: BoardSummary(counts, canViewAll) — "Toàn trung tâm · N việc" | "N việc · N quá hạn · N hôm nay"; không còn HvSegmented
```

## Related Code Files

Sửa
- `task-card.tsx`, `task-card-styles.ts` (xoá `isOverdue`, thêm re-export từ `lib/due-state`), `task-column.tsx`, `board-desktop.tsx`, `board-mobile.tsx` (hover rule không áp — touch luôn hiện), `task-board-page.tsx` (bỏ segmented + `requestedScope`, summary từ `counts`), `task-card-preview.tsx` (dùng `TaskCardBody` mới)
- `schemas/task-schemas.ts` (`boardCountsSchema`, `boardResponseSchema.counts`), `api/tasks-api.ts` (`GetBoardParams` → `{ today }` ở phase này; Phase 5 thêm `filter`/`assignee`), `hooks/use-task-board.ts` (key theo params mới), `__tests__/tasks-handlers.ts` (bỏ `scope`, trả `counts` tính từ fixture)
- `move-menu.tsx` → `actions-menu.tsx` (git mv)
- `lib/due-state.ts` (+ `dueState`), `__tests__/due-state.test.ts`
- `__tests__/task-board-page.test.tsx` (locator "Chuyển cột" → "Thao tác"; thay 2 test segmented `:85-107` bằng test phụ đề theo `canViewAll`), `board-mobile.test.tsx`, `use-board-dnd.test.tsx` (locator)
- `apps/web/e2e/tasks-board.spec.ts:43` (locator), `e2e/helpers/board.ts` (kiểm `topCardTitles` vẫn đúng)

Tạo mới
- `components/card-control-barrier.tsx`, `components/assignee-avatar.tsx`

## Implementation Steps

1. `lib/due-state.ts`: `dueState(task: { dueOn: string|null; completedAt: string|null }, now = new Date())` — so `dueOn` với `localIsoDate(now)` bằng chênh ngày lịch (`Date.UTC` của y/m/d để tránh DST); trả label/variant. Unit test: 18:30 VN hôm hạn → today; hạn hôm qua → "Quá hạn 1 ngày"; không hạn → none; đã xong → none.
2. `task-card-styles.ts`: bỏ `isOverdue`; `taskCardSurfaceClassName` nhận `dueState(task).kind === "overdue"`.
3. `card-control-barrier.tsx`: component `div role="presentation"` với 3 handler stopPropagation (+ `className` prop). Thay barrier trong `task-card.tsx`.
4. `git mv move-menu.tsx actions-menu.tsx`; `ActionsMenu` props như cũ; trigger icon `MoreHorizontalIcon`, `size-10`, `aria-label="Thao tác"`, class hiện/ẩn theo hover; `DropdownMenuLabel` "Chuyển sang". Sửa import ở `task-card.tsx`.
5. `assignee-avatar.tsx`: `AssigneeAvatar({ name })` → `span` 24px, chữ cái đầu (chuẩn hoá `NFD`, lấy ký tự chữ đầu), `aria-label`, `title`; hash đơn giản (`charCodeAt` sum % 4).
6. `task-card.tsx`: hàng đầu `flex items-start gap-2` — `AssigneeAvatar` (nếu có) · `<p>` tiêu đề `flex-1` · `CardControlBarrier` + `ActionsMenu`. Card root thêm `group`. Badges: `priority !== "none"` mới render; due từ `dueState`. Bỏ dòng tên assignee.
7. `task-column.tsx`: "+" `size-10`; đếm `text-ink-500`; cột rỗng → drop zone dashed + nút "Thêm việc" (`canCreate`); spacer giữ.
8. `board-desktop.tsx`: wrapper `relative`; list `snap-x snap-proximity` (Tailwind `snap-x snap-proximity`, cột `snap-start`); state `canScrollRight` từ `onScroll`/`ResizeObserver` → `after:` gradient `from-transparent to-cream-50` 32px `pointer-events-none`.
9. `schemas/task-schemas.ts`: `boardCountsSchema = z.object({ all, mine, overdue, today, unassigned: z.number().int(), by_assignee: z.array(z.object({ teacher_id, count })) })`; `boardResponseSchema.counts`. `tasks-api.ts`: `GetBoardParams = { today: string }` (bỏ `scope`), `getBoard` gửi `today`. `use-task-board.ts`/`tasks-keys.ts`: key gồm params mới. MSW `tasks-handlers.ts:154-156`: bỏ nhánh `scope`, tính `counts` từ fixture (chỉ `completed_at == null`) theo `today` query.
9b. `task-board-page.tsx`: xoá `HvSegmented`, `requestedScope`, `effectiveScope` (giữ `canViewAll` từ perm để chọn phụ đề); `BoardSummary({ counts, canViewAll })` `p.text-[12.5px].text-ink-500`; "Cấu hình cột" ghost + `Columns2Icon`. `formatBoardSummary(counts, canViewAll)` thuần trong `lib/due-state.ts` (hoặc `lib/board-summary.ts`) + unit test.
10. Cập nhật tests: locator "Thao tác"; test badge none không render; test drop zone rỗng có nút "Thêm việc"; test summary: `canViewAll` → "Toàn trung tâm · 3 việc"; không → "3 việc · 1 quá hạn"; xoá 2 test segmented (`task-board-page.test.tsx:85-107`) và assertion `boardScopeRequests`; thêm assertion request board có `today=` đúng ngày địa phương (`vi.setSystemTime`).
11. e2e `tasks-board.spec.ts`: `name: "Chuyển cột"` → `name: "Thao tác"`; chạy `make e2e-isolated E2E_ARGS="tasks-board"` (4 spec tasks-board*).
12. `make lint-web test-web`.

## Todo

- [ ] `dueState` + `summarizeBoard` + tests; xoá `isOverdue`
- [ ] `CardControlBarrier`, `ActionsMenu` (⋯ Thao tác), `AssigneeAvatar`
- [ ] Card: hàng đầu mới, badge none ẩn, nhãn hạn ngữ nghĩa, ink-500
- [ ] Cột: "+" 40px, drop zone rỗng "Thêm việc"
- [ ] Desktop: snap + fade
- [ ] Page: bỏ segmented/scope, `counts` schema + `today` param, summary theo `canViewAll`, ghost "Cấu hình cột"
- [ ] Tests unit + e2e locator; `make lint-web test-web`; `make e2e-isolated E2E_ARGS="tasks-board"`

## Success Criteria

- Card không có chữ "Chuyển"; ⋯ hiện khi hover/focus/touch; menu mở bằng Enter, các `menuitem` là tên cột khác.
- Việc hạn hôm nay lúc 23:00 giờ VN hiển thị "Hôm nay", không "Quá hạn".
- Không còn `text-ink-400` trong `features/tasks/components` (grep = 0).
- e2e `tasks-board*` xanh.
- Không còn `HvSegmented` trong `pages/task-board-page.tsx` và không request board nào mang `scope=` (grep `scope` trong `tasks-api.ts`/`use-task-board.ts` = 0); phụ đề khớp `counts` từ MSW kể cả khi fixture > 50 việc một cột.

## Risk Assessment

- **Đổi tên file `move-menu` → `actions-menu`**: grep import ở tests (`__tests__/*`) và `task-card-preview.tsx`.
- **Tên icon lucide** (`MoreHorizontalIcon`, `Columns2Icon`, `ChevronsLeftIcon`) chưa kiểm trong phiên bản cài — xác nhận bằng `npm ls lucide-react` + import thử; thay `EllipsisIcon` nếu `MoreHorizontal` đã bị đổi tên.
- **Fade bằng `ResizeObserver`** trong jsdom không có → guard `typeof ResizeObserver !== "undefined"`.
- **Snap + dnd auto-scroll**: `proximity` (không `mandatory`) để dnd-kit auto-scroll không bị giật; kiểm tay khi kéo sang cột thứ 4.
- **Bỏ segmented làm mất đường "Của tôi"** cho người có `view_all` trong khoảng giữa Phase 4 và 5 → chấp nhận (segmented vốn không hoạt động, xem Key Insights); Phase 5 bù bằng chip "Của tôi".
- **`today` trong query key** đổi lúc 00:00 → refetch một lần; chấp nhận.

## Security Considerations

Không có thay đổi dữ liệu/quyền; avatar chỉ hiển thị chữ cái tên đã có trong directory.

## Next Steps

Phase 5 dựng dải lọc (server `filter`/`assignee`), checkbox Xong (dùng `CardControlBarrier`), cột tint/thu gọn, URL state.
