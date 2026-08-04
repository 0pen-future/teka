---
title: "Web API E2E Auth Verification"
description: "Run the real stack end-to-end and verify web ↔ api integration live — auth first, then every feature screen — fixing any contract drift found"
status: completed
priority: P1
effort: "1d"
tags: [web, api, auth, integration, verification]
created: 2026-08-04
---

# Web API E2E Auth Verification

## Overview

The web app's API layer (envelope parsing, `ApiError` normalization,
single-flight token refresh, auth endpoints) and the Go backend are both
complete and were built against the same contract
(`docs/api-guidelines.md`). What has never happened is a live end-to-end
run: web unit tests use MSW mocks, and prior sessions previewed UI with a
fake token (every real call 401'd). This plan brings up the real compose
stack, verifies the auth lifecycle live in the browser, then walks every
feature screen against seeded data — fixing any contract drift the live run
exposes.

**This is a verification-and-fix plan, not a build plan.** No new
abstractions; code changes happen only where the live run proves drift.

## Context

- Contract authority: `docs/api-guidelines.md` (envelope, error codes, auth
  design), `docs/local-development.md` (stack walkthrough).
- Web API layer: `apps/web/src/lib/api/{client,envelope,errors,interceptors,auth-bridge,public-client}.ts`.
- Auth feature: `apps/web/src/features/auth/` (login/register pages,
  `SessionRestore`, `ProtectedRoute`, zustand store).
- Backend: `apps/api` (Gin), all six API plans completed; seeds in
  `apps/api/seeds/seed.go` (teacher `+84901000001` / `lan-password`).
- Dev topology: compose sets `VITE_API_URL=/api/v1`, Vite proxies `/api` →
  `http://api:8080` (same-origin: no CORS, no SameSite friction).

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Real stack runs healthy via `make dev` + `make seed` | P1 |
| 2 | Auth lifecycle verified live: login, restore, refresh rotation, logout, error shapes | P1 |
| 3 | Every feature screen renders real seeded data without schema/console errors | P1 |
| 4 | Any discovered drift fixed on the owning side, tests stay green | P1 |

## Non-Goals

- No new features, endpoints, or abstractions.
- No re-doing work from completed plans 01–08.
- No CORS/cross-origin hardening (dev is same-origin by design; host-mode
  `.env.development` absolute URL stays as-is).
- No token-denylist / instant-revocation work (documented accepted trade-off).

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Phase 1: Stack Up and Contract Smoke Test](./phase-01-start.md) | Done |
| 2 | [Phase 2: Auth Flow Live Verification](./phase-02-auth-flow-live-verification.md) | Done |
| 3 | [Phase 3: Feature Screens Real Data](./phase-03-feature-screens-real-data.md) | Done |

Dependencies: 2 blocks on 1; 3 blocks on 2.

## Success Criteria

- [x] `docker compose ps` shows postgres/migrate/api/web healthy; `/healthz`, `/readyz` return 200.
- [x] curl smoke: login success envelope + refresh `Set-Cookie`; 401/409/422 error envelopes match the guidelines table.
- [x] Browser: seed login lands on dashboard; full reload silently restores session; logout revokes (subsequent refresh 401s → `/login`).
- [x] `/s/:token` public route fires no `/auth/refresh`.
- [x] All feature screens render seeded data with zero zod parse or console errors.
- [x] `make lint-web` and web `npm run test` pass after any drift fixes.

## Open Questions

None.

<!-- slug: web-api-e2e-auth-verification -->
