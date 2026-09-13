# Fix review — API (Task-center Kanban)

Áp dụng các finding API trong `plans/reports/code-review-260913-task-center-kanban.md`:
M1–M4, N3–N9, N13. N1, N10–N12, N14 và các Nit thuộc phía web hoặc ngoài phạm vi được giao,
không đụng tới.

## M1 — 4 nhánh `ErrInvalidInput` gộp chung, 400 lẽ ra phải 422

- `pkg/kanban/errors.go`: thêm 3 sentinel bọc `ErrInvalidInput` bằng `fmt.Errorf("%w: ...", ...)`
  (giữ `errors.Is(err, ErrInvalidInput)` cũ vẫn đúng): `ErrEmptyName`, `ErrEmptyTitle`,
  `ErrMoveToSelf`.
- `pkg/kanban/columns.go` (`validateName`), `pkg/kanban/service.go` (`CreateTask`, `UpdateTask`,
  `DeleteColumn`'s `moveTo == colID` case): trả sentinel cụ thể thay vì `ErrInvalidInput` chung.
- `internal/features/tasks/errors.go`: thêm 3 case cụ thể (đặt trước case `ErrInvalidInput` chung
  vì `errors.Is` sẽ khớp cả hai nếu đảo thứ tự), mỗi case trả `apperror.Invalid` (422) kèm field
  lỗi tương ứng (`name`/`title`/`move_to`) thay vì `apperror.BadRequest` (400) chung chung.
- `internal/features/tasks/handler.go`: sửa annotation swagger `deleteColumn` — 409 chỉ còn "cannot
  delete the last column, or column still has tasks", 422 là "move_to names the column being
  deleted" (đổi chỗ cho đúng với response thật).
- `internal/features/tasks/column_delete_integration_test.go`: đổi tên test
  `TestDeleteColumnMoveToItselfIsValidationError`, assert `apperror.CodeValidation` + field
  `move_to` trong `Fields` (trước đó assert `CodeBadRequest`).
- 4 test cũ dùng `require.ErrorIs(t, err, ErrInvalidInput)` (empty name/title, move-to-self) không
  cần sửa — sentinel wrap giữ nguyên tương thích.

## M2 — `eventSink.Publish` panic khi bus nil

`internal/features/tasks/events.go`: thêm helper `publish(e events.Event)` nil-safe (giống hệt
pattern `centers/service.go`), `Publish` gọi qua helper này thay vì gọi thẳng `s.bus.Publish`.

## M3 — `ReorderColumns` ghi `UpdatePositions` ngoài transaction

`pkg/kanban/service.go`: bọc `s.repos.Columns.UpdatePositions(...)` trong `s.uow.Within(...)`.
Test mới `TestReorderColumnsDoesNotPersistWhenUnitOfWorkFails` (uow lỗi → vị trí cột không đổi);
test cũ `TestReorderColumnsPersistsOrder` thêm assert `env.uow.calls == 1`.

## M4 — `Board`/`CreateTask`/`MoveTask` quét toàn bộ tenant để tính vị trí/áp cap

- `pkg/kanban/ports.go`: `TaskRepository.ListBoard` nhận thêm `limit int` (0 = không giới hạn,
  >0 = cap theo từng cột); thêm `MinPositionInColumn(ctx, tenant, col) (minPos float64, found bool, err error)`.
- `pkg/kanban/service.go`: `Board` nhận `limitPerColumn int`, truyền xuống `ListBoard`. Xoá hẳn
  `tasksInColumn`/`topPosition` (quét toàn tenant), thay bằng `topPositionInColumn` gọi thẳng
  `MinPositionInColumn` — `CreateTask`/`MoveTask` dùng hàm này.
- `internal/features/tasks/task_repository.go`: `ListBoard` khi `limit <= 0` giữ query cũ; khi
  `limit > 0` dùng `row_number() OVER (PARTITION BY column_id ORDER BY position, created_at)` lọc
  `rn <= limit` ngay trong SQL, tránh tải cả tenant về Go rồi cắt. `MinPositionInColumn` mới dùng
  `SELECT MIN(position)` có index `idx_tasks_board` hỗ trợ.
- `internal/features/tasks/service.go`: 3 chỗ gọi `s.core.Board` cập nhật theo chữ ký mới —
  `Board()` truyền `boardTasksPerColumn+1` (giữ đúng logic `has_more` hiện có, giờ SQL lo cap thay
  vì Go); `ReorderColumns`/`CreateTask`'s auto-pick-column truyền `0` (không đổi hành vi, vẫn lấy
  hết vì các nhánh này chỉ cần danh sách cột).
