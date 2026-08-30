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

The tenant is the **center** (migration 000007). Every domain table carries a
`center_id`; `teacher_id` remains as attribution within the center. Composite
foreign keys `(id, center_id)` stop cross-center **writes**, but nothing except
a `WHERE` clause stops cross-center **reads** — and teacher-vs-teacher
isolation inside one center exists *only* at the query layer. Rules, applied
without exception:

- Handlers learn the tenant only from `authctx.ScopeFrom(c)` — never from a
  request body, query string, or path segment. A client-supplied `center_id`
  or `teacher_id` is an authorization bypass, and it looks completely ordinary
  in a diff.
- `Scope{TeacherID, CenterID, IsOwner, CanSendReports, Perms}` is resolved
  fresh from the database on every request by `middleware.ResolveScope` and
  never cached in the JWT, so a membership or permission change (kick, leave,
  join, grant, revoke, role edit) takes effect on the very next request.
- Every repository over a tenant table funnels reads through a `scoped`
  helper: always filter by center; callers without center-wide data access
  additionally filter by their own `teacher_id` (reference implementation:
  `apps/api/internal/features/students/repository.go`):

```go
// scoped returns a query bound to one center. A center-wide caller sees every
// student in the center; anyone else sees only the rows they created.
func (r *gormRepository) scoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
    q := database.FromContext(ctx, r.db).Where("students.center_id = ?", sc.CenterID)
    if !sc.CenterWide() {
        q = q.Where("students.teacher_id = ?", sc.TeacherID)
    }
    return q
}
```

`Scope.CenterWide()` (`IsOwner || Has(data.view_center_wide)`) is the **only**
data-scoping switch repositories may branch on — never `IsOwner` directly, so
an owner-granted permission widens reads without touching every repository.

- Writes keep the invariant `teacher_id = $self` for the teacher role; owners
  may write on behalf of any teacher in their center.

**Class-teacher roster reads**: a class handoff moves `classes.teacher_id`
(plus schedules and future sessions) but never the enrollment or student rows,
so own-rows scoping alone would show the new teacher an empty roster. Roster
READ paths therefore widen to "own rows OR the row's class is currently
assigned to the caller": `enrollments` `GetByID`/`List`/`ActiveOn`
(`readScoped` in `enrollments/repository.go`), attendance's `StudentNames`,
and `students` `GetByID`/`List` (`readScoped` in `students/repository.go`,
matching students with any enrollment row in a class assigned to the caller).
Everything else keeps plain member scoping — enrollment and student writes
(end, delete, create, update) stay with the creator or the owner, and the
contacts feature itself is untouched. Widened student rows do carry the linked
contact's name and phone (the roster table shows them), but the class teacher
cannot browse or manage the creator's contact book. Because
`students.Service.Update` saves by primary key after a widened `GetByID`, it
re-checks row ownership in the service before saving.

**Delegated report sending (`can_send_reports`)**: a boolean permission on the
member's live `center_members` stint, granted and revoked only by the owner
(`POST`/`DELETE /centers/me/members/:teacherId/send-reports`). It is a
capability flag, not a role, and it is member-only — the grant endpoint
refuses the owner as target, so `IsOwner` and `CanSendReports` never combine.
`Scope.ReportsOversight()` (`IsOwner || CanSendReports`) is the single helper
both capabilities branch on:

- **Center-wide read cluster**: billing periods, statements, debt views, the
  contact list (`GET /contacts` — recipient names and phones, needed to
  address a send), and the notification ledger (per-period sends and runs)
  scope to the whole center for a `ReportsOversight()` caller instead of the
  member's own rows. Everything else — classes, attendance, payments, single
  contact reads (`GET /contacts/:id`), and every write — keeps the plain
  member scoping above.
- **Send exclusivity**: only `ReportsOversight()` callers may create report
  sends — bulk send, run resume, and the pre-send preview all refuse everyone
  else. This 403 is deliberately honest (not the neutral not-found used for
  cross-tenant probes): the caller can see the period; the missing thing is
  the permission. Plain teachers provide attendance and remarks input and keep
  a read-only ledger of what was sent for their periods; they do not send.

