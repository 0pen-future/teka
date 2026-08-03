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

Schema is owned exclusively by golang-migrate (Phase 3); `gorm.AutoMigrate` is
never used. Multi-write operations go through `database.TxManager` so services
stay transaction-aware but storage-agnostic.

_Pagination parameters, validation rules, and the auth design land with
Phase 3; the feature-module contract is summarized in
[architecture.md](architecture.md)._
