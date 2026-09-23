# Red-team review — góc nhìn SECURITY ADVERSARY

Plan: `plans/260923-0715-giang-day-menu/` (plan.md + phase-01…phase-09)
Vai trò kiểm chứng: FACT CHECKER
Ngày: 2026-09-23

---

## Finding 1: `class_invites.manage` là cửa hậu đi vòng qua cổng owner_only của nhân sự lớp

- **Severity:** Critical
- **Location:** Phase 2, "Key decisions" + bảng API `POST /classes/:id/invitations`, `POST /class-invitations/:id/accept-on-behalf`
- **Flaw:** Mọi thao tác gán/gỡ nhân sự lớp hôm nay là `KindOwnerOnly`, cố tình **không grantable**. Phase 2 tạo một khoá catalog grantable (`class_invites.manage`, `optIn`) đạt được đúng kết quả đó qua đường khác: "vai khác gọi `classstaff` repo `Create` trong cùng tx" — gọi thẳng repository, bỏ qua `Service.Assign`.
- **Failure scenario:** Chủ trung tâm cấp `class_invites.manage` cho một học vụ để họ "mời giáo viên hộ". Người này gọi `POST /classes/:id/invitations {teacher_id: <bất kỳ member>, role_key: 'hoc_vu'}` rồi `POST /class-invitations/:id/accept-on-behalf` — không cần sự đồng ý của người được mời. Kết quả là một dòng `class_staff` mới trên lớp bất kỳ trong trung tâm, tức là chính xác thứ mà `POST /api/v1/classes/:id/staff` từ chối cho bất kỳ ai không phải owner. Một khoá optIn duy nhất thay thế được cả cổng owner.
- **Evidence:**
  - `apps/api/internal/shared/routespec/routespec.go:32-34` — "KindOwnerOnly routes are hard-gated on ownership, **never grantable — one grant away from escalation otherwise**"
  - `apps/api/internal/shared/routespec/routespec.go:169` — `classified("POST", "/api/v1/classes/:id/staff", KindOwnerOnly, req("class.staff.assign", "class", "id"))`
  - `apps/api/internal/shared/routespec/routespec.go:170-171` — `DELETE /api/v1/classes/:id/staff/:staffId` cũng `KindOwnerOnly`
  - `apps/api/internal/features/classstaff/service.go:85-86` — `if !sc.IsOwner { return nil, apperror.Forbidden("chỉ chủ trung tâm được gán nhân sự lớp") }`
  - `apps/api/internal/features/classstaff/service.go:141-142` — cùng cổng cho `Remove`
  - `docs/adding-permissions.md:42` — "use the `owner_only` route policy and **must not become catalog permissions**"
  - Plan: phase-02 "vai khác gọi `classstaff` repo `Create` trong cùng tx"; "`POST /class-invitations/:id/accept-on-behalf` | `class_invites.manage` | nút 'GV nhận lớp'"
- **Suggested fix:** Phân tách hai năng lực. Gửi/nhắc/huỷ lời mời (không đổi `class_staff`) có thể là khoá grantable. Nhưng mọi đường **ghi vào `class_staff`** — accept, accept-on-behalf — phải đi qua `classstaff.Service.Assign` với cổng `IsOwner` nguyên vẹn, hoặc `accept-on-behalf` phải bị xoá khỏi scope và khai báo `KindOwnerOnly`. Ghi rõ trong plan rằng invitation **không** phải là một bề mặt gán nhân sự thứ hai.

---

## Finding 2: Luồng accept vai `giao_vien` qua `handoff.Service.Reassign` không chạy được — hoặc chạy được bằng cách giả mạo scope owner

- **Severity:** Critical
- **Location:** Phase 2, "Key decisions" dòng "Chấp nhận vai `giao_vien` đi qua `handoff.Service.Reassign` (`handoff/service.go:110`)"
- **Flaw:** Plan trích dẫn đúng file và đúng dòng nhưng bỏ qua câu lệnh ngay dòng kế tiếp: `Reassign` chặn cứng `!sc.IsOwner`. Người gọi `POST /class-invitations/:id/accept` theo thiết kế là **người được mời** (route "authenticated (self)"), gần như không bao giờ là owner.
- **Failure scenario:** Giáo viên được mời bấm "Nhận lớp" → service gọi `Reassign` với scope của chính họ → `403 "chỉ chủ trung tâm được bàn giao lớp"`. Luồng chính của Phase 2 chết. Áp lực sửa nhanh khi đó sẽ là một trong hai thứ tệ hơn: (a) dựng một `authctx.Scope{IsOwner: true}` tổng hợp để gọi Reassign — biến mọi lời mời thành một lần leo quyền owner tuỳ ý trong một transaction có `TryLockCenter` + đổi `classes.teacher_id` + dời session tương lai; (b) sao chép logic Reassign vào `classinvites` và làm hỏng bất biến `uq_class_staff_one_gv` mà D2 nói là lý do tái dùng handoff.
- **Evidence:**
  - `apps/api/internal/features/handoff/service.go:110-113`:
    ```go
    func (s *Service) Reassign(ctx context.Context, sc authctx.Scope, classID, newTeacherID uuid.UUID) (*Result, error) {
        if !sc.IsOwner {
            return nil, apperror.Forbidden("chỉ chủ trung tâm được bàn giao lớp")
        }
    ```
  - `apps/api/internal/features/handoff/service.go:103-104` — doc comment: "**Only the owner may do it**"
  - `apps/api/internal/shared/routespec/routespec.go:172` — `PUT /api/v1/classes/:id/teacher` là `KindOwnerOnly`
  - `apps/api/internal/shared/authctx/catalog.go:381-386` — `WriteWide()` chỉ trả true cho owner, kèm chú thích "a key widening writes would be an escalation"
