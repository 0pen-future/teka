# Teka Go Backend (`apps/api`) — Inventory Report

Intermediate artifact for `docs-manager`. Read-only scout; no source modified.
Facts derived from source at commit `0c411fb` (branch `master`, 2026-08-19).
Citations are `path:line` relative to repo root unless noted.

**Stack**: Go 1.25, Gin 1.12, GORM 1.31 + pgx/v5, PostgreSQL 16, golang-migrate v4,
Cobra 1.10, golang-jwt/v5, caarlos0/env/v11, swaggo/swag, testcontainers-go 0.43.
Module path `teka/apps/api`. Single binary `cmd/api`.

---

## 1. Entry points & lifecycle

### 1.1 Binary + Cobra command tree

`apps/api/cmd/api/main.go` is 23 lines: it holds the `version` var (stamped by
`-ldflags "-X main.version=<git sha>"`), the swag root annotations (`@title Teka API`,
`@BasePath /api/v1`, `BearerAuth` apikey security definition), and calls `cli.Execute(version)`.

Command tree (`apps/api/internal/cli/root.go:19`), root `api`, `SilenceUsage`/`SilenceErrors` on:

| Command | File | Purpose | Guard rails |
|---|---|---|---|
| `serve` | `cli/serve.go` | Start HTTP API | — |
| `migrate up` | `cli/migrate.go:25` | Apply all pending migrations | idempotent (`ErrNoChange` swallowed) |
| `migrate down` | `cli/migrate.go:39` | Roll back; `--steps` (default 1), `--all`, `--yes` | `--all` outside development requires `--yes` (`migrate.go:51`) |
| `migrate status` | `cli/migrate.go:72` | Print version + dirty flag | — |
| `seed` | `cli/seed.go` | Insert dev dataset | refuses `API_ENV=production` without `--force` (`seed.go:25`) |
| `create-center` | `cli/create_center.go` | Bootstrap center + owner account | `--name`, `--owner-phone`, `--owner-name` required; `--force` mandatory; `--generate` or interactive prompt |
| `reset-password` | `cli/reset_password.go` | Rewrite password + revoke sessions | `--phone` required; `--force` mandatory; `--generate` or prompt |

`cli/root.go:34` keeps a `notYet(what)` helper for commands provisioned in later phases.

**Password input** (`cli/password_prompt.go`): deliberately **no `--password` flag** (shell
history leak). `--generate` mints a 20-char password from a 64-char alphabet using
`crypto/rand`, alphabet size chosen so `byte % 64` has zero modulo bias
(`password_prompt.go:20-29`). Otherwise `promptPassword` double-prompts stdin with echo off
via a `readPasswordLine = term.ReadPassword` func-var seam (unit-testable).

**`create-center` atomicity** (`cli/create_center.go:94-124`): one transaction —
insert `centers` row with `owner_id = centerID` (self-referencing placeholder satisfying
NOT NULL; the real FK check is `DEFERRABLE INITIALLY DEFERRED`, migration 000007) →
`teachers.CreateInCenter` → `centers.OpenMembership` → `UPDATE centers SET owner_id = <account id>`.
A duplicate phone rolls the whole thing back, so no ownerless center and no centerless owner
can exist. Takes `db/tx/teachersSvc/centersSvc` directly (not `*app.Container`) so it is
integration-testable without the secrets cipher / notifications / zalo wiring it never uses.

**`reset-password`** (`cli/reset_password.go:83-103`): `FindByPhone` → one tx of
`teachersSvc.SetPasswordForRecovery` + `authSvc.RevokeAllForUser`. Works on a **disabled**
account without changing its status. Reports "not found" as a plain error (not the
anti-enumeration rejection `auth.ForgotPassword` uses) — an operator is trusted.
This is the **only** recovery path for a center owner, who is excluded from `forgot-password` by design.

### 1.2 Bootstrap order (`serve`)

`cli/serve.go` → `signal.NotifyContext(SIGINT, SIGTERM)`; a goroutine calls `stop()` on
first cancellation so a **second Ctrl-C force-quits** instead of being swallowed during drain.

`app.RunServer` (`internal/app/app.go:13-37`):
1. `config.Load()`
2. `NewContainer(cfg)` (`defer c.Close()`)
3. `c.Zalo.StartHealthProbe(ctx, zalo.ProbeOptions{})` — process-lifetime goroutine
4. `c.Notifications.ReconcileInterrupted(ctx)` — a `running` notification run found at boot
   necessarily belongs to a dead previous process; mark `interrupted` (resumable) **before**
   any request can observe it (`app.go:28-33`)
5. `server.NewRouter(...)` 
6. `server.Run(ctx, cfg, log, router)` — blocks

`NewContainer` (`internal/app/container.go:50-110`) order and rationale:
1. `logger.New(...)` + `slog.SetDefault` — JSON handler in production, text otherwise
2. `secrets.New(cfg.Zalo.CredKey)` **before** the database: a credential key the cipher
   rejects would make every linked Zalo account unreadable — a reason not to start, not a
   runtime surprise; nothing is open yet to close (`container.go:54-61`)
3. `database.Open(cfg.Database)`
4. `zalo.NewService` → `database.NewTxManager` → `teachers.NewService` → `centers.NewService`
5. `auth.NewService(teachersSvc, repo, tokenIssuer, txMgr, centersSvc, zaloSvc, cfg.Onboarding, cfg.Statements.PublicBaseURL)`
6. Cross-wiring **setters** that break construction cycles (`container.go:86-87`):
   `centersSvc.SetAccountDisabler(authSvc)`, `teachersSvc.SetTokenRevoker(authSvc)`
7. `statements.NewService(repo, txMgr, cfg.Statements, bankCfg, NewQRBuilder())`
8. `notifications.NewService(repo, txMgr, statementsSvc, zaloSvc, log, cfg.Notifications)`

### 1.3 What `app.Container` owns and why

`Container` holds `Cfg, Log, DB, Zalo, Statements, Notifications, Teachers, Centers, Auth, TxManager`.
Two distinct reasons for a service to be here rather than in `server.registerFeatures`
(documented at `container.go:26-42`):

- **Process-lifetime background work**: `Zalo` (link attempts + session health probe),
  `Notifications` (paced `zalo_personal` send runs; consumes both Zalo and Statements),
  and `Statements` (constructor dependency of Notifications). `Container.Close` is where
  that background work stops.
- **Shared with the operator CLI**: `Teachers`, `Centers`, `Auth` — `create-center` and
  `reset-password` need the *exact same* identity wiring the HTTP server uses, including
  the two setters above. Building it twice would let the paths drift.

`server.NewRouter` therefore takes those six pre-built and constructs the other ten features itself.

### 1.4 Graceful shutdown

`internal/server/server.go`: `http.Server` with ReadHeader 5s / Read 10s / Write 30s / Idle 120s,
addr `:cfg.HTTP.Port`. `ListenAndServe` in a goroutine feeding `errCh`; on `ctx.Done()`,
`srv.Shutdown` with a **10s** `shutdownTimeout` on a fresh `context.Background()`.
`http.ErrServerClosed` is not an error.

`Container.Close` (`container.go:115-121`) orders teardown deliberately:
`Notifications.Close()` (runs send through zalo) → `Zalo.Close()` → `database.Close(DB)`,
so nothing is still using the pool when it goes away.

---

## 2. Request pipeline

### 2.1 Engine and middleware

`server.NewRouter` (`internal/server/router.go:47-83`):
- `gin.SetMode(ReleaseMode)` only when `cfg.IsProduction()`
- `gin.New()` (not `Default()`)
- **`r.SetTrustedProxies(nil)`** — `ClientIP()` uses the socket address, never a
  client-forgeable `X-Forwarded-For` (`router.go:53-56`). Direct consequence: IP is useless
  as a rate-limit key behind Traefik, which is why rate limits key on business identity.
- Middleware order (global, `router.go:57-62`): **RequestID → Logger → Recovery → CORS**

| Middleware | File | Behaviour |
|---|---|---|
| `RequestID` | `middleware/requestid.go` | Propagates inbound `X-Request-ID` or mints a UUIDv4; sets it on the response header and gin context |
| `Logger` | `middleware/logger.go` | Attaches request-scoped `slog.Logger` (with `request_id`) into `c.Request.Context()`; one line per request: method, path, status, `latency_ms`, `client_ip`. **`sanitizePath` redacts the `/public/statements/<token>` segment** so an access log never becomes a standing credential leak (`logger.go:24-33`) |
| `Recovery` | `middleware/recovery.go` | Panic → 500 envelope + stack log. Re-panics `http.ErrAbortHandler`; broken-connection panics (`*net.OpError`) log a warn and `c.Abort()` without writing |
| `CORS` | `middleware/cors.go` | `AllowCredentials: true` (refresh cookie crosses origins in host-mode dev), explicit origins only, exposes `X-Request-ID`, MaxAge 12h |

Route-scoped middleware: `RequireAuth` (`middleware/auth.go`), `ResolveScope`
(`middleware/scope.go`), `RateLimit` (`middleware/ratelimit.go`), `RequireRole`
(`middleware/auth.go:54` — **currently unused by any route**, kept for later phases).

`r.NoRoute` returns the standard envelope 404 via `apperror.NotFound("route")` (`router.go:78-80`).

### 2.2 Health probes

`internal/server/health.go` — mounted on the root engine, **outside the envelope** (consumers
are orchestrators, not API clients):
- `GET /healthz` → `200 {"status":"ok"}` — liveness, no dependencies
- `GET /readyz` → pings the DB with a **1s** timeout; `503 {"status":"unavailable","reason":"database unreachable"}` on failure

### 2.3 Swagger

`GET /swagger/*any` via `ginSwagger.WrapHandler(swaggerFiles.Handler)`, mounted **only when
`!cfg.IsProduction()`** (`router.go:66-69`). The generated spec is `apps/api/docs/docs.go`
(10 627 lines, blank-imported for its side-effect registration, `router.go:14`), regenerated by
`make api-docs` → `go tool swag init -g cmd/api/main.go -o docs --parseInternal`.

### 2.4 Routes outside `/api/v1` and/or outside auth

| Path | Auth | Notes |
|---|---|---|
| `GET /healthz`, `GET /readyz` | none | root engine, plain JSON, no envelope |
| `GET /swagger/*any` | none | root engine, non-production only |
| `GET /public/statements/:token` | none | root engine — the **only unauthenticated route in the product that serves child/money data** (`router.go:74-76`) |
| `GET /public/statements/:token/qr.png` | none | same group |

`statements.RegisterPublicRoutes(r, ...)` takes the `*gin.Engine`, not the v1 group, so no future
change to the authenticated group can accidentally pull it under a login requirement
(`features/statements/routes.go`). The group carries `securityHeaders()`
(`features/statements/public_handler.go:33-40`): `X-Robots-Tag: noindex, nofollow, noarchive`,
`Cache-Control: no-store, no-cache, must-revalidate, private`, `Referrer-Policy: no-referrer` —
mounted before lookup so it covers the 404 branch too. Every failure cause (unknown / malformed /
revoked / expired / soft-deleted / fully paid token) funnels through the single
`writeNeutralNotFound` (`public_handler.go:47-49`) so the body can never distinguish them.

