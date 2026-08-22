# Scout Report — Build, Infra, CI/CD, Config Inventory (for docs-manager)

Date: 2026-08-19. Read-only scout. Source of truth = files cited; `docs/deployment.md`
and `docs/local-development.md` cross-checked only (they are accurate; discrepancies
flagged in §11).

Repo root: `/home/vmo/workspace/testing/teka`. Monorepo: `apps/api` (Go/Gin/GORM),
`apps/web` (React/Vite), root = tooling only.

---

## 1. Prerequisites & bootstrap

### Required tools (`scripts/check-tools.sh`)

`git`, `go`, `node`, `npm`, `docker` (+ `docker compose` plugin, checked
separately), `make`. Script exits 1 listing every missing tool.
**Version drift**: the script's hints say Go "1.22+" and Node "20+", but
`apps/api/go.mod` declares `go 1.25.0` (prod build image `golang:1.25-alpine`,
dev image `golang:1.26-alpine`) and CI + both Dockerfiles use **Node 22**.
Optional, host-mode only, non-fatal: `air`, `golangci-lint` (CI pins
golangci-lint **v2.7.2**; dev image pins **air v1.67.4**).

### `make setup` → `scripts/setup.sh`, step by step

(1) `cd` to repo root (script is location-independent). (2) `./scripts/check-tools.sh`
— abort if a required tool is missing. (3) `[ -f .env ] || cp .env.example .env`
(never overwrites an existing `.env`). (4) root `npm ci` (falls back to
`npm install`) — lefthook + commitlint only. (5) `apps/web` `npm ci`, same fallback.
(6) `npx lefthook install` — writes `.git/hooks`; `make hooks` re-runs this alone.
(7) prints "Setup complete. Run 'make dev'".

### Makefile target map (`Makefile`)

| Target | Action |
|---|---|
| `help` (default) | greps `## ` doc comments |
| `dev` / `dev-down` / `dev-nuke` / `dev-logs` | `docker compose up --build` / `down` / `down -v` / `logs -f api web` |
| `api-dev` / `web-dev` | host mode: `air` (falls back to `go run ./cmd/api serve`) / `npm run dev` |
| `test` = `test-api` + `test-web` | integration-tagged `go test` + **coverage floor 60%** (see §5) / vitest+MSW offline |
| `test-api-unit` | `go test -short ./...` (no Docker) |
| `coverage-api` | `go tool cover -html=coverage.out` |
| `api-docs` | `go tool swag init -g cmd/api/main.go -o docs --parseInternal` |
| `e2e` | `npm run e2e` (Playwright; needs running stack + seed) |
| `lint` = `lint-api` + `lint-web` | golangci-lint / eslint + prettier `--check` + tsc |
| `fmt` | **placeholder** — prints "provisioned in a later phase" (`not_yet`) |
| `migrate-up` / `migrate-down` / `migrate-status` / `seed` | `go run ./cmd/api <cmd>` on the host |
| `build` = `build-image-api` + `build-image-web` | `docker build --build-arg GIT_SHA=$(git rev-parse HEAD) -t teka-api:local apps/api` / `--build-arg VITE_API_URL=$(VITE_API_URL)` (`?= /api/v1`) `-t teka-web:local apps/web` |
| `build-api` / `build-web` | host binary (`CGO_ENABLED=0`) / `npm run build` |

`GIT_SHA` = **full** SHA (matches CI `github.sha`) so `api --version` prints
identical provenance regardless of build host; falls back to `dev` without git.

---

## 2. Local dev topology (`docker-compose.yml`)

### Services

| Service | Image / build | Host port binding | Purpose |
|---|---|---|---|
| `postgres` | `postgres:16-alpine` | `127.0.0.1:${POSTGRES_PORT:-5432}:5432` | Dev DB; major pinned to production major |
| `migrate` | build `apps/api/Dockerfile.dev`, cmd `go run ./cmd/api migrate up` | none | One-shot, exits 0, gates api |
| `api` | build `apps/api/Dockerfile.dev` (air) | `${API_HTTP_PORT:-8080}:8080` (all ifaces) | Gin API, hot reload |
| `web` | build `apps/web/Dockerfile.dev` | `${WEB_PORT:-5173}:5173` (all ifaces) | Vite dev server + HMR |
| `adminer` | `adminer:5` | `127.0.0.1:${ADMINER_PORT:-8081}:8080` | DB UI, `ADMINER_DEFAULT_SERVER=postgres` |

### Health-gating order

`postgres` (`pg_isready`, 3s/3s/10) → **healthy** → `migrate`
(`service_completed_successfully`) → `api` (`wget /healthz`, 5s/3s/12,
**`start_period: 120s`** — the first air build compiles the whole module tree) →
**healthy** → `web` (`wget /`, 5s/3s/12, `start_period: 30s`). Each service gets
only its own variables; there is no blanket `env_file`.

### Volumes

| Volume | Mounted at | Why |
|---|---|---|
| `postgres-data` | `/var/lib/postgresql/data` | DB persistence; wiped by `make dev-nuke` |
| bind `./infrastructure/docker/postgres/init` | `/docker-entrypoint-initdb.d:ro` | Extensions only (`citext`, `pgcrypto`); schema owned by migrations |
| `go-mod-cache` | `/go/pkg/mod` (migrate + api) | Seeded from the Dockerfile.dev `go mod download` layer |
| `go-build-cache` | `/root/.cache/go-build` (migrate + api) | Warm restarts; safe to share (migrate exits before api starts) |
| bind `./apps/api` → `/app`, `./apps/web` → `/app` | source | Hot reload |
| anonymous `/app/node_modules` | web | Container-built linux binaries shadow host `node_modules` |

### Same-origin Vite proxy model

