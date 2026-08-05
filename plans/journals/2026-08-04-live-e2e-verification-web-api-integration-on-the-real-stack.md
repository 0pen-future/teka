---
title: "Live e2e verification: web-api integration on the real stack"
date: 2026-08-04
summary: "Verified web ↔ api live on compose; made seeder calendar-independent, hardened e2e specs, fixed /public routing end to end"
---

# Live e2e verification: web-api integration on the real stack

## What happened

Ran the full verification plan for live web ↔ api integration on the real compose stack (postgres → migrate → api → web), covering auth lifecycle, every feature screen against seeded data, and the public parent-statement route.

Key findings and fixes:

- **Public statement routes are root-mounted** (`/public/statements/:token`, outside `/api/v1`). The web `publicApiClient` derived its base URL from `VITE_API_URL` including the `/api/v1` suffix, so parent links 404'd through the dev proxy. Fixed by stripping the suffix in `apps/web/src/lib/api/public-client.ts`, adding a `/public` proxy rule to `apps/web/vite.config.ts`, and a matching guard in `apps/web/nginx.conf`. Docs (`docs/local-development.md`, `docs/deployment.md`) now describe the `/api/*` + `/public/*` routing split.
- **Seeder was calendar-dependent.** On days where a class had no confirmed session in the current month, billing close-out had nothing billable and the e2e suite went red. `apps/api/seeds/seed.go` now computes pending sessions against teacher-local today and backfills one ad-hoc confirmed session per class lacking a current-month confirmed session. Review caught that the backfill predicate must consider only *confirmed* past sessions (not the deliberately-pending ones) — using the full past set left ~126/365 days unbillable for a class whose only current-month session was pending.
- **E2e spec defects:** statement spec keyed notification cards by their first text line, which is the avatar initial — both contacts collided on "C". Now parses the contact name from the message greeting. Also replaced `test.skip` fallbacks with hard `expect(...).toBeTruthy()` so seed/billing regressions fail loudly, made the attendance cancel test date-independent (picks an upcoming session via the date filter), and stabilized the roster card locator.
- **Seed data:** Chị Mai's family got a second child ("Bé Em") so the two-child statement rendering is actually exercised; Chị Hoa stays the collections-spec target whose fully-paid token deliberately 404s.

## Verification

Two full loops on a fresh DB (drop schema → migrate → seed → restart api): Playwright 17/17 with 0 skipped, vitest 93/93, `make test-api-unit` 16 packages ok, `make lint-web` (eslint + prettier + tsc) clean, `go build`/`vet` clean. Tenant isolation spot-checked as the second teacher (empty classes/contacts/pending, no leakage). Independent tester agent reproduced the whole loop from scratch: green. Suite is now green regardless of calendar day: seed always leaves 2 pendings, the attendance spec confirms the newest, and the billing spec conditionally confirms the ≤1 remaining in-period pending.

## Decision

- Backfill counts only held+confirmed sessions toward billability, matching the billing engine's definition.
- Reviewer minors deliberately not applied (loose auth regex, xpath locator brittleness, LoadLocation error swallowing) — noted as acceptable for dev-only seed/e2e code.

## Next steps

- None blocking. Changes committed in four focused commits (seeder, web e2e/integration, docs, plan files) on `main`.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
