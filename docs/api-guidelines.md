# API Guidelines

Owns: backend feature-module contract, error/response envelope, pagination
contract, validation rules, auth design, transaction conventions, backend
testing strategy.

## Response envelope

Every `/api/v1` response uses the envelope from `internal/shared/response`:

```json
{ "success": true,  "data": { }, "meta": { "page": 1, "per_page": 20, "total": 134, "total_pages": 7 } }
{ "success": false, "error": { "code": "VALIDATION_ERROR", "message": "…", "fields": { "phone": "must be a valid Vietnamese phone number" } } }
```

- `meta` appears only on list responses (`response.List`).
- Health probes (`/healthz`, `/readyz`) are **outside** the envelope — their
  consumers are orchestrators, not API clients.

## Error handling

Services return `*apperror.AppError` (or errors wrapping one); handlers call
`response.Err(c, err)`, which maps the error onto the envelope. Unknown errors
become a generic 500 — the cause is logged, never sent to clients.

| Constructor | Code | HTTP status |
|---|---|---|
| `BadRequest` | `BAD_REQUEST` | 400 |
| `Unauthorized` | `UNAUTHORIZED` | 401 |
| `Forbidden` | `FORBIDDEN` | 403 |
| `NotFound` | `NOT_FOUND` | 404 |
| `Conflict` | `CONFLICT` | 409 |
| `Invalid` | `VALIDATION_ERROR` | 422 (with `fields`) |
| `Internal` | `INTERNAL_ERROR` | 500 (generic message) |

## Configuration

All environment access goes through `internal/config` (prefix `API_`).
`os.Getenv` outside that package is rejected by lint (`forbidigo`). Config is
validated at startup and the process exits with a message naming the offending
variable. In development a repo-root `.env` is loaded automatically; test and
production read the process environment only.

## Request lifecycle

Middleware order: request-id → logger → recovery → CORS. Every response carries
`X-Request-ID`; the request-scoped `slog.Logger` (with `request_id`) travels in
the request context — retrieve it with `logger.FromContext(ctx)`.

## Database

Schema is owned exclusively by golang-migrate; `gorm.AutoMigrate` is never
used. Migration SQL lives in `apps/api/migrations` and is embedded into the
binary (`embed.FS` + iofs source), so `api migrate up|down|status` works
without external files. `migrate down` rolls back one step by default;
`--all` requires `--yes` outside development.

Multi-write operations go through `database.TxManager`: `WithinTx` stores the
transaction handle in the context and repositories resolve it with
`database.FromContext(ctx, fallback)`, so the same repository methods work
inside and outside transactions and nested `WithinTx` calls join the ambient
transaction. Services own transaction boundaries.

## Tenancy

Every domain table carries a `teacher_id`; the composite foreign keys in the
schema stop cross-teacher **writes**, but nothing except a `WHERE` clause stops
cross-teacher **reads**. Two rules, applied without exception:

- Handlers learn the tenant only from `authctx.TeacherID(c)` — never from a
  request body, query string, or path segment. A client-supplied `teacher_id`
  is an authorization bypass, and it looks completely ordinary in a diff.
- Every repository over a `teacher_id` table funnels reads through a `scoped`
  helper (reference implementation:
  `apps/api/internal/features/teachers/repository.go`):

```go
// Every read is scoped to one teacher and skips soft-deleted rows.
func (r *gormRepository) scoped(ctx context.Context, teacherID uuid.UUID) *gorm.DB {
    return database.FromContext(ctx, r.db).Where("teacher_id = ?", teacherID)
}
```

`deleted_at IS NULL` comes free from `gorm.DeletedAt` on model-based queries;
raw SQL and `Table()` queries must add it by hand. Postgres row-level security
(schema note m) is deferred as a pre-launch hardening item.

## Feature modules

Each feature under `internal/features/<name>/` follows the same file contract:
`model.go` (GORM model mirroring the migration), `repository.go` (interface
first, GORM implementation below), `service.go` (business logic on the
repository interface), `dto.go` (request/response structs + mappers),
`handler.go` (bind → service → envelope, no business logic), `routes.go`
(`RegisterRoutes`), `service_test.go` (unit tests on a fake repository).

Features never import another feature's repository. Cross-feature calls go
service→service through an interface the consumer defines — e.g. `auth`
declares `AccountService` with only the `teachers.Service` methods it needs.

## Pagination

List endpoints parse `page`, `per_page` (default 20, max 100), and `sort`
through `internal/shared/pagination`. Sort columns are whitelisted per
feature; a leading `-` means descending; unknown columns fall back to the
feature default. Repositories apply `Params.Scope`; handlers return
`pagination.Meta(total)` in the envelope's `meta` block. List data serializes
as `[]`, never `null`.

## Validation

DTOs carry `binding` tags; handlers translate binding failures with
`validation.BindError(err)`: validator errors become a 422 envelope with
per-field messages, anything else (malformed JSON) becomes a 400.

## Authentication

- **Login identifier**: the Vietnamese phone number. Requests accept local
  (`0xxxxxxxxx`) or E.164 (`+84xxxxxxxxx`) form (custom `vnphone` binding
  validator); storage and lookups always use E.164
  (`validation.NormalizePhone`) so both spellings resolve to one account.
- **Access token**: HS256 JWT, 15 min default (`API_JWT_ACCESS_TTL`), claims
  `sub` (account id = teacher id) + `role`. Verified by
  `middleware.RequireAuth`, which injects an `authctx.Principal`. Shared
  claim/principal types live in `internal/shared/authctx` so middleware and
  the auth feature agree without an import cycle.
