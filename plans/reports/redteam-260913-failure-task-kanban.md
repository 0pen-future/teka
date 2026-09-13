# Red-team — Failure Mode Analyst · task-center-kanban

Plan: `plans/260913-1102-task-center-kanban/`
Vai trò xác minh: FLOW TRACER
Ngày: 2026-09-13

---

## Finding 1: Event bàn giao publish TRƯỚC khi tx ngoài commit — audit ghi việc chưa xảy ra

- **Severity:** Critical
- **Location:** Phase 2 "Non-functional" + Implementation Step 10; Phase 3 "Architecture → Observer 2 nhánh" + Risk row "HandoverOnDeparture mở tx riêng"
- **Flaw:** Phase 2 tuyên bố bất biến "Publish event **sau khi** UoW commit thành công" và step 10 nói `HandoverOnDeparture` "publish `TasksHandedOver` sau commit". Bất biến này chỉ đúng khi `uow.Within` là tx ngoài cùng. Trong luồng `RemoveMember`, tx do `centers` mở, nên `WithinTx` lồng **trả thẳng `fn(ctx)` mà không commit**. `Within` trả `nil` khi closure xong, chứ không phải khi DB commit.
- **Failure scenario:** Owner gỡ thành viên. Trong tx: `HandoverOnDeparture` chạy `UnassignBy` + `ReassignCreator`, core thấy `Within` trả nil và publish `TasksHandedOver` → bus → audit subscriber ghi row `task.handover` với `{unassigned: 12, reassigned: 5}`. Ngay sau đó `s.disabler.Disable(ctx, targetID)` lỗi (deadlock, token revoke fail, DB timeout) → toàn bộ tx rollback. Kết quả: 0 việc được bàn giao, thành viên vẫn còn quyền, nhưng nhật ký hoạt động khẳng định đã bàn giao 17 việc. AC10 nói audit "ghi số việc bàn giao" — con số đó là bịa.
- **Evidence:**
  - `apps/api/internal/database/tx.go:23-30` — `if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok && tx != nil { return fn(ctx) }`. Nested join, không commit.
  - `apps/api/internal/features/centers/service.go:298-303` — `WithinTx` bọc `CloseMembership` rồi `s.disabler.Disable(ctx, targetID)`; `Disable` chạy SAU chỗ plan chèn handover.
  - Phase 2 dòng 54: "Publish event **sau khi** UoW commit thành công, đúng thứ tự gọi."
  - Phase 3 dòng 197 mitigation: "Facade **không** gọi `uow.Within`" — nhưng Phase 2 step 10 bắt core `HandoverOnDeparture` phải mở `uow.Within`. Hai phase mâu thuẫn: hoặc facade bỏ qua core (use-case `HandoverOnDeparture` của Phase 2 thành code chết), hoặc core mở tx lồng và publish sớm.
  - Khuôn đúng đã có trong repo: `apps/api/internal/features/centers/service.go:463-490` — `ReplaceRolePermissions` build `ev` **trong** closure, gọi `s.publish(ev)` **sau** khi `WithinTx` trả về ở ngoài cùng.
- **Suggested fix:** Core không được publish cho use-case chạy trong tx của consumer khác. Trả `(unassigned, reassigned int)` lên facade; `centers.RemoveMember` publish sau khi `WithinTx` ngoài cùng trả `nil`, đúng khuôn `ReplaceRolePermissions`. Hoặc thêm port `AfterCommit(fn)` vào `UnitOfWork` và cấm publish khi đang trong tx lồng.

---

## Finding 2: Thêm `task_columns`/`tasks` vào `centerTables` làm vỡ `TestCenterTenancyBackfill`

