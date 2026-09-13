---
phase: 2
title: "Go kanban core lib"
status: completed
priority: P1
effort: "1.5d"
dependencies: []
---

# Phase 2: Go kanban core lib (`apps/api/pkg/kanban`)

## Overview

Lõi nghiệp vụ Kanban thuần: entities, ports, service use-case, sentinel errors,
factory cột mặc định. Không biết gì về Gin, GORM, `authctx`, `center_id`, hay
HTTP. Đây là package `pkg/` đầu tiên của `apps/api` (hiện chưa có `pkg/`).

Biên giới cứng: chỉ `stdlib` + `github.com/google/uuid`. Import path
`teka/apps/api/pkg/kanban`. Trích xuất sang repo riêng sau này = `git mv` +
thêm `go.mod`, không refactor.

Phase này độc lập hoàn toàn với Phase 1 (không đụng catalog, không đụng DB).

## Requirements

### Functional

- Entities: `Column{ID, TenantID, Name, Position, IsDone, CreatedAt, UpdatedAt}`,
  `Task{ID, TenantID, ColumnID, CreatedBy, AssigneeID *ActorID, Title,
  Description, Priority, DueOn *time.Time, Position float64, CompletedAt
  *time.Time}`.
- ID types: `TenantID`, `ActorID`, `TaskID`, `ColumnID` — kiểu định nghĩa mới
  trên `uuid.UUID` (`type TaskID uuid.UUID`), không phải alias, để compiler chặn
  truyền nhầm.
- Ports (8): `ColumnRepository`, `TaskRepository`, `Repositories`,
  `UnitOfWork`, `Policy`, `MemberChecker`, `EventSink`, `Clock`.
- `Actor{ID ActorID, IsOwner bool, Perms map[string]bool}` — value object, core
  không biết catalog. Adapter phải build `Perms` là map **mới** mỗi request,
  không chia sẻ map nội bộ của `Scope`.
- Use-cases trên `Service` (11): `Board`, `CreateColumn`, `UpdateColumn`
  (patch `Name`/`IsDone`, gộp rename + toggle thành một intent *(Red-team)*),
  `ReorderColumns`, `DeleteColumn`, `CreateTask`, `GetTask`, `UpdateTask`,
  `MoveTask`, `DeleteTask`, `HandoverOnDeparture`.
- `Visibility` value object do `Policy` sinh ra cho `Board`: `All` hoặc
  `Participant{ActorID}` (creator ∨ assignee). Repository nhận `Visibility`,
  **không** nhận `Actor` hay thứ gì giống `Scope` — luật read chỉ có một bản
  ở `DefaultPolicy` *(Red-team: D3)*.
- `DefaultColumns(specs []DefaultColumnSpec) []Column` factory: nhận tên +
  cờ `IsDone`, gán `Position` 0..n-1 và ID. Lib **không** chứa chuỗi tiếng
  Việt; tên "Cần làm/Đang làm/Hoàn thành" thuộc adapter Teka (Phase 3) và
  migration 000022 *(Red-team: lib đa ngôn ngữ)*.
- Sentinel errors (10): `ErrColumnNotFound`, `ErrTaskNotFound`,
  `ErrColumnLimit`, `ErrDuplicateColumnName`, `ErrColumnNotEmpty`,
  `ErrLastColumn`, `ErrInvalidPermutation`, `ErrAssigneeNotMember`,
  `ErrForbidden`, `ErrInvalidInput`. Không có `ErrCrossTenant`: cột/việc của
  tenant khác **là** not-found (repo bị ràng theo `tenant`), tránh oracle
  "tồn tại ở nơi khác" *(Red-team)*.
- Functional options: `WithMaxColumns(int)` (default 8), `WithMaxNameLen(int)`
  (default 40), `WithClock(Clock)`, `WithIDGen(func() uuid.UUID)`.

### Non-functional