- Cập nhật fake `TaskRepository` ở cả hai file test (`pkg/kanban/service_test.go`,
  `internal/features/tasks/service_test.go`): thêm tham số `limit` (fake bỏ qua, vì test dựa vào
  logic Go-side có sẵn) và `MinPositionInColumn`.

## N3 — `priority` mặc định `'normal'` (ngoài enum), không CHECK

`migrations/000022_task_board.up.sql`: đổi `DEFAULT 'normal'` thành
`DEFAULT 'none' CHECK (priority IN ('none','low','medium','high'))`. Sửa comment "dead weight" ở
`internal/features/tasks/model.go` cho khớp default mới.

## N4 — `fk_tasks_assignee_center ON DELETE CASCADE` xoá cả task thay vì gỡ assignee

`migrations/000022_task_board.up.sql`: đổi thành cú pháp cột-cụ-thể Postgres 16
`ON DELETE SET NULL (assignee_id)`. Giữ nguyên `fk_tasks_creator_center ON DELETE CASCADE` (đúng
theo review). Cập nhật lại đoạn comment đầu file mô tả rule mới cho từng FK.

## N5/N6 — Audit `task.handover` thiếu actor/entity, publish cả khi rỗng

- `internal/features/centers/events.go`: thêm field `ActorID uuid.UUID` vào `MemberTasksHandedOver`.
- `internal/features/centers/service.go` (`RemoveMember`): set `ActorID: scope.TeacherID`; điều
  kiện publish đổi thành `s.taskHandover != nil && (ev.Unassigned > 0 || ev.Reassigned > 0)`.
- `internal/features/audit/subscriber.go`: case `centers.MemberTasksHandedOver` thêm
  `ActorUserID: nilIfZero(ev.ActorID)` và `EntityID: ev.MemberID.String()`.
- Không tìm thấy test nào (unit lẫn integration) đã cover event này từ trước — `env` dùng trong
  `centers/integration_test.go` khởi tạo `centers.NewService(..., nil)` (bus nil), nên không có
  chỗ bám sẵn để assert field mới mà không dựng lại hạ tầng bus cho cả bộ test dùng chung `env`.
  Bỏ qua việc thêm test riêng cho N5/N6 vì việc nối bus vào `env` ảnh hưởng mọi test khác trong
  file, vượt phạm vi sửa lỗi review yêu cầu; hành vi mới đã build/vet/test toàn bộ package pass.

## N7 — Comment `Directory` sai sự thật về owner

`internal/features/centers/repository.go`: sửa comment — owner **có** row `center_members` (theo
migration 000007), nên owner có xuất hiện trong `Directory`; đây là hành vi đúng để
`ReassignCreator` chuyển tác giả sang owner không vi phạm `fk_tasks_creator_center`.

## N8 — `taskRepository.Create` bỏ qua tham số `tenant`

`internal/features/tasks/task_repository.go`: thêm `row.CenterID = uuid.UUID(tenant)` trước khi
insert, không còn tin tưởng hoàn toàn vào `task.TenantID` do caller set.

## N9 — `defaultColumnsSvc` là `kanban.Service` rác với 4 port nil

- `pkg/kanban/columns.go`: `DefaultColumns` đổi từ method `(s *Service)` thành hàm tự do
  `DefaultColumns(idGen func() uuid.UUID, specs []DefaultColumnSpec) []Column`.
- `internal/features/centers/default_columns.go`: bỏ hẳn `defaultColumnsSvc`, gọi thẳng
  `kanban.DefaultColumns(id.New, defaultColumnSpecs)`.
- Cập nhật `TestDefaultColumnsAssignsPositionsAndKeepsIsDone` trong `pkg/kanban/service_test.go`
  gọi hàm tự do trực tiếp (`DefaultColumns(uuid.New, specs)`); sửa `pkg/kanban/README.md` khớp chữ
  ký mới.

## N13 — Thông báo lỗi `markedBlocks` ghi nhầm tên file

`migrations/backfill_parity_test.go`: thêm tham số `file string` vào `markedBlocks`, cả 2
`t.Fatalf` bên trong dùng `file` thay vì hardcode `backfillUpFile`. 5 call site (2 cho backfill
000018, 3 cho task-board 000022) truyền đúng tên file tương ứng.

