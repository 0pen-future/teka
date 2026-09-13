# Red-team (Assumption Destroyer / Scope Auditor) — Task Center Kanban

Plan: `plans/260913-1102-task-center-kanban/`
Ngày: 2026-09-13 · Vai: ASSUMPTION DESTROYER + SCOPE AUDITOR
Phạm vi: chỉ chất vấn giả định, không chất vấn scope (scope đã chốt ở rev3).

---

## Finding 1: scopelint chỉ nhận **một** repository receiver mỗi package — hai file repository của Phase 3 làm `task_repository.go` biến mất khỏi mọi rule

- **Severity:** Critical
- **Location:** Phase 3, "Related Code Files → Create" và "Non-functional" ("scopelint R1–R3 pass")
- **Flaw:** Plan tạo hai struct repository trong cùng package `tasks` (`column_repository.go`, `task_repository.go`). `findRepositoryReceiver` duyệt tên package-scope theo thứ tự sort và **return ngay ở match đầu tiên**; `collectRepoFiles` chỉ đưa vào file set những file khai báo receiver đó hoặc có method trên nó. `"columnRepository" < "taskRepository"` → toàn bộ `task_repository.go` nằm ngoài R1 và phần repository của R3.
  Tệ hơn: R1 chỉ kích hoạt cho hàm **nhận tham số `authctx.Scope`/`Anchor`/`OwnerAnchor`**. Port của `pkg/kanban` (D4/Phase 2 bước 3) nhận `tenant kanban.TenantID`, không phải `Scope` — nên ngay cả file được phân tích cũng không có hàm nào phải chứng minh witness. `make scopelint` sẽ xanh **một cách rỗng**.
- **Failure scenario:** Một method trên `taskRepository` quên `Where("tasks.center_id = ?", tenant)` (hoặc dùng `.Unscoped()`), CI vẫn xanh, `make scopelint` vẫn xanh, và trung tâm A đọc được việc của trung tâm B. Success criterion "`make scopelint` xanh" của Phase 3 không phải bằng chứng cho bất cứ điều gì. Tất cả 21 feature hiện tại đều có **đúng một** struct `*gorm.DB`, nên đây là feature đầu tiên phá giả định của linter.
- **Evidence:**
  - `apps/api/tools/scopelint/scopelint/analyzer.go:145-169` — `findRepositoryReceiver` `return tn` ở match đầu tiên; comment `:143-144`: "Package-scope names are visited in sorted order so the result is deterministic if more than one type happens to match."
  - `apps/api/tools/scopelint/scopelint/analyzer.go:174-206` — `collectRepoFiles` chỉ nhận file khai báo/có method trên `repoRecv`.
  - `apps/api/tools/scopelint/scopelint/analyzer.go:33-38` — R1 chỉ áp cho hàm "that accepts an `authctx.Scope`, `Anchor` or `OwnerAnchor` parameter".
  - `apps/api/tools/scopelint/scopelint/tree_test.go:22` — `minRepositoryPackages = 18` là **floor**, thêm package không đếm được sẽ không làm test đỏ.
  - Khảo sát 21 package `internal/features/*`: mỗi package đúng một struct chứa `*gorm.DB` (vd `sessions/repository.go:96`, `students/repository.go:70`, `classstaff/repository.go:67`).
  - Plan Phase 3: "`column_repository.go`, `task_repository.go` (implement port của core)" và "scopelint R1–R3 pass".
- **Suggested fix:** Gộp thành **một** struct `gormRepository` với `db *gorm.DB` trong `repository.go`, hai bộ method (`columnRepository`/`taskRepository` chỉ là interface view). Thêm vào mỗi method port một tham số `authctx.Scope` — hoặc, nếu muốn giữ port thuần, viết một lớp mỏng `internal/features/tasks/repository.go` nhận `Scope` và gọi xuống. Bổ sung một test khẳng định package `tasks` có đúng một repository receiver, nếu không R1 sẽ tự tắt lần sau.

---

## Finding 2: thêm `task_columns`/`tasks` vào `centerTables` làm test migration đỏ ngay — bảng không có cột `teacher_id`