- **Suggested fix:** Quyết định dứt điểm ở giai đoạn plan, không đẩy xuống implement: lời mời vai `giao_vien` chỉ **đề xuất**, và bước hoàn tất bàn giao phải do owner bấm (route `KindOwnerOnly` hiện có). Cấm tuyệt đối việc tổng hợp `Scope{IsOwner: true}` trong `classinvites`; thêm một dòng cấm này vào Security Considerations của Phase 2.

---

## Finding 3: Chuỗi leo thang PII — tự mời mình vai `hoc_vu` để mở khoá số điện thoại phụ huynh

- **Severity:** Critical
- **Location:** Phase 2, bảng API (`POST /classes/:id/invitations` + `POST /class-invitations/:id/accept`); Phase 1 "Nhân viên phụ trách = `class_staff` vai `hoc_vu`"
- **Flaw:** Plan không có bất kỳ ràng buộc nào cấm `teacher_id == caller`, và không nhận ra rằng một stint `hoc_vu` **đang hoạt động** là cột mốc quyết định của lớp bảo vệ số điện thoại (PII) trong repo này. Lời mời + tự chấp nhận là hai request.
- **Failure scenario:** Một thành viên được cấp `class_invites.manage` gọi `POST /classes/<lớp bất kỳ>/invitations {teacher_id: <chính họ>, role_key: 'hoc_vu'}`, rồi `POST /class-invitations/:id/accept` (route self, chính họ là người được mời → qua cổng). Một dòng `class_staff` vai `hoc_vu`, `ended_at IS NULL` ra đời. Từ đó `PhoneVisibleViaStudent` trả TRUE cho mọi học viên đang học của lớp đó, và `PhoneVisibleViaContact` kéo theo số điện thoại phụ huynh trên contacts, statements, notifications, collections. Hai lệnh HTTP đổi lấy PII của cả lớp, không có owner nào bấm nút.
- **Evidence:**
  - `apps/api/internal/shared/classscope/classscope.go:74-96` — `PhoneVisibleViaStudent`: `cs3.role_key = 'hoc_vu' AND cs3.ended_at IS NULL` … "Owner and reports-oversight bypass happens in the service"
  - `apps/api/internal/shared/classscope/classscope.go:98-110` — `PhoneVisibleViaContact` mở rộng cùng quy tắc sang contacts/statements/notifications/collections
  - `apps/api/internal/features/classstaff/service.go:85` — hôm nay chỉ owner tạo được stint đó
  - Plan phase-02: `role_key VARCHAR(20) NOT NULL CHECK (role_key IN ('giao_vien','tro_giang','hoc_vu'))`, không có rule self-invite
- **Suggested fix:** (1) Từ chối 422 khi `teacher_id == caller.TeacherID` trên `POST /classes/:id/invitations`. (2) Giữ vai `hoc_vu` **ngoài** luồng lời mời hoàn toàn — nó là vai mở PII, để nguyên ở `POST /classes/:id/staff` owner-only. (3) Thêm một dòng vào Success Criteria: "không luồng nào trong plan tạo được stint `hoc_vu` mà không có hành động của owner".

---

## Finding 4: FK thiếu guard composite `center_id` — rò dữ liệu xuyên trung tâm ở 5 bảng

