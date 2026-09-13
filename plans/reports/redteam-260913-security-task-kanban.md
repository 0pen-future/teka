---
title: "Red-team review (Security Adversary) — Trung tâm công việc Kanban"
plan: plans/260913-1102-task-center-kanban/
reviewer: redteam-security
role: FACT CHECKER (Tier Full)
date: 2026-09-13
verdict: DONE_WITH_CONCERNS
---

# Red-team review — góc nhìn Security Adversary

Phạm vi: 6 phase file + plan index. Hợp đồng brainstorm được coi là cố định,
không thách thức scope. Mọi finding kèm bằng chứng `file:line` từ codebase tại
`master` (bb13590).

---

## Finding 1: Audit `task.handover` được phát trước khi transaction ngoài commit

- **Severity:** Critical
- **Location:** Phase 2, "Implementation Steps" bước 10 + "Non-functional";
  Phase 3, "Implementation Steps" bước 12 + "Architecture / Observer 2 nhánh"

### Flaw

Phase 2 khai báo bất biến "Publish event **sau khi** UoW commit thành công"
(phase-02 §Non-functional) và bước 10 nói `HandoverOnDeparture` chạy "một tx …
publish `TasksHandedOver` sau commit". Nhưng Phase 3 bước 12 đặt lời gọi này
**bên trong** closure `WithinTx` của `RemoveMember`. `WithinTx` của repo không
mở tx lồng — nó *nhập* tx đang có và trả về ngay khi `fn` xong:

```go
// internal/database/tx.go:23-30
func (m *GormTxManager) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok && tx != nil {
		return fn(ctx)          // <-- không commit, chỉ chạy rồi trả nil
	}
	return m.db.WithContext(ctx).Transaction(...)
}
```

Core nhận `nil` từ `uow.Within` và hiểu nhầm đó là "đã commit" → publish event.
Thực tế outer tx vẫn đang mở.

### Failure scenario

`RemoveMember` (`internal/features/centers/service.go:277-310`) chạy:
handover → `CloseMembership` (`:299`) → `disabler.Disable` (`:302`).
`Disable` fail (auth DB lỗi, token revoke lỗi) → toàn bộ tx rollback → **không
việc nào được bàn giao**. Nhưng event `TasksHandedOver` đã ra bus, audit
subscriber ghi row `task.handover` với `{unassigned: 12, reassigned: 7}`.
Nhật ký hoạt động của trung tâm khẳng định một sự kiện chuyển quyền sở hữu dữ
liệu chưa từng xảy ra. Với auditor nội bộ, đây là bằng chứng giả.

Chiều ngược lại cũng hỏng: bus là at-most-once và **drop khi queue đầy** —
`internal/shared/events/async_bus.go:79-98` (`"events: subscriber queue full,
event dropped"`), queue length là `API_AUDIT_BUFFER_SIZE` mặc định 1024
(`internal/config/config.go:169-171`, `internal/app/container.go:84`). Một đợt
offboard hàng loạt làm mất chính dòng audit mà AC10 yêu cầu.

Thêm nữa, hai phase mâu thuẫn trực tiếp: Phase 3 §Risk Assessment ghi "Facade
**không** gọi `uow.Within`", trong khi Phase 2 bước 10 nói core
`HandoverOnDeparture` mở `uow.Within`. Facade gọi core → core gọi `Within`.
Không phase nào giải quyết mâu thuẫn này.

### Evidence

- `apps/api/internal/database/tx.go:23-30` — `WithinTx` nhập tx sẵn có, không commit
- `apps/api/internal/features/centers/service.go:298-303` — thứ tự trong tx
- `apps/api/internal/shared/events/async_bus.go:79-98` — drop khi đầy
- `apps/api/internal/config/config.go:169-171`, `internal/app/container.go:84`
- Plan: phase-02 bước 10 "publish `TasksHandedOver` sau commit"; phase-03 bước
  12 "gọi **trước** `CloseMembership` trong closure `WithinTx`"; phase-03 Risk
  "Facade **không** gọi `uow.Within`"

### Suggested fix

Với một sự kiện chuyển quyền sở hữu dữ liệu, đừng đi qua bus. Ghi thẳng
`audit_logs` row trong chính tx của `RemoveMember` (Phase 3 đã ghi phương án
này ở mục "Giả định có thể sai" nhưng để nó là dự phòng — nên nâng lên thành
quyết định). Nếu giữ bus: core phải nhận một callback `afterCommit` mà chỉ
`UnitOfWork` gốc (top-level) mới kích hoạt, và ghi rõ trong port contract rằng
`Within` trả `nil` **không** đồng nghĩa với commit.

---

## Finding 2: Nil-guard `TaskHandover` biến lỗi wiring thành mất hoàn toàn khả năng thu hồi quyền truy cập

