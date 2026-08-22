# Code Review: Docker, Docker Compose, Makefile (+ orphan scan)

Scope: `Makefile`, `docker-compose.yml`, `docker-compose.prod.yml`,
`docker-compose.homelab.yml`, `infrastructure/postgres/docker-compose.yml`,
`apps/{api,web}/Dockerfile{,.dev}`, `.dockerignore`, `apps/web/nginx.conf`,
`scripts/*.sh`, cross-checked against `.github/workflows/*`, `.github/dependabot.yml`,
`apps/api/internal/config/config.go`, `apps/web/vite.config.ts`, `docs/deployment.md`.

Verdict: overall solid — env contract compose↔Go config matches (prefix `API_`,
verified per struct tag), healthcheck/depends_on chain correct, loopback-only dev
ports, distroless prod image, VITE_API_URL build guard. Findings below.

## Findings

### M1 — Postgres major-version mismatch: dev 17 vs homelab prod 16 — FIXED
User confirmed production target is `postgres:16-alpine`. Dev compose and
docs/local-development.md aligned to 16 (2026-08-13). Existing dev
`postgres-data` volumes hold a PG17 data dir and need `make dev-nuke` once.

- `docker-compose.yml:8` → `postgres:17-alpine`; `infrastructure/postgres/docker-compose.yml:11` → `postgres:16-alpine`.
- Dev + CI e2e test against 17; production runs 16. Migration/SQL using 17-only
  behavior passes CI, fails prod. Align both (pick one, prefer 17 both sides or
  pin prod's real version in dev).

### M2 — `infrastructure/postgres/docker-compose.yml` undocumented; fragile network contract — FIXED
Added "Operator-run PostgreSQL" section to docs/deployment.md (role, separate
lifecycle rationale, `teka_default` project-name contract, first-boot order,
DSN host `teka-db`, version-sync note) and a version-sync comment in the
infrastructure compose file (2026-08-13).

- File header says "see docs/deployment.md" but deployment.md never mentions the
  file, `teka-db`, or `teka_default` (grep: 0 hits).
- Joins external `teka_default` — name only exists if the app stack runs with
  compose project name `teka` (default = repo dir name). `-p` or a different
  clone dir breaks DSN resolution silently. Document the contract in
  deployment.md or set `name: teka` in docker-compose.prod.yml to pin it.

### L1 — Stale `node_modules` anon volume after dependency bumps (dev web)
- `docker-compose.yml:114` anonymous volume shadows `/app/node_modules`. Compose
  reuses anon volumes on recreate, so after `package.json` changes `make dev
  --build` runs new image with OLD volume → module-not-found. Known gotcha;
  today only `make dev-nuke` (destroys DB data) fixes it. Options: named volume
  for node_modules, `up --build -V` (leaks dangling anon volumes), or a README
  note. Recommend named volume.

### L2 — Dependabot digest-pin claim only covers Dockerfiles, not compose images
- `.github/dependabot.yml` docker ecosystem watches `/apps/api`, `/apps/web` only.
- Tag-only, unwatched images: `postgres:17-alpine`, `adminer:5` (root compose),
  `postgres:16-alpine` (infrastructure — production DB). Add root + infrastructure
  dirs to the docker ecosystem or accept drift knowingly.

### L3 — Prod compose can't tune documented config knobs
- `docker-compose.prod.yml` passes through a fixed env list; `API_INVITE_TTL`,
  `API_RESET_TTL`, `API_RESET_COOLDOWN`, `API_DB_MAX_OPEN_CONNS`/`IDLE`/`LIFETIME`
  are not forwarded, so operators can't set them without editing the file.
  Acceptable for a reference topology; add passthrough lines if these are meant
  to be operator-tunable.

### I1 — Comment drift: `apps/api/Dockerfile:1` says `GIT_SHA=$(git rev-parse --short HEAD)`
- Makefile (`GIT_SHA := $(shell git rev-parse HEAD)`) and CI use the FULL sha on
  purpose (comment at Makefile:122 says so). Fix the Dockerfile comment.

## Orphan code

1. **`scripts/wait-for.sh` — orphan.** Zero callers: not in Makefile, CI,
   compose, docs, lefthook, package.json. Only referenced by historical plan
   files. Compose healthchecks + `depends_on` conditions replaced it. Delete.
2. **`Makefile` `fmt` target — permanent placeholder.** Prints "provisioned in a
   later phase" via `not_yet`; every phase shipped, formatting is actually
   enforced elsewhere (`lint-web` runs `format:check`, golangci-lint on api).
   The `not_yet` define (Makefile:9-11) exists ONLY for this target. Either
   implement (`gofmt -w` + `prettier --write`) or delete target + define.
3. **Dead guards in `dev*` targets (Makefile:27,31,35,39).** `if [ -f
   docker-compose.yml ]` — file is committed since Phase 7; the else-branch
   ("provisioned in Phase 7") is unreachable. Simplify to plain
   `docker compose ...`.

Not orphan (verified used): `infrastructure/docker/postgres/init/00-extensions.sql`
(mounted at docker-compose.yml:20), `apps/web/nginx.conf` (prod Dockerfile),
`apps/api/.air.toml` (Dockerfile.dev CMD air), `scripts/setup.sh` + `check-tools.sh`
(make setup), all root-compose services incl. `adminer` (dev DB UI).

## Non-issues (checked, documented decisions)
- Traefik `entrypoints=web` (HTTP) with https origins: TLS terminates at
  Cloudflare per deployment.md:154.
- No prod api compose healthcheck: distroless has no shell; /healthz //readyz
  probes documented in-file; homelab overlay adds Traefik LB healthcheck.
- Shared go-build-cache volume migrate↔api: serialized by
  `service_completed_successfully`.
- Committed dev DB password: ports bound 127.0.0.1 only.
- `homelab` overlay network replacement semantics (web loses default network):
  intentional, web never talks to api server-side.

## Unresolved questions
- None. (M1 resolved: production target of record is Postgres 16.)
