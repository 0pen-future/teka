# Code Review — Plan 01: API Schema Replacement and Auth (phases 1–3)

Date: 2026-08-04
Reviewer: code-reviewer (read-only)
Scope: `apps/api/**` (migrations, seeds, features/auth, features/teachers, shared/{id,validation,authctx}, testutil, server, middleware, cli), `docs/api-guidelines.md`, `adr.md`

## Verdict

**Approve with concerns.** The security-critical work — phone-based login with real
constant-time failure branches, rotating refresh tokens with family revocation,
tenancy accessor, `/me` DB re-check gate — is implemented correctly and is backed by
tests that assert behavior rather than execution. Build, vet, and lint are clean.

Three concerns block "done without caveats": one plan acceptance criterion is not
actually proven by a test (register rollback), the repo-wide e2e/CI surface is left
red by the intentional API replacement without any recorded mitigation, and one dev
doc still advertises the deleted email seed account.

## Verification performed

| Check | Result |
|---|---|
| `diff docs/schema_design.sql apps/api/migrations/000001_baseline_schema.up.sql` | no output (verbatim) |
| `go build ./...` | clean |
| `go vet ./...` | clean |
| `make lint-api` | `0 issues.` |
| `go test ./...` (unit + HTTP layer, no integration tag) | all packages pass |
| `go tool swag init … -o <tmp>` vs `apps/api/docs/` | identical — spec is regenerated and current |
| swagger paths | only `/auth/{register,login,refresh,logout}` and `/me`; no `email`, `admin`, `/users` |
| `grep` for `features/users`, `create_users`, `RoleAdmin`, `IsAdmin`, `GetByEmail`, `RoleUser` | no hits |
| dummy bcrypt hash validity (standalone program) | valid, cost 12, compare = 270 ms vs 253 ms for a real cost-12 compare — the burn is real, not a no-op |
| `go mod tidy` dry run | would drop `golang.org/x/term` (now unused) |

Integration suite intentionally not run (owned by the tester agent).

## Acceptance criteria walk-through

### plan.md

| Criterion | Status | Evidence |
|---|---|---|
| `migrate-up` creates every schema object + `refresh_tokens` | Met | `migrations/migrations_test.go:90` asserts 17 domain tables + `refresh_tokens` + 2 views |
| `migrate-down` returns DB to empty | Met | same test, `require.Len(tables, 1)` after down-to-0 |
| Register creates paired `user_accounts` + `teachers` rows atomically | Partially met | pairing proven (`auth/integration_test.go:99`); atomic **rollback** not proven — see H1 |
| Login returns access token with `sub` = teacher id; wrong phone and wrong password indistinguishable | Met (claim assertion weak) | single `invalid` error value + verified bcrypt burn; no test decodes an issued token — see M4 |
| Refresh rotates; replay revokes family | Met | `auth/service_test.go:299,340`, `auth/integration_test.go:47` |
| `GET/PUT /api/v1/me` | Met | `teachers/handler_test.go`, `teachers/integration_test.go` |
| Disabled or soft-deleted account cannot log in or refresh | Met | `service.go:90,151`; `service_test.go:219,283`; `integration_test.go:139` |
| `make test-api` passes; no test references `users` | Met (suite run owned by tester) | greps clean |
| `make api-docs` has no stale email auth ops | Met | regeneration diff empty; no `email` in spec |

### Phase 1

All nine success criteria met. Verbatim copy verified by `diff`; down-migration drops
all 17 tables and both views in reverse dependency order and correctly leaves
`pgcrypto` (schema creates no functions, triggers, types, or sequences, so nothing is
orphaned); `refresh_tokens.user_id → user_accounts(id) ON DELETE CASCADE` asserted in
`migrations_test.go:127`; `internal/shared/id` returns v7 with tests; seed idempotency
proven in `seeds/seed_test.go`.

Deviation "16 vs 17 domain tables" and "verify via testcontainers instead of the dev
stack" are recorded in `adr.md` — accepted, not re-litigated.

### Phase 2

Met: register/login on phone, E.164 normalization in exactly one place
(`teachers/service.go:60,79`), no pre-check SELECT before insert (unique index is the
guard), both phone spellings collide (`teachers/repository_test.go:16`), refresh
rejects disabled accounts, role constants mirror the CHECK, ids from `shared/id`, no
`AutoMigrate`, rotation logic structurally unchanged (diff confirms only the type
re-point plus the added status gate).

Not met / weak:
- "A failed `teachers` insert leaves no `user_accounts` row (rollback asserted in an
  integration test)" — **not proven**, see H1.
- "all four failure modes return the identical 401 body" — true by construction, but
  the risk table's stated mitigation ("a test asserts all four failure modes produce
  byte-identical responses") does not exist, see M3.
- "JWT `sub` parses to the teacher id and `role` is `teachers`" — no test decodes a
  token issued by register/login, see M4.