- **Severity:** Critical
- **Location:** Phase 1, "Related Code Files → Modify" và Implementation Step 11
- **Flaw:** Plan chỉ thị: "`migrations_test.go` — `domainTables` (`:26`) thêm `task_columns`, `tasks`; `centerTables` (`:44`) thêm **cả hai**". `centerTables` không phải danh sách "bảng có center_id" — nó là danh sách bảng mà backfill 000007 gán center theo **teacher trên cùng row**, và test join qua cột `teacher_id`.
- **Failure scenario:** `TestCenterTenancyBackfill` migrate xuống version 6, seed dữ liệu legacy, migrate up. Vòng lặp `centerTables` chạy `SELECT count(*) FROM task_columns x JOIN teachers tt ON tt.id = x.teacher_id ...` → Postgres error `column x.teacher_id does not exist` → test panic/fail. `task_columns` không có teacher; `tasks` có `created_by`/`assignee_id`, không có `teacher_id`. Ngay cả khi đổi tên cột, assert thứ hai `require.Positivef(t, n, "table %s has no seeded rows")` cũng fail vì fixture legacy không tạo task nào. `make test-api` đỏ ngay Phase 1, chặn cả 5 phase sau.
- **Evidence:**
  - `apps/api/migrations/migrations_test.go:41-43` — comment: "centerTables is every business table 000007 re-keyed... every row's center_id must agree with the center of the **teacher attributed on the same row**".
  - `apps/api/migrations/migrations_test.go:515-522` — `fmt.Sprintf("SELECT count(*) FROM %s x JOIN teachers tt ON tt.id = x.teacher_id WHERE x.center_id IS DISTINCT FROM tt.center_id", tbl)` + `require.Positivef(t, n, "table %s has no seeded rows — the backfill check proved nothing", tbl)`.
  - `apps/api/migrations/migrations_test.go:468-480` — test target `m.Migrate(6)`, seed `seedLegacyTenant`, rồi `MigrateUp`.
  - Brief §4 DDL: `task_columns(id, center_id, name, position, is_done, ...)` — không có `teacher_id`.
- **Suggested fix:** Chỉ thêm vào `domainTables` (dùng cho `TestMigrationRoundTrip`). Bỏ hoàn toàn `centerTables` khỏi Phase 1. Nếu muốn assert tenancy của 2 bảng mới, viết test riêng theo trục `center_id` + FK composite, không tái dùng assert theo `teacher_id` của 000007.

---

## Finding 3: Wiring `SetTaskHandover` như mô tả không lắp được, và lý do biện minh cho vị trí là sai

- **Severity:** High
- **Location:** Phase 3, "Architecture → Consumer-defined interface + setter injection", Implementation Steps 12/14/15, "Related Code Files → Modify" (dòng 117), Success Criteria dòng 173
- **Flaw:** Ba lỗi chồng nhau trong một quyết định.

  (a) **Lý do sai.** Plan viết: "Không đặt ở `registerFeatures` vì Container là nơi wiring dùng chung cho cả operator CLI — đặt ở router thì **CLI mất bước bàn giao**." `RemoveMember` chỉ có đúng một caller trong toàn repo: HTTP handler. CLI không có lệnh gỡ thành viên nào.

  (b) **Không lắp được như mô tả.** `Container` chỉ dựng teachers/centers/auth; mọi feature service dựng bên trong `registerFeatures`. Để `container.go` gọi `SetTaskHandover(tasksSvc)`, nó phải tự dựng `tasksSvc`, rồi luồn instance đó qua `NewRouter` → `registerFeatures`. Cả hai chữ ký phải đổi, cùng 3 call site.

  (c) **Success criterion tự mâu thuẫn.** Plan liệt kê `router_test.go` là "**không sửa**; phải xanh nhờ Specs" — nhưng `router_test.go:69` gọi `NewRouter(...)` với 10 tham số; đổi chữ ký thì file này bắt buộc phải sửa.

  (d) **Nil-guard làm vỡ test hiện có.** Step 12: "Nil-guard: nếu chưa set thì trả lỗi rõ ràng, không skip im lặng." Có 10 chỗ dựng `centers.NewService` ngoài container, không chỗ nào gọi setter mới, và ít nhất 4 file test gọi `RemoveMember` và assert `NoError`.
