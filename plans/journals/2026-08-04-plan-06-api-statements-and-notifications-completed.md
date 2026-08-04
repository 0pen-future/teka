---
title: Plan 06 API statements and notifications completed
date: 2026-08-04
summary: "Per-contact statements with tokenised public links, VietQR image route, message builder, and one-action bulk send"
---

# Plan 06 API statements and notifications completed

## What happened

Delivered all three phases of the statements + notifications feature set.

- **Statements feature**: one `statements` row per contact per period (upsert on
  contact_id+period_id where not soft-deleted, never rotating token_hash).
  Tokens are HMAC-SHA256 derived from the statement id so a re-send recomputes
  the same link; only the SHA-256 hash is persisted, never the plaintext. A
  256-bit token key is required and fatal-on-missing in production, with a
  per-process dev fallback logged only as a fingerprint.
- **Public endpoint**: unauthenticated, token-scoped, renders live session and
  attendance data with money from the invoice snapshot plus adjustments and
  payments. Single neutral 404 for invalid/expired/revoked/paid-in-full; security
  headers (X-Robots-Tag, Cache-Control no-store, Referrer-Policy) on every 200
  and 404; token redacted from request logs; view counters touched after the
  response. Server-rendered VietQR image route; raw bank fields never leave the
  API; adjustment reason text never reaches the parent payload.
- **Message builder + notifications**: pure Vietnamese layer-1 template with a
  grouped-thousands money formatter and a configurable length ceiling that
  collapses per-child detail while keeping the link. Bulk send generates and
  queues every contact of a closed period in one transaction; reminders target
  only outstanding balances, one row per contact.

## Decision

- The notifications table has no message_text/contact_id columns and its purpose
  CHECK uses the plural "statements"; the model mirrors only real columns, message
  text stays response-only, and the API-facing "statement" maps to the DB value.
  Recorded in adr.md.
- QR transfer note is clamped to 25 runes on rune boundaries so a long contact
  name can never push an EMVCo field past its two-digit length and corrupt the
  payload.

## Verification

Code review returned no critical or high findings; real issues fixed (note
clamping + test, a doc-comment wording, env documentation). Lint clean at zero
issues; statements unit and QR tests green; integration coverage above the floor.

## Next steps

Web design-system foundation, then the teacher app and the parent statement page.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