Compose sets `VITE_API_URL=/api/v1` + `WEB_API_PROXY_TARGET=http://api:8080`.
`apps/web/vite.config.ts` reads the latter at config-load time (Node side, **not**
`VITE_`-prefixed → never in the bundle) and proxies **two** prefixes with **no
path rewrite**: `/api` (the API serves `/api/v1/...` and the refresh cookie is
scoped to `/api/v1/auth`, so the prefix must survive the hop) and `/public` (the
unauthenticated parent-statement routes mounted at the API **root**, outside
`/api/v1` — without proxying, the SPA fallback would answer that JSON with
`index.html`). Rationale: browser traffic is same-origin → no CORS, no
cookie-SameSite pain. Dev `API_CORS_ORIGINS` only covers direct-port usage
(curl, host-mode web).

### Hot reload mechanics

- **Go**: `apps/api/.air.toml` — `go build -o ./tmp/api ./cmd/api`, run with
  `args_bin = ["serve"]`, watch `.go` only, exclude `tmp bin docs testutil` and
  `_test.go`, `delay = 200`, `clean_on_exit`. Escape hatch when inotify events
  are dropped: `poll = true` (+ `poll_interval`) under `[build]`.
- **TS**: Vite HMR over the bind mount. Escape hatch: `WEB_USE_POLLING=true`.
  Vite `port: 5173`, `strictPort: true` (API CORS allowlist and docs reference
  it); `--host` in the dev CMD binds 0.0.0.0.

### Loopback-only bindings

`postgres` + `adminer` bind `127.0.0.1` **because the dev password is committed in
`.env.example`** — never LAN-reachable. `web` (5173) and `api` (8080) bind all
interfaces intentionally, for on-device testing. Ports are deterministic: on
"address already in use", kill the stale owner, never remap.

---

## 3. Environment variable reference

Loading model (`apps/api/internal/config/config.go`): all API vars read with
prefix `API_` via `caarlos0/env`. In development (or unset `API_ENV`) `godotenv`
loads `.env` from CWD, else `../../.env` (repo root when run from `apps/api`).
**Test and production read the process environment only.** `os.Getenv` forbidden
outside `internal/config` (forbidigo, §9). All API vars below are **runtime**;
`VITE_*` are **build-time** (baked into the bundle).

### PostgreSQL (dev compose interpolation only)

| Variable | Required | Default | Purpose | Secret |
|---|---|---|---|---|
| `POSTGRES_USER` | no | `teka` | DB role; also used in the healthcheck | no |
| `POSTGRES_PASSWORD` | no | `teka_dev_password` (committed dev placeholder) | DB password | dev: no / prod: **yes** |
| `POSTGRES_DB` | no | `teka` | DB name | no |
| `POSTGRES_PORT` | no | `5432` | Host-side published port (loopback) | no |
| `POSTGRES_PASSWORD` (`infrastructure/postgres` stack) | **yes** (`:?`) | none | Operator-run prod DB password; untracked `infrastructure/postgres/.env`; must match the DSN in `.env.production` | **yes** |

### API — core

| Variable | Required | Default | Purpose | Secret |
|---|---|---|---|---|
| `API_ENV` | no | `development` | One of `development\|test\|production`; anything else = startup error. Production: gin release mode, no `.env` load, no `/swagger`, JSON logs, hard secret validation | no |
| `API_HTTP_PORT` | no | `8080` | Listener port | no |
| `API_DATABASE_URL` | **yes** (`required,notEmpty`) | none | Postgres DSN. Dev compose builds its own from `POSTGRES_*` with host `postgres`; the `.env` value is only for host-mode | **yes** (prod) |
| `API_DB_MAX_OPEN_CONNS` | no | `25` | Pool cap | no |
| `API_DB_MAX_IDLE_CONNS` | no | `5` | Idle pool | no |
| `API_DB_CONN_MAX_LIFETIME` | no | `30m` | Conn recycle | no |
| `API_JWT_SECRET` | **yes** | none | Access/refresh signing. **Min 32 chars, enforced in every env.** Rotating invalidates all sessions | **yes** |
| `API_JWT_ACCESS_TTL` | no | `15m` | Access token life | no |
| `API_JWT_REFRESH_TTL` | no | `720h` (30d) | Refresh token life | no |
| `API_LOG_LEVEL` | no | `info` (Go default; dev compose sets `debug`) | `debug\|info\|warn\|error`, validated | no |
| `API_CORS_ORIGINS` | no | Go default `http://localhost:5173`; dev compose `http://localhost:5173,http://127.0.0.1:5173`; prod compose empty | Comma-separated explicit origins. `*` rejected (credentialed requests); each entry must start `http://`/`https://` (gin-contrib/cors panics on malformed origins at router build). Only needed for split-origin topologies | no |

### API — features (statements, bank, notifications, Zalo, onboarding)