- **Refresh token**: opaque 256-bit random value, stored as a sha256 hash,
  delivered in an httpOnly `SameSite=Lax` cookie scoped to `/api/v1/auth`.
  `Secure` is set in production only — Safari drops Secure cookies on
  `http://localhost`, which would break local development. Every refresh
  rotates the token within its family; presenting an already-rotated token
  revokes the whole family (replay defense). Logout revokes the family and is
  idempotent.
- **Passwords**: bcrypt cost 12. Login responds identically (401) for unknown
  phone, disabled account, passwordless account, and wrong password, with a
  dummy bcrypt comparison on the non-compare paths to keep timing comparable.
- **Roles**: `teachers`, `parent`, `students` (mirroring the
  `user_accounts.role` CHECK constraint). V1 only issues teacher accounts;
  registration hard-codes the role server-side. `middleware.RequireRole`
  exists for later phases.
- **Profile**: `GET/PUT /api/v1/me` live on the `teachers` feature (auth owns
  credentials and sessions, not business data). Both re-check the account
  against the database, so a token issued before a soft-delete or disable
  stops working there immediately.
- **Revocation latency (accepted trade-off)**: access tokens are stateless,
  so after logout, soft-delete, or a role change an already-issued access
  token stays valid for up to its 15-minute TTL. Only refresh is revoked
  immediately. Introduce a token denylist only if the product ever needs
  instant revocation.

**Extension points (deliberately out of scope):** password reset, phone (OTP)
verification, and parent/student portal auth are not implemented. They slot in
as new endpoints on the `auth` feature without changing the token model;
`user_accounts.password_hash` is already nullable for future OTP-only
accounts.

## Seeding

`api seed` inserts the development dataset idempotently (keyed by phone,
existing rows never modified) and refuses `API_ENV=production` without
`--force`. There is no separate admin bootstrap — every account is a teacher
created via `/auth/register` or the seeder.

## Testing

The backend uses a three-layer pyramid; each layer has a distinct job and a
distinct wiring style:

| Layer | Files | Doubles | What it proves |
|-------|-------|---------|----------------|
| Unit | `*_test.go` (in-package, e.g. `service_test.go`) | Hand-written fakes (`fakeRepository`, `fakeAccountService`, `noopTxManager`) | Business rules: token rotation, role checks, validation, error mapping |
| HTTP | `handler_test.go` (in-package) | Same fakes behind the real router slice: real Gin routes, middleware, JWT parsing, envelope encoding | Status codes, envelope shape, auth/role gating, cookie flags |
| Integration | `integration_test.go` (`//go:build integration`, external `_test` package) | None — real PostgreSQL via testcontainers-go, real migrations | SQL correctness: partial unique indexes across soft delete, tenant isolation, pagination, transaction rollback |

Integration tests live in external `_test` packages because `testutil` imports
the feature packages; HTTP tests stay in-package to reuse the unexported fakes
from the unit tests.

**Fixtures and helpers** live in `apps/api/internal/testutil/`:

- `StartPostgres(t)` — one `postgres:16-alpine` container per test, embedded
  migrations applied, terminated via `t.Cleanup`.
- `Teacher(t, db, opts...)` — direct-insert account + teacher fixture pair
  with a unique random `+84` phone and `bcrypt.MinCost` hashing (fast,
  test-only).

Integration tests are excluded two ways: without `-tags=integration` they do
not compile at all (the `make test-api-unit` path), and under
`-tags=integration -short` they self-skip via `testing.Short()` before
touching Docker.

**Running tests:**

```sh
make test-api-unit   # unit + HTTP only (-short): fast, no Docker
make test-api        # everything + coverage gate (needs Docker)
make coverage-api    # HTML report from the last test-api run
```

Coverage is measured with `-coverpkg=./...` across the whole module; `make
test-api` fails when the total drops below the **60%** floor
(`API_COVERAGE_FLOOR` in the root Makefile). It lists only test-bearing
packages (a `go list` filter) because auto-downloaded Go toolchains lack the
`covdata` tool that `go test` would need to synthesize empty profiles for
test-less packages.

**Rules:**

- New SQL (a query with operators, joins, or index-dependent behavior) gets an
  integration test; plumbing CRUD is covered by service/HTTP layers.
- Assert on behavior the schema can mask: the phone unique index is partial
  (`WHERE deleted_at IS NULL`), so a duplicate-phone assertion alone cannot
  distinguish it from a full unique index — pair it with a soft-delete case
  when the distinction matters.
- Never weaken an assertion to make a test pass; fix the code or the fixture.

## OpenAPI docs

The spec is generated from swag annotations on handlers plus the root metadata
block in `cmd/api/main.go`, and served at `/swagger/index.html` in every
environment except production. Regenerate after changing any annotated handler
signature, request/response type, or route:

```sh
make api-docs   # runs: go tool swag init -g cmd/api/main.go -o docs --parseInternal
```

The generated `apps/api/docs/` package is committed: the router imports it for
its side-effect registration, and CI (Phase 8) diffs a fresh generation against
the committed baseline to catch drift. Annotate new endpoints following the
existing pattern — envelope composition like
`response.Envelope{data=TokenResponse}` and `@Security BearerAuth` on
authenticated routes.

The feature-module contract is summarized in
[architecture.md](architecture.md).
