# Phase 7: OpenAPI Docs and Verification Sweep — Completion Report

Status: DONE (make e2e deferred to deploy stage per instruction, not run)

## OpenAPI regeneration

Ran `make api-docs` (`go tool swag init -g cmd/api/main.go -o docs --parseInternal`,
swag v1.16.6, already pinned as a Go tool dependency in `apps/api/go.mod` —
no install needed). Diff vs the pre-phase-7 committed baseline:

```
apps/api/docs/docs.go      | 786 ++++++++++++++++++++++++++++++++++++---------
apps/api/docs/swagger.json | 786 ++++++++++++++++++++++++++++++++++++---------
apps/api/docs/swagger.yaml | 444 ++++++++++++++++++++-----
```

Route-level diff (`+`/`-` on `  /path:` lines):

```
+  /auth/forgot-password
-  /auth/register
+  /auth/reset-password
-  /centers/join
+  /centers/me/invitations
+  /centers/me/invitations/{id}
+  /invitations/accept
+  /invitations/preview
```

(`/centers/me` shows on both sides only because its response shape changed —
still present, not added/removed.) Verified HTTP methods:
`/centers/me/invitations` has GET+POST, `/centers/me/invitations/{id}` has
DELETE, `/centers/me/members/{teacherId}` has DELETE. Matches the phase spec's
intended route set exactly; no missing swag annotation needed.

**Drift check**: ran `make api-docs` a second time and diffed the two
consecutive generations directly (not against git HEAD, since the first
generation is itself uncommitted) — `diff -rq` on `apps/api/docs/` reported
zero differences. Generation is deterministic.

## Docs sweep

Grepped `docs/` for `register|/centers/join|personal center|password reset|create-owner|API_WEB_PUBLIC_BASE_URL`.
Findings and resolution:

