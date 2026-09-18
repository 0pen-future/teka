---
phase: 1
title: "API + migration: màu cột (color), lọc/đếm board, khôi phục việc (restore)"
status: pending
priority: P1
effort: "1.5d"
dependencies: []
---

# Phase 1: API + migration — `task_columns.color`, lọc/đếm `GET /tasks/board`, `POST /tasks/:id/restore`

## Context Links

- Report: `plans/reports/ui-redesign-260917-2117-task-board-and-task-modal.html` (Bảng B: cột tô màu theo giai đoạn; Modal A: "Hoàn tác" sau xoá)
- Quyết định D1, D2, D3 trong [plan.md](./plan.md)
- Migration gốc: `apps/api/migrations/000022_task_board.up.sql`; mới nhất hiện tại: `000023_task_description_html`
- Core: `apps/api/pkg/kanban/{entity.go,ports.go,service.go,columns.go}`; ranh giới import: `apps/api/pkg/kanban/import_boundary_test.go`
- Adapter: `apps/api/internal/features/tasks/{model.go,dto.go,service.go,handler.go,routes.go,column_repository.go,task_repository.go,errors.go}`
- Route policy: `apps/api/internal/shared/routespec/routespec.go:354-381`
- Board hiện tại: `handler.go:56-67` (không đọc query param), adapter `service.go:52-92` (`boardTasksPerColumn+1`, echo `scope`), core `service.go:48-61` (`Board`), repo `task_repository.go:32-71` (`ListBoard` với `row_number() OVER (PARTITION BY column_id)`), `Visibility` ở `ports.go:21-33`
- Default columns: `apps/api/internal/features/centers/default_columns.go`; parity test `apps/api/migrations/backfill_parity_test.go:162-215`

## Overview

Ba bổ sung độc lập về dữ liệu nhưng cùng lớp:

1. **Màu cột** — thuộc tính `color` trên `task_columns` để web tô nền cột
   (`none` = cream mặc định, `sky` = Đang làm, `sun` = Chờ duyệt, `mint` =
   Hoàn thành). Đi xuyên core → repo → DTO → default columns.
2. **Lọc + đếm board** (D3) — `GET /tasks/board` nhận `filter`, `assignee`,
   `today`; trả cột đã lọc (vẫn cắt 50/cột + `has_more`) và `counts` đếm trên
   toàn bộ tập nhìn thấy. Bỏ tham số `scope` chết.
3. **Khôi phục việc** — endpoint idempotent-ish `POST /tasks/:id/restore` xoá
   `deleted_at`, trả `TaskResponse`; dùng cho "Hoàn tác" sau khi xoá ở Modal A.

## Key Insights

- `pkg/kanban` chỉ được import stdlib + uuid và không mang ngôn ngữ/sản phẩm →
  core giữ `Color string` **không enum**; enum sống ở adapter (`binding:"oneof"`)
  và DB CHECK. Core chỉ chặn độ dài (> 16 rune → `ErrInvalidInput`).
- `TaskRepository.Get` loại soft-deleted; `DeleteTask` chỉ gọi `SoftDelete`
  (`service.go:340-349`). Restore cần cặp port mới `GetDeleted` + `Restore`
  để không nới lỏng hợp đồng `Get`.
- Cột của một việc đã xoá luôn tồn tại: FK `fk_tasks_column_center` RESTRICT
  chặn xoá cột còn hàng kể cả soft-deleted (`errors.go:33`), và
  `MoveAllToColumn` (`task_repository.go:238-251`) không lọc `deleted_at` nên
  `DELETE /task-columns/:id?move_to=` dời cả hàng đã xoá. Restore vì thế không
  cần xử lý "cột mất"; bước 9 chỉ thêm test khoá hành vi.
- Parity test default-columns chỉ so `(name, position, is_done)` bằng regex
  `\('([^']+)', *(\d+), *(TRUE|FALSE)\)` → thêm `Color` vào spec Go không phá
  test; **không** sửa block SQL đã đóng băng ở 000022.
- Request-log audit lấy `action` từ routespec (`req("task.restore","task","id")`),
  không cần event riêng như `ColumnDeleted`.
