
Brainstorm — Trung tâm công việc (Kanban, cột cấu hình) · Teka
Bỏ qua tới nội dung
    Brainstorm brief · rev 3
    Trung tâm công việc Kanban với cột cấu hình theo trung tâm
    Bảng công việc chung cho từng trung tâm dạy thêm. Chủ trung tâm (hoặc người được cấp quyền) tự đặt tên, sắp xếp, thêm/xoá cột; thành viên tạo, giao và chuyển việc giữa các cột. Phân quyền đi qua catalog + vai trò đã có. Rev 3: phương án B do người dùng chọn, 4 câu hỏi mở đã chốt — hợp đồng sẵn sàng cho /ak:plan.
    Ngày 13/09/2026Nhánh masterTrạng thái B · hợp đồng đã chốt
    1. Hợp đồng
    2. Bằng chứng repo
    3. Phương án
    4. Thiết kế B
    5. Luồng triển khai
    6. Mockup UI
    7. Rủi ro & câu hỏi
  1. Hợp đồng brainstorm
      Outcome — kết quả mong muốn
        Mục điều hướng mới “Công việc” mở bảng Kanban của trung tâm; mỗi trung tâm sinh ra với 3 cột mặc định Cần làm → Đang làm → Hoàn thành và có thể đổi tên, thêm, sắp xếp, xoá cột (tối đa 8).
        Một cột được đánh dấu “cột hoàn thành”; việc chuyển vào đó được ghi completed_at, chuyển ra thì xoá dấu.
        Thành viên có quyền tạo việc, giao cho thành viên khác, đặt hạn và ưu tiên, chuyển cột; chủ trung tâm thấy và sửa mọi việc.
        Toàn bộ quyền “Công việc” (gồm tasks.manage_board và tasks.view_all) xuất hiện trong ma trận Phân quyền vai trò; hai khoá này mặc định trống, owner bật cho vai trò hoặc override từng thành viên.
        Thành viên rời trung tâm không làm mất việc: việc họ được giao về “chưa giao”, việc họ tạo chuyển quyền sở hữu cho owner.
        Mọi thao tác ghi (việc lẫn cột) để lại dòng trong Nhật ký hoạt động.
      Constraints — ràng buộc
        Catalog quyền code-owned: khoá mới khai báo trong authctx/catalog.go, tăng CatalogVersion, DB chỉ lưu phép gán.
        Không nới ghi cho thành viên qua khoá: WriteWide() chỉ owner; *.view_all chỉ nới đọc, chỉ xuất hiện trong hàm repository có tên chứa read (scopelint).
        Cột là tài nguyên dùng chung của trung tâm, không có “own rows”: mọi ghi lên cột đi qua khoá tasks.manage_board + center_id; không dựa vào WriteWide.
        Mỗi route mới có đúng một entry routespec.Specs kèm phân loại audit; test manifest fail-closed.
        Frontend gate bằng useCenterContext().has(key); nhãn quyền chỉ từ API; page có deep-link guard.
        Không thêm thư viện DnD ở v1; a11y qua Radix + jsx-a11y; dark mode theo token map.
        Migration bất biến up/down, parity test; backup DB trước khi apply; seed cột mặc định cho trung tâm đã có và trung tâm tạo mới.
        Feature centers không import tasks: bàn giao việc khi rời trung tâm đi qua interface tiêm ở wiring (khuôn AccountDisabler), chạy trong cùng tx với CloseMembership.
      Non-goals — không làm ở v1
        Nhiều board / trung tâm, swimlane, WIP limit, màu hay icon cho cột, quy tắc chuyển cột (workflow rules).
        Bình luận, đính kèm, checklist con, nhắc việc qua Zalo/thông báo.
        Liên kết việc với lớp/học sinh/buổi (giữ chỗ cho class_id sau, không xây UI).
        Kéo-thả chuột; v1 chuyển cột và sắp xếp cột bằng nút có phím tắt.
        Compare-and-set cho cấu hình cột; xung đột hai người sửa cột cùng lúc giải quyết bằng refetch.
        Báo cáo/thống kê công việc, escalation quá hạn.
      Acceptance criteria — bằng chứng hoàn thành
        AC1 Owner không cần gán gì vẫn CRUD việc, cấu hình cột, xem toàn bộ board.
        AC2 Thành viên có tasks.list thấy việc mình tạo/được giao trên đúng cột; thêm tasks.view_all thấy toàn trung tâm; trung tâm khác không thấy gì.
        AC3 Người được giao chuyển được cột việc của mình dù không phải người tạo; sửa nội dung/xoá chỉ người tạo hoặc owner, còn lại 403.
        AC4 Thiếu tasks.manage_board → mọi API cột trả 403 và nút “Cấu hình cột” ẩn; có khoá → tạo/đổi tên/sắp xếp/xoá cột thành công.
        AC5 Xoá cột đang có việc bắt buộc kèm move_to; API thực hiện di dời + xoá trong một transaction; thiếu move_to → 409. Không thể xoá cột cuối cùng.
        AC6 Chuyển việc vào cột is_done ghi completed_at; chuyển ra xoá; đổi cờ is_done của cột không ghi lại lịch sử việc cũ.
        AC7 Trung tâm mới tạo có sẵn 3 cột mặc định; migration backfill cho mọi trung tâm đang sống; parity test xanh.
        AC8 Ma trận Phân quyền hiển thị nhóm “Công việc” (6 khoá) + “Thành viên” (members.list) với nhãn từ API; CRUD backfill cho 3 vai trò.
        AC10 Xoá thành viên: trong cùng tx, việc họ được giao có assignee_id = NULL, việc họ tạo có created_by = owner; audit task.handover ghi số việc bàn giao; mời lại thành viên đó không hoàn tác.
        AC9 Audit ghi task.* và task_column.*; manifest tests xanh; make test-api, make test-web, make lint, make scopelint xanh; e2e 1 luồng: thêm cột “Chờ duyệt” → tạo việc → giao → chuyển qua cột mới → hoàn thành.
  2. Bằng chứng từ repo (đã kiểm tra)
      RBAC ba lớp đã có: center_roles (3 vai trò hệ thống seed mỗi trung tâm), center_role_permissions, center_member_permissions grant/deny. Owner đứng ngoài hệ vai trò (migration 000013).
      Seed theo trung tâm sống ở centers/repository.go → CreateCenter: INSERT 3 vai trò rồi gán DefaultRoleKeys(). Cột mặc định của board seed cùng chỗ để trung tâm mới và trung tâm backfill hành xử giống nhau.
      Catalog resource.action: DefaultRoleKeys() trả mọi khoá grantable không phải scope và không thuộc legacy → khoá CRUD mới tự vào backfill; khoá special cũng vào trừ khi bị loại tường minh. Vì vậy tasks.manage_board cần quyết định rõ (câu hỏi #1).
      CAS đã có cho vai trò (assignment_version, ErrStaleVersion); v1 board không dùng CAS, chỉ refetch khi 404/409 — ghi vào non-goals để không bị hiểu là sót.
      Khuôn repository: sessions/repository.go tách readScoped / writeScoped; tasks theo khuôn với “own” = người tạo hoặc người được giao.
      Rời trung tâm là soft: RemoveMember chạy tx CloseMembership (stamp left_at) + disabler.Disable; không hard-delete nên FK CASCADE của tasks không kích hoạt. Owner có stint riêng trong center_members (role_id NULL) → created_by trỏ owner thoả FK composite. Bước bàn giao việc nối vào tx này qua interface tiêm như AccountDisabler.
      Lỗ hổng: danh sách thành viên chỉ trả cho owner qua GET /centers/me; thành viên thường không có endpoint chọn người nhận → cần directory tối giản.
      Điều hướng: dashboard-layout.tsx lọc entry theo perm; nhóm đầu chỉ có “Tổng quan”.
      Design system: HvCard/HvModal/HvSegmented/HvSelect/HvConfirmDialog/StatusPill sẵn dùng; không có DnD lib.
      PRD non-goal về cấu hình vẫn đúng; phương án B là quyết định của người dùng, brief giảm rủi ro bằng seed mặc định + trần 8 cột + không có rule theo cột.
      Migration kế tiếp: 000022.
  3. Phương án đã so sánh và lựa chọn
  Rev 1 đề xuất A; người dùng chọn B. Giữ lại bảng so sánh để plan ghi nhận đánh đổi đã chấp nhận.
      A. Cột cố định (enum)
      3 trạng thái cứng, không bảng cột. Rẻ nhất, hợp PRD, nhưng không cho trung tâm tự đặt quy trình.
      Vì sao không chọnNgười dùng muốn trung tâm tự cấu hình cột; A phải làm lại khi có yêu cầu đó.
      Đã chọn
      B. Cột cấu hình theo trung tâm
      Bảng task_columns, tasks trỏ column_id, khoá tasks.manage_board cho CRUD cột, seed 3 cột mặc định.
        Giả định chịu tảiTrung tâm cần quy trình riêng (thêm “Chờ duyệt”, “Đã giao PH”…).
        Điểm gãy đầu tiênXoá cột đang có việc. Giải bằng move_to bắt buộc trong một tx, và không cho xoá cột cuối.
        Trường hợp xấuHai người cấu hình cột đồng thời; client board cũ trỏ cột đã xoá → 404 → refetch. Chấp nhận ở v1, có thể thêm CAS sau theo khuôn assignment_version.
        Chi phí bỏTrung bình: 2 bảng, 8 khoá, 2 nhóm endpoint. Giảm bằng cách giữ contract cột tối giản (name, position, is_done).
      C. To-do cá nhân
      Không giao việc, không board chung.
      Vì sao không chọnCắt phạm vi “quản lý công việc trung tâm”.
  Quyết định con của phương án B
      Quyết địnhChọnVì sao / đánh đổi
        Ngữ nghĩa “hoàn thành” (đã chốt)Cờ is_done trên cột (0..n cột)Board không cần enum trạng thái; “xong” là thuộc tính cột. Cho phép nhiều cột done (vd “Hoàn thành”, “Huỷ”) mà không thêm khái niệm. Đổi cờ không viết lại completed_at việc cũ.
        Giới hạn cột (đã chốt)1 ≤ số cột ≤ 8; tên ≤ 40 ký tự, duy nhất trong trung tâm (không phân biệt hoa thường)Trần 8 giữ board đọc được trên desktop (scroll ngang) và mobile (dropdown). Unique index (center_id, lower(name)) chặn trùng ở DB, không chỉ ở UI.
        Xoá cộtDELETE …/:id?move_to=; bắt buộc khi cột có việc; 1 tx; audit ghi cả move_toMột request nguyên tử thay vì “bulk move rồi delete” hai bước (bước 2 có thể fail để lại board lệch). FK ON DELETE RESTRICT làm lưới an toàn thứ hai.
        Sắp xếp cộtPUT /task-columns/order nhận toàn bộ danh sách idValidate là hoán vị đúng của tập cột hiện có → không có trạng thái nửa chừng. Position INT gán lại 0..n-1.
        Quyền cấu hình cột (đã chốt)tasks.manage_board — special / high, opt-in (loại khỏi backfill)Cột là tài nguyên chung: một người xoá cột ảnh hưởng mọi người. Owner gán qua ma trận cho vai trò hoặc override thành viên.
        Own rows của tasksngười tạo OR người được giaoNgười nhận chuyển cột việc của mình không cần nới WriteWide. Sửa nội dung/xoá: người tạo hoặc owner. Tách rule qua 2 endpoint (PATCH vs POST …/move).
        Chọn người nhậnKhoá members.list + directory (id, tên, vai trò)members.manage quá mạnh; directory không trả SĐT/email nên không đụng phone-privacy.
        tasks.view_allOpt-in, gán qua ma trận (đã chốt)Scope key đã hiển thị trong ma trận như mọi *.view_all hiện có; owner bật cho vai trò hoặc override. Không cần UI mới.
        Thành viên rời trung tâm (đã chốt)Giữ việc; assignee_id → NULL, created_by → owner trong tx RemoveMembertasks.Service.HandoverOnDeparture(ctx, centerID, teacherID, ownerID) tiêm vào centers.Service qua interface TaskHandover; chạy trước CloseMembership trong cùng tx. Việc mất người nhận hiện “chưa giao” cho owner xử lý lại.
        Thứ tự việc trong cộtposition DOUBLE PRECISION, sort (column_id, position, created_at)Chèn giữa không renumber; sẵn cho DnD sau. v1 “đưa lên đầu cột” khi chuyển.
        Xoá việcSoft delete deleted_atCùng khuôn classes/students; giữ dấu vết audit.
  4. Thiết kế đề xuất (phương án B)
  Mô hình dữ liệu
    centers
id UUID PK
owner_id
…
    task_columns
id UUID PK
center_id → centers
name VARCHAR(40)
position INT
is_done BOOL
created_at / updated_at
UNIQUE (id, center_id)
UNIQUE (center_id, lower(name))
    tasks
id UUID PK
center_id
column_id → task_columns (RESTRICT)
created_by → center_members
assignee_id → center_members (NULL)
title, description, priority
due_on, position, completed_at
created_at / updated_at / deleted_at
    center_members
(teacher_id, center_id)
role_id → center_roles
left_at
      Migration 000022 task_board
CREATE TABLE task_columns (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  center_id  UUID NOT NULL REFERENCES centers(id) ON DELETE CASCADE,
  name       VARCHAR(40) NOT NULL,
  position   INT NOT NULL,
  is_done    BOOLEAN NOT NULL DEFAULT FALSE,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE (id, center_id)                       -- đích cho FK composite
);
CREATE UNIQUE INDEX uq_task_columns_name
  ON task_columns (center_id, lower(name));
CREATE INDEX idx_task_columns_order ON task_columns (center_id, position);
CREATE TABLE tasks (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  center_id    UUID NOT NULL,
  column_id    UUID NOT NULL,
  created_by   UUID NOT NULL,
  assignee_id  UUID,
  title        VARCHAR(200) NOT NULL,
  description  TEXT NOT NULL DEFAULT '',
  priority     VARCHAR(8) NOT NULL DEFAULT 'normal', -- validate trong code
  due_on       DATE,
  position     DOUBLE PRECISION NOT NULL DEFAULT 0,
  completed_at TIMESTAMPTZ,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  deleted_at   TIMESTAMPTZ,
  FOREIGN KEY (column_id, center_id)
    REFERENCES task_columns (id, center_id) ON DELETE RESTRICT,
  FOREIGN KEY (created_by, center_id)
    REFERENCES center_members (teacher_id, center_id) ON DELETE CASCADE,
  FOREIGN KEY (assignee_id, center_id)
    REFERENCES center_members (teacher_id, center_id) ON DELETE CASCADE
);
CREATE INDEX idx_tasks_board ON tasks (center_id, column_id, position)
  WHERE deleted_at IS NULL;
CREATE INDEX idx_tasks_assignee ON tasks (center_id, assignee_id);
CREATE INDEX idx_tasks_creator  ON tasks (center_id, created_by);
-- Backfill: mỗi trung tâm đang sống nhận 3 cột mặc định.
INSERT INTO task_columns (center_id, name, position, is_done)
SELECT c.id, v.name, v.pos, v.done
FROM centers c CROSS JOIN (VALUES
  ('Cần làm', 0, FALSE), ('Đang làm', 1, FALSE), ('Hoàn thành', 2, TRUE)
) AS v(name, pos, done)
WHERE c.deleted_at IS NULL;
-- Backfill quyền cho 3 vai trò hệ thống (ON CONFLICT DO NOTHING):
--   tasks.create / list / read / edit / delete, members.list
-- KHÔNG backfill: tasks.manage_board, tasks.view_all (opt-in).
      FK composite kèm center_id theo khuôn 000007/000015. RESTRICT trên column_id buộc service di dời việc trước khi xoá cột. CreateCenter trong centers/repository.go thêm INSERT 3 cột mặc định ngay sau INSERT 3 vai trò.
      Khoá catalog mới (CatalogVersion +1)
        KhoáKind / RiskNhãnBackfill
          tasks.createcrud / lowTạo công việc3 vai trò
          tasks.listcrud / lowXem bảng công việc3 vai trò
          tasks.readcrud / lowXem chi tiết công việc3 vai trò
          tasks.editcrud / lowSửa & chuyển cột công việc3 vai trò
          tasks.deletecrud / mediumXoá công việc3 vai trò
          tasks.manage_boardspecial / highCấu hình cột bảng công việcopt-in
          tasks.view_allscope / highXem mọi công việc trong trung tâmopt-in
          members.listcrud / lowXem danh bạ thành viên3 vai trò
      Lưu ý catalog: DefaultRoleKeys() hiện gom mọi khoá grantable không phải scope, kể cả special. Để tasks.manage_board opt-in cần loại nó tường minh (như legacyIdentitySet) hoặc thêm thuộc tính DefaultGrant=false trên PermDef. Đề xuất thuộc tính, vì các khoá special sau này cũng cần.
      Quy tắc scope trong repository
        Cột — readColumns: center_id = ? (mọi người có tasks.list). writeColumns: center_id = ?; quyền do middleware (tasks.manage_board) quyết, không có nhánh own-rows.
        Việc — readScoped: center_id ∧ (view_all ∨ created_by = me ∨ assignee_id = me).
        writeOwn (PATCH/DELETE): center_id ∧ (owner ∨ created_by = me).
        writeParticipant (POST …/move): center_id ∧ (owner ∨ created_by = me ∨ assignee_id = me).
        Service kiểm tra column_id và assignee_id thuộc cùng center_id của scope; không nhận center_id/teacher_id từ request.
    Endpoint & phân loại routespec
      Method / PathKhoáAuditGhi chú
        Board & cột
        GET /api/v1/tasks/board?scope=mine|center&assignee_id=tasks.listnoneMột response: columns[] theo position + tasks nhóm theo column_id, mỗi cột ≤50 + cursor. scope=center chỉ hiệu lực khi có view_all.
        POST /api/v1/task-columnstasks.manage_boardtask_column.create / task_column{name, is_done?}; append cuối; 409 khi trùng tên hoặc đã 8 cột.
        PATCH /api/v1/task-columns/:idtasks.manage_boardtask_column.update / task_column / id{name?, is_done?}. Không đụng việc đang nằm trong cột.
        PUT /api/v1/task-columns/ordertasks.manage_boardtask_column.reorder / task_column{ids: [...]} phải là hoán vị đủ của tập cột; 409 nếu lệch (client cũ) → refetch.
        DELETE /api/v1/task-columns/:id?move_to=tasks.manage_boardtask_column.delete / task_column / id1 tx: UPDATE tasks SET column_id = move_to (giữ position, cập nhật completed_at theo cờ cột đích) → DELETE cột. 409 nếu có việc mà thiếu move_to, nếu move_to = chính nó, hoặc là cột cuối cùng.
        Việc
        POST /api/v1/taskstasks.createtask.create / task{title, description?, column_id?, assignee_id?, priority?, due_on?}; thiếu column_id → cột position 0.
        GET /api/v1/tasks/:idtasks.readnonereadScoped → 404 ngoài scope.
        PATCH /api/v1/tasks/:idtasks.edittask.update / task / idtitle, description, assignee, priority, due_on. writeOwn.
        POST /api/v1/tasks/:id/movetasks.edittask.move / task / id{column_id, position?}; set/clear completed_at theo is_done cột đích. writeParticipant.
        DELETE /api/v1/tasks/:idtasks.deletetask.delete / task / idSoft delete. writeOwn.
        Thành viên
        DELETE /api/v1/centers/me/members/:teacherId (đã có)members.manage+ task.handover / taskKhông đổi contract; tx thêm bước bàn giao việc trước CloseMembership. Payload audit: số việc đổi assignee, số việc đổi creator.
        GET /api/v1/centers/me/members/directorymembers.listnone[{teacher_id, display_name, role_name}] — không SĐT/email; chỉ stint đang sống.
      Backend: internal/features/tasks/
        model.go (TaskColumn, Task), dto.go, validation.go (tên cột, trần 8, priority, due_on)
        column_repository.go (readColumns / writeColumns, CountTasksInColumn, MoveAllTasks) và task_repository.go (readScoped / writeOwn / writeParticipant)
        column_service.go (create/rename/reorder/delete-with-move trong TxManager) và task_service.go (validate cột & assignee cùng center, completed_at theo cột)
        handler.go, routes.go; wiring trong server.registerFeatures
        centers/repository.go → CreateCenter: INSERT 3 cột mặc định; directory handler trong features/centers
        centers/service.go: interface TaskHandover tiêm cạnh AccountDisabler; RemoveMember gọi trước CloseMembership. Tasks feature cung cấp HandoverOnDeparture (2 UPDATE trong ctx tx).
        authctx: 8 khoá, thuộc tính DefaultGrant, CatalogVersion++
        Tests: allow/deny/owner/override/cross-center cho việc và cột; delete-with-move nguyên tử; reorder hoán vị sai → 409; seed khi tạo trung tâm; xoá thành viên → bàn giao việc trong cùng tx (rollback khi Disable lỗi)
      Frontend: src/features/tasks/
        api/tasks-api.ts, api/task-columns-api.ts, schemas/*.ts (zod), hooks/use-task-board.ts, hooks/use-task-columns.ts
        pages/task-board-page.tsx (guard has("tasks.list")); board scroll ngang khi >3 cột; mobile: HvSegmented ≤4 cột, HvSelect khi nhiều hơn
        components/task-board.tsx, task-column.tsx, task-card.tsx, task-form-modal.tsx, move-menu.tsx (danh sách cột động)
        components/board-settings-modal.tsx (HvModal; gate has("tasks.manage_board")): đổi tên inline, nút ↑/↓, toggle “cột hoàn thành”, xoá qua HvConfirmDialog + HvSelect “chuyển việc sang”
        features/center/api: fetchMemberDirectory; MSW fixture + mirror CatalogVersion
        Nav: entry “Công việc” /tasks, perm: "tasks.list", nhóm đầu cạnh “Tổng quan”
        Tests: quartet cho board; settings modal ẩn/hiện theo has(); xoá cột có việc bắt buộc chọn đích; 409 reorder → refetch; e2e Playwright
  5. Luồng triển khai
      1API: catalog + migration
      8 khoá, thuộc tính DefaultGrant, CatalogVersion++
000022: 2 bảng + seed cột + backfill quyền
CreateCenter seed cột
Parity tests
      Gate: go test ./internal/shared/authctx/... ./migrations/... ./internal/features/centers/...
      2API: cột + việc + directory
      column/task repo, service, handler
routespec + audit actions
delete-with-move tx, reorder hoán vị
TaskHandover vào RemoveMember
RBAC matrix tests
      Gate: make test-api, make scopelint, make api-docs
      3Web: board + cấu hình cột
      api/schemas/hooks
Board N cột, card, move-menu, form
Board settings modal
Nav + guard + MSW
      Gate: make test-web, make lint
      4Ma trận quyền + e2e
      Nhóm “Công việc” từ catalog
e2e: thêm cột → tạo → giao → chuyển → xong → xoá cột có việc
Dark mode / mobile
      Gate: make e2e (stack teka-e2e)
      5Review, docs, ship
      Review RBAC + tx xoá cột
docs: adding-permissions (DefaultGrant), frontend nav
Deploy API → gán manage_board, view_all
      Gate: CI xanh, PR merge, verify allowed + denied trên prod
  Đường đi request “xoá cột đang có việc”
      Luồng xoá cột: modal cấu hình chọn cột đích, API di dời việc và xoá cột trong một transaction, phát sự kiện audit
        BoardSettingsModalchọn “chuyển sang”
        DELETE /task-columns/:id?move_to=… · manage_board
        Scope middlewareHas(manage_board)?
        column_service.Delete — 1 transaction
        UPDATE taskscolumn_id = move_to
        DELETE columnRESTRICT đã thoả
        200 boardrefetch
        request-events → audit_logs (task_column.delete, move_to)
        403 thiếu khoá
        409: thiếu move_to · cột cuối · move_to = chính nó
  6. Mockup UI có chú thích
  Kế thừa token: nền cream-100, thẻ trắng viền line-200, radius 14–20px, nút chính mint-400 + press shadow, chữ Baloo 2 / Nunito. Cột hoàn thành nền mint-50. Dark mode dùng token map chung.
    Board — desktop (4 cột)
    Cấu hình cột
    Xoá cột có việc
    Board — mobile
    Form công việc
    Ma trận quyền
        Công việc
14 việc đang mở · 3 quá hạn
          Của tôiToàn trung tâm1
          Người nhận: Tất cả ▾
          ⚙ Cấu hình cột2
          ＋ Thêm công việc
          Cần làm5
          3
            Gọi phụ huynh lớp Toán 9A chưa đóng T9
            Cao · hạn 15/09HV
            Chuyển sang ▾
          In phiếu thu tháng 9
Vừa · quá hạn 2 ngàyTG
          Cập nhật lịch lớp Lý 10
Thấp—
          Đang làm4
          Nhập điểm giữa kỳ Anh 8B
Vừa · hạn 18/09HV
            Chuyển sang ▾
          Soạn thông báo nghỉ lễ
ThấpTG
          Chờ duyệt24
          Bảng học phí T9 lớp Văn 12
Cao · hạn 14/09HV
          Kéo thả chưa hỗ trợ — dùng “Chuyển sang”
          Hoàn thành ✓35
          Đối chiếu điểm danh tuần 36
xong 11/09HV
      HvSegmented “Của tôi / Toàn trung tâm” chỉ render khi has("tasks.view_all"); mặc định “Của tôi”. → AC2.
      “Cấu hình cột” ghost button, chỉ render khi has("tasks.manage_board"); mở modal ở tab kế bên. → AC4.
      Thẻ của tôi viền trái mint-400; menu “Chuyển sang ▾” liệt kê cột động theo position (Radix DropdownMenu, phím [/] chuyển cột kề). → AC3.
      Cột do trung tâm thêm hiển thị y hệt cột mặc định; board scroll ngang khi >3 cột, mỗi cột tối thiểu 230px. Không màu/icon riêng theo cột (non-goal).
      Cột hoàn thành nền mint-50 + dấu ✓ ở tiêu đề; thẻ mờ 72%; hiện “xong dd/mm” từ completed_at. → AC6.
        Cấu hình cột
4 / 8 cột1
        ⋮⋮Cần làm↑↓🗑
        ⋮⋮Đang làm↑↓🗑
        ⋮⋮Chờ duyệt|↑↓🗑2
        ⋮⋮Hoàn thành↑↓🗑3
          ＋ Thêm cột
          Đóng
        Mỗi thay đổi lưu ngay (đổi tên khi blur, ↑/↓ gọi reorder, toggle gọi PATCH). Không có nút “Lưu” tổng vì không có CAS ở v1.
      Bộ đếm n/8 lấy từ response; “Thêm cột” disabled khi đủ 8; lỗi 409 trùng tên hiển thị dưới ô (từ ApiError.fields).
      Đổi tên inline (input thường, không cần dialog); ↑/↓ gửi toàn bộ thứ tự id lên PUT /task-columns/order; 409 lệch tập → toast “Bảng đã thay đổi, tải lại” + refetch.
      Toggle “cột hoàn thành” (Radix Switch) — có thể bật nhiều cột; nút xoá disabled khi chỉ còn 1 cột. → AC5, AC6.
        Xoá cột “Chờ duyệt”?
        Cột đang có 2 việc. Chọn cột để chuyển chúng sang trước khi xoá.
        Chuyển việc sangĐang làm ▾
1
        HuỷChuyển & xoá cột2
      HvSelect đích loại trừ chính cột đang xoá; bắt buộc chọn khi count > 0. Cột trống bỏ qua bước này và hiện HvConfirmDialog thường.
      Một request DELETE …?move_to=; API làm cả hai bước trong một tx, audit ghi move_to. → AC5.
      Công việc
＋
      Cần làm 5Đang làm 4Chờ duyệt 2Xong 3
1
        Nhập điểm giữa kỳ Anh 8B
Vừa · 18/09HV
          Chuyển sang ▾
        Soạn thông báo nghỉ lễ
ThấpTG
      ≤4 cột dùng HvSegmented với đếm số; >4 cột chuyển sang HvSelect chọn cột để tránh tab quá hẹp. Chỉ render một cột → không scroll ngang.
      Menu “Chuyển sang ▾” dùng chung với desktop; không phụ thuộc DnD.
        Thêm công việc
Esc để đóng
        Tiêu đề *Gọi phụ huynh lớp Toán 9A chưa đóng T9
        Mô tảDanh sách 6 PH còn nợ, ưu tiên gọi trước 17h…
          CộtCần làm ▾
1
          Giao choHọc vụ · Nguyễn Hoa ▾
2
          Hạn15/09/2026 📅
          Ưu tiênThấpVừaCao
          Xoá3
          HuỷLưu
      HvSelect “Cột” mặc định cột position 0 (hoặc cột đang mở khi bấm “＋” trong cột); danh sách lấy từ response board.
      HvSelect “Giao cho” nạp từ directory (members.list): tên + vai trò, không SĐT.
      Xoá chỉ hiện khi caller là người tạo hoặc owner và có tasks.delete; người được giao thấy form chế độ đọc + menu chuyển cột. → AC3.
      Phân quyền vai trò
Catalog v(N+1)
        Công việc 1Giáo viênHọc vụTrợ giảng
          Xem bảng công việc low✓✓✓
          Tạo công việc low✓✓✓
          Xem chi tiết công việc low✓✓✓
          Sửa & chuyển cột công việc low✓✓✓
          Xoá công việc medium✓✓✓
          Cấu hình cột bảng công việc high 2 ✓ 
          Xem mọi công việc trong trung tâm high ✓ 
        Thành viên
        Xem danh bạ thành viên low✓✓✓
      Không sửa UI ma trận: permission-matrix.tsx nhóm theo Resource từ catalog nên nhóm “Công việc” tự xuất hiện. Bằng chứng AC8 là fixture MSW mirror version mới.
      manage_board và view_all mặc định trống (fail-closed); hình minh hoạ owner gán cho Học vụ. Risk high → nhấn coral như scope keys hiện có.
  7. Rủi ro và câu hỏi mở
      Rủi ro đã nhận diện
        Sản phẩmĐi ngược PRD non-goal “cấu hình”. Đã là quyết định của người dùng; giảm thiểu bằng seed mặc định, trần 8 cột, không rule theo cột, và manage_board opt-in để chỉ một vài người đụng vào cấu trúc.
        Toàn vẹnXoá cột có việc: nếu di dời và xoá không cùng tx, board có thể lệch. Thiết kế bắt buộc 1 tx + FK RESTRICT làm lưới thứ hai; integration test trên Postgres thật.
        RBAC“Own rows = creator OR assignee” là ngoại lệ so với khuôn teacher_id = me; giữ trong đúng 2 hàm write, có test cross-member + scopelint. Cột không có own-rows: mọi write qua tasks.manage_board.
        CatalogDefaultRoleKeys() đang gom cả khoá special; cần DefaultGrant (hoặc loại tường minh) để manage_board không tự vào backfill. Đây là thay đổi contract catalog nhỏ, cần parity test.
        Đồng thờiKhông CAS cho cột ở v1: hai người cấu hình cùng lúc → last-write-wins cho rename/toggle, còn reorder và move fail-closed (409/404) rồi refetch. Có thể thêm board_version theo khuôn assignment_version sau.
        Dữ liệuBàn giao khi rời trung tâm nằm trong tx của centers nhưng SQL thuộc tasks: nếu hook bị quên ở wiring, việc của người đã rời vẫn trỏ stint đã đóng (không mất dữ liệu, nhưng “Của tôi” của owner không thấy). Test wiring ở server.registerFeatures + integration test xoá thành viên.
        RolloutKhông gán manage_board/view_all trước khi mọi instance API chạy catalog mới (docs §9).
        UXKhông DnD; board nhiều cột trên desktop phải scroll ngang. Giữ position cho DnD sau.
      Quyết định đã chốt (13/09/2026)
        tasks.manage_board: opt-in; owner gán qua ma trận (vai trò hoặc override).
        Thành viên rời trung tâm: giữ việc; assignee_id → NULL, created_by → owner trong tx RemoveMember.
        Trần 8 cột, nhiều cột is_done được phép.
        tasks.view_all: opt-in, cấu hình qua ma trận như mọi scope key hiện có.
      Không còn câu hỏi mở.
      Bước tiếp theo: chạy /ak:plan với hợp đồng ở mục 1, quyết định con ở mục 3 và 5 phase ở mục 5; sau đó /ak:cook.
Brainstorm brief rev 3 · Teka · phương án B, 4 quyết định đã chốt · bằng chứng từ repo tại nhánh master (commit bb13590) · mockup dựng bằng HTML/CSS thuần theo token dự án, không dùng trình sinh ảnh.

