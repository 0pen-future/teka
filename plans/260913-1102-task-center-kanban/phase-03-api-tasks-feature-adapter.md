---
phase: 3
title: "API tasks feature adapter"
status: completed
priority: P1
effort: "2d"
dependencies: [1, 2]
---

# Phase 3: API tasks feature adapter (`internal/features/tasks`)

## Overview

Nối `pkg/kanban` vào Teka: GORM repositories, UoW adapter, policy adapter từ
`authctx.Scope`, handlers Gin, routespec entries, mapping error → `apperror`,
publish event lên bus, case audit mới, endpoint directory thành viên, seed cột
khi tạo trung tâm, và bàn giao việc khi thành viên rời trung tâm.

Toàn bộ "biết Teka" nằm ở phase này. `pkg/kanban` không đổi một dòng.

## Requirements

### Functional

- 11 endpoint theo brief §4 (board, 4 cột, 5 việc, 1 directory), mỗi cái một
  entry `routespec.Specs`.
- `GET /api/v1/tasks/board?scope=mine|center&assignee_id=` trả `columns[]` theo
  position + tasks nhóm theo `column_id`, mỗi cột ≤ 50 + cursor.
  `scope=center` chỉ hiệu lực khi caller có `tasks.view_all`, ngược lại degrade
  về `mine` (không 403 — tránh rò rỉ thông tin về quyền).
- `GET /api/v1/centers/me/members/directory` trả `[{teacher_id, display_name,
  role_name}]`, chỉ stint đang sống, **không** SĐT/email.
- `DELETE /api/v1/centers/me/members/:teacherId` giữ nguyên contract, thêm bước
  bàn giao trong cùng tx.
- `CreateCenter` seed 3 cột mặc định từ `kanban.DefaultColumns(specs)` trong
  cùng tx; specs tiếng Việt sống ở `centers/default_columns.go` (`centers` đã
  là nơi gọi `CreateCenter`, chỉ cần import `pkg/kanban`) và là cùng literal
  với khối `-- default-columns` của 000022.
- `CreateTask`/`UpdateTask` với `assignee_id` không phải thành viên sống → 422
  (`ErrAssigneeNotMember`), không phải 500 từ FK.
- Audit: `task.create/update/move/delete`, `task_column.create/update/reorder`
  qua request-events (7 route); `task_column.delete` qua service-level event
  `tasks.ColumnDeleted` vì `move_to` là query string mà middleware bỏ
  (`request_events.go:110-116`); `task.handover` qua service-level event
  `centers.MemberTasksHandedOver` do `centers` publish sau commit *(Red-team)*.

### Non-functional

- scopelint: repository của tasks **không** nhận `authctx.Scope` (port của core
  nhận `kanban.TenantID`), nên R1 **không bắn** cho package này — nói thẳng,
  không mượn uy tín của linter *(Red-team)*. Thay thế: integration test tenancy
  2-center phủ **mọi** method của cả hai port (id của center B qua tenant A →
  not-found/0 dòng). Không `NewDB`/`Unscoped`; không thêm
  `//scopelint:unscoped`. R3 không liên quan vì repo không đọc `IsOwner`/`Has`.
- Package `tasks` có **một** struct repository (`gormRepository`) implement cả
  hai port — scopelint chỉ nhận receiver đầu tiên theo tên
  (`analyzer.go:145-169`); hai file `column_repository.go`/`task_repository.go`
  được phép nếu method cùng receiver.
- `centers` **không** import `tasks`; `audit` **được** import `tasks` và
  `centers`; `tasks` **không** import `audit`.
- Không có lỗi Postgres thô thoát ra 500 từ các route tasks: `translateDBError`
  dịch theo constraint name (khuôn `centers/repository.go:872-883`).
- `make api-docs` regenerate sạch (swag annotation đầy đủ).

## Architecture

**Adapter (DIP).** `internal/features/tasks` implement 8 port của `pkg/kanban`
(`MemberChecker` query `center_memberships` stint sống). Service của feature là
một facade mỏng: dịch `authctx.Scope` → `kanban.Actor` + `kanban.TenantID`, gọi
`kanban.Service`, dịch sentinel error → `apperror`.

Luồng: `Handler (gin) → tasks.Service (facade) → kanban.Service (core) →
tasks repositories (GORM) → database.FromContext(ctx, db)`.

