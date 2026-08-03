---
title: "08 Web Parent Statement Page"
description: "Public tokenized statement page at /s/:token: mobile-first per-child fee breakdown with session dates, family grand total, transfer QR, and a neutral error page for bad or expired tokens."
status: pending
priority: P2
effort: "2.5d"
branch: main
tags: [web, react, typescript, public, mobile, statements, parents, design-system]
created: 2026-08-03
blockedBy: [260803-2244-06-api-statements-and-notifications, 260803-2325-web-design-system-foundation]
---

# 08 Web Parent Statement Page

## Overview

The parent-facing half of PRD R5. The Zalo message a teacher sends (plan 07,
phase 4) carries the summary; this page is the layer-2 detail behind the link.
It is the only screen in the product a non-teacher ever sees.

Four properties define it, and each rules out a whole class of implementation:

1. **Public and unauthenticated.** No login, no app install (PRD §3 non-goals).
   The token in the URL is the entire credential, so this route must not touch
   the auth store, the refresh cookie, or the authenticated axios instance.
2. **Opened roughly twice a month, on a phone, often on mobile data.** Bundle
   weight is a product requirement, not an optimization. A parent who waits for
   a heavy app shell just to read a number will ask the teacher instead — which
   is the exact behavior the product exists to remove.
3. **Always live.** PRD R5 AC: a parent reopening an old link after the teacher
   fixed attendance must see the corrected figures. The page therefore renders
   server data on every load with no client-side caching beyond the request.
4. **Leaks nothing on a bad token.** An invalid, revoked, or expired token gets
   a neutral page that names no student, no teacher, and no reason (PRD R5 AC).

Vietnamese domain terms used on this page: **nợ cũ** (opening balance carried
from the previous period), **buổi** (one class session), **chốt sổ** (the
period close that produced these numbers).

## Design Source — 100% Adherence

The page's design is fixed by the **parent preview modal** in
`So Lop - Prototype.dc.html` ("Học Vui Mỗi Ngày" DS, direction "Dịu Mát") from
the imported Claude Design project
(`claude.ai/design/p/4a7e6c77-0971-44fb-9766-1b6429e8b126`). Plan
`260803-2325-web-design-system-foundation` provides tokens, self-hosted fonts,
and the `components/hv` kit; this page consumes them under the same rules as
plan 07 (tokens only, no raw hex).

The prototype renders the parent view as a 392px-wide phone sheet on
`--cream-100`: a `--mint-400` header with the family name, per-child white
cards with wrapping session-date chips and the fee formula, a `--surface-dark`
grand-total block with a `--sun-400` amount and the QR, and the expiry note.
Phase 2's Design Spec transcribes the exact recipe. The bundle-weight target
is unaffected: the DS arrives as CSS custom properties and the shared kit,
already part of the app-level CSS.

## Scope

In scope:

- A public route `/s/:token` in `apps/web` mounted outside the authenticated
  layout and outside `ProtectedRoute`.
- A minimal public layout with no nav, no theme toggle, no auth context.
- A dedicated unauthenticated API client so the auth interceptors never fire on
  this route.
- Mobile-first statement rendering: per-child sections, per-session
  present/absent dates, the fee formula, nợ cũ, and the family grand total.
- Transfer QR display plus copyable bank details as a fallback.
- Neutral error page for bad, revoked, or expired tokens.
- `noindex, nofollow` so the link never reaches a search engine.
- Playwright coverage for valid token, bad token, and a multi-child family.

Out of scope:

