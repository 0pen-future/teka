# Phase 8 Code Review — CI/CD, Tooling and Deployment

**Verdict: DONE_WITH_CONCERNS** (one blocking-on-first-push defect: H1)

Scope: all uncommitted work on top of `32bf037` — 8 new files (`.github/workflows/{api-ci,web-ci,security}.yml`,
`.github/dependabot.yml`, `apps/api/Dockerfile`, `apps/web/{Dockerfile,nginx.conf}`, `docker-compose.prod.yml`)
and 14 modified files. ~550 LOC net.

The phase is substantially correct and unusually well-commented. The workflows are structurally sound
(path filters, permissions, concurrency, caching, GHCR wiring all check out), the images are genuinely
minimal and non-root, and the Go/React touchpoints carry no business-logic regression. Two defects would
only surface after the repo gets a remote, and one documentation claim would misconfigure a production
readiness probe. Everything below is fixable in well under an hour.

---

## High

### H1 — `aquasecurity/trivy-action@0.28.0` does not exist; the Trivy job fails at action resolution

`.github/workflows/security.yml:60`

The action's tags are `v`-prefixed. The **only** bare-numeric tag in the entire repository is `0.35.0`:

```
$ git ls-remote --tags https://github.com/aquasecurity/trivy-action.git | grep "^[0-9]"
0.35.0
```

`v0.28.0` exists; `0.28.0` does not. GitHub will abort the `trivy` job with
`Unable to resolve action aquasecurity/trivy-action@0.28.0, unable to find version`, before any scan runs.
`actionlint` passes (it does not resolve remote refs), which is exactly why this survived local verification.

This breaks success criteria **3** (zero unaddressed HIGH/CRITICAL — nothing is scanned) and **4**
(SARIF visible in code scanning — nothing is uploaded, because `upload-sarif` has no file to read;
`if: always()` makes it run and then fail on the missing `trivy-api.sarif`).

**Verified.** Every other action ref in all three workflows resolves cleanly (checked all 10).

Fix: `uses: aquasecurity/trivy-action@v0.36.0`.

### H2 — SARIF format silently discards `severity`, so `exit-code: 1` fires on LOW/UNKNOWN findings

`.github/workflows/security.yml:60-66`

From the action's own `entrypoint.sh` (v0.36.0, lines 75-82):

```sh
# Handle SARIF
if [ "${TRIVY_FORMAT:-}" = "sarif" ]; then
  if [ "${INPUT_LIMIT_SEVERITIES_FOR_SARIF:-false,,}" != "true" ]; then
    echo "Building SARIF report with all severities"
    unset TRIVY_SEVERITY
```

`severity: HIGH,CRITICAL` is **unset** because `format: sarif` and `limit-severities-for-sarif` defaults to
false. Trivy then reports every severity, and `exit-code: "1"` is evaluated against that full report. Two
consequences:

1. The job fails on any LOW/MEDIUM/UNKNOWN finding in `distroless/static` or `nginx-unprivileged` — not the
   HIGH/CRITICAL gate the comment and criterion 3 describe. It will go red for reasons nobody intends to act on.
2. The code scanning tab is flooded with all severities instead of the HIGH/CRITICAL set.

Also missing: `ignore-unfixed: true`. Without it, base-image CVEs with no upstream fix will keep the weekly
job permanently red with no remediation path, which trains the team to ignore it.

**Verified** against the action source.

Fix:
```yaml
- uses: aquasecurity/trivy-action@v0.36.0
  with:
    image-ref: scan-target:${{ matrix.app }}
    format: sarif
    output: trivy-${{ matrix.app }}.sarif
    severity: HIGH,CRITICAL
    limit-severities-for-sarif: true
    ignore-unfixed: true
    exit-code: "1"
```

### H3 — `docs/deployment.md` points the readiness probe at `/healthz`, which does not check the database

`docs/deployment.md:126`, `docker-compose.prod.yml:49-50`, vs `apps/api/internal/server/health.go:19-31`

The code has two distinct endpoints:

```go
r.GET("/healthz", ...)  // static {"status":"ok"} — no DB touch
r.GET("/readyz", ...)   // database.Ping with a 1s timeout, 503 on failure
```

The doc table says `/healthz` … "Checks DB connectivity; use for liveness and readiness". Both halves are
wrong, and `/readyz` is never mentioned anywhere in the deployment docs or the prod compose file. An operator
following this page wires a readiness probe that returns 200 while the API cannot reach Postgres — the
orchestrator routes traffic to a pod that 500s every request, and rolling deploys never fail-safe.

