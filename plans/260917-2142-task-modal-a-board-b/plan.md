---
title: "Task board: Modal phương án A + Bảng phương án B"
description: "Triển khai hai phương án UX từ report ui-redesign-260917-2117: modal công việc gọn (A) và bảng điều phối cho quản lý (B) — gồm migration màu cột, lọc + đếm phía server cho GET /tasks/board, endpoint khôi phục việc, kit hv, web."
status: pending
priority: P1
effort: "7d"
tags: [tasks, kanban, web, api, migration, design-system]
blockedBy: []
blocks: []
created: 2026-09-17
branch: feat/task-modal-a-board-b
---

# Task board: Modal phương án A + Bảng phương án B

## Overview

Report UX `plans/reports/ui-redesign-260917-2117-task-board-and-task-modal.html` chọn **Modal · A** ("Form gọn, hành động đúng chỗ") và **Bảng · B** ("Bảng điều phối", kế thừa sửa lỗi của Bảng A). Kế thừa `plans/260913-1102-task-center-kanban` và `plans/260917-1515-task-dnd-rich-text`. Plan chuyển hai phương án thành:

- **Database**: migration `000024` thêm `task_columns.color` (none/sky/sun/mint) — `is_done` không đủ để tô "Đang làm"/"Chờ duyệt".
- **API**: `color` xuyên `pkg/kanban` → adapter → DTO; `GET /tasks/board` nhận `filter`/`assignee`/`today` và trả `counts` (lọc + đếm phía server, qua `Visibility` của core); `POST /tasks/:id/restore` cho "Hoàn tác" sau xoá (soft delete có, chưa có đường khôi phục).
- **Web**: kit hv (`stickyFooter`, segmented nowrap, `HvChip`) → Modal A (assignee đổi được "Cột") → sửa lỗi Bảng A → Bảng B (dải lọc **thay** segmented như report, Xong một cú nhấp, cột màu + thu gọn, URL state).

## Quyết định thiết kế

