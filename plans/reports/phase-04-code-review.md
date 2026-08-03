# Phase 4 Code Review — Backend Testing and API Docs

Reviewer: code-reviewer · Date: 2026-08-03 · Base: `a4bb1e5` (uncommitted working tree)

## Scope

- Modified: `.gitignore`, `Makefile`, `apps/api/cmd/api/main.go`, `apps/api/go.mod`, `apps/api/go.sum`,
  `apps/api/internal/features/{auth,users}/handler.go`, `apps/api/internal/server/router.go`,
  `apps/api/internal/shared/response/response.go`, `docs/api-guidelines.md`, 2 plan files
- Added: `apps/api/testutil/{postgres,server,fixtures}.go`, 4 test files, `apps/api/docs/` (generated, 2888 lines)
- LOC: ~477 changed + ~700 new test/testutil + generated docs

### Checks re-run (read-only)

| Check | Result |
|---|---|
| `go build ./...` | clean |
| `go vet ./...` / `go vet -tags=integration ./...` | clean |
| `gofmt -l .` | clean |
| `golangci-lint run --build-tags integration ./...` | 0 issues |
| `go test -short ./...` | all pass; integration excluded cleanly |
| `go tool swag init` → scratch, diff vs `docs/` | byte-identical (no annotation drift) |
| `go mod tidy -diff` | **fails** — see H1 |

## Overall Assessment

Solid, disciplined work. Tests assert real behavior (not tautologies), wiring in HTTP tests mirrors
production `registerFeatures` exactly, the envelope wire format is provably unchanged, and the
committed OpenAPI spec is in sync with the annotations. Three findings warrant action before landing;
none are security defects.

## Critical Issues

None.

## High Priority

### H1 — `go.mod` is not tidy; six direct dependencies are mislabeled `// indirect`

`go mod tidy -diff` exits with a diff: `stretchr/testify`, `swaggo/files`, `swaggo/gin-swagger`,
`swaggo/swag`, `testcontainers-go`, and `testcontainers-go/modules/postgres` sit in the `// indirect`
block although they are imported directly (`router.go` imports gin-swagger and swaggo/files;
`testutil/postgres.go` imports testcontainers; tests import testify). `go.sum` also diverges.

Impact: the dependency graph misrepresents that gin-swagger/swaggo-files are **production** deps of
the router; the Phase 8 CI tidy/drift check (planned in the phase doc) will fail on a clean tree.

Fix: run `go mod tidy` and commit the result with the phase.

### H2 — Nothing pins the swagger env gate, and `testutil.NewTestServer` is dead code

`testutil/server.go:23 NewTestServer` has **zero callers** (verified by grep across the module). It is
the only helper that builds the real router over a real DB, and it is unused — so acceptance criterion
(c) rests entirely on a one-off manual `curl`. If someone inverts or drops
`if !cfg.IsProduction()` in `router.go:45`, no test fails and the OpenAPI surface is published in
production.

This is cheap to close without Docker — `internal/server/router_test.go` already builds a router with
`db == nil`:

```go
// EnvProduction -> GET /swagger/index.html must be 404
// EnvTest       -> must not be 404
```

Fix: either use `NewTestServer` in an integration test, or delete `testutil/server.go` and add the
env-gate assertion to the existing `router_test.go`. Do not leave an unused exported helper behind.

## Medium Priority

### M1 — The documented 60% coverage floor is not enforced

`docs/api-guidelines.md` says coverage "must stay at or above **60%**" and the Makefile help text calls
`test-api` a "coverage gate", but the recipe only pipes `go tool cover -func` through `tail -1`. Exit
status is unaffected by the number. Criterion (d) is therefore "report generated + manually eyeballed",
not "floor met" in any automated sense.

Fix: enforce it in the recipe (parse the total and `exit 1` below the threshold) or reword the docs and
the phase criterion to "floor checked in CI (Phase 8)".

### M2 — One PostgreSQL container per test, six tests, all `t.Parallel()`

