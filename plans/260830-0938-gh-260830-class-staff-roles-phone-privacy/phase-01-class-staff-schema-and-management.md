---
phase: 1
title: "Schema class_staff + quản lý staff + handoff dual-write"
status: completed
priority: P1
effort: "2d"
dependencies: []
---

# Phase 1: Schema `class_staff` + quản lý staff + handoff dual-write

## Overview

Tạo bảng `class_staff` (migration `000015`), backfill assignment `giao_vien`
từ `classes.teacher_id`, feature package `classstaff` cho owner gán/gỡ học vụ
và trợ giảng, dual-write trong handoff, và nối `class_staff` vào 3 vòng đời
sẵn có: tạo lớp, đóng/mở membership, xoá cứng teacher/center. Sau phase này
CHƯA đổi hành vi đọc/ghi nào — assignment tồn tại song song với scoping cũ.

## Requirements

- Functional: owner CRUD staff assignment (trừ `giao_vien` — chỉ qua handoff,
  quyết định D2 của plan.md); handoff đóng/mở assignment `giao_vien` trong
  cùng tx với `ReassignTeacher`; role_key ngoài danh mục → 422; TẠO LỚP MỚI
  (mọi đường: API + import) sinh assignment `giao_vien` trong cùng tx;
  đóng membership đóng mọi assignment active của member đó.
- Non-functional: migration idempotent-backfill, down sạch; bất biến 1 GV
  active/lớp enforce bằng DB index; không đổi response nào hiện có; bất biến
  drift tự chữa được (không có trạng thái 500 vĩnh viễn).

## Architecture

**Migration `000015_class_staff`** (mới nhất hiện tại là `000014_grading`;
phase-04 của plan RBAC ghi 000014 đã stale — xem plan.md Cross-plan):

```sql
CREATE TABLE class_staff (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    class_id   UUID NOT NULL,
    center_id  UUID NOT NULL,
    teacher_id UUID NOT NULL,
    role_key   VARCHAR(32) NOT NULL,       -- validate trong code (authctx), không CHECK cứng
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at   TIMESTAMPTZ,                -- NULL = active; NOT NULL = soft-close, còn quyền đọc lịch sử
    FOREIGN KEY (class_id, center_id)  REFERENCES classes(id, center_id) ON DELETE CASCADE,
    -- CASCADE bắt buộc: xoá cứng teacher/center (PII) chạy 1 tx qua center_members
    -- (pattern 16 bảng ở 000007); NO ACTION sẽ phá tx đó.
    FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE
);
-- 1 người tối đa 1 assignment active / lớp
CREATE UNIQUE INDEX uq_class_staff_active ON class_staff (class_id, teacher_id) WHERE ended_at IS NULL;
-- Bất biến dual-write: đúng 1 GV chính active / lớp (gỡ khi thiết kế multi-GV sau P5)
CREATE UNIQUE INDEX uq_class_staff_one_gv ON class_staff (class_id) WHERE ended_at IS NULL AND role_key = 'giao_vien';
CREATE INDEX idx_class_staff_teacher ON class_staff (teacher_id, center_id);
CREATE INDEX idx_class_staff_class ON class_staff (class_id);

-- Backfill: mỗi class SỐNG hiện có → 1 assignment giao_vien active.
-- Lọc deleted_at (classes là soft-delete — cascade FK không bao giờ kích hoạt,
-- lớp đã xoá không được chiếm uq_class_staff_one_gv). Idempotent để P5 chạy
-- lại được như lệnh reconcile.
INSERT INTO class_staff (class_id, center_id, teacher_id, role_key)
SELECT c.id, c.center_id, c.teacher_id, 'giao_vien'
FROM classes c WHERE c.deleted_at IS NULL
ON CONFLICT (class_id, teacher_id) WHERE ended_at IS NULL DO NOTHING;
```

Composite FK theo pattern 000007 (`classes` đã có `UNIQUE (id, center_id)`;
`center_members` PK `(teacher_id, center_id)` — owner LUÔN có row membership,
backfill 000007 + đăng ký insert cùng tx, đã verify).

**Vocabulary + capability map** — `apps/api/internal/shared/authctx/class_staff.go`:

