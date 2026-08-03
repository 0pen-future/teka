---
phase: 4
title: "Phase 4: Backend Testing and API Docs"
status: completed
priority: P1
effort: "1d"
dependencies: [3]
---

# Phase 4: Backend Testing and API Docs

## Overview

Harden the API with the full testing pyramid — unit tests (fake repositories), integration tests against real Postgres via testcontainers-go, HTTP-level handler tests — and generate OpenAPI docs with swaggo/swag served at `/swagger` in non-production.

## Requirements

- Functional: `make test-api` runs unit + integration suites; integration tests self-provision Postgres (Docker required, skipped with clear message when unavailable via `-short`).
- Functional: `/swagger/index.html` serves generated OpenAPI UI when `API_ENV != production`.
- Non-functional: tests are parallel-safe (one schema/database per test package); combined coverage reported; no test depends on seed data or execution order.

## Architecture

**Testing strategy (documented in `docs/api-guidelines.md`):**

| Layer | Tool | Scope | Speed |
|---|---|---|---|
| Unit | stdlib + `stretchr/testify` | services with hand-written fake repos; table-driven | ms |
| Integration | `testcontainers-go` + real migrations | repositories + service+repo through real SQL | s |
| HTTP | `httptest` + Gin engine | handler binding, envelope shape, auth middleware, status codes | ms |

- **Fakes over mocks**: hand-written in-memory fakes implementing repository interfaces live next to tests; no mockgen dependency (KISS). Introduce `mockery` only if interfaces exceed ~10 methods.
- `testutil/` package: `StartPostgres(t)` (container, run migrations, return `*gorm.DB`, auto-terminate via `t.Cleanup`), `NewTestServer(t, container deps)`, fixture builders (`testutil.User(t, db, WithRole("admin"))`).
- Integration files use `//go:build integration` tag; `make test-api` runs both (`go test ./... -tags=integration`); `make test-api-unit` for the fast loop.
- Coverage: `go test -coverprofile`; gate in CI (Phase 8) at a pragmatic floor (60% to start) rather than aspirational numbers.

**OpenAPI (swaggo/swag):**
- Annotations on handlers (`// @Summary`, `@Param`, `@Success` with envelope types, `@Security BearerAuth`); root metadata in `cmd/api/main.go`.
- `swag init` output to `apps/api/docs/` — committed, not gitignored (deviation from the original wording: the router imports the generated package, and the Phase 8 drift check needs a committed baseline to diff against); `make api-docs` target; CI regenerates and fails on drift (ensures annotations stay current).
- `gin-swagger` route mounted conditionally by env.

## Related Code Files

- Create: `apps/api/internal/testutil/{postgres.go,fixtures.go}` (moved under `internal/` so Docker test deps stay off the module's public surface; a router-level test covers the swagger env gate instead of a full test-server helper)
- Create: `apps/api/internal/features/users/{integration_test.go,handler_test.go}`
- Create: `apps/api/internal/features/auth/{integration_test.go,handler_test.go}`
- Modify: handler files (swag annotations), `cmd/api/main.go` (swag root info)
- Modify: `apps/api/internal/server/router.go` (conditional swagger route)
- Modify: root `Makefile` (`test-api`, `test-api-unit`, `api-docs`, coverage target)

## Implementation Steps

1. Build `testutil.StartPostgres` running embedded migrations against a testcontainer; verify parallel packages get isolated databases (random db name per package).
2. Write integration tests: users repository CRUD + pagination/filter SQL correctness; auth refresh rotation + family revocation; transaction rollback path (service error → nothing persisted).
3. Write HTTP tests: envelope shape on success/error, 401/403 matrix for role guard, validation 422 fields.
4. Add swag annotations to every endpoint; generate; mount swagger UI; spot-check parameter and security schemes render.
5. Wire Makefile targets + coverage output (`coverage.out`, `go tool cover -func` summary).
6. Document the strategy and how to run subsets in `docs/api-guidelines.md`.

## Success Criteria

- [x] `make test-api` green on a machine with Docker; `-short` path skips integration cleanly without Docker
- [x] Deliberately breaking a migration or SQL query fails an integration test (spot-verified)
- [x] `/swagger/index.html` shows all auth + users endpoints with request/response schemas
- [x] Coverage report generated; floor met

## Risk Assessment

- **Testcontainers flakiness in CI** — mitigate with container reuse off, explicit wait strategies (log + port), and generous startup timeout; CI runners must have Docker (declared in Phase 8 workflow).
- **Swagger annotation drift** — CI drift check makes stale docs a failing build instead of silent rot.