Deviations recorded in `adr.md`: `AccountService` returns `*teachers.Profile`;
`features/users` + `cli/admin.go` deleted in phase 2 instead of 3; `scoped()` filters
`id` on identity tables.

### Phase 3

All ten success criteria met, including mass-assignment rejection
(`teachers/handler_test.go:129` posts `status`/`role`/`phone` and asserts they are not
persisted), invalid IANA timezone → 422 with a field message, two-teacher isolation,
soft-deleted/disabled token → 401, `GET /auth/me` → 404, `lint-api` clean. Tenancy
section added to `docs/api-guidelines.md`. Handler-level `currentProfile` gate is
recorded in `adr.md`.

## Findings

### Critical

None.

### High

**H1 — The register-rollback acceptance criterion is not actually tested.**
`apps/api/internal/features/auth/integration_test.go:72` (`TestRegisterDuplicatePhoneRollsBack`)
exercises the case where the **first** insert (`user_accounts`) fails on the unique
index. Nothing is written before the failure, so the transaction rollback path the
criterion targets — `user_accounts` inserted, then `teachers` insert fails — is never
executed. The assertions (`accountCount == 1`, `teacherCount == 1`) pass whether or not
`WithinTx` rolls back correctly. The test name asserts a guarantee the test body does
not check.

Fix: force a failure on the second insert and assert zero rows survive, e.g. call
`Service.Register` (service layer bypasses the `max=100` binding tag) with a
`full_name` longer than `teachers.full_name VARCHAR(100)` so the `teachers` INSERT
fails after the account row is written, then assert `user_accounts` count is 0.

### Medium

**M1 — `make e2e` and the web-CI e2e job are left broken with no recorded mitigation.**
`apps/web/e2e/auth.spec.ts:4` and `apps/web/e2e/users.spec.ts:3` log in with
`admin@teka.local` / `admin-password` and drive `/users` pages; `.github/workflows/web-ci.yml:145-148`
seeds the compose stack and then runs `npm run e2e` on main pushes and nightly. After
this change the seeder creates phone-keyed teachers, the users API is gone, and that
job goes red on main. The plan anticipates frontend breakage ("web app is plan 07") but
nothing in the diff or `adr.md` handles the CI gate.

Recommend: quarantine/skip the e2e job (or the two specs) with an explicit reference to
plan 07, and record the decision in `adr.md` so it is not discovered by a red main.

**M2 — Stale developer documentation.** `docs/local-development.md:11` still says
`make seed # … seeded users incl. admin@teka.local`. That account no longer exists; the
seeder now creates `+84901000001` / `+84901000002`. Phase 3 required updating docs where
the repo documents this; `api-guidelines.md` was updated but this line was missed.

**M3 — No test pins the identical-401 invariant across all four login failure modes.**
`auth/service_test.go:208-236` asserts each mode returns `CodeUnauthorized`, but nothing
asserts identical wire bodies. The invariant currently holds only because all four
branches return the same `invalid` variable — a future edit adding a "account disabled"
message would silently reintroduce user enumeration and every test would still pass.
Cheap fix: one HTTP-level table test comparing `w.Body.String()` across the four cases.

**M4 — No test decodes an access token issued by register/login.** Phase 2 criterion
"JWT `sub` parses to the teacher id and `role` is `teachers`" is covered only
transitively (`teachers/integration_test.go` mints its own token through the issuer).
A three-line `jwt.ParseWithClaims` assertion in `auth/handler_test.go` would pin the
claim contract that every downstream plan's tenancy depends on.

**M5 — bcrypt cost-12 hashing runs inside the register transaction.**
`auth/service.go:57` opens the transaction, then `teachers/service.go:50` spends ~250 ms
hashing while holding a pooled connection and an open transaction. With
`MaxOpenConns` at its default a modest burst of registrations exhausts the pool. Same
pattern for the unconditional login burn (~250 ms of CPU per failed login) with no rate
limiting anywhere in the middleware stack — an unauthenticated attacker can spend the
API's CPU cheaply. Both are pre-existing patterns rather than regressions, but they are
now on the primary auth path: hash before `WithinTx`, and put rate limiting on the
pre-launch list.

### Low

**L1 — Dead code and unused dependency left by the `cli/admin.go` deletion.**
`apps/api/internal/cli/root.go:34` `notYet()` now has no callers, and
`apps/api/go.mod:22` still requires `golang.org/x/term` (confirmed: `go mod tidy` removes
it). Lint does not flag either.

**L2 — `translateError` labels every duplicate-key violation as a phone duplicate.**
`teachers/repository.go:110`: `user_accounts` also carries `uq_users_role UNIQUE (id, role)`
plus the primary key, and the same helper wraps the `teachers` insert. Any of those
would surface to the client as `409 phone already registered`. Inspect the constraint
name (or scope the mapping to the account insert) before this helper is copied by
plans 02–06.

**L3 — A failed `last_login_at` stamp fails an otherwise valid login.**
`auth/service.go:101`: a transient error from `TouchLastLogin` returns 500 and denies a
correct credential. This write is telemetry, not authentication state — log and continue.

