---
phase: 2
title: "Public Statement Endpoint"
status: pending
priority: P1
effort: "6h"
dependencies: [1]
---

# Phase 2: Public Statement Endpoint

## Overview

Layer 2 of R5: the link a parent opens. One unauthenticated GET, one JSON
payload containing everything the page needs — per child, per class, every
session with its date, the formula, the old-debt line, the family total, and the
QR payload.

This is the only route in the product that serves child data without a login.
Its threat model and its correctness both need to be exactly right.

## Requirements

- R5: no authentication, token in the URL, no search-engine indexing.
- R5: renders **live** data, so a link sent before an attendance correction shows
  the corrected figures.
- R5: per-child sections plus a family grand total.
- R5: an invalid, expired, or revoked token returns a neutral error revealing
  nothing about any student, contact, or teacher.
- R5: everything the page needs arrives in one request (mobile, one screen, no
  horizontal scroll — the API contribution is no N+1 and no second round trip).
- View tracking increments `view_count`, sets `first_viewed_at` once, and
  updates `last_viewed_at` (`docs/schema_design.sql:417-419`).

## Architecture

### Route and mounting

`GET /public/statements/:token` — mounted **outside** the authenticated group.
`registerFeatures` builds the `v1` group with `requireAuth` applied per feature
(`apps/api/internal/server/router.go:63-73`), so the public route is registered
on its own group with no auth middleware and its own response headers.

Response headers on every outcome, success or 404:

```
X-Robots-Tag: noindex, nofollow, noarchive
Cache-Control: no-store, no-cache, must-revalidate, private
Referrer-Policy: no-referrer
```

`Referrer-Policy` matters specifically here: without it, the token leaks in the
`Referer` header of any outbound click from the page.

### Lookup

```
token (43 chars from the path)
  -> hashToken(token)                      (phase 1)
  -> repo.GetByTokenHash(hash)             -- the one teacher-agnostic query
  -> validate: deleted_at IS NULL
              revoked_at IS NULL
              now() < expires_at
              outstanding > 0              -- R5 "expires once paid in full"
```

Any failure at any step returns the **same** response: `404` with
`{"error":{"code":"NOT_FOUND","message":"statement not found"}}`. No branch
returns a different code, a different message, or a measurably different
latency for "wrong token" versus "expired token". Enumeration is already
infeasible at 256 bits; uniformity is what stops the endpoint from confirming
that a given link once existed.

The `outstanding > 0` condition implements R5 literally and is the subject of
OQ-3 in `plan.md` — it means a parent loses the page the moment they finish
paying.

### Payload assembly (one request, no N+1)

Given `statement.contact_id` and `statement.period_id`:

1. **Invoices** — all non-void invoices for that contact and period, with lines.
   One query joining `invoice_lines`.
2. **Sessions** — every session that produced a line, live:

```sql
SELECT il.enrollment_id, cs.session_date, cs.status, a.status AS attendance_status, a.billable
FROM invoice_lines il
JOIN invoices i        ON i.id = il.invoice_id AND i.teacher_id = il.teacher_id
JOIN attendance_records a ON a.enrollment_id = il.enrollment_id AND a.teacher_id = il.teacher_id
JOIN class_sessions cs ON cs.id = a.session_id AND cs.teacher_id = a.teacher_id
JOIN billing_periods bp ON bp.id = i.period_id
WHERE i.teacher_id = :teacher_id
  AND i.contact_id = :contact_id
  AND i.period_id  = :period_id
  AND i.status <> 'void'
  AND a.deleted_at IS NULL
  AND cs.deleted_at IS NULL
  AND cs.session_date BETWEEN bp.period_start AND bp.period_end
ORDER BY il.enrollment_id, cs.session_date
```

3. **Adjustments** — `invoice_adjustments` on those invoices
   (`deleted_at IS NULL`), plus adjustments on *other* periods' invoices whose
   `source_session_id` falls inside this period, which is how a post-close
   correction is shown as carried forward.
4. **Payments** — allocations against those invoices, for the paid/outstanding
   figures. Reuse plan 05's derivation rather than re-summing.

Four queries total, independent of the number of children.

### The live-versus-snapshot rule