- **Severity:** Critical
- **Location:** Phase 5 "Migration sketch — 000029_courses"; Phase 7 "000031_class_programs"; Phase 4 "000028_library_items"; Phase 6 "000030_learning_paths"; Phase 8 "000032_prep"
- **Flaw:** Repo có một khuôn toàn vẹn tenant rõ ràng: bảng cha mang `UNIQUE (id, center_id)`, bảng con mang `center_id` và FK composite. Plan áp dụng khuôn này cho `classes.course_id` (đúng), rồi **bỏ quên nó ở mọi FK còn lại**, dùng FK một cột trỏ vào `id` trần.
- **Failure scenario:** `courses.default_template_version_id UUID NULL REFERENCES program_template_versions(id)` không mang `center_id`. Phase 5 chỉ nói service "kiểm tra published", không nói "cùng trung tâm". Một người dùng ở trung tâm A đoán/lấy được một UUID phiên bản của trung tâm B (rò qua log, qua e2e seed dùng chung, qua một endpoint khác) và gán vào khoá học của mình. Phase 7 rồi `PUT /classes/:id/program {template_version_id}` áp phiên bản đó vào lớp, và `GET /classes/:id/program/lessons` đọc xuyên sang lessons + materials + exercises của trung tâm B — toàn bộ giáo trình thương mại của đối thủ, đọc được qua một endpoint hợp lệ với quyền hợp lệ. Cùng lỗ hổng lặp lại ở `template_lesson_materials.material_id`, `path_stage_courses.course_id`, `prep_projects.generated_template_id`, `classes.parent_class_id`.
- **Evidence:**
  - `apps/api/migrations/000007_centers.up.sql:157` — "UNIQUE (id, center_id) trên bảng cha làm target cho FK con; FK guard"
  - `apps/api/migrations/000007_centers.up.sql:163-166` — `uq_contacts_cid`, `uq_students_cid`, `uq_classes_cid`, `uq_enrollments_cid`
  - `apps/api/migrations/000022_task_board.up.sql:2-4` — "FK composite kèm center_id để **một dòng không bao giờ trỏ chéo trung tâm**"
  - `apps/api/migrations/000014_grading.up.sql:10` và `000009_teaching.up.sql:7` — cùng khuôn
  - Plan phase-05: `default_template_version_id UUID NULL REFERENCES program_template_versions(id)` (không center_id); phase-07: `template_version_id UUID NOT NULL REFERENCES program_template_versions(id)`; phase-04: `template_lesson_materials (lesson_id FK CASCADE, material_id FK, …)`; phase-06: `path_stage_courses (stage_id FK CASCADE, course_id FK, position INT, …)`
- **Suggested fix:** Bắt buộc mọi bảng mới mang `center_id NOT NULL`, mọi bảng được tham chiếu mang `UNIQUE (id, center_id)`, và mọi FK là composite `(x_id, center_id)`. Đưa điều này lên `plan.md` thành quyết định xuyên phase D8 thay vì để mỗi phase tự nhớ. Việc này chặn ở DB, không phụ thuộc service nhớ kiểm tra.

---

## Finding 5: `def()` + backfill mặc định trao quyền sửa kho học liệu và **giá học phí** cho mọi giáo viên; `score-set` dựng lại bề mặt vốn owner-only

- **Severity:** High
- **Location:** plan.md D5; Phase 3 "Quyền: `library.read`, `library.edit` (`def`, DefaultGrant → backfill 2 bảng)"; Phase 4 "Không key mới: dùng `library.read/edit`" + `PUT /library/versions/:vid/score-set`; Phase 5 `courses.read/courses.edit`
- **Flaw:** `def()` bật `DefaultGrant: true`, và backfill theo khuôn 000022 rải khoá cho **mọi vai hệ thống** (`giao_vien`, `hoc_vu`, `tro_giang`) cộng mọi stint không vai trò. Plan không cân nhắc rằng nội dung đằng sau các khoá này là tài sản thương mại chứ không phải dữ liệu vận hành của riêng người dùng.
- **Failure scenario:** Ngay sau khi migrate, **mọi trợ giảng** trong mọi trung tâm có `courses.edit` → sửa được `courses.default_unit_price` và ghi đè toàn bộ `course_tuition_packs` (`PUT /courses/:id/tuition-packs`, ghi đè toàn bộ). Một trợ giảng bất mãn đặt giá gói học phí về 0 cho cả danh mục. Tệ hơn ở Phase 4: `PUT /library/versions/:vid/score-set` gắn dưới `library.edit` — trong khi toàn bộ bề mặt score-set hiện tại (`POST/PUT/DELETE /api/v1/score-sets`, `POST /classes/:id/score-set`) là `KindOwnerOnly`. Phase 4 dựng một đường sửa bộ điểm thứ hai, gắn vào một khoá mà backfill phát cho tất cả mọi người.
- **Evidence:**
  - `apps/api/internal/shared/authctx/catalog.go:150-155` — `def` đặt `Grantable: true, DefaultGrant: true`
  - `apps/api/internal/shared/authctx/catalog.go:362-380` — `DefaultRoleKeys`: "every system role (and, via member grants, every role-less legacy stint)"
  - `apps/api/migrations/000013_center_rbac.up.sql:50-58` — vai hệ thống là `giao_vien`, `hoc_vu`, `tro_giang`
  - `apps/api/migrations/000022_task_board.up.sql:76-95` — backfill `WHERE cr.is_system`, không lọc vai
  - `apps/api/internal/shared/routespec/routespec.go:183-188` — `/api/v1/score-sets` POST/PUT/DELETE và `/classes/:id/score-set` đều `KindOwnerOnly`
  - Plan phase-04: "`log_fields` và `score_set` gắn ở phiên bản"; "Không key mới: dùng `library.read/edit`"
