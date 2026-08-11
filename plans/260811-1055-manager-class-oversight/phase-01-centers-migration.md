---
phase: 1
title: "Centers Migration (re-key center_id)"
status: completed
priority: P1
effort: "1.5d"
dependencies: []
---

# Phase 1: Centers Migration (re-key center_id)

## Overview

Một migration 000007 (một transaction) chuyển tenant key từ teacher sang center: tạo bảng `centers`, backfill center cá nhân cho mỗi teacher hiện có, thêm `center_id NOT NULL` vào mọi bảng nghiệp vụ, đổi composite FK `(id, teacher_id)` → `(id, center_id)`. `teacher_id` GIỮ LẠI trên mọi bảng làm attribution (ai dạy/ai quản) và scope phụ cho role teacher.

## Requirements

- Functional: sau migrate, mọi row nghiệp vụ thuộc đúng center của teacher sở hữu cũ; mỗi teacher hiện có là owner của 1 center cá nhân.
- Non-functional: down migration khôi phục schema cũ; house style — comment tiếng Việt giải thích bất biến ngay trong SQL; không mất dữ liệu.

## Architecture — Schema Design

### Bảng mới

```sql
-- Tenant mới của hệ thống. Mọi bảng nghiệp vụ key theo center_id; teacher_id
-- chỉ còn là attribution trong center. owner là teacher có toàn quyền
-- đọc/ghi trong center. Bất biến owner.center_id = centers.id là app-enforced
-- (FK vòng centers.owner_id ↔ teachers.center_id không khai báo được sạch).
CREATE TABLE centers (
    id          UUID PRIMARY KEY,
    name        VARCHAR(255) NOT NULL,
    owner_id    UUID NOT NULL REFERENCES teachers(id) ON DELETE RESTRICT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ
);
-- 1 teacher chỉ own tối đa 1 center sống (center cá nhân hoặc trung tâm thật)
CREATE UNIQUE INDEX uq_centers_owner ON centers(owner_id) WHERE deleted_at IS NULL;
```

### Membership: cột trên teachers (không cần bảng join — 1 teacher = 1 center)

```sql
ALTER TABLE teachers ADD COLUMN center_id UUID REFERENCES centers(id);

-- Backfill: mỗi teacher hiện có 1 center cá nhân, chính họ là owner.
-- gen_random_uuid() builtin từ PG13 (repo đã yêu cầu PG >= 15 tại 000005).
INSERT INTO centers (id, name, owner_id)
SELECT gen_random_uuid(), t.full_name, t.id FROM teachers t;
UPDATE teachers t SET center_id = c.id FROM centers c WHERE c.owner_id = t.id;

ALTER TABLE teachers ALTER COLUMN center_id SET NOT NULL;
-- Anchor cho guard "teacher thuộc đúng center" trên các bảng nghiệp vụ
ALTER TABLE teachers ADD CONSTRAINT uq_teachers_center UNIQUE (id, center_id);
CREATE INDEX idx_teachers_center ON teachers(center_id);
```

### Re-key 16 bảng nghiệp vụ

Danh sách (mọi bảng đang có `teacher_id` + composite FK, trừ ngoại lệ bên dưới): `contacts`, `students`, `classes`, `class_schedules`, `enrollments`, `class_sessions`, `attendance_records`, `billing_periods`, `invoices`, `invoice_lines`, `invoice_adjustments`, `payments`, `payment_allocations`, `statements`, `notifications`, `notification_runs`.

Mẫu cho từng bảng (thứ tự trong cùng migration: (a) add + backfill cột trên TẤT CẢ bảng → (b) add unique/FK mới → (c) re-point FK con → (d) drop unique/FK cũ):

```sql
-- (a) thêm cột + backfill từ center của teacher sở hữu
ALTER TABLE students ADD COLUMN center_id UUID;
UPDATE students s SET center_id = t.center_id FROM teachers t WHERE s.teacher_id = t.id;
ALTER TABLE students ALTER COLUMN center_id SET NOT NULL;

-- (b) ranh giới toàn vẹn mới = center; guard teacher-thuộc-center thay CHECK
ALTER TABLE students ADD CONSTRAINT uq_students_cid UNIQUE (id, center_id);
ALTER TABLE students ADD FOREIGN KEY (teacher_id, center_id)
    REFERENCES teachers(id, center_id) ON DELETE CASCADE;

-- (c) FK con đổi vế tenant: (student_id, teacher_id) → (student_id, center_id)
--     Cross-teacher TRONG CÙNG center giờ hợp lệ ở DB (chủ đích — owner ghi
--     thay); isolation teacher-teacher chỉ còn ở query layer.
-- (d) drop FK cũ + UNIQUE (id, teacher_id) sau khi con đã re-point

CREATE INDEX idx_students_center ON students(center_id) WHERE deleted_at IS NULL;
-- GIỮ index teacher_id hiện có: read path của role teacher vẫn lọc teacher_id
```