- **Severity:** Critical
- **Location:** Phase 3, "Implementation Steps" bước 12 ("Nil-guard: nếu chưa
  set thì trả lỗi rõ ràng, không skip im lặng") + Risk Assessment hàng "Quên
  `SetTaskHandover`"

### Flaw

Plan chọn fail-closed sai trục. `RemoveMember` là **đường duy nhất** thu hồi
quyền của một thành viên: nó đóng stint và gọi `disabler.Disable`, mà theo
comment tại `centers/service.go:20-27` phải "flip the account to disabled AND
revoke every refresh token it holds, atomically, so a removed member cannot
keep using an old access token". Chặn `RemoveMember` vì thiếu một dependency
*phụ trợ* nghĩa là chặn thu hồi truy cập.

### Failure scenario

Deploy Phase 3 nhưng dòng `centersSvc.SetTaskHandover(tasksSvc)` bị rớt trong
merge conflict tại `internal/app/container.go` (vùng `:100`). Không test nào
chặn được nếu test wiring cũng nằm trong commit bị rớt. Owner phát hiện một
thành viên rò rỉ dữ liệu học viên, bấm "Xoá thành viên" → 500. Thử lại → 500.
Thành viên đó giữ stint sống và refresh token hợp lệ cho tới khi có người
deploy hotfix. Đây là escalation-by-availability: một feature bảng công việc
làm hỏng đường offboarding.

So sánh với chính codebase: `s.bus` được thiết kế nil-safe tường minh
(`centers/service.go:50-54`, "a nil bus makes every publish a no-op"), còn
`s.disabler` thì không có guard nào — vì nó là *bắt buộc*. `TaskHandover`
thuộc nhóm thứ nhất về mặt rủi ro, không phải nhóm thứ hai.

### Evidence

- `apps/api/internal/features/centers/service.go:20-27` — hợp đồng của `AccountDisabler`
- `apps/api/internal/features/centers/service.go:50-54` — precedent nil-safe cho bus
- `apps/api/internal/features/centers/service.go:277-310` — `RemoveMember` là đường duy nhất
- `apps/api/internal/app/container.go:100` — điểm wiring
- Plan: phase-03 bước 12, Risk "Quên `SetTaskHandover` → bàn giao im lặng không chạy | Trung × Cao"

### Suggested fix

Đẩy guard lên **thời điểm khởi tạo**, không phải thời điểm request: `NewContainer`
panic/return error nếu `tasksSvc == nil` khi build xong (giống cách
`config.Validate` chặn `API_AUDIT_BUFFER_SIZE < 1` tại `config.go:280-282`).
Tại request-time, log ở mức `Error` và **vẫn tiếp tục** offboarding — mất bàn
giao là mất tiện lợi, mất thu hồi truy cập là mất bảo mật.

---

## Finding 3: `move_to` không được validate cùng tenant; `ErrCrossTenant` khai báo nhưng không use-case nào phát

- **Severity:** Critical
- **Location:** Phase 2, "Implementation Steps" bước 8 (`DeleteColumn`) và
  §Functional (danh sách sentinel errors); Phase 3 bước 6 (`translate`)

### Flaw

Bước 9 (`MoveTask`) nói rõ "validate cột đích cùng tenant". Bước 8
(`DeleteColumn`) liệt kê đúng ba guard — cột cuối, `CountInColumn > 0` mà
`moveTo == nil`, `moveTo == colID` — và **bỏ sót guard tenant**. `ErrCrossTenant`
nằm trong danh sách 10 sentinel (phase-02 §Functional) và có mapping sang 403
(phase-03 bước 6) nhưng không có một use-case nào trong plan được mô tả là phát
nó. Một sentinel không ai raise là guard không tồn tại.

Cùng lỗ hổng ở `CreateTask`: brief dòng 185 yêu cầu "Service kiểm tra
`column_id` và `assignee_id` thuộc cùng `center_id` của scope", nhưng không
bước nào của Phase 2 hay Phase 3 hiện thực hoá yêu cầu đó.

### Failure scenario

Attacker có `tasks.manage_board` ở trung tâm A. Gọi:

```
DELETE /api/v1/task-columns/{col_A}?move_to={col_B_của_trung_tâm_khác}
```

Core kiểm tra `col_A` thuộc tenant A (repo signature ép `tenant TenantID`), rồi
gọi `MoveAllToColumn(tenant_A, col_B)`. Không có bước nào load `col_B` để so
tenant. Lưới an toàn duy nhất là FK composite `(column_id, center_id) →
task_columns(id, center_id)` trong DDL brief §4. Hệ quả:

1. Lỗi driver Postgres thô, không phải sentinel → `translate()` (phase-03 bước
   6 chỉ map 10 sentinel) không nhận ra → `apperror.Internal` → **500**.
2. 500 vs 409 vs 200 là **existence oracle**: attacker brute-force UUID cột để
   xác nhận cột nào tồn tại ở trung tâm khác. Cùng kỹ thuật áp cho
   `assignee_id` trong `CreateTask` → enumerate `teacher_id` xuyên trung tâm.
3. Nếu vì bất kỳ lý do gì FK composite bị viết thiếu (`REFERENCES
   task_columns(id)` thay vì `(id, center_id)`) thì không còn lưới nào — mà
   Phase 1 bước 8 chỉ mô tả FK bằng prose, không có test nào ngoài "FK composite
   chặn task trỏ cột khác center" ở bước 11 (chỉ test `tasks`, không test
   đường `move_to`).

