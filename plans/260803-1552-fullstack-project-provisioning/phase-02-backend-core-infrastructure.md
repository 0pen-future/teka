---
phase: 2
title: "Phase 2: Backend Core Infrastructure"
status: completed
priority: P1
effort: "1d"
dependencies: [1]
---

# Phase 2: Backend Core Infrastructure

## Overview

Bootstrap the Go application: module, Cobra CLI skeleton, typed env config, slog logging, GORM/Postgres connection, Gin server with middleware stack, error/response standards, graceful shutdown, and health/readiness endpoints. No business features yet — this phase ends with a running server that serves `/healthz`, `/readyz`, and a 404 JSON envelope.

## Requirements

- Functional: `go run ./cmd/api serve` starts an HTTP server; `/healthz` returns 200 always, `/readyz` returns 200 only when the DB pings.
- Functional: all Cobra commands exist (`serve`, `migrate`, `seed`, `admin`) — `migrate`/`seed`/`admin` may print "not yet implemented" until Phases 3.
- Non-functional: config fails fast with a clear message listing missing env vars; JSON logs in production, text in development; SIGINT/SIGTERM drain in-flight requests (10s timeout) then close the DB pool.

## Architecture

**Bootstrap flow:** `cmd/api/main.go` → `internal/cli.Execute()` → command constructs `config.Load()` → `app.NewContainer(cfg)` → dependencies. `main.go` stays under ~15 lines.

**DI — manual constructor injection.** `internal/app/container.go`:

```go
type Container struct {
    Cfg    *config.Config
    Log    *slog.Logger
    DB     *gorm.DB
    TxMgr  database.TxManager
}
// Feature wiring happens in server.RegisterRoutes(c *Container, r *gin.Engine):
//   repo := users.NewRepository(c.DB)
//   svc  := users.NewService(repo, c.TxMgr, c.Log)
//   users.RegisterRoutes(v1, users.NewHandler(svc), authMW)
```
Explicit and greppable; adopt `google/wire` only if wiring exceeds ~5 features (documented in `docs/architecture.md`).

**Config** (`internal/config`): nested structs tagged for `caarlos0/env/v11`; `godotenv.Load()` only when `API_ENV=development`. Validate on load (secret length, required URL). Never log secret values.

**Logging** (`internal/shared/logger`): `slog.New(JSONHandler|TextHandler)` by env; injected, never global. Request-scoped logger (with request_id) attached to context by middleware.

**Error handling** (`internal/shared/apperror`):

```go
type AppError struct { Code string; Status int; Message string; Err error; Fields map[string]string }
// constructors: NotFound, Invalid, Unauthorized, Forbidden, Conflict, Internal
```
Services return `*AppError` or wrapped errors; a single Gin error-mapping helper in `shared/response` converts to the envelope. Unknown errors → 500 with generic message, full detail logged only.

**Response standard** (`internal/shared/response`):

```json
{ "success": true,  "data": {...}, "meta": { "page": 1, "per_page": 20, "total": 134 } }
{ "success": false, "error": { "code": "VALIDATION_ERROR", "message": "...", "fields": {"email": "must be a valid email"} } }
```

**Server** (`internal/server`): `gin.New()` (no default middleware); order: requestid → logger → recovery → CORS; versioned group `/api/v1`; `http.Server` with sane timeouts (Read 10s / Write 30s / Idle 120s); graceful shutdown via `signal.NotifyContext` → `srv.Shutdown(ctx)` → close pgx pool.

**Database** (`internal/database`): GORM with `gorm.io/driver/postgres` (pgx underneath); pool tuning (`MaxOpenConns 25`, `MaxIdleConns 5`, `ConnMaxLifetime 30m`); `AutoMigrate` **never used** — golang-migrate owns schema (Phase 3). `TxManager` interface here; implementation in Phase 3.

**Middleware** (`internal/middleware`): requestid (UUID, `X-Request-ID` passthrough), logger (method, path, status, latency, request_id), recovery (500 envelope), CORS (origins from config), auth stubs compiled but unused until Phase 3.

## Related Code Files

- Create: `apps/api/go.mod`, `apps/api/cmd/api/main.go`
- Create: `apps/api/internal/cli/{root,serve,migrate,seed,admin}.go`
- Create: `apps/api/internal/config/{config.go,config_test.go}`
- Create: `apps/api/internal/app/{app.go,container.go}`
- Create: `apps/api/internal/server/{server.go,router.go,health.go}`
- Create: `apps/api/internal/middleware/{requestid,logger,recovery,cors}.go`
- Create: `apps/api/internal/shared/{logger,apperror,response}/...`
- Create: `apps/api/internal/database/postgres.go`
- Create: `apps/api/.air.toml`, `apps/api/.golangci.yml`
- Modify: root `Makefile` (`api-dev`, `lint-api`, `build-api` real implementations)

## Implementation Steps

1. `go mod init` (module path from repo remote, e.g. `github.com/<owner>/teka/apps/api`); pin Go version.
2. Add deps: `gin-gonic/gin`, `gorm.io/gorm`, `gorm.io/driver/postgres`, `spf13/cobra`, `caarlos0/env/v11`, `joho/godotenv`, `google/uuid`.
3. Implement config with `Load() (*Config, error)` + table-driven `config_test.go`.
4. Implement shared logger, apperror, response packages (in that order — response depends on apperror).
5. Implement middleware stack; requestid first (logger consumes it).
6. Implement database connection with ping-on-start and pool settings.
7. Implement server + router + health endpoints; `/readyz` does `sqlDB.PingContext` with 1s timeout.
8. Implement Cobra: `root.go` (persistent flags: `--config-from-env` only, no config files), `serve.go` wiring config→container→server; stub other commands.
9. Add `.air.toml` (watch `internal`, `cmd`; build `go build -o tmp/api ./cmd/api`; run `tmp/api serve`).
10. Configure `.golangci.yml`: `govet, errcheck, staticcheck, revive, gosec, ineffassign, misspell, gci` — keep the set small and strict.
11. Wire Makefile: `api-dev` (air), `lint-api` (golangci-lint run), `build-api` (CGO_ENABLED=0 build).

## Success Criteria

- [ ] `make api-dev` with Postgres up: server logs structured startup line; `/healthz` 200, `/readyz` 200; stop Postgres → `/readyz` 503 — *code-complete; `/healthz`, 404 envelope, and request-id covered by httptest; live check deferred to Phase 7 (Docker daemon unavailable at implementation time)*
- [x] Missing `API_JWT_SECRET` → process exits non-zero naming the variable
- [ ] Ctrl-C during an in-flight slow request completes the request before exit — *implemented (signal.NotifyContext → 10s drain → pool close; second signal force-quits); live check deferred to Phase 7*
- [x] `make lint-api` and `go test ./...` pass

## Risk Assessment

- **GORM + pgx version drift** — pin versions; smoke-test connection in `/readyz` from day one.
- **Config sprawl** — all env vars go through `config.Config`; direct `os.Getenv` outside `internal/config` is a lint-flagged convention documented in `docs/api-guidelines.md`.