- **Severity:** Critical
- **Location:** Phase 1, "Related Code Files → Modify" (`migrations_test.go` — "`centerTables` (`:44`) thêm cả hai") và Implementation Step 11
- **Flaw:** `centerTables` không phải danh sách "bảng có center_id". Nó là danh sách bảng có **cặp `(teacher_id, center_id)`** và vòng lặp kiểm chứng join thẳng vào `x.teacher_id`. `task_columns` không có cột người dùng nào; `tasks` có `created_by`/`assignee_id` chứ không có `teacher_id`.
- **Failure scenario:** `TestCenterBackfill` chạy `SELECT count(*) FROM task_columns x JOIN teachers tt ON tt.id = x.teacher_id` → Postgres trả `column x.teacher_id does not exist` → test panic/đỏ. Kể cả khi sửa tên cột, `require.Positivef(... "has no seeded rows")` vẫn đỏ cho `tasks` vì test đó seed dữ liệu tiền-000007 và không có task nào. Phase 1 gate `go test ./migrations/...` không bao giờ xanh.
- **Evidence:**
  - `apps/api/migrations/migrations_test.go:41-43` — comment: "every business table 000007 re-keyed to the center tenant. Each must carry center_id, and **every row's center_id must agree with the center of the teacher attributed on the same row**."
  - `apps/api/migrations/migrations_test.go:515-522`:
    ```go
    `SELECT count(*) FROM %s x JOIN teachers tt ON tt.id = x.teacher_id
     WHERE x.center_id IS DISTINCT FROM tt.center_id`
    ...
    require.Positivef(t, n, "table %s has no seeded rows — the backfill check proved nothing", tbl)
    ```
  - Brief §4 DDL (`reports/brainstorm-brief-rev3-extracted.md:119-157`): `task_columns` chỉ có `center_id`; `tasks` có `created_by`/`assignee_id`.
- **Suggested fix:** Chỉ thêm hai bảng vào `domainTables` (`:26`). Bỏ hoàn toàn khỏi `centerTables`. Nếu muốn giữ invariant tương đương, viết một assertion riêng cho `tasks` join qua `center_members(teacher_id, center_id)` và một assertion riêng cho `task_columns` chỉ kiểm `center_id` tồn tại — đừng tái dùng vòng lặp `teacher_id`.

---

## Finding 3: backfill chỉ chạm `center_role_permissions` — thành viên legacy không role mất quyền `tasks`, và test 000018 hiện có sẽ đỏ

- **Severity:** Critical
- **Location:** Phase 1, Implementation Step 9 ("backfill 6 khoá vào `center_role_permissions` cho 3 vai trò hệ thống") + Success Criteria ("không sửa test cũ ngoài 3 test đã liệt kê")
- **Flaw:** `DefaultRoleKeys()` không chỉ là baseline của role — 000018 đã dùng nó để backfill **member-level grants** cho thành viên sống không có role (`cm.role_id IS NULL`). Bump 53 → 59 mà chỉ backfill role sẽ tạo lệch giữa hàm Go và dữ liệu, và test hiện có so hai bên bằng chính hàm đó.
- **Failure scenario:** Hai hỏng cùng lúc.
  1. **Test:** `migrations_test.go:1718` lấy `defaults := authctx.DefaultRoleKeys()` (giờ = 59). `:1767` `require.Len(t, rolelessRows, len(defaults)+10)` kỳ vọng 69 nhưng thực tế 63 (53 default cũ + 10 view_all). Vòng `:1768-1770` `require.Truef(t, rolelessRows[key], ...)` đỏ cho cả 6 khoá mới. File này **không** nằm trong 3 test Phase 1 cho phép sửa.
  2. **Hành vi:** thành viên legacy không role đăng nhập vào prod sau migration → `tasks.list` không có trong `center_member_permissions` của họ, không có role để kế thừa → 403 ở `/tasks`, nav ẩn. AC2 ("Thành viên có `tasks.list` thấy việc mình tạo/được giao") sai cho đúng nhóm thành viên mà 000018 đã phải chăm sóc riêng.
