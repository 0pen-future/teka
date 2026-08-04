---
phase: 3
title: "Feature Screens Real Data"
status: completed
priority: P1
effort: "3h"
dependencies: [2]
---

# Phase 3: Feature Screens Real Data

## Overview

Walk every authenticated feature screen plus the public statement page
against real seeded data, prove each round-trip (list, create, mutate)
works against the live API, and fix any schema drift the strict zod parsing
surfaces.

## Requirements

- Functional: each screen loads real data; at least one write operation per
  feature succeeds and persists (visible after reload / in Adminer).
- Non-functional: `[]`-vs-`null` list contract holds (API serializes empty
  lists as `[]`); pagination `meta` parses on list screens.

## Architecture

Feature pages → TanStack Query hooks → feature `*-api.ts` →
`apiClient` (authed) or `publicApiClient` (statement) → envelope parse
(`parseData`/`parseList`, zod-strict, throws loudly on drift).

## Related Code Files

- Read/verify: `apps/web/src/features/{dashboard,roster,attendance,billing,collections,statement}/**`
  (API modules + schemas), `apps/api/internal/features/**/dto.go` as the
  wire-shape authority when a mismatch appears.
- Modify: only drifted schema/API files, either side, per
  `docs/api-guidelines.md`.

## Implementation Steps

1. Dashboard: loads with seeded stats; empty-state vs seeded-state sanity.
2. Roster: list classes/students (pagination meta parses); create a class
   and a student; edit; soft-delete; confirm persistence after reload and
   tenant scoping (only Cô Lan's rows — cross-check by logging in as
   Thầy Minh `+84901000002` / `minh-password` in a second pass).
3. Attendance: open a session, one-touch mark, verify saved state
   round-trips.
4. Billing: run close-out flow on seeded month; verify amounts render and
   the mutation persists.
5. Collections: record a payment against a seeded balance; verify status
   transition.
6. Statement (public): generate/fetch a real statement token via the API
   (per plan 06's endpoint), open `/s/:token` logged out → renders real
   statement; expired/unknown token → the designed error states (401/403/
   404 are normal outcomes on `publicApiClient`, no login redirect).
7. Throughout: watch console + network for zod parse errors, `UNKNOWN_ERROR`
   normalizations, or non-envelope responses; each is drift — fix the
   owning side, add/adjust the mirroring test (MSW handler shapes must stay
   truthful to the live API).
8. Close-out validation: web `npm run test`, `make lint-web`,
   `npx tsc -b --noEmit`; optionally `npm run e2e` (Playwright statement
   spec) if it targets the dev server; `make test-api-unit` if any api-side
   fix happened.
9. Stack hygiene: `make dev-down` when verification ends; nothing left
   running.

## Todo

- [x] Dashboard real data
- [x] Roster CRUD + tenant-scoping spot-check
- [x] Attendance round-trip
- [x] Billing close-out round-trip
- [x] Collections payment round-trip
- [x] Public statement with real + invalid token
- [x] Drift fixes tested on owning side
- [x] Full validation suite green
- [x] Stack shut down cleanly

## Success Criteria

- [x] Every screen verified against live data, zero console/zod errors.
- [x] MSW mock shapes confirmed truthful to live responses (or corrected).
- [x] All validation commands green; compose stack stopped.

## Risk Assessment

- **Seed data too thin for a flow** (e.g. no billable month) → extend
  `apps/api/seeds/seed.go` idempotently rather than hand-inserting SQL;
  that change is in-scope drift fixing.
- **MSW mocks drifted from live shapes** → fixing mocks may cascade into
  unit-test updates; keep fixes shape-only, never weaken assertions.
- **Cross-tenant leakage observed** → stop, report immediately as a
  security finding; do not continue the walkthrough until triaged.
