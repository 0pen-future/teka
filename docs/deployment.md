# Deployment

Owns: production images, migration execution strategy, runtime topology,
environment/secrets policy, health probe mapping.

Open decision: concrete deployment target (managed container platform,
Kubernetes, or VPS). Everything below is platform-agnostic; the reference
topology in [`docker-compose.prod.yml`](../docker-compose.prod.yml) shows how
the pieces fit on any single host.

The repository also includes a homelab-specific Traefik overlay in
[`docker-compose.homelab.yml`](../docker-compose.homelab.yml). It configures
application routing only; it does not provision DNS, Cloudflare Tunnel,
Traefik, the Docker network, or PostgreSQL.

## Images

CI publishes two images to GHCR on every merge to `main` (see
`.github/workflows/api-ci.yml` and `web-ci.yml`):

| Image | Base | Contents |
|-------|------|----------|
| `ghcr.io/OWNER/REPO/api` | `gcr.io/distroless/static-debian12:nonroot` | Single static Go binary; serves on 8080 |
| `ghcr.io/OWNER/REPO/web` | `nginxinc/nginx-unprivileged:1.29-alpine` | Static Vite bundle behind nginx; listens on 8080 |

Both run as non-root. Tags: `sha-<commit>` for every main build plus a moving
`latest`. Deploy by SHA tag — `latest` is for convenience, not for pinning.

Build the same images locally:

```bash
make build                 # both images: teka-api:local, teka-web:local
make build-image-api       # GIT_SHA is stamped automatically from git
make build-image-web VITE_API_URL=https://api.example.com/api/v1
```

### API image specifics

- The OpenAPI spec is regenerated inside the Docker build (`swag init`), so
  the image can never ship a stale spec regardless of what is in git.
- The binary is built with `-ldflags "-X main.version=<git sha>"`; verify what
  is running with `docker run --rm <image> --version`.
- Distroless: no shell, no package manager. Debug with `docker debug` or an
  ephemeral sidecar, not `docker exec sh`.

### Web image specifics

- `VITE_API_URL` is a **build argument**, baked into the bundle. The default
  `/api/v1` assumes same-origin serving (a reverse proxy routing `/api/*` —
  and `/public/*`, the root-mounted parent-statement routes that live outside
  `/api/v1` — to the API and everything else to the web container). For a
  split-origin topology, rebuild with the real API origin and add that web
  origin to the API's `API_CORS_ORIGINS`.
- nginx serves the SPA with history-API fallback (deep links resolve to
  `index.html`), immutable caching for hashed `/assets/*`, `no-cache` for
  `index.html`, and baseline security headers.

## Migrations

Migrations are embedded in the API binary (`go:embed`) and applied by the same
image that serves traffic, with the command overridden:

```bash
docker run --rm \
  -e API_ENV=production \
  -e API_DATABASE_URL="$API_DATABASE_URL" \
  -e API_JWT_SECRET="$API_JWT_SECRET" \
  ghcr.io/OWNER/REPO/api:sha-<commit> migrate up
```

Run this as a pre-deploy step (compose `migrate` service, Kubernetes Job or
init container, or a release-phase command on a PaaS) **before** the new API
version starts. Migrations are idempotent and forward-only in normal
operation; `migrate down` exists for local development and emergencies.

## Topology

```
                        ┌──────────────┐
   client ── https ───► │ reverse proxy│
                        │  / TLS edge  │
                        └──────┬───────┘
         /api/*, /public/* ────┤──── everything else
                        ┌──────▼───────┐      ┌──────────────┐
                        │  api :8080   │      │  web :8080   │
                        │ (distroless) │      │   (nginx)    │
                        └──────┬───────┘      └──────────────┘
                        ┌──────▼───────┐
                        │  PostgreSQL  │  managed (RDS/Cloud SQL/Neon)
                        └──────────────┘  or operator-run — not in compose
```

`docker-compose.prod.yml` encodes this shape (minus the proxy and the
database) with a one-shot `migrate` service gating the API start:

```bash
API_IMAGE=ghcr.io/OWNER/REPO/api:sha-<commit> \
WEB_IMAGE=ghcr.io/OWNER/REPO/web:sha-<commit> \
API_DATABASE_URL=postgres://... \
API_JWT_SECRET=... \
docker compose -f docker-compose.prod.yml up -d
```

TLS termination is the proxy's job (Caddy, Traefik, nginx, or the platform's
load balancer); neither image speaks TLS itself.

## Environment and secrets

The API is configured entirely through `API_*` environment variables:

| Variable | Required | Notes |
|----------|----------|-------|
| `API_ENV` | yes | `production` disables dev conveniences |
| `API_DATABASE_URL` | yes | Postgres DSN; use `sslmode=require` against managed Postgres |
| `API_JWT_SECRET` | yes | High-entropy secret; rotating it invalidates all sessions |
| `API_HTTP_PORT` | no | Defaults to 8080 |
| `API_JWT_ACCESS_TTL` / `API_JWT_REFRESH_TTL` | no | Default 15m / 720h |
| `API_LOG_LEVEL` | no | Use `info` in production |
| `API_CORS_ORIGINS` | no | Only for split-origin topologies |