- **Evidence:**
  - `apps/api/migrations/000018_resource_action_catalog_backfill.up.sql:174` — `WHERE cm.left_at IS NULL AND cm.role_id IS NULL`; `:112` `INSERT INTO center_member_permissions ...`
  - `apps/api/internal/shared/authctx/catalog.go:322-324` — "baseline every system role (**and, via member grants, every role-less legacy stint**) receives".
  - `apps/api/migrations/migrations_test.go:1718`, `:1765-1772`.
  - `apps/api/internal/shared/authctx/catalog_test.go:246-249` — `if len(defaults) != 53`.
  - Plan Phase 1 Success Criteria: "`go test ./internal/shared/authctx/...` xanh, **không sửa test cũ ngoài 3 test đã liệt kê**".
- **Suggested fix:** 000022 phải backfill **hai** bảng, đúng khuôn 000018: 6 khoá vào `center_role_permissions` cho 3 role hệ thống, **và** 6 khoá vào `center_member_permissions` cho `center_members` sống có `role_id IS NULL` của center chưa xoá. Đưa `migrations_test.go` (khối role-less, `:1765-1772`) vào danh sách file được sửa của Phase 1, và ghi rõ con số kỳ vọng mới.

---

## Finding 4: `SetTaskHandover` trong Container buộc đổi chữ ký `NewRouter` — ba call site không có trong plan, và "router_test.go không sửa" là bất khả thi

- **Severity:** High
- **Location:** Phase 3, Architecture "Consumer-defined interface + setter injection" + Step 14/15 + Success Criteria ("`router_test.go` xanh không sửa")
- **Flaw:** D8 đặt `tasksSvc` ở `app/container.go` (đúng lý do: operator CLI). Nhưng `registerFeatures` không nhận `*Container`; nó nhận một danh sách tham số vị trí, và `NewRouter` cũng vậy. Muốn mount route tasks từ một service đã dựng ở Container thì bắt buộc thêm tham số vào cả `NewRouter` và `registerFeatures`, kéo theo mọi call site.
- **Failure scenario:** Implementer làm theo Step 14 → build đỏ ở 3 chỗ không được liệt kê. Sửa `router_test.go` để compile → phá Success Criterion đã tuyên bố. Lối thoát "dựng tasksSvc lần thứ hai trong `registerFeatures`" tạo **hai instance service khác nhau**: instance của Container giữ hook handover, instance của router phục vụ HTTP — bàn giao vẫn chạy nhưng đó là hai `kanban.Service` với hai `Config`/`Clock` khác nhau, một dạng chia đôi nguồn sự thật rất khó thấy.
- **Evidence:**
  - `apps/api/internal/server/router.go:55` — `func NewRouter(cfg, log, db, zaloSvc, statementsSvc, notificationsSvc, teachersSvc, centersSvc, authSvc, bus)`.
  - `apps/api/internal/server/router.go:108` — `func registerFeatures(v1, cfg, log, db, zaloSvc, ..., bus)`.
  - Call sites: `apps/api/internal/app/app.go:35`, `apps/api/internal/server/router_test.go:69`, `apps/api/internal/server/policy_integration_test.go:72`.
  - `apps/api/internal/app/container.go:87,100` — nơi plan muốn chèn.
  - Plan Phase 3 "Modify" liệt kê `container.go` và `router.go`, **không** liệt kê `app.go`, `router_test.go`, `policy_integration_test.go`.
- **Suggested fix:** Thêm `tasksSvc *tasks.Service` vào `Container`, `NewRouter`, `registerFeatures`; liệt kê rõ 3 call site vào Phase 3 "Modify"; đổi Success Criterion thành "phần bidirectional route-coverage của `router_test.go` xanh không đổi assertion" thay vì "không sửa file". `router_test.go:69` dựng service với `db = nil`, nên `tasks.NewService(tasks.NewRepository(nil), ...)` là an toàn.

---

## Finding 5: `TestActionSnapshotUnchanged` là test hai chiều — 8 route ghi mới làm nó đỏ, file không có trong plan