- **Failure scenario:** Implementer làm theo step 14/15 → `app` phải import `features/tasks`, `NewRouter` đổi chữ ký → `router_test.go:69` và `policy_integration_test.go:72` không compile → `make test-api` không chạy nổi. Nếu implementer né bằng cách dựng `tasksSvc` hai lần (một ở container cho handover, một ở registerFeatures cho routes), hệ thống chạy nhưng có hai instance khác nhau — mọi state/option trong tương lai lệch nhau âm thầm. Nếu bật nil-guard, `TestRemoveMemberByOwnerDataStaysBehind`, `TestRemoveMemberAuthorizationMatrix`, `TestSecretaryCannotRemoveMembers`, và test invitations/send_reports đều đỏ.
- **Evidence:**
  - Caller duy nhất: `apps/api/internal/features/centers/handler.go:124` — `h.svc.RemoveMember(...)`. Không có hit nào khác ngoài test và comment.
  - CLI không có lệnh nào liên quan: `apps/api/internal/cli/` chỉ có `create_center.go`, `migrate.go`, `reset_password.go`, `seed.go`, `serve.go`.
  - `apps/api/internal/server/router.go:55` và `:108` — `NewRouter` / `registerFeatures` cùng 10 tham số cố định; feature services dựng bên trong (`centers.RegisterRoutes`, `audit.NewService(...)` ở `:126-129`).
  - Call site `NewRouter`: `apps/api/internal/app/app.go:35`, `apps/api/internal/server/router_test.go:69`, `apps/api/internal/server/policy_integration_test.go:72`.
  - Test gọi `RemoveMember` và assert NoError: `centers/integration_test.go:250,316,355,356`; `centers/send_reports_integration_test.go:83`; `invitations/integration_test.go:269`; `centers/secretary_integration_test.go:32`.
  - 10 chỗ `centers.NewService(...)`: `container.go:87` + 9 file test (`cli/bootstrap_integration_test.go:52`, `server/router_test.go:63`, `server/policy_integration_test.go:64`, `auth/integration_test.go:74`, `audit/capture_integration_test.go:72`, `teachers/repository_test.go:47`, `teachers/integration_test.go:32`, `handoff/integration_test.go:43,224`).
- **Suggested fix:** Bỏ lý do "CLI" (sai sự thật). Chọn một trong hai: (i) đặt `SetTaskHandover` trong `registerFeatures` cạnh chỗ dựng `tasksSvc` — 0 đổi chữ ký, 0 đổi test hiện có; hoặc (ii) giữ ở container nhưng ghi rõ vào plan rằng `NewRouter`/`registerFeatures` đổi chữ ký và `router_test.go` + `policy_integration_test.go` **phải** sửa (xoá dòng "không sửa"). Nil-guard nên là no-op + log warn, hoặc mọi test dựng centers phải được liệt kê là file phải sửa.

---

## Finding 4: `audit/action_test.go` là snapshot bidirectional bắt buộc, không có trong scope Phase 3

- **Severity:** High
- **Location:** Phase 3, "Related Code Files → Modify" (thiếu file) + Success Criteria dòng 173 ("`router_test.go` xanh không sửa")
- **Flaw:** Plan chỉ nhận diện `router_test.go` là manifest test fail-closed. Có một test thứ hai, chặt hơn: mọi entry trong `routespec.Specs` có `Audit.Action` khác rỗng **phải** xuất hiện trong danh sách literal hardcode `actionSnapshot`. Plan thêm 8 route ghi dùng `req(action, entityType, idParam)` mà không đụng file này.
- **Failure scenario:** Implementer hoàn tất step 11 (11 entry `routespec.Specs`), chạy `make test-api`. `TestActionSnapshotUnchanged` báo 8 lỗi kiểu `POST /api/v1/tasks: has Audit.Action "task.create" but is missing from the snapshot`. Vì Phase 3 tuyên bố "router_test.go xanh không sửa" là tiêu chí thành công, implementer đi tìm sai file, hoặc tệ hơn: giảm 8 route xuống `none()` để test xanh, phá vỡ AC9 ("mọi thao tác ghi có dòng audit") một cách im lặng.
- **Evidence:**
  - `apps/api/internal/features/audit/action_test.go:136-143` — vòng lặp ngược: `for _, s := range routespec.Specs { if s.Audit.Action == "" { continue }; ... t.Errorf("%s: has Audit.Action %q but is missing from the snapshot", ...) }`.
  - `apps/api/internal/features/audit/action_test.go:110-115` — `actionSnapshot` là slice literal hardcode từng route.
  - Phase 3 dòng 100-117 "Modify" liệt kê `routespec.go`, `centers/*`, `container.go`, `router.go`, `audit/subscriber.go`, `centers/rbac_integration_test.go`, `server/router_test.go` — không có `audit/action_test.go`.