| Variable | Required | Default | Purpose | Secret |
|---|---|---|---|---|
| `API_STATEMENTS_TOKEN_KEY` | **prod: yes** | none | HMAC-SHA256 key for statement link tokens. Accepts hex, std base64, raw-url base64, else raw bytes; **min 32 decoded bytes**. Prod: missing/short is fatal. Non-prod: random per-process fallback, only an 8-char SHA-256 fingerprint logged. **Rotating invalidates every link already sent to parents** | **yes** |
| `API_STATEMENTS_PUBLIC_BASE_URL` | no | `http://localhost:5173` | Origin `/s/{token}` links resolve against. **Also the base for invite and password-reset links** (deliberately no second base-URL key). Homelab overlay sets the real host | no |
| `API_ZALO_CRED_KEY` | **prod: yes** | none | AES-256-GCM key sealing linked Zalo session credentials at rest (account-takeover material). Same decoder + **min 32 decoded bytes**. Prod: fatal if missing/short; non-prod: random per-process fallback → previously linked accounts stop decrypting. **Losing/rotating it permanently orphans every linked account (every teacher re-scans a QR).** `.env.example` ships `REPLACE_ME`, deliberately too short so a placeholder can never reach production | **yes** |
| `API_BANK_CODE` / `API_BANK_ACCOUNT_NUMBER` / `API_BANK_ACCOUNT_NAME` | no | empty | VietQR transfer target on parent statements. All optional and unvalidated; unconfigured is a supported state — the QR block is omitted, never faked | no (business data) |
| `API_NOTIFICATIONS_DEFAULT_CHANNEL` | no | `zalo_manual` | Channel when a bulk send omits one (web UI always sends one explicitly) | no |
| `API_NOTIFICATIONS_MAX_MESSAGE_LEN` | no | `1000` | Char ceiling before a message collapses per-child detail into the link | no |
| `API_NOTIFICATIONS_PACE_MIN_SECONDS` / `..._MAX_SECONDS` | no | `3` / `8` | Random gap between two `zalo_personal` DMs. Min ≥ 1, max ≥ min. Anti-ban guardrail (Zalo publishes no limits — the range is a deliberate guess, hence configurable); invalid values = startup error, not a silent fast run | no |
| `API_NOTIFICATIONS_MAX_RUN_SIZE` | no | `50` | Max auto-delivered messages per bulk send; must be ≥ 1 | no |
| `API_INVITE_TTL` | no | `72h` | Owner-created invitation validity; must be > 0 | no |
| `API_RESET_TTL` | no | `48h` | Password-reset link validity; must be > 0 | no |
| `API_RESET_COOLDOWN` | no | `15m` | Min gap between reset requests per account; must be ≥ 0 | no |

The three onboarding TTLs are **absent from `.env.example` and from both compose
files** — defaults only; `docker-compose.prod.yml` does not forward them (nor
`API_DB_*`), so tuning requires editing the file. Documented in `docs/api-guidelines.md`.

### Web

| Variable | Where read | Required | Default | Purpose | B/R |
|---|---|---|---|---|---|
| `VITE_API_URL` | bundle (`src/lib/config/env.ts`, zod-validated at module load) | **yes** for prod image build | dev compose `/api/v1`; `apps/web/.env.development` `http://localhost:8080/api/v1`; Makefile/CI `/api/v1` | API base. Must be an absolute URL or a root-relative path (`//host` rejected as protocol-relative). **Public by design — never a secret** | **B** |
| `WEB_API_PROXY_TARGET` | `vite.config.ts` (Node) | no | unset → no proxy | Enables the same-origin `/api` + `/public` proxy; compose sets `http://api:8080` | R (dev only) |
| `WEB_USE_POLLING` | `vite.config.ts` (Node) | no | `false` | `usePolling` watcher escape hatch for Docker Desktop / WSL | R (dev only) |
| `WEB_PORT` | compose | no | `5173` | Host-published dev port | R |
| `ANALYZE` | `vite.config.ts` | no | unset | `npm run build:analyze` → `stats.html` treemap (gitignored) | B |
| `E2E_BASE_URL` | `playwright.config.ts` | no | `http://localhost:5173` | Playwright target | R (test) |

`apps/web/.env.development` is the **one** dotenv file exempted from `.gitignore`
(`!apps/web/.env.development`) because `VITE_*` values are public and localhost-only.
`.env.example`'s `VITE_API_URL` entry is **reference only — Vite never reads that file.**

### Adminer / prod compose

| Variable | Required | Default | Purpose |
|---|---|---|---|
| `ADMINER_PORT` | no | `8081` | Loopback-published UI port |
| `ADMINER_DEFAULT_SERVER` | fixed | `postgres` | Prefilled server field |
| `API_IMAGE` | **yes** (`:?`) | none | e.g. `ghcr.io/OWNER/REPO/api:sha-COMMIT`; used by both `migrate` and `api` |
| `WEB_IMAGE` | **yes** (`:?`) | none | `ghcr.io/OWNER/REPO/web:sha-COMMIT` |

`.env.production.example` is the tracked template (5 placeholders: `API_IMAGE`,
`WEB_IMAGE`, `API_DATABASE_URL`, `API_JWT_SECRET`, `API_STATEMENTS_TOKEN_KEY`,
`API_ZALO_CRED_KEY`). All secret slots are literal `REPLACE_ME` — **no real
credential is committed anywhere in the repo** (verified across `.env.example`,
`.env.production.example`, both compose files).

---

## 4. Production images

### API — `apps/api/Dockerfile`

| Aspect | Detail |
|---|---|
| Build stage | `golang:1.25-alpine`, **digest-pinned** (Dependabot docker ecosystem bumps it). Layer order `go.mod`/`go.sum` → `go mod download` → `COPY . .` |
| Swag | `go tool swag init -g cmd/api/main.go -o docs --parseInternal` runs **inside** the build; `docs` is in `.dockerignore`, so the swagger package is always regenerated from this source tree — the image can never ship a stale spec regardless of git state |
| Compile | `CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${GIT_SHA}"` |
| `GIT_SHA` | build arg, default `dev`; stamps `main.version`, surfaced by Cobra as `api --version` (`rootCmd.Version`) → `docker run --rm <image> --version` |
| Runtime stage | `gcr.io/distroless/static-debian12:nonroot`, digest-pinned, non-root uid baked in. Static binary `/api`, `EXPOSE 8080`, `ENTRYPOINT ["/api"]`, `CMD ["serve"]` |
| Distroless implications | **No shell, no package manager, no wget/curl** → no compose `healthcheck`, no `docker exec sh`. Debug via `docker debug` or an ephemeral sidecar; probes must be orchestrator-side HTTP probes |
| Same image, two roles | `serve` (default) and `migrate up` (command override) — migrations are `go:embed`ded |
| `.dockerignore` | `bin`, `tmp`, `coverage.out`, `docs`, `.env*` |

