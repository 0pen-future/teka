---
phase: 1
title: "Database Migrations"
status: pending
priority: P1
effort: "0.25d"
dependencies: []
---

# Phase 1: Database Migrations

## Overview

Schema cho oversight trong 1 migration (000007), additive-only — chỉ thêm bảng `management_grants`, không đổi cột/bảng hiện có. (Migration class_note + curriculum đã bị loại khỏi plan cùng phase Session Notes and Curriculum — scope change 260811.)

## Requirements

- Functional: đủ bảng cho grants (tạo/thu hồi, unique cặp sống).
- Non-functional: giữ house style — soft-state qua `revoked_at`, comment tiếng Việt giải thích bất biến ngay trong SQL.

## Architecture — Schema Design

### 000007_management_grants

```sql
-- Quan hệ "manager được xem dữ liệu của managed". Do CHÍNH GV bị quản lý tạo
-- (data owner consent). KHÔNG phải role — cả hai phía đều là teachers.
-- Đây là nguồn scope DUY NHẤT cho mọi truy cập cross-teacher (read-only).
CREATE TABLE management_grants (
    id                  UUID PRIMARY KEY,
    manager_teacher_id  UUID        NOT NULL REFERENCES teachers(id) ON DELETE CASCADE,
    managed_teacher_id  UUID        NOT NULL REFERENCES teachers(id) ON DELETE CASCADE,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Thu hồi = set revoked_at, không xoá — giữ lịch sử vòng đời grant
    -- (KHÔNG phải access log: bảng này không ghi ai đã xem gì, chỉ ghi
    -- quan hệ quản lý tồn tại từ khi nào đến khi nào).
    revoked_at          TIMESTAMPTZ,
    CHECK (manager_teacher_id <> managed_teacher_id)
);
-- Một cặp chỉ có 1 grant đang hiệu lực; re-grant sau khi thu hồi tạo dòng mới.
CREATE UNIQUE INDEX uq_management_grants_active
    ON management_grants (manager_teacher_id, managed_teacher_id)
    WHERE revoked_at IS NULL;
CREATE INDEX idx_management_grants_manager ON management_grants (manager_teacher_id) WHERE revoked_at IS NULL;
CREATE INDEX idx_management_grants_managed ON management_grants (managed_teacher_id) WHERE revoked_at IS NULL;
```

Meta fields: `created_at`, `revoked_at` (không cần `updated_at` — grant bất biến, chỉ tạo/thu hồi).

## Related Code Files

- Create: `apps/api/migrations/000007_management_grants.{up,down}.sql`
- Modify: `apps/api/migrations/migrations_test.go` — **bắt buộc**: `domainTables`
  (`migrations_test.go:23-29`) hardcode danh sách bảng; thêm `management_grants`
- Modify: `docs/schema_design.sql` — artifact schema gốc mà migration test đối
  chiếu; thêm bảng `management_grants` — đây là bảng authz (người đọc schema
  phải thấy được đường đọc cross-teacher tồn tại)

## Implementation Steps

1. Viết cặp up/down theo SQL trên; down drop index rồi bảng.
2. Chạy `migrate up` trên DB dev có seed; chạy `down` toàn bộ rồi `up` lại xác nhận idempotent.
3. Chạy migration test hiện có.

## Success Criteria

- [ ] `up` rồi `down` rồi `up` sạch trên DB có dữ liệu seeds
- [ ] Không đổi bất kỳ cột/constraint hiện có nào
- [ ] Grant self-reference bị CHECK chặn; 2 grant sống trùng cặp bị unique index
      chặn — assert trong `migrations_test.go` (test constraint sống ở đó, cùng
      chỗ với round-trip test)
- [ ] `domainTables` chứa `management_grants`; `TestMigrationRoundTrip` pass
- [ ] `docs/schema_design.sql` cập nhật khớp migration

## Risk Assessment

- **Partial unique + revoke**: thu hồi rồi re-grant cùng cặp phải hợp lệ — covered bởi `WHERE revoked_at IS NULL`; assert trong migration test.