**Consumer-defined interface + setter injection.** `TaskHandover` khai báo trong
`centers` cạnh `AccountDisabler` (`centers/service.go:24`), inject bằng
`centersSvc.SetTaskHandover(tasksSvc)` **trong `server.registerFeatures`**
(`internal/server/router.go:108`) ngay sau khi dựng `tasksSvc`, đúng brief §4.
*(Red-team: bản nháp đưa vào `container.go` với lý do CLI cần bàn giao — sai:
các lệnh CLI là `create_center/seed/reset_password/migrate/serve`, không lệnh
nào gọi `RemoveMember`; seed cột cho `create_center` đi qua `CreateCenter` của
repository nên CLI vẫn đúng. Đặt ở container sẽ phải đổi chữ ký `NewRouter`
10 tham số + 3 call site.)* Setter **nil-safe**: chưa set → log Warn và tiếp
tục, giống `bus` nil-safe ở `centers/service.go:50-54` — thu hồi quyền quan
trọng hơn bàn giao; 7 test hiện có gọi `RemoveMember` qua `centers.NewService`
không set handover vẫn xanh. Bằng chứng wiring là test trong package `server`
(dựng router thật, assert handover đã set) chứ không phải lỗi runtime.

**Observer 2 nhánh, có lý do.** Request-events middleware chỉ sinh audit cho
route đang gọi và chỉ thấy `c.Params`. Hai chỗ nó không đủ:

- Bàn giao là *tác dụng phụ* bên trong tx của `RemoveMember` → không có route
  riêng. `centers.RemoveMember` gọi `TaskHandover` trong closure `WithinTx`,
  nhận `(unassigned, reassigned)`, và publish `centers.MemberTasksHandedOver
  {CenterID, MemberID, SuccessorID, Unassigned, Reassigned}` **sau khi**
  `WithinTx` ngoài cùng trả nil — đúng khuôn `ReplaceRolePermissions`
  (`centers/service.go:463-490`). *(Red-team: `WithinTx` lồng chỉ
  `return fn(ctx)` (`tx.go:23-30`); publish từ bên trong closure hoặc từ core
  sẽ ghi audit cho một tx có thể rollback.)*
- Xoá cột: `move_to` là query string, middleware bỏ → core publish
  `kanban.ColumnDeleted` sau `Within` nil, adapter `EventSink` dịch sang
  `tasks.ColumnDeleted{CenterID, ActorID, ColumnID, MoveTo, MovedCount}` lên
  bus; entry routespec dùng `Audit{Source: SourceService}`.

Hai `case` mới trong `audit/subscriber.go` cạnh `:209`. Trade-off: hai đường
audit trong một feature; bù lại giữ "một route một entry Specs", không bịa
route giả. Bus at-most-once vẫn là blind spot chung của hệ (giữ precedent
`RolePermissionsChanged`, không ghi audit row thẳng trong tx).

**Policy adapter (Strategy).** `tasks.actorFrom(sc)` trả
`kanban.Actor{ID, IsOwner: sc.IsOwner, Perms: map mới}` với đúng 2 key core cần
(`tasks.manage_board`, `tasks.view_all`) — build map mới mỗi request, không
trỏ vào map của `Scope`. Core dùng `kanban.DefaultPolicy`.

**Read-widening ở đâu.** `view_all` **đi vào core** qua `Actor.Perms`:
`DefaultPolicy.Visibility` quyết định `All`/`Participant`, repository chỉ dịch
`Visibility` thành `WHERE` (`center_id = ?` luôn, cộng `(created_by = ? OR
assignee_id = ?)` khi `Participant`). Repository không nhận `Scope`, không gọi
`CenterWideFor` *(Red-team: gộp luật về một nơi, và thẳng thắn rằng scopelint
không canh package này — xem Non-functional)*. `scope=center` degrade về `mine`
khi `Visibility` là `Participant`; response echo scope đã áp dụng.

**Lỗi DB có nghĩa.** `tasks/errors.go` có `translateDBError(err)`: 23505 trên
unique tên cột → `kanban.ErrDuplicateColumnName` (422); 23503 trên FK
`(column_id, center_id)` RESTRICT → `ErrColumnNotEmpty` (409); 23503 khác
(assignee/creator không phải thành viên) → `ErrAssigneeNotMember` (422). Trần
8 cột: `CountByTenant` chạy sau `SELECT pg_advisory_xact_lock(hashtext(center_id))`
trong tx của `CreateColumn` để 2 request song song không vượt trần.