- **Suggested fix:** Tách theo độ nhạy. `library.read`, `courses.read`, `paths.read` dùng `def()`. Nhưng `library.edit`, `courses.edit`, `paths.edit`, `prep.edit` nên là `optIn()` — chúng chưa từng tồn tại nên không có hành vi cũ nào để giữ tương thích, đúng tinh thần `optIn` trong `docs/adding-permissions.md:142-151`. Riêng `score-set` của phiên bản: hoặc `KindOwnerOnly`, hoặc bỏ khỏi Phase 4 và trỏ về bề mặt score-set hiện có.

---

## Finding 6: Route kind `authenticated` không tồn tại; `GET /class-invitations` không có cổng nào ngoài code service chưa ai viết

- **Severity:** High
- **Location:** Phase 2, bảng API — ba dòng `authenticated`, `authenticated (self)`
- **Flaw:** `routespec.Kind` có đúng sáu giá trị và `authenticated` không nằm trong đó. Plan phát minh một kind mới, đồng nghĩa quyết định phân loại quan trọng nhất của ba route nhạy cảm bị bỏ ngỏ tới lúc code. Đồng thời plan giao cho `GET /class-invitations` một hành vi đa nhánh ("owner/manage → toàn trung tâm; người khác → chỉ lời mời của mình") mà không nói nhánh đó sống ở đâu.
- **Failure scenario:** Người thực thi chọn `KindSelf` cho gọn (plan viết "self"). `KindSelf` nghĩa là "no center permission involved" — lớp policy không kiểm gì cả. Nếu repository của `classinvites` quên mệnh đề `teacher_id = sc.TeacherID` ở nhánh không-manage, `GET /class-invitations?class_id=<bất kỳ>` trả về toàn bộ lời mời của trung tâm cho mọi thành viên: ai đang được mời dạy lớp nào, ai từ chối, `message` kèm theo. Không test nào bắt được vì `KindSelf` không có assertion policy. Tương tự `POST /class-invitations/:id/accept` không có tham số class trên URL — nếu service quên đối chiếu `teacher_id == caller`, đó là IDOR thuần: đoán id lời mời của người khác và nhận lớp thay họ.
- **Evidence:**
  - `apps/api/internal/shared/routespec/routespec.go:23-46` — sáu kind: `KindPublic`, `KindPublicToken`, `KindSelf`, `KindOwnerOnly`, `KindPermission`, `KindService`; `grep -rn "KindAuthenticated" apps/api/` không có kết quả
  - `apps/api/internal/shared/routespec/routespec.go:29-31` — `KindSelf`: "no center permission involved"
  - `apps/api/internal/shared/routespec/routespec.go:39-44` — `KindService` tồn tại đúng cho tình huống này: "no single catalog key names the allowed callers … the service's own gates decide, and fail closed"
  - `apps/api/internal/shared/routespec/routespec_test.go:39` `TestEveryKindIsValid`, `:53` `TestPermissionKindHasGrantableKeyOthersDoNot`
- **Suggested fix:** Ghi thẳng `KindService` cho `GET /class-invitations`, `accept`, `decline` trong plan, kèm câu "service fail closed: mặc định chỉ lời mời có `teacher_id == sc.TeacherID`; nhánh center-wide chỉ mở khi `sc.IsOwner || sc.Has(class_invites.manage)`". Thêm một dòng test bắt buộc vào Verification: member A không thấy và không accept được lời mời của member B (404, không phải 403).

---

## Finding 7: Chat nội bộ lớp đọc được bởi nhân sự đã rời lớp — read port cố ý giữ stint đã đóng

- **Severity:** High
- **Location:** Phase 7, "Key decisions" — "Chat = tin nhắn nội bộ bảng `class_messages` … **đọc cần đọc được lớp**"
- **Flaw:** "Đọc được lớp" trong repo này nghĩa là `classscope.ReadExists`, và fragment đó **cố ý không lọc `ended_at`**. Điều đó hợp lý với dữ liệu lịch sử (điểm danh, giáo án cũ) nhưng sai hoàn toàn với một kênh chat vẫn đang sống.
- **Failure scenario:** Giáo viên A dạy lớp X đến tháng 3, bị bàn giao đi (`handoff` đóng stint `giao_vien`, `ended_at` được đặt) và chuyển sang một trung tâm khác về mặt tổ chức nhưng vẫn là member. Tháng 9, lớp X trao đổi trong chat về tình hình từng học viên, học phí trễ, vấn đề gia đình. A gọi `GET /classes/X/messages` và đọc toàn bộ, kể cả tin nhắn viết sau khi A rời lớp. Không có log cảnh báo, vì đây là một GET hợp lệ theo đúng port đọc.
- **Evidence:**
  - `apps/api/internal/shared/classscope/classscope.go:25-34` — `ReadExists` chỉ có `cs.class_id`, `cs.teacher_id`, `cs.center_id`; **không** có `ended_at IS NULL`
  - `apps/api/internal/shared/classscope/classscope.go:36-41` — `WriteExists`: "ACTIVE stint (ended_at IS NULL — **ended stints keep reads**, never writes)"
  - `apps/api/internal/features/classes/repository.go:120-131` — `readScoped`, kèm chú thích "filtering to ACTIVE would strand departed-and-returned teachers off their own classes"
  - `apps/api/internal/features/classes/repository.go:30-33` — read port "own rows plus any class the caller holds a class_staff stint on (**ended included — history reads**)"
