---
title: "Red-team: Assumption Destroyer — plan Menu Giảng dạy"
plan: plans/260923-0715-giang-day-menu/
reviewer: rt-assumptions (Assumption Destroyer / Scope Auditor)
date: 2026-09-23
verdict: DONE_WITH_CONCERNS
---

# Red-team: giả định chịu lực bị gãy

Phạm vi đọc: `plan.md` + `phase-01`…`phase-09`. Mọi phát hiện được đối chiếu bằng grep vào mã
nguồn thật (`apps/api`, `apps/web`). Không sửa file nào.

## Finding 1: `handoff.Service.Reassign` chặn mọi caller không phải chủ trung tâm — luồng "chấp nhận lời mời" không chạy được

- **Severity:** Critical
- **Location:** Phase 2, "Key decisions" và bảng API (`POST /class-invitations/:id/accept`); plan.md D2
- **Flaw:** Plan chọn `handoff.Service.Reassign` làm đường ghi cho vai `giao_vien` khi **người được
  mời** bấm chấp nhận, và trích đúng `handoff/service.go:110` — nhưng dòng ngay sau đó là một cổng
  owner tuyệt đối. Người được mời theo định nghĩa không phải owner (plan ghi rõ route kind
  `authenticated (self)`, service kiểm `teacher_id == caller`).
- **Failure scenario:** Owner mời GV Lan vào lớp Toán 9C. Lan bấm "Chấp nhận" →
  `Reassign` trả `403 chỉ chủ trung tâm được bàn giao lớp`. Không có đường vòng: `classstaff.Service.Assign`
  cũng owner-only *và* từ chối thẳng `giao_vien` bằng 409. Toàn bộ acceptance criteria e2e của plan
  ("mời giáo viên → chấp nhận → xuất hiện trong Đội ngũ") chết ngay ở phase 2, chỉ còn
  `accept-on-behalf` (owner bấm hộ) chạy được — tức là sản phẩm mất đúng tính năng chính.
- **Evidence:**
  - `apps/api/internal/features/handoff/service.go:110-113`
    ```go
    func (s *Service) Reassign(ctx context.Context, sc authctx.Scope, classID, newTeacherID uuid.UUID) (*Result, error) {
        if !sc.IsOwner {
            return nil, apperror.Forbidden("chỉ chủ trung tâm được bàn giao lớp")
        }
    ```
  - `apps/api/internal/features/classstaff/service.go:85-93` — `Assign` cũng `if !sc.IsOwner` → 403,
    và `if req.RoleKey == authctx.StaffRoleGiaoVien` → 409 "giáo viên chính chỉ thay đổi qua bàn giao lớp".
  - Plan phase-02 dòng 21-22: "**Chấp nhận vai `giao_vien` đi qua `handoff.Service.Reassign`**
    (`handoff/service.go:110`) để giữ bất biến `uq_class_staff_one_gv`".
- **Suggested fix:** Không tái dùng `Reassign` trực tiếp. Hoặc (a) thêm vào `handoff` một entrypoint
  `AcceptInvitation` nhận scope người được mời + chứng cứ lời mời pending rồi tự nâng quyền nội bộ
  (owner-equivalent) trong tx, hoặc (b) đặt `classinvites` **trên** cả `handoff`/`classstaff` như
  `handoff` đang đứng trên `classes`+`sessions`, và cho nó gọi một hàm `ReassignForInvitation`
  mới có cổng "caller là người được mời của một lời mời pending". Phải chốt trước khi viết migration.

## Finding 2: Phase 7 bơm `teaching.Service` vào `classes` tạo vòng khởi tạo — router dựng `classes` trước `teaching` 65 dòng

- **Severity:** Critical
- **Location:** Phase 7, "API" → mục "Wiring"; plan.md D4
- **Flaw:** Plan giả định "định nghĩa port trong `classes`, adapter trong `router.go`" là đủ tránh
  vòng import. Nó tránh được vòng **compile-time** nhưng không tránh được vòng **construction-time**:
  `teaching.NewService` nhận `classesSvc` làm tham số bắt buộc, còn `classes.NewService` sẽ phải nhận
  adapter bọc `teachingSvc`. Toàn bộ 20+ service trong repo dùng constructor injection bất biến,
  không có setter nào.