### 2.5 Rate-limited public routes (inside `/api/v1`, outside auth)

`middleware.RateLimit(keyFn, limit, period)` — in-memory fixed-window counter, `sync.Mutex`,
**no background goroutine**: idle keys are swept inline in `allow`, throttled to at most once
per period (`middleware/ratelimit.go:28-84`). An empty key **skips** the limiter rather than
sharing one bucket (`ratelimit.go:86-98`). `JSONBodyKey(field)` reads the field from the JSON
body and restores `c.Request.Body` so downstream `ShouldBindJSON` still works.

| Route | Key | Limit | Registered at |
|---|---|---|---|
| `POST /api/v1/auth/forgot-password` | body `phone` | 5 / min | `router.go:110` |
| `POST /api/v1/auth/reset-password` | body `token` | 10 / min | `router.go:111` |
| `POST /api/v1/invitations/preview` | body `token` | 20 / min | `router.go:198` |
| `POST /api/v1/invitations/accept` | body `token` | 10 / min | `router.go:199` |

Rationale (`middleware/ratelimit.go:16-20`, `router.go:106-111`, `router.go:192-197`): with
`SetTrustedProxies(nil)` behind Traefik, `ClientIP()` collapses every caller into one bucket;
and for an unauthenticated invitee, IP is the only alternative and is far easier to rotate than
a 256-bit token. These four are the only unauthenticated **write** surface in the product.

### 2.6 Feature mount order (construction dependencies)

`registerFeatures` (`router.go:90-200`) order is load-bearing:
`centers → auth (+public) → teachers → contacts → classes → enrollments → students →
sessions → attendance → centers dashboard → billing → payments → collections → statements →
notifications → zalo → invitations (+public)`.

Two setters break what would otherwise be construction cycles:
- `attendanceSvc.SetReconciler(billingSvc)` (`router.go:166`) — billing needs attendance for
  `TallyByEnrollment`; attendance needs billing for post-close reconciliation.
- (in the container) `centersSvc.SetAccountDisabler(authSvc)`, `teachersSvc.SetTokenRevoker(authSvc)`.

---

## 3. Complete endpoint table

94 route registrations. `A` = `RequireAuth` + `ResolveScope`. `A+O` = additionally requires
`scope.IsOwner` (enforced in the service, not the router). `—` = unauthenticated.

### Root engine (outside `/api/v1`)

| Method | Path | Auth | Feature |
|---|---|---|---|
| GET | `/healthz` | — | server |
| GET | `/readyz` | — | server |
| GET | `/swagger/*any` | — | server (non-prod only) |
| GET | `/public/statements/:token` | — | statements |
| GET | `/public/statements/:token/qr.png` | — | statements |

### `/api/v1`

| Method | Path | Auth | Feature |
|---|---|---|---|
| POST | `/auth/login` | — | auth |
| POST | `/auth/refresh` | refresh cookie | auth |
| POST | `/auth/logout` | refresh cookie | auth |
| POST | `/auth/forgot-password` | — (RL 5/min · phone) | auth |
| POST | `/auth/reset-password` | — (RL 10/min · token) | auth |
| POST | `/invitations/preview` | — (RL 20/min · token) | invitations |
| POST | `/invitations/accept` | — (RL 10/min · token) | invitations |
| GET | `/me` | A | teachers |
| PUT | `/me` | A | teachers |
| GET | `/centers/me` | A | centers (owner gets roster, member gets name only) |
| PATCH | `/centers/me` | A+O | centers |
| DELETE | `/centers/me/members/:teacherId` | A+O | centers |
| GET | `/centers/dashboard/teachers` | A+O | centers (dashboard) |
| GET | `/centers/dashboard/overview` | A+O | centers (dashboard) |
| GET | `/centers/dashboard/teachers/:teacherId/classes` | A+O | centers (dashboard) |
| GET | `/centers/dashboard/teachers/:teacherId/classes/:classId/sessions` | A+O | centers (dashboard) |
| GET | `/centers/dashboard/sessions/:sessionId` | A+O | centers (dashboard) |
| POST | `/centers/me/invitations` | A+O | invitations |
| GET | `/centers/me/invitations` | A+O | invitations |
| DELETE | `/centers/me/invitations/:id` | A+O | invitations |
| POST | `/contacts` | A | contacts |
| GET | `/contacts` | A | contacts |
| GET | `/contacts/:id` | A | contacts |
| PUT | `/contacts/:id` | A | contacts |
| DELETE | `/contacts/:id` | A | contacts |
| PUT | `/contacts/:id/zalo-mapping` | A | contacts |
| DELETE | `/contacts/:id/zalo-mapping` | A | contacts |
| POST | `/classes` | A | classes |
| GET | `/classes` | A | classes |
| GET | `/classes/:id` | A | classes |
| PUT | `/classes/:id` | A | classes |
| POST | `/classes/:id/archive` | A | classes |
| DELETE | `/classes/:id` | A | classes |
| POST | `/classes/:id/schedules` | A | classes |
| PUT | `/classes/:id/schedules/:scheduleID` | A | classes |
| DELETE | `/classes/:id/schedules/:scheduleID` | A | classes |
| POST | `/students` | A | students |
| GET | `/students` | A | students |
| GET | `/students/:id` | A | students |
| PUT | `/students/:id` | A | students |
| DELETE | `/students/:id` | A | students |
| POST | `/enrollments` | A | enrollments |
| GET | `/enrollments` | A | enrollments |
| GET | `/enrollments/:id` | A | enrollments |
| POST | `/enrollments/:id/end` | A | enrollments |
| DELETE | `/enrollments/:id` | A | enrollments |
| GET | `/classes/:id/sessions` | A | sessions |
| POST | `/classes/:id/sessions` | A | sessions (ad-hoc) |
| GET | `/sessions/pending` | A | sessions (**registered before `/:id`** — Gin matches in registration order) |
| GET | `/sessions/:id` | A | sessions |
| DELETE | `/sessions/:id` | A | sessions |
| POST | `/sessions/:id/cancel` | A | sessions |
| POST | `/sessions/:id/uncancel` | A | sessions |
| POST | `/sessions/:id/hold` | A | sessions |
| GET | `/sessions/:id/attendance` | A | attendance |
| POST | `/sessions/:id/attendance` | A | attendance (confirm) |
| POST | `/billing-periods` | A | billing (ensure, idempotent) |
| GET | `/billing-periods` | A | billing |
| GET | `/billing-periods/:id` | A | billing |
| GET | `/billing-periods/:id/preview` | A | billing |
| POST | `/billing-periods/:id/draft` | A | billing |
| POST | `/billing-periods/:id/close` | A | billing |
| POST | `/invoices/:id/void` | A | billing |
| POST | `/invoices/:id/adjustments` | A | billing |
| GET | `/invoices/:id/adjustments` | A | billing |
| POST | `/payments` | A | payments |
| GET | `/payments` | A | payments |
| GET | `/payments/:id` | A | payments |
| PUT | `/payments/:id/allocations` | A | payments (manual reallocate) |
| POST | `/payments/:id/allocations/auto` | A | payments (place remainder) |
| POST | `/payments/:id/reverse` | A | payments |
| GET | `/billing-periods/:id/collections` | A | collections (`?view=contact\|class`) |
| GET | `/billing-periods/:id/collections/summary` | A | collections |
| POST | `/billing-periods/:id/statements/generate` | A | statements |
| GET | `/billing-periods/:id/statements` | A | statements |
| GET | `/statements/:id` | A | statements |
| POST | `/statements/:id/revoke` | A | statements |
| POST | `/billing-periods/:id/notifications/bulk` | A | notifications |
| GET | `/billing-periods/:id/notifications` | A | notifications |
| GET | `/billing-periods/:id/notifications/run` | A | notifications |
| POST | `/billing-periods/:id/notifications/run/resume` | A | notifications |
| POST | `/notifications/mark-sent` | A | notifications |
| GET | `/me/zalo` | A | zalo |
| DELETE | `/me/zalo` | A | zalo (unlink) |
| GET | `/me/zalo/friends` | A | zalo |
| POST | `/me/zalo/friends/match` | A | zalo |
| POST | `/me/zalo/friends/request` | A | zalo |
| POST | `/me/zalo/link/start` | A | zalo |
| GET | `/me/zalo/link/status` | A | zalo |

Note on zalo: `resolveScope` runs on `/me/zalo/*` **not** for center filtering (a Zalo account is
personal) but so a teacher kicked from their center loses access in the same request it bites
everywhere else (`features/zalo/routes.go:8-13`).

---

## 4. Feature-by-feature domain summary

Uniform file contract per feature (`docs/api-guidelines.md`): `model.go` (GORM model mirroring
the migration) · `repository.go` (interface first, GORM impl below) · `service.go` (business
logic on the repo interface) · `dto.go` · `handler.go` (bind → service → envelope, no logic) ·
`routes.go` · `errors.go` · tests. **Features never import another feature's repository**;
cross-feature calls go service→service through an interface the *consumer* declares.

### 4.1 Cross-feature dependency graph (consumer-declared interfaces)

| Consumer | Interface (declared at) | Methods | Satisfied by |
|---|---|---|---|
| attendance | `RosterSource` (`attendance/service.go:21`) | `ActiveOn(ctx, sc, classID, on) ([]enrollments.Enrollment, error)` | `*enrollments.Service` |
| attendance | `SessionStore` (`:28`) | `GetByID`, `MarkHeldAndConfirmed(ctx, sc, sessionID, at)` | `*sessions.Service` |
| attendance | `BillingReconciler` (`:59`) | `ReconcileSession(ctx, sc, sessionID) (Reconciliation, error)` | `*billing.Service` (**setter** `SetReconciler`) |
| auth | `AccountService` (`auth/service.go:39`) | `GetByPhone`, `GetByID`, `TouchLastLogin`, `Disable`, `SetPassword` | `*teachers.Service` |
| auth | `OwnerResolver` (`:59`) | `CenterOwner(ctx, teacherID) (ownerID, isOwner, err)` | `*centers.Service` |
| auth | `ResetDMSender` (`:68`) | `LookupPhone`, `SendDM` | `*zalo.Service` |
| centers | `AccountDisabler` (`centers/service.go:19`) | `Disable(ctx, accountID) error` | `*auth.Service` (**setter** `SetAccountDisabler`) |
| centers (dashboard) | `ClassReader`/`SessionReader`/`AttendanceReader` (`centers/dashboard.go:20/28/35`) | `Get`,`List` / `ListRangeReadOnly`,`Get` / `Get` | classes / sessions / attendance services |
| invitations | `ZaloSender` (`invitations/service.go:40`) | `LookupPhone`, `SendDM` | `*zalo.Service` |
| invitations | `AccountOnboarder` (`:52`) | `CreateInCenter`, `Reactivate`, `FindByPhone` | `*teachers.Service` |
| invitations | `MembershipOpener` (`:72`) | `OpenMembership`, `SwitchTeacherCenter`, `WasEverMember` | `*centers.Service` |
| notifications | `StatementsSource` (`notifications/service.go:30`) | `Generate`, `PeriodFigures`, `ToResponse` | `*statements.Service` |
| notifications | `ZaloSender` (`:41`) | `DMSender` + `VerifyAccount` | `*zalo.Service` |
| sessions | `ClassSource`/`TeacherSource`/`EnrollmentSource` (`sessions/service.go:30/38/46`) | `Get`,`ListEffectiveSchedules` / `GetByID` / `ActiveOn` | classes / teachers / enrollments |
| students | `EnrollmentEnder` (`students/service.go:21`) | `EndOpenEnrollments(ctx, sc, studentID, on)` | `*enrollments.Service` |
| teachers | `TokenRevoker` (`teachers/service.go:26`) | `RevokeAllForUser(ctx, userID)` | `*auth.Service` (**setter** `SetTokenRevoker`) |
| billing | `AttendanceSource` (`billing/repository.go:72`) | `TallyByEnrollment`, `SessionAttendance` | `*attendance.Service` (**injected into the repository**, not the service) |
| billing | `PendingSource` (`billing/close.go:30`) | `ListUnconfirmedInWindow` | `*sessions.Service` |
| billing | `EnrollmentSource` (`billing/adjustment.go:26`) | `ActiveOn` | `*enrollments.Service` |