Dev image `apps/api/Dockerfile.dev`: `golang:1.26-alpine` (builds the go-1.25
module fine; **air ≥ 1.67 requires Go 1.26**), `air@v1.67.4`, pre-warmed module
cache, `CMD ["air"]`.

### Web — `apps/web/Dockerfile`

| Aspect | Detail |
|---|---|
| Build stage | `node:22-alpine`, digest-pinned; `npm ci` from `package-lock.json`; `npm run build` (= `tsc -b && vite build`) |
| `VITE_API_URL` | **build arg**, explicitly guarded: `RUN test -n "${VITE_API_URL}" \|\| exit 1`. `env.ts` only validates in the browser at boot, so without the guard a missing arg yields a green build that white-screens at runtime |
| Runtime stage | `nginxinc/nginx-unprivileged:1.29-alpine`, digest-pinned. **uid 101, listens on 8080** (non-root cannot bind 80) |
| Files | `nginx.conf` → `/etc/nginx/conf.d/default.conf`; `dist` → `/usr/share/nginx/html` |
| `.dockerignore` | `node_modules`, `dist`, `coverage`, `test-results`, `playwright-report`, `.env*` |

**Split-origin vs same-origin consequence:** the API origin is *frozen into the
JS bundle*. `/api/v1` (default) only works when one reverse proxy fronts both
containers and routes `/api/*` **and `/public/*`** to the API. For a split
origin you must (a) rebuild the web image with the absolute API URL and (b) add
the web origin to `API_CORS_ORIGINS`. The web image takes **zero** runtime
configuration.

### nginx SPA config (`apps/web/nginx.conf`)

| Location | Behavior |
|---|---|
| `/assets/` | `Cache-Control: public, max-age=31536000, immutable` (Vite content-hashed names), `try_files $uri =404` |
| `/api/` | `return 404` — hard guard so a proxy mis-route fails loudly instead of returning `index.html` with a 200 where JSON was expected |
| `/public/` | same guard for the root-mounted parent-statement API |
| `= /manifest.webmanifest` | `default_type application/manifest+json` (base mime.types has no mapping; a `types{}` block would drop every other mapping), `no-cache` |
| `/` | SPA history fallback `try_files $uri $uri/ /index.html`, `Cache-Control: no-cache` so a new deploy's asset manifest is picked up immediately |

Security headers repeated in **every** location block (nginx `add_header` does not
inherit once a nested level adds its own): `X-Content-Type-Options: nosniff`,
`X-Frame-Options: DENY`, `Referrer-Policy: strict-origin-when-cross-origin`, all
`always`. gzip on for css/js/json/svg, `gzip_min_length 1024`. **No CSP, no HSTS**
(TLS terminates upstream).

---

## 5. CI/CD (`.github/workflows/`)

### `api-ci.yml` — "API CI"

Triggers: PR + push to `master` on `apps/api/**`, the workflow file, and
`Makefile` (CI runs make targets, so Makefile edits must trigger it).
Concurrency `api-ci-${{ github.ref }}`, cancel-in-progress.
Permissions `contents: read` (build job adds `packages: write`).

| Job | Steps | Gate |
|---|---|---|
| `lint` | setup-go from `go-version-file: apps/api/go.mod`; `golangci/golangci-lint-action@v8` **v2.7.2**, workdir `apps/api` | merge |
| `test` | `make test-api` — integration tests via **testcontainers** (ubuntu runners ship Docker) | merge |
| `swagger-drift` | `make api-docs`, then `git status --porcelain -- apps/api/docs`; **non-empty = fail**. Uses `status` not `diff` so newly *untracked* generated files also fail | merge |
| `build` | `needs: [lint, test, swagger-drift]`; buildx; GHCR login **only on push**; `docker/metadata-action` tags `type=sha` + `type=raw,value=latest,enable={{is_default_branch}}`; `build-args: GIT_SHA=${{ github.sha }}`; `push: github.event_name == 'push'`; GHA build cache `mode=max` | publishes |

**Coverage floor** (`make test-api`, `API_COVERAGE_FLOOR := 60`):
`go test -tags=integration -coverpkg=./... -coverprofile=coverage.out` over **only
packages that contain tests** (`go list -f '{{if or .TestGoFiles .XTestGoFiles}}…'`)
— passing `./...` directly makes `go test` synthesize empty profiles for test-less
packages via the covdata tool, which auto-downloaded Go toolchains lack. Then
`go tool cover -func | tail -1 | awk` exits non-zero when total < 60.

### `web-ci.yml` — "Web CI"

Triggers: PR + push to `master` on `apps/web/**`, workflow file, `Makefile`,
`docker-compose.yml`, `.env.example` (the e2e job consumes the compose stack and
its env contract); plus **nightly cron `0 3 * * *`**. Concurrency
`web-ci-${{ github.event_name }}-${{ github.ref }}` — event name in the key so the
nightly cron can never cancel an in-flight release build.
`defaults.run.working-directory: apps/web`.

| Job | Steps |
|---|---|
| `lint` | node 22 + npm cache on `apps/web/package-lock.json`; `npm ci`; `npm run lint`; `npm run format:check`; `npm run typecheck` (identical to `make lint-web`) |
| `test` | `npm run test:coverage`; uploads `apps/web/coverage` artifact, 14-day retention. **No enforced web coverage threshold** |
| `build` | `needs: [lint, test]`; same GHCR/metadata/tag pattern as API; `build-args: VITE_API_URL=${{ vars.VITE_API_URL \|\| '/api/v1' }}` (repository variable override) |
| `e2e` | `if: push \|\| schedule` — quarantined off the PR loop |