- **Suggested fix:** `class_messages` phải dùng một cổng riêng, chặt hơn read port: stint **đang hoạt động** (`ended_at IS NULL`) hoặc owner, theo khuôn `PhoneVisibleViaStudent`. Ghi cổng này vào Key decisions của Phase 7 và thêm test integration "stint đã đóng → 403/404 trên messages, nhưng vẫn 200 trên sessions".

---

## Finding 8: "Rời trung tâm → FK cascade xoá lời mời" là sai; lời mời pending sống sót sau khi bị gỡ khỏi trung tâm

- **Severity:** High
- **Location:** Phase 2, "Risks" — "Người được mời rời trung tâm → FK cascade xoá lời mời (chấp nhận)"
- **Flaw:** Rời trung tâm trong hệ thống này **không xoá** dòng `center_members`; nó đóng stint bằng `left_at = now()`. Không có DELETE nào xảy ra, nên `ON DELETE CASCADE` trên FK tới `center_members` không bao giờ kích hoạt cho tình huống này. Plan ghi nhận một biện pháp dọn dẹp không tồn tại và đánh dấu là "chấp nhận".
- **Failure scenario:** Owner mời B nhận lớp, rồi phát hiện vấn đề và gỡ B khỏi trung tâm (`DELETE /centers/me/members/:teacherId`). Dòng lời mời `status = 'pending'` vẫn nguyên, unique index `uq_class_invitations_pending` vẫn giữ chỗ cho nó. Nếu B sau đó được nhận lại (`repository.go:453` — `DO UPDATE SET left_at = NULL` tái kích hoạt stint cũ), lời mời cũ sống dậy và B bấm nhận lớp mà owner đã quên từ lâu. Ở chiều ngược lại, bảng "Lời mời nhận lớp" của owner hiển thị vĩnh viễn một lời mời gửi cho người không còn trong trung tâm.
- **Evidence:**
  - `apps/api/migrations/000007_centers.up.sql:66-74` — `center_members` PK `(teacher_id, center_id)`, cột `left_at`, `uq_center_members_active ON center_members(teacher_id) WHERE left_at IS NULL`
  - `apps/api/internal/features/centers/repository.go:481-482` — `UPDATE center_members SET left_at = now() WHERE teacher_id = @tid AND center_id = @cid AND left_at IS NULL`
  - `apps/api/internal/features/centers/repository.go:160` — "CloseMembership **stamps left_at** on the live stint"
  - `apps/api/internal/features/centers/repository.go:453` — `DO UPDATE SET left_at = NULL, joined_at = now(), role_id = EXCLUDED.role_id` (tái gia nhập dùng lại dòng cũ)
- **Suggested fix:** Thay giả định cascade bằng một cổng thật ở service accept: kiểm `center_members.left_at IS NULL` tại thời điểm chấp nhận (khuôn `handoff` đã có sẵn `MemberChecker.IsActiveMember`, `handoff/service.go:141-148`). Thêm bước huỷ lời mời pending trong luồng gỡ thành viên, và một test "gỡ member → accept trả 403/404".

---

## Finding 9: Phase 3–8 tạo ~40 route mutating nhưng không phase nào ngoài Phase 2 khai báo audit; route id phẳng không nêu cổng tenant

