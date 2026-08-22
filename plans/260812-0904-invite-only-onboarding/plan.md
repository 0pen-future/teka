---
title: "Invite-Only Onboarding"
description: "Replace self-registration with owner-created Zalo invitation links, disable-on-remove offboarding, self-service forgot-password over Zalo DM, and an operator CLI for bootstrap and recovery."
status: done
priority: P1
effort: 4.5d
branch: master
tags: [feature, backend, frontend, auth, api, security, critical]
blockedBy: []
blocks: []
created: 2026-08-12
---

# Invite-Only Onboarding

## Overview

Teacher accounts come to exist only through owner-created invitation links
(single-use hashed token, 72h default expiry, best-effort Zalo DM + copy-link
fallback). Removing a teacher disables their login instead of provisioning a
personal center. Public self-registration and join-by-phone are deleted.
Teachers recover passwords through a public forgot-password flow delivering a
48h single-use reset link via the center owner's linked Zalo; owners recover
via operator CLI only. Production bootstrap = Cobra subcommands
`create-center` (atomic: creates the center **and** its owner in one tx) and
`reset-password` on the existing `cmd/api` binary.

Contract source: [brainstorm report](../reports/brainstorm-260812-0837-invite-only-onboarding.md)
(9 user-confirmed decisions; all previously unresolved questions closed except
CLI input mechanics, decided in Phase 5).

> **Scope amendment (post red-team, user-approved 2026-08-12):** brainstorm
> decision 7 originally named three CLI subcommands (`create-center`,
> `create-owner`, `reset-password`). The red-team review showed that splitting
> center and owner creation forced `centers.owner_id` to become nullable, which
> in turn broke `is_owner` resolution (NULL→bool scan) and the down-migration.
> The user approved collapsing bootstrap into a single **atomic** `create-center`
> that provisions center + owner + membership in one transaction, keeping
> `owner_id` NOT NULL. The CLI now exposes two onboarding subcommands.

## Cross-Plan Dependencies

None — all prior plans are completed (verified 2026-08-12 scan).

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Schema and Token Foundations](./phase-01-schema-and-token-foundations.md) | Done |
| 2 | [Invitations API — Owner Side](./phase-02-invitations-api-owner-side.md) | Done |
| 3 | [Accept Flow and Offboarding Rewire](./phase-03-accept-flow-and-offboarding-rewire.md) | Done |
| 4 | [Password Reset API](./phase-04-password-reset-api.md) | Done |
| 5 | [Operator CLI — Bootstrap and Recovery](./phase-05-operator-cli-bootstrap-and-recovery.md) | Done |
| 6 | [Web UI — Onboarding Surfaces](./phase-06-web-ui-onboarding-surfaces.md) | Done |
| 7 | [OpenAPI Docs and Verification Sweep](./phase-07-openapi-docs-and-verification-sweep.md) | Done |

Sequential: 1 → 2 → 3 → 4 → 5 → 6 → 7. Phase 5 only needs Phase 1+3 (service
seams), but keep sequence — shared files (`config.go`, `router.go`) make
parallel edits conflict-prone.

## Key Design Decisions (plan-level)

- New feature slice `internal/features/invitations` (sibling of `centers`) —
  it coordinates centers + auth-account + zalo; embedding in `centers` would
  couple those. Cross-feature access via consumer-defined interfaces only.
- Reset tokens live in `features/auth` (credential concern; auth already owns
  refresh-token revocation).
- Token generate/hash helpers promoted from `features/auth/tokens.go` to
  `internal/shared/token` so invitations reuse them without feature coupling.
- Invitation `expired` state is **derived** (`expires_at < now()` while
  `pending`) — no cron, no status writer.
- `centers.owner_id` **stays NOT NULL**. Bootstrap is a single atomic
  `create-center` command (see scope amendment above); the two-command +
  nullable-owner approach was rejected in red-team.
- Account provisioning into an **existing** center is owned by
  `teachers.Service` (`CreateInCenter`) — `auth.Service` has no write path to
  `user_accounts`/`teachers` and cannot host the onboarder facade. `auth`
  keeps only token concerns.
- All-refresh-token revocation for an account is a new
  `auth.Repository.RevokeAllForUser` — the existing repo only revokes a single
  token/family; reset-password and disable both need account-wide revocation.
- Rate limiting keys on **business identity** (phone for forgot-password, token
  for accept/preview), **not** `c.ClientIP()`: the API runs behind Traefik with
  `SetTrustedProxies(nil)`, so `ClientIP()` collapses to one global bucket. One
  shared in-memory fixed-window limiter with a TTL sweeper; add a `429` code to
  `apperror`.
- Public invite/reset links reuse `cfg.Statements.PublicBaseURL` (already set in
  every compose/env) rather than a new localhost-defaulting key.
- Invite/reset tokens travel in the request **body** (`POST /invitations/preview`),
  never the URL path — `middleware.Logger` writes request paths and only redacts
  `/public/statements/`.

## Success Criteria

