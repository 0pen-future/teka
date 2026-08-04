---
phase: 3
title: "Message Builder and Notifications"
status: completed
priority: P1
effort: "6h"
dependencies: [2]
---

# Phase 3: Message Builder and Notifications

## Overview

Layer 1 of R5 and the whole of R6: the text a parent reads in Zalo, and the one
teacher action that produces it for every family in the period.

The bar R6 sets is blunt — a teacher with 50 students must finish sending in
under 10 minutes, and must never assemble a message by hand. Under D9's
`zalo_manual` channel the API cannot deliver anything itself, so it delivers the
next best thing: complete, copy-ready text per contact, plus a bulk copy, plus a
ledger of what was sent.

## Requirements

- R5 layer 1: per child — name, sessions attended, sessions absent, amount. Then
  old debt, family grand total, and the link.
- R5: a parent who never opens the link still knows what to pay.
- R6: **one action** produces content for every parent in the period.
- R6: the message fits the channel's length limit; if it cannot, the channel
  changes rather than the content (OQ-4).
- R7: a reminder to a parent with two children in debt is **one** message
  covering both.
- `notifications` rows per send: `channel` (`docs/schema_design.sql:437`),
  `purpose` (`:438`), `status` `queued → sent/delivered/failed` (`:440-441`).
- Contacts whose only invoice was voided receive nothing (PRD §5).

## Architecture

New package `apps/api/internal/features/notifications`. The message **builder**
lives in `statements` (it renders statement data and is unit-testable without
any notification concept); `notifications` owns queueing, sending and status.

### Message builder

```go
type MessageInput struct {
    ContactName string
    PeriodLabel string          // "08/2025"
    Children    []ChildSummary  // name, billable, absent, amount
    OpeningBalance  int64
    AdjustmentTotal int64
    TotalDue        int64
    Outstanding     int64
    URL             string
}

// Build renders the parent-facing message. It returns the text and whether the
// per-child breakdown had to be collapsed to fit maxLen.
func Build(in MessageInput, maxLen int) (text string, collapsed bool)
```

Full form, Vietnamese, the language the parent actually reads:

```
Chào anh/chị {ContactName},
Học phí tháng {PeriodLabel}:
- {Child A}: {billable} buổi học, {absent} vắng — {amount} đ
- {Child B}: {billable} buổi học, {absent} vắng — {amount} đ
Nợ cũ: {opening} đ
Tổng cộng: {total_due} đ
Chi tiết từng buổi: {URL}
```

Rules baked into the builder, each for a reason:

- Absent lines are printed only when `absent > 0`, so the common case stays
  short.
- `Nợ cũ` (old debt) is printed only when `opening_balance != 0` — printing
  "Nợ cũ: 0 đ" invites the reply "I don't owe anything".
- An adjustment line (`Điều chỉnh`) appears only when `adjustment_total != 0`,
  with its sign, because an unexplained total that differs from the sum of the
  children is the fastest way to lose a parent's trust.
- Money formats as grouped thousands with a `đ` suffix; never a float
  (D5).
- The URL is always last and is never dropped, including in the collapsed form.

Degradation when `len(text) > maxLen` (OQ-4): replace the per-child lines with
`{n} bạn, tổng {sum} buổi học`, keep old debt, total and URL, set
`collapsed=true`. The caller surfaces `collapsed` in the response so the teacher
knows the detail moved to the link. Default ceiling 1000 characters, configurable
as `cfg.Notifications.MaxMessageLen`.

`Build` is a pure function over a struct. No database, no clock, no config
lookup — the golden-file tests in step 7 are the specification.

### Send channels (D9)

```go
type Sender interface {
    Channel() string
    Send(ctx context.Context, n *Notification) error
}
```

- `manualSender` — `Channel() == "zalo_manual"`. `Send` is a no-op that leaves
  the row `queued`; the teacher copies the text and marks it sent. This is the
  V1 default and the only wired implementation.
- `znsSender` — `Channel() == "zalo_zns"`, returns `ErrNotConfigured` until PRD
  Q1 is answered. Present so switching channels is a config change, not a
  refactor.
- `sms` is in the CHECK list (`docs/schema_design.sql:437`) with no
  implementation; attempting it returns `apperror.BadRequest`.

