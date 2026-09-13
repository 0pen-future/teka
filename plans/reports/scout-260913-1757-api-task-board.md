# Scout: Go API Task Board (Kanban) — Architecture & Conventions

## 1. Go Module Layout

**Module:** `teka/apps/api` (go.mod: `/home/cesc/Documents/personal-workspace/teka/apps/api/go.mod`)

**Top-level structure:**
- `cmd/` — CLI entrypoints (migrations, seeds, onboarding operators)
- `internal/` — Feature packages and shared infrastructure
  - `features/<domain>/` — Feature modules (22 features: attendance, audit, auth, billing, centers, classes, classstaff, collections, contacts, enrollments, grading, handoff, imports, invitations, notifications, payments, sessions, statements, students, teachers, teaching, zalo)
  - `shared/` — Cross-feature utilities (apperror, authctx, classscope, events, id, logger, pagination, response, secrets, token, validation, routespec)
  - `app/`, `server/`, `middleware/`, `database/`, `config/`, `cli/`
- `migrations/` — SQL migration pairs (currently 21 numbered migrations up to `000021_attendance_status_late`)
- `tools/` — Binary tools (scopelint static analyzer)
- `docs/` — Generated Swagger/OpenAPI (excluded from edits)
- No `pkg/` shared-library layer: internal packages serve the API binary only; inter-feature decoupling via small consumer-defined interfaces injected at construction (not reusable outside teka)

## 2. Feature Package Anatomy (sessions + centers reference)

**File structure per feature (e.g., sessions/, centers/):**
- `model.go` — Entity/value objects (gorm tags, serialization)
- `dto.go` — Request/response structs (validation tags, pointers for nullability)
- `repository.go` — Interface + GORM impl; owns SQL scoping queries
- `service.go` — Business logic; composes repository + dependencies via small interfaces
- `handler.go` — HTTP handlers; unmarshals DTO, calls service, marshals response
- `routes.go` — Route registration
- `errors.go` — Feature-specific error constants (e.g., `ErrSessionExists`, `ErrNotFound`)
- `integration_test.go`, `*_test.go`, `*_integration_test.go` — Tests (unit + integration with Docker testcontainers)
- Other files: generators, event definitions, internal helpers

**Layering/Data Flow:**
```
Handler (gin.Context → DTO) 
  → Service (Scope/Anchor, business logic, txMgr)
    → Repository (authctx.Scope, SQL queries)
      → GORM → database.FromContext(ctx, r.db)
```

**Sessions repository scoping excerpt** (`sessions/repository.go:112–134`):

```go
// scoped = own rows; center_id + (owner OR teacher_id = me)
func (r *gormRepository) scoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
  q := database.FromContext(ctx, r.db).Where("class_sessions.center_id = ?", sc.CenterID)
  if !sc.WriteWide() {
    q = q.Where("class_sessions.teacher_id = ?", sc.TeacherID)
  }
  return q
}

// readScoped = own rows + class_staff stint rows + center-wide if sessions.view_all
func (r *gormRepository) readScoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
  q := database.FromContext(ctx, r.db).Where("class_sessions.center_id = ?", sc.CenterID)
  if !sc.CenterWideFor(authctx.PermSessionsViewAll) {
    frag, _ := classscope.ReadExists("class_sessions.class_id") // JOIN class_staff predicate
    q = q.Where("(class_sessions.teacher_id = ? OR "+frag+")", sc.TeacherID, sc.TeacherID, sc.CenterID)
  }
  return q
}

// writeScoped = active class_staff stint with role in roles (capability filter)
func (r *gormRepository) writeScoped(ctx context.Context, sc authctx.Scope, roles []string) *gorm.DB {
  q := database.FromContext(ctx, r.db).Where("class_sessions.center_id = ?", sc.CenterID)
  if !sc.WriteWide() { // Owner only
    frag, _ := classscope.WriteExists("class_sessions.class_id")
    q = q.Where(frag, sc.TeacherID, sc.CenterID, roles)
  }
  return q
}
```