Three setters exist purely to break construction cycles: `attendance↔billing`,
`centers→auth`, `teachers→auth`.

### 4.2 Per-feature summary

**auth** — credentials and sessions only (never business data; the profile lives on `teachers`).
Access token = HS256 JWT, claims `sub` (account UUID) + `role` + `iat`/`exp`, TTL 15m default;
no `jti`/`iss`/`aud` and **no center id** (tenancy is re-resolved per request). Refresh token =
opaque 32 random bytes → base64url plaintext, stored only as sha256-hex in `refresh_tokens` with
a `family_id`; TTL 720h. Delivered as httpOnly `SameSite=Lax` cookie on path `/api/v1/auth`,
`Secure` only in production (Safari drops Secure on `http://localhost`). **Rotation with reuse
detection**: `Refresh` revokes the presented token and issues a successor in the same family
inside one tx; presenting an already-revoked token revokes the **whole family**; an *expired*
(but unrevoked) token 401s without revoking the family. Losing the revoke race
(`ErrTokenAlreadyRevoked` from `RowsAffected == 0`) kills the family outside the tx so the
revocation survives the failed request (`auth/service.go:174-176`, `:211-218`). Login: unknown
phone / disabled / passwordless / wrong password all return the identical 401, and the three
non-bcrypt paths run `burnPassword` against a fixed dummy hash to equalise timing
(`auth/service.go:26`, `:153-157`). Logout revokes only the presented family, idempotent; there is
**no HTTP logout-all** (`RevokeAllForUser` is internal, used by `Disable`, `ResetPassword`,
`teachers.Reactivate`, and the CLI). `ForgotPassword` always returns the same body; silent no-op
branches = unknown phone, non-active account, **caller is the center owner**, and inside
`ResetCooldown`. `ResetPassword` runs `ConsumeResetTokenForUpdate` (`SELECT … FOR UPDATE`) →
liveness check → `SetPassword` → `MarkResetTokenUsed` → `RevokeAllForUser` in one tx; every
rejection collapses to one shared `errResetRejected` value so bodies are byte-identical.
No goroutines: the reset DM is sent **inline** under `context.WithoutCancel` + 10s timeout.

**invitations** — invite-only onboarding. Owner-only create/list/revoke; public preview/accept.
Token = `shared/token` (256-bit plaintext once, sha256-hex at rest), TTL 72h, travels in the
**body, never a path segment**. Stored statuses are only `pending|accepted|revoked`; **`expired`
is derived at read time** (`status == pending && expires_at < now`, `invitations/model.go:48-50`) —
no cron, no status writer. `Create` runs `RevokePendingForPhone` + `Create` in one tx; on
`ErrPendingExists` it *rotates the surviving row's token_hash* instead of erroring, keeping the
"one pending invite per (center, phone)" invariant with no duplicate row. `Accept` is one tx over
`GetByTokenHashForUpdate`: new phone → `CreateInCenter` + `OpenMembership`; already-active account
→ reject; disabled account → `WasEverMember` gate → `Reactivate` + `OpenMembership` +
`SwitchTeacherCenter`. All rejections share one `errAcceptRejected`. Best-effort Zalo DM after
commit, inline, 10s. No goroutines.

**teachers** — identity: `user_accounts` (login) + `teachers` (profile) sharing one UUIDv7 id,
which is also the JWT `sub` and every downstream `teacher_id`. Account status `active|disabled`;
role hard-coded to `authctx.RoleTeacher` on create. bcrypt cost **12**, owned entirely here (auth
never hashes); explicit `len(password) > 72` **byte** guard because the `max=72` binding tag counts
runes. `CreateInCenter` inserts on the ambient tx with no pre-check SELECT (the unique phone index
is the concurrency guard); opening `center_members` is the caller's job. `Reactivate` is a
conditional `UPDATE … WHERE status='disabled'` (race-safe) then revokes all refresh tokens.
`SetPassword` is active-only; `SetPasswordForRecovery` has no status filter and deliberately is
**not** part of `auth.AccountService` — its only caller is the operator CLI. `UpdateProfile` maps
only `full_name` + `timezone`, so a body can never touch role/status/phone. No goroutines.