Policy: secrets come from the platform's secret store (or an env file kept out
of git). Nothing under version control contains a production credential; the
only tracked env file is `.env.example` with dev defaults.

The web image takes no runtime configuration — everything is baked at build
time via `VITE_API_URL`.

## Homelab deployment with Traefik

This deployment exposes the API at
`https://teka-api.cauchuyenlaptrinh.com` and the web application at
`https://teka-web.cauchuyenlaptrinh.com`. API and web publish no host ports;
Traefik reaches port 8080 on each container through the external `homelab`
Docker network. The API also remains on Compose's private default network so it
can reach the migration job and the PostgreSQL address in its DSN. The
migration job never joins `homelab` and is not exposed through Traefik.

### Prerequisites

Provision these outside this repository before deploying:

- An existing external Docker network named `homelab`, shared with Traefik.
- Traefik's Docker provider configured with `exposedByDefault=false` and
  attached to `homelab`.
- Cloudflare Tunnel routes for both public hostnames, each targeting
  `http://traefik:80`. TLS terminates at Cloudflare; the Traefik routers use
  the internal `web` entrypoint.
- An external PostgreSQL instance reachable from the API host through
  `API_DATABASE_URL`.

### Prepare images and environment

Pin both images to immutable `sha-<commit>` tags. The web API URL is a Vite
build argument, not a runtime setting, so build or publish the web image with
the production API origin:

```bash
make build-image-api
make build-image-web \
  VITE_API_URL=https://teka-api.cauchuyenlaptrinh.com/api/v1
```

Those commands create local `teka-api:local` and `teka-web:local` images. Tag
and push them to your registry using the immutable commit SHA, then put those
exact references in `.env.production`. If CI publishes the images instead, set
the repository's `VITE_API_URL` variable to the production API URL before the
SHA-tagged web image is built:

```bash
docker tag teka-api:local ghcr.io/OWNER/REPO/api:sha-COMMIT
docker tag teka-web:local ghcr.io/OWNER/REPO/web:sha-COMMIT
docker push ghcr.io/OWNER/REPO/api:sha-COMMIT
docker push ghcr.io/OWNER/REPO/web:sha-COMMIT
```

Copy the tracked placeholder template to the ignored production file, then
replace every placeholder with the image references, database DSN, and
generated secrets for this deployment. The deliberately short `REPLACE_ME`
secret values fail API startup validation if they are not replaced:

```bash
cp .env.production.example .env.production
```

### Validate and start

Always pass the base production file first and the homelab overlay second:

```bash
docker compose --env-file .env.production \
  -f docker-compose.prod.yml \
  -f docker-compose.homelab.yml config

docker compose --env-file .env.production \
  -f docker-compose.prod.yml \
  -f docker-compose.homelab.yml up -d
```

Compose runs `migrate` first and starts the API only after migration exits
successfully. Traefik checks API readiness at `/readyz` before routing traffic.

### Verify and operate

Inspect all containers, including the completed migration job. Expect `api`
and `web` to be running, `migrate` to have exited with code 0, and no published
application ports. Compose does not define container healthchecks; the public
requests below verify readiness through the real Traefik and Tunnel path:

```bash
docker compose --env-file .env.production \
  -f docker-compose.prod.yml \
  -f docker-compose.homelab.yml ps --all

curl --fail https://teka-api.cauchuyenlaptrinh.com/readyz
curl --fail https://teka-web.cauchuyenlaptrinh.com/
```

Inspect logs with the same file order:

```bash
docker compose --env-file .env.production \
  -f docker-compose.prod.yml \
  -f docker-compose.homelab.yml logs --tail=200 api web migrate
```

For an update, change `API_IMAGE` and `WEB_IMAGE` in `.env.production` to the
new immutable SHA tags, ensure that the web image was built with the production
`VITE_API_URL`, run `config` again, and repeat `up -d`. To roll back, restore
the previous SHA tags and repeat the same validation and startup commands.
Database migrations are normally forward-only, so confirm migration
compatibility before rolling the API image back.

## Health probes

| Container | Probe | Endpoint | Notes |
|-----------|-------|----------|-------|
| api | Liveness | HTTP GET `/healthz` on 8080 | Static OK — process is up; never touches the DB |
| api | Readiness | HTTP GET `/readyz` on 8080 | Pings the database (1s timeout), 503 when unreachable — gate traffic on this one |
| web | Liveness/readiness | HTTP GET `/` on 8080 | Static nginx; any 200 means healthy |

Do not use `/healthz` for readiness: it returns 200 even when the API cannot
reach Postgres, so the orchestrator would route traffic to a replica that
fails every request.

The API image is distroless, so probes must be HTTP probes from the
orchestrator — there is no shell or wget inside the container for a
compose-style CMD healthcheck.

## Seeding

`seed` creates the development/demo users (idempotent, dev passwords). It is a
local and staging convenience — do not run it against production:

```bash
docker run --rm -e ... ghcr.io/OWNER/REPO/api:sha-<commit> seed
```