- Zero import ngoài `stdlib` + `github.com/google/uuid` (kể cả transitively).
- Mọi test chạy không cần Docker/Postgres (fake in-memory repos).
- Publish event **chỉ** trong use-case mà `Service` tự mở `uow.Within` ở tầng
  ngoài cùng, và chỉ sau khi `Within` trả `nil`. Use-case chạy **bên trong tx
  của consumer** (v1: `HandoverOnDeparture`) không mở UoW và không publish —
  consumer sở hữu commit nên consumer sở hữu event *(Red-team: `WithinTx` của
  Teka nối tx ambient bằng `return fn(ctx)`, `tx.go:23-30`, không commit;
  publish từ core sẽ chạy trước commit)*.
- Không mở tx lồng nhau: chỉ method public của `Service` gọi `UnitOfWork`.
- README ghi hợp đồng: "Không gọi use-case có publish từ trong tx ambient của
  bạn"; trần cột (`CountByTenant`) là **check-then-act**, adapter phải
  serialize theo tenant (Teka: advisory lock trong tx) — core không tự lock.

## Architecture

**Package phẳng, không sub-package theo layer.** Domain nhỏ (2 entity), chẻ
`domain/ports/usecase` tạo ceremony và risk import cycle mà không giảm phức tạp
thật (KISS). Tách sub-package chỉ khi có ranh giới thật (ví dụ `kanban/inmemory`
cho consumer khác dùng fake — chưa cần ở v1).

**Ports (DIP).** `Service` chỉ nói chuyện với interface. Signature repository
nhận `tenant TenantID` làm tham số bắt buộc → cross-tenant leak bị chặn ngay ở
mức chữ ký, không phụ thuộc kỷ luật của adapter. `MemberChecker.IsMember(ctx,
tenant, actor) (bool, error)` là port riêng (ISP) để `CreateTask`/`UpdateTask`
từ chối `assignee` không phải thành viên sống của tenant — lib không biết
"membership" là bảng nào.

**Unit of Work.** Port cùng shape với `database.TxManager` app hiện có:

```go
type UnitOfWork interface {
    Within(ctx context.Context, fn func(ctx context.Context) error) error
}
```

Repository adapter phải đọc handle tx từ `ctx`, không giữ `*gorm.DB` riêng.
Trade-off: core không kiểm chứng được điều đó — ràng buộc này thuộc Phase 3 và
được test bằng integration test rollback.

**Policy (Strategy).** Core **enforce**, không chỉ tin middleware. Lý do: lib
claim "reusable"; một consumer khác có thể không có middleware tương đương.
Trade-off: check hai lần với app Teka. Chấp nhận — defense in depth, và hai tầng
kiểm hai thứ khác nhau (capability vs object-level, xem D3 của plan).

```go
type Policy interface {
    CanManageBoard(a Actor, tenant TenantID) bool
    CanReadTask(a Actor, t Task) bool
    CanWriteTask(a Actor, t Task) bool      // edit/delete: owner ∨ creator
    CanMoveTask(a Actor, t Task) bool       // owner ∨ creator ∨ assignee
}
```

Thêm `Visibility(a Actor, tenant TenantID) Visibility` cho `Board`: `All` khi
owner ∨ `view_all`, ngược lại `Participant{a.ID}`. Core cung cấp `DefaultPolicy`
implement đúng 4 luật của brief + visibility, dựa hoàn toàn vào `Actor` +
`Task`; adapter chỉ cần build `Actor` từ `authctx.Scope`. Consumer nào muốn
luật khác thì tự implement interface. Repository chỉ dịch `Visibility` thành
`WHERE`, không tự suy luận từ actor.

