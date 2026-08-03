---
phase: 2
title: "Phone-Based Auth Rewrite"
status: pending
priority: P1
effort: "6h"
dependencies: [1]
---

# Phase 2: Phone-Based Auth Rewrite

## Overview

Re-point the auth feature from the deleted `users` table onto `user_accounts` +
`teachers`. Login becomes phone + password. Register creates both rows in one
transaction. The JWT `sub` claim carries the account id, which is also the
`teacher_id` every downstream repository filters on.

The refresh-token rotation machinery — families, revoke-on-reuse, constant-time
failure — is preserved structurally. This is a re-pointing, not a redesign; the
existing logic in `apps/api/internal/features/auth/service.go:95-154` is the
security-critical part and must survive intact.

## Requirements

- `POST /api/v1/auth/register` accepts `{phone, password, full_name}` and
  creates one `user_accounts` row (`role = 'teachers'`, `status = 'active'`,
  bcrypt `password_hash`) and one `teachers` row with the same `id`, atomically.
- `POST /api/v1/auth/login` accepts `{phone, password}`. Wrong phone and wrong
  password are indistinguishable in status, body, and latency.
- Accounts with `status = 'disabled'` or `deleted_at IS NOT NULL` cannot log in
  or refresh.
- `POST /api/v1/auth/refresh` and `/logout` keep their current cookie-based
  contract and rotation semantics.
- Access-token claims: `sub` = account id (= teacher id), `role` = `'teachers'`.
- `authctx.Role*` constants change from `admin`/`user` to the schema's
  `teachers`/`parent`/`students` (D5: mirror `CHECK (role IN (...))`).
- Ids for `user_accounts` and `teachers` come from `internal/shared/id` (D3).
- No `AutoMigrate` anywhere (D6).

## Architecture

**Feature layout.** The auth feature keeps its shape
(`handler.go`, `dto.go`, `service.go`, `repository.go`, `model.go`,
`routes.go`, `tokens.go`) but gains ownership of the identity tables. The
existing `UserService` consumer interface at
`apps/api/internal/features/auth/service.go:18-22` is replaced by an
`AccountService` interface satisfied by the phase-3 teacher-profile service:

```go
type AccountService interface {
    CreateTeacher(ctx context.Context, req teachers.CreateRequest) (*teachers.Account, error)
    GetByPhone(ctx context.Context, phone string) (*teachers.Account, error)
    GetByID(ctx context.Context, id uuid.UUID) (*teachers.Account, error)
}
```

Phase 2 defines the interface and a minimal implementation inside the renamed
feature package; phase 3 fleshes out the profile endpoints on top of the same
service. Splitting it this way keeps the auth package free of profile CRUD.

**Data flow — register**

```
POST /auth/register {phone, password, full_name}
  -> handler: bind + validate (phone regex, password 8..72)
  -> service.Register
       -> tx.WithinTx:
            id := shared/id.New()
            INSERT user_accounts (id, role='teachers', phone, password_hash, status='active')
            INSERT teachers      (id, full_name, timezone='Asia/Ho_Chi_Minh')
            issueSession(account)  -- new refresh family + access JWT
  -> 201 {access_token, token_type, expires_in, teacher{...}} + httpOnly refresh cookie
```

The `uq_users_phone` partial unique index is the concurrency guard: two
simultaneous registrations for the same phone means one transaction fails with
a unique violation, which the repository translates to `ErrDuplicatePhone` and
the service maps to `409 Conflict`. Do not pre-check with a SELECT and treat
absence as permission to insert — that is a TOCTOU race.

**Data flow — login**

```
POST /auth/login {phone, password}
  -> service.Login
       -> GetByPhone (WHERE phone = ? AND deleted_at IS NULL)
       -> not found  -> burn a bcrypt comparison against a dummy hash, return 401
       -> status != 'active' -> burn a bcrypt comparison, return the same 401
       -> password_hash IS NULL -> same 401 (OTP-only account, not supported in V1)
       -> bcrypt mismatch -> same 401
       -> openSession + UPDATE user_accounts SET last_login_at = now()
```

The dummy-hash comparison already exists at
`apps/api/internal/features/auth/service.go:80`. Extend it to cover the
disabled and null-hash branches — otherwise response latency leaks whether a
phone is registered.

**Refresh and the disabled-account gap.** Today `Refresh` re-loads the user at
`service.go:117` only to check existence. It must additionally reject
non-active and soft-deleted accounts, otherwise disabling an account leaves its
refresh token usable for the remainder of the refresh TTL.

