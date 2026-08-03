# Phase 2 — Backend Core Infrastructure: Code Review

Reviewer: code-reviewer | Date: 2026-08-03
Scope: uncommitted tree on top of `68f50f3` — `apps/api/**` (25 files, 925 LOC Go), root `Makefile`
Verification re-run: `go vet ./...` clean, `go test ./...` pass (config only), `golangci-lint run` → **0 issues**

## Verdict

Architecture matches the plan, all 7 acceptance criteria are met in code, and no
Phase 1 regression was found. Two trust-boundary defaults and one test-hermeticity
bug should be fixed before Phase 3 builds auth on top of this skeleton.

| # | Acceptance criterion | Result |
|---|---|---|
| 1 | serve / `/healthz` 200 / `/readyz` 200-503 | Met (code inspection) |
| 2 | Full Cobra tree, stubs error | Met |
| 3 | Fail-fast config, JSON/text logs, 10s drain then pool close | Met (see M1) |
| 4 | `make lint-api`, `go test ./...` | Met (re-verified) |
| 5 | Middleware order, `/api/v1`, 404 envelope, timeouts | Met (see L8) |
| 6 | No AutoMigrate, pool 25/5/30m | Met |
| 7 | Response envelope | Met (see L4) |

---

## High

### H1 — Gin trusts every proxy; `ClientIP()` is attacker-controlled
`internal/server/router.go:22` (`gin.New()`, no `SetTrustedProxies`), consumed at
`internal/middleware/logger.go:28`.

`gin.New()` ships `trustedProxies: ["0.0.0.0/0", "::/0"]` with
`RemoteIPHeaders: ["X-Forwarded-For", "X-Real-IP"]`
(`gin@v1.12.0/gin.go:215,225`). Any client can send `X-Forwarded-For: 1.2.3.4`
and that value lands in the request log as `client_ip`. Today the blast radius
is falsified audit/log data; the moment Phase 3 adds rate limiting, IP
allowlists, or login-attempt throttling on the same primitive, it becomes a
bypass. Gin itself emits `[WARNING] You trusted all proxies, this is NOT safe.`

Fix (one line, decide the trust boundary now):
```go
if err := r.SetTrustedProxies(nil); err != nil { ... } // direct exposure
// or: r.SetTrustedProxies(cfg.HTTP.TrustedProxies) when behind a known LB
```

### H2 — CORS config is unvalidated: typo panics at startup, `*` silently breaks credentials
`internal/middleware/cors.go:17`, `internal/config/config.go:79-92`.

`cors.New` calls `Validate()` and **panics** on a bad origin
(`cors@v1.7.7/cors.go:44,151`): any `API_CORS_ORIGINS` entry lacking
`http://`/`https://` (typo, stray space, trailing comma producing an empty
element) crashes the process with a stack trace at router construction —
violating the "config fails fast with a clear message" contract (criterion 3),
because the failure happens after `config.Load()` succeeded.

Second problem: `newCors` converts `*` to `AllowAllOrigins` *after* validation
(`cors.go:49`), and with `AllowCredentials: true` the middleware then emits both
`Access-Control-Allow-Origin: *` and `Access-Control-Allow-Credentials: true`
(`cors@v1.7.7/utils.go:15,22`). Browsers reject that combination, so a
well-meaning `API_CORS_ORIGINS=*` will silently break the Phase 3 refresh-token
cookie flow with a confusing browser-side error.

Fix: validate origins in `Config.validate()` — require a scheme prefix, reject
empty entries, and reject `*` while `AllowCredentials` is on.

---

## Medium

- **M1 — `.env` is loaded in the `test` environment too; plan says development only.**
  `internal/config/config.go:60` gates on `!= EnvProduction`. `go test ./internal/config`
  runs with CWD = package dir, so `../../.env` resolves to `apps/api/.env` — a
  gitignored file a developer may well create. `godotenv` does not override
  already-set vars, but `TestLoadDefaults` asserts *defaults*
  (`config_test.go:23-37`), so any `API_HTTP_PORT`/`API_LOG_LEVEL`/`API_CORS_ORIGINS`
  in that file fails the suite on one machine and not on CI. Gate on
  `== EnvDevelopment` as the plan specifies.

- **M2 — Criteria 1, 5 and 7 have zero automated coverage.** Only `config` has
  tests. `/healthz`, the 404 envelope, middleware ordering, request-id
  passthrough and the success/error envelope shapes are all reachable via
  `httptest` + `gin` with **no database** (`NewRouter` only stores `db` for
  `/readyz`). Given that the live smoke test was blocked (no Docker), these
  criteria are currently asserted by nothing. A ~60-line
  `internal/server/router_test.go` closes the gap permanently.

- **M3 — Public contract landed in code but not in docs.** `docs/api-guidelines.md`
  states verbatim: "the response envelope and feature-module contract defined in
  the implementation plan land here … when Phase 2/3 are built", and
  `docs/architecture.md` promises "DI container details". The envelope, the
  stable error-code list (`apperror.go:13-21`), and the manual-DI decision are
  public contracts shipped by this phase and are undocumented.

- **M4 — Plan risk mitigation not implemented.** Plan line 104 requires that
  `os.Getenv` outside `internal/config` be "lint-flagged". `.golangci.yml` has no
  `forbidigo`/`revive` rule enforcing it, so config sprawl is unguarded. Either
  add the rule or drop the claim from the plan.

- **M5 — `Recovery` swallows `http.ErrAbortHandler` and broken-pipe panics.**
  `internal/middleware/recovery.go:18`. Gin's stock recovery special-cases these:
  `http.ErrAbortHandler` is the stdlib's *intentional* silent-abort signal and
  must be re-panicked, and on a broken pipe there is no connection left to write
  the 500 envelope to. As written, both produce a full stack-trace log line plus
  a doomed `AbortWithStatusJSON` — noise that will mask real panics.