| # | Quyết định | Lý do (đã kiểm chứng trong code) |
|---|-----------|----------------------------------|
| D1 | `task_columns.color VARCHAR(16) NOT NULL DEFAULT 'none'` + `CHECK IN ('none','sky','sun','mint')`; core `Column.Color string` **mờ** (chỉ giới hạn độ dài), enum do adapter (`binding:"oneof"`) + DB CHECK giữ | `pkg/kanban` là lib trung lập sản phẩm (README "Localization boundary"); tint không suy được từ `is_done` vì sky/sun là cột chưa xong |
| D2 | `POST /tasks/:id/restore` (perm `tasks.delete`, audit `task.restore`); core `RestoreTask` dùng `CanWriteTask`; repo `GetDeleted` + `Restore` | `Get` hiện loại soft-deleted (`ports.go:111`); không có endpoint nào đọc hàng đã xoá. Không có "cửa sổ hoàn tác" phía server — toast 6s phía client là giới hạn UX |
| D3 | Bộ lọc nhanh lọc **phía server**: `GET /tasks/board?filter=all\|mine\|overdue\|today\|unassigned&assignee=<uuid>&today=YYYY-MM-DD`; response thêm `counts { all, mine, overdue, today, unassigned, by_assignee[] }` đếm trên **toàn bộ** tập nhìn thấy (không bị cắt 50/cột), chỉ việc chưa xong. Lọc và đếm đi qua core (`BoardFilter` → `ListBoard`; `CountBoard`) để `Visibility` vẫn là nguồn luật đọc duy nhất | **Quyết định user 2026-09-17** (đảo D3 cũ). Kiểm chứng: handler `board` hiện **bỏ qua** `?scope=` (`handler.go:56-67`, không có `c.Query("scope")`), web vẫn gửi nó (`tasks-api.ts:19-21`) → chuyển tham số về lọc thật. Số đếm phía client sai khi cột bị cắt (`has_more`) — server đếm bằng một aggregate query là chính xác. Reducer lạc quan không đổi: mutation ghi vào query đang active (key gồm filter), `invalidateBoard` onSettled refetch cả board lẫn `counts`. Ngày "hôm nay" do **client gửi** (`today`, ngày lịch địa phương — nhất quán D8); thiếu → server dùng ngày UTC |
| D4 | Trạng thái bảng (`filter`, `assignee`, `collapsed`) nằm trên URL search params (`useSearchParams`, react-router 8); **không còn** `scope` (D11) | Report yêu cầu persist; URL chia sẻ được, không thêm store |
| D5 | "Xong" nhanh = `moveTask(taskId, cộtDoneĐầuTiên, 0)`; Hoàn tác = `moveTask` về cột/vị trí cũ; Hoàn tác sau xoá = D2 + `tasks/upserted` | Tái dùng đường move duy nhất (`use-tasks-data-source.ts`), không thêm mutation kiểu mới |
| D6 | HvModal thêm prop `stickyFooter` (opt-in) thay vì đổi mặc định md/lg; HvSegmented thêm `whitespace-nowrap` toàn cục; Xoá dạng chữ đỏ = `HvButton variant="ghost"` + class coral, không thêm variant | Tránh đổi hành vi cuộn của mọi modal md/lg hiện có; kit chỉ có 5 variant nút |
| D7 | `lib/kanban` nhận generic cột: `KanbanBoard<TTask, TColumn extends KanbanColumn = KanbanColumn>`; feature khai báo `AppColumn { color }` | Lib không cần biết "màu"; reducer/`useKanban` giữ nguyên field khi upsert. Bác bỏ: thêm `color` vào type lib |
| D8 | `dueState(task, now)` dùng **ngày lịch địa phương**; thay `isOverdue` (đang dùng `toISOString()` = UTC, lệch 7h ở VN) | Lỗi hiện hữu trong `task-card-styles.ts:21-25` |
| D9 | ~~Ánh xạ index thả từ danh sách lọc về cột đầy đủ~~ **Bãi bỏ** theo D3 mới: board trong cache đã là board lọc, `afterTaskIdAt` chọn hàng xóm hiển thị liền trước, server (`ListColumnPositions`) chèn sau hàng xóm đó trong cột đầy đủ | Không còn hai danh sách phía client; thông báo "vị trí i/n" đếm theo việc hiển thị — chấp nhận, ghi docs |
| D10 | Sửa lỗi Bảng A (menu ⋯, ẩn badge "Không", nhãn hạn, avatar, ink-500, drop zone rỗng, tóm tắt header, fade cuộn) là phase riêng, đi trước Bảng B | Report: "Bảng B kế thừa mọi sửa lỗi của Bảng A"; e2e phải xanh sau mỗi phase |
| D11 | **Theo report 100%**: bỏ HvSegmented phạm vi khỏi trang bảng. Người có `tasks.view_all`: phụ đề `Toàn trung tâm · N việc` + dải lọc (`Tất cả · Của tôi · Quá hạn · Hôm nay · Chưa giao` ‖ chip giáo viên có số) thay segmented; người không có: phụ đề Bảng A (`N việc · N quá hạn · N hôm nay`), không dải lọc. Chip "Của tôi" = `assignee_id = tôi` (đúng demo report) | **Quyết định user 2026-09-17** (đảo D11 cũ). Segmented hiện là control chết với người có `view_all`: server luôn echo `scope: "center"` (`service.go:87-91`) nên chọn "Của tôi" bị bật ngược sau refetch. Trade-off đã chấp nhận: việc tôi tạo nhưng giao người khác chỉ thấy ở "Tất cả" hoặc chip giáo viên đó |
| D12 | Modal A với người chỉ là assignee: mọi trường disabled **trừ "Cột"**; footer `Huỷ` + `Lưu` (Lưu chỉ enable khi Cột đổi), submit ở nhánh read-only chỉ gọi `onMoveTask`, không gọi update; áp dụng khi `canEdit` (`tasks.edit`) | **Quyết định user 2026-09-17**. Server đã cho phép: route move cần `tasks.edit` (`routespec.go:380`) và `CanMoveTask` = owner ∨ creator ∨ assignee (`policy.go:47-49`); bảng đã gate `canMove={canEdit}` (`task-board-page.tsx:163`) |