**e2e sequence** (order matters): (1) `npm ci` **before** the stack starts —
compose bind-mounts `./apps/web` with an anonymous `node_modules` volume; if the
host dir doesn't exist Docker creates the mountpoint root-owned and a later
`npm ci` fails with EACCES. (2) `cp .env.example .env` at repo root (dev defaults,
no real secrets). (3) `docker compose up -d --build`. (4) Poll
`docker compose ps web --format '{{.Health}}'` for `healthy`, **120 × 5s = 10 min**
(a cold runner compiles the Go module tree twice: migrate, then api under air);
`web` healthy transitively implies `api` healthy via `depends_on`; on timeout dump
`ps` + `logs api web`, exit 1. (5) `docker compose run --rm migrate go run
./cmd/api seed`. (6) `npx playwright install --with-deps chromium`; `npm run e2e`.
(7) On failure upload `playwright-report` + `test-results`, 7-day retention.
(8) `if: always()` → `docker compose down -v`.

### GHCR publishing / tag scheme

Registry `ghcr.io/${{ github.repository }}/{api,web}`. Auth via
`docker/login-action` + `${{ secrets.GITHUB_TOKEN }}` (`packages: write`), **only
when `github.event_name == 'push'`** — PRs build for validation but never push.
Tags: `sha-<commit>` on every default-branch build **plus a moving `latest`**.
Policy: **deploy by SHA tag**; `latest` is convenience, not a pin.

### `security.yml` — "Security"

- **Triggers**: weekly cron `0 4 * * 1`, `workflow_dispatch`, and PRs touching
  the workflow file itself.

| Job | Scans |
|---|---|
| `govulncheck` | `go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...` in `apps/api` (version-pinned; Dependabot cannot bump a `go run` ref) |
| `npm-audit` | `npx audit-ci@^7 --high --allowlist GHSA-qwww-vcr4-c8h2` in `apps/web`. Triaged exception: react-router RSC-mode CSRF bypass — app is a plain SPA (`createBrowserRouter` data mode), RSC never enabled, no patched 7.x; fix lands in react-router v8. Remove the allowlist on that upgrade |
| `trivy` | Matrix `[api, web]`; builds the image locally (web with `VITE_API_URL=/api/v1`), `aquasecurity/trivy-action@v0.36.0`, `severity: HIGH,CRITICAL`, `limit-severities-for-sarif: true` (without it SARIF output silently drops the severity filter and exit-code fires on LOW/UNKNOWN), `ignore-unfixed: true`, `exit-code: 1`; uploads SARIF to code scanning per app (`if: always()`) |

gosec runs on every PR via golangci-lint; this workflow covers what needs a fresh
vuln DB rather than a code change. **Note in-file**: on a private repo the SARIF
upload requires GitHub Advanced Security; without it the step fails even when the
scan succeeded.

### `dependabot.yml`

Weekly, minor/patch **grouped per ecosystem** (majors stay individual):
`gomod:/apps/api`, `npm:/apps/web`, `npm:/`, `github-actions:/`,
`docker:[/apps/api, /apps/web]`.
**Gap**: the docker ecosystem does not watch the root `docker-compose.yml`
(`postgres:16-alpine`, `adminer:5`) nor `infrastructure/postgres/` (production DB image).

---

## 6. Deployment topologies

### Reference production (`docker-compose.prod.yml`)

Explicitly **not** turnkey — it documents how the published images fit together.

| Service | Notes |
|---|---|
| `migrate` | `${API_IMAGE}`, `command: ["migrate","up"]`, `restart: "no"`, `API_ENV=production`. Requires `API_JWT_SECRET`, `API_STATEMENTS_TOKEN_KEY`, `API_ZALO_CRED_KEY` even though it never serves — config validation runs first |
| `api` | `expose: 8080` (no host port), `restart: unless-stopped`, `depends_on: migrate service_completed_successfully`. **No compose healthcheck** (distroless has no shell) — point orchestrator probes at `/healthz` / `/readyz` |
| `web` | `expose: 8080`, `restart: unless-stopped`, **no `depends_on`** — nginx serves static files and never talks to the API container; the browser does, through the reverse proxy |

Dry-run without secrets: `docker compose -f docker-compose.prod.yml config`.
**Does NOT provision**: PostgreSQL (managed RDS/Cloud SQL/Neon or operator-run —
production data does not belong in an anonymous compose volume next to the app),
the reverse proxy / TLS edge (neither image speaks TLS), DNS, secrets storage.
**Single-instance constraint**: run exactly **one** API instance. Zalo
notification runs live in process memory (one per teacher) and each boot marks any
`running` run interrupted, so overlapping instances would flag each other's active
runs and invite duplicate messages on resume. **Deploy stop-then-start.**

### Homelab overlay (`docker-compose.homelab.yml`)

Routing overlay only, applied **after** the prod file:
`docker compose --env-file .env.production -f docker-compose.prod.yml -f docker-compose.homelab.yml <cmd>`.

- `api`: joins `default` + external `homelab`; sets `API_CORS_ORIGINS` and
  `API_STATEMENTS_PUBLIC_BASE_URL` to `https://teka-web.cauchuyenlaptrinh.com`;
  Traefik router `teka-api` on Host `teka-api.cauchuyenlaptrinh.com`, entrypoint
  `web`, LB port 8080, **LB healthcheck `/readyz` every 10s / 2s timeout**.
- `web`: joins `homelab` **only** (loses the default network — intentional, it
  never talks to the API server-side); router `teka-web`, LB port 8080.
