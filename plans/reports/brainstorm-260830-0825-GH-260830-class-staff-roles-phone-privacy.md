# Brainstorm — Class staff roles (GV / học vụ / trợ giảng) + phone privacy

Date: 2026-08-30. Status: contract COMPLETE — 4 rounds decisions answered, no unresolved questions. Feeds: /ak:plan.

## Contract

**Outcome**
- Mỗi class có staff theo role: `giao_vien`, `hoc_vu`, `tro_giang`. Gán qua **một bảng chung cho mọi role** (user decision), vocabulary role **mở rộng được** — sau này thêm role khác cũng assign vào class (user decision R2.4) → không CHECK cứng 3 giá trị, validate bằng code (cùng triết lý RBAC: DB lưu phép gán, code sở hữu danh mục).
- Không enforce cứng "đủ 3 role/lớp" — UI gợi ý/cảnh báo mềm.
- GV được gán (kể cả sau handoff) có **toàn quyền trên artifacts của lớp**: điểm danh, điểm, nhận xét, giáo án, ghi danh/kết thúc ghi danh. KHÔNG gồm sửa/xóa hồ sơ học sinh (học sinh học chéo nhiều lớp).
- **Write đi theo assignment active; GV cũ giữ quyền ĐỌC lịch sử lớp cũ** (user decision R4.1). Cơ chế đề xuất: `class_staff` soft-close (`ended_at`) — assignment đã đóng cấp read-only trên dữ liệu lớp, assignment active cấp write theo capability map. Quy tắc này áp chung cho mọi role bị gỡ khỏi lớp. Handoff chỉ owner thực hiện, trong phạm vi 1 center.
- SĐT người liên hệ là bảo mật của trung tâm: chỉ **owner + học vụ được gán vào lớp** thấy. GV, trợ giảng, member thường không thấy — kể cả member từng tạo contact.
- **Owner sở hữu toàn bộ dữ liệu gốc** (user decision R3): chỉ owner tạo/sửa contact VÀ học sinh. Migration chuyển hết contact hiện có về owner own. Mô hình vận hành: owner tạo contact + học sinh + lớp, rồi handoff/gán staff cho teacher; GV chỉ thao tác artifacts của lớp (trong đó có ghi danh học sinh sẵn có).
- Học vụ (per class được gán): **thuần đọc** — nhận xét, điểm, điểm danh, học phí (user decision R3, đảo R2.2 "được ghi điểm danh" do nhầm); hành động duy nhất: **gửi** thông tin đó cho người liên hệ.
- Trợ giảng v1: xem roster (không SĐT) + xem/ghi điểm danh lớp được gán, **ghi thẳng không cần duyệt** (user decision R2.5). Nhiệm vụ đầy đủ định nghĩa sau.

**Constraints**
- Xây trên RBAC hiện có (`000013_center_rbac`): 3 system role đã seed per-center, permission keys code-owned (`authctx/permissions.go`), owner là implicit superuser. Không tạo hệ role song song.
- Migrations append-only; `classes.teacher_id` là trục scoping hiện tại của attendance/enrollments/students/statements + handoff → chuyển pha, không big-bang.
- Row-level `teacher_id` (enrollments/students/contacts) giữ vai trò creator anchor + composite FK — không rewrite ownership dữ liệu cũ.
- Giữ discipline: repository scope qua helper, DTO mask field, web gate UI bằng effective permission keys từ API.
- Handoff: owner-only, target là teacher trong cùng center (giữ hiện trạng).

**Non-goals (v1)**
- Nhiệm vụ đầy đủ của trợ giảng (TBD); luồng duyệt điểm danh (đã quyết: không có).
- Custom role CRUD UI (vocabulary mở rộng bằng code, chưa cần UI tạo role).
- Đồng giảng dạy nhiều GV chính (schema cho phép, không thiết kế phân công chi tiết).
- Thay đổi owner-superuser model, per-student assignment.

