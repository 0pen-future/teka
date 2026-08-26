---
title: "System-wide audit log"
description: "Owner-only audit log: mutations + auth events, captured via generic in-process event bus (shared/events) with audit as first subscriber"
status: completed
priority: P1
effort: "4-6d"
tags: [api, web, infra, tdd]
created: 2026-08-26
---

# System-wide audit log

## Overview

Center owner xem được audit log mọi hành động user trong center: toàn bộ
mutations `/api/v1` (POST/PUT/PATCH/DELETE) + auth events
(login/logout/login-fail), qua trang web owner-only có filter + keyset
pagination. Capture qua **event bus in-process generic** tại
`internal/shared/events` (fan-out per-subscriber, non-blocking publish) —
audit là subscriber đầu tiên; middleware + auth chỉ phụ thuộc bus, không
import audit.

Nguồn: [brainstorm report](../reports/brainstorm-260826-1008-audit-log.md)
(contract + decisions user-confirmed). Mode: **TDD** — mỗi phase viết test
trước, implement sau.

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Event bus generic tái sử dụng (`shared/events`), zero latency thêm vào request path | P1 |
| 2 | Mọi mutation + auth event ghi đúng 1 dòng `audit_logs`, đúng tenant | P1 |
| 3 | Owner-only read API + trang web filter/paginate | P1 |
| 4 | Graceful shutdown drain đủ buffer trước khi đóng DB pool | P1 |

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Phase 1: Shared Event Bus](./phase-01-shared-event-bus.md) | Done |
| 2 | [Phase 2: Audit Schema And Feature Core](./phase-02-audit-schema-and-feature-core.md) | Done |
| 3 | [Phase 3: Capture Wiring](./phase-03-capture-wiring.md) | Done |
| 4 | [Phase 4: Owner Read Api](./phase-04-owner-read-api.md) | Done |
| 5 | [Phase 5: Web Audit Page](./phase-05-web-audit-page.md) | Done |
| 6 | [Phase 6: E2e And Docs](./phase-06-e2e-and-docs.md) | Done |

Dependency chain: 1 → 2 → 3 → 4 → 5 → 6 (tuần tự; 2 cần bus interface từ 1,
3 cần subscriber từ 2, 4 cần data từ 3, 5 cần API từ 4).

## Key architecture decisions (từ brainstorm, user-confirmed)

1. Capture: HTTP middleware publisher + async observer; auth feature publish
   trực tiếp qua cùng bus.
2. Observer infra generic tại `internal/shared/events` — infra thuần, không
   business logic. Event struct sống cạnh publisher
   (`middleware.RequestCompleted`, `auth.LoginSucceeded/...`).
3. At-most-once: queue đầy → drop + log warning, không chặn response.
4. Không lưu request body. Login-fail lưu **phone masked** (dạng `090***123`)
   + IP trong metadata (không password, không phone đầy đủ) — đủ điều tra
   brute-force, owner-only view. (validated 2026-08-26)
5. Pagination: keyset (occurred_at, id) — audit_logs grow unbounded, offset
   pagination của `shared/pagination` không phù hợp (chấp nhận divergence).
6. Auth events publish từ **auth Service** (không phải handler): service là
   nguồn sự thật outcome + user id (logout resolve user từ refresh token bên
   trong service); handler truyền `ClientMeta{IP, UserAgent}`.
   (validated 2026-08-26)
7. Tunables (subscriber buffer, batch size, flush interval, drain timeout)
   đưa vào config env `cfg.Audit`, default 1024/100/1s/5s.
   (validated 2026-08-26)
8. Auth-event visibility: JOIN `center_members` (left_at NULL) lúc đọc;
   login-fail không actor → owner không thấy ở V1 (limitation ghi docs).
   (validated 2026-08-26)

## Success Criteria

- [x] Mọi request mutating `/api/v1` của user đã đăng nhập tạo đúng 1 dòng
      `audit_logs` (kể cả 4xx/5xx, kèm status code)