- `migrate` stays off `homelab`, never exposed. `networks: homelab: external: true`.

**Does NOT provision**: the `homelab` Docker network, Traefik itself (must run
with `exposedByDefault=false`, attached to `homelab`), Cloudflare Tunnel routes
(both hostnames → `http://traefik:80`; TLS terminates at Cloudflare, hence the
plain `web` entrypoint), DNS, PostgreSQL.

### Operator-run PostgreSQL (`infrastructure/postgres/docker-compose.yml`)

Deliberately a **separate compose stack** so `docker compose down -v` on the app
stack can never touch production data.

- `postgres:16-alpine`, `container_name: teka-db`, named volume `teka-pgdata`,
  `restart: unless-stopped`, healthcheck `pg_isready -U teka -d teka` (5s/3s/12).
- **No published host port.** Joins the app stack's default network declared as
  external `teka_default`; DSN host `teka-db`:
  `postgres://teka:<pw>@teka-db:5432/teka?sslmode=disable`.
- **Network-name contract**: `teka_default` only exists when the app stack's
  compose project name is `teka` (derived from the repo dir name). `-p`, a
  `COMPOSE_PROJECT_NAME` override, or a renamed clone silently breaks it.
- **First-boot order**: the network is created by the *app* stack. On a fresh
  host: app stack `up -d` once (`migrate` exits non-zero while the DB is missing
  — expected), start the DB stack, repeat `up -d`.
- No initdb scripts here; migrations assert extensions idempotently. (The dev
  stack instead mounts `infrastructure/docker/postgres/init/00-extensions.sql` →
  `CREATE EXTENSION IF NOT EXISTS citext; … pgcrypto;`)

### Update / rollback

Set new SHA tags in `.env.production`, confirm the web image was built with the
production `VITE_API_URL`, re-run `config`, then `up -d`. Rollback = restore
previous SHA tags, same sequence — but migrations are forward-only in normal
operation, so confirm schema compatibility first.

---

## 7. Migration strategy in production

- Migrations are **embedded in the binary**: `apps/api/migrations/embed.go`
  (`//go:embed *.sql`) → `iofs` → golang-migrate (`internal/database/migrate.go`).
  Nothing ships separately; the image is self-contained. 8 versioned pairs today.
- Applied by the **same image that serves traffic**, command overridden to
  `migrate up`. Run as a pre-deploy step: compose `migrate` service, a Kubernetes
  Job/init container, or a PaaS release command — **before** the new API starts.
- `MigrateUp` treats `ErrNoChange` as success → **idempotent** across restarts.
  `migrate status` prints `version: N dirty: bool`, or "no migrations applied".

**Rollback** (`migrate down`): default one step, `--steps N` for N; `--steps <=0`
without `--all` errors ("must be at least 1 (use --all for a full rollback)");
`--all` does a full rollback but is **refused outside `API_ENV=development`
unless `--yes` is also passed**. Every `.up.sql` has a matching `.down.sql`.
`down` exists for local development and emergencies; normal operation is
forward-only.

---

## 8. Operations runbook material

### Probes (`apps/api/internal/server/health.go`)

| Endpoint | Semantics |
|---|---|
| `GET /healthz` | Static `{"status":"ok"}` 200. Liveness only — **never touches the DB**. Using it for readiness routes traffic to a replica that fails every request |
| `GET /readyz` | `database.Ping` with a **1s** timeout. 200 `{"status":"ok"}`, else **503** `{"status":"unavailable","reason":"database unreachable"}`. Gate traffic on this |
| web `GET /` | Static nginx; any 200 = healthy |

Both return plain bodies, **not** the `{success,data,meta,error}` API envelope —
consumers are orchestrators, not API clients.

### Logging

`internal/shared/logger`: `slog` — **JSON handler in production, text otherwise**,
stdout, level from `API_LOG_LEVEL`. Request middleware emits one structured line
per request: `request_id`, `method`, `path`, `status`, `latency_ms`, `client_ip`.
`path` is sanitized (`/public/statements/<token>/…` → `…/[redacted]/…`) so access
logs never become a standing credential leak. `SetTrustedProxies(nil)` →
`client_ip` is the socket address, not a forgeable `X-Forwarded-For` (revisit when
an LB with a known range fronts the API). Secret keys are never logged — only an
8-char SHA-256 fingerprint of a generated dev fallback key.

### Shutdown budgets (`internal/server/server.go`)

Signals `SIGINT`/`SIGTERM` via `signal.NotifyContext`; **graceful drain 10s**
(`shutdownTimeout`). HTTP server: `ReadHeaderTimeout` 5s, `ReadTimeout` 10s,
`WriteTimeout` 30s, `IdleTimeout` 120s.
On boot, `app.RunServer` starts the Zalo health probe and calls
`Notifications.ReconcileInterrupted` — any run left `running` by a dead process is
marked interrupted (resumable) before requests can observe it.

### Operator CLI (`api <cmd>`, Cobra; `internal/cli`)

| Command | Flags | Notes |
|---|---|---|
| `serve` | — | default `CMD` |
| `migrate up\|down\|status` | `--steps`, `--all`, `--yes` | §7 |
| `seed` | `--force` | Refuses production without `--force`. Idempotent; 2 teacher accounts (`+8490100000{1,2}`, dev passwords in `apps/api/seeds/seed.go`), 4 contacts, 5 students. Local/staging only |
| `create-center` | `--name`, `--owner-phone`, `--owner-name` (all required), `--generate`, **`--force`** | Center + owner account in **one atomic transaction** — never a center without an owner or vice versa. Center names are not unique: always creates, never updates. Onboards a center's first customer, or recovers a center whose owner was lost |
| `reset-password` | `--phone` (required), `--generate`, **`--force`** | Recovery path for accounts with no self-service route — most commonly a **center owner, deliberately excluded from forgot-password**. Rewrites the password and **revokes every refresh token**. Works on a disabled account **without changing its status** (stays disabled, still cannot log in) |
| `--version` | — | Prints the `GIT_SHA` ldflags stamp (`dev` when unstamped) |

