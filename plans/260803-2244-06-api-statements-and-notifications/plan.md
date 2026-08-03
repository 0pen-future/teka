---
title: "06 API Statements and Notifications"
description: "Per-contact statements with tokenised public links rendering live data, the Zalo message content builder, and one-action bulk send with reminders."
status: pending
priority: P1
effort: "17h"
branch: HEAD
tags: [api, go, statements, notifications, public-endpoint, security]
created: 2026-08-03
blockedBy: [260803-2244-05-api-payments-and-collections]
blocks: [260803-2244-07-web-teacher-app, 260803-2244-08-web-parent-statement-page]
---

# 06 API Statements and Notifications

## Overview

Implements PRD **R5** (the two-layer fee report) and **R6** (one-action bulk
send). This is where the system stops being an internal ledger and starts
talking to parents.

The decision R5 makes and this plan must honour throughout: **the unit of a
statement is the contact, not the student.** One parent, one message, one link,
one total — regardless of how many children in how many classes
(`docs/schema_design.sql:403`).

Two layers, deliberately both:

- **Layer 1 — the Zalo message.** Per child: name, sessions attended, sessions
  absent, amount. Then old debt and the family grand total. A parent who never
  taps the link still knows what to pay.
- **Layer 2 — the link.** Per child, per class, every session with its date,
  the formula, the old-debt line, the family total, and a bank-transfer QR. The
  link renders **live** data, so a parent opening an old link after the teacher
  fixed an attendance record sees corrected numbers (R5 acceptance).

Two feature packages: `statements` (generation, public rendering, message
building) and `notifications` (queueing, bulk send, provider abstraction).

## Scope

In scope:

- One `statements` row per contact per period, unique where not soft-deleted
  (`docs/schema_design.sql:428`), with `total_due` snapshot and expiry.
- Token issuance and hash-only storage (D10).
- Public unauthenticated statement endpoint rendering live data.
- View tracking: `first_viewed_at`, `last_viewed_at`, `view_count`
  (`docs/schema_design.sql:417-419`).
- Expiry and revocation, with a neutral 404 for anything invalid.
- Layer-1 message content builder with a length ceiling.
- `notifications` rows per send, channel per D9, `queued → sent/delivered/failed`
  (`docs/schema_design.sql:440-441`).
- Bulk send: one action generates and queues every contact of the period (R6).
- Reminders: `purpose='reminder'` re-sends, one per contact covering all
  children (R7 acceptance).

Non-goals:

- Read receipts — P2 (`docs/schema_design.sql:601` reserves `acknowledged_at`).
- Automatic reminders after X days — P1.
- Payment through the link — P1; V1 shows a QR and the teacher confirms.
- Any schema change (D1). No new migration files. In particular, **no column or
  table is added for teacher bank details** — see OQ-1.

## Phases

| # | Phase | Effort | Depends on | Status |
|---|-------|--------|-----------|--------|
| 1 | [Statement generation and tokens](./phase-01-statement-generation-and-tokens.md) | 5h | — | Pending |
| 2 | [Public statement endpoint](./phase-02-public-statement-endpoint.md) | 6h | 1 | Pending |
| 3 | [Message builder and notifications](./phase-03-message-builder-and-notifications.md) | 6h | 2 | Pending |

## Key decisions

- **D9 (V1 default channel = `zalo_manual`).** PRD Q1 is still blocking and
  unanswered: ZNS template approval, per-message cost, and industry eligibility
  are all unverified. V1 therefore generates the complete per-contact message
  text and the teacher copies and pastes it (individually or as a bulk copy).
  ZNS sits behind a `Sender` interface as channel `zalo_zns`, ready to switch on
  once Q1 is answered. `sms` is in the CHECK list
  (`docs/schema_design.sql:437`) but has no implementation in V1.
- **D10 (token handling).** 256-bit token, URL-safe, no auth. The database
  stores only `SHA-256(token)` in `token_hash BYTEA`
  (`docs/schema_design.sql:413`); the public route hashes what it is given and
  looks that up. `X-Robots-Tag: noindex, nofollow, noarchive` and
  `Cache-Control: no-store` on every public response. Anything invalid, expired,
  or revoked returns an identical neutral 404 that names no student and no
  teacher (R5 acceptance).
- **D10-refinement — token derivation.** A pure-random token cannot be
  reconstructed for a re-send, because only its hash is stored, and rotating it
  on every reminder would break links parents already have. The token is
  therefore **derived**: `token = base64url(HMAC-SHA256(K, statement.id))`,
  where `K` is a 256-bit key from application config, never in the database.
  Properties preserved: 256 bits of unguessable entropy, no plaintext token in
  the database, hash-only lookup. Property gained: any send or re-send can
  recompute the same link. Trade-off: rotating `K` invalidates every live link
  at once. **Flagged for lead confirmation — see OQ-2.**
- **Statements are generated on demand after close, not by a hook inside the
  close transaction.** R6 asks that one teacher action produce every message;
  it does not require generation to happen during close. Generating lazily on
  the first statements or bulk-send call keeps plan 06 from editing plan 04's
  files, keeps the close transaction short, and lets a teacher review and adjust
  before anything is sent. Generation refuses unless the period is `closed`.