**Acceptance criteria (observable)**
1. Owner gán/gỡ staff per class qua API + UI; role_key ngoài danh mục code bị từ chối; lớp thiếu role chỉ cảnh báo mềm.
2. Handoff = đóng assignment `giao_vien` cũ (`ended_at`) + mở assignment mới (owner-only, cùng center, 1 tx); GV mới ghi được điểm danh/điểm/nhận xét/giáo án/ghi danh; GV cũ chỉ còn ĐỌC lịch sử lớp (mọi write → 403/404).
3. GV & trợ giảng: mọi response + UI roster/attendance KHÔNG chứa `contact_phone`; học vụ được gán + owner thấy đủ. Integration test per role.
4. Contact + student create/update/delete: member gọi → 403; owner → OK. Test cả import path. Migration xong: mọi contact (và student anchor) thuộc owner của center.
5. Học vụ: đọc điểm danh/điểm/nhận xét/statement lớp được gán (mọi write → 403), gửi báo cáo cho contact lớp đó; lớp không gán → 404/empty.
6. Trợ giảng: đọc roster + read/write điểm danh lớp gán, không cần duyệt; lớp khác → 404/empty.
7. Peer cùng center không gán: không thấy gì.

## Current evidence

- `center_roles` seed `giao_vien/hoc_vu/tro_giang` per center; `center_member_permissions` grant/deny; perm keys code-owned.
- `classes.teacher_id` = 1 GV; handoff move teacher_id (+schedules, future sessions), owner-only. Features phụ thuộc: attendance, classes, enrollments, students (readScoped 260830).
- Fix 260830 cho GV nhận lớp thấy `contact_phone` qua students Row → phần phone bị đảo bởi contract này (widening đọc giữ, phone mask).
- Statements/notifications/imports đọc phone server-side để gửi — gửi không đòi caller thấy phone.
- Contacts hiện tại: member tự tạo/sở hữu danh bạ (`contacts.teacher_id`), students.Create check contact thuộc caller.

## Chosen direction

**Bảng `class_staff` chung, vocabulary code-owned:**

```sql
CREATE TABLE class_staff (
  id         UUID PRIMARY KEY,
  class_id   UUID NOT NULL REFERENCES classes(id) ON DELETE CASCADE,
  teacher_id UUID NOT NULL,
  center_id  UUID NOT NULL,
  role_key   VARCHAR(32) NOT NULL,          -- validate trong code, mở rộng được
  started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  ended_at   TIMESTAMPTZ,                   -- soft-close: NULL = active; đã đóng = read-only lịch sử
  FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id)
);
-- Partial unique: 1 người tối đa 1 assignment ACTIVE / lớp
CREATE UNIQUE INDEX ON class_staff (class_id, teacher_id) WHERE ended_at IS NULL;
-- Backfill: mỗi class sống → (class_id, classes.teacher_id, 'giao_vien', active)
```

- Scoping: helper theo assignment — READ match mọi assignment (kể cả đã đóng: lịch sử), WRITE đòi assignment active + role_key nằm trong capability map: `EXISTS (SELECT 1 FROM class_staff WHERE class_id = X AND teacher_id = $self [AND ended_at IS NULL AND role_key = ANY(...)])`. Capability→roles map sống trong code (vd `attendance.write: [giao_vien, tro_giang]`), thêm role mới = sửa map.
- Phone: mask ở DTO (students/attendance/contacts/statements) trừ owner hoặc học vụ của lớp liên quan; web ẩn cột theo effective keys.
- Contact CRUD: owner-only (API 403 cho member; UI ẩn nút). Contact rows member đã tạo giữ nguyên, chỉ đường ghi mới bị khóa.
- Handoff: rewrite thành staff reassignment trong 1 tx; giữ owner-only + same-center.

**Capability matrix v1 (code-owned map)**