**EventSink (Observer).** `Publish(ctx, event any)`, gọi sau `Within` trả nil.
v1 chỉ có **một** event: `ColumnDeleted{ColumnID, MoveTo *ColumnID, MovedCount
int}` — dữ liệu duy nhất mà middleware audit của consumer không nhìn thấy
(`move_to` đi qua query string). Các thao tác khác audit qua request-events
của consumer; bàn giao do consumer publish (xem Non-functional). Giữ port `any`
để consumer khác thêm event mà không đổi chữ ký; nhưng không viết sẵn 8 struct
không ai tiêu thụ *(Red-team: YAGNI + tránh event trước commit)*.

**Functional Options (OCP).** Thêm option không phá compat của `NewService`.

**Testability.** Fake `Repositories` map-backed + `Clock` cố định + `EventSink`
ghi log → assert `CompletedAt` deterministic và thứ tự event. Không Docker.

## Related Code Files

**Create** (tất cả dưới `apps/api/pkg/kanban/`)

- `entity.go` — `Column`, `Task`, `Priority`, các ID type.
- `errors.go` — 10 sentinel errors.
- `ports.go` — `ColumnRepository`, `TaskRepository`, `Repositories`,
  `UnitOfWork`, `Policy`, `MemberChecker`, `EventSink`, `Clock`, `Actor`,
  `Visibility`, `ColumnPatch`.
- `policy.go` — `DefaultPolicy` (4 luật + `Visibility`).
- `service.go` — `Service` + 11 use-case.
- `options.go` — `Config`, `Option`, `WithMaxColumns`, `WithMaxNameLen`,
  `WithClock`, `WithIDGen`, `NewService`.
- `columns.go` — `DefaultColumns(specs)` factory + validate tên/trần
  cột/permutation.
- `events.go` — 1 event struct `ColumnDeleted`.
- `service_test.go` — table-driven tests + fake repos (unexported, trong file
  test).
- `policy_test.go` — ma trận owner/creator/assignee/người lạ × read/write/move.
- `import_boundary_test.go` — chặn import ngoài whitelist.
- `README.md` — ports, cách adapt, luật biên giới, cách trích xuất.

**Modify**: không có.

## Implementation Steps

1. Tạo `apps/api/pkg/kanban/`. Viết `entity.go` với 4 ID type định nghĩa mới
   trên `uuid.UUID` và 2 entity, không tag gorm/json.
2. Viết `errors.go` với 10 sentinel error qua `errors.New`.
3. Viết `ports.go`. `ColumnRepository`: `List`, `Get`, `Create`, `Update`,
   `Delete`, `UpdatePositions`, `CountByTenant`, `ExistsName`. `TaskRepository`:
   `ListBoard(ctx, tenant, Visibility)`, `Get`, `Create`, `Update`,
   `SoftDelete`, `CountInColumn`, `MoveAllToColumn`, `UnassignBy`,
   `ReassignCreator`. Mọi method nhận `tenant TenantID` tham số đầu sau `ctx`.
   Ghi doc trên `CountInColumn`/`MoveAllToColumn`: **tính cả** task đã
   soft-delete (FK RESTRICT của DB vẫn thấy chúng; nếu bỏ qua, `DeleteColumn`
   báo "rỗng" rồi vỡ ở DB). `ListBoard` thì loại soft-deleted.
   `MemberChecker`: `IsMember(ctx, tenant, actor ActorID) (bool, error)`.
4. Viết `policy.go` với `DefaultPolicy` theo 4 luật D3 + `Visibility`.
5. Viết `options.go` + `NewService(repos Repositories, uow UnitOfWork, pol
   Policy, members MemberChecker, sink EventSink, opts ...Option) *Service`.
6. Viết `columns.go`: `DefaultColumns(specs)`; `validateName` (trim, ≤
   MaxNameLen, không rỗng); `validatePermutation(current, incoming []ColumnID)`
   so sánh tập chứ không chỉ độ dài.
