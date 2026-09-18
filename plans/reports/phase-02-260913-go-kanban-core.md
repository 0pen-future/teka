# Phase 2 — Go kanban core lib: báo cáo hoàn thành

**Vị trí:** `apps/api/pkg/kanban` (import path `teka/apps/api/pkg/kanban`).
**Trạng thái:** completed. Tất cả Success Criteria trong
`plans/260913-1102-task-center-kanban/phase-02-go-kanban-core-lib.md` đã tick.

## Bề mặt API xuất khẩu (để Phase 3 adapter dùng, không cần đọc lại code)

### Kiểu ID (định nghĩa mới trên `uuid.UUID`, không phải alias)

`TenantID`, `ActorID`, `TaskID`, `ColumnID` — tất cả cast qua
`uuid.UUID(x)` / `TypeName(u)` khi chuyển đổi với adapter.

### Entities (`entity.go`)

- `Column{ID, TenantID, Name, Position int, IsDone, CreatedAt, UpdatedAt}`
- `Task{ID, TenantID, ColumnID, CreatedBy, AssigneeID *ActorID, Title,
  Description, Priority, DueOn *time.Time, Position float64, CompletedAt
  *time.Time, CreatedAt, UpdatedAt}`
- `Priority` = `PriorityNone|Low|Medium|High` (int enum, zero = None)

### Sentinel errors (`errors.go`, 10 cái, so bằng `errors.Is`)

`ErrColumnNotFound`, `ErrTaskNotFound`, `ErrColumnLimit`,
`ErrDuplicateColumnName`, `ErrColumnNotEmpty`, `ErrLastColumn`,
`ErrInvalidPermutation`, `ErrAssigneeNotMember`, `ErrForbidden`,
`ErrInvalidInput`. Không có `ErrCrossTenant` — id tenant khác = not-found.

### Ports (8, `ports.go`) — Phase 3 phải implement

- `ColumnRepository{List, Get, Create, Update, Delete, UpdatePositions,
  CountByTenant, ExistsName}` — mọi method: `(ctx, tenant TenantID, ...)`.
- `TaskRepository{ListBoard(ctx,tenant,Visibility), Get, Create, Update,
  SoftDelete, CountInColumn, MoveAllToColumn(ctx,tenant,from,to,*time.Time)
  (int,error), UnassignBy(ctx,tenant,actor)(int,error),
  ReassignCreator(ctx,tenant,from,to)(int,error)}`. **Chú ý**:
  `CountInColumn`/`MoveAllToColumn` phải tính cả task đã soft-delete (đọc
  doc comment tại chỗ trong `ports.go`); `ListBoard` loại soft-deleted.
- `Repositories{Columns, Tasks}` — struct gộp, truyền vào `NewService`.
- `UnitOfWork{Within(ctx, fn func(ctx) error) error}` — cùng shape với
  `database.TxManager.WithinTx`, map thẳng 1-1.
- `Policy{CanManageBoard, CanReadTask, CanWriteTask, CanMoveTask,
  Visibility}` — dùng `DefaultPolicy{}` có sẵn (implement đủ 4 luật D3 + view
  scope), không cần viết mới trừ khi luật khác.
- `MemberChecker{IsMember(ctx, tenant, actor) (bool, error)}`.
- `EventSink{Publish(ctx, event any)}` — nhận `ColumnDeleted` (xem dưới).
- `Clock{Now() time.Time}` — production dùng default (`WithClock` không cần
  set trừ test).

### Value objects khác (`ports.go`)

- `Actor{ID, IsOwner, Perms map[string]bool}` — **map mới mỗi request**, core
  không copy.
- `Visibility{All bool, Participant ActorID}` — do `Policy.Visibility` sinh,
  truyền thẳng vào `TaskRepository.ListBoard`.
- `ColumnPatch{Name *string, IsDone *bool}` — patch cho `UpdateColumn`.

### Permission keys `DefaultPolicy` đọc từ `Actor.Perms` (`policy.go`)

