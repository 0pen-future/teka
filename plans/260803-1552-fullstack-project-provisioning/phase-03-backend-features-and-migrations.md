---
phase: 3
title: "Phase 3: Backend Features and Migrations"
status: todo
priority: P1
effort: "1.5d"
dependencies: [2]
---

# Phase 3: Backend Features and Migrations

## Overview

Deliver the data layer and the two reference features. golang-migrate owns schema; `users` and `auth` features demonstrate the full feature-module contract: handler → DTO → service → repository interface → GORM implementation → model → validation → routes. Includes transactions, pagination/filtering, seeding, and the remaining Cobra commands (`migrate`, `seed`, `admin create`).

## Requirements

- Functional: register/login/refresh/logout; user CRUD with pagination, filtering, sorting; role-based authorization (admin vs user).
- Functional: `migrate up|down|status`, `seed`, `admin create --email --password` Cobra commands fully working.
- Non-functional: schema changes only via versioned SQL migrations; passwords bcrypt (cost 12); refresh tokens rotated and stored hashed; all list endpoints capped (`per_page` max 100).

## Architecture

**Feature module contract** (documented in `docs/api-guidelines.md`; both features must match exactly):

```text
features/<name>/
  handler.go      # Gin handlers: bind DTO → call service → write envelope. No business logic.
  dto.go          # Request/response structs + validate tags + ToModel/FromModel mappers
  service.go      # business logic; depends on repository INTERFACE + TxManager
  repository.go   # interface at top, GORM implementation below (split files if >300 lines)
  model.go        # GORM model, table name, indexes as comments matching migration SQL
  routes.go       # RegisterRoutes(rg *gin.RouterGroup, h *Handler, mw ...gin.HandlerFunc)
  service_test.go       # unit: fake repository
  integration_test.go   # real Postgres via testutil (Phase 4 wires containers)
```
Features never import other features' repositories — cross-feature calls go service→service through interfaces defined by the consumer (documented; enforced in review).

**Migrations** (`golang-migrate`): SQL files in `apps/api/migrations`, embedded via `embed.FS` and executed with `migrate` library + `iofs` source so the compiled binary migrates without external files. CLI:
- `api migrate up` / `down [--steps N=1]` / `status`; `down` with no steps requires `--yes` in non-dev.
- Initial migrations: `000001_create_users` (id UUID PK default `gen_random_uuid()`, email citext unique, password_hash, name, role text check in ('admin','user'), timestamps, soft-delete `deleted_at` index), `000002_create_refresh_tokens` (token_hash unique, user_id FK cascade, expires_at, revoked_at).

**Transactions** (`internal/database/tx.go`):

```go
type TxManager interface {
    WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}
```
GORM implementation stores the tx `*gorm.DB` in context; repositories fetch `dbFromContext(ctx, r.db)` so the same repository methods work inside and outside transactions. Services own transaction boundaries (e.g. register = create user + create refresh token atomically).

**Pagination/filtering** (`internal/shared/pagination`): parse `page`, `per_page`, `sort` (whitelisted columns per feature), plus feature-defined filters (`q`, `role`). Returns `Params` consumed by repositories via a `Scope(db)` helper; total count + rows → `meta` envelope block.

**Auth design:**
- Access JWT: HS256, 15 min, claims `sub` (user id) + `role`.
- Refresh: opaque 256-bit random, stored **hashed** (sha256) with expiry; rotation on every refresh; reuse of a rotated token revokes the family. Delivered as httpOnly `Secure` `SameSite=Lax` cookie; access token in JSON body.
- Middleware `auth.go`: `RequireAuth` (parse+verify JWT, inject principal into context), `RequireRole("admin")`.
- Endpoints: `POST /auth/register`, `POST /auth/login`, `POST /auth/refresh`, `POST /auth/logout`, `GET /auth/me`; users: `GET/POST /users` (admin), `GET/PATCH/DELETE /users/:id` (admin or self for GET/PATCH minus role change).

**Seeding** (`apps/api/seeds/seed.go`): idempotent upserts keyed by email; dev dataset (admin + N users). `api seed` refuses when `API_ENV=production` unless `--force`. `admin create` prompts for password when flag omitted (no secrets in shell history).

## Related Code Files

- Create: `apps/api/migrations/0000{1,2}_*.{up,down}.sql`
- Create: `apps/api/internal/database/{migrate.go,tx.go}`
- Create: `apps/api/internal/shared/{pagination,validation}/...`
- Create: `apps/api/internal/features/users/*` (8 files per contract)
- Create: `apps/api/internal/features/auth/*`
- Create: `apps/api/internal/middleware/auth.go` (real implementation)
- Create: `apps/api/seeds/seed.go`
- Modify: `apps/api/internal/cli/{migrate,seed,admin}.go` (implement)
- Modify: `apps/api/internal/server/router.go` (mount feature routes)
- Modify: root `Makefile` (`migrate-up`, `migrate-down`, `migrate-status`, `seed` targets → `go run ./cmd/api ...`)

## Implementation Steps

1. Write migrations; enable `citext` + `pgcrypto` extensions in `000001`.
2. Implement `database/migrate.go` (embed.FS + iofs) and the `migrate` Cobra command; verify up/down/status round-trip.
3. Implement `TxManager` + context-scoped repository db resolution.
4. Implement `pagination` and `validation` shared packages (validator error → `fields` map translation).
5. Build `users` feature bottom-up: model → repository (interface first) → service → DTOs → handler → routes; unit tests alongside (fake repo).
6. Build `auth` feature: token service (issue/verify/rotate), handlers, middleware; wire `RequireAuth`/`RequireRole` into users routes.
7. Mount features in `router.go` via container wiring.
8. Implement seeds + `admin create`.
9. Verify manually: register → login → me → list users (as admin) → refresh rotation → logout.

## Success Criteria

- [ ] `make migrate-up && make seed` on a fresh DB succeeds and is re-runnable (idempotent seed)
- [ ] `migrate down` fully reverses; `status` reports version
- [ ] Auth flow end-to-end with curl; reused rotated refresh token is rejected and family revoked
- [ ] `GET /users?page=2&per_page=10&sort=-created_at&q=alice` returns correct envelope meta
- [ ] Validation errors return 422 envelope with per-field messages
- [ ] Unit tests pass for both services

## Risk Assessment

- **Auth scope creep** (password reset, email verification, OAuth) — explicitly out of scope; document as extension points in `docs/api-guidelines.md`.
- **GORM soft-delete surprises** — `deleted_at` interplay with unique email; mitigate with partial unique index `WHERE deleted_at IS NULL`.
- **Token security** — never log tokens; hash refresh tokens at rest; JWT secret min length enforced in config.
