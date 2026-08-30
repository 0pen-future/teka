# Brainstorm: Cấu hình điểm lớp học (owner) + nhập điểm thành phần

## Contract

**Outcome:** Owner có menu mới trong nhóm sidebar "Trung tâm" để cấu hình lớp
học. Phase này: owner tạo/sửa/xóa các "bộ điểm" đặt tên (VD "IELTS":
Listening, Speaking, Reading, Writing), gán bộ cho lớp bất kỳ; giáo viên của
lớp (và owner) nhập điểm 0–10 cho từng học sinh, theo từng thành phần, trong
từng buổi học.

**Constraints:**
- Bộ điểm là template cấp trung tâm; gán cho lớp theo ngữ nghĩa **snapshot**
  (copy thành phần vào lớp lúc gán — sửa bộ gốc không ảnh hưởng lớp đã gán).
- Thành phần điểm chỉ có tên + thứ tự; thang điểm cố định 0–10 (khớp
  `SessionMark.Score` NUMERIC(4,1) hiện có).
- Mô hình điểm (chốt lại theo review): điểm theo **từng buổi học** — 1 điểm /
  thành phần / học sinh / buổi học / lớp, sửa đè được. Không có entity "bài
  kiểm tra" riêng; buổi học (session) là đơn vị chấm.
- Quyền: cấu hình bộ điểm + gán lớp = owner-only (theo pattern các mục
  owner-only trong nhóm Trung tâm). Nhập điểm = giáo viên sở hữu lớp + owner
  (theo scoping teacher-own-rows / owner center-wide hiện có).
- RBAC key mới (nếu cần) phải vào registry `authctx/permissions.go`; tuân thủ
  scoping guard test hiện có.

**Non-goals:**
- Hệ số / trọng số, điểm tối đa tùy chỉnh, điểm trung bình tự động.
- Nhiều đợt kiểm tra (giữa kỳ/cuối kỳ) per lớp.
- Đưa điểm thành phần vào báo cáo Zalo/phụ huynh.
- Ngữ nghĩa reference (auto-propagate khi sửa bộ) — đã chọn snapshot.

**Acceptance criteria:**
1. Sidebar nhóm "Trung tâm" có entry mới (owner-only, ẩn với member) mở trang
   cấu hình lớp học.
2. Owner CRUD được bộ điểm (tên bộ, danh sách thành phần có thứ tự); tên
   thành phần trong 1 bộ không trùng.
3. Owner gán 1 bộ cho 1 lớp bất kỳ; sau gán, sửa/xóa bộ gốc không đổi thành
   phần của lớp.
4. Giáo viên của lớp và owner nhập/sửa điểm 0–10 (1 chữ số thập phân) từng
   học sinh × thành phần × buổi học; member không phải giáo viên lớp bị 403.
5. Lớp đã có ≥1 điểm nhập thì API từ chối gán bộ điểm khác (kèm thông báo
   rõ ràng trên UI).
6. Migration mới (000014+) up/down sạch; test integration cho repo + handler
   theo pattern feature hiện có; web test cho page/tab mới.

## Hướng đã chọn

- **API:** feature mới `apps/api/internal/features/grading/` (model, repo,
  service, handler, routes) — không nhét vào `classes` hay `teaching` vì đây
  là boundary mới (config trung tâm + điểm học sinh).
- **Schema (snapshot 2 tầng):**
  - `score_sets` (id, center_id, name, timestamps, soft delete)
  - `score_set_components` (id, set_id, name, position)
  - `class_score_components` (id, class_id, name, position, source_set_id
    nullable) — bản copy lúc gán
  - `student_scores` (id, class_id, session_id, class_score_component_id,
    student_id, score NUMERIC(4,1), timestamps;
    unique (session_id, class_score_component_id, student_id))
- **Endpoints:** owner CRUD `/score-sets`; `POST /classes/:id/score-set`
  (apply = replace snapshot); GET/PUT điểm theo lớp cho teacher+owner.
- **Web:** page mới trong feature `center` (hoặc feature `grading` mới) cho
  cấu hình + gán; nhập điểm gắn theo buổi học trong classbook (vị trí chính
  xác — session detail panel hay bảng riêng — chốt ở bước plan).

## Quyết định đã chốt (review 260829)

1. **Re-apply khi đã có điểm: CHẶN.** API trả lỗi, UI không cho gán bộ khác
   khi lớp đã có điểm nhập. Không có flow xác nhận/ghi đè trong phase này.
2. **Điểm theo buổi học:** key = (class, session, student, component). Không
   dùng enrollment_id; student_id + session đã định danh đủ.
3. **Owner gate = `Scope.IsOwner`**, không thêm perm key mới.

## Rủi ro cho bước plan

- Quan hệ với `SessionMark.Score` (điểm đơn 0–10 per buổi hiện có): điểm
  thành phần sống cạnh hay thay thế trong UI buổi học — plan cần soi
  session-detail-panel để chọn chỗ đặt UI nhập điểm (theo buổi, không phải
  tab bảng điểm tổng kết như phác thảo ban đầu).
- "Lớp đã có điểm" định nghĩa = tồn tại ≥1 dòng `student_scores` của lớp
  (kể cả của buổi đã qua).

## Handoff

→ `/ak:plan` (rồi `/ak:cook`) với contract trên. Evidence:
`dashboard-layout.tsx:87` (nhóm Trung tâm), `classes/model.go:27`,
`teaching/model.go:142` (SessionMark 0–10), `authctx/permissions.go`,
migration mới nhất `000013_center_rbac`.