**Scope/authz helpers:**
- `authctx.Scope` — Caller's full context: `TeacherID`, `CenterID`, `IsOwner`, `CanSendReports` (resolved from DB), `Perms` (PermSet)
- `authctx.Anchor` — Row identity (TeacherID + CenterID) for service operations on another teacher's data after Scope already gated permission
- `authctx.OwnerAnchor` — Strongly-typed anchor for owner-only operations (only `centers.Service.MintOwnerAnchor` can construct)
- Methods: `Scope.WriteWide()` (owner only), `Scope.CenterWideFor(key)` (owner OR perm key), `Scope.Has(key)`, `Scope.AnchorTo(teacherID)`, `Scope.Self()`

## 3. Authctx Catalog

**File:** `internal/shared/authctx/catalog.go` (lines 1–374)

**PermDef struct fields:**
```go
type PermDef struct {
  Key         string           // "classes.create", "classes.view_all"
  Resource    string           // "classes" (derived from key)
  Action      string           // "create", "view_all" (derived from key)
  Kind        PermKind         // crud, scope, special
  Label       string           // Vietnamese label
  Description string           // Vietnamese description
  Risk        PermRisk         // low, medium, high
  Grantable   bool            // false if deprecated
  Deprecated  bool
  Order       int             // Position in catalog
}
```

**CatalogVersion:** `const CatalogVersion = 3` (line 294)
- v1: legacy 9-key registry
- v2: resource-action catalog + deprecated `data.view_center_wide` alias
- v3: retired alias + unenforced scope keys (`scores.view_all`, `teaching.view_all`)
- Bumped on any change that alters meaning of stored assignments; clients carry it on reads, echo on writes, 409 if mismatch

**DefaultRoleKeys()** (lines 330–338):
```go
func DefaultRoleKeys() []string {
  // Every grantable operational key EXCEPT scope keys (no visibility widen) 
  // and legacy identity keys (already gated, would escalate)
  out := make([]string, 0, len(permCatalog))
  for _, d := range permCatalog {
    if d.Grantable && d.Kind != PermKindScope && !legacyIdentitySet[d.Key] {
      out = append(out, d.Key)
    }
  }
  return out
}
```

**legacyIdentitySet** (lines 300–309): 8 keys (reports.send, members.manage, center.manage, invitations.manage, audit.read, imports.run, dashboard.view, teaching.review_queue) — pre-catalog gates, never backfilled to avoid escalation

**impliedKeys** (lines 318–320): `reports.send` → {billing.view_all, statements.view_all, notifications.view_all, contacts.view_all} (read-only, applied post-resolution, never stored)

**Parity test:** `migrations/backfill_parity_test.go` — checksum test verifying SQL backfill artifacts match `DefaultRoleKeys()` at apply time

## 4. Routespec

**File:** `internal/shared/routespec/routespec.go`

**Specs struct:**
```go
type Spec struct {
  Method string  // "GET", "POST", "DELETE", "PUT", "PATCH"
  Path   string  // "/api/v1/classes/:id"
  Kind   Kind    // public, public_token, self, owner_only, permission, service
  Key    string  // Permission key if Kind==KindPermission; else ""
  Audit  Audit   // Source + Action/EntityType/IDParam for auditing
}

type Audit struct {
  Source     AuditSource // request, service, auth_session, anonymous, none
  Action     string      // "class.create" (required if Source==SourceRequest)
  EntityType string      // "class"
  IDParam    string      // "id" (route param carrying entity id)
}
```

**Specs declaration location:** `internal/shared/routespec/routespec.go:130+` (var Specs []Spec)

**Example entries** (lines 197–199):
```go
perm("PATCH", "/api/v1/centers/me", authctx.PermCenterManage, req("center.rename", "center", "")),
perm("DELETE", "/api/v1/centers/me/members/:teacherId", authctx.PermMembersManage,
  req("center.member.remove", "teacher", "teacherId")),
```