This is the crux of R5's "mở link cũ thấy số liệu đã cập nhật" against schema
note (k) (`docs/schema_design.sql:533`) and D7.

| Rendered element | Source |
|---|---|
| Session list, dates, present/absent | **Live** `attendance_records` + `class_sessions` |
| Session counts shown next to the list | **Live** count |
| Charged amount for the line | Invoice snapshot `invoice_lines.amount` |
| Unit price | Invoice snapshot `invoice_lines.unit_price` |
| Old debt | Invoice snapshot `opening_balance` |
| Adjustments in this period | Live `invoice_adjustments` |
| Corrections carried to next period | Live, by `source_session_id` |
| Paid / outstanding | Live, from allocations |

When the live count and the charged count differ, the payload includes a
`carried_adjustment` block per child naming the amount and the reason, so the
page can say "buổi ngày 12/8 đã được sửa, chênh lệch chuyển sang kỳ sau"
(the 12 Aug session was corrected; the difference is carried to next period).
The parent sees corrected data and an explainable total. Silently showing a
live-recomputed amount that disagrees with what they were told, or showing a
stale session list, are both worse.

### QR: a server-rendered image URL (OQ-1)

The parent page receives an image URL, never bank fields. Web plan 08 asked for
this so no QR library ships in the public bundle, and it also means the payload
format stays an API concern.

```go
// BankConfig is the teacher's transfer target. The schema has no column for
// this, so V1 reads it from application configuration; see plan.md OQ-1.
type BankConfig struct {
    BankCode      string
    AccountNumber string
    AccountName   string
}

type QRBuilder interface {
    // Payload returns the VietQR string. ok is false when bank config is absent.
    Payload(cfg BankConfig, amount int64, note string) (payload string, ok bool)
    // Render turns a payload into a PNG.
    Render(payload string) ([]byte, error)
}
```

Two routes, both token-scoped:

- `GET /public/statements/:token` → payload contains
  `qr: { image_url, amount, note }` or `qr: null`.
- `GET /public/statements/:token/qr.png` → the PNG itself, same token, same
  neutral 404, same `X-Robots-Tag` and `Referrer-Policy` headers. `Cache-Control`
  here is `private, max-age=300` — the image is derived from data the holder of
  the token can already see, and re-rendering it on every page paint is waste.

`image_url` is absolute, built from `cfg.Statements.PublicBaseURL`, so the page
can use it directly in an `<img>` tag.

When bank config is absent, `Payload` returns `ok=false`, `qr` is `null`, and
the image route returns `404`. No placeholder, no fake account number.

The transfer note is `"HP {contact_name} {MM/YYYY}"` (HP = học phí, tuition),
which is what P1's semi-automatic bank reconciliation will match on
(`docs/schema_design.sql:368`).

Resolving OQ-1 by adding schema columns changes only where `BankConfig` is
loaded from. The contract above does not move.

### Adjustments: amount only, never the reason

`invoice_adjustments.reason` (`docs/schema_design.sql:349`) is teacher-facing
free text and **never** appears in the public payload. Under plan 04 it is
sometimes generated and sometimes typed in a hurry, and it can carry internal
reasoning a parent should not read.

The public payload shows each adjustment as:

```json
{ "amount": -150000, "kind": "correction" | "manual" }
```

`kind` is derived, not stored: `correction` when `source_session_id IS NOT
NULL`, `manual` otherwise. That is enough for the page to label the line
("điều chỉnh do sửa điểm danh" / "điều chỉnh") without exposing a single
character of teacher text. The teacher reads the real reason in the
authenticated invoice view (plan 04 phase 4).

### Payment breakdown per invoice

Web plan 08 renders the underpayment split but never recomputes D8's rule
client-side, so the payload carries the server's answer per invoice:

```json
"payments": {
  "total_paid": 500000,
  "by_invoice": [ { "student_name": "...", "total_due": 800000,
                    "paid": 500000, "outstanding": 300000 } ]
}
```

Sourced from plan 05's allocation data — the same derivation
`paid_amount` uses, not a second sum. This is what lets the page tell a parent
which child's balance their partial payment landed on, which is exactly the
question a partial payment provokes.

### View tracking

After a successful render, in a separate statement (never blocking the
response on a lock):