## Phases

| # | Phase (thứ tự 1→6; Phase 1 `apps/api` và Phase 2 `components/hv` chạy song song được) | Effort | Status |
|---|-------|--------|--------|
| 1 | [API + migration: màu cột, lọc/đếm board, khôi phục việc](./phase-01-api-migration-column-color-restore.md) | 1.5d | Done |
| 2 | [Kit hv: footer dính, segmented nowrap, HvChip](./phase-02-hv-kit-modal-footer-segmented.md) | 0.5d | Done |
| 3 | [Modal · Phương án A](./phase-03-modal-a-form-gon.md) | 1d | Done |
| 4 | [Bảng · sửa lỗi phương án A (card, cột, header)](./phase-04-board-a-card-column-fixes.md) | 1d | Done |
| 5 | [Bảng · Phương án B (lọc, Xong nhanh, cột màu/thu gọn, URL)](./phase-05-board-b-dieu-phoi-filters-quick-done.md) | 2d | Done |
| 6 | [E2E, docs, ship](./phase-06-e2e-docs-ship.md) | 1d | Done (chờ push + PR) |

## Success Criteria

- [ ] Migration `000024` up/down sạch; center mới có cột `Đang làm` = sky, `Hoàn thành` = mint; parity test default-columns vẫn xanh.
- [ ] `PATCH /task-columns/:id {color}` và `POST /task-columns {color}` trả `color`; giá trị ngoài enum → 422.
- [ ] `GET /tasks/board?filter=overdue&today=2026-09-17` chỉ trả việc chưa xong có `due_on < today`; `filter=x` → 422; `assignee=<uuid>` AND với `filter`; `counts` không đổi theo `filter` và đếm đúng khi cột có > 50 việc; assignee thuần vẫn chỉ đếm trong tầm nhìn của mình.
- [ ] `POST /tasks/:id/restore`: 200 + task sống lại; việc chưa xoá → 404; không phải owner/creator → 403; có hàng audit `task.restore`.
- [ ] Modal A: không đóng được khi form dirty nếu chưa xác nhận; chip hạn nhanh set ISO đúng ngày địa phương; Xoá chỉ hiện khi `canDeleteThisTask`; footer dính khi mô tả dài; assignee thuần đổi được Cột + Lưu → chỉ gọi move.
- [ ] Bảng: không còn nút "Chuyển ›"; menu mở bằng bàn phím với tên "Thao tác"; `[`/`]` giữ nguyên; badge "Không" không render.
- [ ] Bảng B: không còn HvSegmented "Phạm vi bảng công việc"; dải lọc chỉ hiện với `tasks.view_all`; chip đổi `filter`/`assignee` trên URL và board refetch theo; số trên chip = `counts` server; checkbox Xong chuyển việc và Hoàn tác trả về đúng cột/vị trí; thu gọn cột ghi vào URL và giữ sau reload.
- [ ] Tương phản `ink-500` trên `sky-50`/`sun-100` ≥ 4.5:1 (kiểm bằng script tính nhanh trong phase 5).
- [ ] Gate xanh: `make lint-api test-api-unit api-docs`, `make test-api` (chạy riêng), `make lint-web test-web`, `make e2e-isolated E2E_ARGS="tasks-board"`; docs `frontend-guidelines`, `api-guidelines`, `architecture` cập nhật.

## Rủi ro & Rollback