**Fail-closed manifest tests:** `internal/server/router_test.go` (lines TBD)
- `TestRoutePolicyCoversEveryRegisteredRoute` — every route in engine.Routes() must be in Specs
- `TestMutatingRoutesHaveAnAuditSource` — POST/PUT/PATCH/DELETE must have audit Source
- `TestRequestSourceRoutesHaveAnAction` — SourceRequest routes must have non-empty Action
- `TestSkipSourceNeverSharesPathWithAuditedRoute` — SourceNone paths never overlap with audited routes

## 5. Audit: Request Events → Audit Logs

**Flow:**
1. Handler calls service, service returns or errors
2. Middleware: `request_events.go` publishes `http.request_completed` event (mutating routes only, post-c.Next())
3. Bus: event queued (buffer: `API_AUDIT_BUFFER_SIZE` default 1024)
4. Subscriber: `audit/subscriber.go` batches by size (`API_AUDIT_BATCH_SIZE` default 100) or interval (`API_AUDIT_FLUSH_INTERVAL` default 1s)
5. `audit/subscriber.go:LookupAction()` resolves route method + path to `Action`, `EntityType`, `IDParam` via `routespec.Specs`
6. Multi-row INSERT → `audit_logs` (at-most-once delivery; full buffer drops event + warning log)

**Example write handler emitting audit** (centers/handler.go:114–129):
```go
func (h *Handler) removeMember(c *gin.Context) {
  scope, ok := h.scope(c)
  if !ok { return }
  targetID, err := uuid.Parse(c.Param("teacherId"))
  if err != nil {
    response.Err(c, apperror.NotFound("member"))
    return
  }
  if err := h.svc.RemoveMember(c.Request.Context(), scope, targetID); err != nil {
    response.Err(c, err)
    return
  }
  c.Status(http.StatusNoContent)
}
```
Route is classified in Specs: `DELETE /api/v1/centers/me/members/:teacherId` → `req("center.member.remove", "teacher", "teacherId")`. Middleware publishes event; subscriber translates it to audit row without handler involvement.

## 6. Transactions: TxManager

**Interface:** `internal/database/tx_manager.go:8–12`
```go
type TxManager interface {
  WithinTx(ctx context.Context, fn func(ctx context.Context) error) error
}
```

**Usage in centers/service.go:298–310** (RemoveMember):
```go
err = s.tx.WithinTx(ctx, func(ctx context.Context) error {
  if err := s.repo.CloseMembership(ctx, targetID, scope.CenterID); err != nil {
    return err
  }
  return s.disabler.Disable(ctx, targetID)  // auth.Service.Disable (disable + revoke tokens)
})
```

**Context propagation:** `WithinTx` opens transaction, passes ctx (with tx handle) to repositories via `database.FromContext(ctx, r.db)`. All repo calls in fn read tx from ctx and run inside it. On error or panic, rolls back; on success, commits.

**Actual implementation:** `database.GormTxManager` wraps gorm.DB.WithTx(fn).

## 7. Centers Service: AccountDisabler Interface & RemoveMember

**File:** `internal/features/centers/service.go`

**AccountDisabler interface** (lines 19–26):
```go
type AccountDisabler interface {
  Disable(ctx context.Context, accountID uuid.UUID) error
}
```
Consumer-defined; implemented by `*auth.Service`. Disables account row + revokes all refresh tokens atomically (a removed member cannot resume with a cached token).

**Injection pattern:**
```go
// Setter (line 57–59), not constructor param, because auth.Service depends on 
// teachers.Service as its AccountService
func (s *Service) SetAccountDisabler(d AccountDisabler) {
  s.disabler = d
}
```

**RemoveMember flow** (lines 277–311):
1. Authorization check: `scope.Has(PermMembersManage)`
2. Guards: not self, not owner
3. Verify member exists in center
4. Transaction: CloseMembership (soft-delete stint + clear perms + end class stints) + Disable (account + tokens)
5. Translate errors (ErrNotFound → 409 Conflict "membership changed concurrently")