Điểm cần chú ý riêng:
- `notifications.run_id`: FK `ON DELETE SET NULL (run_id)` với column list (PG >= 15, comment tại 000005) — tạo lại bản `(run_id, center_id)` tương đương.
- Views `v_contact_balance`, `v_unbilled_attendance`: DROP + CREATE lại thêm `center_id` (giữ `teacher_id`).
- Partial unique index theo `(teacher_id, ...)` hiện có (vd `contacts(teacher_id, phone)`, `billing_periods(teacher_id, year, month)`, index 000003/000006): rà từng cái — unique nghiệp vụ nào là "per tenant" thì đổi sang `center_id` (vd 1 kỳ billing/tháng là per teacher hay per center? → **giữ per teacher**: billing hiện hành theo GV, đổi ngữ nghĩa là scope creep; chỉ đổi index thuần tenant-isolation).

### Ngoại lệ (không re-key)

- `user_accounts`, `refresh_tokens`: identity/user-level, không có tenant key.
- `zalo_accounts`: `teacher_id` là PK — tài khoản Zalo cá nhân của teacher, đi theo người không theo trung tâm. Giữ nguyên.

### Down migration

Drop views → tạo lại bản cũ; drop FK/unique mới + cột `center_id` trên 16 bảng + teachers; drop `centers`. Khôi phục đầy đủ FK/unique `(id, teacher_id)` cũ.

## Related Code Files

- Create: `apps/api/migrations/000007_centers.{up,down}.sql`
- Modify: `apps/api/migrations/migrations_test.go` — `domainTables` (`migrations_test.go:23-29`) thêm `centers`; thêm assert constraint (xem Success Criteria)
- Modify: `docs/schema_design.sql` — artifact schema gốc mà migration test đối chiếu; cập nhật toàn bộ theo shape mới (centers + center_id mọi bảng)

## Implementation Steps

1. Viết up: centers → teachers → 16 bảng theo thứ tự (a)-(d) → views. Một transaction.
2. Viết down đối xứng; chạy `up → down → up` trên DB dev có seed.
3. Cập nhật `domainTables` + `docs/schema_design.sql`; chạy `TestMigrationRoundTrip`.
4. Assert trong migrations_test: (i) backfill — COUNT(centers) = COUNT(teachers), mọi bảng nghiệp vụ 0 row có center_id lệch với center của teacher_id; (ii) guard — INSERT class với `(teacher_id, center_id)` lệch nhau bị FK chặn; (iii) 1 owner 1 center sống bị `uq_centers_owner` chặn.

## Success Criteria

- [x] `up → down → up` sạch trên DB có seed; không mất row nào
- [x] Mỗi teacher hiện có đúng 1 center cá nhân (owner = chính họ), `teachers.center_id NOT NULL`
- [x] FK `(teacher_id, center_id)` chặn gắn teacher center khác; `uq_centers_owner` chặn own 2 center
- [x] `domainTables` chứa `centers`; `TestMigrationRoundTrip` pass; `docs/schema_design.sql` khớp migration
- [x] Views tạo lại có `center_id`; index `center_id` partial `deleted_at IS NULL` trên bảng lớn

## Risk Assessment

- **Migration big-bang trên 16 bảng**: một transaction — fail giữa chừng tự rollback; test round-trip trên seed là gate bắt buộc trước khi merge.
- **Sai backfill (center_id lệch teacher)**: assert đối chiếu từng bảng trong migrations_test, không tin mắt thường.
- **Unique per-teacher vs per-center**: quyết định giữ ngữ nghĩa per-teacher cho unique nghiệp vụ (billing_periods, contacts phone) — chỉ đổi phần tenant-isolation. Ghi rõ từng index trong SQL comment khi implement.
- **Lock time trên DB lớn**: chấp nhận ở quy mô hiện tại (single-tenant nhỏ); không cần online migration.

## Execution Log — 2026-08-11

Hai quyết định tại review gate (code-reviewer chứng minh bằng pg_dump trên
container thật), schema cuối cùng LỆCH so với thiết kế inline phía trên:

1. **Guard FK neo vào `center_members`, không phải `teachers`.** Guard
   `(teacher_id, center_id) → teachers(id, center_id)` chặn đứng leave-flow
   của Phase 2 (`UPDATE teachers SET center_id` fail khi teacher còn dữ liệu ở
   center cũ — DEFERRABLE không cứu được vì trạng thái đích tự nó vi phạm).
   Thay bằng bảng lịch sử membership `center_members(teacher_id, center_id,
   joined_at, left_at)`: row đóng (`left_at`) giữ chân dữ liệu cũ ở center cũ;
   `teachers.center_id` vẫn là membership hiện tại, có FK
   `fk_teachers_membership` (deferrable) trỏ vào center_members;
   `uq_center_members_active` giữ bất biến một membership sống mỗi teacher.
   Rời center = UPDATE `left_at`, không bao giờ DELETE (guard FK CASCADE).
2. **`centers.owner_id` = `ON DELETE NO ACTION DEFERRABLE INITIALLY
   DEFERRED`** (RESTRICT không hoãn được trong PG): giữ đăng ký
   center+owner một transaction VÀ mở lại đường xoá cứng tài khoản —
   DELETE user_accounts + DELETE centers trong cùng một transaction.

Kết quả: 10 integration test xanh (`ok teka/apps/api/migrations`), gồm 2 test
mới `TestTeacherLeavesCenterDataStaysBehind` và
`TestTeacherHardDeleteInOneTransaction`. `uq_teachers_center` bị loại (không
còn FK nào tham chiếu). Mang sang phase sau: fence center soft-deleted ở app
layer (Phase 2); `v_contact_balance` group theo teacher_id nên Phase 4 cần
aggregate riêng cho rollup center.