```sql
UPDATE statements
SET view_count = view_count + 1,
    first_viewed_at = COALESCE(first_viewed_at, now()),
    last_viewed_at = now()
WHERE id = :id
```

Failures here are logged, not returned — a counter must never cost a parent
their page.

### Rate limiting

Not implemented, deliberately. The token is 256 bits, so enumeration is
infeasible; there is no login to brute force and no per-user secret to guess.
Adding a limiter would protect nothing and would risk locking out a shared
mobile network where several parents sit behind one NAT address. Revisit only if
abuse appears in logs.

## Related Code Files

Create:

- `apps/api/internal/features/statements/public_handler.go`
- `apps/api/internal/features/statements/render.go` — payload assembly
- `apps/api/internal/features/statements/render_test.go`
- `apps/api/internal/features/statements/qr.go` — `QRBuilder`, VietQR payload
  and PNG rendering
- `apps/api/internal/features/statements/qr_test.go`
- `apps/api/internal/features/statements/public_integration_test.go`

Modify:

- `apps/api/internal/features/statements/repository.go` — add
  `InvoicesWithLines`, `LiveSessions`, `Adjustments`, `CarriedAdjustments`,
  `TouchView`
- `apps/api/internal/features/statements/service.go` — add `RenderPublic`
- `apps/api/internal/features/statements/dto.go` — add `PublicStatement`,
  `PublicChild`, `PublicClass`, `PublicSession`, `PublicTotals`
- `apps/api/internal/features/statements/routes.go` — add
  `RegisterPublicRoutes` (statement JSON + `qr.png`)
- `apps/api/go.mod` / `go.sum` — add a QR encoding library
- `apps/api/internal/server/router.go` — mount the public group with no auth
- `apps/api/internal/config/config.go` — add `BankConfig` fields
- `apps/api/internal/middleware/logger.go` — redact the token segment from
  logged paths **[verify how the logger records paths before editing]**

Delete: none. No migration files.

## Implementation Steps

1. Inspect `apps/api/internal/middleware/logger.go` and confirm whether the
   request path is logged. If it is, redact `/public/statements/:token` to
   `/public/statements/[redacted]` before this route ships. A token in an
   access log is a permanent credential leak.
2. Add `BankConfig` to config, all fields optional.
3. Write `qr.go` with `Payload` (VietQR-compatible string, `ok=false` on absent
   config) and `Render` (payload → PNG, using a QR encoding library; add the
   dependency to `apps/api/go.mod`). Unit-test the payload checksum, the
   absent-config path, and that `Render` returns a decodable PNG. Never fetch a
   QR from a third-party image service — that would send the teacher's account
   number and the parent's amount to an external host on every page view.
4. Add the four repository read methods per Architecture, each a single query.
5. Write `render.go` assembling `PublicStatement`:
   - `contact_name`, `period` (month/year label)
   - `children[]`: `student_name`, `display_note`, `classes[]`
   - `classes[]`: `class_name`, `unit_price`, `billable_count`, `absent_count`,
     `amount`, `sessions[]`
   - `sessions[]`: `date`, `status` (`present`/`absent`/`excused`),
     `counted` (bool, from `billable`)
   - per child: `opening_balance`, `adjustments[]` (`amount` + derived `kind`,
     **no `reason`**), `carried_adjustment` (nullable), `subtotal`
   - `totals`: `opening_balance`, `current_charge`, `adjustment_total`,
     `total_due`, `paid`, `outstanding`
   - `payments`: `total_paid` and `by_invoice[]` per Architecture
   - `qr`: nullable `{ image_url, amount, note }`
   No teacher identity, no phone number, no adjustment reason text, and no other
   family's data appears anywhere in the payload.
6. Write `public_handler.go`: set the three headers first, then look up, then
   render, then fire `TouchView`. Return the neutral 404 through a single
   helper used by every failure branch so the branches cannot drift.
7. Add the `GET /public/statements/:token/qr.png` handler: same lookup, same
   neutral 404, `Content-Type: image/png`, `Cache-Control: private, max-age=300`.
   Register `RegisterPublicRoutes(r)` in `NewRouter`
   (`apps/api/internal/server/router.go:26-58`) on the root engine, outside
   `v1`, before the `NoRoute` handler at line 53.