- **The link renders live data; the invoice never moves.** Sessions and
  attendance come from live tables. Money comes from the issued invoice snapshot
  plus its adjustments plus payments. Where a post-close correction has been
  carried forward (plan 04, D7), the page shows it as an explicit
  "chuyển sang kỳ sau" (carried to the next period) line rather than silently
  disagreeing with the charged amount. This satisfies R5's "số liệu đã cập nhật"
  and schema note (k) (`docs/schema_design.sql:533`) at the same time.
- **The QR is delivered as a server-rendered image URL, not raw bank fields.**
  Requested by the web plans (07/08) so no QR-generation library enters the
  public parent bundle, and correct on its own merits: the payload format is a
  banking-standard detail the frontend should not have to track, and a parent on
  a slow phone should not run a canvas render to see it. The public payload
  therefore carries `qr.image_url` pointing at a token-scoped image route on
  this API. Raw bank fields are never sent to the parent page.
- **Adjustment `reason` is teacher-facing only; the parent payload carries the
  amount alone.** `invoice_adjustments.reason` (`docs/schema_design.sql:349`)
  is free-form teacher text — under plan 04 it is sometimes generated ("sửa điểm
  danh buổi 12/8"), sometimes typed in a hurry, and it may name internal
  reasoning a parent should not read. The public statement shows the adjustment
  as a signed amount with a neutral label; the teacher sees the reason in the
  authenticated invoice view. This is the web plans' default assumption and it
  is the right one.
- **Expiry.** `expires_at = created_at + 90 days` at issue. The public endpoint
  additionally treats a fully-paid period as expired, per R5's "hết hiệu lực sau
  khi kỳ được thanh toán xong hoặc sau 90 ngày". See OQ-3 — this hides the
  receipt from a parent the instant they pay.
- **D4/D5** as in plans 04 and 05: teacher-scoped queries, `BIGINT` money,
  `deleted_at IS NULL` on soft-delete tables (`statements` and `notifications`
  both have it).

## Acceptance criteria

From PRD R5, R6 and R7.

- [ ] R5: a parent with two children in two classes receives exactly **one**
      message with **one** total.
- [ ] R5: opening the link shows each child's section separately with a family
      total at the end.
- [ ] R5: a parent who reads only the message still knows the amount due and
      each child's session count.
- [ ] R5: after the teacher corrects attendance, an already-sent link shows the
      updated figures.
- [ ] R5: an invalid, expired, or revoked token returns a neutral error page
      that leaks no student, contact, or teacher information.
- [ ] R5: the page fits a phone screen with no horizontal scrolling (API side:
      one request returns everything the page needs, no N+1 round trips).
- [ ] R6: one action generates per-parent content and queues every send for the
      period.
- [ ] R6: the layer-1 message fits within the configured template length ceiling.
- [ ] R7: a parent with two children both in debt receives a single reminder.
- [ ] View counters increment on each open, and `first_viewed_at` is set once.
- [ ] Contacts whose only invoice was voided at close receive no statement and
      no notification (PRD §5: class with no sessions).

## Open questions

- **OQ-1 (blocking for the QR requirement).** The bank-transfer QR in R5 needs
  the teacher's bank code, account number, and account name. **The schema has no
  place for them** — no column on `teachers`, and D1 forbids adding a column or
  a table. Three options, in the order recommended:
  1. **V1 interim (recommended):** the teacher's bank details live in
     application configuration (per-deployment environment variables during the
     3-teacher pilot). The statement renders a VietQR payload only when the
     config is present; otherwise the QR block is omitted and the page shows the
     amount and a plain instruction. No schema change, no fake data.
  2. Omit the QR entirely in V1 and ship it with the schema revision.
  3. Revise the schema to add bank fields to `teachers` — **out of V1 scope**
     because the schema is frozen by D1; this is the right permanent answer for
     V1.1.
  Option 1 does not scale past the pilot: a real multi-teacher deployment cannot
  hold per-teacher bank details in environment variables. **A schema revision is
  required before general availability, and that decision belongs to the lead.**
  This plan builds option 1 behind an interface so option 3 replaces one
  function.
  **Contract requirement independent of the resolution:** whichever option wins,
  the public payload exposes a ready-to-render `qr.image_url` served by this API
  and never raw bank fields. Requested by web plan 08 to keep a QR library out
  of the parent bundle. So resolving OQ-1 changes where `BankConfig` is loaded
  from — it does not change the API contract the web plans code against.
- **OQ-2.** The HMAC-derived token (D10-refinement) departs from D10's literal
  "crypto-random" wording in order to make reminders possible without rotating
  live links. Confirm, or accept that a reminder issues a new token and
  invalidates the parent's existing link.
- **OQ-3.** Expiring the link the moment the balance reaches zero follows R5
  literally but denies the parent the confirmation they just paid for. A short
  grace period (7 days after the final payment, showing a "đã thanh toán đủ"
  view) would fix it at the cost of complexity. Not built; needs a product call.
- **OQ-4 (depends on Q1).** The exact ZNS template length limit is unverified,
  so the builder enforces a configurable ceiling (default 1000 characters) with
  a documented degradation: collapse per-child lines into a single summary line
  and keep the link. R6 states that if ZNS cannot carry the content, the channel
  changes rather than the content — that decision needs Q1 answered first.
- **OQ-5.** Statement `total_due` is a snapshot at issue
  (`docs/schema_design.sql:415`) while the page renders live figures. They will
  differ after a post-close adjustment. The page shows the live number; the
  snapshot is kept for audit ("what did we tell them at the time"). Confirm that
  is the intended reading of the column.

<!-- slug: 06-api-statements-and-notifications -->
