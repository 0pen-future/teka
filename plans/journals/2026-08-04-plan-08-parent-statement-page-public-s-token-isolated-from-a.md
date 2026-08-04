---
title: "Plan 08 parent statement page: public /s/:token, isolated from auth, DTO-driven"
date: 2026-08-04
summary: "Public tokenized parent statement page shipped: auth-isolated route/client, real PublicStatement DTO, partial-payment reconciliation, all gates green."
---

# Plan 08 parent statement page: public /s/:token, isolated from auth, DTO-driven

## What happened

Built and finalized the parent-facing statement page (`/s/:token`) — the only
screen a non-teacher sees. Four hard properties drove the design: public and
unauthenticated, phone-first low bundle, always-live server data, and
zero-leak on a bad token.

Key implementation decisions:

- **Auth isolation.** A dedicated `publicApiClient` (axios, `withCredentials:
  false`, no request interceptors) so the 401-refresh logic never fires on the
  public route. `SessionRestore` short-circuits on the `/s/` prefix (hardcoded,
  with a sync comment) so no `/auth/refresh` request goes out for a parent. An
  eslint `no-restricted-imports` boundary forbids `features/statement` from
  importing `@/features/auth` or `@/lib/api/client`.
- **DTO-driven, not plan-assumed.** The plan assumed a payload shape that the
  real Go `PublicStatement` DTO diverges from in eight ways (single neutral 404
  for every failure, `period` as an "MM/YYYY" string, a unified per-class
  `sessions[]` list, grand total = `totals.total_due`, QR carrying only
  image_url/amount/note with no bank fields, adjustments without reason,
  `display_note`, server-set robots headers). Scouted the DTO first, recorded
  all eight in adr.md, and built the zod schema against the real contract.
- **Partial-payment reconciliation.** The one Medium code-review finding: the
  QR encodes `outstanding` while the headline shows `total_due`, so a
  partially-paid family saw a large total beside a QR asking for less. Added a
  server-values-only "Đã thanh toán / Còn lại" block in `GrandTotal` (no client
  arithmetic) so the two numbers reconcile.

## Gates

- typecheck: 0 errors
- vitest (statement feature): 13/13 pass (added a partial-payment case)
- raw-hex gate: clean
- code review: no BLOCKER; the single Medium was addressed
- e2e `statement.spec.ts`: type-checks and lints; not run end-to-end (no live
  backend, tokens are server-derived from a secret)

## Divergences recorded in adr.md

Eight payload-shape divergences, plus the bundle-analyze target being
unmeasurable here (the build command is blocked by an environment hook — design
still follows the target structurally: route lazy-load, no dashboard code in the
public layout, separate interceptor-free client; re-measure in CI before prod).

## Next steps

This closes the "complete all pending plans" chain (plans 01–08 + design-system
foundation all completed). Before production: measure the route chunk against
the 30 KB gzip target via `build:analyze` in CI, and run `statement.spec.ts`
against a live backend with a real token.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
