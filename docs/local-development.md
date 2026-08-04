# Local Development

Owns: Docker Compose walkthrough, service/port table, hot reload, database
access, troubleshooting.

## Quick start

```bash
make setup   # tool checks, .env from .env.example, git hooks
make dev     # build + start the full stack (attached; Ctrl-C to stop)
make seed    # once, in another terminal: seeded teacher accounts (phone-based)
```

Then open <http://localhost:5173> and log in with a seeded account (see
`apps/api/seeds/`). `make dev-down` stops the stack; `make dev-nuke` also
removes volumes (destroys local DB data); `make dev-logs` tails api + web.

## Services

| Service | URL / port | Notes |
|---|---|---|
| web | <http://localhost:5173> | Vite dev server, HMR; proxies `/api` to the API |
| api | <http://localhost:8080> | Gin + air hot reload; `/healthz`, `/readyz` |
| postgres | `localhost:5432` | `postgres:17-alpine`, data in `postgres-data` volume |
| adminer | <http://localhost:8081> | DB UI; server `postgres`, credentials from `.env` |
| migrate | — | one-shot `migrate up`, gates the API start |

Startup order is health-gated: postgres (healthy) → migrate (exit 0) → api
(healthy) → web. `docker compose ps` shows the states.

All configuration comes from the repo-root `.env` (compose interpolation).
Each service receives only its own variables — there is no blanket `env_file`.

## Networking model

Inside the compose network, services address each other by name
(`postgres:5432`, `api:8080`). The browser talks **same-origin** to the web
dev server: compose sets `VITE_API_URL=/api/v1` and Vite proxies `/api` — plus
`/public`, the root-mounted parent-statement routes that live outside
`/api/v1` — to `http://api:8080`. That sidesteps CORS and refresh-cookie
SameSite issues in dev. The API's published port `localhost:8080` remains available for curl and
host-mode tooling; host-mode web dev (`make web-dev`) keeps using the absolute
URL from `apps/web/.env.development`.

## Hot reload

- **Go**: the api container runs [air](https://github.com/air-verse/air);
  saving a `.go` file rebuilds and restarts the binary in a few seconds. The
  module cache persists in the `go-mod-cache` volume.
- **TypeScript**: Vite HMR through the bind mount; `node_modules` stays
  container-owned via an anonymous volume (host installs never leak in).
- Both watchers rely on filesystem events, which Docker Desktop bind mounts
  sometimes drop. If web edits stop being picked up, set `WEB_USE_POLLING=true`
  in `.env` and restart the web service. If Go edits stop triggering rebuilds,
  set `poll = true` (and optionally `poll_interval`) under `[build]` in
  `apps/api/.air.toml`.

## Database access

- Adminer: <http://localhost:8081>, system PostgreSQL, server `postgres`,
  user/password/database from `.env`.
- psql from the host: `psql postgres://teka:…@localhost:5432/teka` (published
  port), or `docker compose exec postgres psql -U teka teka`.
- Migrations/seeds run host-side against the published port: `make migrate-up`,
  `make seed`, `make migrate-status`. No Go toolchain on the host? Run the same
  seeder inside the stack: `docker compose run --rm migrate go run ./cmd/api seed`.
- Postgres and Adminer are published on `127.0.0.1` only — the dev password is
  committed in `.env.example`, so they are never reachable from the LAN. The
  web (`5173`) and api (`8080`) ports bind all interfaces for device testing.

## Troubleshooting

- **"address already in use"** — a stale process owns the port. Find it with
  `lsof -i :5173` (or `:8080`, `:5432`, `:8081`) and stop it; ports are
  deterministic on purpose, never remap to a free one.
- **api unhealthy on first start** — the first air build compiles the whole
  module tree; the healthcheck allows 120s. `make dev-logs` shows progress.
- **`variable is not set` / jwt secret error** — no root `.env`; run
  `make setup`.
- **schema errors after switching branches** — the `postgres-data` volume kept
  an old schema. `make migrate-up` applies forward; `make dev-nuke && make dev`
  resets from scratch (then `make seed` again).
- **`Cannot find module` after adding an npm dependency** — the container-owned
  `node_modules` volume is carried over from the previous container, so a plain
  `make dev` rebuild keeps the old packages. Recreate it once:
  `docker compose up -d --build --renew-anon-volumes web`.
- **Docker not running** — compose fails with "cannot connect to the Docker
  daemon"; start Docker Desktop first.
- **Windows/WSL: HMR dead** — classic bind-mount event gap; set
  `WEB_USE_POLLING=true` in `.env`.