```go
// Trục độc lập với center_roles (role trung tâm ≠ vai trong lớp), cùng chuỗi key.
const (
    StaffRoleGiaoVien = "giao_vien"
    StaffRoleHocVu    = "hoc_vu"
    StaffRoleTroGiang = "tro_giang"
)
func ValidStaffRole(key string) bool

// Capability → roles được GHI (đọc không cần map — mọi assignment đều đọc được).
// Dùng từ Phase 3 (enrollment create) và Phase 4; định nghĩa ngay để
// vocabulary sống một chỗ. Phase 4 bổ sung thêm CapSessionsWrite
// ("sessions.write", giao_vien) khi chuyển sessions lifecycle sang map.
type ClassCapability string
const (
    CapAttendanceWrite ClassCapability = "attendance.write" // giao_vien, tro_giang
    CapScoresWrite     ClassCapability = "scores.write"     // giao_vien
    CapRemarksWrite    ClassCapability = "remarks.write"    // giao_vien (teaching: session notes/marks)
    CapLessonPlanWrite ClassCapability = "lesson_plan.write"// giao_vien (teaching: lesson plans)
    CapEnrollmentWrite ClassCapability = "enrollment.write" // giao_vien
    CapStatementSend   ClassCapability = "statement.send"   // hoc_vu
)
func StaffRolesFor(cap ClassCapability) []string
```

**Vòng đời assignment — 4 hook bắt buộc (red-team F2, F7):**

1. **Tạo lớp**: `classes.Service.Create` (service.go:35–71, tx
   `CreateWithSchedules`) thêm INSERT assignment `giao_vien` cho
   `sc.TeacherID` trong CÙNG `WithinTx`. Import tạo lớp qua chính service này
   (`imports/apply.go:136`) nên được cover tự động — thêm assert vào test
   import. Lưu ý `POST /classes` hiện không có perm gate (routes.go:9) — giữ
   nguyên hành vi, chỉ đảm bảo creator có assignment.
2. **Handoff** — `features/handoff/service.go Reassign`: trong cùng
   `WithinTx`: đóng (`ended_at = now()`) MỌI assignment `giao_vien` active của
   lớp (kể cả khi ≠ 1 row — drift), insert assignment mới cho teacher đích
   (idempotent, ON CONFLICT bỏ qua nếu đã đúng). KHÔNG trả 500 khi drift —
   handoff là đường TỰ CHỮA: nguồn chân lý trong cửa sổ dual-write là
   `classes.teacher_id`. Nhánh no-op (`newTeacherID == class.TeacherID`,
   service.go:111–116, hiện early-return TRƯỚC tx) phải đi vào tx và chạy cùng
   bước sync assignment — owner "handoff về chính GV hiện tại" trở thành lệnh
   repair hợp lệ. `classstaff` expose repo method cho handoff qua interface
   nhỏ (pattern `ClassReassigner`/`SessionReassigner` sẵn có).
