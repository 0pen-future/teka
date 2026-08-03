# Phase 3 Code Review — Backend Features and Migrations

- Plan: `plans/260803-1552-fullstack-project-provisioning/phase-03-backend-features-and-migrations.md`
- Baseline: uncommitted work since `86161c4`
- Reviewed: 10 modified files + 22 new files (~2.2k LOC in `migrations/`, `features/`, `shared/`, `seeds/`, `middleware/auth.go`)
- Local verification re-run (read-only): `go build ./...`, `go vet ./...`, `go test ./...` → green; `golangci-lint run ./...` → **0 issues**

## Overall Assessment

The feature-module contract is followed exactly in both features, layering is clean, the
error/envelope contract is respected, and the security fundamentals asked for by the plan
(bcrypt 12, sha256-hashed refresh tokens at rest, HS256 alg confinement, sort whitelist,
no token logging) are all present and correct. Migrations are PG14-safe.

Findings below are concentrated in three places: (1) the rotation path lacks a
compare-and-swap on revoke, so reuse detection is defeated by concurrency; (2) a
multibyte password produces a 500 instead of a 422; (3) unvalidated numeric pagination
input overflows into a wrong page of data. None of these are visible on the happy paths
that were verified live, which is why the live acceptance run passed.

**Recommendation: fix H1–H3 before commit; M-items can be scheduled.**

---

## Acceptance Criteria — Verification by Reading

| # | Criterion | Verdict | Evidence |
|---|-----------|---------|----------|
| 1 | `migrate-up && seed` idempotent | Met | `seeds/seed.go:38-55` skips on `GetByEmail` hit before insert; `database/migrate.go:30` treats `ErrNoChange` as success |
| 2 | `down` reverses, `status` reports, `--all` guarded | Met | `cli/migrate.go:47-56`, `:76-88`. Down SQL drops both tables (extensions deliberately kept — see L4) |
| 3 | Auth e2e incl. reuse-revokes-family | Met on the sequential path; **defeated under concurrency** — see H1. `auth/service.go:106-113` |
| 4 | Pagination envelope meta | Met for sane input; **wrong for `page` near `MaxInt`** — see H3 |
| 5 | 422 with per-field messages | Met (`validation/validation.go:18-28`); field naming is fragile — see M5 |
| 6 | Unit tests + lint | Met — re-verified: tests green, lint 0 issues |

**(b) No Phase 1–2 regression.** Confirmed: middleware order (`RequestID → Logger → Recovery → CORS`),
`SetTrustedProxies(nil)`, health registration before `/api/v1`, and the `NoRoute` 404 envelope
are unchanged in `internal/server/router.go`; the three existing router tests still pass.

**(c) Public contracts.** No breaking change to Phase 1–2 surfaces. The single known deviation
(`Secure` cookie only in production) is acknowledged and not re-litigated here; one scoping
consequence is noted as M6.

**(d) Feature-module contract.** Followed exactly. `auth` consumes `users` only through the
consumer-defined `UserService` interface (`auth/service.go:18-22`) and never touches
`users.Repository`. Verified: no cross-feature repository import exists.

**(e) No new build/type/lint errors.** Confirmed by re-run.

---

## High

### H1 — Refresh rotation has no compare-and-swap; concurrent replay bypasses reuse detection
`apps/api/internal/features/auth/repository.go:50-55`, consumed at `apps/api/internal/features/auth/service.go:99-134`

`Revoke` discards `RowsAffected`:

```go
return database.FromContext(ctx, r.db).
    Model(&RefreshToken{}).
    Where("id = ? AND revoked_at IS NULL", id).
    Update("revoked_at", at).Error
```

The reuse check at `service.go:108` reads the token *before* the transaction opens, so the
sequence is check-then-act with no lock. Two requests presenting the same refresh token
concurrently (victim + attacker, or a double-submitting client) both observe
`t.Revoked() == false`, both enter `WithinTx`, and the second `UPDATE` matches **0 rows** —
which returns `nil`. Result: both requests are issued a fresh, live token in the same family,
and the family is never revoked. The exact scenario rotation-with-reuse-detection exists to
catch is the one that slips through.