- **Suggested fix:** Thêm `apps/api/internal/features/audit/action_test.go` vào danh sách Modify của Phase 3 với 8 entry snapshot mới, và sửa Success Criteria thành "manifest tests (`router_test.go` + `audit/action_test.go`) xanh".

---

## Finding 5: `move_to` không bao giờ vào được dòng audit — middleware cố ý loại bỏ query string

- **Severity:** High
- **Location:** Phase 3, "Requirements → Functional" (audit qua request-events) + Phase 1 AC5; hợp đồng brief §3 "Xoá cột"
- **Flaw:** Hợp đồng chốt: `DELETE …/:id?move_to=`; "audit ghi **cả move_to**". Plan chọn cơ chế request-events (`req("task_column.delete", "task_column", "id")`). Middleware request-events publish `Path: c.Request.URL.Path` và **cố ý** không lấy `RequestURI`, kèm comment giải thích rằng query string có thể chứa dữ liệu nhạy cảm. `Params` chỉ chứa route params (`:id`), không chứa query params. Không có đường nào để `move_to` xuất hiện trong `audit_logs`.
- **Failure scenario:** Owner xoá cột "Chờ duyệt" đang chứa 30 việc, chọn dồn sang "Cần làm". Audit ghi `task_column.delete` với `entity_id = <id cột bị xoá>` và không gì khác. Ba tuần sau có tranh chấp "30 việc của tôi biến đi đâu" — cột nguồn đã bị `DROP`, `task_columns` không còn row, `tasks.column_id` chỉ trỏ cột đích hiện tại, và nhật ký không ghi cột đích. Không tái dựng được. Đây chính xác là loại mất dấu vết mà AC9 và brief muốn chặn.
- **Evidence:**
  - `apps/api/internal/middleware/request_events.go:114-116` — `// URL.Path, never RequestURI: query strings can carry phone` ... `Path: c.Request.URL.Path,`.
  - `apps/api/internal/middleware/request_events.go:31-34` — `Params map[string]string` = "route parameters by name" (chỉ `:id`).
  - `apps/api/internal/shared/routespec/routespec.go:119-122` — `req(action, entityType, idParam)` chỉ mang 3 trường, không có chỗ cho metadata động.
  - Brief §3, bảng "Quyết định con": "Xoá cột — `DELETE …/:id?move_to=`; bắt buộc khi cột có việc; 1 tx; **audit ghi cả move_to**".
  - Phase 3 dòng 36-38: audit `task_column.delete` "qua request-events".
- **Suggested fix:** `task_column.delete` phải đi đường service-level event (giống `tasks.HandedOver`), mang `{column_id, move_to, moved_count}` trong metadata, và entry `routespec` đổi sang `Audit{Source: SourceService}` để tránh hai dòng cho một request. Hoặc chấp nhận mất `move_to` và ghi rõ vào Non-goals + `docs/event-bus.md` "Known blind spots" — nhưng đó là đảo ngược một quyết định của hợp đồng, phải hỏi user.

---

## Finding 6: Optimistic rollback xoá mất move đang bay của lệnh khác — và khuôn tham chiếu không tồn tại

