---
title: "Phase 2: Audit Schema And Feature Core"
status: done
priority: P1
effort: "1d"
dependencies: [1]
---

# Phase 2: Audit Schema And Feature Core

## Overview

Migration `000010_audit_logs` + phần lõi `features/audit`: model, repository
(batch insert + list keyset), subscriber (consume events từ bus, tự batch),
bảng map route→action. Chưa wiring vào app — phase 3 làm.

## Requirements

- [x] Bảng `audit_logs` đúng design sketch brainstorm, index
      `(center_id, occurred_at DESC, id DESC)`
- [x] Repository `InsertBatch` 1 câu INSERT cho N dòng (GORM `Create(&rows)`)
- [x] Subscriber type-switch trên event, build `AuditLog` row, batch theo
      size hoặc interval (nhận từ constructor —
      `NewSubscriber(repo, log, batchSize, flushInterval)`; giá trị thực từ
      `cfg.Audit` ở phase 3, default 100/1s), flush còn lại khi Close
- [x] Action mapping: `method + route template` → tên đọc được
      (`class.create`, `billing.adjustment.create`…); fallback `METHOD /path`
- [x] Không lưu request body

## Architecture

- Migration (theo style `apps/api/migrations/0000NN_*.up/down.sql` hiện có):

  ```sql
  CREATE TABLE audit_logs (
      id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
      occurred_at   timestamptz NOT NULL,
      center_id     uuid NULL,            -- NULL cho auth events chưa có scope
      actor_user_id uuid NULL,            -- NULL cho login-fail
      actor_role    text NOT NULL DEFAULT '',
      action        text NOT NULL,
      method        text NOT NULL DEFAULT '',
      path          text NOT NULL DEFAULT '',
      entity_type   text NOT NULL DEFAULT '',
      entity_id     text NOT NULL DEFAULT '',   -- route param, dạng text
      status_code   int  NOT NULL DEFAULT 0,
      request_id    text NOT NULL DEFAULT '',
      ip            text NOT NULL DEFAULT '',
      user_agent    text NOT NULL DEFAULT '',
      metadata      jsonb NOT NULL DEFAULT '{}'
  );
  CREATE INDEX idx_audit_logs_center_time ON audit_logs (center_id, occurred_at DESC, id DESC);
  CREATE INDEX idx_audit_logs_actor ON audit_logs (actor_user_id, occurred_at DESC);
  ```

  Đối chiếu style migration 000009 trước khi viết (naming, extension đã enable
  `gen_random_uuid` chưa). Không FK tới users/centers — audit giữ được log kể
  cả khi actor bị xóa; đúng bản chất append-only.

- `features/audit/subscriber.go`: struct giữ `repo`, `mu`, `buf []AuditLog`,
  ticker goroutine flush interval; `Handle(ctx, e)` là `events.Handler` —
  type-switch:
  - `middleware.RequestCompleted` → map đủ cột (phase 3 mới có struct này;
    phase 2 define interface nội bộ hoặc build sau — xem note dưới)
  - auth events → action `auth.login` / `auth.login_fail` / `auth.logout`
  - `Close()` stop ticker + flush nốt.

  **Ordering note:** để phase 2 tự test được mà chưa có publisher, định nghĩa
  event structs ngay phase này tại vị trí cuối cùng của chúng
  (`middleware.RequestCompleted` trong `internal/middleware/request_events.go`
  chỉ struct + EventName, chưa có middleware func; `auth` events trong
  `features/auth/events.go`: `LoginSucceeded{UserID, IP, UserAgent}`,
  `LoginFailed{PhoneMasked, IP, UserAgent}` — phone đã mask sẵn dạng
  `090***123` từ publisher, subscriber không thấy phone thô,
  `LoggedOut{UserID, IP, UserAgent}`). Phase 3 thêm code publish.
  <!-- Updated: Validation Session 1 - masked phone + config-driven batching -->

- `features/audit/action.go`: `map[string]string` key `"POST /api/v1/classes"`
  (route template từ `c.FullPath()`) → `"class.create"`. Bảng liệt kê đủ route
  mutating hiện có (đọc `server/router.go` + từng `routes.go` lúc implement);
  route không có trong map → fallback `method + " " + path`, vẫn ghi log.

## Related Code Files

- Create: `apps/api/migrations/000010_audit_logs.up.sql` / `.down.sql`
- Create: `apps/api/internal/features/audit/model.go`
- Create: `apps/api/internal/features/audit/repository.go`
- Create: `apps/api/internal/features/audit/subscriber.go`
- Create: `apps/api/internal/features/audit/action.go`
- Create: `apps/api/internal/features/audit/subscriber_test.go`
- Create: `apps/api/internal/features/audit/integration_test.go` (repo phần insert/list)
- Create: `apps/api/internal/middleware/request_events.go` (chỉ event struct)
- Create: `apps/api/internal/features/auth/events.go` (event structs)

## Implementation Steps (TDD)

1. Đọc migration 000009 + 1 model/repository hiện có (vd `features/centers`)
   để khớp convention (embed.go tự pick up file mới? xác nhận).
2. **Test trước** — `subscriber_test.go` với fake repo (ghi nhận các batch):
   - 100 events → flush 1 batch size 100
   - 3 events + đợi interval → flush 1 batch size 3
   - Close → flush phần còn lại
   - RequestCompleted map đủ cột; auth events map đúng action + center_id NULL
3. **Test trước** — `integration_test.go` (testutil postgres): InsertBatch N
   dòng rồi query lại đủ cột, metadata jsonb roundtrip.
4. Viết migration; chạy migrate up/down trên test DB.
5. Implement model/repository/subscriber/action tới khi xanh.
6. `go test ./internal/features/audit/`.

## Todo

- [x] Migration up/down + verify migrate up/down
- [x] Event structs (middleware/request_events.go, auth/events.go)
- [x] subscriber_test.go + integration_test.go (đỏ)
- [x] model/repository/subscriber/action (xanh)
- [x] Bảng action map phủ đủ route mutating hiện có

## Success Criteria

- [x] Test phase pass với `-race`
- [x] Migration up/down sạch trên DB trống và DB có data
- [x] Subscriber không giữ reference tới gin/HTTP — chỉ nhận event struct

## Risk Assessment

- Batch flush trong transaction? Không cần — insert append-only, mất batch khi
  crash đã chấp nhận (at-most-once).
- entity_id nhiều dạng (uuid, số) → cột text, không ép uuid.
- Action map trôi khi thêm route mới → fallback đảm bảo vẫn ghi log; docs
  phase 6 ghi quy ước thêm map entry khi thêm route.
