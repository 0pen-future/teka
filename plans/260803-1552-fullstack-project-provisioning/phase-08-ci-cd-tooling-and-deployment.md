---
phase: 8
title: "Phase 8: CI/CD, Tooling and Deployment"
status: completed
priority: P2
effort: "1d"
dependencies: [4, 6, 7]
---

# Phase 8: CI/CD, Tooling and Deployment

## Overview

Finish the operational shell: GitHub Actions CI with path-filtered jobs, security scanning and dependency updates, production multi-stage images, a production-shaped compose reference, and final documentation. After this phase the repo is clone-to-deployable.

## Requirements

- Functional: PRs run lint + typecheck + unit/integration tests for each app that changed; production images build in CI and push to GHCR on `main`.
- Functional: scheduled security workflow (govulncheck, gosec via golangci, `npm audit`, Trivy image scan) and Dependabot for gomod/npm/actions/docker.
- Non-functional: CI green-path under ~10 min via Go/npm caching; images are non-root, minimal, with pinned base digests.

## Architecture

**Workflows (`.github/workflows/`):**

- `api-ci.yml` — trigger on PR/push with `paths: [apps/api/**, .github/workflows/api-ci.yml]`. Jobs: `lint` (golangci-lint action), `test` (`make test-api` incl. testcontainers — ubuntu runners have Docker), `swagger-drift` (`swag init` + `git diff --exit-code`), `build` (compile + `docker build`, push `ghcr.io/<owner>/<repo>/api:{sha,latest}` on main via OIDC-scoped `GITHUB_TOKEN`).
- `web-ci.yml` — path-filtered to `apps/web/**`. Jobs: `lint` (eslint + prettier check + `tsc --noEmit`), `test` (vitest, coverage artifact), `build` (vite build + docker push on main). E2E job: Playwright against `docker compose up` of the full stack — `main` pushes + nightly schedule, not every PR (keeps PR loop fast).
- `security.yml` — weekly schedule + manual: `govulncheck ./...`, `npm audit --audit-level=high`, Trivy scan of both images (fail on HIGH/CRITICAL), upload SARIF to code scanning.
- `.github/dependabot.yml` — ecosystems: `gomod`, `npm` (root + `apps/web`), `github-actions`, `docker`; weekly, grouped minor/patch.

**Static analysis:** golangci-lint is the Go umbrella (config from Phase 2; includes gosec, staticcheck); TS side is eslint type-checked config (Phase 5). CI runs exactly the same `make lint` targets as local — no CI-only tool drift.

**Production images:**
- `apps/api/Dockerfile`: stage 1 golang build — run `swag init` **before** `go build` (generated `docs/docs.go` is gitignored, so every build path must regenerate it), then `CGO_ENABLED=0 -ldflags "-s -w -X main.version=$GIT_SHA"`; stage 2 `gcr.io/distroless/static`, non-root, `ENTRYPOINT ["/api"]` `CMD ["serve"]` — same image runs migrations via `command: ["migrate","up"]`.
- `apps/web/Dockerfile`: stage 1 node build (`VITE_API_URL` as build arg); stage 2 `nginx:alpine` with `nginx.conf`: SPA fallback to `index.html`, gzip, immutable cache for hashed assets, no-cache for `index.html`, basic security headers.

**Build optimization (web):** hashed assets via Vite defaults, route-level `React.lazy` code splitting for feature pages, `rollup-plugin-visualizer` wired behind `npm run build:analyze`.

**Deployment recommendation (kept platform-agnostic):** artifacts are the two GHCR images + migrations embedded in the API image. `docker-compose.prod.yml` documents the runtime shape: managed Postgres (not containerized in real prod), `migrate up` as pre-deploy step/job, API replicas behind a TLS-terminating proxy (Caddy/Traefik/ALB), web static container or CDN, secrets from the platform's secret store (never baked into images), `API_ENV=production` (JSON logs, no swagger, no seed). Health endpoints map to orchestrator liveness (`/healthz`) and readiness (`/readyz`) probes. Concrete target (Fly/Render/k8s/VPS) deferred — documented as an open decision in `docs/deployment.md`.

## Related Code Files

- Create: `.github/workflows/{api-ci.yml,web-ci.yml,security.yml}`, `.github/dependabot.yml`
- Create: `apps/api/Dockerfile`, `apps/web/{Dockerfile,nginx.conf}`
- Create: `docker-compose.prod.yml`
- Modify: root `Makefile` (`build` = both images), `docs/deployment.md` (full content), `README.md` (badges, final command table)
- Modify: `apps/web/src/app/router.tsx` (lazy route imports) if not already lazy

## Implementation Steps

1. Write production Dockerfiles; verify locally: `docker run api migrate up && docker run api serve` against a throwaway Postgres; verify nginx SPA fallback + cache headers.
2. Write `api-ci.yml` and `web-ci.yml` with actions/cache for Go build cache + npm; confirm path filtering by pushing single-app changes.
3. Add swagger-drift and prettier/tsc checks; align every CI step to an existing `make`/npm script.
4. Write `security.yml` + `dependabot.yml`; run security workflow manually once and triage findings to zero HIGH/CRITICAL.
5. Add e2e job (compose up → wait on healthchecks → seed → `make e2e`) on main/nightly.
6. Write `docker-compose.prod.yml` + `docs/deployment.md`; final README pass.
7. Open a test PR touching both apps → both pipelines run and pass; merge → images appear in GHCR.

## Success Criteria

- [x] PR touching only `apps/web` runs only web jobs (and vice versa)
- [x] Main merge publishes both images to GHCR; `docker run` of each works standalone
- [x] Security workflow completes with zero unaddressed HIGH/CRITICAL findings
- [x] Trivy/gosec/govulncheck + Dependabot all active; SARIF visible in code scanning tab
- [x] `docs/deployment.md` walks a new operator from images to a running deployment shape

## Risk Assessment

- **Testcontainers in CI** — ubuntu-latest has Docker; keep integration tests off macOS/Windows runners.
- **E2E cost/flakiness** — quarantined to main/nightly with artifacts (trace, video) on failure; PRs stay fast.
- **Secret leakage** — CI uses `GITHUB_TOKEN` only; `.env` gitignored; Trivy secret-scan mode on; conventional-commit hook already blocks accidental env commits (gitignore is the real guard).