- **Severity:** High
- **Location:** Phase 5, "Requirements → Functional" (adapter `KanbanDataSource`), "Architecture → Vì sao rollback khác refetch", Implementation Steps 1 và 7
- **Flaw:** Mỗi mutation tự `onMutate` (cancel + snapshot + `setQueryData`) và `onError` rollback về **snapshot của chính nó**. Không có mutation scope, không serialize, không rollback-bằng-recompute. Với hai mutation chồng lấn, snapshot của lệnh sau đã chứa optimistic state của lệnh trước; rollback của lệnh trước xoá luôn phần của lệnh sau. Đây là chế độ tương tác **chính** của v1, không phải trường hợp hiếm: không có kéo-thả, người dùng chuyển việc bằng phím `[` / `]` liên tiếp.
- **Failure scenario:** Người dùng bấm `]` ba lần nhanh trên ba thẻ khác nhau (A→cột2, B→cột2, C→cột2). M_A snapshot S0, M_B snapshot S0+A, M_C snapshot S0+A+B. Mạng chập: M_A trả lỗi timeout (không phải 404/409, nên plan chọn nhánh **rollback**) → `setQueryData(S0)` → A, B, C đều nhảy về cột cũ trên UI, dù B và C đã commit thành công trên server. `onSettled` của M_B/M_C sau đó invalidate và kéo về sự thật — nhưng giữa hai thời điểm, board hiển thị sai; và nếu M_B/M_C settle **trước** M_A (thứ tự trả về không đảm bảo), rollback của M_A là hành động cuối và board đứng yên ở trạng thái sai cho tới lần invalidate kế tiếp. Rủi ro thứ hai: `board(params)` là key theo tham số; khi user đã từng bật "Toàn trung tâm", cache giữ 2 entry (do `keepPreviousData`), optimistic/rollback/invalidate chỉ chạm entry đang active — entry kia giữ trạng thái tiền-move và hiện ra ngay khi user gạt segmented control.
- **Evidence:**
  - Phase 5 dòng 196-199: "optimistic update qua `queryClient.setQueryData`; `onError` — 404/409 → `invalidateQueries`; lỗi khác → **rollback snapshot**".
  - Phase 5 step 7: "9 mutation; mỗi mutation có `onMutate` (cancel + snapshot + `setQueryData` optimistic)" — không nhắc `scope`, không nhắc serialize.
  - Phase 4 dòng 37: "`[` / `]` chuyển task sang cột kề" — move nhanh liên tiếp là UX chính (Non-goals: "Kéo-thả chuột").
  - Phase 5 risk row dòng 340: "chỉ invalidate key `board`, không invalidate `all`" — làm nặng thêm vấn đề nhiều entry board.
  - **Khuôn tham chiếu sai:** Phase 5 step 1 bảo đọc `features/roster/hooks/roster-keys.ts` và `use-students.ts` làm khuôn. Toàn repo web có đúng **một** file dùng `onMutate`: `apps/web/src/features/teaching/hooks/use-teaching-mutations.ts` — và nó **không** optimistic: nó ghi cache từ response server ở `onSuccess` (`:52-56`, `:64`), lỗi thì `invalidateQueries` + toast (`:27-39`), không snapshot, không rollback. Plan giới thiệu một pattern hoàn toàn mới cho codebase mà không nhận diện điều đó.
- **Suggested fix:** Cho cả 9 mutation cùng `scope: { id: "kanban-board" }` (TanStack v5 serialize theo scope) hoặc bỏ rollback-theo-snapshot, dùng luôn `invalidateQueries` cho mọi lỗi — đơn giản hơn, mất "cho user thử lại từ trạng thái cũ" nhưng không bao giờ xoá kết quả của lệnh khác. Ghi vào plan rằng đây là pattern mới, không có precedent trong repo, và `use-teaching-mutations.ts` là điểm so sánh gần nhất chứ không phải khuôn.

---

## Finding 7: Check-then-act không có backstop DB, và lỗi Postgres thô trả 500 thay vì 409/422