The status lifecycle under `zalo_manual` is honest about what the system
actually knows: `queued` when the text exists, `sent` only when the teacher says
so, `delivered` never (no receipt exists — that is the P2 item at
`docs/schema_design.sql:601`).

### Bulk send (R6)

`POST /billing-periods/:id/notifications/bulk` with
`{ purpose: "statement" | "reminder", channel?: "zalo_manual" }`.

One transaction:

1. Require the period `closed`.
2. Call phase 1's `Generate` so statements exist and totals are current. This is
   what makes bulk send a single teacher action.
3. Select target contacts:
   - `purpose='statement'` — every contact with a non-void invoice in the period;
   - `purpose='reminder'` — the same set filtered to `outstanding > 0` from
     `v_contact_balance` (`docs/schema_design.sql:459`).
4. For each contact, build `MessageInput` from the statement render data (phase
   2's assembly, reused — not a second implementation of the same sums) and call
   `Build`.
5. Insert one `notifications` row per contact: `channel`, `purpose`,
   `status='queued'`, `message_text`, `statement_id`, `contact_id`.
6. Return every row with the contact's name and phone, the message text, the
   statement URL, and `collapsed`.

**One row per contact per send, never one per child** — that is R7's acceptance
criterion expressed in the data model, and step 3's `DISTINCT contact_id` is the
line that enforces it.

Repeated bulk sends are allowed and each creates a new row: the ledger records
attempts, not intent. `sent_at` distinguishes them.

### Marking sent

`POST /notifications/mark-sent` with `{ ids: [...] }` sets `status='sent'` and
`sent_at=now()` for rows still `queued`. Idempotent; ids already `sent` are
skipped, not errored. The teacher pastes into Zalo, comes back, and taps once.

`GET /billing-periods/:id/notifications?purpose=&status=` lists the ledger so
the teacher can see who is still unsent — the "under 10 minutes for 50 students"
workflow needs a visible remaining count more than it needs speed.

## Related Code Files

Create:

- `apps/api/internal/features/statements/message.go` — `Build`, `MessageInput`
- `apps/api/internal/features/statements/message_test.go`
- `apps/api/internal/features/statements/testdata/` — golden message files
- `apps/api/internal/features/notifications/model.go`
- `apps/api/internal/features/notifications/repository.go`
- `apps/api/internal/features/notifications/sender.go` — `Sender`,
  `manualSender`, `znsSender`
- `apps/api/internal/features/notifications/service.go`
- `apps/api/internal/features/notifications/dto.go`
- `apps/api/internal/features/notifications/handler.go`
- `apps/api/internal/features/notifications/routes.go`
- `apps/api/internal/features/notifications/integration_test.go`

Modify:

- `apps/api/internal/features/statements/service.go` — export the render data
  assembly so `notifications` can reuse it without importing the handler
- `apps/api/internal/config/config.go` — add `Notifications.DefaultChannel`
  (default `zalo_manual`) and `Notifications.MaxMessageLen` (default 1000)
- `apps/api/internal/server/router.go` — register the feature in
  `registerFeatures` (`apps/api/internal/server/router.go:63-73`)

Delete: none. No migration files.

## Implementation Steps

1. Write `message.go` with `Build` exactly as specified. Keep it free of
   imports beyond `fmt`, `strings` and the money formatter.
2. Write `message_test.go` as golden-file tests covering: one child no absences;
   one child with absences; two children; old debt present; old debt zero
   (line absent); a negative adjustment; a positive adjustment; a message that
   exceeds `maxLen` and collapses; the collapsed form still containing the URL.
   Assert the URL is the last line in every case.
3. Create `model.go` for `notifications` (`docs/schema_design.sql:432-452`),
   with `gorm.DeletedAt`, and Go constants for every `channel`, `purpose` and
   `status` value in the CHECK constraints — no string literals at call sites.
4. Create `sender.go` with the interface and the two implementations. A
   `map[string]Sender` registry resolves the channel; an unknown channel is
   `apperror.BadRequest`.
5. In `statements/service.go`, extract the phase 2 payload assembly into an
   exported method returning the per-contact figures. `notifications` calls it.
   Do not duplicate the sums — a second implementation is how the message and
   the page start disagreeing.
6. Create `repository.go`: `InsertBatch`, `ListByPeriod`, `MarkSent`,
   `TargetContacts(periodID, purpose)`. All teacher-scoped, all
   `deleted_at IS NULL`.
7. Create `service.go` with `BulkSend`, `List`, `MarkSent`. `BulkSend` runs
   inside `tx.WithinTx` (`apps/api/internal/database/tx_manager.go:11`) and
   calls the statements service's `Generate` first.
8. Create `dto.go`, `handler.go`, `routes.go` for the three endpoints, behind
   `requireAuth`. `BulkSendResponse` carries `queued_count`,
   `skipped_paid_count`, `collapsed_count` and the rows. Include a
   `bulk_text` field: every message joined by a separator, so the teacher can
   copy once when pasting into a broadcast tool.
9. Register in `registerFeatures`.
10. Write `integration_test.go`. Seed one closed period with: contact A, two
    children in two classes, unpaid; contact B, one child, paid in full;
    contact C, one child whose only invoice was voided; contact D with old debt
    carried in `opening_balance`.
    Assert:
    - `purpose=statement` bulk send creates exactly three rows (A, B, D) and
      none for C (PRD §5);
    - contact A gets **one** row whose text names both children and whose total
      equals the sum of both invoices (R5, R6);
    - contact D's message contains the `Nợ cũ` line; contact A's does not;
    - `purpose=reminder` creates rows for A and D only, not B (R7);
    - a family with two children both in debt gets one reminder (R7 acceptance);
    - every message contains a working statement URL — fetch it against the
      phase 2 endpoint and assert `200`;
    - marking sent twice is idempotent and does not change `sent_at`;
    - bulk send on an open period returns `409` and writes nothing;
    - `channel=zalo_zns` returns a clear "not configured" error and writes
      nothing;
    - the message text of one contact contains no other contact's or child's
      name.
11. Add a scale check: 50 contacts, 80 students, one bulk send. Assert the call
    completes under 3 seconds and issues a bounded number of queries independent
    of contact count (batch insert, one render query set).
12. Run `go test ./apps/api/internal/features/statements/... ./apps/api/internal/features/notifications/... -race`.

## Success Criteria

- [x] R5: the message alone tells a parent each child's session count and the
      amount due.
- [x] R5/R6: a two-child family receives one message with one total.
- [x] R6: one endpoint call generates statements and queues every contact in the
      period.
- [x] R6: messages stay within the configured ceiling; over-long ones collapse
      and keep the link, and the response reports how many collapsed.
- [x] R7: a reminder run targets only contacts with `outstanding > 0`, one row
      each.
- [x] Contacts with only voided invoices receive no notification.
- [x] Status transitions are limited to `queued → sent`; `delivered` is never
      claimed under `zalo_manual`.
- [x] Mark-sent is idempotent.
- [x] Every queued message contains a URL that resolves to a live statement.
- [x] 50 contacts complete in one call under 3 seconds.

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Message total disagrees with the link's total | Medium | Critical | Both read the same exported render assembly (step 5); integration test compares the number in the text against the endpoint's `total_due` |
| A parent receives another family's data in the bulk text | Low | Critical | Rows built per contact in a loop with contact-scoped inputs; explicit cross-contamination assertion in step 10 |
| One row per child instead of per contact, so a two-child parent gets two messages | Medium | High | `DISTINCT contact_id` target selection; R7 assertion in step 10 |
| ZNS length limit unknown, so messages are rejected once the channel switches | High | Medium | Configurable ceiling with collapse-and-keep-link degradation; OQ-4 tracks the real limit, which needs PRD Q1 answered |
| Teacher sends twice and a parent gets a duplicate | Medium | Low | Allowed by design (the ledger records attempts); the list endpoint shows what is already `sent` so the teacher can filter before the second run |
| Bulk send N+1 across contacts makes the one action slow | Medium | Medium | Batch insert plus one render query set for the whole period; scale check in step 11 |
| `delivered` set optimistically, so the teacher believes a message arrived | Low | High | `manualSender` cannot set `delivered`; the constant exists only for the future ZNS path |
| Vietnamese diacritics mangled in the copied text | Low | Medium | UTF-8 end to end, asserted by comparing golden files byte for byte |

**Rollback.** `notifications` rows are inert records; nothing leaves the system
under `zalo_manual`, so a bad send is a bad copy-paste, not a delivered message.
Back out by removing the routes; statement links continue to work independently.