- [x] Login thành công/thất bại + logout tạo bản ghi auth event
- [x] Publish non-blocking (overflow → drop + warning, không chặn response)
- [x] `GET /api/v1/audit-logs`: 403 cho non-owner; owner thấy đúng center;
      filter actor/action/time-range; keyset pagination
- [x] Trang web audit owner-only; member không thấy nav entry
- [x] Graceful shutdown: bus drain mọi subscriber queue trước khi đóng DB pool
- [x] Auth + middleware chỉ phụ thuộc `shared/events`, không import
      `features/audit`; thêm subscriber mới không sửa publisher
- [x] `go test ./...`, lint, typecheck, web build đều pass

## Validation Log

### Session 1 — 2026-08-26

#### Verification Results
- Claims checked: ~15 (Full tier, 6 phases)
- Verified: 14 | Failed: 1 | Unverified: 0
- Tier: Full
- Failures:
  - Phase 3 "auth publish tại handler": `POST /auth/logout` không mount
    `requireAuth` (`features/auth/routes.go:12-16`), user id chỉ resolve trong
    `Service.Logout` (`service.go:231` — GetByHash → FamilyID). Handler không
    có UserID cho LoggedOut event. → Đã sửa: publish từ service (Q1).
- Verified nổi bật: `auth.NewHandler(svc, cfg)` (`handler.go:29`),
  `apperror.Forbidden` (`apperror:68`), bảng `center_members` + `left_at NULL`
  (`centers/model.go:30-45`), `make api-docs` (Makefile:75), migrations
  embed `*.sql` tự pick-up, web feature `collections` làm mẫu list+filter.

#### Decisions
| # | Question | Answer |
|---|----------|--------|
| 1 | Publish point auth events | **Service publish** — handler truyền ClientMeta{IP, UA}; đổi signature Login/Logout nội bộ feature |
| 2 | Login-fail metadata | **Phone masked** (`090***123`) + IP |
| 3 | Auth-event visibility | **Join membership lúc đọc**; login-fail không actor → owner không thấy V1 |
| 4 | Tunables | **Config env** `cfg.Audit`: buffer/batch/flush-interval/drain-timeout, default 1024/100/1s/5s |

#### Whole-Plan Consistency Sweep
- Propagated: phase-02 (subscriber nhận batch/interval từ config; event
  struct LoginFailed mang PhoneMasked), phase-03 (publish từ service +
  ClientMeta + cfg.Audit; bỏ NewHandler thêm bus), phase-04 (visibility đã
  khớp sẵn — đánh dấu user-confirmed).
- Không còn mâu thuẫn mở. Failed: 0 sau khi propagate.

### Completion sweep — 2026-08-26

Bằng chứng cho 8 success criteria (lệnh chạy trong session hoàn tất Phase 6):

1. Mutation → 1 dòng kể cả 4xx/5xx: `features/audit/capture_integration_test.go`
   (go test ./... -race pass) + e2e `audit.spec.ts` thấy dòng 201 thật.
2. Auth events: `features/auth/events_publish_test.go` + subscriber tests.
3. Non-blocking publish: `shared/events/async_bus_test.go` (queue đầy → drop
   + warning).
4. Read API 403/filter/keyset: `features/audit/read_api_integration_test.go`,
   `list_plan_integration_test.go`.
5. Web owner-only + member không thấy nav: `audit-page.test.tsx` (8 tests),
   `dashboard-layout.test.tsx`, e2e member-negative pass trên stack thật.
6. Shutdown drain: bus `Close` + subscriber flush tests; compose
   `stop_grace_period: 30s`; docs deployment ghi grace ≥30s.
7. Decoupling: `middleware/request_events.go` + `features/auth` chỉ import
   `shared/events`; audit subscriber wire ở `app/container.go`.
8. Gates: `go test ./... -race` pass; web lint 0 error (4 warning có sẵn),
   typecheck sạch, vitest 387 passed/3 skipped, build pass; Playwright full
   suite 24/24 trên stack isolated (đã down -v).

<!-- slug: audit-log -->