**Role constants.** `authctx.RoleAdmin`/`RoleUser`
(`apps/api/internal/shared/authctx/authctx.go:20-23`) are replaced by
`RoleTeacher = "teachers"`, `RoleParent = "parent"`, `RoleStudent =
"students"`, matching the `CHECK` in `user_accounts`. `Principal.IsAdmin()`
(`authctx.go:33`) has no meaning in this product and is deleted along with
`middleware.RequireRole(authctx.RoleAdmin)` usage in
`apps/api/internal/server/router.go:65`. `middleware.RequireRole` itself stays
— P1 assistant accounts (schema note (o)) will need it.

**Phone validation.** Add a `phone` validator to
`internal/shared/validation/validation.go` accepting Vietnamese mobile numbers
in either `0xxxxxxxxx` or `+84xxxxxxxxx` form and normalising to E.164 on
write. Normalisation must happen in one place (the service, before it reaches
the repository) so the unique index sees a canonical value; a number stored
both ways would break the uniqueness guarantee the whole tenancy model rests on.

## Related Code Files

**Create**

- `apps/api/internal/features/teachers/model.go` — `Account` (mirrors
  `user_accounts`) and `Teacher` (mirrors `teachers`)
- `apps/api/internal/features/teachers/repository.go` — `GetByPhone`,
  `GetByID`, `CreateAccountWithProfile`, `TouchLastLogin`
- `apps/api/internal/features/teachers/service.go` — phase 2 writes the
  auth-facing methods only; phase 3 adds profile update
- `apps/api/internal/features/teachers/errors.go` — `ErrNotFound`,
  `ErrDuplicatePhone`
- `apps/api/internal/features/teachers/repository_test.go`

**Modify**

- `apps/api/internal/features/auth/dto.go` — `RegisterRequest{Phone, Password,
  FullName}`, `LoginRequest{Phone, Password}`, `TokenResponse.Teacher`
- `apps/api/internal/features/auth/service.go` — `UserService` →
  `AccountService`; `Register` builds both rows; `Login` on phone; `Refresh`
  gains the status check
- `apps/api/internal/features/auth/handler.go` — swagger annotations updated
- `apps/api/internal/features/auth/model.go` — comment now references
  `user_accounts`
- `apps/api/internal/features/auth/service_test.go`,
  `handler_test.go`, `integration_test.go` — ported to phone
- `apps/api/internal/shared/authctx/authctx.go` — role constants, drop `IsAdmin`
- `apps/api/internal/shared/validation/validation.go` — phone rule
- `apps/api/internal/server/router.go` — wire `teachers` service into auth,
  drop `requireAdmin`
- `apps/api/internal/testutil/fixtures.go` — `Teacher()` fixture

**Delete**

- `apps/api/internal/features/users/` — after phase 3 confirms nothing is left
  to salvage. Phase 2 stops importing it; phase 3 removes the directory.

## Implementation Steps

1. Create `internal/features/teachers/model.go`. `Account` maps `user_accounts`:
   `ID uuid.UUID` (no `default:` tag — D3), `Role`, `Phone`, `PasswordHash
   *string` (nullable per schema), `Status`, `LastLoginAt *time.Time`,
   `CreatedAt`, `UpdatedAt`, `DeletedAt gorm.DeletedAt`. Add
   `func (Account) TableName() string { return "user_accounts" }` — GORM would
   otherwise pluralise to `accounts`. `Teacher` maps `teachers`: `ID`,
   `FullName`, `Timezone`, timestamps, `DeletedAt`. Declare status and role
   constants next to the structs (D5).
2. Create `internal/features/teachers/repository.go` following the pattern in
   `apps/api/internal/features/users/repository.go`: a `Repository` interface,
   a `gormRepository` using `database.FromContext(ctx, r.db)` so it joins the
   ambient transaction, and a `translateError` mapping
   `gorm.ErrDuplicatedKey` → `ErrDuplicatePhone`. Every read filters
   `deleted_at IS NULL` (GORM's `gorm.DeletedAt` does this automatically —
   verify it in the repository test rather than assuming).
3. Add `CreateAccountWithProfile(ctx, acct *Account, t *Teacher) error`: insert
   both rows on the context's transaction. The caller supplies both ids
   already set to the same value.
4. Create `internal/features/teachers/service.go` with `CreateTeacher`,
   `GetByPhone`, `GetByID`, `TouchLastLogin`. `CreateTeacher` generates
   `id.New()` once and uses it for both rows, normalises the phone to E.164,
   and hashes the password with bcrypt at the cost already used in
   `seeds/seed.go` (12).
