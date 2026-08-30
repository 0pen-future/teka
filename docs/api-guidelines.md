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
// scoped returns a query bound to one center. An owner sees every student in
// their center; a member sees only the rows they created themselves.
func (r *gormRepository) scoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
    q := database.FromContext(ctx, r.db).Where("students.center_id = ?", sc.CenterID)
    if !sc.CenterWideFor(authctx.PermStudentsViewAll) {
        q = q.Where("students.teacher_id = ?", sc.TeacherID)
    }
    return q
}
```

`Scope.CenterWideFor(<resource>.view_all key)` (`IsOwner || HasKey(key)`) is
the **only** data-scoping switch repositories may branch on — never `IsOwner`
directly, and each repository passes its own resource's `view_all` scope key
(`authctx.PermStudentsViewAll`, `PermContactsViewAll`, …), so center-wide
visibility is granted per resource. The legacy single-axis
`data.view_center_wide` participates only through alias expansion at
permission-set build time (a legacy grant/deny expands to every per-resource
`view_all` key); `Scope.CenterWide()` survives solely for that compatibility
window and has no production callers.

- Writes on class-anchored artifacts resolve through the class-staff
  capability map (see class-staff writes below); the written rows still stamp
  `teacher_id = $self` as last-writer attribution, and owners may write on
  behalf of any teacher in their center. Contacts and students are the
  exception: they anchor to the owner (see contact-book ownership below).

**Class-staff reads**: `class_staff` stints (giao_vien / hoc_vu / tro_giang)
are the read-granting relationship for class-anchored data. A class handoff
moves `classes.teacher_id` (plus schedules, future sessions, and the giao_vien
stint — a dual write) but never the enrollment or student rows, and hoc_vu /
tro_giang members own no rows at all, so own-rows scoping alone would show
them nothing. READ paths therefore widen to "own rows OR the row's class
carries a class_staff stint for the caller — ended stints included, so
history stays readable after a soft-close". The shared SQL fragments live in
`internal/shared/classscope` (`ReadExists` for class-keyed rows,
`ReadExistsViaEnrollment` for student rows); each repository composes them in
a `readScoped` helper next to its own-rows `scoped`, and services expose the
widened queries as separate read ports (`GetReadable*`, `ListReadable`,
`ListRangeReadable`) so every write keeps resolving through the own-rows
gates. A soft-deleted class grants nothing. Widened today: classes,
enrollments, students, sessions, attendance sheets, grading reads, teaching
reads, and the class staff list itself. Everything else keeps plain member
scoping — writes resolve through the capability map below, never through a
read stint. Widened student rows carry the linked contact's name, but the phone
follows the phone-privacy rule below, and staff cannot browse or manage the
contact book (student record writes are owner-only, so the widened `GetByID`
never feeds a member save). Class responses carry a
per-caller `my_staff_roles` array, but only the GET paths populate it —
create/update/archive responses return it empty, so clients must invalidate
their class caches after a mutation rather than seeding them from the
response.

**Class-staff writes (capability map)**: class-anchored writes require an
ACTIVE stint whose role appears in the capability's role list —
`apps/api/internal/shared/authctx/class_staff.go` is the one authority:
`attendance.write` = giao_vien + tro_giang; `scores.write`, `remarks.write`,
`lesson_plan.write`, `enrollment.write`, `sessions.write` = giao_vien;
`statement.send` = hoc_vu. Owners pass every gate. The shared fragment is
`classscope.WriteExists` (active stint + role list), composed into per-table
`writeScoped` helpers; services resolve writes through `GetWritable*` ports
that disambiguate a miss honestly — a caller who can read the class (any
stint) but lacks the capability gets 403, an outsider gets 404, so session
and class ids cannot be probed. Consequences worth knowing:

- A handoff flips writes instantly: the outgoing giao_vien keeps history
  reads, but every write — including sessions they themselves recorded before
  the handoff — answers 403 the moment the stint closes.
- `teacher_id` on a written row (attendance records especially) is last-writer
  attribution, never a row filter: a trợ giảng saving over a teacher's rows
  takes the credit, and pricing tallies key on the enrollment/session, so
  assistant-recorded sessions bill fully into the class's period.
- Class-scoped statements (migration 000017): statements and notification
  runs carry a `class_id`; an assigned hoc_vu generates and sends a
  class-scoped statement copy through their own linked `zalo_personal`
  channel, the run is attributed to the sender, a class copy's URL token
  cannot open the family statement, and two hoc_vu sending for different
  classes in the same period do not conflict.
- `GET /billing-periods?class_id=` lets a class-staff member discover the
  period their class bills under (handed-off classes bill under another
  teacher's period, so the query deliberately drops the own-teacher filter);
  it requires a stint on that class or `ReportsOversight()`.

*Release note (behavior removal)*: before the capability map, writes followed
row ownership — a teacher who handed off a class could still edit the
sessions and attendance rows they had created. Now the active giao_vien stint
is the write anchor: the previous teacher's writes freeze at handoff, and
assistants/secretaries write only what their role's capabilities admit.

**Contact-book ownership (migration 000016)**: contacts and students are
center data anchored to the owner — `teacher_id = owner` on every row, seeded,
imported, or created. Create/update/delete on both is owner-only at the
service (an honest 403); a plain member's `GET /contacts` returns an empty
list rather than 403, and reads stay owner + `ReportsOversight()`. Imports
keep the grantable `imports.run` gate, but every imported contact/student row
is stamped with the owner anchor server-side regardless of who runs the
import, and dedupe resolves in owner scope. Migration 000016 merged duplicate
contacts per `(center_id, phone)` (earliest row survives), re-keyed the unique
indexes from per-teacher to `(center_id, phone)` / `(center_id,
zalo_user_id)`, then re-anchored existing rows — journaling every change in
`owner_anchor_backfill` for the down path. Teachers put existing students into
their classes through the enrollments picker (`GET
/classes/:id/enrollable-students` — names only, `q` ≥ 2 runes, capped at 20)
instead of creating records; enrollment creation is open to the class's active
giao_vien and publishes an audit event. Payments derive contact scope from
`contacts.teacher_id`, so new payments anchor to the owner and a member's debt
views drain over time.

*Release note (behavior removal)*: before this change every teacher kept a
private contact book and saw every phone they created. Now the owner manages
one center-wide contact book, members no longer create or edit contact and
student records, and a member sees a contact's phone only through the
phone-privacy rule below.

**Phone privacy**: one rule for every surface, list and detail alike — a
caller sees a contact's phone iff `IsOwner || ReportsOversight()` or the
caller holds an active hoc_vu stint on a class where a student of that contact
is actively enrolled. Repositories compute the per-row arm as a derived
`phone_visible` column (an EXISTS in the same query — fragments
`classscope.PhoneVisibleViaStudent` / `PhoneVisibleViaContact`); services
combine it through `Scope.PhoneVisible(rowVisible)` and null the DTO field —
`null`, never an empty string. Masked surfaces: student reads, statements
(including the family statement URL, which is a bearer token and is returned
only to `ReportsOversight()` callers), the notification ledger, and
collections. Sending paths (statements, notifications, zalo) read the phone
server-side, so a sender who cannot see a phone can still send.
`ContactResponse.Phone` stays a non-null string because contact reads are
already owner/oversight-only. Zalo friend-match and per-contact zalo-mapping
stay open to assigned hoc_vu — their send path depends on mapping.

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

**Configurable permissions (resource-action RBAC, migrations 000013/000018)**:
authorization checks branch on a permission catalog, not on `IsOwner` (the
sole exceptions are listed below). Rules:

- The catalog is **code-owned and structured**: `PermDef` entries (key,
  resource, action, kind `crud|scope|special`, Vietnamese label/description,
  risk, grantable, deprecated, order) live in
  `apps/api/internal/shared/authctx/catalog.go` under the pinned
  `authctx.CatalogVersion`. Keys are `<resource>.<action>` with canonical CRUD
  verbs, `<resource>.view_all` scope keys, and named specials; a registry
  guard test forbids any catalog key equal to a class-staff capability
  string. The database stores keys only; a key unknown to the running binary
  is dropped on read, so rolling the code back never grants or crashes
  anything, and unknown/deprecated/non-grantable keys are rejected (422) on
  write.
- **Every route is classified** in the route-policy registry
  (`apps/api/internal/server/route_policy.go`) as public, authenticated-self,
  owner-only, or permission-gated with its exact catalog key; a registry
  coverage test fails the build on any unclassified route, so enforcement
  fails closed. HTTP-level gating lives there — services use
  `authctx.Require` only for boundaries that bypass HTTP middleware.
- Effective set = (role permissions ∪ per-member grants) − per-member denies.
  Roles are per-center rows (`center_roles`, three system roles `giao_vien`,
  `hoc_vu`, `tro_giang`); overrides are per-stint rows in
  `center_member_permissions`. Both are wiped when a membership closes and
  reset to defaults on reopen — permission state never survives a stint.
  Legacy `data.*` keys remain accepted as **explicit aliases** through the
  soak window: a legacy grant expands to all its canonical keys, a legacy
  deny to all canonical denies, and a single-canonical deny never propagates
  back through the legacy key.
- Assignment writes are **versioned (compare-and-set)**: `GET
  /centers/me/permissions` returns the structured catalog with
  `catalog_version` plus a per-role/per-member `assignment_version`; the
  `PUT` writes require both and reject stale versions with 409 so a stale
  client can never erase permissions it has not seen (`0` skips the check
  for legacy clients).
- The **owner is an implicit superuser outside the role tables**: their stint
  holds no role row, `Scope.Has(key)` is unconditionally true for them, and
  member-targeted permission endpoints refuse the owner as target (404, the
  `SetSendReports` precedent). Repositories branch only on
  `Scope.CenterWideFor(<resource>.view_all)` as above.
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
