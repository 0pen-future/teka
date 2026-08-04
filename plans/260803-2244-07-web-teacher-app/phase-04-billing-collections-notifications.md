---
phase: 4
title: "Billing Close (Chốt Sổ), Collections Board, Notifications"
status: completed
priority: P2
effort: "3d"
dependencies: [1, 2, 3]
---

# Phase 4: Billing Close (Chốt Sổ), Collections Board, Notifications

## Overview

The month-end workflow, end to end: review every student's computed fee, close
the period ("chốt sổ" — freeze the numbers and issue invoices), collect money
against contacts, and hand each parent a generated Zalo message. Goal G1 is that
this whole loop takes **under 10 minutes** for a full roster, so every screen
here is judged on how few decisions it forces.

Three screens, one continuous flow:

```
/billing/:periodId      review + adjust + close
        ↓ (after close)
/notifications/:periodId  generated messages, copy, mark sent
        ↓ (as money arrives)
/collections/:periodId    who paid, who owes, mark paid
```

## Requirements

**Review and close (PRD R3, R4)**

- [x] One screen shows every student × session count × amount for the period
      (R4).
- [x] Each row separates nợ cũ (opening balance) from this period's charge
      (R3 AC 2).
- [x] A student in two classes shows one row with two class lines that sum into
      a single invoice (R1 AC 2; `invoice_lines`, `docs/schema_design.sql:318`).
- [x] Per-line manual adjustment with a **required** reason (R4;
      `invoice_adjustments.reason` is `NOT NULL` with a non-empty CHECK,
      `docs/schema_design.sql:343,351`).
- [x] Close is blocked while any session in the period is held but unconfirmed;
      each offending session is listed and links to its attendance screen
      (R4 AC 1).
- [x] Close asks for one confirmation showing student count and grand total,
      then locks the period and issues invoices.
- [x] A class with no sessions in the period produces no invoice and appears as
      such rather than as a zero row (PRD §5 edge case).

**Collections (PRD R7)**

- [x] Two toggleable views over the same data; **by-contact is the default**.
- [x] By-contact: one row per parent merging all children, with total due, total
      paid, and outstanding (`v_contact_balance`, `docs/schema_design.sql:459`).
- [x] By-class: students grouped by class with chưa đóng / đã đóng / đóng thiếu
      status.
- [x] Unpaid filter reachable in one interaction (R7 AC 1).
- [x] Mark-paid happens at contact level; the allocation across children is
      shown before confirming and can be overridden manually (R7; Q8;
      `payment_allocations.allocated_by`, `docs/schema_design.sql:392`).
- [x] Header totals: đã thu / còn phải thu.
- [x] A fully-paid contact shows both children as paid in the by-class view
      (R7 AC 3); an underpaying contact shows where the shortfall landed
      (R7 AC 4).

**Notifications (PRD R5 layer 1, R6)**

- [x] One message per contact per period — never one per child (R5 AC 1).
- [x] Message body contains, per child: name, sessions attended, sessions
      missed, amount; then nợ cũ and the family grand total; then the statement
      link (R5 layer 1, R5 AC 3).
- [x] Copy button per row and a bulk copy for all unsent messages.
- [x] Sent status is tracked per contact and survives reload
      (`notifications.status`, `docs/schema_design.sql:440`).
- [x] A reminder is a single message per family even when several children owe
      (R7 AC 5).

## Architecture

Two feature folders. `features/billing` (created in phase 1 with only the period
hook) grows the review and close screens. `features/collections` owns the
collections board and the notifications screen — both are period-scoped
money-collection views sharing the contact/invoice types, and splitting them
would duplicate the invoice schema.

**Review table layout.** The R4 requirement is "one screen", but a phone cannot
show 8 columns. Two renderings of one data set:

- `sm` and up: a table with a sticky first column (student name) and horizontal
  scroll confined to the table wrapper, never the page.
- under `sm`: one card per student — name, per-class lines, nợ cũ, adjustment,
  total — with a sticky period summary bar on top.

Review data is read-only until close; adjustments are the only mutation, so the
table does not need editable cells, just a per-row "Điều chỉnh" action.