The unit test can't see this: `fakeTokenRepository.Revoke` (`service_test.go:96-104`) also
silently no-ops on an already-revoked token, mirroring the bug.

**Fix.** Make revoke report whether it won the race, and treat "lost" as reuse:

```go
func (r *gormRepository) Revoke(ctx context.Context, id uuid.UUID, at time.Time) error {
    res := database.FromContext(ctx, r.db).
        Model(&RefreshToken{}).
        Where("id = ? AND revoked_at IS NULL", id).
        Update("revoked_at", at)
    if res.Error != nil {
        return res.Error
    }
    if res.RowsAffected == 0 {
        return ErrTokenAlreadyRevoked
    }
    return nil
}
```

In `Refresh`, map `ErrTokenAlreadyRevoked` to the same branch as `t.Revoked()`: revoke the
family (outside the rotation transaction, consistent with the existing deliberate choice)
and return 401. The `UPDATE` takes the row lock, so the loser is serialized behind the
winner and observes 0 rows — no `SELECT … FOR UPDATE` needed. Add a fake-repo counterpart
and a test asserting the second revoke is rejected.

### H2 — Transient errors in `Refresh` are laundered into 401 and destroy the client session
`apps/api/internal/features/auth/service.go:117-120`

```go
u, err := s.usersSvc.GetByID(ctx, t.UserID)
if err != nil {
    return nil, invalid
}
```

`GetByID` returns `apperror.Internal(...)` for any DB failure (`users/service.go:74`). Here every
such failure is collapsed to `invalid` (401). Because `response.Err` only logs at status ≥ 500,
the cause is **never logged** — a DB blip during refresh is invisible in production. Worse,
`handler.go:71-74` clears the refresh cookie on any error, so a few seconds of connection-pool
pressure logs out every user who happened to refresh, and they cannot recover by retrying.

**Fix.** Distinguish the cases:

```go
u, err := s.usersSvc.GetByID(ctx, t.UserID)
if err != nil {
    var appErr *apperror.AppError
    if errors.As(err, &appErr) && appErr.Code == apperror.CodeNotFound {
        return nil, invalid
    }
    return nil, err // propagate 500, logged with cause
}
```

Then narrow the cookie clearing in `handler.go` to 401 responses only, so a 500 does not
discard a still-valid refresh token.

### H3 — Multibyte password → 500 instead of 422 (very likely for Vietnamese users)
`apps/api/internal/features/auth/dto.go:10`, `apps/api/internal/features/users/dto.go:12`, `apps/api/internal/features/users/service.go:34`

`binding:"...,max=72"` on a string is evaluated by `validator` in **runes**, while
`bcrypt.GenerateFromPassword` rejects anything over **72 bytes**. Verified empirically in this
repo's module: a 40-character Vietnamese password (`"ế" × 40` = 40 runes / 120 bytes) passes
validation, then `bcrypt` returns `password length exceeds 72 bytes`, which
`users/service.go:35-37` wraps as `apperror.Internal` → **500 INTERNAL_ERROR** on
`POST /auth/register` and `POST /users`. This is not an edge case for a Vietnamese-language
product; any accented passphrase over ~24 characters trips it.

**Fix.** Validate the byte length at the boundary and, defensively, map the bcrypt error:

- register a custom validator tag (e.g. `bytemax=72`) used by both DTOs, with a matching
  message in `validation.message`, **or** check `len(req.Password) > 72` in the service and
  return `apperror.Invalid` with a `password` field message;
- either way, translate `bcrypt.ErrPasswordTooLong` to 422 rather than letting it reach
  `apperror.Internal`.

Also worth aligning while you are here: `RegisterRequest.Name` allows `max=255`
(`auth/dto.go:11`) but `users.CreateRequest.Name` allows `max=100` (`users/dto.go:13`) — the
same column, two different contracts, depending on which endpoint created the row.

---

## Medium

### M1 — Unbounded `page` overflows the offset and returns page 1 labelled as page N
`apps/api/internal/shared/pagination/pagination.go:36-38`, `:60`