- **Lọc phải đi qua core**: `Visibility` do `Policy` sinh và chỉ repo dịch nó
  thành SQL (plan 260913 D3 — repo không đọc `Actor`). Bộ lọc vì thế là một
  giá trị thuần `BoardFilter` core truyền xuống `ListBoard`, và đếm là port mới
  `CountBoard(ctx, tenant, vis, today)` cũng nhận `Visibility` — không có
  đường tắt từ adapter thẳng xuống DB bỏ qua luật đọc.
- `ListBoard` (limit > 0) đã là subquery `row_number() OVER (PARTITION BY
  column_id …) WHERE center_id = ? AND deleted_at IS NULL AND (vis…)` → predicate
  lọc chỉ cần thêm vào `WHERE` bên trong, `rn <= limit` vẫn cắt sau khi lọc nên
  `has_more` đúng theo tập đã lọc.
- "Hôm nay" là **ngày lịch của client** (D8): server không biết múi giờ người
  xem; `sessions` đọc `teachers.Timezone` nhưng kéo phụ thuộc cross-feature vào
  `tasks` là không đáng. Client gửi `today=YYYY-MM-DD`; thiếu → ngày UTC hiện tại.
- `counts` chỉ đếm việc **chưa xong** (`completed_at IS NULL`) cho mọi khoá kể
  cả `all`, khớp phụ đề Bảng A ("N việc" = việc chưa xong, Phase 4). Riêng bộ lọc
  `mine`/`assignee` **vẫn trả** việc đã xong của người đó (để thấy ở cột Hoàn
  thành, đúng demo report); `overdue`/`today`/`unassigned` chỉ trả việc chưa xong.

## Requirements

Functional
- `color ∈ {none, sky, sun, mint}`, mặc định `none`; `PATCH /task-columns/:id` và
  `POST /task-columns` nhận `color` tuỳ chọn; mọi `ColumnResponse` trả `color`.
- Migration backfill: `is_done = TRUE` → `mint`; `NOT is_done AND name = 'Đang làm'` → `sky` (khớp bộ cột mặc định đã seed); còn lại `none`.
- Center mới: `Cần làm` none · `Đang làm` sky · `Hoàn thành` mint.
- `GET /tasks/board` query: `filter ∈ {all, mine, overdue, today, unassigned}`
  (mặc định `all`; khác → 422), `assignee` uuid tuỳ chọn (AND với `filter`;
  không phải uuid → 422), `today` `YYYY-MM-DD` tuỳ chọn (sai định dạng → 422).
  Predicate: `mine` = `assignee_id = actor`; `overdue` = `due_on < today AND
  completed_at IS NULL`; `today` = `due_on = today AND completed_at IS NULL`;
  `unassigned` = `assignee_id IS NULL AND completed_at IS NULL`; `assignee=X` =
  `assignee_id = X`. Tất cả AND với `Visibility`.
- `BoardResponse.counts`: `{ all, mine, overdue, today, unassigned, by_assignee:
  [{ teacher_id, count }] }` — đếm việc chưa xong trong tập `Visibility`,
  **không** phụ thuộc `filter`/`assignee`, không bị cắt 50/cột; `by_assignee`
  chỉ gồm người có count > 0, sắp theo count giảm dần rồi `teacher_id`.
- Tham số `scope` không còn được web gửi; server vẫn echo `scope` (đã có) để
  client biết tầm nhìn hiệu lực.
- `POST /tasks/:id/restore`: yêu cầu `tasks.delete`; policy `CanWriteTask`
  (owner/creator — cùng tầng với xoá); 404 khi không có hàng soft-deleted khớp
  tenant; giữ `column_id`, `position`, `completed_at` như trước khi xoá; trả 200
  `TaskResponse`.

Non-functional
- Không đổi hành vi endpoint hiện có khi client cũ không gửi `color`.
- Swagger cập nhật (`make api-docs`), routespec test xanh, unit + integration xanh.

## Architecture

