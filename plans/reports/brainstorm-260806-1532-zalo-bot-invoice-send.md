# Brainstorm: Zalo integration — teacher sends invoice to contact

Status: **superseded** by
`brainstorm-260806-1611-zalo-personal-invoice-send.md` — user redirected auth to
a personal Zalo account (no OA, no Bot API). Kept as the record of option B in
that comparison.
Date: 2026-08-06

## Contract

- **Outcome:** teacher sends a per-student (per-invoice) tuition message to the
  student's contact directly over Zalo from the notifications page, automated
  via the official **Zalo Bot API** (`bot-api.zaloplatforms.com`), with the
  existing `zalo_manual` copy-paste flow as fallback for unpaired contacts.
- **Constraints:**
  - Official Zalo Bot API only (token-based, DM-only, 2000-char text limit).
    No unofficial personal-account APIs.
  - Bot cannot initiate contact: parent must message the bot once (pairing)
    before any send. Pairing maps a Zalo chat_id to a `contacts` row.
  - Keep the existing notifications `Sender` interface/registry
    (`apps/api/internal/features/notifications/sender.go`) — new channel slots
    in beside `zalo_manual`/`zalo_zns`.
  - Bot token is a secret → env config, never committed.
  - Reference implementation to port (via `/ak:xia --port`):
    https://github.com/nextlevelbuilder/goclaw/blob/dev/internal/channels/zalo/zalo.go
    (sendMessage + chunking + getMe validation; skip its bus/media/group logic).
- **Non-goals:** ZNS/ZBS business-OA integration (stub stays), SMS channel,
  chat features, parent login, group messaging.
- **Acceptance criteria:**
  1. Parent pairs by sending a teacher-issued code to the bot; contact row
     stores the chat_id; teacher sees paired status.
  2. Teacher triggers per-invoice send; paired contacts get the Zalo message
     (child name, sessions, absences, amount, detail link) ≤2000 chars.
  3. Notification ledger rows progress queued → sent/failed with provider
     message id / error; unpaired contacts fall back to manual copy flow.

## Decisions made (user)

- Channel: **Zalo Bot API** ("zalo personal" bot), not ZNS/ZBS. ZNS requires a
  verified business OA, approved templates, and per-message fees — rejected.
- Send unit: **per-invoice (per-student)**, not the per-contact family
  statement. ⚠ Diverges from PRD R5 (family-grouped message with household
  total) — PRD needs an update; multi-child contacts receive N messages.

## Evidence

- Schema anticipated Zalo: `notifications.channel` CHECK allows
  `zalo_zns|zalo_manual|sms`; `zalo_manual` fully wired (API + web page
  `apps/web/src/features/collections/pages/notifications-page.tsx`);
  `zalo_zns` is an ErrNotConfigured stub pending PRD Q1 — this work answers Q1.
- Message builder `apps/api/internal/features/statements/message.go` is a pure
  function with money formatting + length-collapse; per-invoice variant can
  reuse its helpers.
- Zalo Bot API verified via OpenClaw docs: pairing required, 2000-char limit,
  polling or HTTPS webhook, token from https://bot.zaloplatforms.com.

## Known schema/design impacts (for the plan)

- `contacts`: add nullable `zalo_chat_id` + pairing state; pairing codes table
  or short-lived code on contact.
- `notifications` FK is `statement_id` — per-invoice sends need an invoice
  reference (new column or parallel design). Channel CHECK needs `zalo_bot`.
- New per-invoice message renderer (single child, amount, detail link).
- Inbound path (webhook preferred; polling fallback) to capture pairing
  messages — new for the API app, which is currently request-driven only.
- Per-channel enum growth: migration for CHECK constraint.

## Unresolved questions

1. PRD R5 update: accept per-invoice divergence (multi-child contacts get one
   message per child, no household total)?
2. Pairing UX detail: code-per-contact vs phone-number matching (code is safer;
   phone matching risks spoofing).
3. Webhook vs polling for the bot inbound (webhook needs public HTTPS on the
   homelab deployment).