- **Failure scenario:** Lập trình viên phase 7 tới `router.go:155`, cần truyền `teachingAdapter` vào
  `classes.NewService` nhưng `teachingSvc` chưa tồn tại (dòng 220). Lối thoát duy nhất là thêm
  `classesSvc.SetCurriculumWriter(...)` sau dòng 220 — một setter mutable trên service dùng chung,
  phá đúng bất biến mà package doc của `handoff` viết ra để tránh ("It lives outside classes
  deliberately: sessions is constructed with classes as a dependency, so classes cannot depend on
  sessions. A class handoff needs both, so it sits above them and is wired after both exist").
  Hậu quả thực tế: nếu setter bị quên trong một đường khởi tạo (ví dụ test harness hoặc seed binary),
  `PUT /classes/:id/program` nil-panic tại runtime thay vì lỗi biên dịch.
- **Evidence:**
  - `apps/api/internal/server/router.go:155` `classesSvc := classes.NewService(classes.NewRepository(db), txMgr, classStaffRepo)`
  - `apps/api/internal/server/router.go:220` `teachingSvc := teaching.NewService(teaching.NewRepository(db), classesSvc, sessionsSvc, enrollmentsSvc, txMgr)`
  - `apps/api/internal/features/handoff/service.go:1-10` (package doc mô tả đúng khuôn "feature điều phối đứng trên")
  - Plan phase-07 dòng ~48: "Wiring: `classes` nhận `teaching.Service` và `library.Repository`
    read-only qua interface hẹp trong `classes`".
- **Suggested fix:** Lật hướng: tạo feature điều phối `classprogram` (hoặc đặt trong `library`) đứng
  trên `classes` + `teaching` + `library`, dựng sau dòng 220, sở hữu route `PUT /classes/:id/program`.
  Không service nào bị thêm setter.

## Finding 3: `teaching.PutCurriculum` gác bằng capability `giao_vien`, không phải `classes.edit` — chủ nhiệm học vụ áp chương trình sẽ 403/404

- **Severity:** High
- **Location:** Phase 7, "Key decisions" (ghi đồng thời `class_curricula.lessons` qua `teaching.Service.PutCurriculum`), route `PUT /classes/:id/program` khai `classes.edit`
- **Flaw:** Hai cổng quyền khác vũ trụ. Route khai key catalog `classes.edit`, nhưng thân hàm đi qua
  `resolveClass(..., authctx.CapLessonPlanWrite)` → `classes.GetWritable` → write port `class_staff`
  với role `giao_vien` (owner bypass). Một thành viên có `classes.edit` nhưng giữ stint `hoc_vu` sẽ
  qua middleware rồi chết ở service.
- **Failure scenario:** Nhân viên học vụ (đúng nhân vật prototype giao việc "áp chương trình mẫu vào
  lớp") bấm "Áp dụng từ khóa mẫu". Middleware cho qua vì có `classes.edit`. `PutCurriculum` gọi
  `GetWritable` với `CapLessonPlanWrite` → `ErrNotFound` → normalize thành **404 "không tìm thấy lớp"**
  trên một lớp người đó đang nhìn thấy. Lỗi hiển thị sai hoàn toàn nguyên nhân, và UI không có cách
  nào tiên đoán để disable nút (`canWriteClass` = owner || giao_vien).
- **Evidence:**
  - `apps/api/internal/features/teaching/service.go:93-97` — `resolveClass(ctx, sc, classID, authctx.CapLessonPlanWrite)`
  - `apps/api/internal/features/teaching/service.go:551-552` — `resolveClass` = `s.classes.GetWritable(...)`
  - `apps/api/internal/features/teaching/service.go:562-573` — `normalizeClassErr` biến `classes.ErrNotFound` thành 404
  - `apps/web/src/features/roster/lib/class-permissions.ts:10-12` — `canWriteClass` = owner || `giao_vien`
- **Suggested fix:** Chốt sớm ai được áp chương trình. Nếu là học vụ, cần capability mới
  (`CapCurriculumWrite`) hoặc đường ghi `class_curricula` riêng, không mượn `CapLessonPlanWrite`.
  Ghi quyết định này vào phase 7 trước khi viết migration `class_programs`.

## Finding 4: `classes.CreateAnchored` (đường import Excel) không nằm trong inventory — `code NOT NULL UNIQUE` sẽ làm vỡ import từ lớp thứ hai

- **Severity:** High
- **Location:** Phase 1, bước B/8 ("`classes/service.go`: `Create` — nếu `Code` rỗng sinh…") và bảng File inventory
- **Flaw:** `classes` có **hai** đường tạo lớp. Plan chỉ nhắc `Create`. `CreateAnchored` — đường mà
  feature `imports` dùng — dựng `&Class{...}` trực tiếp, không đi qua logic sinh code.
- **Failure scenario:** Sau migration 000025, `code` là `NOT NULL` (DEFAULT đã DROP) + unique partial
  index `(center_id, code) WHERE deleted_at IS NULL`. Owner import file Excel 20 lớp mới: lớp đầu tiên
  ghi `code = ''` thành công, lớp thứ hai đâm vào `uq_classes_center_code` → cả transaction import
  rollback với lỗi duplicate key thô. Tính năng import (đang chạy tốt ở master) hỏng, và không có test
  nào trong ma trận Phase 1 bắt được vì ma trận chỉ phủ `POST /classes`.
- **Evidence:**
  - `apps/api/internal/features/classes/service.go:56-75` — `CreateAnchored` dựng `Class` không có trường `Code`
  - `apps/api/internal/features/imports/service.go:52` — `CreateAnchored(ctx, a, req classes.CreateClassRequest)` là port của imports
  - `apps/api/internal/features/imports/apply.go:319-321` — `classRequest()` dựng `classes.CreateClassRequest`
  - Plan phase-01 "File inventory" không liệt kê `imports/`; "Success Criteria" chỉ cam kết không đổi
    hành vi `/students`, `/classbook`, `/classes/:id/settings`.
- **Suggested fix:** Đưa việc sinh `code` xuống một helper dùng chung được cả `Create` và
  `CreateAnchored` gọi (hoặc xuống repository `CreateWithSchedules`), và thêm một case integration
  test import 2 lớp trong cùng một run.

## Finding 5: Backfill `code` trong migration 000025 không chống trùng — `CREATE UNIQUE INDEX` có thể làm migration production dirty

- **Severity:** High
- **Location:** Phase 1, "Implementation Steps → A. Migration", và Risk Assessment dòng 1
- **Flaw:** Migration chạy `UPDATE … SET code = 'L' || upper(substr(...))` rồi `CREATE UNIQUE INDEX`
  ngay sau, **không** có vòng khử trùng. Plan tự nhận diện rủi ro rồi giảm thiểu bằng cách đổi sang
  `substr(..., 7, 6)` "phần random" — nhưng UUIDv7 dành **48 bit đầu (12 ký tự hex đầu)** cho timestamp
  mili-giây; vị trí 7–12 vẫn là timestamp (24 bit thấp, lặp lại mỗi ~4,66 giờ). Không có vị trí nào
  trong 12 ký tự đầu là random.
- **Failure scenario:** Trung tâm production có N lớp. Hai lớp bất kỳ tạo cách nhau đúng bội số
  2^24 ms sinh cùng 6 hex → `CREATE UNIQUE INDEX` lỗi giữa chừng. golang-migrate đánh dấu version
  dirty; 4 cột đã tồn tại, index chưa; `make migrate-up` lần sau từ chối chạy. Khắc phục thủ công trên
  DB production (`teka-*` containers theo memory) dưới áp lực.
- **Evidence:**
  - `apps/api/internal/shared/id/id.go:10` — `func New() uuid.UUID { return uuid.Must(uuid.NewV7()) }`
  - Plan phase-01 bước A/1 (khối SQL) và Risk Assessment: "Dùng `substr(replace(id::text,'-',''), 7, 6)`
    (phần random) thay vì 6 đầu; test parity" — tiền đề "phần random" sai.
- **Suggested fix:** Backfill bằng `row_number()` theo `center_id` (`'L' || lpad(rn::text, 5, '0')`),
  hoặc giữ hash rồi thêm vòng `WHILE EXISTS (… GROUP BY center_id, code HAVING count(*) > 1)` gắn hậu
  tố trước khi tạo index. Và tạo index trước khi `ALTER COLUMN … DROP DEFAULT` để lỗi rơi vào bước có
  thể rollback sạch.

## Finding 6: `center_members` không bao giờ bị xoá dòng — giả định "FK cascade xoá lời mời" sai, để lại lời mời pending cho người đã rời trung tâm

- **Severity:** High
- **Location:** Phase 2, "Migration sketch" (FK `center_members` ON DELETE CASCADE) và mục "Risks" dòng cuối
- **Flaw:** Rời trung tâm là **soft close** (`left_at = now()`), không phải DELETE. Vào lại là UPSERT
  đặt `left_at = NULL`. Vì vậy `ON DELETE CASCADE` không bao giờ kích hoạt, và composite FK
  `(center_id, teacher_id)` chỉ chứng minh "từng là thành viên", không chứng minh "đang hoạt động" —
  đúng điều plan tuyên bố nó bảo đảm ("Người được mời phải là `center_members` **đang hoạt động** của
  cùng trung tâm (composite FK)").
- **Failure scenario:** Owner mời GV Hùng. Hùng rời trung tâm trước khi trả lời. Lời mời vẫn `pending`.
  Hai hệ quả: (1) `accept-on-behalf` của owner sẽ gán một người ngoài roster vào `class_staff` — không
  có bước kiểm `IsActiveMember` nào trong đường ghi mà plan mô tả (`classstaff` repo `Create` trực tiếp,
  bỏ qua `Assign`); (2) `uq_class_invitations_pending (class_id, teacher_id) WHERE status='pending'`
  khoá luôn việc mời lại chính người đó khi họ quay lại, cho tới khi ai đó cancel thủ công.
- **Evidence:**
  - `apps/api/internal/features/centers/repository.go:481-482` — `UPDATE center_members SET left_at = now() WHERE teacher_id = @tid AND center_id = @cid AND left_at IS NULL`
  - `apps/api/internal/features/centers/repository.go:453` — `DO UPDATE SET left_at = NULL, joined_at = now(), role_id = EXCLUDED.role_id`
  - `apps/api/migrations/000007_centers.up.sql:66-74` — PK `(teacher_id, center_id)` + `uq_center_members_active … WHERE left_at IS NULL`
  - `apps/api/internal/features/classstaff/service.go:94-100` — `IsActiveMember` là bước mà `Assign` bắt buộc, còn plan đi thẳng repo `Create`
  - Plan phase-02 "Risks": "Người được mời rời trung tâm → FK cascade xoá lời mời (chấp nhận)."
- **Suggested fix:** Bỏ giả định cascade. Kiểm `IsActiveMember` trong service ở **cả** accept và
  accept-on-behalf; thêm job/điều kiện tự chuyển lời mời sang `cancelled` khi membership đóng, hoặc
  đổi unique index pending thành `(class_id, teacher_id) WHERE status='pending'` + cho phép cancel
  ngầm khi mời lại.

## Finding 7: `GET /centers/me/members` không tồn tại; endpoint thật là `/centers/me/members/directory` và bị gác bởi key opt-in `members.list`

- **Severity:** Medium
- **Location:** Phase 2, "Web (feature `roster`)" — `InviteTeacherDialog` (chọn thành viên từ `GET /centers/me/members`)
- **Flaw:** Sai đường dẫn, và sai giả định về quyền. `members.list` là một trong đúng hai key
  `DefaultGrant: false` của catalog v4 — vai trò mặc định **không** có nó.
- **Failure scenario:** Owner cấp `class_invites.manage` cho quản lý cơ sở để họ tự mời GV. Người này
  mở `InviteTeacherDialog` → `GET /centers/me/members/directory` trả 403 vì thiếu `members.list`.
  Dialog rỗng, không giải thích được, trong khi nút "+ Mời GV" vẫn hiện (quyền gửi lời mời họ có đủ).
- **Evidence:**
  - `apps/api/internal/features/centers/routes.go:10` — `g.GET("/me/members/directory", h.directory)` (không có `GET /me/members`)
  - `apps/api/internal/shared/routespec/routespec.go:202` — `perm("GET", "/api/v1/centers/me/members/directory", authctx.PermMembersList, none())`
  - `apps/api/internal/shared/authctx/catalog_test.go:41-44` — `optInKeys = []string{PermTasksManageBoard, PermMembersList}`
- **Suggested fix:** Sửa đường dẫn trong plan, và chốt: hoặc `class_invites.manage` `implied` ⇒
  `members.list` cho reads, hoặc thêm một endpoint hẹp "thành viên có thể mời vào lớp này" nằm trong
  `classinvites` với chính key `class_invites.manage`.

## Finding 8: `CountReadableByPhase` **không** thoả luật đặt tên của scopelint — `Readable` không phải token `read`

- **Severity:** Medium
- **Location:** Phase 1, bước B/7: "(tên hàm chứa `read` để `scopelint` cho phép `CenterWideFor`)"
- **Flaw:** Analyzer tách camelCase thành token rồi so **bằng** (`EqualFold`), có comment nói rõ vì sao
  không dùng substring. `CountReadableByPhase` → `{Count, Readable, By, Phase}`; không token nào là
  `read`. Khuôn đang chạy trong repo là `readScoped` → `{read, Scoped}`.
- **Failure scenario:** Viết xong repository, `make scopelint` (chạy trong cả `test-api-unit` và
  `lint-api`) báo "CenterWideFor may only widen a read-named function" và CI đỏ; người sửa dễ nhầm
  hướng sang `WriteWide()` (sai ngữ nghĩa, chỉ owner) thay vì đổi tên hàm.
- **Evidence:**
  - `apps/api/tools/scopelint/scopelint/analyzer.go:428-434` — "contains a token equal to \"read\" … A substring match would wrongly accept" + `strings.EqualFold(tok, "read")`
  - `apps/api/tools/scopelint/scopelint/analyzer.go:489-490` — chính thông điệp lỗi
  - `apps/api/internal/features/classes/repository.go:124-127` — khuôn hợp lệ `readScoped` gọi `sc.CenterWideFor(authctx.PermClassesViewAll)`
  - `apps/api/Makefile:68,72` — `test-api-unit: scopelint`
- **Suggested fix:** Đặt tên `countReadScopedByPhase` (hoặc đơn giản là gọi `r.readScoped(ctx, sc)`
  có sẵn thay vì tự gọi `CenterWideFor`). Áp dụng cùng lưu ý cho Phase 3/5/6/8 vốn ghi "read port
  `CenterWideFor` trong hàm `*read*`".

## Finding 9: Ba "tái dùng nguyên" ở Phase 1 không khớp chữ ký/khả năng thật: `StatusPill`, `canWriteClass`, `tableHeadCellClassName`

- **Severity:** Medium
- **Location:** Phase 1, Implementation Steps bước 16 và 18; R9
- **Flaw:** Cả ba đều được plan mô tả như có sẵn dùng ngay.
  1. `StatusPill` chỉ nhận `"paid" | "partial" | "unpaid"` (bảng màu hoá đơn), không có biến thể cho
     4 trạng thái lớp `upcoming/running/ended/archived`.
  2. `canWriteClass` có **hai** tham số `(isOwner, klass)` chứ không phải `canWriteClass(cls)` như plan
     viết; và ngữ nghĩa là owner-or-`giao_vien`, tức là học vụ không sửa được ghi chú vận hành — mâu
     thuẫn với R9 vốn xem ô ghi chú là việc vận hành.
  3. `tableHeadCellClassName` là `const` module-private trong `classes-tab.tsx`, không export.
- **Failure scenario:** Typecheck đỏ ngay ở bước đầu (mục 1 và 2), và mục 3 dẫn tới copy-paste chuỗi
  class Tailwind sang 3 bảng mới — đúng loại trùng lặp mà rule DRY của repo cấm.
- **Evidence:**
  - `apps/web/src/components/hv/status-pill-labels.ts:1` — `export type StatusPillStatus = "paid" | "partial" | "unpaid";`
  - `apps/web/src/components/hv/status-pill.tsx:10-18` — `statusPillClasses: Record<StatusPillStatus, string>`
  - `apps/web/src/features/roster/lib/class-permissions.ts:10` — `export function canWriteClass(isOwner: boolean, klass: Pick<Class, "my_staff_roles">): boolean`
  - `apps/web/src/features/roster/components/classes-tab.tsx:10` — `const tableHeadCellClassName =` (không `export`)
  - Plan phase-01 bước 16: "trạng thái dùng `StatusPill`"; bước 18: "gate `canWriteClass(cls) && has(\"classes.edit\")`"
- **Suggested fix:** Dùng `HvChip` cho chip trạng thái lớp (đã có, plan cũng đã chọn cho dải chip), nâng
  `tableHeadCellClassName` lên `components/hv` rồi cho `classes-tab.tsx` import ngược, và chốt lại ai
  sửa được ô vận hành trước khi viết `ClassOpsCard`.

## Finding 10: `classesKeys.stats` không được invalidate khi tạo lớp, và handler MSW `/classes/stats` sẽ bị `/classes/:id` nuốt

- **Severity:** Medium
- **Location:** Phase 1, bước 14 ("`useUpdateClass` invalidate thêm `classesKeys.stats()`") và bước 21 (MSW)
- **Flaw:** Hai lỗ nhỏ cùng nằm ở state mới `stats`.
  1. Chỉ `useUpdateClass` được nhắc. `useCreateClass` invalidate **duy nhất** `classesKeys.lists()`.
  2. MSW khớp handler theo **thứ tự đăng ký**, không theo độ đặc hiệu như gin. `roster-handlers.ts`
     đã đăng ký `GET /classes/:id` ở dòng 401; handler `/classes/stats` thêm vào cuối mảng sẽ không
     bao giờ chạy.
- **Failure scenario:** (1) Người dùng tạo lớp mới: bảng cập nhật ngay (lists invalidated), nhưng dải
  chip vẫn hiện "Đang học 12" thay vì 13 cho tới khi reload — ngay cạnh hàng mới. (2) Trong test,
  `useClassStats` nhận về một `ClassResponse`; `classStatsSchema.parse` ném lỗi và trang rơi vào nhánh
  error — người viết test sẽ nghi ngờ hook trước khi nghi ngờ thứ tự handler.
- **Evidence:**
  - `apps/web/src/features/roster/hooks/use-classes.ts:38-45` — `useCreateClass … invalidateQueries({ queryKey: classesKeys.lists() })` (chỉ một dòng)
  - `apps/web/src/features/roster/hooks/use-classes.ts:70` — `useArchiveClass`-nhóm dùng `classesKeys.all` (khuôn đúng)
  - `apps/web/src/features/roster/__tests__/roster-handlers.ts:401` — `http.get(`${API_URL}/classes/:id`, …)`
  - `apps/web/src/features/roster/hooks/roster-keys.ts:30-33` — cây key hiện tại, chưa có `stats`
- **Suggested fix:** Định nghĩa `stats: () => [...classesKeys.all, "stats"]` rồi đổi `useCreateClass`
  sang invalidate `classesKeys.all` (một dòng, phủ cả lists lẫn stats); chèn handler `/classes/stats`
  **trước** dòng 401 trong `roster-handlers.ts` và ghi chú lý do.

---

# Verification Results — Scope Auditor

Vòng đời (lifetime) của từng state mới mà plan thêm, kèm mọi site khởi tạo/tiêu thụ đã grep.

| State mới | Phase | Lifetime | Site khởi tạo / tiêu thụ đã grep | Đã có thứ phục vụ cùng mục đích? | Kết luận |
|---|---|---|---|---|---|
| `classes.code / tags / recruiting / note` (4 cột) | 1 | Persistent (row-scoped) | Ghi: `classes/service.go:34 Create`, **`:56 CreateAnchored` (plan bỏ sót)**, `Update`; đọc: `repository.go:255 ListReadable`, `:243 GetReadableByID`, `seeds/seed.go:128,175` | Không. `classes` (000001:127-141) không có khái niệm code/tags/note nào; `class_staff` và `class_schedules` cũng không | **PASS có điều kiện** — xem Finding 4 và 5 |
| `classes.course_id`, `parent_class_id`, `lineage_note` | 5, 7 | Persistent | Chưa tồn tại; composite FK cần `uq_classes_cid` — **đã có** tại `migrations/000007_centers.up.sql:165` | Không trùng | PASS (composite FK target hợp lệ) |
| `ListFilter{Q, Weekday, Shift, Tag, Phase}` | 1 | Request-scoped (struct giá trị, không chia sẻ) | Khởi tạo: `classes/handler.go:107`; test: `classes/integration_test.go:214,384`. Toàn bộ 4 site, không site nào ngoài package | Khuôn có sẵn: `students/handler.go:133` `ListFilter{...}` nhiều trường, `students.ListFilter{ClassID, Unenrolled}` | PASS — thêm trường không phá caller nào (tất cả dùng literal có tên trường) |
| `ClassStats` / `GET /classes/stats` | 1 | Request-scoped | Mới hoàn toàn; không đụng state chia sẻ | Không có endpoint đếm lớp nào hiện tại | PASS về scope; **FAIL về tên hàm** — Finding 8 |
| `shared/dbtypes.StringList` | 1 | Type-level (không có lifetime runtime) | Định nghĩa trùng duy nhất trong repo: `teaching/model.go:42 type StringList []string` (Scan/Value JSONB y hệt) | **CÓ** — `teaching.StringList` đã tồn tại và plan chủ động copy, để lại 2 bản | **CONCERN** — DRY. Hoặc chuyển `teaching` sang `dbtypes` cùng lúc (rẻ: 1 type, 1 package), hoặc bỏ `dbtypes` và tự khai trong `classes`. Không nên để 2 bản song song lâu dài |
| `classSchema` + `.default()` cho 5 trường | 1 | Request-scoped (parse mỗi response) | `roster-schemas.ts:177-190`; đã có tiền lệ `.default()` tại `:187 my_staff_roles`, `:189 student_count` | Có khuôn sẵn; zod mặc định **strip** key lạ nên thêm trường không phá consumer nào | PASS — rủi ro "fixture đổi shape làm đỏ test khác" mà plan xếp Trung bình thực tế là Thấp |
| Query key `classesKeys.stats` | 1 | Session-scoped (QueryClient cache) | `roster-keys.ts:30-33` (định nghĩa), 15 site đọc/invalidate trong `use-classes.ts` + `use-enrollments.ts:71,103` | Cây key đã có `all/lists/list/details/detail` | **FAIL một phần** — `useCreateClass` (`use-classes.ts:38-45`) không invalidate stats. Finding 10 |
| MSW class fixture (`makeClass`) | 1 | Test-scoped | Chỉ **3** file: `test/msw/handlers.ts:488` (định nghĩa), `features/center/__tests__/class-config-page.test.tsx`, `features/dashboard/__tests__/dashboard-page.test.tsx` | — | PASS — blast radius nhỏ hơn plan lo (plan xếp "Trung bình"); hạ xuống Thấp |
| `CatalogVersion` mirror | 2,3,5,6,7,8 | Build-time hằng số, nhân đôi API↔web | API: `authctx/catalog.go:334`; **test ghim cứng** `authctx/catalog_test.go:316-318` (`if CatalogVersion != 4 { t.Fatalf("...must be 4...") }`); web: `test/msw/handlers.ts:17 CATALOG_VERSION = 4` + `:390` | Có sẵn cơ chế, nhưng plan chỉ nói "bump + mirror MSW" mà **không** liệt kê `catalog_test.go:316` — mỗi lần bump phải sửa cả literal lẫn message của test | **CONCERN** — thêm `catalog_test.go` vào file inventory của mọi phase có key mới (2,3,5,6,7,8). Plan dự kiến bump 6 lần → v10 |
| Nav entries mới (6 mục) | 1,2,3,5,6,8 | Render-scoped | `layouts/dashboard-layout.tsx:172-186 OVERFLOW_LABELS`, `:189-206 OVERFLOW_PATH_PREFIXES`, `:544-545` lọc primary/overflow | Plan chỉ nhắc `OVERFLOW_LABELS`; **thiếu** `OVERFLOW_PATH_PREFIXES` → tab "Thêm" không sáng khi đang ở `/classes`, `/library`, `/courses`, `/paths`, `/prep` | **CONCERN** — thêm 5 prefix mới vào danh sách `:189-206` |
| `backfill_parity_test` như tiêu chí nghiệm thu | plan.md Success Criteria | — | `migrations/backfill_parity_test.go:13-21` tự mô tả: đóng băng theo catalog v2 của **riêng 000018**, "The test guards exactly one invariant: nobody edits the shipped SQL" | — | **FAIL** — test này xanh bất kể 000027/000029/000030/000032 backfill đúng hay sai. Tiêu chí "backfill_parity_test xanh" tạo cảm giác an toàn giả; cần integration test riêng kiểm `center_role_permissions` + `center_member_permissions` sau mỗi backfill mới |

## Giả định đã kiểm và **đúng** (không phải finding)

- `classes` **có** `UNIQUE (id, center_id)` → composite FK của Phase 2/5/7 hợp lệ.
  `migrations/000007_centers.up.sql:165`.
- `GET /classes/:id/sessions` tồn tại + fixture MSW có sẵn. `sessions/routes.go:10`,
  `routespec.go:270`, `roster-handlers.ts:578`, `test/msw/handlers.ts:515`.
- `useEnrollmentsList({class_id, active})` đúng shape. `use-enrollments.ts:19`,
  `enrollments-api.ts:12-19`.
- `HvSelect`, `HvSegmented variant="tabs"`, `HvChip`, `HvStateBlock`, `HvConfirmDialog`,
  `formatWeekday`, `formatScheduleLabel`, `useBoardUrlState` đều tồn tại đúng như plan mô tả.
- gin **chấp nhận** sibling tĩnh cạnh `:param` — tiền lệ `centers/routes.go:10` (`/me/members/directory`)
  cạnh `:12` (`/me/members/:teacherId`). `GET /classes/stats` cạnh `/classes/:id` không panic.
- react-router xếp hạng `classes/:id/settings` trên `classes/:id`; đã có comment tiền lệ cho
  `students/import` tại `roster/routes.tsx:23-25`.
- `uq_class_staff_one_gv` + `classes.teacher_id NOT NULL` đúng như D2 mô tả.
  `migrations/000015_class_staff.up.sql:32-33`, `000001_baseline_schema.up.sql:129`.
- `class_curricula.class_id` là `UNIQUE` → `class_programs(class_id PK)` một-lớp-một-chương-trình
  nhất quán. `migrations/000009_teaching.up.sql:22`.

## Câu hỏi chưa giải quyết

1. Ai được phép áp chương trình mẫu vào lớp — owner, `giao_vien`, hay `hoc_vu`? Quyết định này chặn
   Finding 3 và quyết định luôn shape của Phase 7.
2. Khi đổi sang phiên bản có **ít buổi hơn**, `lesson_plans` ở index ngoài phạm vi được plan ghi là
   "vẫn còn trong DB (documented)". Nhưng trường hợp nguy hiểm hơn chưa được nhắc: đổi sang phiên bản
   **cùng số buổi khác thứ tự** — giáo án index i im lặng gắn sang một buổi học khác. Có cần
   khoá/cảnh báo không?
3. Phase 8 vẫn treo ở "Chờ xác nhận Validation (Q2)" nhưng Phase 9 lại phụ thuộc cứng vào 8. Nếu Q2
   trả lời "để sau", Phase 9 cần định nghĩa lại dependency.