**Wiring location:** `internal/app/container.go:100`
```go
centersSvc.SetAccountDisabler(authSvc)
teachersSvc.SetTokenRevoker(authSvc)
```
Built in Container so operator CLI's onboarding commands reuse exact same wiring.

## 8. Centers Repository: CreateCenter + Role Seeding

**File:** `internal/features/centers/repository.go:349–374`

**CreateCenter excerpt:**
```go
func (r *gormRepository) CreateCenter(ctx context.Context, c *Center) error {
  if err := translateError(database.FromContext(ctx, r.db).Create(c).Error); err != nil {
    return err
  }
  // INSERT 3 system roles
  if err := database.FromContext(ctx, r.db).Exec(`
    INSERT INTO center_roles (id, center_id, key, name)
    VALUES (gen_random_uuid(), @cid, 'giao_vien', 'Giáo viên'),
      (gen_random_uuid(), @cid, 'hoc_vu', 'Học vụ'),
      (gen_random_uuid(), @cid, 'tro_giang', 'Trợ giảng')`,
    map[string]any{"cid": c.ID}).Error; err != nil {
    return err
  }
  // INSERT default operational permissions into each role
  return database.FromContext(ctx, r.db).Exec(`
    INSERT INTO center_role_permissions (role_id, permission_key)
    SELECT cr.id, k
    FROM center_roles cr
    CROSS JOIN unnest(string_to_array(?, ',')) AS k
    WHERE cr.center_id = ?`,
    strings.Join(authctx.DefaultRoleKeys(), ","), c.ID).Error
}
```
- Three rows in center_roles (giao_vien, hoc_vu, tro_giang) with is_system=true
- Each role seeded with DefaultRoleKeys() — baseline operational permissions (no scope/legacy identity keys)
- New members join under giao_vien by default (OpenMembership query, line 405)

## 9. Migrations

**Directory:** `apps/api/migrations/`

**Naming:** NNNNNN_slug.up.sql / NNNNNN_slug.down.sql (e.g., `000021_attendance_status_late.up.sql`)

**Latest:** 000021 (as of 2026-09-01)

**File style:**
- `.up.sql` — forward transformation (CREATE TABLE, ALTER, INSERT backfill, etc.)
- `.down.sql` — reverse (DROP, TRUNCATE, DELETE, etc.)
- Embedded via `go:embed` in `embed.go`, applied by golang-migrate v4

**Parity test:** `migrations_test.go` + `backfill_parity_test.go`
- Runs migrations from bare Postgres (testcontainers)
- Verifies all domainTables exist with correct columns
- Verifies centerTables carry center_id and tenancy invariants
- Checksum test: backfill SQL artifacts match DefaultRoleKeys() at migration apply time (prevents silent permission drift)

**Backup script:** Not a Make target. Manual `pg_dump` or `docker-compose exec postgres pg_dump …` before migration on production (handled by deployment pipeline, not local dev).

## 10. Handler Conventions: Error Handling, DTO Validation, Caller Identity

**Error types & HTTP status:**
- `apperror.BadRequest(msg)` → 400
- `apperror.Invalid(msg, fields map[string]string)` → 422 (validation fields included in response)
- `apperror.Unauthorized(msg)` → 401
- `apperror.Forbidden(msg)` → 403
- `apperror.NotFound(resource)` → 404 ("resource not found")
- `apperror.Conflict(msg)` → 409
- `apperror.Internal(err)` → 500 (hides err from client, logs it)
- `apperror.TooManyRequests(msg)` → 429
- `apperror.From(err)` — wraps unknown errors as Internal

**Response envelope:** `response.Envelope{data?, error?, message?}` (response/response.go)
- Success: `c.JSON(200, Envelope{Data: data})`
- Error: `response.Err(c, err)` — reads err Code/Status/Message/Fields, returns envelope

