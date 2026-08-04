---
phase: 1
title: "Stack Up and Contract Smoke Test"
status: completed
priority: P1
effort: "2h"
dependencies: []
---

# Phase 1: Stack Up and Contract Smoke Test

## Overview

Bring the full compose stack up healthy, seed the database, and prove the
live API's response/error envelope matches `docs/api-guidelines.md` with
curl — before any browser work.

## Requirements

- Functional: stack healthy, seeded; auth endpoints answer with the exact
  envelope/error shapes from the guidelines.
- Non-functional: no orphaned host processes holding compose ports; no
  changes to `.env` values beyond what `make setup` scaffolds.

## Architecture

Compose startup is health-gated: postgres (healthy) → migrate (exit 0) →
api (healthy) → web. Browser traffic is same-origin: web dev server proxies
`/api` → `http://api:8080`. Host port 8080 stays available for curl.

## Related Code Files

- Read: `docs/local-development.md`, `apps/api/seeds/seed.go`, root `Makefile`, root `.env` / `.env.example`.
- Modify: none expected (report, don't patch, if the stack itself is broken).

## Implementation Steps

1. Port hygiene (process-management rule): verify 5173/8080/5432/8081 are
   free (`lsof -i :PORT`). Stop stale owners only if we started them.
2. `make setup` if root `.env` is missing; then `make dev` (attached or via
   background task) and wait for health gates; `docker compose ps` all
   healthy.
3. `make seed` once; confirm idempotency note (re-run safe, keyed by phone).
4. Probe: `curl -i localhost:8080/healthz` and `/readyz` → 200, outside the
   envelope (by design).
5. Contract smoke with curl against `http://localhost:8080/api/v1`:
   - `POST /auth/login` `{phone:"0901000001",password:"lan-password"}` →
     200 `{success:true,data:{access_token,token_type,expires_in,teacher}}`
     plus `Set-Cookie` refresh: httpOnly, `SameSite=Lax`, path
     `/api/v1/auth`, no `Secure` in dev.
   - Same login with `+84901000001` → same account (phone normalization).
   - Wrong password → 401 `{success:false,error:{code:"UNAUTHORIZED"}}`
     (identical shape to unknown-phone: timing/enumeration defense).
   - Malformed phone → 422 `VALIDATION_ERROR` with `fields.phone`.
   - `POST /auth/register` with a seeded phone → 409 `CONFLICT`.
   - `GET /me` with the access token → 200 teacher profile; without →
     401.
6. Record any shape mismatch verbatim in the phase Todo before fixing
   anything (fixes belong to the owning side and later phases unless the
   stack is unusable).

## Todo

- [x] Ports free / stale owners stopped
- [x] `make dev` healthy end-to-end
- [x] `make seed` applied
- [x] Health probes 200
- [x] Login success envelope + cookie flags verified
- [x] 401 / 409 / 422 error envelopes verified
- [x] `/me` auth gating verified

## Success Criteria

- [x] All compose services healthy; seeds present.
- [x] Every curl assertion above matches `docs/api-guidelines.md` exactly, or the mismatch is recorded as drift input for Phase 2/3.

## Risk Assessment

- **Docker not running / ports busy** → start Docker Desktop; free ports
  per process-management rules (never kill processes we don't own).
- **Migrate step fails** → `make dev-logs`; a schema problem here blocks
  the plan and gets reported, not hot-patched.
- **Seed refuses** → only refuses `API_ENV=production`; dev default is fine.
