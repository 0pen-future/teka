---
phase: 2
title: "Statement View, QR, Mobile Layout, Tests"
status: pending
priority: P2
effort: "1.5d"
dependencies: [1]
---

# Phase 2: Statement View, QR, Mobile Layout, Tests

## Overview

Fill the isolated public route from phase 1 with the actual layer-2 statement:
a section per child listing every attended and missed session by date, the fee
formula, nợ cũ, the family grand total, and a transfer QR — all readable on a
phone with no horizontal scrolling (PRD R5 AC 6).

The reader is a parent, not the teacher. They arrived from a Zalo message that
already told them the total; this page exists so they can verify it. Every
element must answer "why is this the number?" — which is why per-session dates
and the explicit formula are non-negotiable rather than nice-to-have.

## Requirements

- [ ] One section per child, in a stable order, each headed by the child's name.
- [ ] Within a child, one block per class when they attend several classes
      (`invoice_lines` is per enrollment, `docs/schema_design.sql:314-318`).
- [ ] Per-class: attended session dates and missed session dates, both visible
      (parent story 2).
- [ ] Per-class fee formula rendered literally, for example
      `12 buổi × 150.000 ₫ = 1.800.000 ₫` (PRD R3 formula).
- [ ] Nợ cũ (opening balance) shown as its own line, separate from this period's
      charge (PRD R3 AC 2).
- [ ] Manual adjustments shown with their amount; the reason is shown only if
      the API deliberately includes it in the public payload — see plan open
      question 5.
- [ ] Family grand total at the end, visually dominant, matching the sum of all
      children plus nợ cũ (PRD R5 AC 2).
- [ ] Transfer QR for the grand total, plus bank name, account number, account
      holder, and a suggested transfer note — all copyable, so a parent whose
      banking app cannot scan can still pay.
- [ ] Nothing overflows horizontally at 320px width.
- [ ] Period label states which month these figures are for.
- [ ] A short "last updated" or live-data cue, since the figures may have
      changed since the message was sent (PRD R5 AC 4).

## Architecture