`kanban.PermManageBoard = "tasks.manage_board"`,
`kanban.PermViewAll = "tasks.view_all"`. Nếu catalog Phase 1 dùng đúng 2 khoá
này thì adapter chỉ cần build `Perms` map từ `authctx.Scope.Has(...)`.

### `Service` — 11 use-case (`service.go`, khởi tạo qua `options.go`)

```go
func NewService(repos Repositories, uow UnitOfWork, pol Policy,
    members MemberChecker, sink EventSink, opts ...Option) *Service
```

Options: `WithMaxColumns(int)` (default 8), `WithMaxNameLen(int)` (default
40), `WithClock(Clock)`, `WithIDGen(func() uuid.UUID)`.

| Method | Chữ ký | Ghi chú cho adapter |
|---|---|---|
| `Board` | `(ctx, tenant, actor) ([]Column, []Task, error)` | Cột đã sort theo `Position`; không check quyền "được xem board" (đó là việc của middleware capability) |
| `CreateColumn` | `(ctx, tenant, actor, name string, isDone bool) (Column, error)` | Cần `CanManageBoard`; check-then-act với trần cột — **adapter phải lock theo tenant** |
| `UpdateColumn` | `(ctx, tenant, actor, colID, ColumnPatch) (Column, error)` | Nil field = không đổi; đổi `IsDone` không đụng task |
| `ReorderColumns` | `(ctx, tenant, actor, order []ColumnID) error` | So permutation theo tập, không theo thứ tự |
| `DeleteColumn` | `(ctx, tenant, actor, colID, moveTo *ColumnID) error` | Mở `Within`; publish `ColumnDeleted` sau khi `Within` nil |
| `CreateTask` | `(ctx, tenant, actor, CreateTaskInput) (Task, error)` | `CreateTaskInput{ColumnID, Title, Description, Priority, DueOn *time.Time, AssigneeID *ActorID}`; không check policy "được tạo" (middleware lo); task mới lên đầu cột |
| `GetTask` | `(ctx, tenant, actor, id TaskID) (Task, error)` | `CanReadTask` |
| `UpdateTask` | `(ctx, tenant, actor, id, UpdateTaskInput) (Task, error)` | `UpdateTaskInput{Title, Description, Priority, DueOn *time.Time, AssigneeID *ActorID}` — **không có `ColumnID`** (đổi cột đi qua `MoveTask`); mọi field replace toàn bộ (không phải patch từng phần); `CanWriteTask` |
| `MoveTask` | `(ctx, tenant, actor, id TaskID, target ColumnID) (Task, error)` | `CanMoveTask`; không có tham số position (không DnD); set/clear `CompletedAt` theo `target.IsDone` |
| `DeleteTask` | `(ctx, tenant, actor, id TaskID) error` | Soft-delete; `CanWriteTask` |
| `HandoverOnDeparture` | `(ctx, tenant, departed, newOwner ActorID) (unassigned, reassigned int, err error)` | **Không** mở `Within`, **không** publish — gọi bên trong tx của `RemoveMember`, consumer tự publish event của mình sau khi tx đó commit |

### Factory (`columns.go`)

```go
func (s *Service) DefaultColumns(specs []DefaultColumnSpec) []Column
type DefaultColumnSpec struct{ Name string; IsDone bool }
```

Gán `Position` 0..n-1 và `ID` mới; **không** gán `TenantID`/timestamps —
adapter set `TenantID` trước khi persist (thường trong `CreateCenter`, cùng
tx với việc tạo center).

### Event (`events.go`)

```go
type ColumnDeleted struct {
    ColumnID   ColumnID
    MoveTo     *ColumnID
    MovedCount int
}
```

Duy nhất 1 event ở v1; adapter `EventSink.Publish` type-switch trên `any`.

## Quyết định thiết kế không có trong phase file gốc (đã tự quyết trong phạm vi)

1. **`Board` trả về `([]Column, []Task, error)`** thay vì một struct
   `BoardView` — phase file không đặt tên kiểu trả về; chọn multi-return cho
   đơn giản (KISS), không thêm kiểu mới không ai yêu cầu.