```
migration 000024 ──► task_columns.color
kanban.Column.Color (string, opaque) ──► columnModel.Color ──► ColumnResponse.Color
kanban.DefaultColumnSpec.Color ──► centers.defaultColumnSpecs
kanban.BoardFilter{AssigneeID *ActorID, Unassigned, DueBefore *Date, DueOn *Date, OpenOnly} ──► TaskRepository.ListBoard(ctx, tenant, vis, filter, limit)
kanban.Service.BoardCounts(ctx, tenant, actor, today) ──► TaskRepository.CountBoard(ctx, tenant, vis, today) ──► BoardCountsResponse
handler.board: c.ShouldBindQuery(BoardQuery{Filter, Assignee, Today}) ──► service.Board(ctx, scope, query)
kanban.Service.RestoreTask ──► TaskRepository.GetDeleted / Restore ──► handler.restoreTask
routespec: POST /api/v1/tasks/:id/restore · PermTasksDelete · req("task.restore","task","id")
```

## Related Code Files

Tạo mới
- `apps/api/migrations/000024_task_column_color.up.sql`, `.down.sql`

Sửa
- `apps/api/pkg/kanban/entity.go` (`Column.Color`, `ColumnPatch.Color *string`), `columns.go` (`DefaultColumnSpec.Color`, `validateColor`), `ports.go` (`BoardFilter`, `BoardCounts`, `ListBoard` thêm `filter`, `CountBoard`, `GetDeleted`, `Restore`), `service.go` (`Board` nhận `filter`; `BoardCounts`; `CreateColumn` nhận color; `UpdateColumn` áp `patch.Color`; `RestoreTask`), `service_test.go` + fakes (`fakeTaskRepo.ListBoard` áp filter thuần, `CountBoard`, `GetDeleted/Restore`), `README.md` (mục use-case + port)
- `apps/api/internal/features/tasks/model.go` (`columnModel.Color`, map to/from core), `column_repository.go` (`Create`, `Update` thêm `color`), `task_repository.go` (`ListBoard` áp `filter`, `CountBoard`, `GetDeleted`, `Restore`), `dto.go` (`BoardQuery`, `BoardCountsResponse`, `BoardResponse.Counts`, `ColumnResponse.Color`, `CreateColumnRequest.Color *string`, `UpdateColumnRequest.Color *string` với `binding:"omitempty,oneof=none sky sun mint"`), `service.go` (`Board` nhận query → filter core + counts; map color; `RestoreTask`), `handler.go` (`board` bind query + swag `@Param`; `restoreTask` + swag), `routes.go`, `errors.go` (`23514` `ck_task_columns_color` → Invalid), `service_test.go`, `restore_integration_test.go` (mới), `board_filter_integration_test.go` (mới), `board_integration_test.go`, `column_delete_integration_test.go`, `rbac_integration_test.go`
- `apps/api/internal/features/centers/default_columns.go`
- `apps/api/internal/shared/routespec/routespec.go`
- `apps/api/docs/*` (sinh bởi `make api-docs`)

## Implementation Steps

1. **Migration** — viết `000024_task_column_color.up.sql`:
   ```sql
   ALTER TABLE task_columns ADD COLUMN color VARCHAR(16) NOT NULL DEFAULT 'none';
   ALTER TABLE task_columns ADD CONSTRAINT ck_task_columns_color
     CHECK (color IN ('none', 'sky', 'sun', 'mint'));
   UPDATE task_columns SET color = 'mint' WHERE is_done;
   UPDATE task_columns SET color = 'sky' WHERE NOT is_done AND name = 'Đang làm';
   ```
   `down.sql`: `DROP CONSTRAINT ck_task_columns_color; DROP COLUMN color`. Chạy
   `make migrate-up`/`down` trên DB dev; sao lưu trước khi chạy ở nơi có dữ liệu.
2. **Core entity** — thêm `Color string` vào `Column`; `ColumnPatch.Color *string`;
   `DefaultColumnSpec.Color`; `DefaultColumns` sao chép color. `validateColor`:
   trim, rỗng → giữ nguyên giá trị hiện tại (adapter điền `none`), > 16 rune →
   `ErrInvalidInput`.
3. **Core use-case cột** — `CreateColumn` nhận color (mở rộng chữ ký hiện có, giữ
   thứ tự tham số cũ + `color string` cuối); `UpdateColumn` áp `patch.Color`
   sau validate. Cập nhật fakes + test: tạo cột có color, patch color, patch quá dài.