**L4 — `PUT /me` loads the profile twice.** `teachers/handler.go:55` (`currentProfile`)
then `service.UpdateProfile` → `repo.GetByID` again; each `GetByID` is itself two
queries (account + teacher). Four round trips for a two-column update. Not a
correctness issue; worth folding before the pattern is copied.

**L5 — Silent validator registration.** `shared/validation/validation.go:26-30` swallows
the `RegisterValidation` error and skips registration entirely if the type assertion
fails; in that case every `vnphone`-tagged bind panics at runtime instead of failing at
startup. Prefer panicking at init — this is a programming invariant, not a runtime
condition.

**L6 — Integration test file naming.** `teachers/repository_test.go` carries
`//go:build integration` and lives in `teachers_test`, while `docs/api-guidelines.md`
documents `integration_test.go` as the integration layer's file name. Harmless, but the
convention table is now slightly inaccurate.

## Explicit check confirmations

**(a) Acceptance criteria** — walked one by one above. All met except H1 (register
rollback, not proven) and the weak spots M3/M4. Recorded deviations verified present in
`adr.md`.

**(b) No business-logic regression in the blast radius** — router wiring constructs
`teachers` once and shares it with `auth` (`server/router.go:63-71`); `requireAuth` is
mounted on both `/me` routes; `middleware.RequireAuth` and `RequireRole` are unchanged
and `response.Err` uses `AbortWithStatusJSON`, so a rejected token cannot fall through to
the handler. `git diff` of `auth/service.go` confirms the rotation, family-revocation,
and race-loss handling are byte-for-byte the same apart from the type re-point plus the
added disabled-account gate; the "revocations run outside the transaction" comments
survive. `WithinTx` nesting semantics unchanged; the teachers repository resolves the
ambient handle via `database.FromContext`, so register really is one transaction. Seeds
keep the idempotency contract (keyed on phone, `deleted_at IS NULL`) and the production
guard in `cli/seed.go` is intact.

**(c) Public-contract changes** — the users API, `GET /auth/me`, and `api admin create`
are removed as the plan intends. Unintended breakage found: the shipped web app and its
e2e suite (M1) and one dev doc (M2). No other exported contract was silently changed;
`authctx.Principal`, `middleware.RequireRole`, the response envelope, and the apperror
codes are all preserved.

**(d) Repo patterns** — response envelope, `apperror` mapping, repository-interface-first
with a hand-written in-memory fake, the seven-file feature contract
(`handler/dto/service/repository/model/routes/errors`), binding-tag validation with
`validation.BindError`, and the three-layer test pyramid (in-package unit + in-package
HTTP over fakes + tagged external integration) are all followed. Cross-feature access is
a consumer-defined interface (`AccountService`), matching the documented rule. No new
managers, no parallel utilities, no `any` widening, no lint suppressions, no
catch-and-swallow beyond L5.

**(e) Lint/type/build** — `go build ./...` clean, `go vet ./...` clean, `make lint-api`
reports `0 issues.`, unit/HTTP tests pass. Only hygiene residue is L1.

**Security specifics** — the dummy bcrypt constant is a valid cost-12 hash and its
comparison measurably costs the same as a real one (270 ms vs 253 ms measured), so the
burn on the not-found, disabled, and null-hash branches genuinely equalizes latency; the
wrong-password branch does a real compare. Refresh tokens appear only in the httpOnly
`SameSite=Lax` cookie scoped to `/api/v1/auth` (`Secure` in production only, documented
rationale) and never in a response body — asserted at `auth/service_test.go:200`. Only
sha256 hashes are stored. Tenancy is read solely from `authctx.TeacherID(c)`; no handler,
service, or repository accepts a client-supplied teacher id. `TeacherResponse` exposes no
hash or internal error text; `apperror.Internal` keeps causes server-side. No secrets in
the diff — seed passwords are dev-only behind the production guard, and the JWT secret
stays config-driven with a 32-char minimum.

## Recommended actions

1. Fix H1: make the register-rollback test actually fail the second insert.
2. Decide and record the e2e/CI disposition (M1) before this lands on `main`.
3. Update `docs/local-development.md:11` to the phone-based seed accounts (M2).
4. Add the identical-401 body test (M3) and the JWT claim assertion (M4).
5. Move bcrypt hashing out of the register transaction; add rate limiting to the
   pre-launch list (M5).
6. Sweep the L-items: drop `notYet` and `golang.org/x/term`, tighten `translateError`,
   demote the `TouchLastLogin` failure, collapse the double load in `PUT /me`.

## Unresolved questions

1. Is the web e2e job expected to stay red until plan 07 lands, or should it be
   quarantined now? (Affects whether M1 is a blocker.)
2. Plan open question 1 (E.164 vs local storage) is settled in code as E.164 —
   should the plan's Open Questions section be closed out to match?