**DTO validation:**
- Struct tags: `validate:"required,min=1,max=255"` (go-playground/validator/v10)
- Handler: parse request body with `c.ShouldBindJSON(&dto)` (Gin built-in validation)
- Invalid field errors → `apperror.Invalid(msg, fields)` with per-field messages

**Caller identity from context:**
```go
principal, ok := authctx.From(c)        // UserID, Role from verified JWT
scope, ok := authctx.ScopeFrom(c)       // TeacherID, CenterID, IsOwner, Perms (resolved from DB)
```
- Principal set by auth middleware (requires valid access token)
- Scope set by ResolveScope middleware (re-reads center membership + perms on every request)
- Center ID derives from scope, never from request params

## 11. Tests: Conventions & Quartet RBAC Pattern

**Test harness:**
- `go test ./apps/api/...` runs unit + HTTP tests (fast, no Docker)
- `make test-api` (integration, needs Docker): testcontainers-go postgres v16-alpine, one container per test run
- Test files in same package as source: `service.go` → `service_test.go` + `integration_test.go` (build tag `//go:build integration`)

**Test utilities:** `internal/testutil/` provides:
- `Teacher(t, db)` — create test account + teacher row
- `JoinCenter(t, db, teacherID, centerID)` — add member to center
- Database setup helpers (creates full tenant environment)

**RBAC "quartet" pattern** (centers/rbac_integration_test.go):

Four test cases for authorization:
1. **Owner bypass** — owner always allowed (no permission check)
2. **Member with grant** — permission key in role or override → allowed
3. **Member without grant** — no key → 403 Forbidden
4. **Member with deny override** — grant in role BUT explicit deny override → 403 (deny beats grant)

Example (TestResolveScopeEffectivePermissions, lines 97–130):
```go
// Assemble: member + dashboard.view grant + deny pair
require.NoError(t, e.db.Exec(`
  INSERT INTO center_role_permissions (role_id, permission_key, allowed)
  VALUES (?, ?, true), (?, ?, false)`,
  roleID, authctx.PermDashboardView, memberID, authctx.PermDashboardView).Error)

// Assert: deny beats role grant
sc := e.scope(t, member.ID)
require.False(t, sc.Has(authctx.PermDashboardView), "deny must beat the role grant")

// Assert: owner bypass (no check)
ownerScope := e.scope(t, owner.ID)
require.True(t, ownerScope.Has(authctx.PermAuditRead), "owner bypass")
```