- **Severity:** High
- **Location:** Phase 3, Step 11 + 18, và "Related Code Files → Modify" (chỉ liệt kê `audit/subscriber.go`)
- **Flaw:** Plan giả định manifest test chỉ đòi "mọi route ghi có audit source và action". Thực tế còn một snapshot **frozen list** trong `audit/action_test.go`: mọi entry `routespec.Specs` có `Audit.Action != ""` phải xuất hiện trong `actionSnapshot`, nếu không là lỗi.
- **Failure scenario:** Thêm 8 entry `req("task.create", ...)`, `req("task_column.reorder", ...)` … vào `Specs` → `TestActionSnapshotUnchanged` báo 8 lỗi dạng `"POST /api/v1/tasks: has Audit.Action "task.create" but is missing from the snapshot"`. `make test-api-unit` đỏ ở gate cuối Phase 3, và implementer sẽ tưởng mình làm sai routespec chứ không phải thiếu một file.
- **Evidence:**
  - `apps/api/internal/features/audit/action_test.go:123-146`:
    ```go
    for _, s := range routespec.Specs {
        if s.Audit.Action == "" { continue }
        id := s.Method + " " + s.Path
        if _, ok := want[id]; !ok {
            t.Errorf("%s: has Audit.Action %q but is missing from the snapshot", id, s.Audit.Action)
        }
    }
    ```
  - `apps/api/internal/features/audit/action_test.go:110-115` — `actionSnapshot` là literal cứng.
  - Plan Phase 3 Step 11 chỉ nói "11 entry" vào `routespec.go`.
- **Suggested fix:** Thêm `apps/api/internal/features/audit/action_test.go` (`actionSnapshot`) vào "Modify" của Phase 3 với 8 dòng mới, và đưa `go test ./internal/features/audit/...` lên ngay sau bước 11 thay vì chỉ ở gate cuối.

---

## Finding 6: hai nguồn sự thật cho board — reducer của lib và cache TanStack — plan không định nghĩa ai đồng bộ ai

- **Severity:** High
- **Location:** Phase 4, "Port + Command (DIP, ISP)" + Step 7; Phase 5, "Adapter (DIP)" + "Vì sao rollback khác refetch"
- **Flaw:** Phase 4 cho `useKanban` giữ state bằng `useReducer` (khởi tạo từ prop `board`) và tuyên bố "lib **không** biết optimistic update — rollback là việc của adapter". Phase 5 cho adapter làm optimistic + rollback + invalidate **trên `queryClient` cache**. Không phase nào nói ai dispatch `board/replaced` khi query refetch xong, cũng không nói reducer được rollback thế nào khi mutation lỗi. `useReducer` không tự cập nhật khi prop `board` đổi.
- **Failure scenario:** Ba kịch bản cụ thể, không cái nào bị test nào trong plan bắt được:
  1. User bấm `]` → adapter `setQueryData` optimistic → `onSettled` `invalidateQueries` → refetch trả board mới → prop `board` đổi nhưng reducer giữ state cũ → **board không bao giờ phản ánh dữ liệu server** cho tới khi unmount trang.
  2. Move lỗi mạng → adapter restore snapshot vào cache; reducer đã dispatch `tasks/moved` → thẻ ở lại cột sai, không có gì kéo nó về.
  3. Người khác xoá cột → 404 → adapter invalidate → cache đúng, reducer vẫn có `columnId` không tồn tại → `selectTasksByColumn` trả task mồ côi.
  Test được liệt kê ở Phase 5 (`use-tasks-data-source.test.tsx`) chỉ kiểm mutation + cache, không render `useKanban`, nên cả ba đi lọt.
- **Evidence:**
  - Plan Phase 4 Requirements: action `board/replaced` tồn tại nhưng không có consumer nào được chỉ định.
  - Plan Phase 4 Architecture: "lib gọi `dataSource.moveTask(...)` rồi dispatch action; lib **không** biết HTTP, không biết optimistic update — rollback là việc của adapter."
  - Plan Phase 5 Requirements: "optimistic update qua `queryClient.setQueryData`; `onError` … rollback snapshot; `onSettled` → `invalidateQueries`."
  - Precedent trong repo: các feature khác không giữ bản sao state cục bộ — key factory + query cache là nguồn duy nhất (`apps/web/src/features/roster/hooks/roster-keys.ts:12-50`).
