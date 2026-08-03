---
phase: 7
title: "Phase 7: Docker Compose Local Dev"
status: todo
priority: P1
effort: "0.5d"
dependencies: [3, 5]
---

# Phase 7: Docker Compose Local Dev

## Overview

One-command local environment: `make dev` starts Postgres, migration runner, API (air hot reload), web (Vite dev server), and Adminer, with health-checked startup ordering, named volumes, and a single `.env` as the source of configuration.

## Requirements

- Functional: `make dev` on a clean machine reaches: migrated DB, API on `:8080`, web on `:5173` proxied to the API, Adminer on `:8081`.
- Functional: editing Go or TS source hot-reloads without container restarts.
- Non-functional: deterministic ports (no auto-increment on conflict — fail loudly per `process-management.md`); `make dev-down` stops everything; `make dev-nuke` also removes volumes.

## Architecture

**Services (`docker-compose.yml`):**

| Service | Image/Build | Port | Healthcheck | Depends on |
|---|---|---|---|---|
| `postgres` | `postgres:17-alpine` | 5432 | `pg_isready -U $user -d $db` | — |
| `migrate` | build `apps/api/Dockerfile.dev`, cmd `api migrate up` | — | — (one-shot) | postgres healthy |
| `api` | `apps/api/Dockerfile.dev` (air) | 8080 | `wget -qO- localhost:8080/healthz` | migrate completed successfully |
| `web` | `apps/web/Dockerfile.dev` (vite) | 5173 | HTTP / | api started |
| `adminer` | `adminer:latest` | 8081 | — | postgres healthy |

**Startup order:** `postgres (healthy)` → `migrate (service_completed_successfully)` → `api (healthy)` → `web`. Compose `depends_on.condition` encodes it; `/readyz` remains the API's own truth.

**Networking:** single default bridge network; containers address each other by service name (`API_DATABASE_URL=postgres://…@postgres:5432/…`). The browser reaches the API via Vite dev-server proxy (`/api` → `http://api:8080`) so the web app uses same-origin `VITE_API_URL=/api/v1` in compose — avoids CORS and cookie SameSite pain in dev; direct `localhost:8080` stays available for curl.

**Volumes:**
- `postgres-data` named volume (survives `dev-down`, removed by `dev-nuke`).
- Bind mounts for source: `./apps/api → /app` (air watches), `./apps/web → /app` with **anonymous volume for `/app/node_modules`** (container-owned deps, avoids host/linux binary mismatch).
- Go module cache named volume (`go-mod-cache:/go/pkg/mod`) for fast rebuilds.

**Environment:** root `.env` consumed by compose interpolation; each service gets only its own variables via `environment:` blocks (no blanket `env_file` into every container). Postgres init scripts in `infrastructure/docker/postgres/init/` (create extensions; app schema comes from migrations only).

**Dev images:**
- `apps/api/Dockerfile.dev`: golang base, install air + golang-migrate CLI is unnecessary (migrations embedded), entrypoint `air`.
- `apps/web/Dockerfile.dev`: node base, `npm ci`, `npm run dev -- --host` (poll watch flag documented for Docker Desktop file-event gaps).

**Hot reload:** air rebuild-and-restart for Go; Vite HMR for web (`server.watch.usePolling` toggle via env for macOS/Windows edge cases).

## Related Code Files

- Create: `docker-compose.yml`
- Create: `apps/api/Dockerfile.dev`, `apps/web/Dockerfile.dev`
- Create: `infrastructure/docker/postgres/init/00-extensions.sql`
- Modify: `apps/web/vite.config.ts` (proxy + polling toggle), root `Makefile` (`dev`, `dev-down`, `dev-nuke`, `dev-logs`), `.env.example` (compose vars), `docs/local-development.md` (full walkthrough: ports, URLs, troubleshooting)

## Implementation Steps

1. Write dev Dockerfiles; verify each builds standalone.
2. Write `docker-compose.yml` with healthchecks/conditions above; wire `.env` interpolation.
3. Add Vite proxy config; switch compose web env to `VITE_API_URL=/api/v1`.
4. Implement Makefile targets (`dev-logs` = `docker compose logs -f api web`).
5. Test cold start on empty volumes; test warm start; test `docker compose down && up` preserves data; `dev-nuke` resets.
6. Verify hot reload both stacks by touching a file each; verify seeded login via Adminer + web UI.
7. Write `docs/local-development.md` (startup flow diagram, service table, common failures: port busy → find owner via `lsof`, stale volume, Docker not running).

## Success Criteria

- [ ] Clean machine: `make setup && make dev` → login page works against seeded admin (after `make seed`) with no manual steps
- [ ] `docker compose ps` shows postgres/api healthy; migrate exited 0
- [ ] Go edit → air rebuild < ~5s; TS edit → HMR without page reload
- [ ] Adminer at `:8081` connects to the dev DB
- [ ] Second `make dev` while running does not spawn duplicates or hop ports

## Risk Assessment

- **File-watch gaps on Docker Desktop** — polling toggles documented for both air and Vite.
- **Migration/service race** — `service_completed_successfully` gate prevents the API starting on an unmigrated schema; migrate is idempotent so restarts are safe.
- **Cookie auth in dev** — same-origin proxy sidesteps SameSite issues; direct-port usage documented as curl-only.