The phase plan itself specified the correct mapping: *"Health endpoints map to orchestrator liveness
(`/healthz`) and readiness (`/readyz`) probes."* The implementation dropped it.

**Verified** against source.

Fix: split the table row into liveness `/healthz` and readiness `/readyz`, and update the
`docker-compose.prod.yml:49-50` comment.

---

## Medium

### M1 — Web image builds green without `VITE_API_URL` and ships a blank page; the Dockerfile comment claims the opposite

`apps/web/Dockerfile:3-4,16-18`

The header comment asserts: *"the build fails loudly if it is missing or malformed (src/lib/config/env.ts
validates at boot)"*. It does not. `env.ts` validates in the **browser** at module load, not at build time.

Verified empirically — `docker build -t teka-web:noarg apps/web` (no `--build-arg`) exits 0, and the emitted
bundle contains:

```
Parse({BASE_URL:`/`,DEV:!1,MODE:`production`,PROD:!0,SSR:!1,VITE_API_URL:``});if(!wf.success){...
```

(versus `VITE_API_URL:`/api/v1`` in the correctly-built `teka-web:local`). Empty string fails the zod
`refine`, `env.ts` throws during module evaluation before React mounts → white screen, console error only.

This is the same failure mode `plans/reports/phase-05-code-review.md:48` raised as H1 ("`make build-web`
produces a bundle that throws at boot"). It was never fixed, and Phase 8 now carries a comment stating it *is*
guarded. Both CI and the Makefile always pass the arg, so nothing is broken today — but the safety claim in
the file is false, which is worse than no claim.

**Verified.**

Fix — one line makes the comment true:
```dockerfile
ARG VITE_API_URL
RUN test -n "$VITE_API_URL" || { echo "VITE_API_URL build-arg is required" >&2; exit 1; }
ENV VITE_API_URL=${VITE_API_URL}
```

### M2 — The root `Makefile` is CI's command surface but is in no workflow's path filter

`.github/workflows/api-ci.yml:8-15`, `web-ci.yml:8-16`

`api-ci` runs `make test-api` and `make api-docs`. A PR that touches only the root `Makefile`,
`docker-compose.yml`, or `.env.example` triggers **zero jobs** — a broken `test-api` target, a compose change
that breaks the e2e job, or a removed `.env.example` key all merge to main unverified, then blow up on the
next unrelated `apps/api/**` PR.

**Verified by inspection.**

Fix: add `Makefile` to both filter lists; add `docker-compose.yml` and `.env.example` to `web-ci` (the e2e job
consumes both).

### M3 — nginx SPA fallback answers `/api/*` with `200 text/html`

`apps/web/nginx.conf:26-32`

Verified against the built image:

```
$ curl -D - http://localhost:18099/api/v1/users
HTTP/1.1 200 OK
Content-Type: text/html
Content-Length: 701
```

With the default `VITE_API_URL=/api/v1` and the reverse proxy missing or mis-routed (the exact shape
`docker-compose.prod.yml` publishes — `web` on `:80` with no proxy in the file), every API call returns
`index.html` with a 200. Axios does not throw; the app gets HTML where it expects JSON and fails in whatever
obscure way each call site handles a malformed body. A misconfiguration that should be a loud 502 becomes a
silent data-shaped bug.

**Verified.**

Fix — make the mis-wire loud:
```nginx
# The API is served by the reverse proxy, never by this container. Without
# this, the SPA fallback would answer /api/* with 200 index.html.
location /api/ { return 404; }
```

### M4 — e2e job waits on `api` health but Playwright targets `web`

`.github/workflows/web-ci.yml:117-128`, `apps/web/playwright.config.ts` (`baseURL: http://localhost:5173`)

The wait loop polls `docker compose ps api --format '{{.Health}}'` only. `web` starts *after* `api` is healthy
(`depends_on: api: service_healthy`) and has its own 30s `start_period`. `npm ci` + `playwright install
--with-deps chromium` usually covers the gap, but nothing guarantees it — this is a latent flake on a job that
only runs on main and nightly, i.e. the worst place to debug one.

I did verify the loop's mechanics are sound: `docker compose ps <svc> --format '{{.Health}}'` returns
`healthy` as expected (tested locally against the dev stack's postgres), and GitHub does not interpolate
`{{...}}` — only `${{...}}`.

**Race is suspected, not observed.** Fix: poll `web` too (or instead — it transitively implies `api`).

### M5 — 300s health budget may be too small for a cold CI cache

`.github/workflows/web-ci.yml:119-128`

The loop allows 60 × 5s = 300s. On a GitHub runner the `go-mod-cache` and `go-build-cache` volumes are empty,
so the sequence is: `migrate` compiles the module tree from scratch (`go run ./cmd/api migrate up`), then `api`
compiles it again under `air` (`start_period: 120s` exists precisely because this is slow). Two cold Go builds
of a Gin+GORM+testcontainers module tree plausibly exceed 300s.

**Suspected** — not verifiable without a remote. The failure is at least diagnosable (the step dumps
`docker compose logs api`). Consider 90 × 5s, or add a `actions/cache` step for the Go module cache.

---

## Low

| # | File:line | Finding |
|---|-----------|---------|
| L1 | `security.yml:24` | `govulncheck@latest` is the only unpinned dependency in a phase that digest-pins base images and version-pins golangci-lint. A breaking govulncheck release reds the job with no diff to blame. Pin the version. |
| L2 | `web-ci.yml:57` | `npx vitest run --coverage` is not a package script, breaking the "CI runs the same targets as local" claim in `api-ci.yml:3` and `README.md:44-46`. Add `"test:coverage"` to `package.json` and call it. Related: the web side has no coverage floor while the API enforces 60% (`Makefile:59`) — coverage is collected and archived but never gated. |
| L3 | `README.md:4-7` | Badges ship with literal `OWNER/REPO`, so the README renders three broken images until someone remembers the comment. Consider omitting them until the remote exists. |
| L4 | `docker-compose.prod.yml:11-12` | Comment shows `api:<sha>`; `metadata-action`'s `type=sha` actually emits `sha-<7hex>`. `docs/deployment.md:21` gets it right — align the compose header. |
| L5 | `Makefile:121` vs `api-ci.yml:87` | Local builds stamp the **short** sha, CI stamps the **full** `${{ github.sha }}`. `--version` output differs by build path, which undercuts its purpose as a provenance check. |
| L6 | `api-ci.yml:59` | `git diff --exit-code -- apps/api/docs` ignores newly-*untracked* files. `git status --porcelain -- apps/api/docs` is strictly stronger for the same cost. (Low impact: swag emits exactly the three tracked files today.) |
| L7 | `apps/web/src/app/router.tsx:18` | `HydrateFallback: () => null` renders a blank frame while the first lazy chunk loads on any deep link. Correct (it silences the router warning), but a minimal skeleton would be better UX than nothing. |
| L8 | `web-ci.yml:17-18,20-22` | `paths` filters do not apply to `schedule`, so the nightly cron also runs `lint`/`test`/`build`. Harmless but wasteful. More notable: the nightly shares `concurrency.group` with pushes to main (`github.ref` is identical), so a 03:00 cron can cancel an in-flight release build. Add `github.event_name` to the group key. |
| L9 | `security.yml:7-10` | No `pull_request` trigger and no path filter, so edits to `security.yml` itself are never validated before merge — the workflow is only exercised weekly. Adding a `pull_request` trigger on `paths: [.github/workflows/security.yml]` closes the loop. |
| L10 | `security.yml:41` | `audit-ci` runs only in `apps/web`; the root `package-lock.json` (commitlint/lefthook) is never audited, though Dependabot does cover it. |
| L11 | `dependabot.yml` | No `open-pull-requests-limit` (defaults to 5/ecosystem × 5 ecosystems = up to 25 open PRs). Also note the `docker` ecosystem will propose bumps for `Dockerfile.dev` alongside the production Dockerfiles — probably desirable, worth knowing. |
| L12 | `docker-compose.prod.yml:60-61` | `web.depends_on: api` is decorative — nginx never talks to the API container (there is no proxy block). Harmless, but it implies a dependency that does not exist. |
| L13 | `security.yml:67-71` | `upload-sarif` requires GitHub Advanced Security on private repos. If this repo lands private without GHAS, the step fails regardless of H1/H2. Worth a note in the workflow comment. |

---

## Gate results

### (a) Acceptance criteria

| # | Criterion | Result |
|---|-----------|--------|
| 1 | PR touching only one app runs only that app's jobs | **PASS with gap (M2)** — filters are correct for `apps/api/**` / `apps/web/**`; the root `Makefile` that CI depends on is unfiltered |
| 2 | Main merge publishes both images; `docker run` works standalone | **PASS** — `push: ${{ github.event_name == 'push' }}` + `packages: write` + GHCR login are all correct; standalone run independently verified locally (I re-confirmed the web image serves assets with the documented headers) |
| 3 | Security workflow, zero unaddressed HIGH/CRITICAL | **FAIL** — H1 (job cannot start) and H2 (gate is not HIGH/CRITICAL-scoped) |
| 4 | Trivy/gosec/govulncheck + Dependabot active; SARIF in code scanning | **PARTIAL** — gosec confirmed enabled (`apps/api/.golangci.yml:11`) and govulncheck/Dependabot are correct; Trivy + SARIF blocked by H1 |
| 5 | `docs/deployment.md` walks an operator from images to a running shape | **PASS with defect (H3)** — the walkthrough is complete and accurate (env var names, `swag`-in-build, `migrate up` as pre-deploy, secrets policy, topology all check out against source) except the probe table |

### (b) No business-logic regression

**PASS.**

- `cli.Execute` — grepped the whole module: exactly one caller (`apps/api/cmd/api/main.go:22`), zero test
  callers, and the package is `internal/` so no external consumer can exist. `rootCmd.Version = version` is set
  before `Execute()`, which is where Cobra registers the `--version` flag — correct ordering.
- `route.lazy` conversion — all five pages are still reachable and every dynamic import resolves to a real
  named export (`LoginPage`, `RegisterPage`, `DashboardPage`, `UsersPage`, `UserDetailPage`, all verified
  present). Auth guards are untouched: `ProtectedRoute` still wraps `DashboardLayout` at
  `router.tsx:25-29`, and the lazy children sit *inside* it, so no route escapes the guard. `path: "*"` →
  `NotFound` unchanged. Confirmed in the built image that each page is its own chunk
  (`login-page-*.js`, `register-page-*.js`, `dashboard-page-*.js`, `users-page-*.js`).
- `vite.config.ts` — visualizer is spread-guarded behind `ANALYZE=true`, so the default build path is byte-identical.

### (c) Public contract changes

**PASS, contained.** `make build` changed from host artifacts to Docker images. Grepped every caller: only
`README.md:42`, `docs/deployment.md:27`, and a `docker-compose.prod.yml:10` comment — all updated in this diff.
No script, no `lefthook.yml` command, no CI step invokes it. `build-api`/`build-web` are preserved with the
same behavior, so anything relying on `apps/api/bin/api` or `apps/web/dist` has a migration path. The one
unstated consequence: `make build` now **requires Docker**, where it previously did not.

### (d) Repo patterns

**PASS.** Comment style (intent + gotcha, not restatement) matches Phases 2/5/7 closely — the nginx
`add_header` inheritance note, the distroless-has-no-shell rationale, and the `.dockerignore`-excludes-`docs`
explanation are all the kind of comment this repo writes. Compose conventions carry over from Phase 7
(one-shot `migrate` gating `api`, `${VAR:?}` for required secrets, explicit "reference, not turnkey" framing).
Make-target parity holds on the API side; L2 is the one drift on the web side.

### (e) Lint / type / build

**PASS.** Re-verified this session: `go build ./...` and `go vet ./...` clean; `npm run typecheck` clean;
`actionlint` exits 0 across all three workflows. Note that actionlint's clean pass is exactly what masked H1 —
it validates syntax and expressions, never remote action resolution.

---

## Verified vs. suspected

**Verified (ran a command or read the source):** H1 (`git ls-remote --tags`), H2 (trivy-action
`entrypoint.sh:75-82` at v0.36.0), H3 (`health.go:19-31`), M1 (built the image without the arg, grepped the
baked `VITE_API_URL:``` from the bundle), M2 (path filters read), M3 (`curl` against `teka-web:local` returned
`200 text/html` for `/api/v1/users`), M4's tooling half (`docker compose ps --format '{{.Health}}'` → `healthy`),
all of (b)/(c)/(e), plus: `.env.example:19` does contain `API_JWT_SECRET` (compose `:?` interpolation satisfied);
root `package.json` exists and is tooling-only, so the `/` npm Dependabot entry is valid; `apps/api/docs` is
tracked, so the swagger-drift diff works; `go.mod:129` has the `tool github.com/swaggo/swag/cmd/swag` directive,
so `make api-docs` needs nothing extra on the runner; `Dockerfile.dev` uses `CMD` not `ENTRYPOINT`, so
`docker compose run --rm migrate go run ./cmd/api seed` correctly overrides it; nginx 1.29's `mime.types` maps
`.js` → `application/javascript`, which **is** in `gzip_types`, and `curl` confirmed `Content-Encoding: gzip`
on `/assets/*.js` (index.html is correctly unzipped — it is 701 bytes, below `gzip_min_length 1024`).

**Suspected (not reproducible without a remote):** M4 (web-readiness race), M5 (300s budget), L8 (cron/push
concurrency collision), L13 (GHAS requirement).

---

## Recommended actions

1. **H1** — `aquasecurity/trivy-action@v0.36.0`. One character, unblocks criteria 3 and 4.
2. **H2** — add `limit-severities-for-sarif: true` and `ignore-unfixed: true`.
3. **H3** — fix the probe table in `docs/deployment.md` and the `docker-compose.prod.yml` comment to
   distinguish `/healthz` (liveness) from `/readyz` (readiness).
4. **M1** — add the `test -n "$VITE_API_URL"` guard so the Dockerfile comment becomes true, closing a finding
   that has now survived two phases.
5. **M2** — add `Makefile` (both workflows) and `docker-compose.yml` / `.env.example` (web) to the path filters.
6. **M3** — `location /api/ { return 404; }`.
7. **M4/M5** — poll `web` health and widen the loop budget.
8. Batch the Low items; L2 (web coverage script + floor) and L8 (concurrency key) are the two worth doing now.

## Metrics

- Go: `build` + `vet` clean; `test-api` 71.4% vs a 60% floor (reported, not re-run)
- TypeScript: `tsc -b --noEmit` clean; strict config unchanged
- Workflows: `actionlint` 0 findings; 10/11 action refs resolve (H1 is the exception)
- Images: api 38.3MB (distroless, uid 65532), web 54.9MB (nginx-unprivileged, uid 101)

## Unresolved questions

1. Will this repo be public or private? L13 (SARIF upload needs GHAS on private repos) only bites in the
   private case, and it would silently defeat criterion 4 a second time after H1 is fixed.
2. Is the permanently-red-weekly-scan tradeoff acceptable, or should the Trivy job be advisory
   (`exit-code: "0"`, findings visible in code scanning) with only the SARIF gate enforcing policy? H2's fix
   assumes you want it blocking.
3. `api-ci.yml` has no e2e trigger, so a backend change that breaks the frontend contract is only caught by
   the nightly cron. Intentional (the plan quarantines e2e for PR speed), but worth confirming that the
   asymmetry — web pushes get e2e, api pushes do not — is deliberate rather than an oversight.

---

## Resolutions (applied same session, all re-verified)

| Finding | Resolution |
|---------|-----------|
| H1 | `aquasecurity/trivy-action@v0.36.0` — tag existence re-verified via `git ls-remote` |
| H2 | Added `limit-severities-for-sarif: true` + `ignore-unfixed: true` |
| H3 | Probe table split into liveness `/healthz` / readiness `/readyz` with an explicit warning; `docker-compose.prod.yml` comment updated |
| M1 | `RUN test -n "${VITE_API_URL}"` guard added; verified: build with arg exits 0, without arg exits 1 (closes the Phase 5 H1 recurrence) |
| M2 | `Makefile` added to both workflows' path filters; `docker-compose.yml` + `.env.example` added to web-ci |
| M3 | `location /api/ { return 404; }` — verified 404 against rebuilt image; SPA deep link still 200 |
| M4/M5 | Wait loop now polls `web` (transitively implies `api`) with a 600s budget and dumps `compose ps` + both logs on timeout |
| L1 | govulncheck pinned to `@v1.1.4` (module version verified) |
| L2 | `test:coverage` script added to package.json; CI calls it. Web coverage floor deferred — no agreed threshold; noted for a future decision |
| L4 | Compose header now shows `sha-<commit>` (metadata-action's actual tag format) |
| L5 | Makefile stamps the full sha (`git rev-parse HEAD`), matching CI's `${{ github.sha }}`; verified `--version` prints the full sha |
| L6 | swagger-drift uses `git status --porcelain` so untracked generated files also fail |
| L8 | `github.event_name` added to the web-ci concurrency key |
| L9 | `pull_request` trigger (self-path-filtered) added to security.yml |
| L13 | GHAS-on-private-repos caveat noted above the SARIF upload step |

**Accepted, not fixed:** L3 (badges keep the `OWNER/REPO` placeholder — plan mandates badges; no remote exists to name), L7 (`HydrateFallback: () => null` is correct; skeleton UX is polish beyond phase scope), L10 (root lockfile is tooling-only and Dependabot covers it), L11 (Dependabot PR-limit defaults acceptable with grouping; `Dockerfile.dev` bumps are desirable), L12 (fixed as part of the compose pass: decorative `depends_on` removed).

**Unresolved questions:** repo visibility (L13) and blocking-vs-advisory Trivy stay open until the repo has a remote; the api-push/no-e2e asymmetry is deliberate per the phase plan (e2e quarantined for PR speed).

Post-fix verification: actionlint clean, prod compose config valid, `npm run test:coverage` green (75.7% lines), web image rebuilt and smoke-tested (guard, /api/ 404, SPA fallback, headers), api image rebuilt (`--version` → full sha).