- **Suggested fix:** Chọn **một** nguồn sự thật. Đơn giản nhất, đúng KISS và đúng precedent: bỏ `useReducer` khỏi `useKanban`; hook nhận `board` derived thuần từ query cache và chỉ giữ state UI phù du (`activeTaskId`, focus). Nếu giữ reducer, plan phải ghi rõ `useEffect(() => dispatch({type:'board/replaced', board}), [board])` cùng quy tắc "reducer không bao giờ mutate trước khi cache mutate", và thêm một test render `useKanban` + `QueryClient` thật cho cả ba kịch bản trên.

---

## Finding 7: `DefaultColumns()` bị nhân bản thành literal SQL trong 000022, không có parity test — chính điều mà Factory ở D5 hứa sẽ tránh

- **Severity:** Medium
- **Location:** Phase 1, Step 9 ("backfill cột mặc định CROSS JOIN VALUES 3 dòng"); Phase 2, `columns.go` `DefaultColumns()`; plan Pattern map ("Factory … Trung tâm mới và trung tâm backfill sinh cột giống hệt nhau")
- **Flaw:** Tên/`position`/`is_done` của 3 cột mặc định sẽ tồn tại ở **ba** nơi: `pkg/kanban/columns.go`, `000022_task_board.up.sql`, và (gián tiếp) `e2e/tasks-board.spec.ts`. Parity test mà Phase 1 mở rộng (`backfill_parity_test.go`) chỉ so **khoá quyền**, không so cột. Success criterion AC7 "parity test xanh" vì thế không chứng minh điều nó nghe như đang chứng minh.
- **Failure scenario:** Ai đó đổi `"Hoàn thành"` thành `"Đã xong"` trong `DefaultColumns()`. Center mới sinh cột `"Đã xong"`; center cũ vẫn `"Hoàn thành"`. Không test nào đỏ. E2E dùng suffix timestamp cho cột tự tạo (Phase 6 bước 4) nên cũng không chạm cột mặc định. Bug chỉ lộ khi người dùng hỏi vì sao hai trung tâm khác nhau.
  Phụ đề: `DefaultColumns()` nhúng chuỗi tiếng Việt vào một lib được tuyên bố là "trích xuất sang repo khác = `git mv`, không refactor" (D1). Consumer khác ngôn ngữ sẽ phải refactor.
- **Evidence:**
  - Plan Phase 2 Requirements: "`DefaultColumns()` factory: 3 cột 'Cần làm' (pos 0), 'Đang làm' (pos 1), 'Hoàn thành' (pos 2, `IsDone: true`)".
  - Brief §4 (`reports/brainstorm-brief-rev3-extracted.md:159`) — `INSERT INTO task_columns (center_id, name, position, is_done)` là literal SQL riêng.
  - `apps/api/migrations/backfill_parity_test.go:24-46,110-146` — parity chỉ phủ `frozenDefaultKeys`/`frozenScopeKeys`, cơ chế là `markedBlocks` + checksum trên khoá quyền.
  - `apps/api/internal/features/centers/repository.go:349-373` — precedent: `CreateCenter` gọi `authctx.DefaultRoleKeys()` **runtime**, không nhúng literal.
- **Suggested fix:** Đưa `DefaultColumns()` thành tham số hoá (`DefaultColumns(names [3]string)`) hoặc để adapter Teka sở hữu tên. Thêm vào `backfill_parity_test.go` một khối `-- default-columns` được đánh dấu trong 000022 và assert nó khớp `kanban.DefaultColumns()` — cùng khuôn `markedBlocks` đã có, chi phí gần bằng 0.

---

## Finding 8: audit row `task.*` / `task_column.*` không lọc được ở UI Nhật ký — `ACTION_GROUPS` là danh sách cứng, Phase 6 không đụng tới

