---
phase: 4
title: "Password Reset API"
status: done
priority: P1
effort: "6h"
dependencies: [3]
---

# Phase 4: Password Reset API

## Overview

Public forgot-password + reset endpoints on the auth feature: active
**member** phones get a 48h single-use hashed reset token DM'd from their
center owner's linked Zalo; owners, disabled and unknown phones get the same
generic response with no token and no send.

## Key Insights

- Owners are excluded (brainstorm decision 9) — their recovery is the
  operator CLI. Exclusion check = account's teacher is `centers.owner_id`
  of their current center.
- Delivery chain for a member: phone → account → teacher → `center_id` →
  center owner → owner's linked Zalo session → `LookupPhone(owner, member phone)`
  → UID → `SendDM(owner, uid, link)`. This reuses the same `ZaloSender`
  (`LookupPhone`/`SendDM`) adapter added in Phase 2 — there is no `FindUser`
  seam. Every link may be missing; all failures collapse into the same generic
  202-style response. The lookup+send runs under a bounded `context.WithTimeout`
  and only after the token row is committed.
- One live token per account + cooldown: the partial unique index
  `uq_password_reset_active` (Phase 1) enforces at-most-one live token in the DB;
  the service also checks the newest token is older than `ResetCooldown` (15m)
  and sets `superseded_at` on the previous row in the same tx before inserting
  the new one, so the index never rejects a legitimate re-request.
- Reset must revoke **all** refresh-token families for the account via the new
  `auth.Repository.RevokeAllForUser` (added in Phase 3) — the pre-existing repo
  only revoked a single token/family.

## Requirements

- Functional:
  - `POST /auth/forgot-password {phone}` → always the same generic success
    envelope. Side effects only when: phone matches active account AND
    account is a member (not owner) → create token (48h TTL), supersede
    previous, single best-effort DM with link
    `{cfg.Statements.PublicBaseURL}/reset-password/{plaintext}`.
  - `POST /auth/reset-password {token, password}` → valid unused unexpired
    unsuperseded token + active account → set bcrypt password, mark
    `used_at`, revoke all refresh tokens → 200; anything else → one generic
    400. Disabled accounts always fail (their path back is re-invite).
- Non-functional: RateLimit on both endpoints keyed on **phone** (forgot) /
  **token** (reset), reusing the Phase 3 shared limiter — not `ClientIP()`;
  cooldown enforced per-account in service; no retry on DM failure; Zalo
  lookup+send under a bounded timeout.

## Architecture

All inside `features/auth` (credential concern): extend `model.go` with
`PasswordResetToken` GORM model (table from migration 000008), repository
methods (`CreateResetToken`, `LatestResetToken`, `ConsumeResetToken` with
row lock, `SupersedeResetTokens`), service methods `ForgotPassword` /
`ResetPassword`.

Consumer-defined interfaces on auth (wired in `router.go`):

```go
type OwnerResolver interface {
    // CenterOwner returns the owner teacher id of the teacher's current
    // center, and whether the teacher IS that owner.
    CenterOwner(ctx context.Context, teacherID uuid.UUID) (ownerID uuid.UUID, isOwner bool, err error)
}
type ResetDMSender interface { // the same LookupPhone/SendDM adapter as invitations
    LookupPhone(ctx context.Context, teacherID uuid.UUID, phone string) (uid string, ok bool, err error)
    SendDM(ctx context.Context, teacherID uuid.UUID, toUID, text string) (string, error)
}
```

`OwnerResolver` implemented by `centers.Service` (single query joining
teacher→center). ForgotPassword: token create in tx (cooldown check +
supersede inside), then — after commit, under a bounded timeout —
`LookupPhone(ownerID, memberPhone)` to anchor the invitee's Zalo UID from the
owner's friend list, then `SendDM`. `ok=false`/`ErrNotLinked`/timeout → no send,
generic response, error logged only. Delivery is owner-anchored: the DM is sent
*from the owner's* Zalo session to the member's UID, so the member does not need
their own linked session.

Routes (`auth/routes.go`): both public, wrapped by the phone/token-keyed
RateLimit.

## Related Code Files

- Modify: `apps/api/internal/features/auth/{model,repository,service,dto,handler,routes}.go`
  (repository reuses `RevokeAllForUser` from Phase 3; reset token methods set
  `superseded_at`/`used_at` to satisfy `uq_password_reset_active`)
- Modify: `apps/api/internal/features/auth/{service_test,handler_test,integration_test}.go`
- Modify: `apps/api/internal/features/centers/service.go` (+`CenterOwner`)
- Modify: `apps/api/internal/server/router.go` (wiring; phone/token RateLimit)

## Implementation Steps (TDD)

### Tests Before
1. Unit (fake repo + fake OwnerResolver + fake ResetDMSender): member phone →
   token created + DM attempted (seam asserted); owner phone → no token, no
   send, same response; unknown phone → same response; disabled → same
   response; cooldown window blocks second token; second request after
   cooldown supersedes first (old token stops working); DM failure → token
   still valid + generic response.
2. Unit reset: happy path sets password + revokes refresh tokens; used /
   expired / superseded token → generic; disabled account → generic;
   password rules match register-era validation (reuse binding).
3. HTTP: envelope shapes; byte-identical generic bodies; rate-limit 429 path.

### Refactor
4. Model + repository + service + handler + routes + wiring.

### Tests After
5. Integration: full forgot→reset→login-with-new-password roundtrip; old
   refresh cookie dead after reset (`RevokeAllForUser`); concurrent
   double-consume of one token → one success (row lock); concurrent
   double-forgot for one account → `uq_password_reset_active` yields exactly
   one live token (supersede path exercised).

### Regression Gate
```sh
make test-api && make lint-api
```

## Todo

- [ ] Reset token repo/service/endpoints
- [ ] Owner-exclusion + cooldown + supersede logic
- [ ] DM seam assertions
- [ ] Integration roundtrip green

## Success Criteria

- [ ] Brainstorm forgot-password acceptance bullet passes in full
- [ ] Generic response byte-identical across all no-op branches

## Risk Assessment

- Timing side-channel: no-op branches skip bcrypt/DB writes → measurably
  faster. Mitigation consistent with existing login pattern (dummy work);
  keep the token-create branch cheap and accept coarse parity — document.
- Owner's Zalo unlinked/expired, or member not in owner's friend list
  (`LookupPhone` → `ok=false`) → member can never receive DM; response stays
  generic. Product accepts (copy-link deliberately absent); owner re-invite is
  the documented fallback.
- `MatchFriends`-backed lookup is slow/unofficial — bounded timeout + after-
  commit execution keep the endpoint responsive and the token valid regardless
  of Zalo latency.

## Security Considerations

48h TTL is user-accepted (decision 8) with single-use + hashed + supersede
guards; owners excluded from the public flow (decision 9); rate limit + per
account cooldown throttle enumeration and Zalo ban risk.

## Next Steps

Phase 5 adds the operator CLI (owner recovery path).
