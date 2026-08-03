# API Guidelines

Owns: backend feature-module contract, error/response envelope, pagination
contract, validation rules, auth design, transaction conventions, backend
testing strategy.

## Response envelope

Every `/api/v1` response uses the envelope from `internal/shared/response`:

```json
{ "success": true,  "data": { }, "meta": { "page": 1, "per_page": 20, "total": 134, "total_pages": 7 } }
{ "success": false, "error": { "code": "VALIDATION_ERROR", "message": "…", "fields": { "email": "must be a valid email" } } }
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

## Feature modules

Each feature under `internal/features/<name>/` follows the same file contract:
`model.go` (GORM model mirroring the migration), `repository.go` (interface
first, GORM implementation below), `service.go` (business logic on the
repository interface), `dto.go` (request/response structs + mappers),
`handler.go` (bind → service → envelope, no business logic), `routes.go`
(`RegisterRoutes`), `service_test.go` (unit tests on a fake repository).

Features never import another feature's repository. Cross-feature calls go
service→service through an interface the consumer defines — e.g. `auth`
declares `UserService` with only the three `users.Service` methods it needs.

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

- **Access token**: HS256 JWT, 15 min default (`API_JWT_ACCESS_TTL`), claims
  `sub` (user id) + `role`. Verified by `middleware.RequireAuth`, which
  injects an `authctx.Principal`; `middleware.RequireRole("admin")` gates
  admin routes. Shared claim/principal types live in `internal/shared/authctx`
  so middleware and the auth feature agree without an import cycle.
- **Refresh token**: opaque 256-bit random value, stored as a sha256 hash,
  delivered in an httpOnly `SameSite=Lax` cookie scoped to `/api/v1/auth`.
  `Secure` is set in production only — Safari drops Secure cookies on
  `http://localhost`, which would break local development. Every refresh
  rotates the token within its family; presenting an already-rotated token
  revokes the whole family (replay defense). Logout revokes the family and is
  idempotent.
- **Passwords**: bcrypt cost 12. Login responds identically (401) for
  unknown email and wrong password, with a dummy bcrypt comparison on the
  unknown-email path to keep timing comparable.
- **Roles**: `admin` and `user`, checked in routes (middleware) and re-checked
  in services (defense in depth). Users read/update themselves but cannot
  change their own role; admins cannot delete their own account.
- **Revocation latency (accepted trade-off)**: access tokens are stateless,
  so after logout, soft-delete, or a role change an already-issued access
  token stays valid for up to its 15-minute TTL. Only refresh is revoked
  immediately. Introduce a token denylist only if the product ever needs
  instant revocation.

**Extension points (deliberately out of scope):** password reset, email
verification, and OAuth/social login are not implemented. They slot in as new
endpoints on the `auth` feature without changing the token model; email
verification would add a `verified_at` column via a new migration.

## Seeding and admin bootstrap

`api seed` inserts the development dataset idempotently (keyed by email,
existing rows never modified) and refuses `API_ENV=production` without
`--force`. `api admin create --email …` creates an admin, prompting for the
password without echo when `--password` is omitted so secrets stay out of
shell history.

The feature-module contract is summarized in
[architecture.md](architecture.md).