**Close gating.** The review response carries `blocking_sessions[]`. When
non-empty the close button is disabled and a warning panel lists each session
with a link to `/sessions/:id/attendance` (phase 3). The check is server-side
too; the UI must not be the only guard.

**Allocation preview.** Marking a contact paid opens a dialog that:

1. shows each child's invoice with total due and already-paid;
2. shows the default allocation the server would apply — oldest debt first, then
   this period's charge, ties broken by earlier class start (PRD Q8);
3. lets the teacher edit each child's amount, which flips
   `allocated_by` to `manual`;
4. validates that the allocations sum exactly to the payment amount before the
   button enables.

The default allocation is **requested from the server** as a preview rather than
recomputed in the browser. Duplicating the rule in TypeScript guarantees the
preview and the write eventually disagree, and disagreement here means telling a
parent the wrong number.

**Notification content is server-rendered.** The API returns the final message
text (`message_text`) per contact. The browser only displays and copies it. The
same text must match what the statement page (plan 08) shows, and the ZNS
template constraint in R6 is a server concern.

**Copy on mobile.** Use `navigator.clipboard.writeText` with a fallback to a
selectable `<textarea>` when the clipboard API is unavailable (non-secure
contexts). Copy does **not** mark the message sent — a separate "Đã gửi" action
does, because copying is not evidence of sending.

**Data flow.**

```
BillingReviewPage
  GET /billing/periods/:id/review
     -> { period, rows[{ student_id, student_name, contact_id, contact_name,
                          lines[{ class_name, billable_count, absent_count,
                                  unit_price, amount }],
                          opening_balance, adjustment_total, total_due }],
          totals, blocking_sessions[] }
  POST /invoices/:invoiceId/adjustments { amount, reason }  -> invalidate review
  POST /billing/periods/:id/close                            -> period closed,
                                                                invoices issued

CollectionsPage (?view=contact|class&status=unpaid)
  GET /billing/periods/:id/collections?view=contact  -> contact rows
  GET /billing/periods/:id/collections?view=class    -> class-grouped rows
  GET /contacts/:id/allocation-preview?period_id&amount -> proposed split
  POST /payments { contact_id, amount, method, received_on, allocations[] }

NotificationsPage
  GET /billing/periods/:id/notifications
     -> [{ contact_id, contact_name, phone, message_text, statement_url,
            status, sent_at, purpose }]
  POST /notifications/mark-sent { notification_ids[] }
  POST /billing/periods/:id/reminders { contact_ids[] }  -> one per contact
```

**Assumed API contract** (reconcile with plans 04, 05, 06): the endpoints above,
plus `GET /billing/periods?limit=2` for the period switcher.

## Design Spec (prototype `close`, `pay`, `send` + `modalAdjust`, `modalPay`, `modalWarn`)

All styling via DS tokens and `@/components/hv`. Per-screen recipes:

Responsive tiers per the plan's "Responsive Strategy": review cards `< sm`,
sticky-first-column table with confined scroll `sm`–`lg`, full table without
scroll at `lg+`; message cards 1-col below `lg`, 2-col at `lg+`; every
`HvModal` here is a bottom sheet `< sm` and a centered ≤480px panel above.

**Chốt sổ (`close`).**

- Review table inside an `HvCard flat` (padding 0): 11px uppercase `--ink-400`
  headers; rows grouped by class under **`--mint-50` group-header rows** (class
  name `font-display` 700 `--mint-600` + session count). Numeric cells 15px;
  amounts right-aligned `font-display` 700.
- Nợ cũ cells `--coral-600` when non-zero; adjustment cells `--sun-600` with
  sign; per-row "Sửa" `HvButton variant="ghost" size="sm"` (disabled with
  muted look after close).
- `BlockingSessionsPanel` = prototype blocked panel: `--coral-100` bg
  `--radius-xl`, title `font-display` 700 `--coral-600` ("Chưa thể chốt sổ"),
  each blocking session a row with an `HvButton variant="danger" size="sm"`
  "Điểm danh" (press-coral) linking to the attendance screen.
- Post-close-edit adjustments panel (when the API reports adjustments created
  from closed-period edits): `--sun-100` bg, `--sun-600` title, line per
  adjustment, note "sẽ ghi vào kỳ {next}".