7. Viết `service.go`. Quy tắc chung mỗi use-case: (a) `Policy` check trước;
   (b) load entity cần thiết **qua repo ràng theo `tenant`** — id của tenant
   khác trả `ErrColumnNotFound`/`ErrTaskNotFound`, không phân biệt; (c) validate
   invariant; (d) `uow.Within(...)` cho mọi thao tác đa-bước; (e) publish event
   sau khi `Within` trả `nil`, chỉ khi use-case này sở hữu `Within`.
   `UpdateColumn(ctx, tenant, actor, colID, ColumnPatch{Name, IsDone *…})`:
   patch nil-field = không đổi; đổi `IsDone` không đụng task cũ (AC6).
   `CreateTask`/`UpdateTask`: `assignee != nil` → `MemberChecker.IsMember`
   phải `true`, ngược lại `ErrAssigneeNotMember` (DB FK cũng chặn nhưng lỗi
   phải có nghĩa trước khi tới DB).
8. `DeleteColumn(ctx, tenant, actor, colID, moveTo *ColumnID)`: từ chối khi là
   cột duy nhất (`ErrLastColumn`); khi `CountInColumn > 0` mà `moveTo == nil` →
   `ErrColumnNotEmpty`; `moveTo == colID` → `ErrInvalidInput`; `moveTo` phải
   load được qua `columns.Get(tenant, *moveTo)` (khác tenant →
   `ErrColumnNotFound`); ngược lại một tx `MoveAllToColumn` (cập nhật
   `completed_at` theo `IsDone` cột đích) rồi `Delete`; sau `Within` nil →
   `Publish(ColumnDeleted{colID, moveTo, movedCount})`.
9. `MoveTask`: cột đích load qua `columns.Get(tenant, …)`; `CanMoveTask`; set
   `CompletedAt = clock.Now()` khi cột đích `IsDone`, `nil` khi không; position
   mặc định "lên đầu cột" = `minPosition - 1`.
10. `HandoverOnDeparture(ctx, tenant, departed ActorID, newOwner ActorID)
    (unassigned, reassigned int, err error)`: **không** mở `uow.Within`,
    **không** publish — gọi `UnassignBy(departed)` rồi `ReassignCreator(departed
    → newOwner)` trên ctx nhận được; consumer bọc trong tx của mình và publish
    event của mình sau commit (Teka: `centers.RemoveMember`). Doc comment nêu
    rõ hợp đồng này *(Red-team)*.
11. Viết fake repos + fake `MemberChecker` trong `service_test.go` (map-backed,
    không export) và table-driven test cho mọi use-case + mọi sentinel error;
    fake `TaskRepository` giữ cả task soft-deleted để test `DeleteColumn` đếm
    đúng.
12. Viết `policy_test.go`: 4 actor (owner, creator, assignee, người lạ) × 3 luật.
13. Viết `import_boundary_test.go`:

    ```go
    out, err := exec.Command("go", "list", "-deps", ".").Output()
    // fail nếu bất kỳ dòng nào chứa "teka/apps/api/internal",
    // "github.com/gin-gonic", hoặc "gorm.io"
    ```

    Whitelist: mọi package stdlib + `github.com/google/uuid`.
14. Viết `README.md`: bảng ports, sơ đồ luồng use-case → port, ví dụ adapter tối
    giản, luật biên giới, quy trình `git mv` khi tách repo.
15. Gates: `go test ./pkg/kanban/...`, `make lint-api`,
    `go test ./tools/...` (scopelint tự kiểm — `pkg/` không có Scope nên không
    ảnh hưởng, xác nhận bằng chạy thật).

## Success Criteria

- [x] `go test ./pkg/kanban/...` xanh, chạy < 2s, không cần Docker. (0.078–0.7s
      tuỳ có tính compile hay không, đã đo nhiều lần.)
- [x] `import_boundary_test.go` fail khi cố tình thêm import `internal/...`
      (kiểm chứng thủ công một lần rồi revert) → AC-LIB.
- [x] `go list -deps ./pkg/kanban` chỉ ra stdlib + `github.com/google/uuid`
      (+ `vendor/golang.org/x/net/dns/dnsmessage` — vendor nội bộ của `net`
      trong stdlib, kéo theo bởi `uuid` dùng `net.Interfaces()`, không phải
      third-party).
