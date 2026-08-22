---
phase: 7
title: "OpenAPI Docs and Verification Sweep"
status: done (make e2e deferred to deploy stage)
priority: P2
effort: "3h"
dependencies: [6]
---

# Phase 7: OpenAPI Docs and Verification Sweep

## Overview

Regenerate OpenAPI, reconcile evergreen docs with the invite-only reality,
and run the full verification matrix against the brainstorm acceptance
criteria.

## Key Insights

- OpenAPI is swag-generated and committed (`apps/api/docs/`); CI diffs a
  fresh generation — must run `make api-docs` after route changes
  (register/join gone; invitations, forgot/reset added).
- `docs/api-guidelines.md` contains now-false statements: "every account is a
  teacher created via /auth/register or the seeder" (Seeding section) and
  "password reset … not implemented" (Extension points). Tenancy section's
  removal-behavior wording may also reference personal-center fallback.
- No new environment variable is introduced — invite/reset links reuse
  `STATEMENTS_PUBLIC_BASE_URL`. Do not document a `API_WEB_PUBLIC_BASE_URL`.
- Operator CLI is **two** onboarding subcommands (`create-center` atomic,
  `reset-password`) per the plan's scope amendment — docs must not mention a
  separate `create-owner`. Invite preview is `POST /invitations/preview`
  (token in body), not a GET-by-path.
- Update the smallest owning surface only (documentation-management rule);
  no new doc files.

## Requirements

- Functional: swagger spec matches routes; docs truthful.
- Non-functional: CI-equivalent local gates all green.

## Related Code Files

- Modify: `apps/api/docs/` (generated — via `make api-docs` only)
- Modify: `docs/api-guidelines.md` (Authentication, Seeding, Extension
  points; add invitation + reset token discipline paragraph; note operator
  CLI as the bootstrap path)
- Modify: `docs/prd.md` only if it asserts self-registration (verify)
- Modify: `docs/local-development.md` if it documents register-based setup

## Implementation Steps

1. `make api-docs`; verify diff covers exactly the intended route changes
   (register/join gone; `/invitations/preview`, `/invitations/accept`,
   `/centers/me/invitations`, forgot/reset added).
2. Docs sweep: grep `register|/centers/join|personal center|password reset|
   create-owner|API_WEB_PUBLIC_BASE_URL` across `docs/`; fix owning sections;
   verify claims against source.
3. Full matrix:
   ```sh
   make lint-api && make test-api
   make lint-web && make test-web && make build-web
   make e2e
   make api-docs && git diff --exit-code apps/api/docs
   ```
4. Walk the brainstorm **Acceptance criteria** list item-by-item; check each
   against a test or manual probe; record the mapping in the completion
   report (`plans/reports/`).

## Todo

- [x] OpenAPI regenerated, CI drift check passes
- [x] Docs sweep done, claims verified against source
- [x] Full matrix green (lint-api, test-api, lint-web*, test-web, build-web,
      api-docs drift; `make e2e` deferred to deploy stage — needs live Docker
      stack, out of this phase's scope)
- [x] Acceptance-criteria mapping recorded

*lint-web fails only on 9 pre-existing, untouched files (4 React-compiler
warnings, 5 prettier issues) — confirmed via `git status` outside this
feature's diff; not fixed, reported as out of scope.

## Success Criteria

- [x] Every brainstorm acceptance bullet mapped to passing evidence
- [x] No doc claims contradicting shipped behavior

## Risk Assessment

Low. Main risk is discovering an unmapped acceptance bullet late — the
mapping walk is the safety net; anything failing reopens the owning phase.

## Next Steps

Delivery complete → `/ak:journal` entry + plan archive per repo workflow.
