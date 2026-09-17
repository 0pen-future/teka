---
phase: 1
title: "API: move có vị trí (after_task_id) + renormalize"
status: completed
priority: P1
effort: "1d"
dependencies: []
---

# Phase 1: API move có vị trí (`after_task_id`) + renormalize

## Overview

Mở rộng `POST /tasks/:id/move` để nhận `after_task_id` (tuỳ chọn, nullable),
server tự tính `position` float ở giữa hai hàng xóm trong cùng transaction và
renormalize cột khi khoảng cách quá nhỏ. Body v1 `{ column_id }` giữ nguyên
hành vi "đầu cột".

## Requirements

- Functional:
  - `after_task_id` vắng/`null` → đầu cột (min − 1, hoặc 0 nếu cột rỗng) — y v1.
  - `after_task_id` có giá trị → việc nằm ngay sau nó; kế tiếp là việc từng đứng
    sau `after` (nếu có) — cả khi cùng cột lẫn khác cột.
  - `after_task_id` không tồn tại, khác tenant, soft-deleted, không thuộc
    `column_id`, hoặc bằng chính `id` → `ErrInvalidAfterTask` (wrap
    `ErrInvalidInput`) → 422 `fields.after_task_id`. Chuỗi không phải UUID
    → `BindError` sẵn có → **400** (không phải 422; xem `validation.go`).
  - Hai request move đồng thời vào cùng cột phải tuần tự hoá: `uow.Within`
    chỉ mở transaction READ COMMITTED, không khoá → bắt buộc lấy
    `pg_advisory_xact_lock` theo `(center_id, column_id)` ngay đầu tx (bên
    trong `ListColumnPositions`), trước khi đọc vị trí.
  - Khi `next.position − after.position < 1e-6` → renormalize cả cột đích về
    `0, 1, …, n−1` theo thứ tự hiện tại rồi tính lại; toàn bộ trong `uow.Within`.
  - `CompletedAt` vẫn theo `column.IsDone` như v1; policy `CanMoveTask` không đổi.
- Non-functional:
  - `pkg/kanban` không thêm dependency (`import_boundary_test` xanh).
  - Query vị trí dùng `idx_tasks_board (center_id, column_id, position)`; cột
    tối đa vài trăm việc nên `ListColumnPositions` O(n) chấp nhận được.
  - `tasks.Service.Board` (feature) sắp lại bằng `sort.Slice` chỉ theo
    `Position` → đổi sang `sort.SliceStable` (giữ tie-break `created_at` từ
    `ListBoard`) để thứ tự xác định khi position trùng tạm thời.
  - Swagger cập nhật, `make api-docs` diff sạch.

## Architecture

```
handler.moveTask ──► tasks.Service.MoveTask(req{ColumnID, AfterTaskID Optional[uuid]})
                          │  translate: Optional → *kanban.TaskID (nil = top)
                          ▼
                kanban.Service.MoveTask(ctx, tenant, actor, id, target, after *TaskID)
                          │ Get task → CanMoveTask → Columns.Get(target)
                          │ uow.Within:
                          │   after == nil  → topPositionInColumn (v1)
                          │   after != nil  → rows := Tasks.ListColumnPositions(target)
                          │                   pos, ok := positionAfter(rows, id, *after)
                          │                   !ok → ErrInvalidAfterTask
                          │                   needRenorm → Tasks.RenormalizeColumn(target, order)
                          │                                → rows lại → positionAfter
                          │   task.ColumnID/Position/CompletedAt → Tasks.Update
                          ▼
                     TaskResponse (position mới)
```

Hàm thuần trong core (test không cần DB):

```go
// TaskPosition is one live task's ordering key inside a column.
type TaskPosition struct {
	ID       TaskID
	Position float64
}

// positionAfter returns the Position that places moving directly after
// anchor within rows (already ordered by Position, then CreatedAt).
// rows excludes soft-deleted tasks; moving may or may not be in rows
// (same-column reorder vs. cross-column move) and is skipped when looking
// for anchor's successor. ok is false when anchor is absent or equals
// moving. renormalize is true when the gap to the successor is below
// minPositionGap — the caller must renormalize and call again.
func positionAfter(rows []TaskPosition, moving, anchor TaskID) (pos float64, renormalize, ok bool)

const minPositionGap = 1e-6
```

Port mới trong `TaskRepository`:

```go
// ListColumnPositions returns the live tasks of col ordered like ListBoard
// (Position, then CreatedAt), only id and position — the ordering key
// MoveTask needs to place a task between two neighbours. It must be called
// inside a UnitOfWork and serializes concurrent callers on the same column
// for the rest of that transaction (Postgres: pg_advisory_xact_lock keyed
// on tenant+column), so two moves cannot compute the same midpoint.
ListColumnPositions(ctx context.Context, tenant TenantID, col ColumnID) ([]TaskPosition, error)
// RenormalizeColumn rewrites Position to 0..len(order)-1 following order
// (every live task of col exactly once). Called inside the caller's
// transaction when float gaps get too small to bisect.
RenormalizeColumn(ctx context.Context, tenant TenantID, col ColumnID, order []TaskID) error
```

`RenormalizeColumn` implement bằng một `UPDATE … FROM (VALUES …)` hoặc vòng
`UPDATE` theo id trong cùng tx (n nhỏ); phải scope `center_id`. Lưu ý:
`scopelint` chỉ soi hàm nhận `Scope`, hai port mới nhận `TenantID` nên việc
thêm `center_id = ?` là kỷ luật tay — integration test tenant khác là lưới
bảo vệ.

## Related Code Files

- Modify: `apps/api/pkg/kanban/ports.go` (2 port + `TaskPosition`)
- Modify: `apps/api/pkg/kanban/service.go` (`MoveTask` thêm tham số `after *TaskID`, `positionAfter`, dùng `uow.Within`)
- Modify: `apps/api/pkg/kanban/errors.go` (`ErrInvalidAfterTask`)
- Modify: `apps/api/pkg/kanban/service_test.go` (fake repo thêm 2 method; 5 test `MoveTask` hiện có sửa chữ ký thêm `nil`; test bảng cho `positionAfter`; test `MoveTask` after/top/renormalize/invalid)
- Modify: `apps/api/pkg/kanban/README.md` ("Position strategy": mô tả midpoint + renormalize; bảng port; bảng use-case → `MoveTask` đọc `MinPositionInColumn` hoặc `ListColumnPositions`, không phải `ListBoard`)
- Modify: `apps/api/internal/features/tasks/dto.go` (`MoveTaskRequest{ColumnID uuid.UUID; AfterTaskID Optional[uuid.UUID] \`json:"after_task_id"\`}`)
- Modify: `apps/api/internal/features/tasks/service.go` (`MoveTask` map Optional → pointer; `Board` dùng `sort.SliceStable`)
- Modify: `apps/api/internal/features/tasks/errors.go` (`ErrInvalidAfterTask` → 422 `fields.after_task_id`)
- Modify: `apps/api/internal/features/tasks/task_repository.go` (2 method mới)
- Modify: `apps/api/internal/features/tasks/handler.go` (swag `@Description` bỏ câu "always lands at the top", thêm 422)
- Modify: `apps/api/internal/features/tasks/service_test.go` (bộ fake riêng của feature implement `kanban.TaskRepository` ở dòng ~100–235 → thêm 2 method; HTTP/unit: body v1 vẫn top; body v2; 422; chuỗi không UUID → 400)
- Create: `apps/api/internal/features/tasks/move_integration_test.go` (`//go:build integration`: thứ tự thật qua `ListBoard`, renormalize, tenant khác, 2 goroutine move đồng thời cùng `after_task_id` → không trùng position)
- Modify: `apps/api/internal/features/tasks/service.go` (`HandoverOnDeparture` không đổi; chỉ kiểm compile)
- Generated: `apps/api/docs/*` qua `make api-docs`

## Implementation Steps