## Phát sinh trong lúc sửa (không phải finding gốc)

`make lint-api` báo `revive: redefines-builtin-id` cho biến `min` (shadow hàm built-in `min` của
Go) ở 5 chỗ vừa thêm cho M4 (`task_repository.go`, cả hai fake `MinPositionInColumn`,
`ports.go`'s named return, `service.go`'s `topPositionInColumn`). Đổi tên thành `minPos` ở cả 5
chỗ; lint xanh trở lại, không đổi hành vi.

## Test & lint

| Lệnh | Kết quả |
|---|---|
| `gofmt -l .` (từ `apps/api`) | sạch |
| `go vet ./...` | pass |
| `go test ./pkg/... ./internal/...` | tất cả pass (không skip) |
| `go test -tags=integration -p 1 -count=1 ./internal/features/tasks/... ./internal/features/centers/... ./internal/features/audit/... ./migrations/... ./seeds/...` | tất cả pass, gồm `TestRemoveMemberHandsOverTasks`, `TestTaskGuardsRejectCrossCenterRows`, `TestTaskBoardSQLMatchesFrozenKeysAndColumns` |
| `make lint-api` (từ repo root) | 0 issues |
| `make scopelint` (từ repo root) | pass |
| `make api-docs` (từ repo root) | regenerate xong; diff chỉ đúng đoạn 409/422 của `deleteColumn` (M1/N2), không có drift ngoài dự kiến |

Không container `teka-api-1`/`teka-web-1`/`teka-db` nào bị đụng — integration test tự dựng
Postgres qua testcontainers riêng.

## File đã sửa

- `apps/api/pkg/kanban/errors.go`
- `apps/api/pkg/kanban/columns.go`
- `apps/api/pkg/kanban/service.go`
- `apps/api/pkg/kanban/ports.go`
- `apps/api/pkg/kanban/service_test.go`
- `apps/api/pkg/kanban/README.md`
- `apps/api/internal/features/tasks/errors.go`
- `apps/api/internal/features/tasks/handler.go`
- `apps/api/internal/features/tasks/events.go`
- `apps/api/internal/features/tasks/service.go`
- `apps/api/internal/features/tasks/service_test.go`
- `apps/api/internal/features/tasks/task_repository.go`
- `apps/api/internal/features/tasks/model.go`
- `apps/api/internal/features/tasks/column_delete_integration_test.go`
- `apps/api/internal/features/centers/events.go`
- `apps/api/internal/features/centers/service.go`
- `apps/api/internal/features/centers/repository.go`
- `apps/api/internal/features/centers/default_columns.go`
- `apps/api/internal/features/audit/subscriber.go`
- `apps/api/migrations/000022_task_board.up.sql`
- `apps/api/migrations/backfill_parity_test.go`
- `apps/api/docs/docs.go`, `apps/api/docs/swagger.json`, `apps/api/docs/swagger.yaml` (generated
  lại qua `make api-docs`)

Status: DONE
Summary: Đã áp dụng đầy đủ M1–M4, N3–N9, N13 phía API — sửa 400→422 cho 3 loại lỗi validate cụ
thể, chặn panic khi event bus nil, bọc `ReorderColumns` trong transaction, đẩy cap per-column và
tính vị trí task xuống SQL (window function + `MIN`) thay vì quét toàn tenant, thêm CHECK/sửa FK
cho `priority`/`assignee_id`, sửa audit thiếu actor + publish thừa, sửa comment sai, chặn
`taskRepository.Create` ghi nhầm tenant, bỏ `Service` rác khỏi `DefaultColumns`, và sửa thông báo
lỗi sai tên file trong parity test. Toàn bộ build/vet/unit/integration/lint/scopelint xanh;
`api-docs` regenerate chỉ đổi đúng đoạn liên quan.
Concerns/Blockers: Không có blocker. Một điểm đáng lưu ý: N5/N6 (actor/entity id cho audit
`task.handover`, và điều kiện publish) hiện chưa có test tự động nào phủ trực tiếp — hạ tầng test
tích hợp của `centers` package dùng chung một `env` với bus nil cho toàn bộ file, nối bus riêng cho
một test sẽ đổi hành vi baseline của các test khác trong cùng file nên tôi không tự ý mở rộng.
Nếu team-lead muốn, có thể tách một test file/`env` riêng có bus thật để phủ N5/N6 trong một lượt
sau.