`page` is accepted as any positive `int`. Verified: `?page=9223372036854775807&per_page=20`
yields `Offset() == -40`; GORM discards a negative offset, so the client receives **page 1
rows** inside a `meta` block claiming `page: 9223372036854775807`. Data correctness bug from
unvalidated external input, not just a cosmetic one.

**Fix.** Clamp in `Parse`: `if v, err := strconv.Atoi(...); err == nil && v > 0 { p.Page = min(v, maxPage) }`
with `maxPage` bounded (e.g. 1e6), or guard the multiplication in `Offset()`. Add a
`pagination` unit test — the package currently has none, despite being shared by every future
list endpoint.

### M2 — bcrypt cost-12 hashing runs inside the registration transaction
`apps/api/internal/features/auth/service.go:50-62` → `apps/api/internal/features/users/service.go:34`

`Register` opens the transaction first, then `users.Create` spends ~250 ms of CPU on
`GenerateFromPassword` while holding an open transaction and a pooled connection
(`DB_MAX_OPEN_CONNS` default 25). Registration throughput is therefore capped by the pool
rather than by CPU, and a burst of registrations starves *all* other queries of connections.

**Fix.** Hash before the transaction boundary: give the service an internal
`createWithHash(ctx, hash, …)` seam, or have `auth.Register` pre-hash and pass the digest, so
the transaction contains only the two INSERTs.

### M3 — Login/register have no rate limiting; each attempt costs ~250 ms of CPU
`apps/api/internal/features/auth/routes.go:12-15`

The dummy-hash timing defence (`service.go:80`) is correct and does burn a real cost-12
comparison (verified: 268 ms), which means an unauthenticated attacker can consume a full
core with ~4 requests/second and saturate a small instance with a few dozen. There is also no
lockout on repeated failures for a single account.

**Fix.** Not necessarily this phase, but it should be an explicit backlog item rather than an
omission: per-IP and per-account throttling in front of `/auth/login`, `/auth/register`, and
`/auth/refresh`. Note it in `docs/api-guidelines.md` next to the extension points if it is
deferred.

### M4 — Access tokens survive deletion, demotion, and logout for up to 15 minutes
`apps/api/internal/middleware/auth.go:37-47`

`RequireAuth` trusts the `role` claim and never consults the database. Consequences:
a soft-deleted admin keeps full admin API access until the access token expires; an admin
demoted via `PATCH /users/:id` keeps admin authority for the same window; logout revokes the
refresh family but not the outstanding access token.

This is a normal stateless-JWT trade-off and 15 minutes is a defensible window, but it is
currently undocumented, which makes it look like an oversight to the next reader.

**Fix.** Document the window explicitly in the Authentication section of
`docs/api-guidelines.md`; if a tighter bound is needed later, add a `token_version` column
checked on refresh.

### M5 — Validation field names are derived from Go field names, not JSON tags
`apps/api/internal/shared/validation/validation.go:33-35`

`strings.ToLower(fe.Field())` happens to be correct for every current DTO because all fields
are single words. The first multi-word field (`PerPage`, `PasswordHash`, `FirstName`) will
silently emit `perpage` instead of `per_page`, breaking the client contract with no test
failure. Numeric fields will also report `min`/`max` as "characters"
(`validation.go:44-47`).

**Fix.** Register the JSON tag name once at startup:

```go
if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
    v.RegisterTagNameFunc(func(f reflect.StructField) string {
        name := strings.SplitN(f.Tag.Get("json"), ",", 2)[0]
        if name == "-" { return "" }
        return name
    })
}
```

and keep `fieldName` as the fallback. Add a `validation` unit test — this package also has no
tests.

### M6 — Environment guards are inconsistent, and staging gets the weaker half of each
- `apps/api/internal/cli/seed.go:25` refuses only when `IsProduction()`. A staging deployment
  (any `API_ENV` that is not `production`) can be seeded with the well-known credentials in
  `seeds/seed.go:28-33` with no flag.
- `apps/api/internal/features/auth/handler.go:119` sets `Secure` only when `IsProduction()`,
  so a staging deployment served over HTTPS issues a non-Secure refresh cookie. (The
  production-only rationale itself is accepted and not re-litigated.)