- **Severity:** Medium
- **Location:** Phase 6, "Related Code Files → Modify" (chỉ có `docs/event-bus.md`, `docs/architecture.md`, `permission-schemas.ts`)
- **Flaw:** Plan đúng khi nói ma trận quyền tự sinh nhóm từ catalog, nhưng áp cùng giả định "tự sinh" cho trang Nhật ký. Bộ lọc hành động phía web là một mảng prefix hardcode, và chính `docs/event-bus.md` nói rõ điều đó.
- **Failure scenario:** AC9 ("Mọi thao tác ghi có dòng audit") đạt ở tầng dữ liệu, nhưng owner mở `/audit` và không có mục "Công việc" trong dropdown hành động. Tất cả row `task.create`, `task_column.delete`, `task.handover` rơi vào "Tất cả" và chỉ tìm được bằng free-text. Với `task.handover` — dòng audit duy nhất chứng minh bàn giao đã xảy ra (AC10) — đây là bằng chứng khó truy nhất lại bị chôn sâu nhất.
- **Evidence:**
  - `apps/web/src/features/audit/components/audit-filters.tsx:9-28` — `/** Action prefixes actually recorded by the API's action map. */` rồi 18 entry cứng (`auth.`, `class.`, …), không có `task.`.
  - `docs/event-bus.md` mục "Action map convention": "The web filter groups (`apps/web/src/features/audit/components/audit-filters.tsx`) key off these action prefixes."
  - Plan Phase 6 Requirements chỉ liệt kê `RESOURCE_LABELS` là "điểm phải sửa thật".
- **Suggested fix:** Thêm `{ value: "task.", label: "Công việc" }` (và cân nhắc `task_column.`, vì prefix `task.` không match `task_column.`) vào `ACTION_GROUPS`, cộng một dòng vào "Modify" của Phase 6 và một assertion trong test trang audit.

---

## Verification Results

**Tier:** Full
**Claims checked:** 42 · **Verified:** 34 · **Failed:** 7 · **Unverified:** 1

### Verified (mẫu)
`catalog.go:34/294/330`, chèn khối trước `PermReportsSend` (`catalog.go:231`), `Order` gán từ vị trí (`catalog.go:243-253`), count 53 (`catalog_test.go:248`), `catalog_test.go:123/246/298`, `legacyIdentitySet` (`catalog.go:300`), migration kế tiếp = 000022, `migrations_test.go:26/44`, `backfill_parity_test.go:110`, `centers/service.go:24/57/277/299`, `centers/repository.go:349`, `container.go:100`, `router.go:108`, `audit/subscriber.go:209`, `routespec.go:105/117/121/130`, scopelint chỉ chạy `./internal/...` (`Makefile:73`) nên `pkg/kanban` ngoài tầm — plan đúng, R3 token "read" chấp nhận `readScoped` (`analyzer.go:428-434`), `permission-schemas.ts:70/105/150` + `members` ∈ `ADMIN_RESOURCES` (`:127-135`), `handlers.ts:17` = 3, toàn bộ `HvSelect/HvSegmented/HvModal/HvConfirmDialog` tồn tại (`src/components/hv/`), `center_members` PK `(teacher_id, center_id)` (`000007:71`) đủ cho FK composite, owner luôn có stint (`000007:83-85` `fk_teachers_membership`), `docs/event-bus.md` §Event catalog/§Action map/§Known blind spots, `docs/architecture.md:5/12`, `docs/adding-permissions.md:44/108/211`, `TxManager.WithinTx` nhập tx sẵn có (`database/tx.go:23-26`) nên lo ngại nested-tx của Phase 3 là đúng, không có xung đột route gin cho `GET /centers/me/members/directory` (cây GET không có `:teacherId` dưới `/me/members`, `centers/routes.go:9-15`).