| Rủi ro | Giảm thiểu |
|--------|-----------|
| Khôi phục việc vào cột đã bị xoá | Đã kiểm chứng: `MoveAllToColumn` (`task_repository.go:238-251`) không lọc `deleted_at` → hàng soft-deleted cũng dời cột; Phase 1 thêm integration test khoá hành vi này |
| Query lọc chậm khi center nhiều việc | Predicate lọc nằm trong subquery đã có `idx_tasks_board`/`idx_tasks_assignee`; `counts` là một aggregate có `FILTER (WHERE …)` trên cùng predicate `Visibility`; nếu EXPLAIN cho thấy seq scan trên `due_on`, thêm index partial `(center_id, due_on) WHERE deleted_at IS NULL AND completed_at IS NULL` trong cùng migration 000024 |
| Mutation lạc quan trên board đang lọc | Việc vừa tạo/sửa có thể không khớp filter cho tới khi `invalidateBoard` refetch (onSettled đã có); chấp nhận nháy ≤ 1 round-trip, ghi docs |
| Thay đổi HvModal làm hỏng modal khác | `stickyFooter` opt-in (D6); test kit hiện có giữ nguyên hành vi mặc định |
| **Rollback**: sao lưu DB trước migrate; `down.sql` bỏ CHECK + cột | Một commit mỗi phase → revert theo phase; Phase 5 tắt được bằng cách không render dải lọc |
| e2e đỏ giữa các phase | Locator "Chuyển cột" → "Thao tác" đổi ngay trong Phase 4; helper `topCardTitles` dựa vào `<p>` đầu là tiêu đề — giữ tiêu đề là `<p>` đầu tiên trong card |

## Câu hỏi mở

Không còn. Ba câu hỏi ban đầu đã được user chốt ngày 2026-09-17 và ghi vào D3 (lọc phía server), D11 (UI theo report 100%), D12 (assignee đổi "Cột" trong modal).

## Validation Log

- 2026-09-17 · Tier Full (6 phase) · Kiểm chứng bằng grep/sed trên source: `ports.go:111` (`Get` tasks), `service.go:340-348` (`DeleteTask`→`SoftDelete`), `task_repository.go:238-251` (`MoveAllToColumn` gom soft-deleted), core `CreateColumn(ctx,tenant,actor,name,isDone)` có 1 caller adapter (`tasks/service.go:100`), routespec tests `TestMutatingRoutesHaveAnAuditSource`/`TestLookupFindsEveryDeclaredRoute`, migration mới nhất `000023`, `isOverdue` dùng `toISOString()` (`task-card-styles.ts:25`), `useSearchParams` đã dùng ở collections/notifications/sessions, token `ink-500 #5b756c`/`sky-50 #eaf5fb`/`sun-100 #fff4d6`, make targets `test-api-unit|test-api|lint-api|api-docs|test-web|lint-web|e2e-isolated|migrate-up|migrate-down` tồn tại, 4 spec e2e `tasks-board*`. Sửa sau kiểm: số dòng D2/D8, rủi ro "cột đã xoá" hạ xuống đã đóng, tên file integration test API.
- 2026-09-17 · Cập nhật theo quyết định user (D3, D11, D12). Kiểm chứng thêm: `handler.go:56-67` không đọc query param nào; `service.go:87-91` echo scope từ policy; `task_repository.go:32-71` `ListBoard` dùng `row_number() OVER (PARTITION BY column_id)` — predicate lọc chèn được vào `WHERE` của subquery; `policy.go:47-49` `CanMoveTask` gồm assignee; `routespec.go:380` move cần `tasks.edit`; `task-board-page.test.tsx:85-107` hai test segmented cần thay; MSW `tasks-handlers.ts:154-156` đọc `scope` → đổi sang `filter`/`assignee`/`today`; `sessions/service.go:556-565` là tiền lệ đọc `teachers.Timezone` — **không** tái dùng cho board (cross-feature), client gửi `today` thay thế.
- Chưa kiểm (để lúc triển khai): icon lucide `MoreHorizontalIcon`/`Columns2Icon`/`ChevronsLeftIcon` (node_modules bị chặn đọc), tỉ lệ tương phản chính xác (script ở Phase 5 bước 12).

<!-- slug: task-modal-a-board-b -->