### Evidence

- Plan phase-02 bước 8 (3 guard, không có tenant) vs bước 9 ("validate cột đích
  cùng tenant") — bất đối xứng trong cùng một file
- Plan phase-02 §Functional — `ErrCrossTenant` trong danh sách sentinel
- Plan phase-03 bước 6 — `translate()` chỉ map sentinel, lỗi khác rơi xuống Internal
- Brief `reports/brainstorm-brief-rev3-extracted.md:185` — yêu cầu chưa được phase nào nhận
- Precedent fail-closed của repo: `internal/server/route_policy_enforce.go:61-65`
  (route không có manifest entry → deny) cho thấy dự án đã chuẩn hoá "thiếu
  classification = từ chối"; guard tenant nên cùng chuẩn đó

### Suggested fix

Trong `DeleteColumn`, `CreateTask`, `UpdateTask`: load mọi `ColumnID`/`ActorID`
đến từ input qua repo có `tenant` rồi so sánh, trả `ErrCrossTenant` khi lệch.
Thêm vào Phase 2 Success Criteria một dòng: "test chứng minh `move_to` trỏ cột
tenant khác → `ErrCrossTenant`, không phải lỗi DB". Thêm vào Phase 3: "mọi lỗi
không phải sentinel từ repo không được lộ ra ngoài dưới dạng 500 phân biệt được".

---

## Finding 4: Cơ chế audit đã chọn về mặt vật lý không thể ghi `move_to`

- **Severity:** High
- **Location:** Phase 3, §Requirements/Functional ("Audit: … `task_column.
  create/update/reorder/delete` qua request-events") + bước 11; AC5 của plan
  index ("audit ghi cả `move_to`" — brief §4)

### Flaw

Plan phân loại `DELETE /api/v1/task-columns/:id?move_to=` bằng helper
`req(action, entityType, idParam)` (`routespec.go:121`). Middleware
request-events **cố ý** loại bỏ query string:

```go
// internal/middleware/request_events.go:110-114
	// URL.Path, never RequestURI: query strings can carry phone
	// numbers and must not reach the audit table.
	Path:       c.Request.URL.Path,
```

Payload duy nhất còn lại là `c.Params` — **path params**
(`request_events.go:129-135`). `move_to` là query param → không bao giờ tới
`audit_logs`. Đây không phải bug có thể sửa trong tasks; đó là một quyết định
privacy đã đóng băng của dự án.

### Failure scenario

Owner mở Nhật ký hoạt động sau khi phát hiện board bị xáo trộn. Thấy dòng
`task_column.delete / task_column / {id}`. Không biết 40 việc trong cột đó đi
đâu. Kẻ có `tasks.manage_board` xoá cột "Chờ duyệt" và dồn toàn bộ việc vào cột
`is_done` — mọi việc lập tức được đóng dấu `completed_at` (phase-02 bước 8:
"cập nhật `completed_at` theo `IsDone` cột đích") và biến mất khỏi tầm nhìn vận
hành. Audit trail không giữ được bất kỳ dấu vết nào về đích đến. AC5 ghi rõ
"audit ghi cả `move_to`" nhưng không thể đạt được với `req()`.

Vấn đề tương tự, nhẹ hơn: `PUT /api/v1/task-columns/order` không có id param
và không capture body → dòng audit chỉ chứng minh "có ai đó reorder", không
chứng minh thứ tự trước/sau.

### Evidence

- `apps/api/internal/middleware/request_events.go:110-114` — bỏ query string, có comment giải thích
- `apps/api/internal/middleware/request_events.go:129-135` — chỉ capture `c.Params`
- `apps/api/internal/shared/routespec/routespec.go:121-123` — `req()` chỉ nhận action/entityType/idParam
- Precedent đúng: `internal/features/audit/subscriber.go:209-224`
  (`centers.RolePermissionsChanged` mang `before`/`after` vì "Service events
  carry the before/after sets the request middleware cannot see")
- Plan: brief §4 bảng endpoint "audit ghi cả `move_to`"; phase-03 bước 11

### Suggested fix

Chọn một trong hai, và ghi vào Phase 3:
(a) đổi contract sang `POST /api/v1/task-columns/:id/delete` với body
`{move_to}` — vẫn không tự vào audit, nên vẫn cần (b); hoặc
(b) phát service-level event `tasks.ColumnDeleted{ColumnID, MoveTo, MovedCount}`
và thêm `case` vào `audit/subscriber.go` đúng khuôn `:209`. Plan đã dùng đúng
khuôn này cho `task.handover` — chỉ cần áp cho `task_column.delete` và
`task_column.reorder`.

---

## Finding 5: Việc đã soft-delete khoá vĩnh viễn khả năng xoá cột (FK RESTRICT không thấy `deleted_at`)

- **Severity:** High
- **Location:** Phase 1, bước 8 + Success Criteria; Phase 2, bước 8
  (`CountInColumn > 0`)

### Flaw

DDL brief §4 đặt `FOREIGN KEY (column_id, center_id) REFERENCES task_columns
(id, center_id) ON DELETE RESTRICT` — FK **không có predicate**, và Postgres
không hỗ trợ partial FK. Cùng lúc `tasks` có `deleted_at` và index board là
partial: `CREATE INDEX idx_tasks_board ON tasks (center_id, column_id,
position) WHERE deleted_at IS NULL`. Plan không nơi nào định nghĩa
`CountInColumn` đếm có hay không đếm row đã soft-delete.

### Failure scenario

Cột "Chờ duyệt" từng có 5 việc, cả 5 bị xoá mềm (`DELETE /api/v1/tasks/:id` →
soft delete, brief §3). Owner mở modal cấu hình cột, thấy badge "0" (đếm từ
board response, loại `deleted_at`). Bấm xoá. Service gọi `CountInColumn` → 0 →
bỏ qua yêu cầu `move_to` (phase-02 bước 8) → `DELETE FROM task_columns` →
Postgres RESTRICT chặn vì 5 row soft-deleted vẫn tham chiếu.

Kết quả: lỗi driver thô, không phải sentinel → 500 (cùng cơ chế Finding 3).
Cột đó **không bao giờ xoá được qua UI**, và nó vẫn chiếm một slot trong trần 8
cột. Trung tâm dùng lâu sẽ tích luỹ cột zombie và chạm trần, mất khả năng cấu
hình quy trình — chính là giá trị cốt lõi của phương án B.

Chiều ngược lại nếu chọn "đếm cả soft-deleted": UI hiện "cột đang có 5 việc",
owner buộc phải chọn cột đích cho 5 việc **không tồn tại với người dùng** — UX
khó hiểu nhưng ít nhất không kẹt.

### Evidence

- Brief `reports/brainstorm-brief-rev3-extracted.md:147-155` — FK RESTRICT không predicate + index partial `WHERE deleted_at IS NULL`
- Brief `:89` — "Xoá việc: Soft delete `deleted_at`"
- Plan phase-02 bước 8 — chỉ gate trên `CountInColumn > 0`
- Plan phase-01 Success Criteria — "`DELETE` cột còn việc bị RESTRICT chặn" (không phủ trường hợp soft-deleted)
- Plan phase-01 bước 11 — 3 test migration, không có test soft-delete

### Suggested fix

Chốt trong port contract của Phase 2: `CountInColumn` và `MoveAllToColumn`
**bao gồm** row soft-deleted (vì FK cũng vậy), còn selector board thì loại.
Thêm test migration ở Phase 1 bước 11: "xoá cột chỉ chứa task soft-deleted vẫn
bị RESTRICT chặn". Cân nhắc thay `ON DELETE RESTRICT` bằng `ON DELETE NO ACTION
DEFERRABLE` nếu muốn service tự xử lý trong tx.

---

## Finding 6: Không phase nào nhắc `Scope.WriteWide()`, trong khi scopelint R3 cấm `IsOwner`/`Has` trong repository — implementer sẽ bị dồn vào lối tắt nguy hiểm

- **Severity:** High
- **Location:** Phase 3, bước 3 + Risk Assessment hàng "scopelint R3"; plan
  index D3 (`writeOwn = owner ∨ creator`, `writeParticipant = owner ∨ creator ∨
  assignee`)

### Flaw

Plan mô tả `writeOwn`/`writeParticipant` chứa nhánh `owner`, nhưng scopelint R3
báo lỗi **vô điều kiện** khi repository đọc `Scope.IsOwner` hoặc gọi
`Scope.Has`:

```go
// tools/scopelint/scopelint/analyzer.go:474-476, 486-488
if node.Sel.Name == "IsOwner" && scopeKindOf(...) == "Scope" {
	pass.Reportf(node.Pos(), "Scope.IsOwner authority fields are resolved by services, not read in repositories — ...")
...
case isMethodOn(fnObj, authctxPkgPath, "Scope", "Has"):
	pass.Reportf(node.Pos(), "Scope.Has is forbidden in repositories — ...")
```

Lối thoát hợp lệ duy nhất là `Scope.WriteWide()` (`catalog.go:352-354`, chính
thông điệp lỗi của analyzer chỉ tới nó, và 4 feature khác đã dùng:
`sessions/repository.go:114,170`, `statements/repository.go:272`,
`students/repository.go:104`). **Chuỗi "WriteWide" không xuất hiện một lần nào
trong 6 file phase** — chỉ có trong brief ở thư mục `reports/`.

Tệ hơn, hàng Risk duy nhất về scopelint trong Phase 3 chỉ tới **nhánh sai**:
"scopelint R3 chặn fork `CenterWideFor` ngoài hàm 'read'". `CenterWideFor` là
nhánh *đọc* và plan đã xử lý đúng (đặt tên `readScoped`). Nhánh *ghi* — nhánh
thực sự sẽ vỡ — không được nhận diện.

### Failure scenario

Implementer viết `writeOwn(ctx, sc)` với `sc.IsOwner ||` rồi chạy `make
scopelint` (Makefile `:72`) và ăn lỗi ở cuối Phase 3, dưới áp lực. Ba lối thoát
rẻ tiền, cả ba đều là lỗ hổng:

1. Bỏ nhánh owner → **AC1 và AC3 vỡ**: owner không sửa/xoá được việc của thành
   viên khác, nhưng test quartet (owner bypass) có thể vẫn xanh nếu chỉ test
   trên việc do chính owner tạo.
2. Thêm `//scopelint:unscoped <reason>` (`analyzer.go:23`) → tắt luôn kiểm tra
   tenancy R1 cho method đó, không chỉ R3. Một directive trên `writeOwn` nghĩa
   là không còn ai kiểm chứng `center_id` được bind trong đường ghi.
3. Chuyển owner-check lên service nhưng để repository trả row chưa lọc creator
   → TOCTOU giữa service check và query.

### Evidence

- `apps/api/tools/scopelint/scopelint/analyzer.go:470-490` — R3 báo lỗi vô điều kiện cho `IsOwner` và `Has`
- `apps/api/tools/scopelint/scopelint/analyzer.go:427-438` — `hasReadToken` chỉ áp cho `CenterWideFor`
- `apps/api/tools/scopelint/scopelint/analyzer.go:20-23` — `directivePrefix = "//scopelint:unscoped"`
- `apps/api/internal/shared/authctx/catalog.go:352-354` — `WriteWide()`
- `apps/api/internal/features/sessions/repository.go:114,170` — precedent đúng
- `grep -rn "WriteWide" plans/260913-1102-task-center-kanban/*.md` → 0 hit trong phase files
- Plan phase-03 Risk: "scopelint R3 chặn fork `CenterWideFor` ngoài hàm 'read' | Cao × Thấp"

### Suggested fix

Sửa Phase 3 bước 3 thành: "`writeOwn`/`writeParticipant` biểu diễn nhánh owner
bằng `sc.WriteWide()`, không bao giờ `sc.IsOwner`; `readScoped` dùng
`sc.CenterWideFor(authctx.PermTasksViewAll)`". Thêm Success Criterion: "không
có `//scopelint:unscoped` directive mới trong `internal/features/tasks`".

---

## Finding 7: Migration 000022 cấp `members.list` cho mọi thành viên hiện hữu mà owner không hay biết

- **Severity:** High
- **Location:** Phase 1, §Requirements/Functional ("backfill 6 khoá mới vào 3
  vai trò hệ thống") và Success Criteria (`len(DefaultRoleKeys()) == 59`)

### Flaw

Trong 6 khoá được backfill, 5 khoá là `tasks.*` trên bảng rỗng — vô hại. Khoá
thứ sáu là `members.list`, mở một **directory danh bạ toàn bộ thành viên** trên
dữ liệu đã tồn tại. Plan đối xử với nó như một khoá CRUD low-risk bình thường
và không thảo luận riêng ở bất kỳ đâu.

Điều này đi ngược chính nguyên tắc mà catalog đã ghi thành code:

```go
// internal/shared/authctx/catalog.go:296-299
// legacyIdentitySet is the pre-catalog identity keys ... They stay out
// of the default baseline — membership never granted them, so backfilling
// them would escalate.
```

`members.list` chưa từng tồn tại; không thành viên nào từng đọc được danh bạ.
Brief dòng 57 tự gọi tình trạng hiện tại là "Lỗ hổng: danh sách thành viên chỉ
trả cho owner qua `GET /centers/me`". Nói cách khác, plan lấy một hạn chế hiện
hành và gỡ nó bằng một câu SQL trong migration, cho mọi trung tâm cùng lúc.

### Failure scenario

Trung tâm chạy `make migrate-up`. Không có thông báo, không có bước owner chấp
thuận. Ngay sau đó mọi trợ giảng ở mọi trung tâm gọi
`GET /api/v1/centers/me/members/directory` và nhận về `[{teacher_id,
display_name, role_name}]` của toàn bộ nhân sự — kể cả người họ chưa từng làm
việc cùng, kèm vai trò (thông tin cấu trúc tổ chức). Với trung tâm dùng Teka
cho nhiều cơ sở, đây là rò rỉ danh sách nhân sự. Owner không có cách nào biết
quyền này vừa được cấp, vì ma trận Phân quyền chỉ hiển thị "đã tick" — giống hệt
một khoá owner tự tick.

Trớ trêu: plan đã xây đúng cơ chế để tránh việc này (`DefaultGrant: false`) và
áp cho `tasks.manage_board`, nhưng không áp cho khoá duy nhất chạm dữ liệu thật.

### Evidence

- `apps/api/internal/shared/authctx/catalog.go:296-309` — nguyên tắc "backfilling them would escalate"
- `apps/api/internal/shared/authctx/catalog.go:330-338` + đo thực tế `len(DefaultRoleKeys()) == 53`
- Brief `reports/brainstorm-brief-rev3-extracted.md:57` — thừa nhận trạng thái hiện tại là hạn chế có chủ đích
- Brief `:178` — `members.list` / crud / low / "3 vai trò"
- Plan phase-01 §Functional và Success Criteria (53 → 59)
- Đối chiếu: `members` đã nằm trong `ADMIN_RESOURCES`
  (`apps/web/src/features/center/schemas/permission-schemas.ts:127-135`) — dự án
  đã xếp resource này vào nhóm quản trị

### Suggested fix

Khai báo `members.list` với `DefaultGrant: false` cùng `tasks.manage_board` →
`DefaultRoleKeys()` thành 58, backfill 5 khoá. UI form "Giao cho" sẽ trống cho
tới khi owner bật khoá — đúng fail-closed, và owner nhìn thấy quyết định đó
trong ma trận. Nếu người dùng vẫn muốn backfill, đưa nó lên bảng "Decisions" của
plan index như một quyết định tường minh, không để nó ẩn trong con số "6 khoá".

---

## Finding 8: `DefaultGrant` mặc định về phía không an toàn cho backfill, và không lộ ra API ma trận

- **Severity:** Medium
- **Location:** Phase 1, bước 2-4 và §Architecture ("SOLID (OCP)")

### Flaw

Plan đặt `DefaultGrant: true` **trong thân** `def()`/`viewAll()`
(`catalog.go:130-145`). `PermDef` là struct exported, field thường
(`catalog.go:34-46`). Hôm nay cả 71 entry đều đi qua helper (đã kiểm chứng), nên
plan đúng ở thời điểm này. Nhưng zero value của `bool` là `false` → bất kỳ entry
tương lai nào viết dạng struct literal sẽ **im lặng rơi khỏi** `DefaultRoleKeys()`.

Hướng an toàn cho *enforcement* là deny-by-default, nhưng đây không phải
enforcement — đây là danh sách backfill. Hỏng theo hướng này nghĩa là "một vai
trò mất khoá vận hành lẽ ra phải có", phát hiện ở production khi thành viên báo
403. Lưới duy nhất là con số 59 hardcode trong
`TestDefaultRoleKeysPreserveLegacyBaseline` (`catalog_test.go:246`) — mà chính
plan yêu cầu sửa bằng tay mỗi lần thêm khoá (phase-01 bước 6), nên nó không phải
lưới, nó là một con số người ta cập nhật cho test xanh.

Vấn đề phụ: `Permissions()` build `PermissionInfo` field-by-field
(`centers/service.go:402-411`) và không mang `DefaultGrant`. Ma trận web vì thế
không phân biệt được "khoá opt-in, cố ý trống" với "khoá owner vừa bỏ tick".
Owner nhìn ô trống của `tasks.manage_board` giống hệt ô trống do mình tạo ra.

### Evidence

- `apps/api/internal/shared/authctx/catalog.go:34-46` — `PermDef` struct exported, plain fields
- `apps/api/internal/shared/authctx/catalog.go:130-145` — `def()`/`viewAll()`
- `apps/api/internal/shared/authctx/catalog_test.go:246` — count assertion sửa tay
- `apps/api/internal/features/centers/service.go:402-411` — `PermissionInfo` không có `DefaultGrant`
- Plan phase-01 bước 2 ("đặt `DefaultGrant: true` trong thân `def()`")

### Suggested fix

Đảo cực: `NoDefaultGrant bool` — zero value giữ nguyên hành vi hiện tại, khoá
opt-in phải khai báo tường minh. Nếu giữ `DefaultGrant`, thêm một test khẳng
định mọi entry của `permCatalog` có `DefaultGrant == true` trừ một allowlist
tường minh. Và cân nhắc đưa field vào `PermissionInfo` để ma trận hiển thị được
"opt-in" — nếu không, ghi rõ trong Phase 6 rằng owner không có tín hiệu nào.

---

# Verification Results

**Tier:** Full
**Claims checked:** 61
**Verified:** 57 | **Failed:** 1 | **Unverified:** 3

## Phase 1 — 20/20 checked

| # | Claim | Result |
|---|-------|--------|
| 1 | `apps/api/internal/shared/authctx/catalog.go` tồn tại | VERIFIED |
| 2 | `PermDef` tại `:34` | VERIFIED `catalog.go:34` |
| 3 | `DefaultRoleKeys()` tại `:330` | VERIFIED `catalog.go:330` |
| 4 | `CatalogVersion` tại `:294`, giá trị 3 | VERIFIED `catalog.go:294` |
| 5 | Khối admin quanh `PermReportsSend` tại `:232` | VERIFIED (thực tế `:231`, lệch 1 dòng) |
| 6 | `Order` gán từ vị trí tại `:250` | VERIFIED `catalog.go:250` |
| 7 | `len(DefaultRoleKeys()) == 53` | VERIFIED (chạy thật: 53; catalog 71 entry) |
| 8 | 53 → 59 khi thêm 6 khoá | VERIFIED (số học nhất quán) |
| 9 | `TestCatalogVersion` tại `:298` | VERIFIED `catalog_test.go:298` |
| 10 | `TestDefaultRoleKeysPreserveLegacyBaseline` tại `:246` | VERIFIED `catalog_test.go:246` |
| 11 | `TestScopeKeysCompleteAndHighRisk` tại `:123` | VERIFIED `catalog_test.go:123` |
| 12 | `TestEffectiveKeysCoversCatalogInOrder` tồn tại | VERIFIED `catalog_test.go:232` |
| 13 | helper `def()` / `viewAll()` tồn tại | VERIFIED `catalog.go:130`, `:139` |
| 14 | Mọi entry `permCatalog` đi qua helper | VERIFIED (71/71) |
| 15 | `legacyIdentitySet` tồn tại | VERIFIED `catalog.go:299` |
| 16 | `migrations/embed.go` tồn tại | VERIFIED |
| 17 | `domainTables` tại `migrations_test.go:26` | VERIFIED |
| 18 | `centerTables` tại `migrations_test.go:44` | VERIFIED |
| 19 | `TestBackfillSQLMatchesFrozenCatalogV2` tại `backfill_parity_test.go:110` | VERIFIED |
| 20 | Migration kế tiếp là 000022 (cuối cùng: 000021) | VERIFIED |

## Phase 2 — 5/5 checked

| # | Claim | Result |
|---|-------|--------|
| 21 | `apps/api/pkg/` chưa tồn tại | VERIFIED (`ls pkg` → No such file) |
| 22 | `github.com/google/uuid` có trong go.mod | VERIFIED `go.mod:12` (v1.6.0) |
| 23 | Port `UnitOfWork.Within` cùng shape `database.TxManager` | VERIFIED `internal/database/tx_manager.go:8-11` |
| 24 | Import path module là `teka/apps/api` | VERIFIED (`analyzer.go:20` dùng `teka/apps/api/internal/...`) |
| 25 | `make lint-api`, `go test ./tools/...` là gate có thật | VERIFIED (Makefile `:72` scopelint self-enforced) |

## Phase 3 — 21/21 checked

| # | Claim | Result |
|---|-------|--------|
| 26 | `routespec.Specs` tại `:130` | VERIFIED |
| 27 | helper `perm()` tại `:105` | VERIFIED |
| 28 | helper `none()` tại `:117` | VERIFIED |
| 29 | helper `req()` tại `:121` | VERIFIED |
| 30 | `AccountDisabler` tại `centers/service.go:24` | **FAILED** — thực tế `:25` (`:19-24` là comment) |
| 31 | `SetAccountDisabler` tại `:57` | VERIFIED |
| 32 | `RemoveMember` tại `:277` | VERIFIED |
| 33 | `CloseMembership` gọi tại `:299` | VERIFIED |
| 34 | `CreateCenter` tại `centers/repository.go:349` | VERIFIED |
| 35 | `container.go:100` là dòng `SetAccountDisabler` | VERIFIED |
| 36 | `registerFeatures` tại `router.go:108` | VERIFIED |
| 37 | `case centers.RolePermissionsChanged` tại `audit/subscriber.go:209` | VERIFIED |
| 38 | `centers/rbac_integration_test.go` tồn tại | VERIFIED |
| 39 | `server/router_test.go` tồn tại | VERIFIED |
| 40 | `sessions/repository.go` có `readScoped`/`writeScoped` | VERIFIED `:126`, `:168` |
| 41 | `database.FromContext` tồn tại | VERIFIED `internal/database/tx.go:35` |
| 42 | `Scope.CenterWideFor` tồn tại | VERIFIED `catalog.go:342` |
| 43 | `internal/testutil` tồn tại | VERIFIED |
| 44 | `API_AUDIT_BUFFER_SIZE` là env key thật | VERIFIED `config.go:171`, `:281` |
| 45 | Container là wiring dùng chung cho operator CLI | VERIFIED (`cli/create_center.go:59`, `cli/reset_password.go:54`, `cli/seed.go:29`) |
| 46 | "đặt ở router thì CLI mất bước bàn giao" | **UNVERIFIED** — không lệnh CLI nào gọi `RemoveMember`; lý lẽ chỉ đúng cho seed cột trong `CreateCenter`, không đúng cho handover |

## Phase 4 — 3/3 checked

| # | Claim | Result |
|---|-------|--------|
| 47 | precedent `no-restricted-imports` tại `eslint.config.js:45-57` | VERIFIED |
| 48 | Alias `@/*` → `src/*` dùng được cho pattern chặn | VERIFIED (block `:43-59` dùng `@/features/auth`, `@/lib/api/client`) |
| 49 | react-aria kanban example là nguồn verify a11y | **UNVERIFIED** — nguồn ngoài repo, không kiểm được ở đây; Phase 4 bước 1 đã đặt đúng làm điều kiện chặn |

## Phase 5 — 8/8 checked

| # | Claim | Result |
|---|-------|--------|
| 50 | `features/roster/hooks/roster-keys.ts` tồn tại | VERIFIED |
| 51 | `src/lib/api/envelope.ts` tồn tại | VERIFIED |
| 52 | `src/app/router.tsx` tồn tại | VERIFIED |
| 53 | `src/layouts/dashboard-layout.tsx` tồn tại | VERIFIED |
| 54 | `src/test/msw/handlers.ts` có `CATALOG_VERSION` tại `:17` (=3) | VERIFIED |
| 55 | `useCenterContext().has(key)` + `isResolved` tồn tại | VERIFIED `src/features/teaching/hooks/use-center-context.ts:13,19,32` (lưu ý: nằm ở `features/teaching`, không phải `features/center` — Phase 5 không ghi đường dẫn) |
| 56 | `src/features/center/api/center-api.ts` tồn tại | VERIFIED |
| 57 | `src/styles/tokens/colors.css` tồn tại | VERIFIED |

## Phase 6 — 9/9 checked

| # | Claim | Result |
|---|-------|--------|
| 58 | `RESOURCE_LABELS` tại `permission-schemas.ts:70-92` | VERIFIED `:70-91` |
| 59 | `groupCatalog` tại `:105` | VERIFIED |
| 60 | `buildCatalogTabs` tại `:150` | VERIFIED |
| 61 | `members` có nhãn "Thành viên" và nằm trong `ADMIN_RESOURCES` | VERIFIED `:84`, `:127-135` |
| 62 | `center-permissions.test.tsx`, `center-handlers.ts` tồn tại | VERIFIED |
| 63 | `docs/event-bus.md` §Event catalog `:35`, §Action map `:60`, §Known blind spots `:76` | VERIFIED (cả 3 khớp chính xác) |
| 64 | `docs/architecture.md` §Monorepo `:5`, §Applications `:12` | VERIFIED |
| 65 | `docs/adding-permissions.md` §2 `:44`, §4 `:108`, §9 `:211` | VERIFIED |
| 66 | `apps/web/e2e/roster.spec.ts` tồn tại làm khuôn | VERIFIED |

## Failures

1. **Claim #30 — Phase 3 §Related Code Files:** "`centers/service.go` — interface
   `TaskHandover` cạnh `AccountDisabler` (`:24`)". `type AccountDisabler interface`
   nằm ở `:25`; `:19-24` là doc comment. Lệch 1 dòng, không ảnh hưởng thực thi.

## Unverified

1. **Claim #46 — Phase 3 §Architecture:** "Container là nơi wiring dùng chung cho
   cả operator CLI — đặt ở router thì CLI mất bước bàn giao." Container đúng là
   dùng chung (`internal/cli/*.go` gọi `app.NewContainer`), nhưng ba lệnh CLI hiện
   có là `create_center`, `reset_password`, `seed` — **không lệnh nào gọi
   `RemoveMember`**. Kết luận wiring vẫn đúng (vì `CreateCenter` seed cột thì CLI
   có chạy), nhưng lý do được nêu không có bằng chứng. Nên sửa lý do trong plan để
   không dẫn dắt sai người đọc sau.
2. **Claim #49 — Phase 4 bước 1:** nguồn react-aria kanban example nằm ngoài repo.
3. **Ma trận tab order:** Phase 6 §Architecture khẳng định `buildCatalogTabs` giữ
   thứ tự catalog nên tab "Công việc" đứng trước "Quản trị". Hàm tồn tại
   (`:150`) và đọc `groupCatalog` (`:153`), nhưng thứ tự thực tế chỉ chứng minh
   được bằng cách chạy test — Phase 6 bước 2 đã đặt đúng đó là kiểm tra thủ công.

---

## Tóm tắt mức độ

| Severity | Số lượng | Finding |
|---|---|---|
| Critical | 3 | 1 (audit trước commit), 2 (nil-guard chặn offboarding), 3 (`move_to` cross-tenant) |
| High | 4 | 4 (audit không ghi được `move_to`), 5 (soft-delete khoá xoá cột), 6 (thiếu `WriteWide`), 7 (`members.list` backfill) |
| Medium | 1 | 8 (`DefaultGrant` zero-value + không lộ ra API) |

Ba finding Critical đều nằm trên đường bàn giao/xoá cột — tức là đúng hai chỗ
plan tự nhận là rủi ro cao nhất, nhưng mitigation được viết nhắm vào triệu chứng
sai. Finding 1, 3, 5 có chung một gốc: **lỗi không phải sentinel không có đường
xử lý xác định**, nên chúng nổi lên thành 500 và thành oracle.