*Release note (behavior removal)*: before this permission existed every
teacher could generate and send statements for their own periods. Now sending
is exclusive to the owner and `can_send_reports` holders — a teacher keeps
that ability only after the owner grants them the flag.

**Configurable permissions (RBAC, migration 000013)**: authorization checks
branch on a permission catalog, not on `IsOwner` (the sole exceptions are
listed below). Rules:

- The catalog is **code-owned**: keys, registry order, and Vietnamese labels
  live in `apps/api/internal/shared/authctx/permissions.go`. The database
  stores keys only; a key unknown to the running binary is ignored on read, so
  rolling the code back never grants or crashes anything.
- Effective set = (role permissions ∪ per-member grants) − per-member denies.
  Roles are per-center rows (`center_roles`, three system roles `giao_vien`,
  `hoc_vu`, `tro_giang`, born with empty sets); overrides are per-stint rows in
  `center_member_permissions`. Both are wiped when a membership closes and
  reset to defaults on reopen — permission state never survives a stint.
- The **owner is an implicit superuser outside the role tables**: their stint
  holds no role row, `Scope.Has(key)` is unconditionally true for them, and
  member-targeted permission endpoints refuse the owner as target (404, the
  `SetSendReports` precedent). Handlers gate with `scope.Has(authctx.Perm…)`;
  repositories branch only on `Scope.CenterWide()` as above.
- **Owner-only by design, not by catalog key**: the permission-management
  endpoints themselves (`GET /centers/me/permissions`, `PUT
  /centers/me/roles/:roleId/permissions`, `PUT
  /centers/me/members/:teacherId/role`, `PUT
  /centers/me/members/:teacherId/overrides`) check `scope.IsOwner` directly —
  a grantable "manage permissions" key would be one hop from self-escalation.
  Member removal and the send-reports grant stay owner-only for the same
  reason.
- **Dual life of `reports.send`** (until the flag column is dropped): the
  `can_send_reports` column stays authoritative; every mutation dual-writes
  column + override row in one transaction, the role matrix rejects
  `reports.send` (per-member only), and `ResolveScope` computes
  `CanSendReports = column OR Has(reports.send)`.
- Permission mutations are audited twice under the same action name: the
  request middleware row is the HTTP evidence (status, IP, failed attempts)
  and a service-published event row carries the committed before/after diff
  (empty `Method` distinguishes it). Clients read their own effective set from
  the `permissions` array on `GET /centers/me`.

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

**Invitation and reset-token discipline**: teacher onboarding
(`POST /centers/me/invitations`) and self-service password reset
(`POST /auth/forgot-password`) both mint an opaque 256-bit token
(`internal/shared/token`) — plaintext handed to the client once, only its
sha256 digest stored at rest. Invitations stay acceptable for `API_INVITE_TTL`
(default 72h); reset links expire after `API_RESET_TTL` (default 48h) and are
rate-limited by `API_RESET_COOLDOWN` (default 15m, one live token per
account). Both tokens are single-use and travel in the request body, never a
path segment, so they never land in an access log. Owners are excluded from
`forgot-password` by design — their phone gets the same generic response as
any other request, no token minted, no DM sent; their only recovery path is
the operator CLI's `reset-password`.

**Extension points (deliberately out of scope):** phone (OTP) verification and
parent/student portal auth are not implemented. They slot in as new endpoints
on the `auth` feature without changing the token model;
`user_accounts.password_hash` is already nullable for future OTP-only
accounts.

## Seeding

`api seed` inserts the development dataset idempotently (keyed by phone,
existing rows never modified) and refuses `API_ENV=production` without
`--force`. There is no public self-registration: teacher accounts come to
exist only via invitation accept (`POST /invitations/accept`), and the first
center/owner pair on a fresh database is bootstrapped by the operator CLI
(`api create-center`, atomic — center + owner land together or neither does).
`api reset-password` rewrites any account's password (including an owner's)
and revokes its refresh tokens; both are Cobra subcommands on the `cmd/api`
binary (`internal/cli`), alongside `serve`/`migrate`/`seed`.

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