Both destructive commands hard-require `--force`; `--generate` prints the password
**once** ("store this now, it will not be shown again").

### DB access paths

| Context | Path |
|---|---|
| Dev UI | Adminer `http://localhost:8081`, system PostgreSQL, server `postgres`, creds from `.env` |
| Dev psql | host: `psql postgres://teka:…@localhost:5432/teka`; container: `docker compose exec postgres psql -U teka teka` |
| Dev migrate/seed | `make migrate-up` / `migrate-status` / `seed`; without a host Go toolchain: `docker compose run --rm migrate go run ./cmd/api seed` |
| Prod | `docker run --rm -e API_ENV=production -e API_DATABASE_URL=… -e API_JWT_SECRET=… -e API_ZALO_CRED_KEY=… <api-image> migrate up` |
| Homelab DB | No published port; reachable only on `teka_default` as `teka-db` |

### Backup gaps (nothing in-repo covers these)

- **No backup/restore automation** — no `pg_dump` cron, scripted restore, or PITR
  config. `teka-pgdata` is a plain named volume. No retention policy, restore
  drill, or secret-rotation runbook.
- **`API_ZALO_CRED_KEY` must be backed up alongside the database**: a restored DB
  with a lost key leaves every linked Zalo account permanently undecryptable.
  Same coupling for `API_STATEMENTS_TOKEN_KEY` (all parent links die on rotation).
- No log shipping / metrics / tracing stack; logs are stdout only.

---

## 9. Quality gates & conventions

### golangci-lint (`apps/api/.golangci.yml`, schema `version: "2"`)

`default: none`, explicitly enabled: **govet, errcheck, staticcheck, revive,
gosec, ineffassign, misspell, forbidigo**.
Formatters: **gofmt** + **gci** with sections `standard → default → localmodule`.

| Rule / exclusion | Justification (from file) |
|---|---|
| forbidigo forbids `os.Getenv` / `os.LookupEnv` | "read configuration through internal/config, not the raw environment" |
| `internal/config/` exempt from forbidigo | The one package allowed to read the environment |
| `_test.go` exempt from gosec | Tests favor readability over hardened patterns |
| `internal/features/zalo/protocol/` exempt from gosec `G401\|G501` | Port of a reverse-engineered wire format; **md5 is dictated by Zalo** (IMEI, request signing, key derivation) and cannot be substituted |
| same path exempt from revive `exported:` | Wire structs stay comment-free so re-porting upstream changes remains a diff, not a rewrite |

### Frontend

- **eslint** (flat config): `js.recommended`, `tseslint.recommendedTypeChecked` +
  `stylisticTypeChecked` (type-aware via `projectService`), `react-hooks`,
  `react-refresh/vite`, `jsx-a11y`, `eslint-config-prettier` last; ignores
  `dist`, `coverage`. Two scoped overrides: `src/components/ui/**` (generated
  shadcn primitives) turns off `react-refresh/only-export-components` and
  `@typescript-eslint/array-type` to stay re-generatable; `src/features/statement/**`
  + `src/layouts/public-layout.tsx` ban imports of `@/features/auth*` and
  `@/lib/api/client` — the public statement route must render a neutral error
  page on 401/404, never trigger a refresh attempt or redirect.
- **prettier** (`.prettierrc`): `printWidth 100`, `singleQuote: false`,
  `trailingComma: "all"`; ignores `dist`, `coverage`, `node_modules`,
  `package-lock.json`, `test-results`, `playwright-report`.
- **tsc**: `npm run typecheck` = `tsc -b --noEmit`; production build is
  `tsc -b && vite build`, so type errors fail the image build.
- **vitest**: jsdom, `include: src/**/*.test.{ts,tsx}` only (Playwright owns
  `e2e/*.spec.ts`), setup `src/test/setup.ts`, MSW intercepts everything → fully
  offline; `VITE_API_URL` injected so `env.ts` boot validation passes.
- **Playwright**: `testDir: ./e2e`, `fullyParallel: false`, `workers: 1` (specs
  mutate the shared users table), timeout 30s, `trace: retain-on-failure`. 8
  specs: attendance, auth, billing, collections, forgot-password, invite-accept,
  roster, statement.

### Git hooks (`lefthook.yml`) + commits

`glob_matcher: doublestar` so `**` matches zero directories. `pre-commit` runs
**`parallel: false`** on purpose — `stage_fixed` re-stages files and concurrent
git index writes race on `.git/index.lock`.

| Hook | Command | Notes |
|---|---|---|
| pre-commit `go-fmt` | `gofmt -l -w {staged_files}` in `apps/api/` | `stage_fixed: true` |
| pre-commit `go-lint` | `golangci-lint run` **if installed** | silently skipped otherwise |
| pre-commit `web-format` / `web-lint` | `npx --no-install prettier --write` / `eslint --fix` on `{staged_files}` | each guarded by `[ -x node_modules/.bin/<tool> ] \|\| exit 0`; `stage_fixed` |
| commit-msg `commitlint` | `npx --no-install commitlint --edit {1}` | `commitlint.config.mjs` = `@commitlint/config-conventional` |

**Escape hatch: `LEFTHOOK=0 git commit …`** (in the `lefthook.yml` header and
README). Personal overrides `lefthook-local.yml` / `.lefthook-local/` gitignored.
Conventional Commits enforced. `.editorconfig`: LF, UTF-8, final newline, trim
trailing whitespace, 2-space indent; tabs for `*.go` + `Makefile`; no trim for `*.md`.