| Capability | GV | Học vụ | Trợ giảng | Owner |
|---|---|---|---|---|
| Roster read (không phone) | ✓ | ✓ | ✓ | ✓ (full) |
| Contact phone | ✗ | ✓ | ✗ | ✓ |
| Contact CRUD | ✗ | ✗ | ✗ | ✓ |
| Student CRUD (hồ sơ) | ✗ | ✗ | ✗ | ✓ |
| Điểm danh write | ✓ | ✗ (read) | ✓ (không duyệt) | ✓ |
| Điểm, nhận xét write | ✓ | ✗ (read) | ✗ | ✓ |
| Giáo án | ✓ | ✗ | ✗ | ✓ |
| Ghi danh lớp (student sẵn có) | ✓ | ✗ (read) | ✗ | ✓ |
| Học phí/statement read + gửi | ✗ | ✓ | ✗ | ✓ |

**Phasing (mỗi phase shippable)**
- **A — Schema + quản lý staff**: migration `class_staff` + backfill; API/UI owner gán-gỡ; dual-write handoff (sync `classes.teacher_id` ↔ assignment `giao_vien`).
- **B — Read scoping theo assignment**: thay check `classes.teacher_id = $self` bằng assignment check; roster cho học vụ/trợ giảng; điểm danh read cho cả 3 role.
- **C — Phone privacy + data ownership**: mask DTO + contacts endpoints; contact + student CRUD owner-only; migration chuyển `teacher_id` anchor của contacts VÀ students hiện có về owner center (user decision R4.2); import contact/student/class → owner-only (user decision R4.3); UI ẩn cột/nút; luồng owner tạo contact→student→gán lớp; GV ghi danh qua picker student toàn center (tên, không phone).
- **D — Writes theo capability map**: GV full class-artifact writes; trợ giảng ghi điểm danh; học vụ statements read/send per class (thuần đọc + gửi); reconcile perm `reports.send` center-wide → per-class.
- **E — Cleanup**: gỡ scoping cũ theo `classes.teacher_id` khi mọi reader đã chuyển.

**Luồng nhập liệu (đã chốt R3+R4):** owner tạo contact → student → lớp → gán staff/handoff. GV ghi danh học sinh sẵn có vào lớp mình qua picker tìm theo tên toàn center (không phone). Member không còn đường tạo contact/student/class nào — import các loại này owner-only (R4.3); phần import khác nếu có (điểm danh/điểm) theo capability map của lớp.

## Risks

- Blast radius lớn: ~6 feature + e2e + seeds + imports; phasing A→E bắt buộc.
- Đường nhập liệu hiện có của member (contact, student, import) bị siết về owner — thay đổi UX đáng kể, cần thông báo/migration UX rõ.
- `reports.send` (center-wide, delegated) vs học vụ per-class send: chọn per-class thay thế hay cộng gộp — quyết ở plan phase D.
- Dashboard/imports/notifications đang giả định GV-owns-rows; audit ở phase D.
- Dual-write A→E phải giữ bất biến `classes.teacher_id` = đúng 1 assignment `giao_vien`... nếu cho nhiều GV/lớp trước phase E thì cột không biểu diễn được — khóa "1 GV chính/lớp" cho tới E.

## Decision log

- R1: quyền GV = artifacts lớp; phone chỉ owner + học vụ; bảng gán chung; trợ giảng v1 roster + điểm danh.
- R2: contact create owner-only; handoff owner-only cùng center; role vocabulary mở rộng bằng code; trợ giảng ghi không cần duyệt.
- R3: học vụ THUẦN ĐỌC (đảo R2 "học vụ ghi điểm danh" — user nhầm); owner tạo hết contact + student, migration chuyển contact về owner.
- R4: GV cũ giữ quyền đọc lịch sử lớp cũ (soft-close assignment); students migrate anchor về owner cùng contacts; import contact/student/class owner-only.

## Unresolved questions

Không còn — contract đủ để plan. Chi tiết cơ chế (soft-close vs own-rows cho lịch sử, phạm vi picker ghi danh, loại import giữ cho member) chốt trong plan theo hướng đã ghi ở trên.