**Testability.** Core đã có unit test không DB (Phase 2). Phase này test cái mà
core không kiểm được: SQL scoping, atomicity tx, wiring, HTTP status. Dùng
`internal/testutil` + testcontainers.

## Related Code Files

**Create** (dưới `apps/api/internal/features/tasks/`)

- `model.go`, `dto.go`, `errors.go` (map sentinel → apperror +
  `translateDBError`)
- `repository.go` (struct `gormRepository`), `column_repository.go`,
  `task_repository.go` (method trên cùng receiver, implement 2 port),
  `member_checker.go` (port `MemberChecker`)
- `uow.go` (wrap `database.TxManager`), `policy.go` (Scope → Actor)
- `service.go` (facade + `HandoverOnDeparture`), `events.go`
  (`ColumnDeleted` + adapter `EventSink`)
- `handler.go`, `routes.go`
- `service_test.go`, `rbac_integration_test.go`, `board_integration_test.go`,
  `column_delete_integration_test.go`, `handover_integration_test.go`,
  `tenancy_integration_test.go`

**Modify**

- `apps/api/internal/shared/routespec/routespec.go` — 11 entry vào `Specs`
  (`:130`): `none()` (`:117`) cho 3 route đọc, `req()` (`:121`) cho 7 route
  ghi, và một entry tay `Audit{Source: SourceService}` cho
  `DELETE /task-columns/:id`.
- `apps/api/internal/features/audit/action_test.go` — `actionSnapshot` (`:42`)
  thêm 7 hàng SourceRequest mới (loop bidirectional `:136-143` fail nếu
  thiếu) *(Red-team)*.
- `apps/api/internal/features/centers/service.go` — interface `TaskHandover`
  cạnh `AccountDisabler` (`:24`), setter nil-safe cạnh `SetAccountDisabler`
  (`:57`), gọi trong `RemoveMember` (`:277`) trước `CloseMembership` (`:299`),
  publish `MemberTasksHandedOver` sau `WithinTx` (khuôn `:463-490`).
- `apps/api/internal/features/centers/events.go` (hoặc file event hiện có) —
  struct `MemberTasksHandedOver` + `EventName()`.
- `apps/api/internal/features/centers/default_columns.go` (mới) — literal
  `[]kanban.DefaultColumnSpec` tiếng Việt, nơi duy nhất phía Go.
- `apps/api/internal/features/centers/repository.go` — `CreateCenter` (`:349`)
  seed 3 cột.
- `apps/api/internal/features/centers/handler.go`, `routes.go`, `dto.go` —
  endpoint directory.
- `apps/api/internal/server/router.go` — `registerFeatures` (`:108`) dựng
  `tasksSvc`, mount routes, gọi `centersSvc.SetTaskHandover(tasksSvc)`.
  `container.go` và chữ ký `NewRouter` **không đổi**.
- `apps/api/internal/server/` — test wiring mới (cùng package, dựng router qua
  `registerFeatures` như `router_test.go:69`) assert handover đã được set.
- `apps/api/internal/features/audit/subscriber.go` — `case
  centers.MemberTasksHandedOver` và `case tasks.ColumnDeleted` cạnh `:209`.
- `apps/api/internal/features/centers/rbac_integration_test.go` — case remove
  member kèm bàn giao.
- `docs/event-bus.md` — 2 event mới + blind spot "reorder chỉ có after-order".
- `apps/api/internal/server/route_policy_test.go` (`:16`,
  `TestRoutePolicyCoversEveryRegisteredRoute`) — không sửa; phải xanh nhờ
  Specs *(sửa tên: không phải `router_test.go`)*.

## Implementation Steps

1. Đọc `sessions/repository.go` (khuôn `database.FromContext` + bind
   `center_id` hiển thị — repo của tasks bind theo `tenant`, không theo
   `Scope`), `centers/service.go:277-311` (khuôn tx `RemoveMember`) và
   `:463-490` (khuôn publish sau commit) trước khi viết.