- **M6 — A second Ctrl-C during the 10s drain is swallowed.**
  `internal/cli/serve.go:17-19`: `stop()` is deferred to `RunE` exit, so the
  signal handler stays installed for the whole drain. An operator who wants to
  abort a hung shutdown has no escape short of `SIGKILL`. Call `stop()` as soon
  as the context is cancelled to restore default signal handling.

---

## Low

- **L1** `internal/database/postgres.go:27-39` — when `db.DB()` or `PingContext`
  fails, the already-open `*sql.DB` pool is never closed. Harmless for `serve`
  (process exits) but leaks once `migrate`/`seed`/tests call `Open` repeatedly.
- **L2** `config.go:63` — `_ = godotenv.Load(path)` swallows parse errors; a
  malformed `.env` surfaces as a misleading "required variable not set".
- **L3** `middleware/requestid.go:17-22` — inbound `X-Request-ID` is echoed and
  logged unbounded and unvalidated. Not a response-splitting risk (net/http
  replaces CR/LF in header values) and not a log-injection risk (slog quotes),
  but an 8 KB id multiplies every log line for that request. Cap at ~64 chars /
  printable ASCII, else generate.
- **L4** `shared/response/response.go:32` — `Data any` carries `omitempty`, so
  `OK(c, 200, nil)` emits `{"success":true}` with no `data` key at all. The plan
  envelope always shows `data`. Drop `omitempty` on `Data` if clients rely on it.
- **L5** `middleware/logger.go:23` — every `/healthz` and `/readyz` probe logs at
  Info (k8s probes = ~2 lines/sec), and 5xx responses log at Info too. Skip
  probe paths; escalate level by status.
- **L6** `response.go:51` — causes attached to 4xx `AppError`s are never logged
  (only `>= 500`), so wrapped validation/conflict causes vanish.
- **L7** `app/container.go:25` `slog.SetDefault` and `server/router.go:19`
  `gin.SetMode` are process-global mutations inside constructors; the plan says
  the logger is "injected, never global". Defensible as a `FromContext`
  fallback, but it makes future parallel tests order-dependent.
- **L8** CORS rejections abort with a bare `403` and no error envelope,
  inconsistent with criterion 7.
- **L9** `API_DB_MAX_OPEN_CONNS` / `API_DB_MAX_IDLE_CONNS` /
  `API_DB_CONN_MAX_LIFETIME` exist in `config.go:32-34` but not in `.env.example`.
- **L10** `postgres.go:21` — GORM logger is fully `Silent`, so slow queries and
  driver-level warnings are invisible. Consider a slog adapter in Phase 3.
- **L11** `Makefile:43` — `set -a; [ -f .env ] && . ./.env` shell-*executes* a
  Compose-style env file; values containing `$`, backticks or unquoted spaces
  break or execute. Also redundant: `config.Load` already reads `../../.env`.
- **L12** `config_test.go:49` — the "missing database url" case asserts only
  `"DATABASE_URL"`, so losing the `API_` prefix would not be caught. Assert the
  full `API_DATABASE_URL`.

---

## Verified non-issues (checked, no action needed)

- **Shutdown ordering is correct.** `app/app.go:22` defers `c.Close()` and
  `server.Run` blocks until the drain finishes, so the DB pool closes **after**
  in-flight requests complete.
- **No goroutine leak in `server.Run`** — `errCh` is buffered (cap 1), so the
  listener goroutine exits even on the `Shutdown`-error return path.
- **Shutdown-before-listen race is handled** — `ErrServerClosed` is filtered at
  `server.go:46`.
- **No secret leakage on the DB error path** — pgx redacts passwords in both
  `ParseConfigError` and connect errors (`pgconn/errors.go:140,230`), so the
  `open postgres: %w` message printed to stderr cannot contain the DSN password.
- **No internal detail reaches clients** — `apperror.From` wraps unknown errors
  as a generic `internal server error`; stacks and causes go to logs only.
- **Lefthook Go hooks work with `root` + `glob`** (Phase 1 regression check).
  Lefthook v2 applies `byGlob` to repo-relative paths *first*, then `byRoot`
  trims the prefix (`internal/run/controller/filter/filter.go`), so
  `glob: "**/*.go"` matches `apps/api/internal/cli/root.go` under
  `glob_matcher: doublestar`, and `gofmt` receives `./internal/...` with CWD
  `apps/api/`.
- `make help` still renders all targets; `bin/` and `tmp/` are gitignored; every
  `API_*` name in `config.go` matches `.env.example`; no `AutoMigrate` anywhere;
  timeouts are Read 10s / Write 30s / Idle 120s plus a `ReadHeaderTimeout` 5s.

## Recommended order

1. H1 `SetTrustedProxies` (1 line, decides a trust boundary Phase 3 inherits).
2. H2 validate CORS origins in `Config.validate()`.
3. M1 gate `.env` loading to development.
4. M2 add `internal/server/router_test.go` (healthz, 404 envelope, request-id, CORS headers).
5. M5, M6 recovery/signal polish; M3/M4 docs + lint rule.
6. Low items opportunistically.

## Unresolved questions

- Will the API sit behind a reverse proxy/LB in Phase 7 (Docker/Compose)? The
  answer decides whether H1's fix is `SetTrustedProxies(nil)` or a config knob.
- Should health probes stay outside the response envelope? Current choice
  (plain bodies for orchestrators) is reasonable but is not written in the plan
  or `docs/api-guidelines.md`.