- **Severity:** High
- **Location:** Phase 2 Implementation Steps 6/8 và Success Criteria dòng 186-188; Phase 3 Implementation Step 6 (`translate()`)
- **Flaw:** Ba invariant của core đều là read-then-write không khoá, và `translate()` của adapter chỉ map 10 sentinel error của core. Không có nhánh nào cho `gorm.ErrDuplicatedKey` / pg `23505` / `23503`. Bất kỳ vi phạm ràng buộc nào thắng cuộc đua đều rơi xuống `apperror.Internal` → 500 + stack log, thay vì lỗi nghiệp vụ.
- **Failure scenario:** Ba kịch bản cụ thể.
  1. **Trần 8 cột bị vượt.** Hai admin cùng bấm "Thêm cột" khi đang có 7 cột. Cả hai `CountByTenant` → 7 < 8 → cả hai INSERT thành công. Trung tâm có 9 cột. Không có `CHECK`/trigger nào ở migration 000022 chặn. UI desktop scroll ngang vẫn chạy, nhưng trần 8 — lý do tồn tại của `WithMaxColumns` — đã vỡ vĩnh viễn và không có đường tự sửa (chỉ xoá thủ công).
  2. **Trùng tên → 500.** Hai người tạo cột "Chờ duyệt" đồng thời. `ExistsName` cả hai trả false; người thua nhận unique violation trên `(center_id, lower(name))`. `translate()` không nhận diện → 500. Dialog cấu hình cột của Phase 5 mong 409/422 để "hiện lỗi dưới ô" (step 13) — thay vào đó user thấy lỗi hệ thống.
  3. **Xoá cột rỗng đua với tạo việc → 500.** A xoá cột X (`CountInColumn` = 0 → không cần `move_to`). Trong khoảng giữa count và DELETE, B tạo việc vào X và commit. DELETE va FK `ON DELETE RESTRICT` → pg `23503` → 500. Dữ liệu an toàn (RESTRICT làm đúng việc của nó), nhưng A nhận 500 thay vì 409 "cột đã có việc, chọn cột đích", và không biết phải làm gì.
- **Evidence:**
  - Phase 2 step 6/8: `validateName`, `CountInColumn > 0`, `CountByTenant` — toàn bộ là read rồi write, không nhắc `SELECT ... FOR UPDATE` hay advisory lock.
  - Phase 3 step 6: `translate(err error) error` liệt kê đúng 10 sentinel, dừng ở đó.
  - Brief §3: "FK ON DELETE RESTRICT làm **lưới an toàn thứ hai**" — plan dùng lưới này nhưng không xử lý khi nó bung.
  - Khuôn đã có trong repo, plan không tham chiếu: `apps/api/internal/features/classstaff/repository.go:157-161` (`isUniqueViolation`, pg code `23505`), `apps/api/internal/features/centers/repository.go:872-883` (`translateError` switch theo `pgErr.ConstraintName`), và 12 chỗ khác dùng `errors.Is(err, gorm.ErrDuplicatedKey)` (`enrollments/repository.go:187`, `billing/repository.go:413`, `grading/service.go:129`, ...).
- **Suggested fix:** (a) Thêm `translateDBError` trong `tasks/errors.go` theo khuôn `centers/repository.go:872`: map constraint name của unique tên cột → `ErrDuplicateColumnName` (422), FK RESTRICT `23503` → `ErrColumnNotEmpty` (409). (b) Với trần 8 cột, hoặc lấy `SELECT count(*) ... FOR UPDATE` trên `centers` row trong cùng tx, hoặc thêm partial unique index / trigger ở 000022 — plan hiện không có backstop nào và test Phase 2 chỉ chứng minh invariant bằng fake in-memory tuần tự.

---

## Finding 8: Rollback Phase 1 = mất toàn bộ dữ liệu công việc, và để lại quyền mồ côi

