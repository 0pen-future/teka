# Brainstorm: Invite-only onboarding via Zalo, disable-on-remove

Status: accepted contract, ready for plan.
Amended 2026-08-12: added self-service forgot-password (decision 6) and fixed
operator CLI scope (decision 7); resolves prior unresolved questions 1 and 2.
Amended 2026-08-12 (second pass): reset-token expiry 48h (decision 8), owner
reset via operator CLI only (decision 9); resolves questions 3 and 4.
Supersedes V1 consent model (self-register + join-by-phone). Builds on predict
report (session 2026-08-12): tenancy (migration 000007) and owner-is-teacher
already satisfied; this delivery replaces the onboarding/offboarding edges only.

## Decisions (user-confirmed)

1. Remove public self-registration.
2. Onboard teachers via owner-created **invitation link** (no credentials in
   transit), auto-sent as DM through the owner's linked personal Zalo
   (`FindUser` phone→UID + `SendDM`, both exist in `features/zalo`), with
   always-visible copy-link fallback.
3. Removing a teacher **disables their login** (no personal-center fallback).
4. No self-leave: `DELETE /centers/me/members/:id` becomes owner-only.
5. Center management owner-only. Members keep read access to center *name*
   only (their UI needs it); roster/rename/invite/remove/dashboard = owner.
6. Teachers get **self-service forgot-password**: public page, enter phone →
   single-use reset link delivered best-effort as Zalo DM sent from the
   center owner's linked personal Zalo. No copy-link surface for reset (link
   shown to the requester would defeat the password barrier).
7. Operator CLI is limited to exactly three capabilities: **create center**,
   **create owner**, and **reset password** (decision 9). No other operator
   commands in this delivery.
8. Reset token expires in **48h** (user-set; longer than the 15–60 min
   convention — accepted because delivery is a Zalo DM the teacher may read
   late; single-use + hashed + one-live-token-per-account remain the guard).
9. Owners may **not** use the public forgot-password flow (their phone gets
   the same generic response, no token, no send). Owner recovery = contact
   the system operator, who runs the CLI `reset-password` command.

## Outcome

- Teacher accounts come to exist only through invitation accept. Production
  bootstrap: Cobra subcommands on the existing `cmd/api` binary
  (`internal/cli`) to create a center and create an owner — nothing else;
  system-owner UI is future work.
- Owner creates invite for a phone → single-use tokenized link with expiry →
  best-effort Zalo DM + copy link. Invitation lifecycle:
  `pending → accepted | expired | revoked`.
- Accept (unauthenticated page): teacher sets name + password → account
  created active + live membership in inviting center. If phone belongs to a
  **disabled** (removed) account: accept re-enables it with the new password
  and opens membership in the inviting center (re-invite path).
- Remove: close membership, set account status disabled, create **no** new
  center. Data stays anchored in the old center (existing guard FKs).
- Delete join-by-phone flow (API `POST /centers/join` + web join form).
- Forgot-password: public request page (phone) → always-generic response →
  if phone matches an **active member** account (owners excluded, decision
  9), create single-use hashed reset token (48h expiry) and best-effort DM
  the reset link from that teacher's center owner's linked Zalo. Reset page:
  set new password → revoke all outstanding refresh tokens for the account
  (repository already supports revocation). Disabled accounts never get a
  reset link — their only path back is owner re-invite.
- Operator CLI `reset-password`: sets a new password for an account looked
  up by phone and revokes its refresh tokens. Works for any account;
  primarily the owner-recovery path (operator already holds DB superpowers,
  so restricting the target adds no security). Password input must not land
  in shell history — non-echo prompt or generated one-time value.

## Constraints

- Anti-enumeration discipline preserved: invite-create and accept responses
  never distinguish new phone vs existing account; accept failures collapse to
  one generic error.
- Token: high-entropy, stored hashed, single-use, default expiry ~72h,
  revocable by owner.
- Zalo send is best-effort transport only — invitation validity never depends
  on DM delivery; handle ErrNotLinked/lookup-privacy/send failure by falling
  back to copy-link. No retry storms (ban risk on unofficial protocol).