4. **Core restore** — `ports.go`: `GetDeleted(ctx, tenant, id) (Task, error)` (chỉ
   trả hàng có `deleted_at`), `Restore(ctx, tenant, id) error`. `service.go`:
   ```go
   func (s *Service) RestoreTask(ctx, tenant, actor, id) (Task, error) {
       task, err := s.repos.Tasks.GetDeleted(ctx, tenant, id) // ErrTaskNotFound nếu sống hoặc không có
       if !s.policy.CanWriteTask(actor, task) { return ErrForbidden }
       uow.Within: Restore → Get
   }
   ```
   Test: restore thành công; việc đang sống → `ErrTaskNotFound`; assignee không
   phải creator → `ErrForbidden`.
4b. **Core board filter + counts** — `ports.go`:
   ```go
   // BoardFilter narrows ListBoard beyond Visibility. Zero value = no filter.
   type BoardFilter struct {
       AssigneeID *ActorID   // assignee_id = X
       Unassigned bool       // assignee_id IS NULL
       DueBefore  *time.Time // due_on < date (calendar date, time part ignored)
       DueOn      *time.Time // due_on = date
       OpenOnly   bool       // completed_at IS NULL
   }
   type BoardCounts struct {
       All, Overdue, Today, Unassigned int
       ByAssignee map[ActorID]int // open tasks per assignee, absent when 0
   }
   ListBoard(ctx, tenant, vis, filter BoardFilter, limit int) ([]Task, error)
   CountBoard(ctx, tenant, vis, today time.Time) (BoardCounts, error)
   ```
   `service.go`: `Board(ctx, tenant, actor, filter, limitPerColumn)`;
   `BoardCounts(ctx, tenant, actor, today)` — cả hai gọi `s.policy.Visibility`
   rồi xuống repo. `mine` **không** nằm trong core (là `ByAssignee[actor.ID]`,
   adapter suy ra). Fake repo trong `service_test.go` áp filter bằng vòng lặp
   thuần; test: filter `DueBefore` loại việc đã xong khi `OpenOnly`, `Unassigned`,
   `AssigneeID`; `CountBoard` với participant chỉ đếm việc của mình. README:
   thêm `BoardFilter`/`CountBoard` vào bảng port, ghi "date-only comparison".
5. **Adapter model/repo** — `columnModel.Color`; `columnToCore/FromCore`;
   `columnRepository.Create` chèn color, `Update` thêm `"color": col.Color`.
   `taskRepository.GetDeleted` (`WHERE id=? AND center_id=? AND deleted_at IS NOT NULL`),
   `Restore` (`UPDATE tasks SET deleted_at = NULL, updated_at = now() WHERE ... AND deleted_at IS NOT NULL`, RowsAffected=0 → `ErrTaskNotFound`).
5b. **Repo lọc + đếm** — `ListBoard`: dựng chuỗi predicate từ `BoardFilter`
   (`assignee_id = ?`, `assignee_id IS NULL`, `due_on < ?::date`, `due_on = ?::date`,
   `completed_at IS NULL`) và chèn vào **cả hai** nhánh (limit ≤ 0 dùng gorm
   `Where`, limit > 0 nối vào `WHERE` bên trong subquery `ranked`, trước
   `row_number`). `CountBoard`: một câu
   ```sql
   SELECT count(*) FILTER (WHERE TRUE) AS all,
          count(*) FILTER (WHERE due_on < ?::date) AS overdue,
          count(*) FILTER (WHERE due_on = ?::date) AS today,
          count(*) FILTER (WHERE assignee_id IS NULL) AS unassigned
   FROM tasks WHERE center_id = ? AND deleted_at IS NULL AND completed_at IS NULL
     AND (? OR created_by = ? OR assignee_id = ?)
   ```
   cộng `SELECT assignee_id, count(*) … GROUP BY assignee_id` cùng predicate cho
   `ByAssignee` (2 query, cùng `Visibility`). Chạy `EXPLAIN` trên DB dev với
   ~1k việc; nếu seq scan theo `due_on` đáng kể → thêm index partial
   `idx_tasks_open_due (center_id, due_on) WHERE deleted_at IS NULL AND completed_at IS NULL`
   vào `000024` (up + down).