### Failed
| # | Claim của plan | Thực tế |
|---|---|---|
| F1 | Phase 3: "scopelint R1–R3 pass" với 2 file repository | `analyzer.go:145-169` chỉ nhận 1 receiver; `task_repository.go` ngoài mọi rule; R1 vacuous vì port nhận `kanban.TenantID` |
| F2 | Phase 1: "`centerTables` (`:44`) thêm cả hai" | `migrations_test.go:515-522` join `x.teacher_id` — không bảng nào có cột đó; `tasks` cũng 0 row |
| F3 | Phase 1 Step 9: backfill chỉ `center_role_permissions` | 000018 (`:174`) backfill member-level cho `role_id IS NULL`; `migrations_test.go:1718,1767-1772` sẽ đỏ và thành viên legacy mất quyền |
| F4 | Phase 3: "`router_test.go` — không sửa" | `NewRouter` (`router.go:55`) là tham số vị trí; đổi chữ ký chạm `app.go:35`, `router_test.go:69`, `policy_integration_test.go:72` — cả 3 vắng khỏi "Modify" |
| F5 | Phase 3: chỉ `audit/subscriber.go` cần sửa ở audit | `audit/action_test.go:123-146` là snapshot hai chiều; 8 route ghi mới làm nó đỏ |
| F6 | Phase 4/5: reducer của lib + cache TanStack cùng tồn tại an toàn | Không phase nào chỉ định ai dispatch `board/replaced`; `useReducer` không theo prop → board không cập nhật sau refetch |
| F7 | Plan Pattern map: Factory bảo đảm "center mới và center backfill sinh cột giống hệt nhau" | Không có parity test cho cột; `backfill_parity_test.go` chỉ phủ khoá quyền |

### Unverified
- Phase 4 Step 1: pattern ARIA của react-aria kanban example. Plan tự đánh dấu chưa verify và đặt việc đọc source làm bước 1 bắt buộc — đây là xử lý đúng, không phải khuyết điểm; chỉ nêu để đủ sổ.

### Ghi chú nhỏ (không tính là finding)
- Plan trỏ precedent `no-restricted-imports` ở `eslint.config.js:45-57`; block thực tế là `:42-61`. Không ảnh hưởng thực thi.
- `TestCatalogWellFormed` (`catalog_test.go:59-61`) bắt buộc `Description` không rỗng cho mọi khoá; Phase 1 Step 3 chỉ nhắc Kind/Risk/Label.

---

## Scope Audit — phân loại vòng đời state mới

| State mới | Vòng đời | Site | Kết luận |
|---|---|---|---|
| `PermDef.DefaultGrant` | process-global, immutable sau `init()` | `catalog.go:34` (field), `catalog.go:243-253` (`init`) | OK — `PermDefs()` trả copy (`catalog.go:259-263`), không rò |
| `kanban.Service` (Config, Clock, MaxColumns) | process-global singleton | `app/container.go` (dự kiến) | OK nếu immutable sau `NewService`; **phải** không giữ cache theo tenant |
| `centers.Service.taskHandover` (setter) | process-global, ghi một lần lúc wiring | `container.go:100` khu vực | OK về race (ghi trước khi server nhận request) — **nhưng** xem F4: nguy cơ dựng 2 instance |
| Repository `*gorm.DB` của tasks | process-global | `column_repository.go`/`task_repository.go` | **FAILED** — xem F1, không có rào chắn tenancy nào được kiểm |
| `kanban.Actor{Perms map[string]bool}` | per-request | `policy.go` `actorFrom(sc)` | OK nếu map dựng mới mỗi request; nếu tái dùng map từ `Scope.Perms` thì là con trỏ vào state chia sẻ — plan không nói rõ, nên ghi vào Phase 3 rằng map phải là bản dựng mới |
| `useKanban` reducer state | per-mount component | `src/lib/kanban/use-kanban.ts` | **FAILED** — xem F6, chia đôi nguồn sự thật với cache |
| `tasksKeys` query cache | per-session (QueryClient) | `hooks/tasks-keys.ts` | OK trong mô hình một-teacher-một-center hiện tại (`ResolveScope` trả đúng một center; `roster-keys.ts` cũng không nhúng `centerId`). Nếu sau này có chuyển center, key phải nhúng `centerId` |
| module-level state trong `src/lib/kanban` | không có | — | OK — plan không khai báo biến module nào; giữ nguyên |