**centers** — the tenant. `Center{ID, Name, OwnerID}`; `center_members{teacher_id, center_id,
joined_at, left_at}` is a **history, not a flag** — live stint = `left_at IS NULL`; rejoining
reopens the closed row; closed rows are **never deleted** because the business tables' guard FKs
cascade off them (a teacher's data stays in the center after they leave). `teachers.center_id` is
the current pointer. `ResolveScope` returns `authctx.Scope{TeacherID, CenterID, IsOwner}`; liveness
(teacher not soft-deleted ∧ center not soft-deleted ∧ account `active` ∧ account not soft-deleted)
is folded into the SQL and any miss collapses to `401 "account is not active"`. `CenterOwner`
deliberately does **not** filter on status (ForgotPassword's owner-exclusion must answer for a
disabled account too). `Me` gives the owner the full roster, a member only the center name.
`RemoveMember` (owner-only, cannot remove self) closes the membership and calls
`AccountDisabler.Disable` in one tx (status flip + revoke-all); `teachers.center_id` is left
pointing at the old center — the reactivate path moves it. **Dashboard** is a separate service
mounted after classes/sessions/attendance exist; every endpoint is owner-only, drill-down targets
are validated via `WasEverMember`, and reads run under `targetScope(sc, teacherID)` =
`{target, owner's center, IsOwner:false}` so consumed features scope exactly as the viewed teacher
would. `Overview` reports `InvoicedRevenue` as **null** (never a misleading 0) unless that month's
billing period is closed (`centers/dashboard.go:173-179`). No goroutines.

**contacts** — the người liên hệ (parent/guardian) who receives fee messages and pays them.
Fields: `FullName`, `Phone` (E.164), `UserID` (NULL throughout V1 — parents open a token link,
they don't log in), `ZaloUserID`/`ZaloName` (always set and cleared together; `ZaloName` is
snapshotted so lists render without refetching the friend list). **No relation column and no
primary flag** — the link is the reverse direction (`students.contact_id`, one contact → many
students). `uq_contacts_phone` is **per-teacher, deliberately not global**: a parent whose children
study with several teachers is several independent rows. Delete is soft and blocked by live
students (409 naming up to 5 of them). No TxManager, no goroutines.

**students** — closed field list: `FullName`, `ContactID`, `DisplayNote` (attendance-screen
disambiguator). **Delete = anonymise, not erase**: one tx of `EndOpenEnrollments` then
`AnonymizeAndDelete`, a single UPDATE setting `full_name = "Đã xoá"`, `display_note = NULL`,
`anonymized_at`, `deleted_at`. The row survives so financial FKs keep holding. List filters:
`Query`, `ContactID`, `ClassID` (via open enrollments = the class roster), `Unenrolled`.

**classes** — the lớp plus its weekly timetable. Schedules are a **separate table**
(`class_schedules`), not JSON: `Weekday int16` (0 = Sunday, deliberately equal to
`int(time.Sunday)` so it casts directly), `StartTime` (Postgres `TIME`, Go type is a `"HH:MM"`
string with `driver.Valuer`/`Scanner`), `DurationMin` (default 90), and a validity interval
`[EffectiveFrom, EffectiveTo]`. **Schedule changes close the old row and insert a replacement,
never edit in place**, so past sessions stay explicable. `DefaultUnitPrice` is only a *template* —
enrollments copy and freeze it. `Delete` refuses with 409 when open enrollments exist (archive
instead); `Archive` is idempotent. Create is transactional (class + all schedules or nothing).

**enrollments** — student↔class link with `StartedOn` / `EndedOn` (NULL = open) and
**`UnitPrice`, the frozen price copy** (PRD R1: the rate lives on the enrollment, not the class, so
raising a class price never rewrites past debts). `uq_enrollments_active` =
`UNIQUE(student_id, class_id) WHERE ended_on IS NULL AND deleted_at IS NULL` → at most one open
enrollment per pair, enforced by the index, not a pre-check. `ActiveOn` uses **both boundaries
inclusive** — an exclusive boundary would lose one session of revenue per departure. Overlapping
*closed* enrollments are not prevented. No TxManager.

**sessions** — materialisation is **on demand, no cron**. `generator.go` is pure and DB-free:
`Expand` clamps to `[class.StartDate, class.EndDate] ∩ [schedule.effective_from, effective_to] ∩
[from, to]`, walks day-by-day with `AddDate(0,0,1)` (never a fixed 24h step → DST-safe), dedupes at
UTC midnight, sorts. The calendar day comes from the **class's own teacher's** IANA timezone, so
generation is viewer-independent. Range capped at 400 days. Idempotency is Postgres's job:
`ON CONFLICT (class_id, session_date) WHERE deleted_at IS NULL DO NOTHING`, whose `TargetWhere`
must reproduce the partial index predicate or Postgres cannot match the index at all.
Status `planned|held|cancelled`; **"confirmed" is not a status** but the nullable
`attendance_confirmed_at` column. `Cancel` requires a reason and refuses a confirmed session;
`Uncancel` refuses anything not cancelled (reopening a held+confirmed row would produce
"planned but confirmed", which billing would silently drop). `MarkHeldAndConfirmed` runs on
`database.FromContext` so it joins attendance's transaction. `pending.go` = past sessions never
confirmed: two statements total regardless of size (a join-free `Count` that stays on the partial
index, plus a limited query with one grouped `LEFT JOIN enrollments`); default limit 50, max 200;
`ListUnconfirmedInWindow` is the shared predicate billing's close gate calls directly.
**Caveat**: `GET /classes/:id/sessions` **writes** (generates); `ListRangeReadOnly` is the
non-generating variant, used only by the centers dashboard.

**attendance** — one row per student per session **including present students**, so "có mặt" is
distinguishable from "chưa điểm danh". `EnrollmentID` is captured at confirm time so billing can
price a past session without re-deriving which enrollment was active. Statuses `present|absent|
excused`, but **`excused` is reserved for P1 and never written in V1**; `Billable` is written
unconditionally `true`. Soft-delete's only legitimate use is a student removed from the roster;
an absent student is never soft-deleted (that would change billable counts already reported to a
parent). `Confirm` = resolve session (409 if cancelled) → `ActiveOn` roster → reject absentee ids
not on the roster (422, listed by id, never silently dropped) → **one tx** of `UpsertMany` +
`SoftDeleteMissing` + `sessions.MarkHeldAndConfirmed`. Upsert sets status/enrollment/billable/note
but deliberately **not** `recorded_at`, so it survives edits and record ids stay stable. The
billing reconciliation runs **outside** that tx and is best-effort: attendance is already
committed, so a failure becomes `resp.Warning` on an otherwise-200, never a 5xx.

**billing / payments / collections / statements / notifications / zalo** — see §5 and §8.

### 4.3 Background goroutines (exhaustive, non-test)

| # | Location | Lifetime | Stopped by |
|---|---|---|---|
| 1 | `server/server.go:29` | `ListenAndServe` feeding an error channel | `srv.Shutdown` |
| 2 | `cli/serve.go:21` | signal re-arm so a 2nd Ctrl-C force-quits | process exit |
| 3 | `zalo/service.go:168` | health-probe sweep loop, one per Service | `Service.Close` → `stopProbe` |
| 4 | `zalo/link_manager.go:196` | one per QR link attempt, TTL 105s | `Cancel`/`Close`, awaited on `done` |
| 5 | `notifications/run_manager.go:200` | one paced send run per teacher | `Close`, awaited on `done` |

No cron/ticker sweeps anywhere else. `LinkManager` prunes expired records lazily inside
`sweepLocked` on every `Begin`/`Status` specifically to avoid a third goroutine; the rate limiter
sweeps inline for the same reason; `ReconcileInterrupted` is a one-shot boot call.
`classes`, `enrollments`, `students`, `contacts`, `sessions`, `attendance`, `auth`, `invitations`,
`teachers`, `centers`, `billing`, `payments`, `collections` contain **zero** goroutines.

---

## 5. Billing engine flow (the money path)

Money is **`int64` đồng — whole units, not cents**; no float, no decimal, no division, no rounding
function anywhere in billing or payments; all arithmetic is exact integer add/multiply. Currency is
implicit and single (no currency column or field). DATE values are always normalised to **UTC
midnight** (`billing.dateOnly`, `payments.today`).

```
attendance_records ──TallyByEnrollment──► billing.ComputePeriod ──► invoices + invoice_lines
       ▲                                        ▲                          │
       │ Confirm (tx)                           │ PreviousClosedPeriod     │ Close
       │                                   opening_balance (carried debt)  ▼
  class_sessions ──ListUnconfirmedInWindow──► close gate (409)      billing_periods.closed
       │                                                                   │
       └──ReconcileSession──► invoice_adjustments (on NEXT open period)     │
                                                                           ▼
payments ──Allocate──► payment_allocations ──RecalcInvoicePaid──► invoices.paid_amount/status
                                                     │
                                                     ├──► collections (read-only reporting)
                                                     └──► statements ──► notifications ──► zalo
```

**Step 1 — attendance tally.** `attendance.Service.TallyByEnrollment(ctx, sc, from, to)
([]EnrollmentTally, error)` groups non-deleted `attendance_records` by `enrollment_id`, joined to
`class_sessions` on `(id, center_id)`, filtered to `status='held' AND attendance_confirmed_at IS
NOT NULL AND session_date BETWEEN from AND to`, with `COUNT(*) FILTER` per bucket. This is
billing's **sole** entry into `attendance_records` — billing never writes its own aggregate.

**Step 2 — ensure the period.** `POST /billing-periods` → `EnsurePeriod`: period_start/period_end
are the first/last calendar day of the month **in the teacher's IANA timezone**, then re-expressed
at UTC midnight. Idempotent via the unique index, not a pre-check: `ErrDuplicatePeriod` re-reads
the existing row and returns it — never a 409.

**Step 3 — compute (pure read).** `ComputePeriod` (`billing/preview.go:35-164`) is shared by
Preview / Draft / Close. After the initial authorisation read it switches to
**`periodScope = {period.TeacherID, period.CenterID}`** — an owner computing a member's period must
not aggregate the whole center. It then: tallies attendance; finds `PreviousClosedPeriod`
(nil+nil = the student's first-ever cycle, opening legitimately 0); reads `CarriedDebtStudents`
and `OpeningBalances` where `opening = max(0, prev.total_due − prev.paid_amount)` of the non-void
previous invoice; reads existing invoices and adjustment totals. A **zeroing guard** force-injects
any student who already has an invoice this period but no tally/opening/adjustment with value 0,
so the persist loop reaches and zeroes their stale lines.

**Step 4 — the formula.** `Compute` (`billing/calculator.go:87-173`), pure:
- one `ComputedLine` per enrollment, `Amount = billable_count × unit_price`
- `CurrentCharge = Σ lines.Amount`
- `TotalDue = OpeningBalance + CurrentCharge + AdjustmentTotal`

Both mirror DB CHECK constraints (`amount = billable_count * unit_price`;
`total_due = opening_balance + current_charge + adjustment_total`). No proration, no fractional
per-session pricing. Line order: class start date, then class name. Extra students (carried-debt
only, or adjustment only) get an invoice with empty lines, iterated in sorted-UUID order for
determinism.

**Step 5 — persist (draft).** `DraftPeriod` must run inside a tx and **re-reads** invoices and
adjustment totals inside it rather than trusting the pre-tx snapshot. Fails fast with
`ErrInvoiceNotDraft` (→409) if any targeted invoice has moved past `draft`. Then per invoice:
`UpsertInvoice` on `(period_id, student_id)`; `UpsertInvoiceLine` on `(invoice_id, enrollment_id)`;
`ZeroUnmatchedLines` sets `billable_count=absent_count=amount=0` on lines whose enrollment left the
keep-set — **rows are never deleted**, so an invoice always reconciles against its own detail.

**Step 6 — close.** `POST /billing-periods/:id/close`, one transaction (`billing/close.go:124-230`):
`today` and `closedAt` are resolved **once before** the tx (nothing inside calls `time.Now`), then
1. `LockPeriod` (`SELECT … FOR UPDATE`), 409 unless `open`
2. **Hard gate**: `PendingSource.ListUnconfirmedInWindow(periodScope, from=period_start,
   to=min(period_end, today), before=today, limit=1000)`. The decision reads `resp.Total`,
   **not `len(resp.Items)`**, because sessions clamps the limit. Non-zero → `*ErrUnconfirmedSessions`
   returned unwrapped (deliberately **not** an `*apperror.AppError`, because `apperror.Fields` is a
   flat `map[string]string` and cannot carry the list) → handler emits 409 + `error.details`
3. `ComputePeriod` → 4. `DraftPeriod`
5. `VoidInvoices` — bulk-void every draft invoice with all three components zero. **Must precede**
   issuing, whose blanket `WHERE status='draft'` would otherwise issue them
6. `IssueDraftInvoices` — bulk `draft → issued`
7. `ClosePeriod` guarded on `WHERE status='open'`; a row count ≠ 1 is an internal invariant error
8. informational (non-blocking) warnings about unconfirmed sessions still ahead in the period

`POST /invoices/:id/void` works post-close from `issued`/`partially_paid` only, and **refuses when
`paid_amount != 0`** ("reverse the payment first").

**Step 7 — adjustments and carried debt.** Table `invoice_adjustments` (signed `amount`, mandatory
`reason`, nullable `source_session_id`). Two entry points:
- **Manual** `POST /invoices/:id/adjustments`: amount ≠ 0, reason 3–500 runes, refused on `void`
  and on `paid`. One tx of `CreateAdjustment` + `RecalcInvoiceTotals`.
- **Post-close reconciliation** `ReconcileSession`, triggered by `attendance.Confirm` through the
  `SetReconciler` seam. Finds the **closed** period containing the session date (no-op if none),
  sorts affected student ids **before locking** to avoid deadlock between concurrent
  reconciliations, and per student: `LockInvoice` FOR UPDATE **before** reading the
  already-adjusted total (otherwise two edits both read 0 and double-bill the parent), recomputes
  `liveCharge` from `LiveBillableCounts`, then
  `delta = liveCharge − invoice.CurrentCharge − alreadyAdjusted`. When `delta != 0` the row lands
  on the **next open period's** invoice for that student — `NextOpenPeriod(after = period_end)`,
  else the current calendar month in the teacher's tz (rolled forward if it is not strictly after
  the closed period, and once more if the ensured period turns out closed) — creating a draft
  invoice seeded from `StudentSnapshot` when the student has none. Then `RecalcInvoiceTotals`.
  **The closed invoice is never mutated.**

`RecalcInvoiceTotals` is one atomic `UPDATE … FROM`: `adjustment_total = SUM(non-deleted
adjustments)`, `total_due = opening + current_charge + that`, plus a status CASE that leaves
`draft`/`void` untouched.

**Step 8 — payments and allocation.** Debt is per **student**, money arrives per **contact**;
`payment_allocations` is the bridge. `Allocate` (`payments/allocator.go:60-112`) is pure — no I/O,
no clock, no errors. Candidate ordering: `period_start` asc → `earliest_class_start` asc
(**nil last**, so pure carry-over invoices sort last within a period) → `invoice_id` asc.
It then makes **two passes over that same order**: pass 1 takes `min(remaining, opening_unpaid)`
across *all* candidates, pass 2 takes `min(remaining, rest_unpaid)`. I.e. oldest-debt-first, but
the **carried opening-balance portion of every invoice is settled before any current charge on any
invoice** — not a plain per-invoice FIFO. Both passes accumulate into a per-invoice tally so an
invoice touched twice yields exactly one row (required by `uq_payment_allocations`).

The candidate query is `FOR UPDATE OF i` with **`ORDER BY i.id`** — deliberately the lock
acquisition order, not the allocation order (the Go comparator re-sorts), so every contact-scoped
write path locks in the same total order and cannot deadlock.

**Overpayment**: no credit ledger and no negative invoice. Surplus stays unallocated, surfaced as
`unallocated_amount` on every read and aggregated as `unallocated_credit` in the collections
summary. It can be placed later via `POST /payments/:id/allocations/auto`, which re-runs `Allocate`
over the remainder and merges into existing rows via `ON CONFLICT … amount = existing + excluded`.

**Reversal** is counter-entry ledger style, never a delete: `POST /payments/:id/reverse` writes a
**new `payments` row** (`reverses_payment_id = original.ID`, `received_on = today`, note = reason)
plus a **mirror of the original's allocations** pointing at it, and stamps the original's
`reversed_at`. A reversal cannot itself be reversed; the fix for a wrong reversal is recording a
fresh payment (explicit YAGNI — no partial reversal in V1). The sign convention lives in SQL:
`paid_amount = Σ CASE WHEN p.reverses_payment_id IS NULL THEN pa.amount ELSE -pa.amount END` —
re-derived, never incremented, so re-running is a no-op and it cannot drift.
`PUT /payments/:id/allocations` (manual reallocate) is the one place allocation rows are deleted,
justified because an allocation is a link between two preserved facts, not a fact itself.

**Step 9 — collections reporting.** Owns **no table**. Projects `invoices`, `invoice_lines`,
`payments`, `payment_allocations` and the `v_contact_balance` view. Status derivation is
`due − paid <= 0 → paid`; `paid == 0 → unpaid`; else `partial` — **only three statuses, there is no
`overdue`** (no due-date concept exists anywhere). The rule is implemented once in Go
(`derivePaymentStatus`) and once as a SQL CASE fragment (`paymentStatusExpr`) for the status
filter; the fragment only ever interpolates non-user-controlled column names. The contact view
**deliberately omits `c.deleted_at IS NULL`** — an archived contact who still owes must keep
appearing, flagged `contact_archived` instead — and scans `DeletedAt` as `*time.Time` rather than
`gorm.DeletedAt` precisely so GORM does not inject its own filter. Child invoices are nested via
one batched query (N+1 guard). The summary's `unallocated_credit` restricts to
`p.reversed_at IS NULL AND p.reverses_payment_id IS NULL` (the same double-count guard).

**Step 10 — statements.** See §8.3.

**Invariants that repeat across the money path**
- **Scope inheritance**: every written row inherits the *owning entity's* `teacher_id`/`center_id`,
  never the acting caller's (`periodScope`, `ownerScope`, `paymentScope`). An owner acting on a
  member's data must not reassign it, and a date-range query with no id anchor under an owner's raw
  scope would widen to the entire center.
- **Deterministic lock ordering** everywhere: invoice-id ascending in payments, sorted student ids
  in `ReconcileSession`.
- **No hard or soft deletes on financial rows**: `invoices`, `invoice_lines`, `payments`,
  `payment_allocations` have neither `deleted_at` nor `gorm.DeletedAt`. Void uses `status='void'`;
  a bad payment gets a counter-entry; a bad line gets zeroed; a bad adjustment gets an
  opposite-signed row.
- Every guarded UPDATE that touches the wrong row count becomes a 409 or an internal invariant
  error, never a silent success.

**Sentinels** — billing: `ErrPeriodNotFound`, `ErrDuplicatePeriod`, `ErrTeacherNotFound`,
`ErrInvoiceNotDraft`, `ErrInvoiceNotFound`, `ErrSessionNotFound`, `ErrStudentNotFound`, plus the
struct error `*ErrUnconfirmedSessions`. payments: `ErrPaymentNotFound` only. collections: none.

---

## 6. Data model

Schema is owned **exclusively** by golang-migrate; `gorm.AutoMigrate` is never used
(`internal/database/postgres.go:1-2`). SQL lives in `apps/api/migrations/` and is embedded via
`embed.FS` + the `iofs` source (`migrations/embed.go`), so `api migrate up|down|status` needs no
external files. Extension: `pgcrypto`. Primary keys are bare `UUID` with **no DB-side default** —
every insert supplies a **UUIDv7** from `shared/id` (time-sortable, good B-tree locality).

### 6.1 Migration history

| # | Name | What it added |
|---|---|---|
| 000001 | `baseline_schema` | 16 tables + 2 views: `user_accounts`, `teachers`, `contacts`, `students`, `classes`, `class_schedules`, `enrollments`, `class_sessions`, `attendance_records`, `billing_periods`, `invoices`, `invoice_lines`, `invoice_adjustments`, `payments`, `payment_allocations`, `statements`, `notifications`; views `v_contact_balance`, `v_unbilled_attendance` |
| 000002 | `refresh_tokens` | `refresh_tokens` (id, user_id, token_hash, family_id, expires_at, revoked_at) — deliberately outside the domain schema: an implementation detail of the chosen auth mechanism |
| 000003 | `widen_pending_sessions_index` | Rebuilds `idx_class_sessions_pending` so the partial predicate covers `status IN ('held','planned')`, not just `'held'` — a past session never explicitly marked held is exactly the warning case the baseline missed |
| 000004 | `zalo_accounts` | `zalo_accounts` — **PK is `teacher_id`** (one account per teacher made structurally impossible to violate); `encrypted_credentials BYTEA` only, no plaintext column; `consent_version` NOT NULL |
| 000005 | `zalo_personal_mapping` | `contacts.zalo_user_id` + `zalo_name`; `uq_contacts_zalo_user` partial unique; `notifications.channel` CHECK widened with `zalo_personal`; new `notification_runs` table; `notifications.run_id` with a **composite FK `(run_id, teacher_id)` ON DELETE SET NULL** (needs PG ≥ 15 for the column list) |
| 000006 | `one_running_run_per_teacher` | `uq_notification_runs_one_active ON notification_runs(teacher_id) WHERE status='running'` — the cross-process backstop the in-process guard cannot provide |
| 000007 | `centers` | **The tenancy migration.** New `centers` + `center_members`; `teachers.center_id`; `center_id` added and backfilled on **16 business tables**; `UNIQUE (id, center_id)` on parent tables; guard FKs `(teacher_id, center_id) → center_members`; child FKs re-keyed from `(x_id, teacher_id)` to `(x_id, center_id)`; old `(id, teacher_id)` uniques/FKs dropped |
| 000008 | `invitations_and_reset_tokens` | `invitations` + `password_reset_tokens`, both following the 000002 template (opaque token, sha256-hex at rest); `uq_invitations_pending_phone`, `uq_password_reset_active` |

### 6.2 Tables

| Table | Purpose | Tenancy cols | Soft delete |
|---|---|---|---|
| `user_accounts` | login identity; `role` CHECK `teachers\|parent\|students`, `status` CHECK `active\|disabled`, nullable `password_hash` (future OTP-only) | — (identity, no tenant) | ✅ |
| `teachers` | business profile; `id` **is** `user_accounts.id`; `timezone` default `Asia/Ho_Chi_Minh`; `center_id` (000007) | `center_id` | ✅ |
| `centers` | the tenant; `owner_id` NOT NULL, FK `NO ACTION DEFERRABLE INITIALLY DEFERRED` | — | ✅ |
| `center_members` | membership **history**; PK `(teacher_id, center_id)`; live = `left_at IS NULL` | both | — (`left_at`) |
| `refresh_tokens` | opaque refresh sessions with `family_id` | — | — (`revoked_at`) |
| `invitations` | owner-created onboarding tokens; `expired` derived, never stored | `center_id` | — |
| `password_reset_tokens` | single-use reset links; `used_at`/`superseded_at` | — | — |
| `contacts` | parent/guardian; `zalo_user_id`/`zalo_name` (000005) | `teacher_id`, `center_id` | ✅ |
| `students` | child; `contact_id`, `display_note`, `anonymized_at` | both | ✅ |
| `classes` | lớp; `default_unit_price`, `status active\|archived` | both | ✅ |
| `class_schedules` | weekly slots; `weekday 0–6`, `start_time TIME`, `duration_min`, `[effective_from, effective_to]` | both | ✅ |
| `enrollments` | student↔class + **frozen `unit_price`**; `started_on`/`ended_on` | both | ✅ |
| `class_sessions` | one teaching occurrence; `status planned\|held\|cancelled`, `attendance_confirmed_at` | both | ✅ |
| `attendance_records` | one row per student per session incl. present; `status`, `billable` | both | ✅ |
| `billing_periods` | month close unit; `year`/`month`, `period_start`/`period_end`, `status open\|closed` | both | ✅ |
| `invoices` | per-student debt snapshot; `opening_balance`, `current_charge`, `adjustment_total`, `total_due`, `paid_amount`, `status draft\|issued\|partially_paid\|paid\|void`, snapshot `student_name`/`contact_name` | both | ❌ **by design** |
| `invoice_lines` | one per enrollment; `billable_count`, `absent_count`, snapshot `unit_price`/`class_name`, `amount` | both | ❌ **by design** |
| `invoice_adjustments` | signed manual/reconciliation deltas; `reason` NOT NULL, `source_session_id` | both | ✅ (typo-only) |
| `payments` | contact-axis receipts; `method cash\|transfer\|other`, `reverses_payment_id`, `reversed_at` | both | ❌ **by design** |
| `payment_allocations` | payment→invoice bridge; `allocated_by auto\|manual` | both | ❌ **by design** |
| `statements` | contact+period public link; `token_hash BYTEA`, `expires_at`, `total_due`, view counters, `revoked_at` | both | ✅ |
| `notifications` | per-send audit; `channel`, `purpose`, `status queued\|sent\|delivered\|failed`, `run_id` | both | ✅ |
| `notification_runs` | one paced send pass; `status running\|completed\|interrupted\|expired`; counters **never stored** (always derived) | both | — |
| `zalo_accounts` | per-teacher linked session; PK `teacher_id`; sealed `encrypted_credentials` | `teacher_id` only (personal, not re-keyed) | ✅ |

Views: `v_contact_balance` (non-void invoices grouped by teacher/center/period/contact →
`student_count`, `total_due`, `total_paid`, `outstanding`) and `v_unbilled_attendance`
(taught + confirmed sessions not yet in any invoice line — foundation for a P1 "money leaking" board).

### 6.3 Why the four financial tables have no `deleted_at`

Documented at `migrations/000001_baseline_schema.up.sql` note (i): a soft-deleted `invoice_line`
makes the invoice total stop matching its own detail (a parent asking "how did you get this
number" cannot be answered); a soft-deleted `payment` cannot be reconciled against a bank
statement (accountants never delete an entry, they reverse it); and **one query missing
`AND deleted_at IS NULL` sends a parent the wrong amount**. Replacements: `invoices.status='void'`
+ `voided_at` + `void_reason`; `payments.reverses_payment_id` (counter-entry);
opposite-signed `invoice_adjustments`.

### 6.4 Notable indexes and composite FKs

- **Every UNIQUE on a soft-deletable table is a partial index `WHERE deleted_at IS NULL`**
  (note (j)) so delete-then-recreate is possible. Direct consequence for Go: every read query
  **must** carry `deleted_at IS NULL` — free from `gorm.DeletedAt` on model queries, manual on raw
  SQL and `Table()` queries.
- `uq_users_phone(phone) WHERE deleted_at IS NULL`
- `uq_contacts_phone(teacher_id, phone) WHERE deleted_at IS NULL` — **per teacher, not global**
- `uq_contacts_zalo_user(teacher_id, zalo_user_id) WHERE zalo_user_id IS NOT NULL AND deleted_at IS NULL`
- `uq_enrollments_active(student_id, class_id) WHERE ended_on IS NULL AND deleted_at IS NULL`
- `uq_class_sessions_per_day(class_id, session_date) WHERE deleted_at IS NULL` — the ON CONFLICT target
- `idx_class_sessions_pending(teacher_id, session_date) WHERE status IN ('held','planned') AND attendance_confirmed_at IS NULL AND deleted_at IS NULL`
- `uq_billing_periods(teacher_id, year, month)`, `uq_invoices(period_id, student_id)`,
  `uq_invoice_line(invoice_id, enrollment_id)`, `uq_payment_allocations(payment_id, invoice_id)`
- `uq_statements(contact_id, period_id)` partial + `uq_statements_token(token_hash)`
- `uq_centers_owner(owner_id) WHERE deleted_at IS NULL`, `uq_center_members_active(teacher_id) WHERE left_at IS NULL`
- `uq_notification_runs_one_active(teacher_id) WHERE status='running'`
- `uq_invitations_pending_phone(center_id, phone) WHERE status='pending'` — a second pending invite
  for a *different* center is allowed
- `uq_password_reset_active(user_id) WHERE used_at IS NULL AND superseded_at IS NULL`

**Composite FKs** are the anti-cross-tenant mechanism. Baseline used `(x_id, teacher_id) →
parent(id, teacher_id)`. 000007 flipped the tenant side: parents gained `UNIQUE (id, center_id)`,
children now reference `(x_id, center_id)`, and **every** business table additionally carries a
guard FK `(teacher_id, center_id) → center_members(teacher_id, center_id) ON DELETE CASCADE`.
That anchors rows to **membership history rather than to `teachers`**, so a teacher can leave and
their data stays in the center still attributed to them. Deliberate consequence recorded in the
migration header: cross-teacher writes *within one center* are now legal at the DB level (owners
edit on behalf of members); **teacher-vs-teacher isolation inside a center exists only at the query
layer**. Business uniques that were per-teacher stayed per-teacher (`uq_contacts_phone`,
`uq_contacts_zalo_user`, `uq_billing_periods`, `uq_notification_runs_one_active`,
`idx_class_sessions_pending`). Not re-keyed at all: `user_accounts`, `refresh_tokens` (identity,
no tenant) and `zalo_accounts` (personal, follows the person).

Deliberate design choices recorded in the baseline: money is `BIGINT` đồng, never FLOAT/DOUBLE;
states are `VARCHAR` + CHECK, never native ENUM; totals are computed in Go, **never in a trigger**
(a money-computing trigger is the hardest thing to debug when a customer reports a wrong number),
with CHECK constraints as the arithmetic backstop. Postgres RLS is recommended in note (m) but
**deferred** as a pre-launch hardening item.

---

## 7. Cross-cutting conventions

**Response envelope** (`shared/response`) — every `/api/v1` response:
`{"success":true,"data":…,"meta":{page,per_page,total,total_pages}}` /
`{"success":false,"error":{code,message,fields,details}}`. `meta` appears only via
`response.List`. `details` is populated only by `ErrWithDetails` (used for billing's
unconfirmed-sessions list, which a flat `map[string]string` cannot express). Health probes are
**outside** the envelope. Responses with status ≥ 500 log the cause through the request-scoped
logger and return a generic message.

**apperror → HTTP** (`shared/apperror`):

| Constructor | Code | Status |
|---|---|---|
| `BadRequest` | `BAD_REQUEST` | 400 |
| `Unauthorized` | `UNAUTHORIZED` | 401 |
| `Forbidden` | `FORBIDDEN` | 403 |
| `NotFound(resource)` | `NOT_FOUND` | 404 |
| `Conflict` | `CONFLICT` | 409 |
| `Invalid(msg, fields)` | `VALIDATION_ERROR` | 422 |
| `TooManyRequests` | `TOO_MANY_REQUESTS` | 429 |
| `Internal(err)` | `INTERNAL_ERROR` | 500 (cause kept for logs, hidden from clients) |

`apperror.From(err)` wraps anything unrecognised as `Internal`, so an unmapped error can never leak
detail. Services return `*AppError`; each feature keeps raw sentinels in `errors.go` and a
`translate()` that wraps them while setting `appErr.Err = <sentinel>` so `errors.Is` still works
upstream.

**Pagination** (`shared/pagination`) — `page`, `per_page` (default 20, **max 100**), `sort`
(leading `-` = DESC, columns **whitelisted per feature**, unknown keys silently fall back to the
default). `maxPage = 1_000_000` bounds the offset so it can never overflow `int` negative (which
GORM silently drops, serving page 1). Bounds are clamped, never errors. Repositories apply
`Params.Scope`; handlers return `Params.Meta(total)`. List data serialises as `[]`, never `null`.

**Validation** (`shared/validation`) — DTOs carry `binding` tags; handlers translate with
`BindError`: validator failures → 422 with a per-field map, anything else (malformed JSON, wrong
types) → 400. `init()` registers two custom validators on gin's binding engine — `vnphone`
(`^(0|\+84)(3|5|7|8|9)\d{8}$`) and `hhmm` (`^([01]\d|2[0-3]):[0-5]\d$`, because Postgres `TIME`
travels as a string, never a `time.Time` that would drag a date and zone along) — plus a
`RegisterTagNameFunc` so error keys are the request JSON names (`full_name`), not Go field names.
`NormalizePhone` converts `0xxxxxxxxx → +84xxxxxxxxx`; storage and lookups are always E.164 so both
spellings resolve to one account. A recurring DTO idiom: numeric fields that may legitimately be 0
(`weekday`, `default_unit_price`) are declared as **pointers** so `binding:"required"` does not
reject zero.

**Configuration** — `internal/config`, prefix **`API_`**, parsed by `caarlos0/env/v11`, validated at
startup. `os.Getenv`/`os.LookupEnv` outside that package is a **lint error**
(`forbidigo`, `apps/api/.golangci.yml:15-18`, with `internal/config/` the sole exclusion). In
development (or unset `API_ENV`) a `.env` is loaded from `.` or `../../` — test and production read
the process environment only, so tests stay hermetic. Validation refuses: an `API_ENV` outside
`development|test|production`; `API_JWT_SECRET` under 32 chars; a bad log level; `*` in
`API_CORS_ORIGINS` (incompatible with credentialed requests) or an origin without an
`http(s)://` scheme; non-positive onboarding TTLs; pacing values that defeat the anti-ban
guardrail. Two secrets (`API_STATEMENTS_TOKEN_KEY`, `API_ZALO_CRED_KEY`) accept hex **or** base64
**or** raw bytes, require ≥32 decoded bytes, are **fatal in production**, and outside production
fall back to a random per-process key logging only an 8-hex-char SHA-256 **fingerprint**, never the
key. Rotating either is destructive by design (statement links already sent out die; every linked
Zalo account becomes undecryptable), which is exactly why production refuses a substitute.

**Transactions** — `database.TxManager` interface with `GormTxManager` implementation. `WithinTx`
stores the `*gorm.DB` in the context under a private key; repositories resolve it with
`database.FromContext(ctx, fallback)`, so identical repository methods work inside and outside a
transaction, and **nested `WithinTx` calls join the ambient transaction** rather than opening a
second one. Services own transaction boundaries. This is what lets `attendance.Confirm` call
`sessions.MarkHeldAndConfirmed` and have it commit atomically with the attendance records, and lets
`create-center` span two services.

**Tenancy** — the tenant is the **center** (000007). Rules applied without exception:
- Handlers learn the tenant **only** from `authctx.ScopeFrom(c)` — never a body, query string, or
  path segment. A client-supplied `center_id`/`teacher_id` is an authorisation bypass that looks
  completely ordinary in a diff.
- `Scope{TeacherID, CenterID, IsOwner}` is resolved fresh from the database on every request by
  `middleware.ResolveScope` and **never cached in the JWT**, so a kick/leave/join takes effect on
  the very next request rather than at token expiry.
- Every repository over a tenant table funnels reads through a `scoped(ctx, sc)` helper: always
  `WHERE <table>.center_id = sc.CenterID`, plus `AND <table>.teacher_id = sc.TeacherID` when
  `!sc.IsOwner`. Present in **11** feature repositories (attendance, billing, classes, contacts,
  enrollments, notifications, payments, sessions, statements, students, teachers); reference
  implementation `features/students/repository.go`.
- Writes keep `teacher_id = $self` for a plain member; owners may write on behalf of any teacher in
  their center. Child rows inherit the **parent's** anchors, top-level creates are stamped as the
  caller, and FK-existence checks on create run with owner rights **stripped**
  (`ownScope := authctx.Scope{TeacherID, CenterID}`).
- Three places deliberately strip `IsOwner` even for reads/writes an owner could otherwise do:
  `notifications` run writes (`runsOwnScoped` — an owner must never drive a member's run through
  the owner's own Zalo account), `ResumeRun`, and the create-time reference checks above.

**Auth token model** — summarised in §4.2 (auth). Accepted trade-off recorded in
`docs/api-guidelines.md`: access tokens are stateless, so after logout / soft-delete / disable an
already-issued access token stays valid for up to its 15-minute TTL; only refresh is revoked
immediately. `GET/PUT /me` and `ResolveScope` both re-check the account against the database, so
the practical blast radius is small. A denylist is deferred until instant revocation is a product
requirement. Invitation and reset tokens share the `shared/token` primitive with refresh tokens:
256-bit random plaintext handed out once, only the sha256-hex digest at rest, single-use, and
always carried in the **request body** so they never land in an access log.

---

## 8. Zalo integration

### 8.1 `features/zalo` — the service layer

Links a teacher's **personal** Zalo account so Teka can act as them: send DMs, list friends,
resolve phone→account, send friend requests. All routes are `/api/v1/me/zalo/*`; the teacher id
comes only from `authctx.ScopeFrom`, never a path segment.

**Link flow.** `POST /link/start` requires a non-empty `consent_version` (linking hands a third
party's session to this system, so a linked row must always be backed by the exact consent text
acknowledged, with `consent_at`). `LinkManager.Begin` returns a link UUID immediately (202) and
spawns one goroutine per attempt; the client polls `GET /link/status?id=`. States:
`pending → qr_ready → scanned → confirmed → linked`, plus `expired` and `error`. The QR PNG is
returned base64 and nulled once the attempt resolves. Attempt TTL 105s (QR is valid ~100s),
finished-record retention 2m, pruned lazily inside `sweepLocked` on every `Begin`/`Status` so no
third goroutine exists. **Failures never echo upstream text** — a fixed
`"could not complete the Zalo login"` is returned and the detail only logged. On success the
goroutine checks it has not been superseded, then persists on `context.WithoutCancel` + a 10s
timeout so a completed scan is never lost to the attempt deadline.

**Credential storage.** Table `zalo_accounts`, PK `teacher_id`. `persistLink` marshals
`protocol.Credentials` (IMEI, cookie jar, user agent, language) → `cipher.Seal` → then performs a
**second login** on a fresh session, because the QR session never fetched the service map and
cannot send or list. Status `linked|expired`; delete is soft and `Upsert` clears `deleted_at` so a
re-link revives the row rather than colliding on the PK. An undecryptable blob is treated as
`ErrLinkExpired` and the account is marked expired.

**Encryption at rest** — `shared/secrets`, a thin **AES-256-GCM** envelope. `MinKeyLen = 32`. The
AES key is **`SHA-256(suppliedBytes)`, not the bytes themselves**, so hex / base64 / passphrase
shapes all work uniformly, the key is stable across restarts, and a longer key keeps its entropy
(unlike truncation). `Seal` → `nonce ‖ ciphertext ‖ tag` with a fresh 12-byte `crypto/rand` nonce
on every call. `Open` returns a fixed error string that never carries the ciphertext; there is no
partial or best-effort decryption. Constructed **before** the database opens, so a bad key is a
refuse-to-start. Its only consumer is `zalo.Service`.

**Health probe.** `StartHealthProbe(ctx, ProbeOptions{Interval, Jitter})` derives its own
cancellable context and spawns exactly **one** goroutine (a second call is a no-op). Default
interval **15 minutes**, jitter = interval/3 drawn from **`crypto/rand`** so sweeps never settle
into a fixed pattern — verification costs a real login against Zalo, and frequent programmatic
logins are what makes an account look automated. A sweep lists accounts with `status='linked'` and
calls `VerifyAccount` **sequentially** — never a goroutine per account, which would turn a roster
into a burst of simultaneous logins. `VerifyAccount` evicts the session cache first, forcing a real
relogin. Writes: success → `last_verified_at = now()` plus a revival to `linked` if it was
`expired`; rejection → cache evict + `status='expired'`. Purpose: the profile page can say
"reconnect needed" before a teacher discovers it by having a send fail.

**Close semantics.** `Service.Close()` = `stopProbe()` + `links.Close()`, safe twice. `stopProbe`
cancels the service's own derived context and blocks on `<-done`, so shutdown does not depend on
the caller cancelling. `LinkManager.Close` cancels the base context, snapshots **all** live records
(including superseded ones no longer in `active`), and waits on each `done` channel.

**Concurrency primitives.** `LinkManager`: `sync.Mutex` over `active map[teacherID]*linkRecord` and
`live map[teacherID]map[*linkRecord]struct{}`; per-record `CancelFunc` + `done` channel closed in
`retire()` as the goroutine's last act; `update()` mutates only if the record is still current, so a
superseded goroutine cannot overwrite its replacement. `SessionCache`: `sync.RWMutex` plus a
per-teacher **eviction counter** read before a slow restore and passed to
`PutUnlessEvicted(…, since)`, so a login that an unlink overtook is discarded and never cached.
`Service`: `probeMu` + `probeStop` + `probeDone`.

**Friend matching.** `MaxMatchPhones = 200` per request, `matchChunkSize = 30` per upstream call,
with a 1–3s randomised pace between chunks. Phones are normalised against
`^0(3|5|7|8|9)\d{8}$` then converted to `84…` for the wire (Zalo's lookup only resolves
country-code form); non-matching input never reaches Zalo. Nothing is persisted and **phones are
never logged** (counts and durations only). `LookupPhone(ctx, teacherID, phone) (uid, ok, err)` is a
thin single-phone adapter returning `ok=false` unless matched **and** already a friend — this is
the method `auth.ResetDMSender` and `invitations.ZaloSender` consume.

**Session death.** `expireIfLoggedOut` converts `protocol.APIError{Code: -3}` into `ErrLinkExpired`
and expires the account on every send/friends/lookup/friend-request path. Handler mapping:
`ErrLinkNotFound` → 404, `ErrNotLinked` → 404, `ErrLinkExpired` → 409,
`ErrConsentVersionRequired` → 400. Another teacher's link id is a 404, never a 403.

### 8.2 `features/zalo/protocol` — the wire protocol port

A **reverse-engineered reimplementation of Zalo's personal-account web protocol**, ported from
`zcago` (MIT) and partly `goclaw`. Quarantined: it imports nothing from Teka. There is **no
WebSocket listener** (`zpw_ws` is decoded but unused), and groups/media are deliberately absent.
`.golangci.yml:29-40` grants it two scoped exclusions — `gosec` G401/G501 (MD5 is dictated by Zalo
for IMEI, request signing, and key derivation, so it cannot be substituted) and `revive exported:`
(the wire structs stay comment-free so re-porting upstream changes stays a diff, not a rewrite).

| File | Role |
|---|---|
| `doc.go` | scope, upstream attribution, "credentials are account-takeover material" warning |
| `config.go` | wire constants (UA, api type/version, `DefaultZCIDKey`, base URL), `Credentials`, cookie types with array-or-object custom JSON, `BuildCookieJar` |
| `crypto.go` | `EncodeAESCBC`/`DecodeAESCBC` — **AES-CBC with an all-zero IV** (documented Zalo quirk), PKCS#7; rejects non-block-multiple ciphertext because `CryptBlocks` panics on partial blocks from server-controlled input |
| `client.go` | `Session`, `GenerateIMEI` (`uuid + "-" + md5(userAgent)`), signed/encrypted request helpers, ZCID + per-request key derivation, service-map URL resolution. `doRequest` **strips the query string out of transport errors** because URLs carry the IMEI in clear |
| `models.go` | wire structs incl. a custom unmarshal for Zalo's own typo `"setttings"` |
| `auth.go` | `LoginWithCredentials` (concurrent `getLoginInfo` + `getServerInfo` via `errgroup`, then re-seeds the cookie jar per service-map host because Go's jar does not propagate across subdomains) and `LoginQR` (scrape → generate → **long-poll** waiting-scan / waiting-confirm, 100s timeout → checksession → userinfo) |
| `send.go` | `SendMessage` — DM only, encrypted params, returns `msgId` |
| `contacts.go` | `FetchFriends`, `FindUser` (batched phone lookup), `SendFriendRequest` |
| `errors.go` | `ErrCodeNotLoggedIn = -3`; `APIError{Op, Code}` deliberately carries no Zalo error text |

All six protocol tests are `httptest`-based; no network.

### 8.3 Statements → notifications → Zalo

**A statement** is one contact's billing summary for one **closed** period plus a tokenised public
link. `Generate` requires `period.status == 'closed'` (409 otherwise), TTL 90 days, and upserts on
`(contact_id, period_id)` updating **only `total_due`** — so the token and URL never change under a
parent who already received them — with `WHERE revoked_at IS NULL` so a revoked statement is
skipped, never resurrected. `center_id` is inherited from the **period's** owning teacher, never the
caller's, so a forged token cannot cross tenancy.

**Token**: `deriveToken(key, statementID) = base64url(HMAC-SHA256(key, statementID[:16 raw bytes]))`,
keyed on `cfg.Statements.TokenKey`. At rest only `SHA-256(token)[:32]` is stored in
`statements.token_hash`; the plaintext is never stored and is only recomputable by a holder of the
key — hence `ToResponse` recomputes it per response and must only ever leave teacher-authenticated
paths. URL = `PublicBaseURL + "/s/" + token`.

**Public page**: root-engine routes with `securityHeaders()` (see §2.4). One neutral 404 for every
failure cause, including **already fully paid** — `RenderPublic` returns not-found when
`Totals.Outstanding <= 0`, so a statement dies the moment it is paid, independently of
`expires_at`. `TouchView` runs *after* the response is written and only logs its error;
`qr.png` deliberately does **not** count a view and serves `private, max-age=300`.

**Message rendering** (`statements/message.go`): `Build(in, maxLen)` renders full, and if it exceeds
`maxLen` re-renders with per-child lines collapsed into one `"N bạn, tổng M buổi học"` summary,
returning `collapsed=true`. Greeting, `Nợ cũ` (only when non-zero), `Điều chỉnh` (only when
non-zero, signed), `Tổng cộng`, and the URL are unchanged — **the URL is always the last line and is
never dropped**. Money formats with `.` thousands separators and a trailing ` đ`. Golden files live
in `statements/testdata/`.

**VietQR** (`statements/qr.go`): `BankConfig{BankCode, AccountNumber, AccountName}` is read from app
config because V1 has no per-teacher bank column; **every field is optional and an unconfigured
account is a supported state — the QR block is omitted, never faked**. `emvQRBuilder` builds an
EMVCo TLV payload (NAPAS AID `A000000727`, service `QRIBFTTA`, VND `704`, note under sub-tag 08)
terminated by **CRC-16/CCITT-FALSE** as 4 uppercase hex digits. The note is clamped to 25 runes
because EMVCo lengths are two ASCII digits and a long contact name would emit a 3-digit length and
corrupt the payload. Rendered by `skip2/go-qrcode` at medium recovery, 256px.

**Notifications** carry **no message text at rest** — no `message_text` column and no `contact_id`;
the contact is reached through `statement_id → statements.contact_id`. Channels: `zalo_manual`
(default; render text, queue the row, hand it back for the teacher to paste, then
`POST /notifications/mark-sent`), `zalo_personal` (automatic paced DM through the teacher's own
session), plus `zalo_zns` (returns `ErrNotConfigured`) and `sms` (deliberately absent from the
sender registry → 400). Both wired senders are **no-ops inside the transaction** — a real Zalo send
must never happen inside `BulkSend`'s tx; delivery is post-commit via the `RunManager`.

`BulkSend` in personal mode splits targets by `contacts.zalo_user_id`: mapped contacts become run
items, **unmapped contacts fall back to `zalo_manual`** and are counted in `FallbackManualCount`.
Personal mode refuses another teacher's period with 409, checked inside the tx so the rollback also
undoes the statement refresh.

**Runs.** `notification_runs` = one paced pass. Progress counters are **never stored** — always
derived by `COUNT` over `notifications.run_id`, so there is no second source of truth to drift.
`RunManager` uses a two-phase start: `Reserve(teacherID, centerID)` claims the in-memory slot
**before** the DB transaction (`ErrRunBusy` otherwise), then either `Start` (spawns the goroutine)
or `Release`; `Start` launches the goroutine even during shutdown so it observes the cancelled
context and closes `done`, never leaving `Close` waiting. The send loop paces
`paceMin + rand(paceMax − paceMin)` seconds (defaults 3–8s, `MaxRunSize` 50 enforced inside the tx)
purely as an anti-ban guardrail — Zalo publishes no limits, which is exactly why it is configurable.
A **circuit breaker** at 3 consecutive failures marks the run `interrupted` with remaining rows left
`queued`. `zalo.ErrLinkExpired`/`ErrNotLinked` marks the run `expired` and fails the remaining
rows. **All outcome writes ride `context.WithoutCancel` + a 10s timeout**, so a send that already
reached Zalo is recorded even during shutdown — otherwise a resume would double-send.

**Interrupt / resume.** `Close` cancels and awaits each run; rows never reached stay `queued` and
the run stays `running`. At the next boot `ReconcileInterrupted` runs **before the router is built**
and flips every `running` row to `interrupted` — deliberately untenanted, since there is no
requesting teacher at boot, and safe because a run can only be `running` while a goroutine in *this*
process drives it. `POST …/run/resume` accepts only `interrupted`, **strips `IsOwner`** so an owner
can never resume a member's run through the owner's own Zalo account, re-generates statements and
**re-renders** every still-queued row from live data (texts live only in memory), and fails rows
that can no longer be sent with fixed Vietnamese reasons (`Chưa gán bạn Zalo`, `Đã thanh toán đủ`,
`Không còn dữ liệu để gửi`).

**One running run per teacher** is defended three times: the in-memory `active` map, a
`HasActiveRun` pre-check under `runsOwnScoped`, and the DB partial unique index
`uq_notification_runs_one_active` — the cross-process backstop for an overlapping deploy or
accidental scale-out, where two passes would DM the same parents twice from the teacher's personal
account.

---

## 9. Testing layers

Three-layer pyramid, each with a distinct wiring style:

| Layer | Files | Doubles | Proves |
|---|---|---|---|
| Unit | in-package `service_test.go` etc. | hand-written fakes (`fakeRepository`, `fakeAccountService`, `noopTxManager`) | business rules: token rotation, role checks, validation, error mapping |
| HTTP | in-package `handler_test.go` | same fakes behind a **real router slice** (real Gin routes, middleware, JWT parsing, envelope encoding) | status codes, envelope shape, auth/role gating, cookie flags |
| Integration | `integration_test.go`, `//go:build integration`, external `_test` package | none — real PostgreSQL via testcontainers, real migrations | SQL correctness: partial unique indexes across soft delete, tenant isolation, pagination, transaction rollback |

**Build tags**: exactly one tag in the codebase — `//go:build integration`, on **26** files.
Integration tests are excluded two ways: without `-tags=integration` they do not compile at all, and
under `-tags=integration -short` they self-skip via `testing.Short()` **before touching Docker**
(`testutil/postgres.go:26-28`). Integration tests live in external `_test` packages because
`testutil` imports the feature packages; HTTP tests stay in-package to reuse the unexported fakes.

**Testcontainers** — `testutil.StartPostgres(t)` launches a `postgres:16-alpine` container **per
test** (fully isolated and parallel-safe), applies the embedded migrations, opens a GORM pool
(5/2/1min), and terminates via `t.Cleanup`.

**Fixtures** (`testutil/fixtures.go`, 518 LOC, functional-options style): `Teacher` (inserts the
personal `centers` row + `user_accounts`/`teachers` pair + live `center_members` row in one
transaction, because the center's owner FK is deferred until the teachers row exists; unique random
`+84` phone; **`bcrypt.MinCost`** so tests stay fast), plus `Contact`, `Student`, `Class`,
`Schedule`, `Enrollment`, `Session`, `AttendanceRecord`, `ScopeFor`, `JoinCenter`. Shared constants
`DefaultPassword` and `JWTSecret` let tests mint tokens against the same key the service verifies
with. Production code never imports `testutil`.

**Coverage floor: 60 %** (`API_COVERAGE_FLOOR` in the root `Makefile`), measured with
`-coverpkg=./...` across the whole module but run only over test-bearing packages (a `go list`
filter) — because auto-downloaded Go toolchains lack the `covdata` tool `go test` would need to
synthesise empty profiles for test-less packages.

**Commands**: `make test-api-unit` (`go test -short ./...`, no Docker) · `make test-api`
(everything + the coverage gate, needs Docker) · `make coverage-api` (HTML report) ·
`make lint-api` (golangci-lint) · `make api-docs` (regenerate OpenAPI).

**Lint** (`apps/api/.golangci.yml`): `govet`, `errcheck`, `staticcheck`, `revive`, `gosec`,
`ineffassign`, `misspell`, `forbidigo`; formatters `gofmt` + `gci` with sections
`standard / default / localmodule`. Exclusions: `gosec` off in `_test.go`; `forbidigo` off in
`internal/config/`; the two zalo-protocol carve-outs described in §8.2.

**Seeds** (`apps/api/seeds/seed.go`, 670 LOC) — idempotent: teachers keyed by phone, roster data
gated on the owning teacher having none yet, existing rows never modified, so reseeding a database
with real data is safe. The first seed teacher **owns** the center and every following teacher joins
that same center as a member, because invite-only onboarding has no self-registration or
join-by-phone path any more. bcrypt cost 12. Demo roster is shaped for the UI: one contact with two
children (so the attendance-sheet disambiguation note has data) and another with two children in
the same class (so a public statement can show a multi-child family still unpaid after the first is
settled).

---

## 10. Statistics

Non-test Go LOC excluding the generated OpenAPI spec: **26 560**. Test LOC: **28 673**
(ratio ≈ 1.08:1). 257 `.go` files total.

### Features

| Feature | Src LOC | Test LOC | Src files | Test files |
|---|---:|---:|---:|---:|
| zalo (incl. `protocol/`) | 3 659 | 4 246 | 19 | 11 |
| billing | 3 149 | 2 955 | 10 | 6 |
| statements | 1 906 | 2 273 | 12 | 8 |
| notifications | 1 838 | 2 025 | 8 | 6 |
| payments | 1 642 | 1 855 | 9 | 4 |
| centers | 1 580 | 910 | 7 | 1 |
| sessions | 1 399 | 2 272 | 9 | 5 |
| classes | 1 108 | 1 189 | 7 | 3 |
| auth | 998 | 1 452 | 7 | 3 |
| invitations | 956 | 1 441 | 7 | 3 |
| enrollments | 814 | 1 280 | 7 | 3 |
| attendance | 793 | 1 165 | 7 | 3 |
| contacts | 780 | 1 184 | 7 | 3 |
| collections | 777 | 507 | 6 | 1 |
| teachers | 697 | 742 | 7 | 4 |
| students | 691 | 985 | 7 | 4 |
| **features total** | **22 787** | **26 481** | **136** | **68** |

### Infrastructure

| Package | Src LOC | Test LOC | Src files | Test files |
|---|---:|---:|---:|---:|
| `seeds` | 670 | 35 | 1 | 1 |
| `internal/shared` (9 pkgs) | 625 | 280 | 9 | 4 |
| `internal/cli` | 548 | 396 | 7 | 4 |
| `internal/testutil` | 518 | 0 | 2 | 0 |
| `internal/middleware` | 401 | 224 | 7 | 2 |
| `internal/config` | 361 | 354 | 1 | 1 |
| `internal/server` | 283 | 156 | 3 | 1 |
| `internal/database` | 176 | 0 | 4 | 0 |
| `internal/app` | 158 | 0 | 2 | 0 |
| `cmd/api` | 23 | 0 | 1 | 0 |
| `migrations` (Go) | 10 | 747 | 1 | 1 |
| `docs` (generated) | 10 627 | 0 | 1 | 0 |

**Migrations SQL**: 8 up + 8 down files. `000001_baseline_schema.up.sql` 33 KB (the largest),
`000007_centers.up.sql` 24 KB. `migrations_test.go` is 30 KB of integration assertions over the
applied schema.

**Route surface**: 94 registrations — 5 on the root engine (all unauthenticated), 89 under
`/api/v1`. Of those 89, **7 do not use `RequireAuth`**: `login` (fully public), `refresh` and
`logout` (authenticated by the refresh cookie), and the 4 rate-limited public routes
(`forgot-password`, `reset-password`, `invitations/preview`, `invitations/accept`). The remaining
82 are `RequireAuth` + `ResolveScope`; **10** of those additionally require `scope.IsOwner`
(`PATCH /centers/me`, `DELETE /centers/me/members/:teacherId`, the 5 dashboard routes, and the 3
invitation-management routes). `GET /centers/me` is not owner-gated but degrades its payload for
members (roster is owner-only data).

---

## Unresolved questions / flags for `docs-manager`

Behaviour-affecting, ordered roughly by significance. None of these were changed — this is a
read-only scout.

1. **Status-rule divergence.** `billing.deriveInvoiceStatus` treats `paid_amount >= total_due` as
   `paid` with no `total_due > 0` guard; `payments.recalcInvoicePaidQuery` requires
   `paid >= total_due AND total_due > 0`. For a `total_due = 0` issued invoice the two disagree
   (billing → `paid`, payments → `issued`), yet both packages' comments claim they can never
   disagree about what "paid" means.
2. **Enumeration oracle on invitation accept.** `invitations/service.go:304-307` returns
   `CreateInCenter`'s `apperror.Conflict("phone already registered")` raw, escaping the uniform
   `errAcceptRejected` 400 that every other rejection collapses to.
3. **Byte-vs-rune message ceiling.** `statements.Build` compares `len(full)` in **bytes** against
   `NOTIFICATIONS_MAX_MESSAGE_LEN`, documented as a "character ceiling". Vietnamese is multibyte, so
   messages collapse earlier than the config name implies. Possibly intentional (Zalo limits are
   byte-ish); not stated anywhere.
4. **`GET /classes/:id/sessions` writes.** Session materialisation happens on a GET. Intentional and
   documented, but the endpoint is not idempotent-in-effect and needs write DB access;
   `ListRangeReadOnly` exists but only the centers dashboard uses it.
5. **Timezone inconsistency.** `enrollments.today()` uses server-local `time.Now()` normalised to
   UTC midnight, whereas `sessions.ListPending` and `billing.Close` deliberately use the teacher's
   IANA timezone. A default `started_on`/`ended_on` can land on a different calendar day than a
   session generated at the same moment.
6. **Synchronous best-effort DMs.** `invitations.attemptDM` and `auth.attemptResetDM` run inline in
   the request (10s timeout, no goroutine or queue), so a slow Zalo endpoint directly inflates p99
   on `POST /centers/me/invitations` and `POST /auth/forgot-password`.
7. **Single-replica assumptions.** `zalo.SessionCache` is explicitly single-replica; the DB partial
   unique index backstops notification runs across replicas, but Zalo sessions would be duplicated
   and re-logged-in per replica.
8. **VietQR sub-tag 01** carries `cfg.BankCode` where NAPAS expects a numeric BIN (acknowledged in
   `statements/qr.go:53-59` and `adr.md`; no BIN table in V1). Wallets may fail to resolve the bank.
9. **`protocol.DefaultZCIDKey`** is a hardcoded shared secret committed to the repo (dictated by the
   Zalo protocol; `client.go` scrubs URLs from errors partly because of it).
10. **Stale doc comment**: `centers/model.go:15-17` still describes the removed self-registration /
    personal-center model. Actual bootstrap is CLI-only.
11. **Missing validation**: `classes` has neither a service check nor a DB CHECK for
    `end_date >= start_date` (unlike `enrollments`, which has both).
12. **Dead-ish surface**: `middleware.RequireRole` is mounted on no route; `excused` attendance and
    `billable=false` are never written in V1; `contacts.user_id` and `students.user_id` are modelled
    but never populated; `zalo_zns`/`sms` channels exist in the CHECK but have no working sender.
13. **Deferred hardening** explicitly recorded in the schema: Postgres RLS on every `teacher_id`
    table (note m), and the personal-data deletion job (note q) whose retention policy is an open
    legal question, not a technical one.