- **Severity:** Medium
- **Location:** `plan.md` "Rollback" (Phase 1); Phase 1 Implementation Step 10 và Requirements "Down migration đảo ngược đúng"
- **Flaw:** Mục Rollback của plan mô tả từng phase như thể độc lập và có thể lùi riêng: "Phase 1: `make migrate-down` về 000021 (down script drop 2 bảng + xoá 6 khoá backfill)". Sau khi Phase 3-6 ship, hai bảng đó chứa dữ liệu production. Down script cũng chỉ xoá đúng 6 khoá đã backfill, bỏ lại `tasks.manage_board` và `tasks.view_all` mà owner đã gán tay.
- **Failure scenario:** Sản phẩm chạy 3 tuần, các trung tâm có vài nghìn việc. Một sự cố không liên quan khiến operator theo mục Rollback chạy `make migrate-down`. `DROP TABLE tasks` xoá sạch, không có bước export, không có soft-disable. Backup đề cập trong plan là "backup **trước khi apply**" — tức là ảnh chụp trước khi tính năng tồn tại, restore nó cũng không cứu được 3 tuần dữ liệu. Vấn đề phụ: sau khi lùi, `center_role_permissions` và `center_member_permissions` vẫn còn row `tasks.manage_board` / `tasks.view_all`; catalog v3 không biết hai khoá đó, `normalizeKeys` từ chối chúng như "unknown permission key" nếu owner cố lưu lại, và lần lưu ma trận kế tiếp âm thầm nuốt chúng.
- **Evidence:**
  - `plan.md:158-159` — "Phase 1: `make migrate-down` về 000021 (down script drop 2 bảng + xoá 6 khoá backfill), revert `CatalogVersion` về 3. **Backup DB trước khi apply.**"
  - Phase 1 step 10 — "`DELETE` đúng 6 khoá khỏi `center_role_permissions`, `DROP TABLE tasks`, `DROP TABLE task_columns`". Không đụng `center_member_permissions`, không đụng 2 khoá opt-in.
  - `apps/api/internal/features/centers/service.go` `normalizeKeys` — `if _, ok := authctx.PermDefOf(key); !ok { return apperror.Invalid(... "unknown permission key %q") }`; và `d.Grantable` check ngay sau.
  - `apps/api/migrations/migrations_test.go:133-137` — round-trip test chỉ chứng minh down script chạy sạch trên **DB rỗng**, không chứng minh gì về dữ liệu thật.
- **Suggested fix:** Tách Rollback thành hai chế độ trong plan: "trước khi ship" (drop bảng an toàn) và "sau khi ship" (KHÔNG drop — chỉ gỡ route + revert `CatalogVersion`; bảng để nguyên như Phase 3 rollback đã làm đúng). Down script nên xoá cả `tasks.manage_board`/`tasks.view_all` khỏi `center_role_permissions` **và** `center_member_permissions` để không để lại khoá ngoài catalog.

---

## Verification Results

**Tier:** Full
**Claims checked:** 27 · **Verified:** 20 · **Failed:** 7 · **Unverified:** 0

### Verified (trích)
| Claim | Traced path |
|---|---|
| `RemoveMember` :277, tx :298, `CloseMembership` :299 | `centers/service.go:277` → `:298 WithinTx` → `:299 CloseMembership` → `:302 disabler.Disable` |
| `AccountDisabler` :24, setter :57 | `centers/service.go:24` interface, `:57 SetAccountDisabler` |
| `container.go:100` là chỗ cross-wiring | `app/container.go:100 centersSvc.SetAccountDisabler(authSvc)` |
| `registerFeatures` tại `router.go:108` | `server/router.go:108` |
| `CreateCenter` tại `:349`, chạy trong tx | `cli/create_center.go:98 tx.WithinTx` → `:101 centerRepo.CreateCenter` → `:124 OpenMembership` |
| Owner có row `center_members` (FK `created_by` an toàn) | `cli/create_center.go:124 OpenMembership(accountID, centerID)`; `migrations/000007_centers.up.sql:77` |
| `TxManager.WithinTx` + `FromContext` shape khớp port `UnitOfWork` | `internal/database/tx.go:23`, `:35` (lưu ý: package là `internal/database`, không phải `internal/shared/database`) |
| Bus at-most-once, drop khi buffer đầy | `shared/events/async_bus.go:93-100` |
| `audit/subscriber.go` case `centers.RolePermissionsChanged` là khuôn hợp lệ | `audit/subscriber.go:208-225` |
| `catalog.go` `:34 PermDef`, `:294 CatalogVersion = 3`, `:329 DefaultRoleKeys`, khối admin `:231` | đọc trực tiếp |
| `catalog_test.go` `:123`, `:246` (count 53), `:298` | 53 + 5 CRUD tasks + `members.list` = 59 ✓ |
| `migrations_test.go:26 domainTables`, `:44 centerTables` | đọc trực tiếp |
| `backfill_parity_test.go:110 TestBackfillSQLMatchesFrozenCatalogV2` | đọc trực tiếp |
| scopelint R3 chấp nhận tên `readScoped` | `tools/scopelint/scopelint/analyzer.go:428` ("token equal to `read`, split on camelCase"), `:490` |
| `eslint.config.js` có precedent `no-restricted-imports` | `apps/web/eslint.config.js:38-62` |
| `permission-schemas.ts:70 RESOURCE_LABELS`, `:105 groupCatalog`, `:127 ADMIN_RESOURCES`, `:150 buildCatalogTabs` | đọc trực tiếp |
| `docs/event-bus.md:35 / :60 / :76` | đọc trực tiếp |
| Bump catalog v3→v4 không phá client web cũ | `permission-matrix.tsx:117` echo `data.catalog_version` từ API, không hardcode → CAS tự khớp |
| Deploy tách api/web/migrate | `docker-compose.yml:32 migrate`, `:50 api`, `:104 web` |
| `translateError` theo `pgErr.ConstraintName` là pattern có sẵn | `centers/repository.go:872-883` |

