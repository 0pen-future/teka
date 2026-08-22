---
phase: 3
title: "Accept Flow and Offboarding Rewire"
status: done
priority: P1
effort: "6h"
dependencies: [2]
---

# Phase 3: Accept Flow and Offboarding Rewire

## Overview

Public accept endpoints create (or re-enable) the teacher account and open
membership in the inviting center. Simultaneously delete the two old edges —
`POST /auth/register` and `POST /centers/join` — and change member removal
from "provision personal center" to "disable login".

## Key Insights

- `centers.Service.RemoveMember` (`service.go:238-278`) currently creates a
  personal center + switches `teachers.center_id`. New behavior: close
  membership + set `user_accounts.status='disabled'` + revoke refresh-token
  families. `teachers.center_id` stays pointing at the last center (brainstorm
  constraint) — `ResolveScope` already 401s a disabled account.
- Account creation into an existing center belongs on **`teachers.Service`**,
  not `auth.Service`. `auth.Service` owns only tokens/credentials and has no
  write path to `user_accounts`/`teachers`; `teachers.Service` already creates
  teacher+account rows (`teachers/service.go:82` provisions in `CreateTeacher`).
  Add `teachers.Service.CreateInCenter(...)` — the accept flow's onboarder — and
  reuse it from the CLI (Phase 5). bcrypt cost lives at `teachers/service.go:18`
  (cost 12), not in auth.
- `SetCenterProvisioner` cycle-break on teachers→centers was introduced for
  register's personal-center creation. After register is deleted, audit its
  remaining callers: if only seed/`CreateInCenter` use it, keep the seam and
  state that as the justification; if nothing uses it, delete it. Do not carry
  the "still needed for create-owner" rationale — Phase 5 is now a single
  atomic `create-center`, not a separate `create-owner`.
- Membership invariants: `uq_center_members_active` + close-before-open
  inside one tx (existing helpers `CloseMembership`/`OpenMembership`).
- Re-invite must only re-open an account that `WasEverMember` of the inviting
  center (`centers/repository.go:344-356` already provides this) — binds the
  disabled→active transition to real prior membership instead of trusting the
  token alone.
- Accept is unauthenticated: token entropy + rate limit are the defense; all
  failures collapse to one generic error (anti-enumeration).

## Requirements

- Functional:
  - `POST /invitations/preview {token}` (public): valid pending unexpired →
    `{center_name, phone_masked}`; anything else → generic 404. Token is in the
    **body**, never the path, so it stays out of access logs.
  - `POST /invitations/accept {token, full_name, password}` (public):
    - no account for phone → create active account + teacher + membership in
      inviting center via `teachers.Service.CreateInCenter`
      (`teachers.center_id` = inviting center);
    - disabled account that `WasEverMember` of the inviting center → set new
      password + name, status→active, open membership, switch
      `teachers.center_id`, revoke any refresh tokens;
    - disabled account that was never a member of this center → generic failure;
    - active account → generic failure;
    - used/expired/revoked token → same generic failure (400, one message).
  - `DELETE /centers/me/members/:teacherId`: owner-only (member self-removal
    path deleted), disables target account, closes membership, revokes
    refresh tokens, creates **no** center row. Owner cannot remove self.
  - Delete `POST /auth/register` and `POST /centers/join` (handler, service,
    DTO, routes, swag annotations, tests).
  - `GET /centers/me`: owner → full roster; member → center name only
    (no members array). `PATCH /centers/me` is untouched and still returns the
    full center payload (see Architecture — only the GET read model is
    role-shaped).
- Non-functional: rate limit on all public onboarding endpoints keyed on
  **business identity** (invite/preview → token, forgot/reset → phone), not
  `c.ClientIP()`. The API sits behind Traefik with `SetTrustedProxies(nil)`
  (`router.go:48-51`), so `ClientIP()` is a single global bucket. New
  `middleware.RateLimit(keyFn, limit, window)`, one shared in-memory fixed-window
  store with a TTL sweeper; add a `429`/`TooManyRequests` code to `apperror`.

## Architecture

Accept lives in the invitations feature (it owns the token). Consumer-defined
interfaces added to `invitations.Service`:

```go
type AccountOnboarder interface {
    // CreateInCenter creates account+teacher rows (role teachers, status
    // active) inside the ambient tx. Implemented by teachers.Service, which
    // already owns teacher/account creation (teachers/service.go:82) and the
    // bcrypt cost (teachers/service.go:18).
    CreateInCenter(ctx context.Context, phone, fullName, password string, centerID uuid.UUID) (uuid.UUID, error)
    // Reactivate sets password+name, flips status to active, revokes all
    // refresh tokens. Returns ErrNotDisabled when account is active.
    Reactivate(ctx context.Context, accountID uuid.UUID, fullName, password string) error
    FindByPhone(ctx context.Context, phone string) (Account, error)
}
type MembershipOpener interface {
    OpenMembership(ctx context.Context, teacherID, centerID uuid.UUID) error
    SwitchTeacherCenter(ctx context.Context, teacherID, centerID uuid.UUID) error
    WasEverMember(ctx context.Context, teacherID, centerID uuid.UUID) (bool, error)
}
```

Implementations: `AccountOnboarder` by `teachers.Service` (owns the write path;
`Reactivate` uses `auth.Repository.RevokeAllForUser` — see below);
`MembershipOpener` by `centers.Service` (existing tx helpers +
`WasEverMember` at `centers/repository.go:344-356`). The accept service gates
the disabled→active branch on `WasEverMember(teacherID, invitingCenter)`. Whole
accept runs in one `WithinTx`; invitation row flips to `accepted` in the same tx.