- By contrast, `apps/api/internal/cli/migrate.go:51` correctly guards on `!IsDevelopment()`.

**Fix.** Standardise the risky-operation guards on `!cfg.IsDevelopment()` — i.e. seed refuses
outside development without `--force`, and `Secure` is set unless running in development.
That preserves the localhost/Safari behaviour exactly while closing staging.

### M7 — `Update` is a full-row `Save`, so concurrent PATCHes silently lose writes
`apps/api/internal/features/users/repository.go:99-102`, called from `users/service.go:120-137`

Read-modify-write with no optimistic concurrency: two concurrent `PATCH /users/:id` (say one
changing `name`, one changing `role`) each load the row, mutate one field, and `Save` the
whole struct — last writer wins and silently reverts the other. `Save` also rewrites `email`
and `password_hash` from the in-memory struct, so any future caller that hands `Update` a
partially populated `User` will blank those columns.

**Fix.** Switch to a column-scoped update (`Model(&User{ID: id}).Updates(map[string]any{...})`)
built from the non-nil DTO fields, and check `RowsAffected` to detect a concurrently deleted
row.

### M8 — `refresh_tokens` grows without bound
`apps/api/migrations/000002_create_refresh_tokens.up.sql`

Every refresh inserts a row and nothing ever deletes one. With a 15-minute access TTL and a
30-day refresh TTL, an active user contributes ~2,900 rows per month, all retained forever.

**Fix.** Add a pruning path — a `DELETE FROM refresh_tokens WHERE expires_at < now() - interval '30 days'`
in a scheduled command (natural fit alongside the existing Cobra commands) plus an index on
`expires_at` to support it.

---

## Low

- **L1 — `q` filter passes LIKE metacharacters through unescaped.**
  `apps/api/internal/features/users/repository.go:80-83`. Not SQL injection (parameterised),
  but `q=%` matches everything and `q=%_%_%_%` forces a pathological scan. The double
  leading-wildcard `ILIKE` cannot use a btree index, so every `GET /users?q=` is a sequential
  scan. Escape `%`/`_` in the input and plan a `pg_trgm` GIN index if the table is expected to
  grow.
- **L2 — Handler reaches into the service's private field.**
  `apps/api/internal/features/auth/handler.go:105`, `:116` read `h.svc.issuer.…`. Legal
  (same package) but it breaches the handler → service boundary the contract sets up. Expose
  `svc.AccessTTL()` / `svc.RefreshTTL()` instead.
- **L3 — Cookie path is hardcoded and duplicated from the mount point.**
  `apps/api/internal/features/auth/handler.go:20` hardcodes `/api/v1/auth` while
  `routes.go:11` derives the real path from the injected group. Changing the API prefix
  silently breaks refresh, with no compile or test failure. Derive it or assert it once.
- **L4 — `000001` down leaves `citext`/`pgcrypto` installed.** Deliberate and correct
  (dropping a shared extension can break unrelated objects), but the intent should be a
  comment in the down file so the next reader does not "fix" it.
- **L5 — `users_deleted_at_idx` is a full index on a column that is NULL for nearly every row**
  (`000001_create_users.up.sql`), while every query filters `deleted_at IS NULL`. The plan
  asked for it, so keep it, but `WHERE deleted_at IS NOT NULL` would be the useful form.
- **L6 — `admin create` bypasses DTO validation.** `apps/api/internal/cli/admin.go:58-63`
  builds `users.CreateRequest` directly, so `--email not-an-email` is accepted (only the
  length-8 password rule is applied at `:39`). Run the same validator over the struct.
- **L7 — `closeMigrator` writes warnings with `fmt.Println`** (`cli/migrate.go:108-111`)
  while every other CLI message uses `cmd.Println`, so warnings bypass Cobra's output stream
  and are invisible to tests.
- **L8 — Registration allows account enumeration** via the 409 `email already in use`
  (`users/service.go:45`), whereas login is correctly generic. Usually an accepted trade-off;
  worth one line in the docs so the asymmetry reads as a decision.

---

## Edge Cases Checked and Found Sound

Recorded so they are not re-reviewed later:

