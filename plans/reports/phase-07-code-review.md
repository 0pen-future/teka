# Phase 7 Code Review — Docker Compose Local Dev

Plan: `plans/260803-1552-fullstack-project-provisioning` · Phase file: `phase-07-docker-compose-local-dev.md`
Reviewer: `code-reviewer` subagent · Verdict: **DONE_WITH_CONCERNS** → all High/Medium findings fixed same-session → **PASS**

## Scope

10 files (6 new, 4 modified): `docker-compose.yml`, `apps/api/Dockerfile.dev`, `apps/web/Dockerfile.dev`, both `.dockerignore`s, `infrastructure/docker/postgres/init/00-extensions.sql`, `apps/web/vite.config.ts`, `apps/web/src/lib/config/env.ts`, `.env.example`, `docs/local-development.md`.

## Gate results (a–e)

| Check | Result |
|---|---|
| (a) Acceptance criteria met | PASS — criterion 3's "HMR without page reload" half was initially inferred, later proven directly (see Verification) |
| (b) No touchpoint regressions | PASS — reviewer empirically verified Vite 8 env precedence: compose `environment` beats `.env.development`, so the proxy path is genuinely used; host mode unchanged when `WEB_API_PROXY_TARGET` unset; only `env.ts` consumer is the axios `baseURL` |
| (c) No unannounced contract breaks | PASS — `VITE_API_URL` widening (root-relative now valid) and `apiOrigin` removal were intentional and zero-consumer |
| (d) Follows repo patterns | PASS |
| (e) No new lint/type/build errors | PASS — lint, typecheck, format, 22 vitest tests, `docker compose config -q` all green after fixes |

## Findings and resolutions

### High — fixed
- **H1 Postgres + Adminer published on 0.0.0.0 with committed default credentials** — anyone on the same LAN could open Adminer as the DB superuser (arbitrary SQL, `COPY … FROM PROGRAM`). Fixed: both ports now bind `127.0.0.1` only; web `:5173` and api `:8080` intentionally stay on all interfaces for device testing. Documented in `docs/local-development.md`.

### Medium — fixed
- **M1 Stale `node_modules` anonymous volume** — compose carries anonymous volumes across container recreation, so `make dev` after adding an npm dependency runs with old packages (`Cannot find module`). Fixed: troubleshooting entry documenting `docker compose up -d --build --renew-anon-volumes web`.
- **M2 No Go build cache volume** — every recreate recompiled the whole module tree (`start_period: 120s` was masking this). Fixed: `go-build-cache:/root/.cache/go-build` shared by migrate and api (safe: migrate exits before api starts; the cache is concurrency-safe regardless).
- **M3 Doc claimed air "polls" and is immune to Docker Desktop event gaps** — false: air uses fsnotify like Vite, and the phase risk item required polling toggles documented for *both*. Fixed: doc now covers `WEB_USE_POLLING` (Vite) and `poll = true` in `apps/api/.air.toml` (air).

### Low — fixed
- **L1** web port hard-coded → `${WEB_PORT:-5173}:5173` + `WEB_PORT` in `.env.example`.
- **L2** web healthcheck missing vs phase architecture table → added (wget `/`, start_period 30s).
- **L3** `env.ts` refine accepted protocol-relative `//host/path` → now requires `/` and rejects `//`.
- **L5** seeding required host Go toolchain → doc adds `docker compose run --rm migrate go run ./cmd/api seed`.
- **L6** `POSTGRES_PORT` / host-mode `API_DATABASE_URL` coupling → explicit "not linked, update both" comment in `.env.example`.
- **L7** `.env*` added to both `.dockerignore`s (build-context hygiene; Dockerfiles never copied them).

### Accepted, not fixed
- **L4** Any compose command fails without root `.env` (`API_JWT_SECRET` `:?` guard) — fail-loud is the intent; the missing-`.env` troubleshooting entry covers it. Trade-off: `make dev-down` also needs `.env`.
- **L8** `/healthz`/`/readyz` are not behind the Vite proxy — no web-side consumer exists; noted so nobody calls them from the browser and receives Vite's HTML.

## Verification (live, this session)

- Cold `docker compose up --build -d`: postgres healthy → migrate `Exited (0)` ("migrations applied") → api healthy → web healthy.
- `make seed`, then full Playwright suite **6/6** against the compose stack through the proxy at `:5173` (login, session across reload — refresh cookie survives the un-rewritten `/api` proxy — admin CRUD).
- Adminer 200 on `:8081`; after H1 fix, `netstat` shows `127.0.0.1.5432` / `127.0.0.1.8081` binds only.
- Go hot reload: edit → air `building… running…` within seconds.
- **HMR proven directly**: WebSocket client on Vite's HMR channel (inside the web container, module graph populated first) received `{"type":"update","updates":[{"type":"js-update",…login-page.tsx…}]}` on file edit — a hot update, not `full-reload`. Closes the reviewer's open question on criterion 3.
- Second `docker compose up -d` while running: identical container IDs, no port hops.
- `down` → `up`: seeded admin still logs in (named-volume persistence); migrate re-runs idempotently.
- Teardown clean: all four dev ports free.

## Notes

- `golang:1.26-alpine` (not 1.25, the module's `go` directive) is deliberate: air v1.67.4 requires go ≥ 1.26; the image builds the go-1.25 module fine.
- `adminer:5` pinned instead of the plan's `adminer:latest` (deterministic dev stack).
- Reviewer's open question 2 (LAN device testing) resolved by the H1 split: app ports on all interfaces, credentialed infra on loopback.
