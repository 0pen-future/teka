---
phase: 2
title: "Auth Flow Live Verification"
status: completed
priority: P1
effort: "3h"
dependencies: [1]
---

# Phase 2: Auth Flow Live Verification

## Overview

Verify the full auth lifecycle in a real browser against the live API:
login UX, silent session restore, refresh rotation, logout revocation, and
error surfacing — then fix any drift the run exposes.

## Requirements

- Functional: every auth path below behaves as designed with zero console
  errors and zero zod parse failures.
- Non-functional: no leftover dev-only session seeding in
  `apps/web/src/app/main.tsx` (was removed after the UI-preview session —
  confirm before testing, or every result is invalid).

## Architecture

Browser → compose web (`localhost:5173`, `VITE_API_URL=/api/v1`) → Vite
proxy → api. Access token lives in the zustand store only; refresh token in
the httpOnly cookie scoped to `/api/v1/auth`. `SessionRestore` runs one
silent `/auth/refresh` on full load except on `/s/…`. Interceptors:
attach bearer → on 401 retry-with-newer-token or single-flight refresh →
normalize to `ApiError`.

## Related Code Files

- Read/verify: `apps/web/src/app/main.tsx` (no dev seed),
  `apps/web/src/features/auth/**`,
  `apps/web/src/lib/api/{interceptors,auth-bridge,envelope,errors}.ts`.
- Modify: only files proven drifted by a live failure.

## Implementation Steps

1. Precondition: grep `main.tsx` for any dev-session seeding — must be
   absent.
2. Login happy path: `0901000001` / `lan-password` → dashboard; network tab
   shows one `POST /auth/login`, envelope parsed, no console errors.
3. Login error paths:
   - wrong password → single generic error (401 UNAUTHORIZED), form stays
     usable;
   - invalid phone format → client-side zod message (no request fired);
   - server-side 422 (e.g. bypass client validation via short password on
     register) → per-field messages from `error.fields` render on the form.
4. Session restore: full reload while logged in → spinner →
   `POST /auth/refresh` 200 → dashboard without bouncing to `/login`.
   Each reload rotates the refresh cookie (new `Set-Cookie` value).
5. Refresh-dead path: logout (`POST /auth/logout`) → store cleared →
   `/login`; reload → `/auth/refresh` 401 → stays on `/login`, exactly one
   refresh attempt (gate closed, no loop).
6. Public-route skip: open `/s/any-token` logged out → **no**
   `/auth/refresh` request fires; page renders the statement feature's own
   error state for an unknown token.
7. 401-mid-session path: with the app open, wait for access-token expiry
   (dev TTL 15 min — or temporarily lower `API_JWT_ACCESS_TTL` in root
   `.env` and restart api) → next API call 401s → single silent refresh →
   original request retried successfully. Restore TTL afterwards.
8. Fix drift: any zod parse failure, wrong error rendering, redirect loop,
   or cookie misbehavior gets a minimal fix on the owning side, mirrored in
   the existing test layer (vitest+MSW for web logic; Go tests for api).

## Todo

- [x] main.tsx clean of dev seeding
- [x] Login happy path verified
- [x] 401 / client-zod / server-422 error UX verified
- [x] Reload restores session; cookie rotates
- [x] Logout revokes; no refresh loop after death
- [x] `/s/:token` fires no refresh
- [x] Expired-token mid-session silent refresh verified
- [x] Drift fixes (if any) covered by tests

## Success Criteria

- [x] All eight todo items check off with zero console errors.
- [x] Web unit tests (`npm run test`) still green after any fixes.

## Risk Assessment

- **Safari-specific cookie behavior** → verify in Chrome first (design
  already accounts for Safari's Secure-on-localhost quirk by not setting
  `Secure` in dev).
- **TTL tampering forgotten** → restore `API_JWT_ACCESS_TTL` before closing
  the phase; never commit a lowered TTL.
- **Flaky manual observation** → assert via browser network inspection, not
  UI impressions; capture request/response pairs for any drift report.