- Membership invariants untouched: `uq_center_members_active`,
  close-before-open, guard FKs. `teachers.center_id` stays NOT NULL pointing
  at last center; disabled + no live membership already resolves to 401 via
  `ResolveScope`.
- Accept endpoint is public: rate-limit / token-entropy is the defense.
- Forgot-password endpoint is public and auto-triggers a DM from the owner's
  personal Zalo without owner action: strict per-phone cooldown (one live
  reset token per account, minimum interval between sends) and per-IP rate
  limit are mandatory — both anti-enumeration and Zalo ban-risk defense.
  Single best-effort send, never retried. Response identical whether the
  phone exists, is disabled, or the DM failed.
- `make seed` (dev) keeps working; operator CLI adds create-center,
  create-owner, and reset-password subcommands only, following the existing
  `internal/cli` Cobra pattern (`serve`/`migrate`/`seed`).

## Non-goals

- System-owner admin UI/API for creating centers; any operator CLI command
  beyond create-center / create-owner / reset-password (no list, update, or
  delete ops).
- Zalo OAuth login ("authen with Zalo" = existing personal-account link as
  transport, already shipped).
- Non-Zalo reset transports (email/SMS) and owner-side "reset password"
  roster action — owner re-invite already covers the locked-out-and-DM-
  unreachable case.
- Migrating/merging existing personal centers; multi-center membership; roles
  beyond owner/member; email/SMS transports.

## Acceptance criteria

- `POST /auth/register` removed from router + OpenAPI; web register page gone.
- Owner invite: creates pending invitation, returns/shows link; Zalo DM
  attempted when owner linked (assert via SendFunc seam in tests); copy always
  available.
- Accept: valid token → active account + membership in inviting center;
  used/expired/revoked → generic failure; disabled-account re-invite →
  re-enabled with new password in the new center.
- Removed teacher: next request 401 (scope) and login rejected; no new centers
  row; their classes/students remain visible on owner dashboard.
- `POST /centers/join` gone; `GET /centers/me` returns roster only to owner,
  name to members; remove endpoint owner-only.
- Forgot-password: request with active-member phone → reset token created
  (48h expiry), DM attempted via owner's Zalo (SendFunc seam asserted in
  tests); request with unknown/disabled/**owner** phone → identical generic
  response, no token, no send; valid reset token → new password works, old
  refresh tokens revoked; used/expired token → generic failure; cooldown
  blocks a second token inside the window.
- Operator CLI: create-center and create-owner subcommands work against a
  fresh database (owner can log in); reset-password sets a new password for
  an existing account (including an owner) and revokes its refresh tokens;
  no other operator subcommands exist.
- Focused API tests (invite lifecycle, remove-disable, re-invite, reset
  lifecycle) + web tests for invite/accept/forgot/reset UI; full suites
  green.

## Design choices recorded

- Account created at **accept-time**, not invite-time: no orphan accounts, no
  temp credentials, phone collision handled inside accept.
- Invitation is its own table/feature slice under `features/centers` (or
  sibling `invitations`) — plan decides placement; reuse membership tx helpers
  from `centers/service.go` (close/open/switch).
- Reset token shares the invitation token *discipline* (high-entropy, stored
  hashed via existing `auth.HashToken`, single-use) but not its lifecycle or
  table: reset never creates accounts or memberships. Plan decides whether it
  lives in `features/auth` or beside invitations.

## Unresolved questions

1. ~~Forgot-password gap~~ — resolved: self-service forgot-password in scope
   (decision 6).
2. ~~Bootstrap command scope~~ — resolved: create-center + create-owner only
   (decision 7). Exact flags/naming remain plan detail.
3. ~~Reset-token expiry~~ — resolved: 48h (decision 8, user-set).
4. ~~Owner self-lockout~~ — resolved: owners excluded from public reset;
   operator CLI `reset-password` is the owner-recovery path (decision 9).
5. CLI reset-password input mechanics (non-echo prompt vs generated one-time
   password) and flags/naming — plan detail; either satisfies the no-secrets-
   in-shell-history constraint.