- **SQL injection via `sort`** — closed. `pagination.Parse:47-53` resolves the key through the
  feature whitelist and falls back to the default; the raw query value never reaches
  `Order()`.
- **JWT alg confinement** — closed. `jwt.WithValidMethods([]string{"HS256"})`
  (`middleware/auth.go:31`) plus `token.Valid`, `sub` parsed as UUID, and empty `role`
  rejected. No `none`-alg or RS/HS confusion path.
- **Middleware abort semantics** — closed. `response.Err` uses `AbortWithStatusJSON`
  (`response.go:55`), so `RequireAuth`/`RequireRole` rejections genuinely stop the chain.
- **Token logging** — closed. Grep of the change set finds no token, cookie, or Authorization
  header in any log call; `Logger` logs only method/path/status/latency/IP; `users.Response`
  omits `password_hash`.
- **Soft-delete vs unique email** — closed. Partial unique index `WHERE deleted_at IS NULL`
  matches GORM's `gorm.DeletedAt` behaviour, and `TranslateError: true` (`postgres.go:22`)
  maps 23505 → `gorm.ErrDuplicatedKey` → `ErrDuplicateEmail` → 409.
- **Timing side channel on login** — closed and genuinely effective. The dummy hash at
  `auth/service.go:80` is a structurally valid cost-12 bcrypt string; measured 268 ms, matching
  a real comparison.
- **Postgres 14 compatibility** — closed. `gen_random_uuid()` (core since PG13), `citext`,
  partial unique index; no PG15+ syntax (`NULLS NOT DISTINCT`) anywhere.
- **Transaction context plumbing** — closed. `FromContext` falls back to
  `fallback.WithContext(ctx)` so cancellation propagates outside transactions, and nested
  `WithinTx` joins the ambient transaction instead of deadlocking on a second connection.
- **Authorization defence in depth** — closed. Route middleware plus service-level re-checks;
  `authctx.From` returning the zero `Principal` on an unauthenticated path fails closed
  (`IsAdmin() == false`).

---

## Recommended Actions

1. **H1** — return `RowsAffected` from `Revoke`, treat 0 as reuse, revoke the family; update
   the fake repository and add a test for the lost race.
2. **H3** — validate password length in bytes and map `bcrypt.ErrPasswordTooLong` to 422;
   align the `name` max between the two DTOs.
3. **H2** — propagate non-`NOT_FOUND` errors from `Refresh` and stop clearing the cookie on 5xx.
4. **M1** — clamp `page`; add `pagination` and `validation` unit tests (both packages are
   shared infrastructure with zero coverage today).
5. **M2** — move bcrypt hashing outside the registration transaction.
6. **M6** — standardise the seed guard and cookie `Secure` flag on `!IsDevelopment()`.
7. **M5, M7** — register the JSON tag-name func; switch `Update` to column-scoped updates.
8. **M4, M8, M3** — document the access-token revocation window; schedule token pruning and
   auth rate limiting.

## Metrics

- Build / vet / test: green. Lint (`golangci-lint v2`): **0 issues**.
- Type safety: no `any` widening, no `interface{}` escape hatches, no lint suppressions
  introduced in the change set.
- Test coverage: both services covered by fake-repository unit tests (7 auth cases, 6 users
  cases) exercising real behaviour, not phantom assertions. Untested: `pagination`,
  `validation`, `middleware/auth`, `database/tx`, `cli`. Integration coverage is Phase 4 by
  plan and is not counted against this phase.
- Scope discipline: no unrelated files, no speculative abstractions, no parallel
  reimplementation of existing helpers. The change set matches the plan's file list.

## Unresolved Questions

1. Should staging (`API_ENV` neither `development` nor `production`) be treated as production
   for the seed guard and the cookie `Secure` flag (M6)? This is a product/ops decision.
2. Is auth rate limiting (M3) in scope for Phase 4, or does it belong to the deployment layer
   (reverse proxy / WAF)? It should be recorded somewhere either way.
3. `RegisterRequest.Name` allows 255 characters and `users.CreateRequest.Name` allows 100 —
   which is the intended contract for the column?