2. `model.go`: struct GORM `TaskColumn`, `Task` với tag khớp migration 000022;
   hàm convert 2 chiều sang entity của core (`toCore`, `fromCore`).
3. `repository.go`: một struct `gormRepository{db *gorm.DB}`; method port chia
   vào `column_repository.go`/`task_repository.go`. Mọi query mở bằng
   `database.FromContext(ctx, r.db).Where("<table>.center_id = ?", tenant)`.
   `ListBoard` nhận `kanban.Visibility`, thêm `(created_by = ? OR assignee_id
   = ?)` khi `Participant`, loại `deleted_at IS NOT NULL`. `CountInColumn`/
   `MoveAllToColumn` **không** lọc `deleted_at` (FK RESTRICT thấy cả dòng
   soft-deleted). `CountByTenant` chạy `pg_advisory_xact_lock` theo tenant
   trước khi đếm. `member_checker.go`: `IsMember` query stint `left_at IS
   NULL` theo `(center_id, teacher_id)`.
4. `uow.go`: struct một field wrap `database.TxManager`, method `Within` gọi
   `WithinTx`. Một dòng, giữ core không phụ thuộc kiểu cụ thể của app.
5. `policy.go`: `actorFrom(sc authctx.Scope) kanban.Actor` — `Perms` là map
   literal mới với 2 key.
6. `errors.go`: `translate(err error) error` map sentinel → `apperror`:
   `ErrColumnNotFound`/`ErrTaskNotFound` → 404; `ErrColumnLimit`/
   `ErrDuplicateColumnName`/`ErrInvalidPermutation`/`ErrInvalidInput`/
   `ErrAssigneeNotMember` → 422 kèm `fields`; `ErrColumnNotEmpty`/
   `ErrLastColumn` → 409; `ErrForbidden` → 403. Cộng `translateDBError(err)`
   trong repository: `*pgconn.PgError` 23505 (index tên cột) →
   `ErrDuplicateColumnName`; 23503 theo `ConstraintName` (FK cột RESTRICT →
   `ErrColumnNotEmpty`; FK assignee/creator → `ErrAssigneeNotMember`); còn lại
   trả nguyên. Khuôn: `centers/repository.go:872-883`.
7. `service.go`: facade cho 11 use-case + `HandoverOnDeparture(ctx, centerID,
   teacherID, ownerID) (int, int, error)`: **không** mở tx, **không** publish;
   dùng `database.FromContext` nên tự nằm trong tx mà `centers` mở; chỉ
   forward counts của core.
8. `events.go`: `ColumnDeleted{CenterID, ActorID, ColumnID, MoveTo *uuid.UUID,
   MovedCount int}` + `EventName() "tasks.column_deleted"`. Adapter `EventSink`
   type-switch `kanban.ColumnDeleted` → publish lên `events.Bus`; event lạ →
   log Warn, không panic. Facade gọi core **ngoài** mọi `WithinTx` để core sở
   hữu tx ngoài cùng (README Phase 2).
9. `handler.go` + `routes.go`: 10 route của tasks; parse DTO bằng
   `ShouldBindJSON`, lấy `scope` từ `authctx.ScopeFrom(c)`, **không** nhận
   `center_id`/`teacher_id` từ request. Thêm swag annotation đầy đủ.
10. Endpoint directory: thêm handler vào `centers/handler.go` + route vào
    `centers/routes.go` + DTO vào `centers/dto.go`; query chỉ trả
    `teacher_id, display_name, role_name` với `left_at IS NULL`.
11. `routespec.go`: 11 entry. Board/GET task/directory dùng `none()`; 7 route
    ghi dùng `req(action, entityType, idParam)` đúng bảng brief §4;
    `DELETE /task-columns/:id` khai báo tay với `Audit{Source: SourceService,
    Action: "task_column.delete"}`. Ngay sau đó thêm 7 hàng vào
    `actionSnapshot` (`audit/action_test.go:42`) và chạy
    `go test ./internal/features/audit/...` — không đợi cuối phase.