1. `pkg/kanban/ports.go`: thêm `TaskPosition`, `ListColumnPositions`, `RenormalizeColumn` với doc comment như trên. `errors.go`: `ErrInvalidAfterTask = fmt.Errorf("%w: after_task_id is not a live task in the destination column", ErrInvalidInput)`.
2. `pkg/kanban/service.go`: viết `positionAfter` (thuần) — duyệt `rows` tìm `anchor`; successor = phần tử kế tiếp có `ID != moving`; không successor → `anchor.Position + 1`; có → `(anchor+next)/2`, `renormalize = next−anchor < minPositionGap`. Đổi chữ ký `MoveTask(ctx, tenant, actor, id, target, after *TaskID)`; bọc phần tính vị trí + `Update` trong `s.uow.Within`; cập nhật comment "no manual position parameter".
3. `service_test.go` (core): mở rộng `fakeTaskRepo` (giữ slice có thứ tự); test bảng `positionAfter` (anchor cuối, anchor giữa, anchor = moving, anchor vắng, gap nhỏ → renormalize); test `MoveTask` với `after=nil` giữ hành vi cũ (test hiện có chỉ thêm tham số `nil`), `after` hợp lệ, cùng cột, `after` ở cột khác → `ErrInvalidAfterTask`, renormalize gọi `RenormalizeColumn` đúng `order`.
4. `internal/features/tasks/dto.go`: `AfterTaskID Optional[uuid.UUID]`; `service.go`: `var after *kanban.TaskID; if req.AfterTaskID.Set && req.AfterTaskID.Value != nil { v := kanban.TaskID(*req.AfterTaskID.Value); after = &v }`. `errors.go`: case `ErrInvalidAfterTask` → `apperror.Invalid("validation failed", map[string]string{"after_task_id": "phải là việc đang nằm trong cột đích"})` (đặt **trước** case `ErrInvalidInput` chung nếu có).
5. `task_repository.go`: `ListColumnPositions` — trước hết `SELECT pg_advisory_xact_lock(hashtext(?::text || ':' || ?::text))` với `center_id`, `column_id` (tx lấy từ ctx theo mẫu `uow`), rồi `SELECT id, position FROM tasks WHERE center_id=? AND column_id=? AND deleted_at IS NULL ORDER BY position, created_at`; `RenormalizeColumn` — vòng `UPDATE tasks SET position=? WHERE center_id=? AND id=?` trong tx hiện tại (GORM `db.WithContext(ctx)` lấy tx từ ctx theo mẫu `uow.go`). Chạy `make scopelint`.
6. `handler.go`: sửa swag (`@Description`: "Omit or null after_task_id to land at the top; otherwise the task is placed right after that task in the destination column"), thêm `@Failure 422`. Chạy `make api-docs`, commit docs.
7. `service_test.go` (feature) + `move_integration_test.go`: các case AC1–AC4; AC4 = lặp 60 lần chèn vào giữa hai việc cố định rồi assert `ListBoard` đúng thứ tự và mọi cặp kề nhau có gap ≥ 1e-6 (việc vừa chèn nhận midpoint nên không assert số nguyên). Test đồng thời: 2 goroutine cùng move 2 việc khác nhau vào sau cùng một `after_task_id` → hai position khác nhau, thứ tự hợp lệ.
8. Cập nhật `pkg/kanban/README.md` mục "Position strategy" và bảng port; xoá câu "manual reordering would need renormalization" thay bằng mô tả đã làm.
9. Gate: `make test-api-unit`, `make lint-api`, `make test-api` (chạy đơn lẻ), `make api-docs` rồi `git diff --exit-code apps/api/docs`.

## Success Criteria

- [x] Test core: `positionAfter` bảng đầy đủ; `MoveTask` after/top/renormalize/invalid xanh; `TestImportBoundary` xanh.
- [x] Test HTTP: body `{ column_id }` → position = min−1 (test cũ giữ nguyên kỳ vọng); body `{ column_id, after_task_id }` → 200 và position giữa; UUID hợp lệ nhưng không phải việc sống trong cột đích → 422 `fields.after_task_id`; chuỗi không UUID → 400.
- [x] Integration: 2 move đồng thời không trùng position (advisory lock); `Board` feature dùng `sort.SliceStable`.
- [x] Integration: thứ tự `ListBoard` đúng sau reorder cùng cột và khác cột; AC4 renormalize; `after_task_id` của tenant khác → 422.
- [x] `make api-docs` không tạo diff sau khi commit; `make lint-api` + `scopelint` sạch.

## Risk Assessment

- **Đồng thời**: không khoá thì hai move cùng `after` tạo trùng position, renormalize đan xen với midpoint cũ → sai thứ tự (tx READ COMMITTED không tự bảo vệ). Đã chốt advisory lock theo cột trong `ListColumnPositions` (khoá cả cột rỗng, khác `FOR UPDATE`); chi phí: các move vào cùng cột tuần tự hoá — vài trăm việc/cột, chấp nhận. Tín hiệu vỡ: integration test 2 goroutine thấy trùng position.
- **Chữ ký `MoveTask` core đổi** làm vỡ mọi caller: chỉ có `internal/features/tasks/service.go` (kiểm bằng `grep -rn "\.MoveTask("`). Compile là bằng chứng.
- **Optional[uuid] với chuỗi không phải UUID** → `json.Unmarshal` lỗi → `validation.BindError` 422 sẵn có; test một case.
