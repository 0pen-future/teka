---
title: "Phase 3: Capture Wiring"
status: done
priority: P1
effort: "1d"
dependencies: [2]
---

# Phase 3: Capture Wiring

## Overview

Nối tất cả vào app: middleware publish `RequestCompleted`, auth publish
login/logout/login-fail, container build bus + audit subscriber
(process-lifetime), graceful shutdown drain đúng thứ tự.

## Requirements

- [x] Mọi request mutating `/api/v1` của user đã đăng nhập publish đúng 1
      event (kể cả 4xx/5xx); GET và request chưa auth không publish
- [x] Auth: login OK → `auth.LoginSucceeded`; sai credentials →
      `auth.LoginFailed` (phone **masked** `090***123` + ip trong metadata,
      không password, không phone thô); logout → `auth.LoggedOut`
- [x] Container: bus + subscriber sống theo process; `Close` thứ tự:
      Notifications → Zalo → **bus.Close (drain)** → **audit subscriber Close
      (flush)** → database.Close
- [x] Middleware + auth không import `features/audit` (chỉ `shared/events`)
- [x] (Review P2 — user quyết 260826) `POST /auth/forgot-password` +
      `/auth/reset-password` được middleware audit dù không principal
      (allowlist riêng trong middleware; action map đã có entry, actor NULL)
- [x] (Review P2 — user quyết 260826) invitations service publish event khi
      accept thành công, mang `CenterID` + user mới → owner thấy được row
      `invitation.accept`; bỏ 2 entry public invitations khỏi action map
      (middleware skip no-principal nên entry chết)
- [x] (Review P2, M2) `validateAudit` trong config theo pattern `validateX`
      hiện có + clamp phòng thủ trong `NewSubscriber` (batchSize ≥ 1 và
      ≤ 4000 — pg limit 65535 params, flushInterval > 0)

## Architecture

- `middleware/request_events.go` thêm `RequestEvents(bus events.Bus) gin.HandlerFunc`:
  chạy `c.Next()` trước, sau đó:
  - skip nếu method không thuộc {POST, PUT, PATCH, DELETE}
  - skip nếu không có `authctx.From` principal (login/logout do auth feature
    tự publish — middleware skip nhóm `/auth/*` để không double-log:
    check `c.FullPath()` prefix hoặc đơn giản skip khi không có principal +
    skip route `/api/v1/auth/*` có principal như logout)
  - build `RequestCompleted{OccurredAt, Method, RouteTemplate (c.FullPath()),
    Path, Status, ActorUserID, ActorRole, CenterID (*uuid — từ ScopeFrom, nil
    nếu chưa resolve), EntityID (param "id" nếu có), RequestID, IP
    (c.ClientIP()), UserAgent}` → `bus.Publish`
  - Mount: `v1.Use(middleware.RequestEvents(bus))` trong `NewRouter` ngay sau
    tạo group, trước `registerFeatures` (đọc principal/scope sau `c.Next()`
    nên thứ tự mount trước requireAuth vẫn đúng).
- **Double-log rule (đã sửa theo review P2):** middleware skip
  `/api/v1/auth/login|logout|refresh` (login/logout do auth service publish;
  refresh = noise, non-goal) và mọi request không principal — TRỪ allowlist
  `POST /api/v1/auth/forgot-password` + `POST /api/v1/auth/reset-password`
  (user quyết: password reset phải vào trail, actor NULL). Invitations public
  routes vẫn skip ở middleware; accept audit qua service event (xem dưới).
- **Invitations accept event (user quyết 260826):** thêm
  `invitations.MemberJoined{OccurredAt, CenterID, UserID, InvitationID, IP,
  UserAgent}` publish từ service sau accept thành công (transaction OK);
  audit subscriber map → action `invitation.accept`, entity invitation,
  center_id resolve được → hiển thị cho owner qua JOIN Phase 4. Service nhận
  bus + ClientMeta như auth. Xóa entry `invitations/preview|accept` khỏi
  action map (dead entries — middleware skip no-principal).
- Auth publish tại **service** (validated: logout không có principal ở
  handler — `/auth/logout` không mount requireAuth; user id resolve trong
  `Service.Logout` từ refresh token → family). Thiết kế:
  - `auth.ClientMeta{IP, UserAgent string}` — handler build từ gin ctx,
    truyền xuống: `Login(ctx, req, meta)`, `Logout(ctx, plaintext, meta)`
    (đổi signature nội bộ feature; cập nhật callsites + tests hiện có).
  - `auth.NewService(...)` nhận thêm `events.Bus`; publish:
    `LoginSucceeded` sau `openSession` OK; `LoginFailed{maskPhone(req.Phone), meta}`
    tại các nhánh trả `invalid` (không publish cho lỗi internal);
    `LoggedOut{t.UserID}` sau `RevokeFamily` OK (token not found → không publish).
  - `maskPhone`: giữ 3 đầu 3 cuối, giữa `***`; helper trong features/auth.
  - Handler KHÔNG nhận bus — `NewHandler(authSvc, cfg)` giữ nguyên.
