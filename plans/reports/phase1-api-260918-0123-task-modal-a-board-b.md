# Báo cáo Phase 1 — API: migration màu cột, board filter/counts, restore task

## Trạng thái các gate

| Gate | Kết quả | Tóm tắt |
|---|---|---|
| `make lint-api` | PASS | `0 issues.` |
| `make test-api-unit` | PASS | Toàn bộ package `ok`/`(cached)`, không có `FAIL`; `scopelint` chạy sạch. |
| `make api-docs` | PASS | Regenerate `docs.go`/`swagger.json`/`swagger.yaml` thành công; xác nhận có `/tasks/{id}/restore` (swagger.json:13579) và trường `color` xuất hiện ở 4 schema tasks/kanban liên quan (dòng 16395, 16473, 16513, 16648). |
| `make test-api` (integration, một mình) | PASS | 40/40 package `ok`, không `FAIL` nào. `total coverage: 77.3% (floor 60%)` — vượt sàn coverage. |

Lần chạy `test-api` đầu tiên có 1 package đỏ (`teka/apps/api/migrations`, test `TestDownFoldsPersonalChannelIntoManual`) — xem mục Sai lệch bên dưới; sau khi sửa, chạy lại full `make test-api` sạch hoàn toàn, không container test nào còn sót lại (`docker ps` chỉ còn các container production `teka-api-1`/`teka-web-1`/`teka-db` không liên quan).

## Backup DB dev + migrate up/down

Không có dev DB thật có dữ liệu trong môi trường này. Đã dùng một Postgres scratch (container tạm, `docker compose ... down -v` sau khi xong — không phải dev DB thật nên không cần lệnh backup `pg_dump`) để kiểm `make migrate-up` rồi `make migrate-down` cho `000024_task_column_color`:
- `migrate-up`: `schema_migrations.version=24, dirty=false`; `\d task_columns` cho thấy `color VARCHAR(16) NOT NULL DEFAULT 'none'` cùng constraint `ck_task_columns_color`.
- `migrate-down`: xác nhận sạch, cột `color` và constraint biến mất, `version=23`.
Cả hai chiều đều chạy sạch, không lỗi.

## EXPLAIN và index partial

Có chạy. Đã seed ~1000 dòng `tasks`/tenant vào Postgres scratch nói trên và chạy `EXPLAIN ANALYZE` cho 3 dạng query thật của `ListBoard`/`CountBoard`. Postgres luôn chọn `Index Scan` trên các index có sẵn dẫn đầu bằng `center_id` (`idx_tasks_creator`, `idx_tasks_assignee`), áp `due_on`/`completed_at`/`deleted_at` như `Filter:` sau scan — không thấy `Seq Scan` ở quy mô này. Vì vậy **không thêm** index partial `idx_tasks_open_due` vào `000024` — quyết định có bằng chứng thực nghiệm, không phải suy đoán.

## Các file cross-cutting đã sửa ngoài danh sách sở hữu gốc — vì sao

1. **`internal/features/audit/action_test.go`** và **`internal/server/route_policy_snapshot_test.go`**: hai file này giữ snapshot thủ công (liệt kê cứng) khớp 1-1 với `routespec.Specs`. Thêm route mới `POST /tasks/:id/restore` vào `routespec.go` (file tôi sở hữu) tất yếu làm `TestActionSnapshotUnchanged` và `TestRoutePolicySnapshotUnchanged` đỏ vì thiếu dòng tương ứng. Đây là hệ quả cơ học bắt buộc, không phải mở rộng phạm vi — đã thêm đúng 1 dòng mỗi file (`task.restore`/`tasks.delete` permission, khớp cách `DELETE /tasks/:id` dùng).
2. **`apps/api/migrations/migrations_test.go`**: `TestDownFoldsPersonalChannelIntoManual` hardcode số bước rollback (`MigrateDown(m, 19)`) dựa trên tổng số migration hiện có tại thời điểm viết test, với comment giải thích rõ con số này phải tăng theo mỗi migration cộng thêm. Thêm `000024` (migration tôi sở hữu) làm tổng số migration tăng lên 24, khiến 19 bước rollback dừng sớm 1 bước (dừng ở version 5 thay vì 4), không undo được migration `zalo_personal_mapping` — test đỏ (`expected: "zalo_manual", actual: "zalo_personal"`) dù chủ đề (Zalo channel) không liên quan gì đến board/color/restore. Đã sửa `19 → 20` bước và cập nhật comment (`000008-000023` → `000008-000024`); chạy lại `test-api` xác nhận `teka/apps/api/migrations` xanh.
3. **`move_integration_test.go`**: sửa cơ học trong phiên trước (đã báo) — các lệnh gọi `e.svc.Board(ctx, scope)` cần đổi thành `e.svc.Board(ctx, scope, tasks.BoardQuery{})` vì file này là `package tasks_test` (external), không thấy được `BoardQuery{}` trần trụi từ package `tasks`.
4. **`handler_test.go`** (file mới, không phải sửa file có sẵn): `boardFilterFrom`'s switch không có nhánh `default` lỗi — nghĩa là validate 422 cho `filter`/`assignee`/`today` sai định dạng chỉ xảy ra ở tầng Gin binding (`ShouldBindQuery`), không lộ qua được nếu gọi thẳng `Service.Board`. Viết unit test binding nhẹ (`gin.CreateTestContext` + `ShouldBindQuery`) thay vì dựng scaffolding HTTP/DB/auth không cần thiết để chứng minh 422 cho input sai.

Cả 4 điều chỉnh trên đều là hệ quả cơ học, tất yếu của thay đổi trong phạm vi sở hữu (route mới, migration mới, kiểu package test có sẵn) — không có thay đổi hành vi nào ngoài phạm vi phase.

## Files thay đổi/tạo mới

**Sở hữu, sửa**: `apps/api/pkg/kanban/{entity,columns,ports,service,service_test}.go`, `README.md`; `apps/api/internal/features/tasks/{dto,errors,handler,model,routes,service,service_test,task_repository,column_repository}.go`, `board_integration_test.go`, `column_delete_integration_test.go`, `rbac_integration_test.go`, `move_integration_test.go`; `apps/api/internal/features/centers/default_columns.go`; `apps/api/internal/shared/routespec/routespec.go`; `apps/api/docs/{docs.go,swagger.json,swagger.yaml}` (qua `make api-docs`).

**Sở hữu, mới**: `apps/api/migrations/000024_task_column_color.{up,down}.sql`; `apps/api/internal/features/tasks/{board_filter_integration_test.go,handler_test.go,restore_integration_test.go}`.

**Ngoài sở hữu, sửa vì hệ quả cơ học (đã giải trình ở trên)**: `apps/api/internal/features/audit/action_test.go`, `apps/api/internal/server/route_policy_snapshot_test.go`, `apps/api/migrations/migrations_test.go`.

Không commit — controller session sẽ commit theo phase.

---

Status: DONE
Summary: Cả 4 gate (lint-api, test-api-unit, api-docs, test-api) đều PASS; migrate-up/down sạch cả hai chiều trên Postgres scratch; đã chạy EXPLAIN thực nghiệm và quyết định không thêm index partial; một regression thật (migrations_test.go) do migration 000024 làm lệch số bước rollback đã được phát hiện và sửa.
Concerns/Blockers: Không có blocker. Lưu ý duy nhất: 3 file ngoài danh sách sở hữu gốc đã bị sửa như hệ quả cơ học bắt buộc (liệt kê ở trên) — cần controller xác nhận không xung đột với agent khác trước khi commit.