- [x] `DefaultColumns(specs)` gán position 0..n-1 và giữ cờ `IsDone` theo
      spec; `grep -rn "Cần làm" pkg/kanban` → 0 hit → nền cho AC7 + AC-LIB.
- [x] Test chứng minh: quá 8 cột → `ErrColumnLimit`; trùng tên (không phân biệt
      hoa thường) → `ErrDuplicateColumnName`; reorder thiếu/thừa id →
      `ErrInvalidPermutation`; xoá cột cuối → `ErrLastColumn` → AC4, AC5.
- [x] Test chứng minh: move vào cột `IsDone` set `CompletedAt` bằng `Clock` cố
      định; move ra clear; đổi cờ `IsDone` của cột không đụng task cũ → AC6.
- [x] Test chứng minh: assignee move được, không edit/delete được; creator và
      owner edit/delete được → AC3.
- [x] Test chứng minh: `DeleteColumn` có `moveTo` thuộc tenant khác →
      `ErrColumnNotFound`; `CreateTask` với assignee không phải thành viên →
      `ErrAssigneeNotMember`; cột chỉ còn task soft-deleted vẫn cần `moveTo`.
- [x] Fake `EventSink` không nhận `ColumnDeleted` khi `UnitOfWork` trả error;
      nhận đúng 1 event với `MovedCount` khi thành công.
- [x] `HandoverOnDeparture` không gọi `UnitOfWork.Within` và không gọi
      `EventSink` (fake ghi lại số lần gọi = 0).
- [x] `README.md` liệt kê đủ 8 port, hợp đồng tx/event, hợp đồng lock trần
      cột, và ví dụ adapt.

## Risk Assessment

| Rủi ro | Mức | Mitigation |
|---|---|---|
| `go list -deps` trong test chạy chậm hoặc fail trong sandbox CI | Trung × Trung | Cache `GOFLAGS`; nếu `exec` bị chặn, fallback sang đọc `go/build` `ImportPath` của package. Quyết định fallback ngay ở bước 13, không để lộ ra CI. |
| Core enforce Policy trùng lặp với middleware → luật lệch nhau | Trung × Cao | Phân vai tường minh (D3): middleware kiểm capability key, core kiểm object-level. Integration test Phase 3 chốt cả hai tầng cùng ra 403. |
| ID type định nghĩa mới gây nhiều conversion ồn ào ở adapter | Cao × Thấp | Chấp nhận: một hàm convert nhỏ trong adapter, đổi lại compiler chặn nhầm ID. |
| Position float64 cạn kiệt precision khi liên tục chèn đầu cột | Thấp × Trung | v1 dùng `min - 1` (giảm tuyến tính, không chia đôi) nên không cạn. Ghi vào README rằng đổi sang fractional indexing cần renormalize. |
| `UnitOfWork` nhận đúng ctx nhưng adapter quên `FromContext` → thao tác ngoài tx | Trung × Cao | Không kiểm được ở core. Chuyển thành success criterion của Phase 3 (test rollback thật). |
| Consumer gọi `DeleteColumn` từ trong tx ambient → event bay trước commit | Thấp × Trung | Hợp đồng README + Teka handler gọi service ngoài tx (integration test Phase 3: audit row chỉ xuất hiện sau commit). |
| Trần 8 cột bị vượt do 2 request song song (check-then-act) | Thấp × Thấp | Core ghi hợp đồng; adapter Teka lock theo tenant (Phase 3). |

**Giả định có thể sai:** package phẳng đủ cho vòng đời lib. *Tín hiệu vỡ:*
`service.go` vượt ~600 dòng hoặc xuất hiện nhóm hàm không dùng chung state.
*Ứng phó:* tách theo *capability* (`columns.go`/`tasks.go` cùng package), không
tách theo layer.