`RemoveMember` rewrite stays in `centers.Service` but gains a consumer-defined
`AccountDisabler` interface (implemented by auth: set status disabled + revoke
refresh families) so centers never touches `user_accounts` directly.

**New `auth.Repository.RevokeAllForUser(ctx, userID)`** — the current repo
(`auth/repository.go:22-28`) only revokes a single token/family; disable,
reactivate, and reset all need account-wide revocation. Add it here so Phases 3–5
share one method.

`middleware.RateLimit(keyFn, limit, window)`: one shared fixed-window store
(counter map + mutex) with a background/lazy **TTL sweeper** so idle keys don't
leak memory; `keyFn` extracts the business identity (token for preview/accept)
from the request, not the IP. Emits the new `apperror` `429`. Reused in Phase 4
keyed on phone.

Routes: public group in `invitations.RegisterRoutes` (no requireAuth):
`POST /invitations/preview`, `POST /invitations/accept`.

## Related Code Files

- Modify: `apps/api/internal/features/invitations/{service,dto,handler,routes}.go` (+tests)
- Modify: `apps/api/internal/features/teachers/service.go` — add
  `CreateInCenter` + `Reactivate` onboarder facade (owns the write path) (+tests)
- Modify: `apps/api/internal/features/auth/{service,handler,routes,dto}.go` —
  delete register; add `AccountDisabler` facade (+tests)
- Modify: `apps/api/internal/features/auth/repository.go` — add
  `RevokeAllForUser` (+test)
- Modify: `apps/api/internal/features/centers/{service,handler,routes,dto}.go` —
  delete join; rewrite RemoveMember; role-shaped `GET /centers/me` only
  (`PATCH /centers/me` unchanged) (+tests)
- Modify: `apps/api/internal/apperror/*` — add `429`/`TooManyRequests` code
- Create: `apps/api/internal/middleware/ratelimit.go` + `ratelimit_test.go`
  (shared store + TTL sweeper, `keyFn`-based)
- Modify: `apps/api/internal/server/router.go` (wiring, middleware)
- Modify: `apps/api/internal/testutil/` fixtures if they call `svc.Register`

## Implementation Steps (TDD)

### Tests Before
1. Pin current invariants that must survive: login rejects disabled account
   (exists); `uq_center_members_active` integration probes (exist — verify).
2. New failing tests, unit + HTTP: accept new-phone happy path; accept
   re-invite (disabled + `WasEverMember`) path; accept disabled-but-never-member
   → generic; accept active-account → generic; token used/expired/revoked →
   byte-identical generic body; `POST /invitations/preview` with token in body
   (assert token never appears in a logged path); remove disables + revokes +
   no new center; remove by member → 403; owner self-remove → error;
   `GET /centers/me` member shape has no roster while `PATCH /centers/me` still
   returns full payload; register/join routes return 404 after removal; rate
   limiter unit test (window rollover, per-key isolation, sweeper evicts idle
   keys, 429 response).

### Refactor
3. Delete register + join (code, DTOs, swag, web-facing tests updated in P6);
   audit `SetCenterProvisioner` callers and keep-with-justification or delete.
4. Implement `teachers.CreateInCenter`/`Reactivate` + `auth.RevokeAllForUser` +
   `AccountDisabler` + accept service (WasEverMember gate) + RemoveMember
   rewrite + shared rate limiter + apperror 429.
5. Update `testutil`/seed fixtures that depended on Register (direct-insert
   `Teacher(t, db, ...)` fixture already bypasses it — verify seed path).

### Tests After
6. Integration: full invite→accept→login roundtrip; re-invite roundtrip
   (remove → disabled login 401 → re-invite → accept → login OK, data still
   anchored in old center); concurrent double-accept of one token → exactly
   one success (row-lock on invitation).

### Regression Gate
```sh
make test-api && make lint-api
```

## Todo

- [ ] Accept + preview endpoints (token in body) + generic-failure discipline
- [ ] Re-invite gated on `WasEverMember`
- [ ] `teachers.CreateInCenter`/`Reactivate` + `auth.RevokeAllForUser` in place
- [ ] Register + join fully deleted (grep `register\|/join` in api); dead
      `SetCenterProvisioner` resolved
- [ ] RemoveMember = disable + revoke, owner-only
- [ ] Role-shaped `GET /centers/me`; `PATCH /centers/me` unchanged
- [ ] Identity-keyed RateLimit middleware + sweeper + apperror 429, tests green
- [ ] Integration roundtrips green

## Success Criteria

- [ ] Brainstorm acceptance: accept/removal/re-invite bullets all pass
- [ ] Removed teacher: next request 401, login rejected, no new centers row,
      classes/students still visible on owner dashboard (integration assert)

## Risk Assessment

- **Biggest phase** — if it balloons, split delete-register/join into its own
  commit first (independently green: seeds/tests don't use them).
- Double-accept race → `SELECT ... FOR UPDATE` on invitation row inside tx.
- Deleting member self-removal changes a shipped UI affordance — web catches
  up in Phase 6; API-first ordering accepted (web tests would break only if
  they hit real API; MSW mocks isolate until P6).

## Security Considerations

Generic error bodies compared byte-for-byte in tests; rate limit on public
endpoints; account creation always role `teachers`, status transitions only
active↔disabled through defined facades.

## Next Steps

Phase 4 reuses RateLimit + token discipline for password reset.