2. **`UpdateTask` dùng full-replace input, không dùng tri-state patch kiểu
   con trỏ-kép.** Lý do: form sửa việc trên UI luôn có đủ state hiện tại nên
   không cần phân biệt "không đổi" vs "xoá về nil" cho `DueOn`/`AssigneeID`;
   giữ tri-state sẽ thêm ceremony không cần thiết (KISS). `ColumnID` bị loại
   khỏi input này có chủ đích — đổi cột là việc của `MoveTask`, giữ đúng ranh
   giới `CanWriteTask` vs `CanMoveTask`.
3. **`MoveTask` không có tham số vị trí thủ công.** Non-goal "không kéo-thả"
   + Risk Assessment của phase file ("v1 dùng min-1") gợi ý chiến lược "luôn
   chèn lên đầu cột" áp dụng thống nhất cho cả `CreateTask` lẫn `MoveTask`.
4. **`tasksInColumn` (nội bộ) tính vị trí đầu cột bằng cách lọc
   `TaskRepository.ListBoard(tenant, Visibility{All:true})`** — port list
   không có method "lấy theo cột", nên đây là cách duy nhất không thêm port
   mới ngoài 8 port đã chốt.
5. Tên độ dài cột so bằng `unicode/utf8.RuneCountInString`, không phải
   `len()` byte, để đúng trực giác "số ký tự" với tên có dấu.

## Test

`go test ./pkg/kanban/... -count=1` — xanh, ~0.08–0.7s (có/không tính
compile), không Docker. Gồm `service_test.go` (fakes map-backed, cover đủ 10
sentinel error + 11 use-case + event/UoW semantics + `HandoverOnDeparture`
không mở tx/publish), `policy_test.go` (ma trận owner/creator/assignee/lạ ×
read/write/move + manage_board + visibility), `import_boundary_test.go` (đã
kiểm chứng thủ công: thêm import `internal/database` tạm thời → test fail →
revert, xác nhận trong quá trình implement).

Gates đã chạy từ `apps/api`: `go build ./...`, `go vet ./pkg/...`,
`go list -deps ./pkg/kanban` (chỉ thấy stdlib + `github.com/google/uuid`,
cộng `vendor/golang.org/x/net/dns/dnsmessage` — vendor nội bộ của `net`
trong stdlib do `uuid` gọi `net.Interfaces()`, không phải third-party thật,
không vi phạm biên giới), và từ gốc repo `make lint-api` (0 issues sau khi
sửa vài cảnh báo `revive`: thiếu doc comment trên const/method exported,
biến tên `min` đè built-in) + `go test ./tools/...` (scopelint tests xanh,
không ảnh hưởng vì `pkg/` không có `Scope`).

## Sai lệch / vấn đề đã tự sửa

- README bản nháp đầu tiên và một doc comment trong `columns.go` từng chứa ví
  dụ "Cần làm/Đang làm/Hoàn thành" — vi phạm trực tiếp yêu cầu "lib không
  chứa chuỗi tiếng Việt" (`grep -rn "Cần làm" pkg/kanban` phải = 0). Đã phát
  hiện bằng grep trước khi báo cáo hoàn thành và sửa cả hai file bằng mô tả
  chung chung, không nêu tên cụ thể. Đã re-verify: `grep -rn "Cần làm"
  pkg/kanban` → 0 hit.

## Câu hỏi chưa giải quyết

Không có — mọi quyết định thiết kế còn thiếu trong phase file đã tự quyết
theo KISS/DRY và ghi lại ở mục "Quyết định thiết kế" trên, phù hợp phạm vi đã
chốt (không mở lại quyết định D1–D10 của plan.md).

Status: DONE
Summary: pkg/kanban hoàn chỉnh — 11 use-case, 8 port, 10 sentinel error, DefaultPolicy, DefaultColumns, options, README; go test/vet/build/lint-api/tools đều xanh; boundary test đã verify fail-then-revert thủ công.
Concerns/Blockers: none