- `docs/api-guidelines.md:167` ("Extension points… password reset… not
  implemented") — **false**, fixed: removed password reset from the
  out-of-scope list, kept OTP/parent-student portal (still genuinely out of
  scope — verified no such endpoints exist).
- `docs/api-guidelines.md:178` ("no separate admin bootstrap — every account
  is a teacher created via `/auth/register` or the seeder") — **false**,
  fixed: replaced with accurate description — no public self-registration,
  accounts created via invitation accept, first center/owner bootstrapped by
  `api create-center` (atomic, verified against
  `apps/api/internal/cli/create_center.go`), `api reset-password` for
  recovery. Confirmed exactly two onboarding subcommands exist in
  `internal/cli/root.go:19` (`createCenterCmd`, `resetPasswordCmd` — no
  `create-owner`).
- Added a new "Invitation and reset-token discipline" paragraph under
  Authentication: token shape (opaque 256-bit, sha256-hashed at rest, single
  use, body-only never path), TTLs sourced from
  `internal/config/config.go:141-148` (`API_INVITE_TTL` default 72h,
  `API_RESET_TTL` default 48h, `API_RESET_COOLDOWN` default 15m), and the
  owner-exclusion-from-forgot-password rule.
- `docs/prd.md` — grepped for register/onboard/invite: zero hits, no
  self-registration claim present. Not modified (nothing to fix).
- `docs/local-development.md` — reviewed in full: `make seed` reference is
  still accurate (seeding is untouched, still phone-keyed dev fixtures), no
  register-based bootstrap documented. Not modified (nothing to fix).
- "personal center" fallback wording: zero hits anywhere in `docs/` — the
  phase file's "may reference" caveat did not materialize; nothing to fix in
  the Tenancy section.

Files modified: `docs/api-guidelines.md` only (smallest owning surface, per
documentation-management rule).

## Verification matrix

| Command | Result |
|---|---|
| `make lint-api` | **PASS** — 0 issues |
| `make test-api` | **PASS** — all packages ok, total coverage 70.5% (floor 60%) |
| `make lint-web` | **FAIL, pre-existing, out of scope** — see below |
| `make test-web` | **PASS** — 44 files, 251 tests |
| `make build-web` | **PASS** — built in 715ms |
| `make api-docs && git diff --exit-code apps/api/docs` (drift, run as two-consecutive-generation diff) | **PASS** — identical |
| `make e2e` | **deferred**, not run (needs live Docker stack per deploy stage) |

### `make lint-web` failure detail (pre-existing, confirmed out of scope)

4 React-compiler "incompatible library" warnings (`react-hooks/incompatible-library`,
`form.watch()` from react-hook-form) and 5 prettier formatting failures.
Confirmed via `git status --short` on every implicated file — **zero** of
these 9 files appear in the working tree diff for this feature:

- Compiler warnings: `profile-page.tsx`, `class-dialog.tsx`, `student-dialog.tsx`,
  `class-settings-page.tsx`
- Prettier: `zalo-auto-map.test.tsx`, `enroll-student-dialog.tsx`,
  `schedule-slots-editor.tsx`, `zalo-auto-map-dialog.tsx`,
  `dashboard-layout.test.tsx`

Not fixed — outside this feature's scope per the assignment brief. Reported
here, not silently skipped.

## Acceptance-criteria mapping

Walking `plans/reports/brainstorm-260812-0837-invite-only-onboarding.md`
"Acceptance criteria" section item by item:

1. **`POST /auth/register` removed from router + OpenAPI; web register page
   gone.**
   - Source: `apps/api/internal/features/auth/routes.go` registers only
     login/refresh/logout/forgot-password/reset-password — no register.
   - Test: `apps/api/internal/features/auth/handler_test.go::TestAuthRegisterRouteIsGone`.
   - OpenAPI: `swagger.yaml` route list has no `/auth/register` (verified above).
   - Web: `apps/web/src/features/auth/pages/register-page.tsx` deleted;
     `apps/web/src/features/auth/__tests__/register-page.test.tsx` deleted;
     `apps/web/src/features/auth/routes.tsx` no longer references it (git
     status confirms both deletions).

2. **Owner invite: creates pending invitation, returns/shows link; Zalo DM
   attempted when owner linked; copy always available.**
   - API: `invitations/handler_test.go::TestCreateReturnsEnvelopeWithLinkAndDMStatus`,
     `invitations/service_test.go::TestCreateReturnsLinkAndCommitsBeforeAttemptingDM`,
     `::TestCreateDMStatusSkippedWhenOwnerHasNoLinkedZalo`,
     `::TestCreateDMStatusSkippedWhenPhoneIsNotAFriend`,
     `::TestCreateDMStatusFailedWhenSendDMErrors`,
     `::TestCreateDMStatusFailedOnTimeoutWithoutFailingCreate` (SendFunc seam
     asserted per test name).
   - Web: `invitation/__tests__/invite-section.test.tsx::"creates an invite
     and shows the copyable link dialog"`, `::"copies the link to the
     clipboard"`.

3. **Accept: valid token → active account + membership in inviting center;
   used/expired/revoked → generic failure; disabled-account re-invite →
   re-enabled with new password in the new center.**
   - `invitations/integration_test.go::TestAcceptNewPhoneRoundTripCreatesAccountThatCanLogIn`,
     `::TestAcceptReInviteRoundTripReactivatesRemovedMember`,
     `::TestAcceptRejectsDisabledAccountNeverAMemberOfThisCenter`,
     `::TestAcceptConcurrentDoubleAcceptOfSameTokenOnlyOneWins`.
   - `invitations/service_test.go::TestAcceptRejectionIsIdenticalAcrossEveryFailureReason`
     covers used/expired/revoked collapsing to one generic error.
   - Web: `invitation/__tests__/accept-invite-page.test.tsx` (4 cases,
     including the generic-error case).

4. **Removed teacher: next request 401 (scope) and login rejected; no new
   centers row; their classes/students remain visible on owner dashboard.**
   - `centers/integration_test.go::TestKickIsEffectiveOnTheNextRequest`,
     `::TestRemoveMemberByOwnerDataStaysBehind`,
     `::TestDashboardKeepsARemovedTeachersData`.
   - `auth/service_test.go::TestLoginRejectsDisabledAccount`,
     `auth/integration_test.go::TestLoginRejectsDisabledAccountAgainstRealSQL`.

5. **`POST /centers/join` gone; `GET /centers/me` returns roster only to
   owner, name to members; remove endpoint owner-only.**
   - Source: `centers/routes.go` has no join registration; confirmed absent
     from regenerated OpenAPI route list.
   - Web: `apps/web/src/features/center/components/join-center-form.tsx`
     deleted (git status).
   - `centers/integration_test.go::TestMeShowsCenterAndMembers`,
     `::TestRemoveMemberAuthorizationMatrix`.
   - Web: `center/__tests__/center-page.test.tsx::"shows only the center name
     and the member badge, no owner-only controls"` (member view).

6. **Forgot-password: active-member phone → token created (48h expiry), DM
   attempted; unknown/disabled/owner phone → identical generic response, no
   token, no send; valid token → new password works, old refresh tokens
   revoked; used/expired → generic failure; cooldown blocks a second token.**
   - `auth/service_test.go::TestForgotPasswordMemberMintsTokenAndAttemptsDM`,
     `::TestForgotPasswordOwnerIsANoOpWithNoTokenOrSend`,
     `::TestForgotPasswordUnknownPhoneIsANoOp`,
     `::TestForgotPasswordDisabledAccountIsANoOp`,
     `::TestForgotPasswordCooldownBlocksSecondRequest`,
     `::TestForgotPasswordAfterCooldownSupersedesPreviousToken`,
     `::TestForgotPasswordDMFailureDoesNotInvalidateToken`,
     `::TestResetPasswordHappyPathSetsPasswordAndRevokesTokens`,
     `::TestResetPasswordRejectionIsIdenticalAcrossEveryFailureReason`.
   - `auth/integration_test.go::TestForgotPasswordAndResetRoundTripAgainstRealSQL`,
     `::TestForgotPasswordExcludesCenterOwnerAgainstRealSQL`,
     `::TestForgotPasswordConcurrentRequestsLeaveExactlyOneLiveToken`,
     `::TestResetPasswordConcurrentDoubleConsumeOnlyOneWins`.
   - Web: `auth/__tests__/forgot-password-page.test.tsx` (3 cases),
     `auth/__tests__/reset-password-page.test.tsx` (3 cases).

7. **Operator CLI: create-center and create-owner subcommands work against a
   fresh database (owner can log in); reset-password sets a new password for
   an existing account (including an owner) and revokes its refresh tokens;
   no other operator subcommands exist.**
   - Note: brainstorm decision 7 originally named 3 capabilities
     (create-center/create-owner/reset-password); the accepted plan amended
     this to **2** subcommands — `create-center` is atomic (creates center +
     owner in one transaction), no separate `create-owner`. Verified against
     `internal/cli/root.go:19`: `rootCmd.AddCommand(serveCmd, migrateCmd,
     seedCmd, createCenterCmd, resetPasswordCmd)` — exactly 2 onboarding
     subcommands beyond serve/migrate/seed, matching the amendment.
   - `cli/bootstrap_integration_test.go::TestBootstrapCenterFreshDBOwnerCanLogInAndIsOwner`,
     `::TestBootstrapCenterDuplicatePhoneRollsBackEverything`,
     `::TestResetPasswordRevokesSessionAndAllowsReloginWithNewPassword`,
     `::TestResetPasswordWorksOnDisabledAccountWithoutReactivatingIt`.
   - `cli/create_center_test.go`, `cli/reset_password_test.go`,
     `cli/password_prompt_test.go` cover flag validation and non-echo
     password input mechanics.

8. **Focused API tests (invite lifecycle, remove-disable, re-invite, reset
   lifecycle) + web tests for invite/accept/forgot/reset UI; full suites
   green.**
   - `make test-api`: PASS (all packages, 70.5% coverage).
   - `make test-web`: PASS (44 files / 251 tests), including
     `invitation/__tests__/*` (4 files), `auth/__tests__/forgot-password-page.test.tsx`,
     `auth/__tests__/reset-password-page.test.tsx`.
   - e2e specs exist (`e2e/invite-accept.spec.ts`, `e2e/forgot-password.spec.ts`)
     but `make e2e` itself is deferred to the deploy stage per this phase's
     scope — not executed in this pass.

All 8 acceptance bullets map to passing evidence. No unmapped bullet found.

## Constraints check

- Anti-enumeration: `TestAcceptRejectionIsIdenticalAcrossEveryFailureReason`,
  `TestForgotPasswordUnknownPhoneIsANoOp`/`DisabledAccountIsANoOp`/`OwnerIsANoOpWithNoTokenOrSend`,
  `TestResetPasswordRejectionIsIdenticalAcrossEveryFailureReason` all assert
  identical responses across failure branches.
- Token discipline (high-entropy, hashed, single-use): `internal/shared/token/token.go`
  (256-bit via `crypto/rand`, sha256 hash stored) — shared by invitations and
  reset, as designed.
- Rate limiting: `internal/middleware/ratelimit.go` +
  `ratelimit_test.go` (7 tests) back the per-IP/per-phone limits on
  forgot-password and preview/accept.

## Concerns/Blockers

None. `make e2e` intentionally not run (deferred). Pre-existing lint-web
residue documented above, not fixed (explicitly out of scope for this phase).

Status: DONE
Summary: OpenAPI regenerated with zero drift and the exact expected route
diff; api-guidelines.md docs sweep fixed the two false claims (password-reset
"not implemented", register-only account creation) and added token-discipline
documentation; full static verification matrix green except pre-existing,
untouched-file lint-web residue; all 8 brainstorm acceptance bullets mapped
to passing tests.