8. Write `render_test.go` as unit tests over fixture data: two children in two
   classes; one child in two classes; a child with an absent session; a carried
   adjustment present; a zero-QR-config case.
9. Write `public_integration_test.go`:
   - valid token → 200, both children present, family total equals the sum
     (R5 acceptance);
   - the three headers are present on 200 **and** on 404;
   - unknown token, malformed token, revoked token, expired token, soft-deleted
     statement, and fully-paid statement → byte-identical 404 bodies;
   - correct an attendance record after issue, re-fetch the same link → session
     list shows the correction and `carried_adjustment` explains the difference
     (R5 acceptance);
   - view tracking: first open sets `first_viewed_at` and `view_count=1`;
     second open leaves `first_viewed_at` and sets `view_count=2`;
   - the payload contains no other contact's data (seed two families, assert the
     other family's names appear nowhere in the response body);
   - adjustment `reason` text never appears: seed an adjustment whose reason is a
     unique sentinel string and assert it is absent from the response body,
     while its `amount` and `kind` are present;
   - `payments.by_invoice` reflects an underpayment split correctly: pay less
     than the family total, assert the per-child `paid`/`outstanding` match plan
     05's allocations exactly (no client-side recomputation is possible);
   - `qr.image_url` resolves: fetch it and assert `200` with a decodable PNG,
     `image/png`, and the three security headers; with bank config absent,
     `qr` is `null` and the image route returns the neutral 404;
   - query count for a three-child family equals the count for a one-child
     family (no N+1) — assert with a query counter on the test DB handle.
10. Run `go test ./apps/api/internal/features/statements/... -race`.

## Success Criteria

- [ ] R5: a two-child parent sees both children separately with a grand total.
- [ ] R5: an old link reflects an attendance correction made after it was sent.
- [ ] R5: invalid, expired, revoked, and paid-in-full tokens all return the same
      neutral 404 with no identifying information.
- [ ] R5: one request returns everything the page needs; query count does not
      grow with the number of children.
- [ ] `X-Robots-Tag`, `Cache-Control`, and `Referrer-Policy` are set on every
      response from the route.
- [ ] The token never appears in an application log.
- [ ] View counters behave correctly across repeated opens and never fail the
      request.
- [ ] The QR block is omitted, not faked, when bank config is absent.
- [ ] The QR arrives as a server-rendered `image_url`; no bank code, account
      number, or account name appears in the parent payload.
- [ ] Adjustment `reason` text never reaches the parent payload; only signed
      amounts and a derived `kind`.
- [ ] `payments.by_invoice` gives the per-child split, so the page never
      recomputes the allocation rule.

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Token leaks through access logs or `Referer` | High | Critical | Log redaction verified in step 1 before shipping; `Referrer-Policy: no-referrer` on every response |
| Error branches differ and confirm a token once existed | Medium | Medium | Every failure routed through one helper; integration test compares response bodies byte for byte |
| Payload leaks another family's or the teacher's data | Low | Critical | Every query filtered by `contact_id` **and** `period_id`; two-family integration assertion searches the whole response body |
| Teacher's internal adjustment note reaches a parent | Medium | High | `reason` is excluded at the DTO level, not filtered at render time; sentinel-string assertion in step 9 |
| Teacher's bank account number exposed in the JSON payload | Low | Medium | Bank fields never leave the server; only a rendered image URL is returned |
| Live session list disagrees with the charged amount, destroying trust in the number | High | High | `carried_adjustment` block makes the difference explicit with its reason; this is the single most important product detail in the phase |
| Public route accidentally mounted inside the authenticated group | Low | High | Registered on the root engine in `NewRouter`, not in `registerFeatures`; integration test calls it with no `Authorization` header |
| N+1 across children makes the page slow on mobile | Medium | Medium | Four fixed queries; query-count assertion in step 9 |
| `TouchView` lock contention on a popular statement | Low | Low | Single-row `UPDATE` after the response is assembled; failures logged, not returned |
| Search engine indexes a link a parent shared | Low | High | `X-Robots-Tag` on every response; no sitemap; tokens never linked from any public page |

**Rollback.** Read-only apart from the view counters. Remove the public route
registration from `NewRouter` to disable it instantly without touching data;
revoking individual statements is the per-parent equivalent.