- **Severity:** Medium
- **Location:** Phase 3 "API — feature `internal/features/library/`"; Phase 4 "API (thêm vào `library`)"; Phase 5, 6, 8 mục API
- **Flaw:** Chỉ Phase 2 viết "Mọi route mutating: `req(action, "class_invitation", "id")` audit". Phase 3–8 liệt kê publish, archive, delete, reorder, generate, gán assignee, ghi đè tuition-packs, ghi đè score-set — tất cả đều là mutating — mà không nhắc audit một lần nào. Đồng thời các route dùng id phẳng (`PUT /library/lessons/:lid`, `POST /library/versions/:vid/publish`, `PATCH /prep/items/:id`, `GET/PUT /prep/items/:id/checklist`) không có `class_id`/`project_id` trên đường dẫn, nên toàn bộ cách ly tenant nằm ở repository, và plan chỉ mô tả cổng đọc ("read port `CenterWideFor` trong hàm `*read*`"), không mô tả cổng ghi.
- **Failure scenario:** (a) Một `library.edit` holder publish rồi archive các phiên bản chương trình, phá vỡ chương trình của nhiều lớp. Owner mở "Nhật ký hoạt động" và không thấy gì — không có dòng audit nào được sinh, vì Spec khai `none()`. Điều tra sự cố bất khả thi. Trên thực tế test sẽ chặn trước (`TestMutatingRoutesHaveAnAuditSource`), nên hậu quả trực tiếp là phase bị chặn giữa chừng và người thực thi sẽ bị cám dỗ khai `SourceNone` cho nhanh để test xanh. (b) `PATCH /prep/items/:id` với id của trung tâm khác: nếu repo không mang `center_id` vào mệnh đề WHERE, đó là IDOR ghi xuyên tenant.
- **Evidence:**
  - `apps/api/internal/shared/routespec/routespec_test.go:92` `TestMutatingRoutesHaveAnAuditSource`, `:109` `TestRequestSourceRoutesHaveAnAction`
  - `docs/adding-permissions.md:175-179` — "a mutating route with no audit source fails `TestMutatingRoutesHaveAnAuditSource` **unless it is on the documented `SourceNone` allowlist**"
  - `docs/adding-permissions.md:186-199` — §6 "Preserve data scope": "repository `center_id` predicates always isolate tenants. **Never accept request `center_id` or `teacher_id` as authorization context.**"
  - `apps/api/internal/features/classes/repository.go:140-147` — khuôn `writeScoped` mà các feature mới cần tương đương
  - Plan phase-03/04/05/06/08: không xuất hiện chuỗi "audit" ở bất kỳ mục API nào
- **Suggested fix:** Đưa lên `plan.md` thành quyết định xuyên phase: mỗi route mutating mới **phải** có `req(action, entity, idParam)`, và mỗi feature mới **phải** có cặp `readScoped`/`writeScoped` mang `center_id` — kèm danh sách action đặt tên trước cho từng phase. `SourceNone` cho một route mutating chỉ được dùng kèm lý do viết ra, đúng như Specs hiện hành làm với `/invitations/accept`.

---

## Finding 10: Bộ lọc `q` không escape metacharacter ILIKE và `tag` nội suy chuỗi vào JSONB; quy tắc scopelint bị trích dẫn sai

- **Severity:** Medium
- **Location:** Phase 1, "Implementation Steps" bước 7 (`classes/repository.go`)
- **Flaw:** Ba vấn đề trong một bước. (1) `Q` → `name ILIKE %q% OR code ILIKE %q%` không nhắc escape `%`, `_`, `\` — repo đã có khuôn escape đúng nhưng chỉ ở một nơi, hai nơi khác thì không, nên "theo pattern hiện có" là mơ hồ. (2) `Tag` → `tags @> '["x"]'` viết dưới dạng literal JSON có nội suy; repo **không có tiền lệ** `@>` nào để sao chép. (3) Plan khẳng định đặt tên `CountReadableByPhase` để "scopelint cho phép `CenterWideFor`" — sai: scopelint tách camelCase và so khớp **token** `read`, còn `Readable` là token khác.
- **Failure scenario:** (2) là vấn đề nặng nhất. Nếu người thực thi làm đúng như plan viết — ghép chuỗi `"tags @> '[\"" + tag + "\"]'"` — thì `?tag=x"],"y` thoát khỏi literal JSON và vào thẳng câu SQL: JSON/SQL injection trên một endpoint mà mọi thành viên có `classes.list` gọi được. (1) `?q=%` khiến ILIKE khớp toàn bộ, biến bộ lọc thành một scan bảng trên mỗi keystroke sau debounce 300ms. (3) khiến `make scopelint` báo `"CenterWideFor may only widen a read-named function"` và người thực thi sẽ đổi tên hàm cho lint xanh thay vì hiểu rằng phải đi qua helper `readScoped` — đúng loại "sửa cho lint im" mà scopelint sinh ra để ngăn.
- **Evidence:**
  - `apps/api/internal/features/enrollments/repository.go:406-412` — khuôn đúng: `esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)` + `ILIKE ? ESCAPE '\'`
  - `apps/api/internal/features/students/repository.go:185` và `contacts/repository.go:157` — **không** escape, cùng repo
  - `grep -rn "@>" apps/api/internal/features/*/repository.go` → không kết quả (không có tiền lệ JSONB containment)
  - `apps/api/tools/scopelint/scopelint/analyzer.go:427-439` — `hasReadToken` so khớp token, "A substring match would wrongly accept 'alreadyClosed' or 'spreadRows'; a token match does not"
  - `apps/api/tools/scopelint/scopelint/analyzer.go:489-490` — `CenterWideFor && !isRead` → báo lỗi
  - `apps/api/internal/features/classes/repository.go:124-131` — `readScoped` là helper hợp lệ duy nhất đang dùng
  - Plan phase-01 bước 7: "`Tag` → `tags @> '[\"x\"]'`"; "(tên hàm chứa `read` để `scopelint` cho phép `CenterWideFor`)"
