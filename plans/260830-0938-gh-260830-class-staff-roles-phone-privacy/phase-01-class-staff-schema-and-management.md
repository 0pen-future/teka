---
phase: 1
title: "Schema class_staff + quản lý staff + handoff dual-write"
status: pending
priority: P1
effort: "1.5d"
dependencies: []
---

# Phase 1: Schema `class_staff` + quản lý staff + handoff dual-write

## Overview

Tạo bảng `class_staff` (migration `000015`), backfill assignment `giao_vien`
từ `classes.teacher_id`, feature package `classstaff` cho owner gán/gỡ học vụ
và trợ giảng, và dual-write trong handoff. Sau phase này CHƯA đổi hành vi đọc/
ghi nào — assignment tồn tại song song với scoping cũ.

## Requirements

- Functional: owner CRUD staff assignment (trừ `giao_vien` — chỉ qua handoff,
  quyết định D2 của plan.md); handoff đóng/mở assignment `giao_vien` trong
  cùng tx với `ReassignTeacher`; role_key ngoài danh mục → 422.
- Non-functional: migration idempotent-backfill, down sạch; bất biến 1 GV
  active/lớp enforce bằng DB index; không đổi response nào hiện có.

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
    FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id)
);
-- 1 người tối đa 1 assignment active / lớp
CREATE UNIQUE INDEX uq_class_staff_active ON class_staff (class_id, teacher_id) WHERE ended_at IS NULL;
-- Bất biến dual-write: đúng 1 GV chính active / lớp (gỡ khi thiết kế multi-GV sau P5)
CREATE UNIQUE INDEX uq_class_staff_one_gv ON class_staff (class_id) WHERE ended_at IS NULL AND role_key = 'giao_vien';
CREATE INDEX idx_class_staff_teacher ON class_staff (teacher_id, center_id);
CREATE INDEX idx_class_staff_class ON class_staff (class_id);

-- Backfill: mỗi class hiện có → 1 assignment giao_vien active cho classes.teacher_id
INSERT INTO class_staff (class_id, center_id, teacher_id, role_key)
SELECT c.id, c.center_id, c.teacher_id, 'giao_vien' FROM classes c;
```

Composite FK theo pattern 000007 (`classes` đã có `UNIQUE (id, center_id)`;
`center_members` PK `(teacher_id, center_id)` — owner LUÔN có row membership,
backfill 000007 + đăng ký insert cùng tx, đã verify). Member rời center
(`left_at`) không chặn FK vì FK trỏ row membership lịch sử, khớp triết lý
"dữ liệu ở lại center".

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
// Dùng từ Phase 4; định nghĩa ngay để vocabulary sống một chỗ.
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

**Feature package `apps/api/internal/features/classstaff/`** (model, dto,
repository, service, handler, routes, integration_test — theo skeleton các
feature hiện có):

- `GET  /api/v1/classes/:id/staff` — owner hoặc caller có assignment (kể cả
  ended) trên lớp; trả active + ended (flag `ended_at`), kèm tên teacher.
- `POST /api/v1/classes/:id/staff {teacher_id, role_key}` — owner-only;
  422 role ngoài danh mục; 409 khi (a) đã có assignment active của người đó
  trên lớp, (b) `role_key = giao_vien` (message trỏ sang handoff);
  target phải là member sống của center (400 nếu không).
- `DELETE /api/v1/classes/:id/staff/:staffId` — owner-only; soft-close
  (`ended_at = now()`); 409 nếu là `giao_vien` active (phải handoff);
  idempotent trên assignment đã ended → 404.

Repository tuân `scoped` discipline: mọi query filter `center_id`; guard
`scoping_guard_test.go` không được vi phạm (không `IsOwner` trong repo — owner
gate ở service như `grading`).

**Handoff dual-write** — `features/handoff/service.go Reassign` (dòng 99–158,
owner gate dòng 100): thêm bước trong cùng `WithinTx`: UPDATE assignment
`giao_vien` active của lớp → `ended_at = now()`; INSERT assignment mới cho
teacher đích. Nếu không tìm thấy đúng 1 active GV assignment → lỗi 500 (bất
biến gãy, không nuốt). `classstaff` expose repo method cho handoff qua
interface nhỏ (pattern `ClassReassigner`/`SessionReassigner` sẵn có).

**Web (owner)** — trang class settings hiện có: section "Nhân sự lớp": list
staff theo role, thêm/gỡ học vụ + trợ giảng (member picker từ
`/centers/me/members`), GV chính hiển thị read-only + link sang dialog handoff
sẵn có. Cảnh báo mềm (badge) khi lớp thiếu 1 trong 3 role. Gate hiển thị:
`useCenterContext().has()` — section chỉ owner (theo pattern owner-gate sidebar).

## Related Code Files

- Create: `apps/api/migrations/000015_class_staff.{up,down}.sql`
- Create: `apps/api/internal/shared/authctx/class_staff.go` (+ test)
- Create: `apps/api/internal/features/classstaff/{model,dto,repository,service,handler,routes,errors}.go` + `integration_test.go`
- Modify: `apps/api/internal/features/handoff/service.go` (dual-write tx),
  `apps/api/cmd/api/container` wiring (theo chỗ các feature khác đăng ký routes)
- Modify: `apps/api/internal/testutil/fixtures.go` (fixture staff assignment)
- Modify: `apps/web/src/features/roster` class settings page + api client mới
  `class-staff-api.ts`, hook `use-class-staff.ts`, component
  `class-staff-section.tsx`
- Modify: `apps/api/seeds/seed.go` — seed 1 học vụ + 1 trợ giảng gán vào lớp
  seed (để e2e các phase sau dùng)

## Implementation Steps

1. Migration 000015 + `make api-migrate` (hoặc lệnh tương đương trong Makefile) + test migration up/down.
2. `authctx/class_staff.go` vocabulary + capability map + unit test.
3. Feature package `classstaff` (repo → service → handler → routes, wire container), integration tests: owner CRUD, member 403, role lạ 422, GV 409, cross-center 404.
4. Handoff dual-write + test: sau reassign, đúng 1 GV active = teacher mới, assignment cũ ended; rollback tx khi insert fail.
5. Seeds + fixtures.
6. Web staff section + vitest cho component/hook.
7. `make test-api`, web vitest, swagger regen (`make api-docs`) nếu annotate.

## Success Criteria

- [ ] Migration up/down sạch trên DB có dữ liệu; backfill = số class.
- [ ] Owner gán/gỡ học vụ + trợ giảng qua API + UI; các case 403/404/409/422 test đủ.
- [ ] Handoff giữ bất biến 1 GV active (test tx + index DB chặn race).
- [ ] Không response/hành vi hiện có nào đổi (regression suite xanh).

## Risk Assessment

- **Race 2 handoff song song** → partial unique `uq_class_staff_one_gv` biến
  race thành lỗi tx (retry-able), không silent corrupt.
- **Class bị xóa** → FK `ON DELETE CASCADE` theo classes; kiểm tra classes có
  soft-delete không (nếu soft-delete, cascade không kích hoạt — assignment
  active của lớp deleted vô hại vì mọi đường đọc join classes).
- **Migration số 000015 đụng plan RBAC phase-04** → đã ghi nhận ở plan.md;
  RBAC cleanup renumber khi chạy.
