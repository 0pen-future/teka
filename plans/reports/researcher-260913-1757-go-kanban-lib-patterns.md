# Research: Go reusable domain lib patterns cho Kanban core (`apps/api/pkg/kanban`)

Context đã đọc: `plans/reports/scout-260913-1757-api-task-board.md` (kiến trúc hiện có: feature module
`model/dto/repository/service/handler/routes/errors`, `TxManager.WithinTx(ctx, fn)`, `authctx.Scope`,
event bus in-process, consumer-defined interface DI, không có `pkg/` layer hiện tại — đây sẽ là cái đầu tiên).

## 1. Package structure (hexagonal / ports-and-adapters)

Nguồn: cách làm phổ biến trong cộng đồng Go — "Standard Package Layout" (Ben Johnson), go-kit, ardanlabs/service,
và search kết quả về hexagonal-in-Go [dev.to/bagashiz](https://dev.to/bagashiz/building-restful-api-with-hexagonal-architecture-in-go-1mij),
[dev.to/gabrielanhaia](https://dev.to/gabrielanhaia/hexagonal-for-the-rest-of-us-ports-and-adapters-without-ddd-2ko8).

Khuyến nghị: **một package phẳng** `pkg/kanban` (không chẻ sub-package theo layer như
`kanban/domain`, `kanban/ports`) vì:
- Domain nhỏ (columns + cards), chẻ layer tạo import cycle risk và ceremony thừa (vi phạm KISS).
- Convention Go: package theo domain/capability, không theo kiến trúc layer (tránh
  `kanban/domain`, `kanban/usecase`, `kanban/ports` — anti-pattern "layered package" Ben Johnson từng chỉ trích).
- Sub-package chỉ tách khi có ranh giới thật: nếu sau này cần `kanban/inmemory` (fake store cho test
  của consumer khác) thì tách, còn lại giữ 1 package.

Cấu trúc file (đặt tên giống convention hiện tại của repo: `model.go/service.go/errors.go`):

```
apps/api/pkg/kanban/
  entity.go       // Column, Card structs — plain structs, no gorm tags, no json tags
  errors.go       // sentinel errors: var ErrColumnLimitExceeded = errors.New(...)
  ports.go        // ColumnRepository, CardRepository, TxManager, EventSink, Policy interfaces
  service.go       // Service struct — the use-case orchestrator
  options.go       // functional options: Option, WithMaxColumns, WithClock, WithIDGen
  events.go       // ColumnDeleted, CardsHandedOver, CardMoved event types
  service_test.go // table-driven tests against in-memory fakes (fakes live in _test.go, unexported)
```

Naming convention (đã verified qua ardanlabs/service style, go-kit style): port interface đặt tên theo
role không theo tech (`ColumnRepository` chứ không `ColumnStore` lẫn `GormColumnRepo`), impl ở adapter đặt tên
theo tech (`gormColumnRepository` — adapter, unexported, matches repo's own convention `gormRepository` đã thấy ở
`centers/repository.go`).

```go
// entity.go
type Column struct {
    ID       uuid.UUID
    TenantID string // opaque string, core doesn't know it's "center_id"
    Name     string
    Position int
    IsDone   bool
}

type Card struct {
    ID          uuid.UUID
    ColumnID    uuid.UUID
    Title       string
    Description string
    Priority    Priority
    DueDate     *time.Time
    AssigneeID  *uuid.UUID
    CreatorID   uuid.UUID
    Position    float64
    CompletedAt *time.Time
}
```

Quan trọng: `TenantID` là **opaque** (`string`/`uuid.UUID`), core không biết đó là `center_id`. Adapter
(`internal/features/tasks`) map `authctx.Scope.CenterID` -> `kanban.TenantID` khi gọi service. Đây là cách giữ
zero-import vào `internal/*`.

## 2. Unit of Work / TxManager port

App hiện có `database.TxManager.WithinTx(ctx, fn func(ctx) error) error` (đã dùng ở `centers/service.go`).
Core lib nên định nghĩa **port riêng cùng shape**, không import `internal/database`:

```go
// ports.go
type TxManager interface {
    WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}
```

Adapter side (`internal/features/tasks/adapter.go`) chỉ cần **truyền thẳng** `*database.GormTxManager` app hiện
có — vì signature giống hệt, không cần wrapper nếu Go structural typing cho phép (interface identical → app's
concrete type tự động thoả mãn `kanban.TxManager`, không cần adapter struct). Nếu muốn tách hẳn dependency-type
(tránh core vô tình phụ thuộc kiểu cụ thể của app dù chỉ interface), viết 1 dòng wrapper:

```go
type txAdapter struct{ tx database.TxManager }
func (a txAdapter) WithinTx(ctx context.Context, fn func(context.Context) error) error {
    return a.tx.WithinTx(ctx, fn)
}
```

Service dùng UoW để bọc case cần atomic (`DeleteColumn` với `moveTo`, `HandoverOnDeparture`):

```go
func (s *Service) DeleteColumn(ctx context.Context, tenant TenantID, colID uuid.UUID, moveTo *uuid.UUID) error {
    return s.tx.WithinTx(ctx, func(ctx context.Context) error {
        if moveTo != nil {
            if err := s.cards.ReassignColumn(ctx, tenant, colID, *moveTo); err != nil { return err }
        } else if n, _ := s.cards.CountInColumn(ctx, tenant, colID); n > 0 {
            return ErrColumnNotEmpty
        }
        return s.columns.Delete(ctx, tenant, colID)
    })
}
```

Pitfalls:
- **Ctx propagation**: repository implementation PHẢI đọc tx từ `ctx` (như `database.FromContext(ctx, r.db)`
  hiện tại), không giữ `*gorm.DB` field riêng — nếu không, calls bên trong `WithinTx` sẽ chạy ngoài transaction.
- **Nested tx**: GORM's `db.Transaction` (dùng bên trong `WithinTx`) không tự cho phép nested transaction thật
  (chỉ có savepoint nếu driver support) — service không nên tự gọi `WithinTx` lồng nhau; nếu 1 use-case cần gọi
  use-case khác trong cùng tx, dùng ctx-carried tx xuyên suốt, KHÔNG mở tx mới. Rule: chỉ top-level use-case
  method mở `WithinTx`, các helper nội bộ chỉ nhận ctx.
- Core không được biết `*gorm.DB` tồn tại — port chỉ thấy `context.Context`, giữ đúng zero-import constraint.

## 3. Authorization Strategy port

```go
// ports.go
type Actor struct {
    ID      uuid.UUID
    IsOwner bool
    Perms   map[string]bool // opaque flat map; adapter fills from authctx.Scope
}

type Policy interface {
    CanManageColumns(actor Actor, tenant TenantID) bool
    CanEditCard(actor Actor, card Card) bool
    CanMoveCard(actor Actor, card Card, target Column) bool
}
```

- `Actor` là value object **không** import `authctx.Scope` — adapter build `Actor` từ `Scope` (`IsOwner:
  scope.IsOwner`, `Perms: map[string]bool{"tasks.manage_columns": scope.Has(authctx.PermTasksManageColumns)}`).
  Core chỉ biết key string phẳng, không biết `PermDef`/catalog — giữ ranh giới sạch.
- Tenant scoping (center_id predicate, view_all read-widening) **ở lại trong adapter repository**, không lộ
  vào core: repository interface method chỉ nhận `tenant TenantID` làm điều kiện lọc cứng, core service
  không tự query cross-tenant — bản thân interface signature đã enforce điều đó (giống cách
  `sessions/repository.go` scoped() giấu logic view_all trong impl, không lộ ra service).
- Default policy: core có thể cung cấp `AllowAllPolicy` (cho test / khi app tự quản lý authz ở handler layer
  thay vì service layer) qua Factory function `NewAllowAllPolicy()`.

Trade-off: Strategy pattern ở đây là *pluggable decision*, nhưng nếu app luôn check authz ở handler
(pattern hiện tại: handler check `scope.Has()` trước khi gọi service — xem `centers/handler.go`), Policy port
có thể chỉ là **defense-in-depth** (double-check trong service), không phải primary gate. Cần quyết định rõ:
core enforce hay chỉ adapter enforce? Khuyến nghị: core enforce (Policy port bắt buộc, không optional) vì lib
này claim "reusable" — nếu không tự bảo vệ invariant thì không an toàn khi tái dùng ở nơi khác.

## 4. Domain events / Observer cho audit

```go
// events.go
type ColumnDeleted struct {
    TenantID   TenantID
    ColumnID   uuid.UUID
    MovedToID  *uuid.UUID
    CardCount  int
}
type CardsHandedOver struct {
    TenantID     TenantID
    DepartedID   uuid.UUID
    ReassignedTo uuid.UUID // new creator (owner) if applicable
    CardIDs      []uuid.UUID
}

// ports.go
type EventSink interface {
    Publish(ctx context.Context, event any) // adapter type-switches to map -> app bus event
}
```

- **Sync, post-commit**: gọi `Publish` **sau khi** `WithinTx` trả về nil (không publish bên trong closure của
  tx, để tránh audit event cho 1 transaction rollback). Pattern:

```go
func (s *Service) DeleteColumn(...) error {
    var count int
    err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
        count, _ = s.cards.CountInColumn(ctx, tenant, colID)
        ...
        return s.columns.Delete(ctx, tenant, colID)
    })
    if err != nil { return err }
    s.events.Publish(ctx, ColumnDeleted{TenantID: tenant, ColumnID: colID, MovedToID: moveTo, CardCount: count})
    return nil
}
```

- App's event bus hiện tại là in-process, async (buffer + batch) — `EventSink.Publish` không cần block; adapter
  wraps to app's `bus.Publish(ctx, "kanban.column_deleted", payload)`. Core không quan tâm bus có ordering
  guarantee hay không — đó là adapter's job. Nếu ordering giữa nhiều event trong 1 use-case quan trọng
  (vd CardsHandedOver phải phát trước 1 event khác), core publish theo đúng thứ tự gọi `Publish` tuần tự
  (bus adapter chịu trách nhiệm giữ order nếu cần, thường in-process channel giữ FIFO tự nhiên).
- Đặt `any` cho event type parameter là chấp nhận được (giống `interface{}` pattern của Go event bus phổ biến)
  để tránh phải định nghĩa generic `EventSink[T]` — KISS, vì chỉ có 2-3 loại event.

## 5. Functional options + testability

```go
// options.go
type Config struct {
    MaxColumns     int
    MaxNameLen     int
    DefaultColumns func() []Column // factory, called on tenant provisioning
    Clock          func() time.Time
    IDGen          func() uuid.UUID
}

type Option func(*Config)

func WithMaxColumns(n int) Option { return func(c *Config) { c.MaxColumns = n } }
func WithClock(fn func() time.Time) Option { return func(c *Config) { c.Clock = fn } }

func NewService(cols ColumnRepository, cards CardRepository, tx TxManager, ev EventSink, pol Policy, opts ...Option) *Service {
    cfg := Config{MaxColumns: 8, MaxNameLen: 60, Clock: time.Now, IDGen: uuid.New}
    for _, o := range opts { o(&cfg) }
    return &Service{columns: cols, cards: cards, tx: tx, events: ev, policy: pol, cfg: cfg}
}
```

Đây là pattern chuẩn Go (Dave Cheney "Functional options for friendly APIs") — không cần link riêng, đã là
kiến thức nền tảng ổn định trong Go ecosystem, không có tranh cãi hay thay đổi gần đây.

Testability: table-driven test dùng in-memory fake implement `ColumnRepository`/`CardRepository` (map-backed,
sống trong `service_test.go`, không export) + fake `Clock`/`IDGen` cố định để assert `CompletedAt` deterministic.
Không cần Docker/testcontainers cho core lib — đó là điểm khác biệt lớn nhất so với feature hiện tại (feature
integration test cần Postgres, core lib domain test thì không, chạy nhanh, đúng tinh thần "reusable kit").

## 6. Import boundary enforcement

Nguồn: [OpenPeeDeeP/depguard](https://github.com/OpenPeeDeeP/depguard),
[golangci-lint depguard v2 config](https://pkg.go.dev/github.com/golangci/golangci-lint/v2/pkg/config).

Có 2 lựa chọn bổ sung nhau, khuyến nghị dùng cả 2 (lint = fast feedback local/IDE; test = enforced trong CI
không phụ thuộc lint config có chạy hay không):

**a) golangci-lint depguard rule** (`.golangci.yml`):
```yaml
version: "2"
linters:
  settings:
    depguard:
      rules:
        kanban-core-no-internal:
          files:
            - "**/pkg/kanban/**/*.go"
          deny:
            - pkg: "teka/apps/api/internal"
              desc: "pkg/kanban must stay import-free of internal/*; use ports instead"
```

**b) Go test dùng `go list -deps` (đơn giản nhất, không cần thêm dependency)**:
```go
// pkg/kanban/import_boundary_test.go
func TestNoInternalImports(t *testing.T) {
    out, err := exec.Command("go", "list", "-deps", "./...").Output()
    require.NoError(t, err)
    for _, line := range strings.Split(string(out), "\n") {
        if strings.Contains(line, "/apps/api/internal/") {
            t.Fatalf("pkg/kanban must not import internal package: %s", line)
        }
    }
}
```
Chạy trong `pkg/kanban` dir (`go list -deps ./...` chỉ list deps của package hiện tại + sub). Đây là cách
"robust nhất, đơn giản nhất" theo đúng yêu cầu — không cần `golang.org/x/tools/go/packages` (heavier, chỉ cần
khi cần phân tích AST/type info, ở đây chỉ cần check import path string).

## 7. Idempotent / optimistic concerns

- **Reorder = full permutation**: validate input là permutation hợp lệ của tập ID hiện có trước khi ghi (so
  sánh set, không phải chỉ length) — reject nếu thiếu/thừa ID → tránh silent partial reorder. Có thể ghi trong
  1 UoW loop update từng `Position`.
- **Float position "put on top"**: `newPos = minExistingPos - 1` (hoặc `/2` nếu dùng gap-based fractional
  indexing thay vì -1 cố định, tránh cạn kiệt độ chính xác float sau nhiều lần chèn liên tiếp ở đầu). Renormalize
  khi khoảng cách giữa 2 vị trí kề nhau < epsilon (vd `1e-6`) — hoặc đơn giản: renormalize định kỳ
  (batch update `position = index * 1000.0`) khi phát hiện collision trong transaction insert.
- **Error → HTTP mapping** (dùng convention `apperror` sẵn có của app, core chỉ trả sentinel error, adapter map):
  - `ErrColumnLimitExceeded`, `ErrDuplicateColumnName` → 422 (validation, semantic input problem, retryable
    with different input)
  - `ErrColumnNotEmpty` (delete without moveTo) → 409 (conflict với state hiện tại, cần thêm thông tin
    `moveTo` để resolve — giống pattern `ErrNotFound -> 409` đã thấy ở `centers/service.go` removeMember)
  - `ErrColumnNotFound`, `ErrCardNotFound` → 404
  - `ErrInvalidPermutation` (reorder) → 422 (input tự nó sai, không phải race)
  - Optimistic concurrency thật (nếu thêm version column sau này) → 409 với retry hint

## 8. Named design patterns summary

| Pattern | Vai trò | Trade-off / testability |
|---|---|---|
| Repository | `ColumnRepository`/`CardRepository` port | Tách persistence khỏi domain; test bằng in-memory fake, không cần DB |
| Unit of Work | `TxManager.WithinTx` | Giữ atomicity xuyên nhiều repo call mà core không biết SQL/tx cụ thể; test bằng fake tx (no-op wrapper) |
| Strategy | `Policy` interface | Cho phép hoán đổi luật authz mà không sửa service; risk: nếu app luôn check ở handler, Policy dễ bị bỏ quên/duplicate logic |
| Observer | `EventSink.Publish` | Audit decoupled khỏi use-case; test bằng fake sink ghi nhận events, assert nội dung |
| Factory | `NewService(...)`, `DefaultColumns func() []Column` | Tạo default state nhất quán (columns khởi tạo cho tenant mới); dễ test vì factory function thuần |
| Functional Options | `Option`, `WithMaxColumns` | API mở rộng được mà không phá compat khi thêm option mới; test dễ vì mọi field có default rõ ràng |
| Specification | (tuỳ chọn, không cần) | Domain nhỏ, rule đơn giản (permutation check, name uniqueness) — thêm Specification pattern ở đây là over-engineering, vi phạm YAGNI |

## Giới hạn nghiên cứu

- Không tìm case study cụ thể "Go Kanban library" công khai — pattern đề xuất tổng hợp từ nguyên lý hexagonal Go
  phổ biến + convention đã quan sát trực tiếp trong chính repo teka (nguồn đáng tin nhất vì phải khớp style hiện có).
- Không benchmark hiệu năng float-position fractional indexing tại scale lớn — giả định column/card count nhỏ
  (per-tenant kanban board), không cần renormalize phức tạp kiểu CRDT.
- Chưa kiểm chứng golangci-lint v2 config syntax bằng cách chạy thực tế trong repo (repo dùng bản golangci-lint
  nào chưa xác nhận) — cần verify version trước khi áp dụng.

## Unresolved questions

1. Policy check nên nằm ở service layer (core enforce) hay chỉ ở handler layer (giống pattern hiện tại các
   feature khác)? Ảnh hưởng tới việc Policy port có bắt buộc hay optional.
2. `TenantID` type nên là `string` (id thô từ authctx) hay `uuid.UUID` — cần biết `center_id` hiện tại là kiểu gì
   trong DB/authctx.Scope để tránh convert qua lại không cần thiết ở adapter.
3. Default columns cho tenant mới (`DefaultColumns` factory) tạo lúc nào — lúc center được tạo (qua event) hay
   lazy khi tenant lần đầu gọi API kanban?

Status: DONE
Summary: Đề xuất `pkg/kanban` phẳng 1 package, port-based (Repository, UoW, Policy-as-Strategy, EventSink-as-Observer), functional options config, in-memory fake test, và 2 cách enforce import boundary (depguard config + `go list -deps` test) — kèm code sketch cho toàn bộ 8 mục yêu cầu.