- **Suggested fix:** Viết vào plan: `q` dùng khuôn escape của `enrollments.SearchEnrollableStudents`; `tag` dùng tham số bind — `Where("classes.tags @> ?::jsonb", string(jsonBytes))` với `jsonBytes` do `json.Marshal([]string{tag})` sinh, không bao giờ ghép chuỗi; và sửa ghi chú scopelint thành "gọi qua `readScoped`", bỏ mẹo đặt tên.

---

## Ghi chú bổ sung (không tính là finding)

**Xung đột cây route gin giữa Phase 3 và Phase 8.** Phase 3 đăng ký `POST /library/templates/:id/versions`; Phase 8 thêm `POST /library/templates/generate`. gin v1.12 chỉ chấp nhận static và wildcard cùng vị trí khi **static đăng ký trước**; ngược lại nó panic lúc khởi động. Xem `~/go/pkg/mod/github.com/gin-gonic/gin@v1.12.0/tree.go:210-234`. Phase 1 xử lý đúng chuyện này cho `/classes/stats` (đăng ký trước `/:id`), Phase 8 thì không có cảnh báo tương ứng vì file routes đã được viết ở Phase 3. Đây là sự cố khởi động toàn API, không chỉ một route.

---

## Verification Results

Vai trò FACT CHECKER — 24 claim được kiểm chứng trực tiếp.