12. `centers/service.go`: khai báo `TaskHandover interface { HandoverOnDeparture
    (ctx, centerID, teacherID, ownerID uuid.UUID) (int, int, error) }`, setter,
    và gọi **trước** `CloseMembership` trong closure `WithinTx` của
    `RemoveMember`, giữ counts vào biến ngoài closure. Nil-safe: chưa set →
    `log.Warn("task handover not wired")` và tiếp tục (giống `bus` nil-safe
    `:50-54`). Sau khi `WithinTx` trả nil: `s.publish(MemberTasksHandedOver{…})`
    (khuôn `ReplaceRolePermissions` `:463-490`). Bổ sung `case` tương ứng vào
    `audit/subscriber.go` với action `task.handover`, entity `task`, payload
    `{member_id, successor_id, unassigned, reassigned}`.
13. `centers/default_columns.go`: `var defaultColumnSpecs =
    []kanban.DefaultColumnSpec{{"Cần làm", false}, {"Đang làm", false},
    {"Hoàn thành", true}}` — literal duy nhất phía Go (lib không chứa tiếng
    Việt, `centers` không import `tasks`; đường CLI `create_center` đi qua
    cùng `CreateCenter` nên không cần wiring thêm). `centers/repository.go`
    `CreateCenter`: INSERT 3 cột từ `kanban.DefaultColumns(defaultColumnSpecs)`
    sau khi INSERT roles, cùng tx. `centers` import `pkg/kanban` (được phép —
    không phải `features/tasks`).
14. `server/router.go` `registerFeatures`: dựng `tasksSvc`, mount
    `tasks.RegisterRoutes(v1, ...)`, gọi `centersSvc.SetTaskHandover(tasksSvc)`.
    Thêm test wiring trong package `server`.
15. `app/container.go`: **không đổi**.
16. `audit/subscriber.go`: `case tasks.ColumnDeleted` → action
    `task_column.delete`, entity `task_column`, payload `{column_id, move_to,
    moved_count}`; `case centers.MemberTasksHandedOver` như bước 12.
17. Tests: quartet RBAC (owner bypass / có grant / không grant / deny override)
    cho cả task và column; `tenancy_integration_test.go` 2 center × mọi method
    port (thay cho scopelint R1); delete-with-move nguyên tử (assert rollback
    khi bước xoá fail; audit `task_column.delete` chỉ xuất hiện khi commit);
    xoá cột chỉ còn task soft-deleted → 409; reorder sai permutation → 409;
    2 request tạo cột song song ở trần 7 → đúng một 422; assignee không phải
    thành viên → 422; trùng tên cột qua DB race → 422; board `scope=center`
    không có `view_all` → chỉ thấy việc của mình; xoá thành viên → bàn giao
    trong cùng tx, audit `task.handover` có counts, và **0 audit row khi
    `Disable` lỗi**; `RemoveMember` khi chưa `SetTaskHandover` → vẫn 200 + log
    Warn; seed cột khi `CreateCenter`.
18. Gates: `make scopelint`, `make test-api-unit`, `make test-api`,
    `make lint-api`, `make api-docs` (commit spec regenerate).

## Success Criteria

- [x] `route_policy_test.go` xanh không sửa: 11 route đều có entry Specs, mọi
      route ghi có audit source và action; `audit/action_test.go` xanh với 7
      hàng mới → AC9.
- [x] Test wiring trong package `server` chứng minh `SetTaskHandover` đã gọi
      trong `registerFeatures`; `git diff --stat internal/app/container.go`
      rỗng.
- [x] `tenancy_integration_test.go`: mọi method của 2 port với id center B qua
      tenant A → not-found/0 dòng → AC2.
- [x] Owner không gán gì vẫn CRUD việc + cấu hình cột + xem toàn board → AC1.
- [x] Member chỉ `tasks.list` thấy đúng việc mình tạo/được giao; thêm
      `tasks.view_all` thấy toàn trung tâm; center khác 0 dòng → AC2.
- [x] Assignee `POST /tasks/:id/move` 200; `PATCH /tasks/:id` 403;
      `DELETE /tasks/:id` 403 → AC3.
- [x] Thiếu `tasks.manage_board`: 4 route cột đều 403 → AC4.
- [x] `DELETE /task-columns/:id` không `move_to` khi cột có việc → 409; có
      `move_to` → 200 và task đổi cột trong cùng tx; cột cuối → 409 → AC5.
- [x] Move vào cột `is_done` ghi `completed_at`; move ra clear; `PATCH` cờ
      `is_done` không đụng `completed_at` cũ → AC6.