### Verified secret hygiene

`.gitignore` excludes `.env` and `.env.*` with exactly three exemptions:
`.env.example`, `.env.production.example`, `apps/web/.env.development`. Also
ignored: `apps/api/{tmp,bin}/`, `coverage.out`, `*.test`, `node_modules/`,
`dist/`, `apps/web/coverage/`, `playwright-report/`, `test-results/`, IDE dirs, `*.log`.

---

## 10. Troubleshooting (known failure modes)

| Symptom | Cause | Fix |
|---|---|---|
| `address already in use` | Stale process owns a deterministic port | `lsof -i :5173` (or `:8080`, `:5432`, `:8081`), stop it. **Never remap** |
| api unhealthy on first start | First air build compiles the whole module tree | Healthcheck allows 120s; watch `make dev-logs` |
| `variable is not set` / JWT secret error | No root `.env` (compose uses `${API_JWT_SECRET:?…}`) | `make setup` |
| Schema errors after branch switch | `postgres-data` volume holds an old schema | `make migrate-up` (forward) or `make dev-nuke && make dev` + `make seed` |
| `Cannot find module` after adding an npm dep | Anonymous `node_modules` volume is reused on recreate, so a plain `--build` keeps old packages | `docker compose up -d --build --renew-anon-volumes web` |
| Docker daemon unreachable | Docker not running | Start Docker Desktop first |
| HMR dead (Windows/WSL/Docker Desktop) | Bind-mount inotify event gap | `WEB_USE_POLLING=true` in `.env`, restart web |
| Go rebuilds stop firing | Same event gap | `poll = true` under `[build]` in `apps/api/.air.toml` |
| Web image builds green, white-screens | Missing/malformed `VITE_API_URL` | Dockerfile guard already fails the build; for host builds check `env.ts` zod message |
| Prod API refuses to start | `API_JWT_SECRET` < 32 chars, or `API_STATEMENTS_TOKEN_KEY` / `API_ZALO_CRED_KEY` < 32 decoded bytes, or bad `API_ENV`/`API_LOG_LEVEL`/`API_CORS_ORIGINS` | Regenerate with `openssl rand -base64 32`; `.env.production.example` placeholders are deliberately too short |
| Traefik never routes to the API | LB healthcheck on `/readyz` failing = DB unreachable | Check DSN, `teka-db` reachability, `teka_default` network name |
| Homelab first boot: `migrate` exits non-zero | DB stack not up yet (app stack creates the network) | Up app stack once, up DB stack, repeat app `up -d` |
| DSN host `teka-db` unresolvable | Compose project name ≠ `teka` (`-p`, `COMPOSE_PROJECT_NAME`, renamed clone dir) | Keep the project name `teka` |
| Trivy SARIF upload fails on a private repo | Needs GitHub Advanced Security | Scan itself still succeeded; treat upload as optional |
| e2e job EACCES on `npm ci` | Compose created the bind-mount root-owned before host install | Keep `npm ci` **before** `docker compose up` |
| Dev restart loses Zalo links / statement links | Non-prod fallback generates a random per-process key | Set real `API_ZALO_CRED_KEY` / `API_STATEMENTS_TOKEN_KEY` in `.env` |

---

## 11. Discrepancies & gaps for docs-manager to resolve

| # | Item |
|---|---|
| D1 | **Go drift**: `check-tools.sh` + `README.md` say "Go 1.22+"; `go.mod` declares `go 1.25.0`, prod image `golang:1.25-alpine`, dev image `golang:1.26-alpine` (air ≥1.67 needs 1.26). Docs should say **Go 1.25+** |
| D2 | **Node drift**: `check-tools.sh` says "20+"; CI + both Dockerfiles use **Node 22** |
| D3 | `apps/api/Dockerfile` header comment says `git rev-parse --short HEAD`; Makefile and CI deliberately use the **full** SHA. Comment drift only |
| D4 | `make fmt` is a permanent placeholder (`not_yet`) — formatting is actually enforced by `lint-web` (`format:check`) + golangci-lint. Do not document it as usable |
| D5 | `scripts/wait-for.sh` has **zero callers** (Makefile, CI, compose, docs, hooks all clean); compose healthchecks + `depends_on` replaced it. Do not document |
| D6 | `docker-compose.prod.yml` does not forward `API_INVITE_TTL`, `API_RESET_TTL`, `API_RESET_COOLDOWN`, `API_DB_MAX_OPEN_CONNS/IDLE/LIFETIME` |
| D7 | `.env.production.example` omits `API_STATEMENTS_PUBLIC_BASE_URL`; without the homelab overlay it defaults to `http://localhost:5173`, so **statement, invite, and reset links would point at localhost in a non-homelab production deploy**. Deployment guide must call this out |
| D8 | Dependabot's docker ecosystem does not watch root `docker-compose.yml` (`postgres:16-alpine`, `adminer:5`) or `infrastructure/postgres/` (production DB image) |
| D9 | No web coverage threshold (artifact only) vs. the API's hard 60% floor |
| D10 | No backup/restore/rotation automation or runbook anywhere (§8) |
| D11 | Makefile `dev*` targets carry unreachable `if [ -f docker-compose.yml ]` guards from an earlier phase |

### Unresolved questions

1. Is the 60% API coverage floor meant to rise, and should the web suite get an
   equivalent gate? (docs state the current number either way)
2. Is `latest` intended to stay published given the "deploy by SHA" policy?
3. Is backup/restore an explicit operator responsibility (out of scope), or is a
   runbook expected in the deployment guide?