`StartPostgres` starts a container per call; six integration tests run in parallel across two packages,
so a CI runner can hold six concurrent `postgres:16-alpine` containers. The phase plan asked for
per-package isolation ("random db name per package"), which is materially cheaper. Isolation is correct
today, but startup cost and runner memory scale with test count, and this is the layer most likely to
become flaky under load (the phase's own stated risk).

Fix (follow-up is acceptable): one container per package via `TestMain`/`sync.Once` plus a per-test
`CREATE DATABASE`/schema, keeping the same `t.Cleanup` semantics.

### M3 — `testutil` sits outside `internal/`

`apps/api/testutil` is a normal, tag-free package on the module's public surface. Consequences: Docker
client libraries become direct module requirements (after H1's tidy), `go build ./...` compiles
testcontainers, and external consumers can import test infrastructure. Every other non-`cmd` package in
this module lives under `internal/`.

Fix: move to `apps/api/internal/testutil`. The external `_test` package trick used by the integration
files keeps working unchanged.

### M4 — No HTTP-level test for the admin-or-self authorization path

`handler_test.go` covers 401 (no token) and 403 (role `user` on admin-gated routes), but nothing
exercises a non-admin caller hitting `GET`/`PATCH /users/{someoneElsesID}`. The service unit tests own
that rule, yet the handler does `caller, _ := authctx.From(c)` and drops the `ok` — if the caller were
ever unset, a zero-value `caller` reaches the service. An IDOR-shaped regression in the
handler→service handoff would pass the current suite. One table case in `users/handler_test.go` closes
this.

## Low Priority

1. `users/handler_test.go:174-175` — `repo.users[uuid.New()] = &User{ID: uuid.New(), ...}`: map key and
   `User.ID` are different UUIDs. Works only because `List` ignores keys; breaks the moment the test
   grows a `Get`. Use the same value for both.
2. `users/integration_test.go:28-30` — the sort whitelist is retyped by hand instead of reusing the
   production `listSorts` (unexported, and the test is an external `_test` package). Changing
   `listSorts` will not be caught. Consider exporting the whitelist or moving the parse helper in-package.
3. `auth/handler_test.go:191` — `_ = wireEnv` dead assignment plus a shadow `env` struct at 177-179;
   clean-up noise in an otherwise tidy file.
4. `auth/handler_test.go:218` — `cleared.MaxAge >= 0 && cleared.Value != ""` passes whenever `Value` is
   empty, regardless of expiry. Assert both conditions explicitly (`MaxAge < 0` **and** `Value == ""`).
5. `List` orders by a single column with no tiebreaker; `TestRepositoryListFilterSortPagination` relies
   on distinct `created_at` from insert timing. Real flake risk is low (bcrypt runs between inserts),
   but `ORDER BY created_at DESC, id DESC` would make both the production query and the test
   deterministic under ties.
6. Doc inaccuracy: api-guidelines says integration tests "call `t.Skip` under `-short`". In the
   `make test-api-unit` path they are excluded by the `integration` build tag and never compiled; the
   `t.Skip` only fires under `-tags=integration -short`. Both behaviors are fine — the sentence is not.
7. Plan drift: `phase-04-*.md:39` still says `swag init` output is "gitignored, regenerated" while the
   implementation (correctly, since `router.go` imports the package) commits `apps/api/docs/` and drops
   the ignore rule. Record the deviation in the phase doc.
8. `make test-api` has no `-count=1`; a re-run can report green from cache.
9. `swaggo/files` is imported unconditionally, so the swagger-UI assets are linked into the production
   binary even though the route is not registered. Informational only.

## Verification of the Stated Acceptance Criteria

| # | Criterion | Verdict |
|---|---|---|
| a | `make test-api` green; `-short` skips cleanly | **Met** for the `-short` half (re-verified locally). Docker half not re-run per instruction; recipe is sound. |
| b | Breaking a SQL query fails an integration test | **Met.** "Liddell" appears only in `name`, so `email ILIKE`'s citext case-insensitivity cannot mask an `ILIKE`→`LIKE` swap on `name`. Note the converse: breaking **`email ILIKE`** would *not* fail any test — harmless, because citext makes it a no-op. |
| c | `/swagger/index.html` serves all endpoints with schemas | **Met in artifact, unpinned in tests.** Spec contains 7 paths, 9 definitions, `BearerAuth`, `basePath /api/v1`, no host. Regenerating with `swag init` produces a byte-identical spec. See H2. |
| d | Coverage report generated, floor met | **Partially met.** Report is generated; the floor is not enforced anywhere. See M1. |

## Regression / Contract Checks

- **Envelope JSON unchanged.** `envelope`→`Envelope` and `errorBody`→`ErrorBody` are Go identifier
  renames only; every `json:` tag is identical in the diff, no other referents exist (module builds
  clean), and the module is the only Go consumer. No wire-level change.
- **Handlers:** both diffs are comment-only. No statement was touched.
- **Router:** purely additive, env-gated block plus a blank import. Existing `router_test.go` still passes.
- **`cmd/api/main.go`:** comment-only.
- **Routes, function signatures, env vars:** unchanged. No new configuration was introduced (swagger is
  gated on the existing `API_ENV`).
- **Test wiring fidelity:** `newHandlerTest` / `newHandlerHTTPTest` pass exactly the middleware set that
  `registerFeatures` passes, so the 401/403 matrix reflects production.
- **Secrets:** none committed. The two test JWT constants are test-only and intentional. Generated spec
  contains no host, credentials, or internal paths.

## Recommended Actions

1. Run `go mod tidy`, commit the result (H1).
2. Add a swagger env-gate assertion to `internal/server/router_test.go`; then either use or delete
   `testutil/server.go` (H2).
3. Enforce the coverage floor in the Makefile, or reword the docs/criterion to match reality (M1).
4. Move `testutil` under `internal/` (M3).
5. Add one HTTP case for non-admin access to another user's record (M4).
6. Cheap clean-ups: L1, L3, L4, L6, L7.
7. Follow-up (Phase 8 or a small refactor): per-package containers (M2), `ORDER BY` tiebreaker (L5).

## Unresolved Questions

1. Is a 60% coverage floor intended to be enforced in Phase 4 (Makefile) or deferred to Phase 8 CI? The
   docs and the phase criterion currently disagree with the code.
2. Should the swagger UI also be gated off in staging, or is `API_ENV=development` on a
   publicly-reachable staging host an accepted exposure?

## Resolution (post-review fixes, same session)

- **H1 fixed** — `go mod tidy` run; `go mod tidy -diff` is clean.
- **H2 fixed** — dead `testutil.NewTestServer` deleted (`JWTSecret` moved to
  `fixtures.go`); new router-level test `TestSwaggerServedOutsideProductionOnly`
  pins the env gate: 200 in test env, 404 in production.
- **M1 fixed** — `make test-api` now fails below `API_COVERAGE_FLOOR` (60);
  gate verified to exit 1 at 59.9%. Docs updated to match.
- **M3 fixed** — `testutil` moved to `apps/api/internal/testutil`; imports updated.
- **M4 fixed** — `TestGetAndUpdateForbidOtherUsers`: non-admin GET/PATCH on
  another user's record → 403 FORBIDDEN, record unmutated.
- **Lows fixed** — list-fixture map keys now use the user's own ID; `_ = wireEnv`
  replaced with an UNAUTHORIZED-envelope assertion; logout assertion now requires
  empty value AND negative MaxAge; docs `-short`/build-tag wording corrected;
  plan phase-04 updated (committed `docs/` deviation noted, testutil paths).
- **Deferred** — M2 per-test containers (isolation is correct; revisit if suite
  cost grows) and the `ORDER BY` tiebreaker (production SQL change, out of
  Phase 4 scope; noted for a follow-up).
- **Unresolved Q1 resolved as: enforce in Makefile now** — Phase 8 CI inherits
  the gate by calling `make test-api`.
- **Unresolved Q2** — config only admits development|test|production; a staging
  deployment would run one of those. Swagger exposure follows `API_ENV`, which
  matches the plan; flagged to the user at the commit gate.

Post-fix verification: gofmt/go build/go vet (integration tags) clean,
golangci-lint 0 issues, `go test -short ./...` green, `make test-api` green at
71.4% (floor 60%).