- [x] All 9 acceptance criteria from the brainstorm report pass (register/join
      removed, invite lifecycle, disable-on-remove, re-invite, reset
      lifecycle, owner exclusion, CLI bootstrap + recovery, roster scoping)
- [x] `make test-api` (unit + HTTP + integration, ≥60% coverage) green (~70.9%)
- [x] `make test-web` (251/251), `make lint-web` (pre-existing residue only),
      `make lint-api` (0 issues), `make api-docs` clean
- [ ] Playwright e2e updated for invite-only world; smoke-tested live via
      public HTTPS edge (login 422, forgot-password 200, register 404) — full
      `make e2e` run against seeded stack still pending

## Red Team Review

Four hostile reviewers (security, failure-mode, assumption-audit, scope) ran a
full-verification pass over the draft. Findings required `file:line` evidence or
were rejected. 15 evidence-backed clusters were adjudicated; all 15 accepted
(the user walked each). Two dissolved once bootstrap became atomic. The plan and
phase files above already fold in every disposition; this section is the record.

The draft's most damaging defects were four **false "already exists" claims** —
each would have surfaced only mid-implementation:

- A `zalo.FindUser(phones)` seam. It does not exist; the only phone→UID API is
  `MatchFriends` (`zalo/service.go:392`), which pulls the whole friend list and
  returns `protocol.FoundUser` (not `zalo.FoundUser`). → **F1**: narrow
  `LookupPhone` adapter; both invitations and reset consume it.
- Reusable `auth.Register` internals to onboard accounts. `auth.Service` has no
  write path to `user_accounts`/`teachers`; that path lives on `teachers.Service`
  (`teachers/service.go:82`). → **F2**: onboarder is `teachers.CreateInCenter`.
- Account-wide refresh revocation in `auth.repository`. It only revokes one
  token/family (`auth/repository.go:22-28`). → **F7**: add `RevokeAllForUser`.
- bcrypt cost living in auth. It's `teachers/service.go:18`. Folded into F2.

| # | Finding (evidence) | Disposition |
|---|--------------------|-------------|
| F1 | No `FindUser` seam; only `MatchFriends` (`zalo/service.go:392`, slow, unofficial) | Accept — `LookupPhone` adapter + bounded timeout (P2, P4) |
| F2 | Onboarder impossible on `auth.Service`; write path is `teachers.Service` (`:82`) | Accept — `teachers.CreateInCenter`/`Reactivate` (P3, P5) |
| F3 | Token-in-path leaks via `middleware.Logger` (only `/public/statements/` redacted) | Accept — `POST /invitations/preview`, token in body (P3) |
| F4 | Disabled→active trusted the token alone | Accept — gate on `WasEverMember` (`centers/repository.go:344-356`) (P3) |
| F5 | `c.ClientIP()` collapses to one bucket behind `SetTrustedProxies(nil)` (`router.go:48-51`) | Accept — key by phone/token; shared limiter + TTL sweeper; apperror 429 (P3, P4) |
| F6 | New `API_WEB_PUBLIC_BASE_URL` would default to localhost in prod | Accept — reuse `Statements.PublicBaseURL` (`config.go:69`) (P1, P2, P4, P7) |
| F7 | Only single token/family revocation exists (`auth/repository.go:22-28`) | Accept — add `RevokeAllForUser` (P3, P4, P5) |
| F8 | Nullable `owner_id` for two-command bootstrap | Dissolved by atomic `create-center` (P1, P5) |
| F9 | Invite creation could block/fail on slow Zalo | Accept — return link before DM; DM after commit under timeout (P2, P4) |
| F10 | Cooldown/supersede only in service logic | Accept — partial unique index `uq_password_reset_active` + concurrent test (P1, P4) |
| F11 | Nullable `owner_id` breaks `(c.owner_id = t.id)` scan (`centers/repository.go:161,205,364`) | Dissolved — `owner_id` stays NOT NULL (P1) |
| F12 | Role-shaping `centers/me` would strip `members` from `PATCH` too | Accept — only `GET /centers/me` reshaped (P3, P6) |
| F13 | e2e assumed a seeded owner/member pair that may not exist | Accept — seeder must produce one; isolate accept specs (P6) |
| F14 | Dead `SetCenterProvisioner` rationale + hand-duplicated CLI wiring + missing `x/term` | Accept — audit/justify-or-delete seam; shared wiring helper; `go get x/term`; `--force` over TTY prompt (P3, P5) |
| F15a | Reset delivery needed a concrete Zalo anchor | Accept — owner-anchored `LookupPhone(owner, member phone)` (P4) |
| F15b | Concurrent invite-create could hit the partial unique index | Accept — on unique-violation reload + return surviving row idempotently (P2) |

No finding reversed a user decision. F8/F15's atomic-CLI suggestion *did* touch
brainstorm decision 7 (three subcommands → two); that reversal was surfaced and
approved separately (see the scope amendment in Overview) before adoption, per
the review-audit-self-decision rule. The Zalo-only delivery (decision 6) and the
48h TTL (decision 8) were treated as fixed; F15a hardens delivery without
reopening them.