**Payload shape** (extends phase 1's minimal schema; reconcile with plan 06):

```
statement = {
  contact_name, period: { year, month, period_start, period_end },
  children: [{
    student_name,
    classes: [{
      class_name, unit_price,
      billable_count, absent_count,
      attended_dates: string[],     // ISO dates
      absent_dates:  string[],
      cancelled_dates: string[],    // see open question 3 in plan.md
      amount
    }],
    opening_balance, adjustment_total, subtotal
  }],
  grand_total,
  payment: { bank_name, account_number, account_holder,
             transfer_note, qr_image_url | null }
}
```

Child subtotal and grand total come from the server (`invoices.total_due`,
`docs/schema_design.sql:292`); the page never sums money itself. Client-side
arithmetic on money is how a page ends up disagreeing with the message the
parent already received.

**Layout rhythm.** Single column, `max-w-md`, sections separated by the existing
`Separator` primitive. Per child:

```
┌ Tên con ───────────────────────────┐
│ Lớp Toán 9                          │
│   Có mặt: 03/08 · 05/08 · 10/08 …   │   wrapping chips, not a table
│   Vắng:   07/08                     │
│   12 buổi × 150.000 ₫ = 1.800.000 ₫ │
│ Lớp Lý 9  (second class block)      │
│ Nợ cũ:                    500.000 ₫ │
│ Tạm tính cho con:       2.300.000 ₫ │
└─────────────────────────────────────┘
… (next child)
TỔNG CỘNG CẢ GIA ĐÌNH        4.100.000 ₫
[ QR ]  Bank · số TK · chủ TK · nội dung CK
```

Dates render as wrapping inline chips rather than a table. A table with 12–20
date columns is exactly what forces horizontal scrolling on a phone, which R5
AC 6 forbids.

**QR.** Preferred path is `payment.qr_image_url` from the server rendered as an
`<img>` with fixed intrinsic dimensions to avoid layout shift, `loading="eager"`
(it is the point of the page), and an `alt` describing it as a transfer QR for
the amount. If plan 06 does not supply the URL (plan open question 1), fall back
to a client QR renderer and record the bundle cost. Always render the textual
bank details underneath — a QR alone fails for parents using a bank app without
a scanner or with the image blocked.

**Formatting.** Reuse `formatMoney` and date helpers from
`apps/web/src/lib/utils/format.ts`. That module is created by plan 07 phase 1;
if plan 07 has not landed, this phase creates it with the same signatures rather
than a private copy, so the two plans converge on one implementation.

**Freshness cue.** Render a muted line: "Số liệu cập nhật theo thời gian thực.
Nếu thầy/cô sửa điểm danh, trang này sẽ tự đổi theo." That is the plain-language
version of R5 AC 4 and preempts a "the message said a different number" call.

## Design Spec (prototype parent preview modal)

The page reproduces the prototype's parent sheet full-page: single column,
`max-w` ≈ `--w-phone` 390px centered, `--cream-100` bg, generous 16px gutters.

**Tablet/desktop** (`sm+`): the same 390–480px column stays centered on the
`--cream-100` page — the sheet gains `--shadow-md` and `--radius-xl` so it
reads as a card on the wider canvas; nothing reflows into multiple columns
(the content is a phone-shaped document by design, and a parent on desktop
still gets the exact artifact the teacher previewed). No horizontal scroll at
any width from 320px up.

**Header** — `--mint-400` block, white text, rounded bottom `--radius-xl`:
teacher line 13px at 90% opacity ("Lớp {tên GV} · Học phí tháng M/YYYY"),
family/contact name `font-display` 800 21px, sub-line 12px "cập nhật trực
tiếp, không cần đăng nhập" (this line doubles as the freshness cue — merge the
Architecture section's freshness copy here rather than rendering two notices).

**Adjustment banner** (only when a post-close adjustment exists): `--sun-100`
bg `--radius-lg`, 13px `--sun-600` text explaining the change.

**`ChildSection`** — white `HvCard` per child: child name `font-display` 700
16px + class name as a small mint pill; session-date chips wrap in a flex row —
attended: `--mint-50` bg `--mint-600` "dd/07 ✓"; absent: `--coral-100` bg
`--coral-600` "dd/07 ✕"; cancelled (if in payload): `--cream-200` bg
`--ink-400` "dd/07 huỷ". Formula line 14px `--ink-700`: "{N} buổi có mặt ×
{đơn giá} = {thành tiền}" (server values). Adjustment line `--sun-600` with
sign; nợ cũ line `--coral-600`. Child subtotal row separated by a dashed
`--line-200` top border, amount `font-display` 700 right-aligned.

**`GrandTotal`** — the `--surface-dark` block, `--radius-xl`, white text:
label 12px uppercase letter-spaced at 70% opacity ("TỔNG CỘNG CẢ GIA ĐÌNH"),
amount `font-display` 800 30px `--sun-400`; when fully paid, a
`--mint-400`-tinted "✓ Đã thanh toán" pill.

**`PaymentQr`** — inside the dark block: white `--radius-lg` panel ~150px
square holding the QR image; transfer details underneath in white 13px:
account holder · bank + account number · "ND: {transfer_note}", each with its
`CopyField` (copy icon buttons ≥44px, white at 80% opacity).

**Footer** — expiry note 12px `--ink-400` centered: "Link riêng của gia đình,
hết hiệu lực sau khi thanh toán xong hoặc sau 90 ngày." plus the closing
contact-the-teacher line.

Typography, radii, shadows, and colors all resolve from DS tokens via the
foundation plan's utilities; `rg "#[0-9a-fA-F]{3,6}" apps/web/src/features/statement`
must return nothing.

## Related Code Files

**Modify**

- `apps/web/src/features/statement/schemas/statement-schemas.ts` — expand to the
  full payload above.
- `apps/web/src/features/statement/types/statement-types.ts` — follow the schema.
- `apps/web/src/features/statement/pages/statement-page.tsx` — render
  `StatementView` on success instead of the phase 1 placeholder.
- `apps/web/src/features/statement/components/statement-skeleton.tsx` — match
  the final layout more closely.
- `apps/web/src/test/msw/handlers.ts` — one-child and two-child fixtures, a
  fixture with nợ cũ, and one with a cancelled session.
- `apps/web/src/features/statement/__tests__/statement-page.test.tsx` — extend
  with content assertions.

**Create**

- `apps/web/src/features/statement/components/statement-view.tsx`
- `apps/web/src/features/statement/components/child-section.tsx`
- `apps/web/src/features/statement/components/class-block.tsx`
- `apps/web/src/features/statement/components/session-date-list.tsx`
- `apps/web/src/features/statement/components/grand-total.tsx`
- `apps/web/src/features/statement/components/payment-qr.tsx`
- `apps/web/src/features/statement/components/copy-field.tsx`
- `apps/web/src/features/statement/__tests__/statement-view.test.tsx`
- `apps/web/e2e/statement.spec.ts`
- `apps/web/src/lib/utils/format.ts` — only if plan 07 phase 1 has not landed.

**Delete**

- None.

## Implementation Steps

1. Expand `statement-schemas.ts` to the payload above. Money fields are
   `z.number().int()` (đồng, `docs/schema_design.sql:24`); date arrays are
   `z.array(z.string())` parsed as ISO dates at render time.
2. Build `SessionDateList`: a label ("Có mặt" / "Vắng" / "Buổi huỷ") plus
   wrapping chips of `dd/MM`. Absent chips take a distinct but non-alarming
   tone — this is an invoice, not a discipline report. Omit an empty category
   entirely rather than rendering "Vắng: —".
3. Build `ClassBlock`: class name, the two or three date lists, and the formula
   line built from `billable_count`, `unit_price`, and `amount`, each passed
   through `formatMoney`. The formula displays server values; it does not
   compute `billable_count × unit_price` in the browser.
4. Build `ChildSection`: student name heading, a `ClassBlock` per class, the nợ
   cũ line when non-zero, an adjustment line when non-zero, and the child
   subtotal.
5. Build `GrandTotal`: a visually dominant block with the label "Tổng cộng cả
   gia đình" and the amount at a larger type scale. This is the single number the
   parent came for.
6. Build `CopyField`: a labelled read-only value with a copy button, reusing the
   clipboard helper `apps/web/src/lib/utils/copy-to-clipboard.ts` (created by
   plan 07 phase 4; if that has not landed, create it here with the same
   signature). Used for the account number and the transfer note.
7. Build `PaymentQr`: the QR image with fixed dimensions and descriptive `alt`,
   then bank name, `CopyField` for the account number, account holder, and
   `CopyField` for the transfer note. When `qr_image_url` is null, render the
   textual details alone with a short line explaining that the transfer note
   must be preserved — never a broken image.
8. Build `StatementView` composing: a header (teacher-facing period label
   "Học phí tháng M/YYYY", contact name), the freshness cue, the child sections,
   `GrandTotal`, `PaymentQr`, and a closing line telling the parent to contact
   the teacher with questions.
9. Replace the phase 1 placeholder in `statement-page.tsx` with `StatementView`.
10. Tighten `StatementSkeleton` to the final rhythm: header, one child block,
    total, QR square.
11. Extend the msw fixtures: (a) one child, one class, no debt; (b) two children
    across three classes with nợ cũ on one of them; (c) a child with a cancelled
    session; (d) a payload with `qr_image_url: null`.
12. Write `statement-view.test.tsx` asserting: two children render two named
    sections and exactly one grand total; a child in two classes shows two class
    blocks under one name; attended and absent dates both render; the formula
    line shows the session count, unit price, and amount; nợ cũ renders as its
    own line and is not folded into the class amount; the null-QR payload still
    renders the account number.
13. Extend `statement-page.test.tsx` with a check that the grand total shown is
    the server's `grand_total` value verbatim.
14. Write `apps/web/e2e/statement.spec.ts` at a 375×667 viewport: a valid token
    renders the total and the QR; an invalid token renders the neutral error; a
    two-child token shows both names; and
    `document.documentElement.scrollWidth <= document.documentElement.clientWidth`
    at 320px width — the executable form of R5 AC 6.
15. Run `npm --prefix apps/web run build:analyze` and record the statement
    route's chunk size against the < 30 KB gzipped target in the plan. If the
    chunk pulls in dashboard code, find the offending import before merging.
16. Run typecheck, lint, vitest, and `statement.spec.ts`.

## Success Criteria

- [ ] A two-child family sees one section per child and a single grand total.
- [ ] Each class block shows attended dates, missed dates, and the literal fee
      formula.
- [ ] Nợ cũ appears as a distinct line from this period's charge.
- [ ] The grand total equals the server's value with no client arithmetic.
- [ ] The QR renders for the grand total, and the account number and transfer
      note are copyable.
- [ ] With `qr_image_url` null, the bank details still render and nothing looks
      broken.
- [ ] At 320px, `scrollWidth <= clientWidth` — no horizontal scrolling.
- [ ] Reloading after the teacher edits attendance shows the new figures.
- [ ] The route chunk is under the 30 KB gzipped target, or the overage is
      recorded with its cause.
- [ ] The page matches the prototype parent sheet: mint-400 header, wrapping
      ✓/✕ date chips, dashed-top child subtotal, surface-dark total block with
      the sun-400 amount and white QR panel, and the expiry footer — all from
      DS tokens (no raw hex in `features/statement`).
- [ ] On tablet/desktop widths (768px, 1280px) the sheet renders as a centered
      card on cream with no layout break and no horizontal scroll.
- [ ] typecheck, lint, vitest, and `statement.spec.ts` pass.

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Page total disagrees with the Zalo message a parent already read | Medium | High | Both render from the same server-side statement builder; the page performs no money arithmetic. Flagged as plan open question 4. |
| 20+ date chips push a long class name into horizontal overflow | Medium | Medium | Wrapping chips, `break-words` on names, and the 320px e2e assertion. |
| QR image slow or blocked on mobile data | Medium | Medium | Fixed dimensions to avoid layout shift, and textual bank details always rendered as an independent fallback. |
| A client QR library is pulled in and doubles the bundle | Medium | Medium | Server-supplied image is the preferred path; the analyze step in step 15 catches a regression before merge. |
| Absent dates read as an accusation rather than an explanation | Low | Medium | Neutral tone and neutral color for absent chips; the framing is "why this number", not attendance discipline. |
| Adjustment reasons expose internal notes to parents | Medium | High | Show the reason only if plan 06 deliberately includes it in the public payload; default to amount-only. |

**Rollback:** purely additive within `features/statement`. Reverting this phase
leaves phase 1's route in place showing the placeholder, so parents still get a
working page rather than a 404.