5c. **DTO + service board** — `dto.go`:
   ```go
   type BoardQuery struct {
       Filter   string `form:"filter" binding:"omitempty,oneof=all mine overdue today unassigned"`
       Assignee string `form:"assignee" binding:"omitempty,uuid"`
       Today    string `form:"today" binding:"omitempty,datetime=2006-01-02"`
   }
   type AssigneeCountResponse struct { TeacherID uuid.UUID `json:"teacher_id"`; Count int `json:"count"` }
   type BoardCountsResponse struct { All, Mine, Overdue, Today, Unassigned int; ByAssignee []AssigneeCountResponse `json:"by_assignee"` }
   // BoardResponse thêm Counts BoardCountsResponse `json:"counts"`
   ```
   `service.Board(ctx, sc, q BoardQuery)`: `today` = parse hoặc `time.Now().UTC()`
   cắt về ngày; dựng `kanban.BoardFilter` từ `q.Filter`/`q.Assignee` (bảng ánh xạ
   trong Requirements); gọi `core.Board(..., filter, boardTasksPerColumn+1)` và
   `core.BoardCounts(..., today)`; `Mine = ByAssignee[actor.ID]`; `ByAssignee`
   sắp count desc rồi id; `by_assignee` luôn là mảng (không `null`).
   `handler.board`: `c.ShouldBindQuery(&q)` → lỗi → `apperror.Invalid`; swag
   `@Param filter query string false "all|mine|overdue|today|unassigned"`,
   `@Param assignee query string false "" format(uuid)`, `@Param today query string false "YYYY-MM-DD, client's calendar date"`.
6. **DTO + service adapter (cột)** — `ColumnResponse.Color json:"color"`;
   `CreateColumnRequest.Color *string binding:"omitempty,oneof=none sky sun mint"`;
   `UpdateColumnRequest.Color` tương tự; service map `nil → "none"` khi tạo.
   `Service.RestoreTask` gọi core rồi `taskFromCore`.
7. **Handler/routes/routespec** — `restoreTask` với swag
   `@Router /tasks/{id}/restore [post]`, `@Success 200 {object} response.Envelope{data=TaskResponse}`,
   `@Failure 403/404`; `routes.go` đăng ký; routespec thêm
   `perm("POST", "/api/v1/tasks/:id/restore", authctx.PermTasksDelete, req("task.restore", "task", "id"))`.
   Chạy routespec test (`TestLookupFindsEveryDeclaredRoute`, `TestMutatingRoutesHaveAnAuditSource`).
8. **errors.go** — thêm case `23514` (check_violation) với
   `ck_task_columns_color` → `apperror.Invalid(..., {"color": "phải là none, sky, sun hoặc mint"})`
   (phòng client bỏ qua binding).
9. **Khoá hành vi `MoveAllToColumn` với hàng soft-deleted** — thêm case vào
   `column_delete_integration_test.go`: xoá việc → xoá cột với `move_to` →
   restore việc → việc nằm ở cột đích (không sửa repo; đã kiểm là không lọc
   `deleted_at`).
10. **Default columns** — `centers.defaultColumnSpecs` thêm Color (none/sky/mint);
    chạy `go test ./apps/api/migrations/...` (parity) + `./internal/features/centers/...`.
11. **Integration tests** (theo mẫu `*_integration_test.go` hiện có): thêm
    `restore_integration_test.go` (delete → restore → 200 và board hiện lại;
    restore lần 2 → 404; assignee không phải creator → 403 — có thể đặt case
    403 trong `rbac_integration_test.go` cùng bảng quyền); mở rộng
    `board_integration_test.go`: tạo cột `color: "sun"` → 201 có color,
    patch `color: "pink"` → 422, board trả `color`. Thêm
    `board_filter_integration_test.go`: seed việc `due_on` hôm qua / hôm nay /
    không hạn, có/không assignee, một việc đã xong quá hạn; (a) `filter=overdue&today=…`
    chỉ trả việc chưa xong quá hạn; (b) `filter=today`; (c) `filter=unassigned`;
    (d) `filter=mine` trả cả việc đã xong của tôi; (e) `assignee=X&filter=overdue`
    AND; (f) `filter=bogus` / `assignee=abc` / `today=17-09-2026` → 422; (g)
    `counts` giống nhau dù `filter` khác, `counts.all` = số việc chưa xong,
    `by_assignee` sắp theo count desc, không có người count 0; (h) 51 việc một cột
    → `has_more=true` nhưng `counts.all` = 51; (i) teacher không `view_all`:
    `counts` chỉ đếm việc mình tạo/được giao.