| # | Claim trong plan | Kết quả | Bằng chứng |
|---|---|---|---|
| 1 | D1: `classes` có read/write port theo `class_staff` (`repository.go:19-60`) | VERIFIED | `apps/api/internal/features/classes/repository.go:30-43` (read port 30-33, write port 37-43); `ListFilter` ở 17-20 |
| 2 | Phase 2: `handoff.Service.Reassign` ở `handoff/service.go:110` | VERIFIED (nhưng thiếu ngữ cảnh) | `apps/api/internal/features/handoff/service.go:110`; cổng `!sc.IsOwner` ở dòng 111 mà plan không nhắc — xem Finding 2 |
| 3 | D2: bất biến `uq_class_staff_one_gv` | VERIFIED | `apps/api/migrations/000015_class_staff.up.sql:32` |
| 4 | D2: `classes.teacher_id NOT NULL` | VERIFIED | `apps/api/migrations/000001_baseline_schema.up.sql:80` |
| 5 | Phase 2 scout Q: "có `uq (id, center_id)` trên `classes` chưa? 000007" | VERIFIED — có | `apps/api/migrations/000007_centers.up.sql:165` `uq_classes_cid` |
| 6 | Phase 2: `center_members(center_id, teacher_id)` làm đích FK | VERIFIED (PK là `(teacher_id, center_id)`, PostgreSQL khớp theo tập cột) | `apps/api/migrations/000007_centers.up.sql:71` |
| 7 | Phase 2 risk: rời trung tâm → FK cascade xoá lời mời | **FAILED** | `apps/api/internal/features/centers/repository.go:481-482` đặt `left_at`, không DELETE — Finding 8 |
| 8 | Phase 2: route kind `authenticated` | **FAILED** — kind không tồn tại | `apps/api/internal/shared/routespec/routespec.go:23-46`; `grep KindAuthenticated` rỗng — Finding 6 |
| 9 | Phase 1: `GET /classes` hiện chỉ lọc `status` (`classes/handler.go:102-115`) | VERIFIED | `apps/api/internal/features/classes/handler.go:102` (`func list`), 108-119 (switch status) |
| 10 | Phase 1 bước 10: `authctx.PermClassesList` tồn tại | VERIFIED | `apps/api/internal/shared/authctx/catalog.go:61` |
| 11 | Phase 1 bước 10: thêm `GET /classes/stats` không cần bump `CatalogVersion` | VERIFIED | `catalog.go:325-334` — bump chỉ khi "alters what a stored assignment means"; route mới dùng khoá cũ |
| 12 | Phase 2: `CatalogVersion` hiện là 4, bump 4→5 | VERIFIED | `apps/api/internal/shared/authctx/catalog.go:334`; test ghim ở `catalog_test.go:317-318`; mirror web `apps/web/src/test/msw/handlers.ts:17` |
| 13 | Phase 1 bước 9: `/stats` đăng ký trước `/:id` để không bị nuốt | VERIFIED | `apps/api/internal/features/classes/routes.go:10` là GET `/:id` đầu tiên dưới `/classes`; gin v1.12 chấp nhận static-trước-wildcard (`tree.go:196-234`) |
| 14 | Phase 1: `CountReadableByPhase` có tên chứa `read` nên scopelint cho `CenterWideFor` | **FAILED** | `apps/api/tools/scopelint/scopelint/analyzer.go:427-439` khớp **token**; `Readable` ≠ `read` — Finding 10 |
| 15 | Phase 1 bước 3: `teaching.StringList` làm khuôn cho `dbtypes.StringList` | VERIFIED | `apps/api/internal/features/teaching/model.go:39-46` |
| 16 | Phase 3: backfill quyền "theo khuôn 000022" trên 2 bảng | VERIFIED | `apps/api/migrations/000022_task_board.up.sql:76-95` (role) và `:99-120` (member) |
| 17 | D5: `def()` → DefaultGrant, `optIn()` → không backfill | VERIFIED | `catalog.go:150-155` (`def`), `:160-165` (`optIn`), `:371-379` (`DefaultRoleKeys`) |
| 18 | Phase 7: `audit` list thêm filter, route `GET /audit?entity_type=…` | **FAILED** — đường dẫn API là `/api/v1/audit-logs`; `/audit` là route web | `apps/api/internal/features/audit/routes.go:8`; `apps/api/internal/shared/routespec/routespec.go:206`; nav web `apps/web/src/layouts/dashboard-layout.tsx:129` |
| 19 | Phase 7: audit list hiện chưa có filter entity | VERIFIED | `apps/api/internal/features/audit/handler.go:41-64` — chỉ `actor_id`, `action`, `from`, `to`, `cursor`, `limit` |
| 20 | Phase 7: `teaching.Service.PutCurriculum` tồn tại | VERIFIED | `apps/api/internal/features/teaching/service.go:93` |
| 21 | Phase 1: `ClassDialog`, `class-staff-section.tsx`, `class-config-page.tsx` tồn tại | VERIFIED | `apps/web/src/features/roster/components/class-dialog.tsx`, `…/class-staff-section.tsx`, `apps/web/src/features/center/pages/class-config-page.tsx` |
| 22 | Phase 1: `HvStateBlock`, `HvSegmented variant="tabs"`, `HvChip`, `HvSelect` (+`searchThreshold`), `HvConfirmDialog`, `useBoardUrlState`, `formatWeekday`, `formatScheduleLabel`, `tableHeadCellClassName`, `StatusPill`, `useEnrollmentsList`, `canWriteClass` | VERIFIED (12/12) | `apps/web/src/components/hv/hv-state-block.tsx`, `hv-segmented.tsx:88`, `hv-chip.tsx`, `hv-select.tsx` (`searchThreshold`), `hv-confirm-dialog.tsx`, `apps/web/src/features/tasks/hooks/use-board-url-state.ts`, `apps/web/src/features/roster/index.ts`, `apps/web/src/features/teaching/components/class-select.tsx`, `apps/web/src/features/roster/components/classes-tab.tsx`, `apps/web/src/components/hv/status-pill-labels.ts` |
| 23 | Phase 1: `OVERFLOW_LABELS` và nav có `perm` | VERIFIED | `apps/web/src/layouts/dashboard-layout.tsx:172-187`, `:45`, `:162` |
| 24 | Phase 8: `apps/web/src/lib/kanban` và `apps/api/pkg/kanban` tồn tại | VERIFIED | cả hai thư mục có thật |

Tóm tắt: **19 VERIFIED, 5 FAILED**, 0 UNVERIFIED. Các claim FAILED đều nằm ở tầng quyết định bảo mật (route kind, vòng đời membership, đường dẫn audit, quy tắc scopelint), không phải ở tầng đường dẫn file — plan đọc code rất kỹ nhưng dừng lại ở dòng trên cùng của mỗi cổng bảo vệ.

---

Status: DONE_WITH_CONCERNS
Summary: Bốn lỗ hổng Critical nằm ở Phase 2 (khoá `class_invites.manage` đi vòng qua cổng owner-only của `class_staff`/handoff, luồng accept mâu thuẫn với `Reassign` chặn `!IsOwner`, chuỗi tự mời vai `hoc_vu` để mở PII số điện thoại phụ huynh) và ở toàn bộ sườn migration Phase 4–8 (FK thiếu guard composite `center_id` → đọc chéo trung tâm). Bốn lỗi High: backfill `def()` phát quyền sửa giá học phí và bộ điểm cho mọi giáo viên, route kind `authenticated` không tồn tại, chat lớp đọc được bởi nhân sự đã rời lớp, và giả định cascade khi rời trung tâm là sai.
Concerns/Blockers: Phase 2 không nên thực thi trước khi quyết lại mô hình quyền của lời mời — ba trong bốn Critical đều ở đó. Quyết định D8 về FK composite `center_id` nên được thêm vào `plan.md` trước Phase 4.