- Footer bar: left — student/contact counts 13px `--ink-400`; center — grand
  total `font-display` 800 22px `--mint-600`; right — `HvButton
  variant="primary"` "Chốt kỳ & tạo phiếu thu", replaced after close by a
  `--mint-50`/`--mint-600` pill "✓ Đã chốt — kỳ đã khóa" plus an `HvButton
  variant="secondary"` "Gửi thông báo →" linking to the notifications screen.
- `AdjustmentDialog` = `modalAdjust` in `HvModal`: number input step 10.000,
  required reason, live new-total line; title "Sửa thủ công".
- `ClosePeriodDialog` in `HvModal` with the primary action `HvButton
  variant="primary" size="lg" block`.
- Closed-period edit warning (`modalWarn` recipe, used by phase 3's flow):
  reward-variant action "Sửa & điều chỉnh kỳ sau".

**Thu tiền (`pay`).**

- Segmented contact/class toggle: pill container white with line border;
  active segment `--mint-400` white `font-display` 700 (prototype `segStyle`).
- Filter chips (prototype `chipStyle`): "Tất cả / Chưa đóng / Đóng thiếu / Đã
  đóng" — active chip `--ink-900` bg white text, idle white + line border,
  `--radius-pill`. The "Chưa đóng" chip is the R7 one-interaction unpaid
  filter.
- Progress header `HvCard`: Phải thu / Đã thu / Còn lại (amounts
  `font-display` 700; Đã thu `--mint-600`, Còn lại `--coral-600`) + a
  `ProgressBar` with percentage.
- Contact rows: status via `StatusPill` (paid mint-50/mint-600 "Đã đóng",
  partial sun-100/sun-600 "Đóng thiếu", unpaid coral-100/coral-600 "Chưa
  đóng"); actions per row — "Thu tiền" `HvButton variant="primary" size="sm"`
  (press-mint), "Nhắc nợ" `variant="ghost" size="sm"` (hover sky tint),
  "Trang PH" as a 13px `--sky-600` link.
- Class view: per-child rows with `StatusPill` and a `--coral-600` "thiếu
  {amount}" shortfall note where allocation fell short (R7 AC 4).
- `RecordPaymentDialog` = `modalPay` in `HvModal`: title "Ghi nhận thu",
  amount money-input, then the allocation preview in a `--cream-100`
  `--radius-lg` box titled "PHÂN BỔ TỰ ĐỘNG — nợ cũ trước, rồi kỳ này" (11px
  uppercase `--ink-400`), one line per child with editable amounts
  (`AllocationEditor`), remainder line `--coral-600` when non-zero.

**Gửi thông báo (`send`).**

- Header `HvCard`: contact count + total, and the demo-only "Gửi tất cả bằng
  1 chạm" concept is replaced by the real V1 action "Sao chép tất cả chưa
  gửi" (`HvButton variant="secondary"`) — the prototype's one-tap send is
  explicitly simulation; V1 channel is `zalo_manual`.
- `MessageCard` grid (2-col `lg+`, 1-col below): avatar = 40px `--sky-100`
  circle with the contact's initial in `--sky-600` `font-display` 700; name
  700 15px + phone 13px `--ink-400`; message preview in a `--sky-50`
  `--radius-lg` box, 13px, `white-space: pre-wrap` (structure: "[Học phí
  T{M}/{YYYY}]" header line, per-child lines, `--coral-600` nợ-cũ line,
  `--mint-600` total line, statement link); actions "Sao chép" (`secondary
  sm`) and "Đã gửi" (`primary sm`); sent state = `--mint-50`/`--mint-600` "✓
  Đã gửi" pill with `sent_at`, buttons demoted to ghost.
- Toasts for copy/sent via `HvToast`.

## Related Code Files

**Create**

- `apps/web/src/features/billing/pages/billing-review-page.tsx`
- `apps/web/src/features/billing/components/review-table.tsx`
- `apps/web/src/features/billing/components/review-card-list.tsx`
- `apps/web/src/features/billing/components/blocking-sessions-panel.tsx`
- `apps/web/src/features/billing/components/adjustment-dialog.tsx`
- `apps/web/src/features/billing/components/close-period-dialog.tsx`
- `apps/web/src/features/billing/components/period-switcher.tsx`
- `apps/web/src/features/billing/routes.tsx`
- `apps/web/src/features/billing/__tests__/billing-review-page.test.tsx`
- `apps/web/src/features/billing/__tests__/adjustment-dialog.test.tsx`
- `apps/web/src/features/collections/api/collections-api.ts`
- `apps/web/src/features/collections/api/notifications-api.ts`
- `apps/web/src/features/collections/schemas/collections-schemas.ts`
- `apps/web/src/features/collections/types/collections-types.ts`
- `apps/web/src/features/collections/hooks/use-collections.ts`
- `apps/web/src/features/collections/hooks/use-notifications.ts`
- `apps/web/src/features/collections/components/collections-view-toggle.tsx`
- `apps/web/src/features/collections/components/contact-collection-row.tsx`
- `apps/web/src/features/collections/components/class-collection-group.tsx`
- `apps/web/src/features/collections/components/record-payment-dialog.tsx`
- `apps/web/src/features/collections/components/allocation-editor.tsx`
- `apps/web/src/features/collections/components/message-card.tsx`
- `apps/web/src/features/collections/pages/collections-page.tsx`
- `apps/web/src/features/collections/pages/notifications-page.tsx`
- `apps/web/src/features/collections/routes.tsx`
- `apps/web/src/features/collections/index.ts`
- `apps/web/src/features/collections/__tests__/collections-page.test.tsx`
- `apps/web/src/features/collections/__tests__/record-payment-dialog.test.tsx`
- `apps/web/src/features/collections/__tests__/notifications-page.test.tsx`
- `apps/web/src/lib/utils/copy-to-clipboard.ts`
- `apps/web/e2e/billing.spec.ts`
- `apps/web/e2e/collections.spec.ts`

**Modify**

- `apps/web/src/features/billing/api/billing-api.ts` — add `getReview`,
  `closePeriod`, `createAdjustment`, `getPeriods`.
- `apps/web/src/features/billing/schemas/billing-schemas.ts` — add
  `reviewRowSchema`, `invoiceLineSchema`, `blockingSessionSchema`.
- `apps/web/src/features/billing/hooks/use-billing.ts` — add `useReview`,
  `useClosePeriod`, `useCreateAdjustment`.
- `apps/web/src/features/billing/index.ts` — export the review query key so
  phase 3's attendance mutation can invalidate it.
- `apps/web/src/app/router.tsx` — mount `billingRoutes` and `collectionsRoutes`.
- `apps/web/src/test/msw/handlers.ts` — review, collections, and notification
  fixtures.

**Delete**

- None.

## Implementation Steps

1. Extend `billing-schemas.ts` with `invoiceLineSchema`
   (`{ class_name, billable_count, absent_count, unit_price, amount }`, mirroring
   `docs/schema_design.sql:318`), `reviewRowSchema`
   (`{ invoice_id, student_id, student_name, contact_id, contact_name, lines[],
   opening_balance, current_charge, adjustment_total, total_due }`, mirroring
   `invoices` at `docs/schema_design.sql:276`), `blockingSessionSchema`, and
   `reviewSchema` wrapping rows plus totals.
2. Extend `billing-api.ts` and `use-billing.ts` with the review query, close
   mutation, and adjustment mutation. `useClosePeriod` invalidates the review,
   the current-period query, and the collections and notifications keys.
3. Build `ReviewTable` (`sm+`): columns Học sinh | Lớp & số buổi | Vắng | Đơn
   giá | Thành tiền | Nợ cũ | Điều chỉnh | Tổng | (action). Multi-class students
   render their class lines stacked inside one row so the invoice stays one row.
   Sticky first column, `overflow-x-auto` on the wrapper only.
4. Build `ReviewCardList` (under `sm`) rendering the same rows as cards. Both
   components consume the identical row type — no second data shape.
5. Build `BlockingSessionsPanel`: warning-toned, lists each blocking session as
   "Tên lớp — dd/MM" linking to `/sessions/:id/attendance`, with copy explaining
   that closing is blocked until every past session is attended.
6. Build `AdjustmentDialog`: signed amount input (allow negative for a
   reduction, matching `docs/schema_design.sql:342`) and a required reason
   textarea. Disable submit while the reason is blank or whitespace, mirroring
   the DB CHECK. Show the resulting new total before confirming.
7. Build `ClosePeriodDialog`: student count, grand total, count of contacts to
   notify, and irreversible-action copy. Disable it entirely when
   `blocking_sessions` is non-empty.
8. Build `PeriodSwitcher`: current and previous period only (plan open question
   3). Selecting one navigates to `/billing/:periodId`.
9. Build `BillingReviewPage` composing period switcher, totals header, blocking
   panel, review table or card list, and the close action. Fetch through
   `useReview`; render the empty case ("Kỳ này chưa có buổi học nào") rather
   than an empty table.
10. Register `billingRoutes` (`billing`, `billing/:periodId`) in
    `apps/web/src/app/router.tsx:31`; `billing` redirects to the current period.
11. Create `features/collections` with `collections-schemas.ts`:
    `contactCollectionRowSchema`
    (`{ contact_id, contact_name, phone, student_count, total_due, total_paid,
    outstanding, students[{ student_id, student_name, total_due, paid_amount,
    status }] }`, mirroring `v_contact_balance`, `docs/schema_design.sql:459`)
    and `classCollectionGroupSchema`.
12. Build `CollectionsViewToggle` writing `?view=contact|class` into the URL
    with `replace: true`, following the parameter pattern at
    `apps/web/src/features/users/pages/users-page.tsx:75`. Default is `contact`
    when the parameter is absent — the PRD's stated default.
13. Build `ContactCollectionRow`: parent name, children count, total due, paid,
    outstanding, a status chip, and a "Đã thu" action. Expanding a row reveals
    the per-child breakdown so the by-contact view can answer by-class questions
    without switching.
14. Build `ClassCollectionGroup`: class header with collected/total, then
    student rows with chưa đóng / đã đóng / đóng thiếu chips. For an underpaying
    family, show the allocated portion per child so R7 AC 4 is satisfied here.
15. Add the status filter as the prototype's four chips — "Tất cả / Chưa đóng /
    Đóng thiếu / Đã đóng" — writing `?status=unpaid|partial|paid`, present in
    both views. "Chưa đóng" satisfies R7 AC 1's one-interaction unpaid filter;
    chips style per the Design Spec (`chipStyle`).
16. Build `RecordPaymentDialog`: amount (money input), method
    (`cash | transfer | other`, `docs/schema_design.sql:365`), `received_on`
    defaulting to today, optional `reference_code` and note. On amount change,
    fetch the allocation preview (debounced) and render `AllocationEditor`.
17. Build `AllocationEditor`: one row per child invoice with the proposed amount
    prefilled and editable. Editing any row marks the whole payment
    `allocated_by: "manual"`. Show a live remainder line; block submit unless the
    allocations sum exactly to the payment amount. Include a "Dùng phân bổ mặc
    định" reset button.
18. Build `CollectionsPage` with a totals header (đã thu / còn phải thu), the
    view toggle, the filter, and the two view bodies. Register
    `collectionsRoutes` (`collections`, `collections/:periodId`).
19. Build `notifications-api.ts` and `use-notifications.ts`
    (`getPeriodNotifications`, `markSent`, `sendReminders`).
20. Write `apps/web/src/lib/utils/copy-to-clipboard.ts`: try
    `navigator.clipboard.writeText`, fall back to a hidden textarea plus
    `document.execCommand("copy")`, return a boolean so callers can toast
    accurately instead of claiming success blindly.
21. Build `MessageCard`: parent name and phone, the generated `message_text` in
    a monospace-ish preformatted block, the statement URL, a "Sao chép" button,
    a "Đã gửi" button, and a status chip driven by `notifications.status`. When
    already sent, show `sent_at` and demote both buttons to secondary.
22. Build `NotificationsPage`: period header, a "Sao chép tất cả chưa gửi"
    action producing one blob separated by blank lines and a divider per parent,
    a status summary (`N/M đã gửi`), and the list of message cards. Add a
    "Nhắc nợ" action for contacts with outstanding balance that creates exactly
    one reminder per contact — the request takes `contact_ids`, so a family with
    several indebted children still yields one message (R7 AC 5).
23. Add msw fixtures covering: a two-child family across two classes, a
    single-child family with nợ cũ, a class with no sessions in the period, one
    blocking unconfirmed session, and one contact who underpaid.
24. Write the vitest suites: review renders one row per student with the
    multi-class student showing two class lines and one total; the close button
    is disabled with blocking sessions listed and each linking to its attendance
    screen; the adjustment dialog blocks an empty reason; the collections page
    defaults to the contact view; the unpaid filter reduces the row set in one
    interaction; the payment dialog blocks a mismatched allocation sum and marks
    manual on edit; the notifications page shows one card per contact (not per
    child) and reflects sent status after marking.
25. Write `apps/web/e2e/billing.spec.ts`: attempt close with a pending session
    (blocked), take that attendance, return, close successfully, and confirm the
    period reads closed.
26. Write `apps/web/e2e/collections.spec.ts`: mark a two-child family paid in
    full, reload, assert the status persists, then switch to the by-class view
    and assert both children read as paid.
27. Run typecheck, lint, vitest, and both new e2e specs.

## Success Criteria

- [x] The review screen shows every student with session count, unit price,
      amount, nợ cũ, and total, on one screen at `sm+` and as cards below it.
- [x] A student in two classes appears as one row with two class lines and one
      total.
- [x] Adjusting a line requires a reason; the new total is visible before
      confirming.
- [x] Close is blocked while any past session is unattended, and each blocking
      session links to its attendance screen.
- [x] Closing shows one confirmation with the student count and grand total,
      then locks the period.
- [x] The collections board opens in the by-contact view without a URL
      parameter.
- [x] The unpaid group is reachable in one interaction.
- [x] Marking a family paid persists across reload and reduces outstanding in
      both views.
- [x] Under-payment shows the per-child allocation in the by-class view.
- [x] A family with two children receives exactly one message and one reminder.
- [x] Copying a message works on a phone browser and does not silently mark it
      sent.
- [x] All three screens match their prototype recipes: mint-50 class group
      headers and the coral blocked panel on chốt sổ; segmented toggle, ink-900
      filter chips, and the StatusPill trio on thu tiền; sky-tinted message
      cards with initial avatars on gửi thông báo — DS tokens throughout (no
      raw hex in `features/billing` or `features/collections`).
- [x] The review screen renders correctly at all three tiers — cards at 375px,
      sticky-column scrolling table at 768px, full no-scroll table at 1280px —
      with no horizontal page scroll (e2e viewport assertions).
- [x] typecheck, lint, vitest, and both e2e specs pass.

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Client-side allocation preview diverges from the server's write | Medium | High | Preview is fetched from the server, never recomputed in the browser. |
| Review table unreadable on a phone | Medium | High | Card layout under `sm`; horizontal scroll confined to the table wrapper at `sm+`. |
| Teacher closes a period with wrong numbers | Medium | High | Blocking-session gate, explicit totals in the confirm dialog, and adjustments-with-reason as the correction path (invoices are immutable after close, `docs/schema_design.sql:274`). |
| Clipboard API unavailable in a non-secure context | Medium | Medium | Textarea fallback plus an honest failure toast; the message text stays selectable on screen either way. |
| Teacher assumes copying sent the message | Medium | Medium | "Đã gửi" is a separate explicit action; the status chip stays "Chưa gửi" until it is pressed. |
| Bulk copy exceeds a practical clipboard size for 150 parents | Low | Medium | Bulk copy covers only unsent messages and shows the count being copied; per-row copy remains the primary path. |
| More than 5% of rows need manual adjustment (PRD leading metric) | Medium | High | Not a UI bug but a signal; surface the adjustment count in the period summary so it is observable from day one. |

**Rollback:** two additive feature folders plus route registrations. Reverting
the route registrations disables the month-end flow while leaving roster and
attendance intact. Nothing here writes persistent client state; every mutation
is a server call whose reversal path is defined server-side (void invoice,
reversing payment, opposite-sign adjustment — `docs/schema_design.sql:512`).