12. **Gates** — `make lint-api`, `make test-api-unit`, `make api-docs`, rồi
    `make test-api` **chạy riêng** (serial, `-p 1`).

## Todo

- [ ] 000024 up/down + backfill
- [ ] Core: `Column.Color`, `ColumnPatch.Color`, `DefaultColumnSpec.Color`, `validateColor`, `CreateColumn`/`UpdateColumn`
- [ ] Core: `BoardFilter`, `BoardCounts`, `ListBoard(filter)`, `CountBoard`, `Board(filter)`, `BoardCounts` + tests + README
- [ ] Adapter: `ListBoard` predicate cả hai nhánh, `CountBoard` (2 query), EXPLAIN + index partial nếu cần
- [ ] DTO `BoardQuery` (binding), `BoardCountsResponse`, `BoardResponse.Counts`; handler bind query + swag
- [ ] Core: `GetDeleted`/`Restore` ports + `RestoreTask` + tests + README
- [ ] Adapter: model, column repo, task repo (`GetDeleted`, `Restore`); test khoá `MoveAllToColumn` với hàng soft-deleted
- [ ] DTO `color` + binding; service map; `errors.go` 23514
- [ ] Handler `restoreTask` + swag; routes; routespec; audit action `task.restore`
- [ ] `centers/default_columns.go` colors; parity test xanh
- [ ] Integration tests (color 201/422, restore 200/404/403, restore sau xoá cột, board filter a–i)
- [ ] `make lint-api test-api-unit api-docs`; `make test-api` riêng

## Success Criteria

- Migration lên/xuống sạch trên DB dev; `\d task_columns` có `color` + CHECK.
- `GET /tasks/board` mọi cột có `color`; center mới có `Đang làm=sky`, `Hoàn thành=mint`.
- `GET /tasks/board?filter=overdue&today=2026-09-17` chỉ trả việc chưa xong quá hạn; `counts` bất biến theo `filter` và đúng khi cột > 50 việc; giá trị query sai → 422.
- `POST /tasks/:id/restore` đúng 200/404/403 như test; bảng audit có `task.restore`.
- Swagger có route mới và field `color`; toàn bộ gate API xanh.

## Risk Assessment

- **Chữ ký `CreateColumn` core đổi** → mọi caller (adapter, seed, test) phải cập
  nhật; grep `CreateColumn(` trước khi sửa.
- **Backfill theo tên `'Đang làm'`** chỉ đúng với bộ cột mặc định; cột người
  dùng tự đặt vẫn `none` — chấp nhận, chỉnh tay qua Cấu hình cột (Phase 5).
- **Chữ ký `ListBoard` core đổi** → 2 fake (`pkg/kanban/service_test.go:164`,
  `features/tasks/service_test.go:118`) + repo thật; grep `ListBoard(` trước.
- **Hai query cho `counts`** cộng một cho board = 3 round-trip mỗi lần tải bảng;
  chấp nhận ở quy mô center (≤ vài nghìn việc). Không gộp vào một CTE để giữ
  `ListBoard` không đổi hình dạng.

## Security Considerations

- Restore gate bằng `tasks.delete` ở route + `CanWriteTask` ở core: người có
  quyền xoá mới hoàn tác xoá; assignee thuần không thể "hồi sinh" việc người khác.
- Cross-tenant id → `ErrTaskNotFound` (404), giống `Get`.
- `color` bị ràng ở cả binding và DB → không có đường chèn giá trị lạ.
- `assignee=<uuid>` chỉ là predicate thêm **bên trong** `Visibility`: người không
  có `view_all` truyền id người khác vẫn chỉ thấy việc mình tạo/được giao (test i).
  `counts.by_assignee` cũng nằm trong `Visibility` nên không rò số việc của người
  khác cho assignee thuần.

## Next Steps

Phase 2 (kit) độc lập; Phase 3 tiêu thụ `restore`; Phase 4 tiêu thụ `counts` (phụ đề); Phase 5 tiêu thụ `color`, `filter`/`assignee`/`today`, `counts`.