3. **Đóng membership**: `centers/repository.go CloseMembership` (373–394) hiện
   xoá `center_member_permissions` cùng statement ("no code path can resurrect
   them from stale rows"). Thêm cùng tx: soft-close MỌI `class_staff` active
   của (teacher, center). `OpenMembership` (343–366, `DO UPDATE SET left_at =
   NULL`) defensive-close lần nữa. Nếu không đóng, kick → mời lại tự hồi sinh
   full write lớp cũ.
4. **Xoá cứng teacher/center**: FK CASCADE ở trên. Mở rộng
   `TestTeacherHardDeleteInOneTransaction` (migrations_test.go:633–660) seed
   thêm 1 row `class_staff` — không seed thì test vô nghĩa với bảng mới.

Lớp soft-delete: KHÔNG đóng assignment (giữ lịch sử); mọi đường đọc P2 lọc
`classes.deleted_at IS NULL` bên trong fragment (xem phase-02) nên assignment
của lớp đã xoá không cấp quyền gì.

**Feature package `apps/api/internal/features/classstaff/`** (model, dto,
repository, service, handler, routes, integration_test — theo skeleton các
feature hiện có):

- `GET  /api/v1/classes/:id/staff` — owner hoặc caller có assignment (kể cả
  ended) trên lớp; trả active + ended (flag `ended_at`), kèm tên teacher.
- `POST /api/v1/classes/:id/staff {teacher_id, role_key}` — owner-only;
  422 role ngoài danh mục; 409 khi (a) đã có assignment active của người đó
  trên lớp, (b) `role_key = giao_vien` (message trỏ sang handoff);
  target phải là member SỐNG (`left_at IS NULL`) của center (400 nếu không).
- `DELETE /api/v1/classes/:id/staff/:staffId` — owner-only. Hai chế độ:
  - mặc định: soft-close (`ended_at = now()`) — người bị gỡ GIỮ quyền đọc
    lịch sử (R4.1);
  - `?mode=void`: HARD DELETE row — đường thu hồi cho gán nhầm (red-team F12:
    soft-close + luật đọc "mọi assignment" nghĩa là DELETE thường không thu
    hồi gì). Ghi audit event (feature `audit` sẵn có) cả hai chế độ.
  - 409 nếu là `giao_vien` active (phải handoff); trên assignment đã ended →
    404.

**Contract mã lỗi (kéo từ P4 lên — acceptance 7):** caller không có quyền đọc
lớp (không owner, không assignment) → **404** cho MỌI verb (không leak tồn
tại lớp qua 403). Caller đọc được lớp nhưng không phải owner → **403** cho
POST/DELETE. Test đủ cả hai nhánh.

Repository tuân `scoped` discipline: mọi query filter `center_id`; guard
`scoping_guard_test.go` không được vi phạm (không `IsOwner` trong repo — owner
gate ở service như `grading`).

**Web (owner)** — trang class settings hiện có: section "Nhân sự lớp": list
staff theo role, thêm/gỡ học vụ + trợ giảng (member picker từ
`/centers/me/members`, chỉ member sống), GV chính hiển thị read-only + link
sang dialog handoff sẵn có. Gỡ có confirm 2 lựa chọn (kết thúc vai / gán nhầm
— void). Cảnh báo mềm (badge) khi lớp thiếu 1 trong 3 role. Gate hiển thị:
`useCenterContext().has()` — section chỉ owner (theo pattern owner-gate sidebar).

## Related Code Files

- Create: `apps/api/migrations/000015_class_staff.{up,down}.sql`
- Create: `apps/api/internal/shared/authctx/class_staff.go` (+ test)
- Create: `apps/api/internal/features/classstaff/{model,dto,repository,service,handler,routes,errors}.go` + `integration_test.go`
- Modify: `apps/api/internal/features/classes/service.go` (create-hook assignment cùng tx)
- Modify: `apps/api/internal/features/handoff/service.go` (dual-write tx + no-op vào tx + self-heal drift)
- Modify: `apps/api/internal/features/centers/repository.go` (Close/OpenMembership đóng assignment)
- Modify: `apps/api/migrations/migrations_test.go` (hard-delete test seed class_staff)
- Modify: `apps/api/cmd/api/container` wiring (theo chỗ các feature khác đăng ký routes)
- Modify: `apps/api/internal/testutil/fixtures.go` (fixture staff assignment)
- Modify: `apps/web/src/features/roster` class settings page + api client mới
  `class-staff-api.ts`, hook `use-class-staff.ts`, component
  `class-staff-section.tsx`
- Modify: `apps/api/seeds/seed.go` — seed 1 học vụ + 1 trợ giảng gán vào lớp
  seed (để e2e các phase sau dùng)

## Implementation Steps

1. Migration 000015 + test migration up/down (backfill lọc deleted, idempotent,
   FK cascade; mở rộng hard-delete tx test).
2. `authctx/class_staff.go` vocabulary + capability map + unit test.
3. Create-hook trong `classes.Service.Create` + test "tạo lớp (API + import) →
   đúng 1 assignment active".
4. Feature package `classstaff` (repo → service → handler → routes, wire
   container), integration tests: owner CRUD, non-owner-có-đọc 403,
   không-đọc-được 404, role lạ 422, GV 409, cross-center 404, void hard-delete.
5. Handoff dual-write + self-heal: sau reassign đúng 1 GV active = teacher mới,
   assignment cũ ended; no-op path sửa được lớp drift (xoá tay row rồi handoff
   về chính GV → assignment tái sinh); rollback tx khi insert fail.
6. Close/OpenMembership đóng assignment + test kick → mời lại → không còn
   assignment active (write lớp cũ sẽ 403 từ P4; ở P1 assert trạng thái DB).
7. Seeds + fixtures.
8. Web staff section + vitest cho component/hook.
9. `make test-api`, web vitest, swagger regen (`make api-docs`) nếu annotate.

## Success Criteria

- [x] Migration up/down sạch trên DB có dữ liệu; backfill = số class sống;
      chạy lại backfill không đổi gì (idempotent).
- [x] Tạo lớp mới (API + import) có đúng 1 assignment `giao_vien` active.
- [x] Owner gán/gỡ học vụ + trợ giảng qua API + UI; 403/404/409/422 + void
      test đủ; non-assigned member nhận 404 (không leak).
- [x] Handoff giữ bất biến 1 GV active (test tx + index DB chặn race) và tự
      chữa drift qua no-op path.
- [x] Kick member → mọi assignment active của họ đóng; mời lại không hồi sinh.
- [x] Hard-delete teacher/center 1 tx vẫn chạy (test seed class_staff).
- [x] Không response/hành vi hiện có nào đổi (regression suite xanh).

## Risk Assessment

- **Race 2 handoff song song** → partial unique `uq_class_staff_one_gv` biến
  race thành lỗi tx (retry-able), không silent corrupt.
- **Drift dual-write** (nguồn nào đó insert fail nửa chừng) → handoff no-op =
  lệnh repair; P5 parity query đo và reconcile bằng chính backfill idempotent.
- **Migration số 000015 đụng plan RBAC phase-04** → đã ghi nhận ở plan.md;
  RBAC cleanup renumber khi chạy.
- **Void xoá nhầm assignment có lịch sử thật** → confirm dialog 2 bước trên
  web + audit event giữ vết; void không đụng dữ liệu lớp, chỉ quyền đọc.
