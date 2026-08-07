---
phase: 2
title: "Migration + mapping backend"
status: completed
priority: P1
effort: "1d"
dependencies: [1]
---

# Phase 2: Migration + mapping backend

## Overview

Migration `000005`: thêm mapping Zalo trên `contacts`, mở CHECK constraint
`notifications.channel` cho `zalo_personal`, và tạo bảng `notification_runs`
(+ cột `notifications.run_id`) cho run persistence. API: giáo viên xem danh
sách bạn Zalo, map/unmap từng contact.

<!-- Updated: Validation Session 1 - thêm bảng notification_runs + notifications.run_id -->

## Requirements

- Functional:
  - `contacts.zalo_user_id VARCHAR(32) NULL` + `contacts.zalo_name VARCHAR(100) NULL`
    (tên bạn Zalo tại thời điểm map, để UI hiển thị không cần refetch friends).
  - `notifications.channel` CHECK thêm `'zalo_personal'`.
  - Bảng `notification_runs`: `id UUID PK`, `teacher_id UUID NOT NULL` (FK
    teachers), `billing_period_id UUID NOT NULL` (FK billing_periods),
    `purpose VARCHAR`, `status VARCHAR` CHECK
    (`running`/`completed`/`interrupted`/`expired`), `created_at`,
    `finished_at NULL`. Cột `notifications.run_id UUID NULL` FK
    `notification_runs` — counters (total/sent/failed) derive bằng COUNT trên
    `notifications` theo `run_id`, không lưu counter trùng lặp trong bảng runs.
  - `GET /me/zalo/friends` (feature zalo): trả `[{user_id, display_name, avatar}]`
    từ `Service.ListFriends`; chưa link → 404 (`ErrNotLinked` qua `linkError`,
    nhất quán các endpoint `/me/zalo` hiện có); expired → 409 để UI đưa về
    profile re-scan.
  - `PUT /contacts/:id/zalo-mapping` body `{zalo_user_id, zalo_name}` — validate
    non-empty, teacher-scoped; `DELETE /contacts/:id/zalo-mapping` → NULL cả 2 cột,
    idempotent (theo chuẩn Unlink 204).
  - Contact DTO responses (list/get) trả thêm `zalo_user_id`, `zalo_name`.
- Non-functional: docs `docs/schema_design.sql` + swagger regen; migration
  up/down qua `migrations_test.go` cycle.

## Architecture

- Mapping sống trên `contacts` (per-teacher tenant sẵn có, phone-unique per
  teacher) — không bảng mới, không FK sang zalo_accounts (mapping vẫn giá trị
  sau khi unlink/relink cùng account).
- Endpoint friends thuộc feature `zalo` (cần session); endpoint mapping thuộc
  feature `contacts` (chỉ ghi cột, không gọi Zalo). Không validate
  `zalo_user_id` với live friends list ở backend (picker UI là nguồn; validate
  live = phụ thuộc Zalo up mỗi lần sửa contact — từ chối có chủ đích).
- Down migration: `UPDATE notifications SET channel='zalo_manual' WHERE
  channel='zalo_personal'` trước khi restore CHECK cũ; drop cột
  `notifications.run_id`, drop bảng `notification_runs`, drop 2 cột contacts —
  down luôn chạy được.

## Related Code Files

- Create: `apps/api/migrations/000005_zalo_personal_mapping.up.sql` / `.down.sql`
- Modify: `apps/api/internal/features/contacts/{model,dto,handler,repository,service,routes}.go` + tests
- Modify: `apps/api/internal/features/zalo/{handler,dto,routes}.go` + tests
  (endpoint friends)
- Modify: `docs/schema_design.sql` (contacts + notifications CHECK)
- Modify: `apps/api/docs/*` (swagger regen)

## Implementation Steps

1. Viết migration 000005 (up/down như Architecture); chạy `migrations_test.go`.
2. Contacts: thêm field model + DTO, `UpdateZaloMapping`/`ClearZaloMapping`
   repo+service+handler, routes PUT/DELETE `/contacts/:id/zalo-mapping`.
3. Zalo: handler `GET /me/zalo/friends` → `ListFriends`; DTO hand-built (chuẩn
   reflection test chống lộ credentials đã có trong `handler_test.go`).
4. Integration tests (`-tags=integration`) cho mapping CRUD + constraint mới.
5. Regen swagger, cập nhật `docs/schema_design.sql`.

## Success Criteria

- [x] Migration up/down cycle pass trong `migrations_test.go` (gồm bảng
      `notification_runs` + `run_id`).
- [x] Map → contact trả `zalo_user_id`/`zalo_name`; unmap idempotent 204.
- [x] `GET /me/zalo/friends` trả list khi linked; 4xx rõ ràng khi chưa link/expired.
- [x] `go test ./...` + `-tags=integration` pass; swagger + schema doc khớp code.

## Risk Assessment

- **CHECK constraint name:** baseline dùng inline CHECK (auto-name
  `notifications_channel_check`) — xác nhận tên thật từ catalog trong migration
  (`ALTER TABLE ... DROP CONSTRAINT` đúng tên, hoặc dùng
  `information_schema`/`pg_constraint` lookup khi viết) trước khi merge.
- **Friends list lớn:** trả nguyên list một lần (bạn bè cá nhân ~vài trăm, đủ
  nhỏ); không phân trang — ghi nhận YAGNI.