### FAILED
1. **"Publish event sau khi UoW commit thành công"** (Phase 2:54, step 10) — `internal/database/tx.go:24-26` cho thấy `WithinTx` lồng `return fn(ctx)` không commit; trong luồng `centers/service.go:298-303` event bắn trước commit và trước `disabler.Disable`. → Finding 1.
2. **"`centerTables` (`:44`) thêm cả hai"** (Phase 1 step 11) — `migrations_test.go:515-522` join `x.teacher_id`, hai bảng mới không có cột đó; `require.Positivef` cũng fail. → Finding 2.
3. **"Container là nơi wiring dùng chung cho cả operator CLI — đặt ở router thì CLI mất bước bàn giao"** (Phase 3 Architecture) — `RemoveMember` chỉ có caller `centers/handler.go:124`; `internal/cli/` không có lệnh gỡ thành viên. → Finding 3a.
4. **"`router_test.go` — không sửa; phải xanh nhờ Specs"** (Phase 3:117, Success Criteria) — `server/router_test.go:69` gọi `NewRouter` với chữ ký cố định; luồn `tasksSvc` từ container bắt buộc phải sửa file này và `policy_integration_test.go:72`. → Finding 3b/3c.
5. **Nil-guard TaskHandover không phá gì** (ngụ ý của Phase 3 step 12 + "make test-api xanh") — 4 file test gọi `RemoveMember` với `centers.NewService` không có setter: `centers/integration_test.go:250,316,355`, `centers/send_reports_integration_test.go:83`, `centers/secretary_integration_test.go:32`, `invitations/integration_test.go:269`. → Finding 3d.
6. **Audit `task_column.delete` ghi được `move_to` qua request-events** (Phase 3 Requirements, brief §3) — `middleware/request_events.go:114-116` cố ý dùng `URL.Path` chứ không phải `RequestURI`; `Params` chỉ có route params. → Finding 5.
7. **`roster-keys.ts` / `use-students.ts` là khuôn cho optimistic + rollback** (Phase 5 step 1) — toàn repo web chỉ có `teaching/use-teaching-mutations.ts` dùng `onMutate`, và nó ghi cache từ response ở `onSuccess` (`:52-56`, `:64`), không snapshot/rollback. Không có precedent optimistic. → Finding 6.

### Không kiểm được bằng đọc code (ghi nhận, không tính là finding)
- Hành vi thực tế của golang-migrate khi `000022` chứa DDL + backfill trong một file (transactional hay không) — cần chạy thật; ảnh hưởng tới việc backfill nửa chừng có để lại bảng rỗng hay không.