- Any client-side analytics. The open-rate metric (PRD §7, "tỉ lệ phụ huynh mở
  link" 50%/75%) is recorded server-side when the statement endpoint is hit —
  `statements.first_viewed_at`, `last_viewed_at`, `view_count`
  (`docs/schema_design.sql:417-419`). The page needs no tracking code at all.
- Online payment. V1 shows a QR and the teacher confirms manually (PRD §3).
- Parent login, message history, read receipts (PRD P1/P2).
- Editing anything. The page is strictly read-only.

## Phases

| # | Phase | Effort | Depends on | Status |
|---|-------|--------|------------|--------|
| 1 | [Public route, data layer, neutral error page](./phase-01-public-route-and-data-layer.md) | 1d | — | Pending |
| 2 | [Statement content, QR, mobile layout, tests](./phase-02-statement-view-and-tests.md) | 1.5d | 1 | Pending |

Both phases own files under `apps/web/src/features/statement` and
`apps/web/src/layouts/public-layout.tsx`; they are sequential, not parallel.
The only file shared with plan 07 is `apps/web/src/app/router.tsx` — this plan
adds one top-level route entry outside the protected subtree.

## Key Screens → PRD Mapping

| Route | State | PRD source |
|---|---|---|
| `/s/:token` | Loading | — |
| `/s/:token` | Statement for a one-child family | R5 layer 2; parent story 1 and 2 |
| `/s/:token` | Statement for a multi-child family: a section per child, one grand total | R5 AC 2; §5 "một phụ huynh nhiều con" |
| `/s/:token` | Invalid / revoked / expired token | R5 AC 5 |
| `/s/:token` | Server error | derived from R5 AC 5 (same neutral treatment) |

## Acceptance Criteria

From PRD R5:

- [ ] Given a parent with 2 children, when they open the link, then each child
      has their own section and the total appears at the end (R5 AC 2).
- [ ] Given the teacher edited attendance after sending, when the parent reopens
      the old link, then the updated figures are shown (R5 AC 4).
- [ ] Given a wrong or expired token, when it is opened, then a neutral error
      page appears revealing no student information (R5 AC 5).
- [ ] Given a parent on a phone, when the page loads, then all information is
      readable with no horizontal scrolling (R5 AC 6).
- [ ] Given a child's section, when the parent reads it, then the attended and
      missed session dates and the fee formula are visible (parent story 2).
- [ ] Given the total amount, when the parent wants to pay, then a transfer QR
      for that amount is on the same screen (parent story 3).
- [ ] Given any statement page load, when it renders, then the document carries
      `noindex, nofollow` (R5 "not indexed by search engines").

## Non-Functional Targets

| Target | Value | Why |
|---|---|---|
| Route JS payload (gzipped, excluding React runtime) | < 30 KB | Parents open this once or twice a month on mobile data. |
| No auth code on the route's critical path | 0 imports from `features/auth` | Public route must not pull the auth store or interceptors. |
| Data freshness | no cache; refetch on mount | R5 AC 4 requires live figures. |
| Robots | `noindex, nofollow` meta on this route | R5. |

Measure with `npm --prefix apps/web run build:analyze`
(`apps/web/package.json:10`), which writes a treemap to `stats.html`.

## Test Strategy

| Level | Tool | Covers |
|---|---|---|
| Unit / component | vitest + Testing Library + msw | Rendering per-child sections, totals arithmetic display, absent-date rendering, error-state neutrality |
| E2E | playwright (`apps/web/playwright.config.ts:8`) | Valid token, bad token, multi-child family, no horizontal scroll at 375px |

## Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| The shared axios instance's 401 refresh logic fires on the public route and redirects a parent to `/login` | High | High | A separate unauthenticated client with no interceptors and `withCredentials: false`; phase 1 owns this and a test asserts no auth import. |
| Error page leaks whether a token merely expired versus never existed | Medium | High | One error component for every non-200; no status-specific copy, no student data in any branch. |
| Bundle regresses as the app grows | Medium | Medium | Route-level lazy chunk plus a public layout that imports no dashboard code; check `stats.html` before merge. |
| Long class names or 20+ session dates break the phone layout | Medium | Medium | Wrapping date chips instead of a table; explicit e2e assertion that `scrollWidth <= clientWidth`. |
| QR rendering approach undecided | Medium | Medium | Prefer a server-supplied QR image URL over a client QR library — see open question 1. |

## Open Questions

1. **QR delivery.** Preferred: the API returns a ready QR image URL (for example
   a VietQR image endpoint) plus the raw bank details, and the page renders an
   `<img>`. This keeps the client bundle free of a QR library and keeps the
   bank-account data server-side. Confirm with plan 06 whether the statement
   payload carries `qr_image_url`; if it does not, phase 2 falls back to a
   small client-side QR renderer and the bundle target rises by roughly 10 KB.
2. **Expiry semantics.** `statements.expires_at` plus `revoked_at`
   (`docs/schema_design.sql:414,420`) and the PRD rule "expires once the period
   is paid or after 90 days". Does the API return 404 or 410 for an expired
   token? The page treats every non-200 identically, so this affects only
   logging, but it should be settled.
3. **Cancelled sessions.** `class_sessions.status = 'cancelled'` keeps a
   `cancel_reason` and the schema comment says it is shown to parents
   (`docs/schema_design.sql:207`). Should cancelled dates appear in the child's
   session list as "buổi huỷ, không tính tiền"? Recommended yes — it explains a
   gap in the dates a parent can otherwise see on the calendar. Confirm the
   payload includes them.
4. **Statement content parity.** The message text (plan 07, phase 4) and this
   page must agree on per-child session counts and the total. Both should read
   from the same server-side statement builder rather than two renderers.
5. **Adjustment reasons.** `invoice_adjustments.reason` is free text written by
   the teacher (`docs/schema_design.sql:343`) and may contain internal notes.
   Does the public payload include it? Default assumption for phase 2: the page
   shows the adjustment amount only, never the reason, until plan 06 states
   otherwise.

<!-- slug: 08-web-parent-statement-page -->