- [x] `CreateCenter` sinh 3 cột → AC7.
- [x] `RemoveMember`: `assignee_id` NULL, `created_by` = owner, audit
      `task.handover` có số việc và chỉ ghi sau commit; rollback nguyên vẹn
      **và 0 audit row** khi `Disable` lỗi → AC10.
- [x] Không route tasks nào trả 500 cho unique/FK violation; test race trần cột
      và trùng tên xanh.
- [x] `make scopelint` xanh (không thêm `//scopelint:unscoped`);
      `grep -rn "features/tasks" centers/` → 1 hit thực tế, nhưng chỉ trong
      `centers/rbac_integration_test.go` (package `centers_test` biên dịch
      riêng, không tham gia đồ thị import production) — không tạo import
      cycle `centers` ↔ `tasks` mà rule này nhắm tới; xem báo cáo phase để
      biết lý do;
      `grep -rn "authctx.Scope" features/tasks/*repository*.go` → 0 hit.
- [x] `make test-api` xanh, không giảm coverage floor — xanh khi chạy `-p 1` (tester 2026-09-13 21:57: coverage 77.1%, floor 60%); hai lần FAIL trước đó là lỗi hạ tầng testcontainers (`Timeout waiting for systemd to create docker-...scope`), xem `plans/reports/test-report-260913-task-center-kanban.md`.
      cho bằng chứng thay thế (`tasks` package chạy riêng 23/23 pass).

## Risk Assessment

| Rủi ro | Mức | Mitigation |
|---|---|---|
| scopelint không canh repo của tasks (port nhận `TenantID`, R1 không bắn) → cảm giác an toàn giả | Cao × Cao | Nói rõ trong Non-functional; `tenancy_integration_test.go` phủ mọi method port; grep chốt repo không nhận `Scope`. |
| Quên `SetTaskHandover` → bàn giao im lặng không chạy | Trung × Cao | Nil-safe + Warn (không chặn offboarding); test wiring trong `server` là bằng chứng, cộng integration test `RemoveMember` end-to-end. |
| `HandoverOnDeparture` mở tx riêng → nested tx với tx của `centers` | Trung × Cao | Core lẫn facade **không** gọi `Within`; chỉ dùng `database.FromContext` để nhập tx sẵn có. Integration test rollback là bằng chứng. |
| Event bàn giao/xoá cột publish trước commit | Trung × Cao | `centers` publish sau `WithinTx`; facade gọi core ngoài tx; test "0 audit row khi rollback". |
| `actionSnapshot` bidirectional đỏ vì quên 7 hàng | Cao × Thấp | Bước 11 sửa cùng lúc với routespec và chạy test audit ngay. |
| Literal tên cột mặc định tồn tại 2 nơi (Go + SQL 000022) và lệch nhau | Trung × Trung | Go literal duy nhất ở `centers/default_columns.go`; parity test Phase 1 so SQL với literal đóng băng; test `CreateCenter` assert đúng 3 tên đó. |
| Import cycle `centers` ↔ `tasks` | Trung × Cao | `centers` chỉ import `pkg/kanban` (cho `DefaultColumns`), không import `features/tasks`. Kiểm bằng grep trong success criteria. |
| `audit` import `tasks` tạo cycle nếu `tasks` cần audit type | Thấp × Cao | `tasks` chỉ publish struct của chính nó; không import `audit`. Kiểm bằng grep. |
| Board query N+1 khi 8 cột × 50 việc | Trung × Thấp | Một query tasks theo `center_id` + sort `(column_id, position, created_at)`, group ở Go; index `idx_tasks_board` đã có từ 000022. |
| `scope=center` degrade thầm lặng gây hiểu nhầm "quyền bị mất" | Trung × Thấp | Response echo `scope` thực tế đã áp dụng để web hiển thị đúng segmented control. |

**Giả định có thể sai:** publish event sau commit qua bus at-most-once là đủ
cho audit. *Tín hiệu vỡ:* audit thiếu dòng khi bus buffer đầy
(`API_AUDIT_BUFFER_SIZE`), thấy warning log drop event. *Ứng phó:* giữ khuôn
`RolePermissionsChanged` (red-team đã bác việc ghi audit row thẳng trong tx —
lệch precedent toàn hệ); nếu drop thật sự xảy ra, mở follow-up cho outbox
chung, không vá riêng cho tasks.