5. Rewrite `auth/dto.go`. `RegisterRequest`: `Phone string \`json:"phone"
   binding:"required,vnphone"\``, `Password string
   \`binding:"required,min=8,max=72"\``, `FullName string
   \`json:"full_name" binding:"required,min=1,max=100"\`` (100 matches
   `teachers.full_name VARCHAR(100)`). `LoginRequest`: phone + password.
   `TokenResponse` embeds a teacher response object instead of `users.Response`.
6. Add the `vnphone` validator in
   `internal/shared/validation/validation.go` plus a `Normalize(phone) string`
   helper. Unit-test both forms and the reject cases.
7. Rewrite `auth/service.go`:
   - Replace the `UserService` interface (lines 18-22) with `AccountService`.
   - `Register` calls `teachersSvc.CreateTeacher` inside the existing
     `s.tx.WithinTx` block, then `openSession`.
   - `Login` looks up by normalised phone; add explicit branches for
     `status != active`, `deleted_at set`, and `PasswordHash == nil`, each
     burning the dummy bcrypt comparison before returning the shared `invalid`
     error. On success, call `TouchLastLogin`.
   - `Refresh`: after loading the account at the equivalent of line 117, reject
     non-active accounts with the same generic `invalid` error.
   - `issueSession` passes `authctx.RoleTeacher` to `IssueAccess`.
   - Leave the rotation, family-revocation, and race-loss handling
     (lines 106-152) structurally unchanged.
8. Update `authctx.go`: replace `RoleAdmin`/`RoleUser` with the three schema
   roles, delete `Principal.IsAdmin`. Add
   `func TeacherID(c *gin.Context) (uuid.UUID, bool)` returning the principal's
   `UserID` only when `Role == RoleTeacher` — this is the single accessor every
   later feature uses for tenancy (D4). Phase 3 documents it further.
9. Update `internal/server/router.go`: construct
   `teachers.NewService(teachers.NewRepository(db))`, pass it to
   `auth.NewService`, drop `requireAdmin` and the `users` wiring.
10. Update `internal/testutil/fixtures.go`: `Teacher(t, db, opts...)` inserts a
    `user_accounts` + `teachers` pair with a unique random phone and a
    `bcrypt.MinCost` hash, returning both. Keep `DefaultPassword` and
    `JWTSecret` as they are.
11. Port the auth tests. `service_test.go` cases map one-to-one (register
    success, duplicate phone → conflict, login wrong password, refresh
    rotation, reuse revokes family). Add: login to a disabled account fails;
    refresh with a valid token on a since-disabled account fails; register
    rolls back the `user_accounts` row when the `teachers` insert fails.
12. Update swagger annotations in `auth/handler.go`, then run `make api-docs`.
13. Run `make test-api-unit`, then `make test-api`.

## Success Criteria

- [x] Registering returns 201 with an access token; `user_accounts` and
      `teachers` both have a row sharing the same id
- [x] A failed `teachers` insert leaves no `user_accounts` row (transaction
      rollback asserted in an integration test)
- [x] Registering an already-registered phone returns 409, driven by the unique
      index rather than a pre-check SELECT
- [x] `0912345678` and `+84912345678` resolve to the same account
- [x] Login with an unknown phone, a wrong password, a disabled account, and a
      null `password_hash` all return the identical 401 body
- [x] Refresh rotates the token; replaying a rotated token revokes the family
- [x] Refresh on a disabled account returns 401
- [x] JWT `sub` parses to the teacher id and `role` is `teachers`
- [x] `grep -rn "features/users" apps/api` returns nothing outside the
      `features/users` directory itself
- [x] `make test-api` passes; `make api-docs` produces no email-based operations

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Rotation/reuse-revocation semantics get lost while re-pointing the service | Medium | High — silent auth regression | Port `service_test.go` cases before touching `service.go`; keep the comments at lines 106-113 and 137-140 that explain why revocations run outside the transaction |
| Phone stored in mixed formats, defeating `uq_users_phone` | Medium | High — duplicate accounts break tenancy | Normalise in exactly one place (service, pre-repository); repository test asserts both input forms collide |
| GORM pluralises `Account` to `accounts` and every query 404s | High | Low — caught immediately | Explicit `TableName()` on both models; repository test hits a real Postgres |
| Timing side channel reintroduced by the new disabled/null-hash branches | Medium | Medium | Every failure branch burns the dummy bcrypt comparison before returning; a test asserts all four failure modes produce byte-identical responses |
| Deleting `IsAdmin` breaks callers not found by grep (e.g. templates) | Low | Low | Compiler catches Go callers; `make test-api` and `make lint-api` gate the merge |
