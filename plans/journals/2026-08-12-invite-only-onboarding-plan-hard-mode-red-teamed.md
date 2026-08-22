---
title: Invite-only onboarding plan authored and red-teamed (hard mode, TDD)
date: 2026-08-12
summary: "Produced plans/260812-0904-invite-only-onboarding/ (7 phases, TDD) from the accepted brainstorm; a 4-reviewer red-team surfaced 15 evidence-backed defects — all folded in, including 4 false 'already exists' seams and an atomic-CLI scope amendment."
---

# Invite-only onboarding plan authored and red-teamed

## What happened

Ran `/ak-plan` in hard+TDD mode against
`plans/reports/brainstorm-260812-0837-invite-only-onboarding.md` (9 confirmed
decisions). Authored a 7-phase plan under
`plans/260812-0904-invite-only-onboarding/` (schema/token → invitations API →
accept+offboarding → password reset → operator CLI → web surfaces → docs/verify),
each phase carrying Tests-Before/Refactor/Tests-After/Regression-Gate structure.

Four hostile reviewers (security, failure-mode, assumption-audit, scope) ran a
full-verification pass; ~40 raw findings consolidated into 15 evidence-backed
clusters, all adjudicated Accept (user walked each). Two dissolved once bootstrap
became atomic. Plan + phase files now fold in every disposition; `plan.md` carries
a `## Red Team Review` record. Phases hydrated as tasks #1–#7 with the sequential
dependency chain (5 also blocked by 1). User declined the optional validation
interview and chose to implement later (no cook this session).

## Decision

- **Four "already exists" claims in the draft were false** and each would have
  broken only mid-implementation. The draft assumed a `zalo.FindUser(phones)`
  seam (real API is `MatchFriends`, returning `protocol.FoundUser`), reusable
  `auth.Register` onboarding internals (write path is `teachers.Service`, not
  `auth`), account-wide refresh revocation in `auth.repository` (only single
  token/family existed → add `RevokeAllForUser`), and bcrypt cost in auth (it's
  `teachers/service.go:18`). Fix: verify seams against `file:line` before
  asserting reuse in a plan.
- **Atomic `create-center` over nullable `owner_id`** (scope amendment to
  brainstorm decision 7, user-approved). Splitting center/owner into two CLI
  commands forced `centers.owner_id` nullable, which breaks the raw-SQL
  `(c.owner_id = t.id)` is_owner scan and the down-migration. Collapsing to one
  atomic tx keeps the column NOT NULL and the schema untouched — the cheaper fix
  by far. Surfaced the decision-reversal to the user before adopting, per the
  review-audit-self-decision rule.
- **Rate-limit on business identity, not `ClientIP()`.** API sits behind Traefik
  with `SetTrustedProxies(nil)` (`router.go:48-51`), so `c.ClientIP()` collapses
  to one global bucket — an IP-keyed limiter is security theatre. Key on
  phone/token; one shared in-memory limiter + TTL sweeper; add apperror 429.
- **Tokens in request body, never URL path.** `middleware.Logger` records paths
  and redacts only `/public/statements/`, so `GET /invitations/:token` would leak
  live tokens into access logs → `POST /invitations/preview`.
- **Reuse `Statements.PublicBaseURL`** for invite/reset links rather than a new
  `API_WEB_PUBLIC_BASE_URL` that would silently default to localhost in prod.

## Next steps

Implement via `/ak:cook plans/260812-0904-invite-only-onboarding/ --tdd` (or
per-phase), following tasks #1→#7. Phase 3 is the largest and the integration
risk pivot (accept/offboarding rewire + shared rate limiter + teachers/auth
facades); consider splitting the register/join deletion into its own green commit
first. No code written yet.