Also tests: cross-center isolation (a grant in center A doesn't leak to center B).

**scopelint:** `make scopelint` (tools/scopelint/scopelint/analyzer.go)
- R1 "scope witness": repository methods accepting Scope/Anchor must visibly bind scoping via helper or .CenterID access
- R2 "no reset": forbidden .Session(&gorm.Session{NewDB: true}) and .Unscoped() in non-test files
- R3 "authority in authctx": Scope.IsOwner / .Has() / .CenterWideFor() forking only allowed inside functions with "read" in name; no hand-built Anchor/OwnerAnchor literals outside centers/middleware/testutil/authctx
- Runs on `go test ./tools/...` and before every `make test-api-unit`

## 12. Makefile Targets

**API-relevant:**
- `make test-api-unit` — run unit + HTTP tests, no Docker, includes scopelint
- `make test-api` — unit + integration (testcontainers Postgres), enforces coverage floor
- `make scopelint` — scoping linter (go run ./tools/scopelint ./internal/...)
- `make lint-api` — golangci-lint + scopelint
- `make api-docs` — regenerate swagger.json/swagger.yaml from swag annotations (via swag CLI tool)
- `make migrate-up`, `make migrate-down`, `make migrate-status` — apply/rollback/inspect migrations (via go run ./cmd/api migrate)
- No backup Make target; backups happen outside repo (deployment pipeline)

## 13. Existing Interface-Based Decoupling Between Features

**Consumer-defined interfaces injected at service construction:**

| Consumer | Interface Name | Provider |
|---|---|---|
| centers | AccountDisabler | auth.Service.Disable() |
| teachers | TokenRevoker | auth.Service.RevokeRefreshFamily() |
| students | EnrollmentEnder | enrollments.Service (closes open enrollments on student delete) |
| classes | StaffSeeder | classstaff.Repository (seeds class_staff when class created) |
| sessions | ClassSource, TeacherSource, EnrollmentSource | classes, teachers, enrollments services |
| attendance | RosterSource, SessionStore | enrollments, sessions services |
| attendance | BillingReconciler | billing.Service (reconciles money deltas on post-close attendance edit) |
| teaching | ClassSource, SessionSource, RosterSource | classes, sessions, enrollments services |
| grading | ClassSource, SessionSource, RosterSource | classes, sessions, enrollments services |
| handoff | SessionReassigner, MemberChecker, StaffReassigner | sessions, centers, classstaff services |
| imports | MemberDirectory, ClassWriter, ContactWriter, StudentWriter, EnrollmentWriter | centers, classes, contacts, students, enrollments services |
| invitations | ZaloSender, AccountOnboarder, MembershipOpener | zalo, auth, centers services |
| auth | AccountService, OwnerResolver, ResetDMSender | teachers, centers, zalo services |
| billing | AttendanceSource, PendingSource, EnrollmentSource | attendance, sessions, enrollments services |

**Setter-based cross-wiring** (avoid construction cycles): SetAccountDisabler, SetTokenRevoker, SetReconciler (app/container.go lines 100–101, 243).

**No direct registry or service locator;** all wiring explicit in app/container.go and server/router.go.

## 14. Generics & Ports-and-Adapters Patterns

**Minimal generics:** Go 1.25, but no parameterized types in API codebase (pre-generics design habits, acceptable legacy).

**Hexagonal / ports-and-adapters:**
- **Port = interface:** Repository interface (Repository, defined in service.go or as consumer-defined helper) = contract
- **Adapter = implementation:** GORM-backed repository struct (gormRepository, unexported) = concrete implementation
- **Domain = service:** Service struct, no persistence knowledge, talks only to Repository interface + injected dependencies
- **Driver = handler:** HTTP handler, unpacks DTO, calls service, packs response
- **Driven = repository + external services:** Database queries, auth disabler, zalo sender, etc., all behind interfaces

**Example flow:**
```
Handler → Service (talks to Repository interface, not GORM)
       → *gormRepository (implements Repository, uses GORM, scopes queries by Scope)
       → Database
```

No formal hexagonal framework (Vert.x, Quarkus analog); pattern achieved through small, focused interfaces and explicit DI.

## Architecture Docs

| File | Summary |
|---|---|
| `docs/architecture.md` | System overview: monorepo rationale, app boundaries, DI approach, request lifecycle, in-process events, graceful shutdown |
| `docs/api-guidelines.md` | Response envelope, error codes, tenancy scoping, pagination, validation, auth, testing conventions, handler patterns |
| `docs/event-bus.md` | Bus contract (async, at-most-once), event catalog, audit capture pipeline, subscriber wiring, known blind spots, tunables |
| `docs/adding-permissions.md` | Comprehensive permission-addition checklist: define capability, declare key, choose rollout, classify route, preserve scope, gate frontend, add tests, deploy |
| `docs/schema_design.sql` | Full schema: 40+ tables, composite foreign keys for center tenancy, CHECK constraints, indexes, comments on design choices |

---

**Status:** DONE

**Summary:** Codebase uses manual DI (no framework), small consumer-defined interfaces for inter-feature decoupling, read/write scoping via repository helpers, permission catalog with CatalogVersion CAS, audit trail via in-process event bus + batcher, testcontainers integration tests, and scopelint static analysis enforcing tenancy invariants. Ready for task-board feature design as reusable library with core domain + ports (repository interface, dependency interfaces), thin Teka adapter (DTO/handler/routes), and standard feature integration pattern.

**Concerns/Blockers:** None; repo is well-organized and extensively documented.
