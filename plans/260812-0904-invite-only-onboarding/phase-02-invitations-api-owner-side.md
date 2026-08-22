---
phase: 2
title: "Invitations API — Owner Side"
status: done
priority: P1
effort: "6h"
dependencies: [1]
---

# Phase 2: Invitations API — Owner Side

## Overview

New `internal/features/invitations` slice: owner creates / lists / revokes
invitations; create attempts a best-effort Zalo DM with the accept link and
always returns the copy-link.

## Key Insights

- `SendDM(ctx, teacherID, toUID, text)` (`features/zalo/service.go:268`) is the
  real send seam. **There is no `FindUser` seam** — the only public phone→UID
  API is `MatchFriends(ctx, teacherID, phones)` (`service.go:392`), which
  fetches the owner's *entire* friend list and returns
  `map[phone]protocol.FoundUser` (note: `FoundUser` lives in `protocol`, not
  `zalo`). This phase must add a narrow `LookupPhone(ctx, teacherID, phone)
  (uid string, ok bool, err error)` adapter on `*zalo.Service` wrapping
  `MatchFriends` for a single phone; the consumer interface targets that, not a
  non-existent `FindUser`.
- A missing linked Zalo session surfaces as `ErrNotLinked` (not
  `ErrAccountNotFound`, which is the account-lookup error) — treat it plus an
  absent phone as DM status `skipped`; any send/timeout error → `failed`. None
  is an HTTP error.
- `MatchFriends` "can take seconds" (`service.go:526-527`) and talks to an
  unofficial endpoint — the lookup+send must run under a bounded
  `context.WithTimeout` and strictly *after* the invitation is committed, so a
  slow/failed Zalo call never delays or fails invite creation.
- Owner-only pattern: authorization check before param validation
  (`centers/handler.go` style, uniform 403).
- Anti-enumeration: create response is identical whether the phone belongs to
  an existing account or not — no lookup leak.

## Requirements

- Functional: `POST /centers/me/invitations {phone}` → creates pending
  invitation (TTL = `OnboardingConfig.InviteTTL`), supersedes any previous
  pending invite for that (center, phone) by revoking it in the same tx,
  returns `{id, phone, expires_at, link, dm_status}`;
  `GET /centers/me/invitations` → list (pending first, derived `expired`
  status in DTO); `DELETE /centers/me/invitations/:id` → revoke (idempotent
  on already-revoked; 404 for other centers' invites).
- Non-functional: single DM attempt, no retries (Zalo ban risk); link =
  `{cfg.Statements.PublicBaseURL}/invite/{plaintext-token}`.

## Architecture

Feature contract per `docs/api-guidelines.md`: `model.go`, `repository.go`
(interface + GORM), `service.go`, `dto.go`, `handler.go`, `routes.go`,
`service_test.go`, `handler_test.go`, `integration_test.go`.

Cross-feature interface defined by invitations (consumer side):

```go
// service.go
type ZaloSender interface {
    // LookupPhone reports the invitee's Zalo UID from the owner's friend
    // list; ok=false when the phone isn't a friend. ErrNotLinked when the
    // owner has no live Zalo session.
    LookupPhone(ctx context.Context, teacherID uuid.UUID, phone string) (uid string, ok bool, err error)
    SendDM(ctx context.Context, teacherID uuid.UUID, toUID, text string) (string, error)
}
```

Implemented by `*zalo.Service` via the new `LookupPhone` adapter (this phase
adds it, wrapping `MatchFriends`); wired in `server.registerFeatures`. DM flow,
under `context.WithTimeout`: `LookupPhone(owner, invitee-phone)` → `ErrNotLinked`
or `ok=false` ⇒ `dm_status:"skipped"`; `SendDM` error/timeout ⇒ `"failed"`;
success ⇒ `"sent"`. All non-fatal.

Create flow (service, inside `TxManager.WithinTx`): revoke existing pending
row for (center, phone) → insert new row with `token.New()` hash → commit;
the response link is built and returned **before** any Zalo work. The DM
attempt happens *after* commit and outside the tx (invitation validity never
depends on delivery, and the copy-link is returned even if the DM stalls). A
concurrent second create for the same (center, phone) is serialized by the
partial unique index `uq_invitations_pending_phone`: on unique-violation the
service reloads and returns the surviving pending row idempotently rather than
erroring.

Routes: `invitations.RegisterRoutes(v1, h, requireAuth, resolveScope)`
mounting under `/centers/me/invitations`; handler enforces `scope.IsOwner`.

## Related Code Files

- Create: `apps/api/internal/features/invitations/{model,repository,service,dto,handler,routes}.go`
- Create: `apps/api/internal/features/invitations/{service_test,handler_test,integration_test}.go`
- Modify: `apps/api/internal/features/zalo/service.go` (add `LookupPhone`
  adapter over `MatchFriends`; export `ErrNotLinked` if not already) + its test
- Modify: `apps/api/internal/server/router.go` (construct + register feature,
  pass zalo service + onboarding config)

## Implementation Steps (TDD)

### Tests Before
1. `service_test.go` (fake repo, fake ZaloSender, noopTxManager): create
   happy path (link present in response before any DM call); supersede-
   previous-pending; non-owner → Forbidden; DM skipped (`ErrNotLinked` /
   `ok=false`) / failed (SendDM error) / sent all return success with matching
   `dm_status`; DM timeout → `failed` without failing the create; phone
   normalized to E.164; revoke idempotency; revoke other-center → NotFound.
2. `handler_test.go`: 401 without token, 403 member, envelope shape, 422 on
   bad phone (`vnphone` binding), list serializes `[]` not `null`.

### Refactor
3. Implement model/repo (scoped by `center_id` from `authctx.ScopeFrom`
   only), service, DTO mappers (derived `expired`), handler, routes.
4. Wire in `router.go`.

### Tests After
5. `integration_test.go` (real Postgres): partial-unique supersede race —
   two concurrent creates for same phone → exactly one pending survives;
   token_hash uniqueness; cross-center read isolation.

### Regression Gate
```sh
make test-api && make lint-api
```

## Todo

- [ ] Feature slice files + wiring
- [ ] Unit + HTTP tests green (DM seam asserted)
- [ ] Integration supersede/isolation tests green

## Success Criteria

- [ ] Owner invite lifecycle works end-to-end via HTTP tests
- [ ] DM assertion through SendFunc seam (brainstorm acceptance line)
- [ ] Copy-link always present in create response

## Risk Assessment

- **`MatchFriends` fetches the whole friend list and is slow/unofficial** — the
  `LookupPhone` adapter isolates that cost behind a bounded timeout, run only
  after commit. If the owner is unlinked (`ErrNotLinked`) or the invitee isn't a
  friend (`ok=false`), map to `skipped`. Never let it block or fail the create.
- Token travels in the request body on accept (Phase 3, `POST /invitations/
  preview`), never the URL path — `middleware.Logger` records paths and redacts
  only `/public/statements/`, so a path-token would leak into access logs.

## Security Considerations

Owner-only creation; phone normalization prevents duplicate-spelling dupes;
response identical for known/unknown phone (anti-enumeration).

## Next Steps

Phase 3 consumes the token on the public accept endpoint.