- Config: thêm `cfg.Audit` trong `internal/config` (theo pattern section
  hiện có): `BufferSize` (1024), `BatchSize` (100), `FlushInterval` (1s),
  `DrainTimeout` (5s) — env override.
- `app/container.go`: `Bus events.Bus` + `Audit *audit.Subscriber` fields;
  `NewContainer` build `events.New(log)` →
  `audit.NewSubscriber(audit.NewRepository(db), log, cfg.Audit.BatchSize, cfg.Audit.FlushInterval)`
  → `bus.Subscribe("audit", cfg.Audit.BufferSize, sub.Handle)`; bus truyền
  vào `auth.NewService`. `NewRouter` nhận thêm `bus`.
- `Close`: drain với `context.WithTimeout(cfg.Audit.DrainTimeout)`. Thứ tự
  bắt buộc (doc trên `Subscriber.Close`): `bus.Close(ctx)` →
  `subscriber.Close()` → `database.Close` — subscriber có cờ `closed`, event
  đến sau Close bị drop + warn.
- **Carry-forward review P2:** middleware dùng `c.Request.URL.Path` (không
  `RequestURI` — query string có thể chứa số điện thoại); `flushTimeout` 5s
  đã bound mọi insert trong subscriber (kể cả final flush).
<!-- Updated: Validation Session 1 - service publish + ClientMeta + cfg.Audit -->


## Related Code Files

- Modify: `apps/api/internal/middleware/request_events.go` (thêm middleware func)
- Modify: `apps/api/internal/features/auth/service.go` (publish 3 events + maskPhone + bus param)
- Modify: `apps/api/internal/features/auth/handler.go` (build ClientMeta, signature calls)
- Create: `apps/api/internal/features/invitations/events.go` (MemberJoined)
- Modify: `apps/api/internal/features/invitations/{service,handler,routes}.go` (bus + publish accept)
- Modify: `apps/api/internal/features/audit/{action,subscriber}.go` (map MemberJoined, bỏ entry public invitations)
- Modify: `apps/api/internal/config/config.go` (validateAudit)
- Modify: `apps/api/internal/config/*` (section `Audit`)
- Modify: `apps/api/internal/app/container.go` (bus + subscriber + Close order)
- Modify: `apps/api/internal/app/app.go` (RunServer truyền bus vào NewRouter)
- Modify: `apps/api/internal/server/router.go` (param bus, v1.Use)
- Create: `apps/api/internal/middleware/request_events_test.go`
- Modify: `apps/api/internal/features/auth/integration_test.go` (+ audit rows)
- Create/Modify: `apps/api/internal/features/audit/integration_test.go`
  (end-to-end: HTTP mutation → 1 dòng)

## Implementation Steps (TDD)

1. **Test trước** — `request_events_test.go` (gin test ctx + `SyncBus` + fake
   subscriber ghi nhận event):
   - POST có principal+scope → 1 event đủ trường, status 201
   - POST trả 422 → vẫn 1 event, status 422
   - GET → 0 event; POST không principal → 0 event
   - POST `/api/v1/auth/logout` (có principal) → 0 event từ middleware
2. **Test trước** — auth: unit test service (SyncBus + fake handler ghi
   event): login OK/fail/logout publish đúng event + phone masked, lỗi
   internal không publish; integration: 3 flows → đúng action rows
   (SyncBus + audit subscriber thật + test DB).
3. **Test trước** — audit integration: dựng router test (theo pattern
   integration test hiện có) + SyncBus: POST tạo class → đúng 1 dòng
   audit_logs, center_id khớp scope.
4. Implement middleware, auth handler publish, container/router wiring tới xanh.
5. Shutdown test: publish N event → `Container.Close` → đủ N dòng trong DB
   (AsyncBus thật, không SyncBus).
6. `go test ./... -race` toàn API.

## Todo

- [x] request_events_test.go (đỏ) → middleware (xanh)
- [x] Auth events publish + tests
- [x] Container/router wiring + shutdown drain test
- [x] go test ./... -race pass

## Success Criteria

- [x] Acceptance 1-3 + 6 của brainstorm chứng minh bằng test
- [x] `grep` xác nhận middleware + auth không import `features/audit`
- [x] Benchmark nhẹ hoặc reasoning ghi lại: publish path chỉ là channel send

Reasoning (260826, thay benchmark): `AsyncBus.Publish` chỉ làm nil-check,
`RLock`, rồi non-blocking `select` gửi vào channel từng subscriber (queue đầy
→ drop + warn). Không I/O, không DB, không alloc đáng kể trên request path;
mọi ghi DB xảy ra ở worker goroutine của subscriber. Overhead per-request ở
mức sub-microsecond — benchmark riêng là không cần thiết.

## Risk Assessment

- Double-log auth: rule skip prefix `/api/v1/auth` — test #1.4 khóa lại.
- Drain treo khi DB chết lúc shutdown → Close ctx timeout 5s, log error, vẫn
  đóng DB (mất batch cuối, chấp nhận at-most-once).
- `c.FullPath()` rỗng cho route không match (404) → skip publish khi FullPath
  rỗng (không audit 404 route lạ, tránh noise/log-injection qua path tùy ý).
