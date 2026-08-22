# Brainstorm: Zalo personal send — UX/UI design

Status: design proposed, ready for `/ak:plan` once forks 1–3 confirmed
Date: 2026-08-06
Builds on: `brainstorm-260806-1611-zalo-personal-invoice-send.md` (technique accepted)
Mockup: `brainstorm-260806-1626-zalo-personal-ux.html` (annotated, self-contained)

## Contract (UX slice)

- **Outcome:** teacher links their Zalo account once, confirms an auto-matched
  contact↔friend mapping once, then every month taps one button and watches a
  paced send complete — with the existing copy-paste flow intact underneath at
  every step.
- **Constraints:**
  - Vietnamese UI; `components/hv` design system (`HvCard`, `HvButton`,
    `HvModal`, `StatusPill`, `ProgressBar`, `hvToast`) + tokens in
    `styles/tokens/colors.css`. No new primitives.
  - Responsive shell already fixed: sidebar ≥lg, icon rail md–lg, **bottom tab
    bar <md** → mobile is a first-class target.
  - No SSE/WebSocket anywhere in the web app; server state = TanStack Query.
  - No settings page and no account menu exist (only a logout button).
- **Non-goals:** a general settings section, multi-account Zalo, inbound
  chat/replies UI, parent-facing changes, redesign of `MessageCard`'s message body.
- **Acceptance criteria:**
  1. Teacher completes link + mapping without typing a contact name (confirm-only path).
  2. A paced run reports live progress, survives leaving the page, and resumes after restart.
  3. Every failure mode (unlinked, expired, unmapped, send error) still yields a
     copyable message — the page never dead-ends.
  4. Full keyboard + screen-reader path on QR modal and mapping list.

## The core insight

**Mapping is the whole UX.** Everything else is routine. Zalo's friend record
carries `userId` + `displayName` and *no phone*, while Teka keys contacts by
phone — so ~40 contacts must each be bound to a Zalo friend. If that reads as a
40-row data-entry chore, the feature is dead on arrival regardless of how good
the protocol port is. The design goal is therefore: **teacher confirms, never types.**

Second principle: **Zalo is an accelerator bolted onto the manual flow, never a
replacement.** `Sao chép` stays on every card in every state. This is what makes
the whole risky integration safe to ship — worst case it degrades to today's app.

Third: **explain the slowness.** A 40-contact run at 3–8s pacing takes ~4 minutes.
Unexplained, that reads as a hang. The pacing is a *safety feature* and the UI
must say so, in the teacher's words: "gửi chậm để Zalo không khoá tài khoản".

## Surfaces

### 1. Entry point — on the notifications page, not a new nav item

No settings page exists; adding a 7th nav destination for one toggle violates
YAGNI. The Zalo connection lives as a **status strip at the top of "Gửi thông
báo"**, where the job is actually done. Three states:

| State | Strip | Primary action |
|---|---|---|
| Not linked | cream-100, neutral | `Kết nối Zalo` → consent+QR modal |
| Linked, healthy | white + avatar, `28/40 đã liên kết` | `Gửi qua Zalo (28)` |
| Session expired | coral-100, warning | `Quét lại mã QR`; send disabled, copy alive |

Promote to a real settings page only when a second integration arrives.

### 2. Consent + QR modal (`HvModal` — bottom sheet <sm, centered ≥sm)

Step 1 **consent** (once, ever): plain-language risk statement — Teka signs in as
*your* Zalo account; Zalo may lock accounts that send in bulk; you can unlink
anytime. Explicit checkbox; `Tiếp tục` disabled until checked. Honest, not
alarmist, not buried in a tooltip.

Step 2 **QR**: PNG from `LoginQR`, 100s countdown ring, states
`đang tạo → chờ quét → đã quét, xác nhận trên điện thoại → xong`, plus expired
→ `Tạo mã mới`.

> **Mobile trap worth naming:** on a phone the teacher is *already* on the device
> that holds the Zalo app, so they cannot scan a QR shown on that same screen.
> Under `md` the modal must offer **`Lưu ảnh QR`** (download) with the
> instruction to open Zalo → quét mã → chọn ảnh từ thư viện. Missing this makes
> the feature mobile-unusable, and the bottom tab bar proves mobile is a target.

### 3. Mapping screen — confirm, don't type

Reached from the strip (`28/40 đã liên kết`) or automatically right after a
successful link. Auto-match runs `contacts.full_name` against Zalo
`displayName`, diacritics-normalized, token-overlap scored, and buckets:

```
✓ Khớp chắc chắn (24)          [ Xác nhận tất cả ]   ← one tap clears 60%
   Nguyễn Thị Lan  →  Lan Nguyễn          [đổi]
⚠ Cần kiểm tra (9)             each row: pre-filled searchable picker
   Mẹ bé Bảo       →  [ Trần Thu Hà  ▾ ]  [bỏ qua]
✗ Chưa tìm thấy (7)            search picker + [Bỏ qua — gửi thủ công]
```

`Bỏ qua` is first-class, not a failure: that contact simply stays on the manual
path forever, and its card says so. New contacts added later get a just-in-time
prompt on their card rather than forcing a return to this screen.

### 4. The paced run

Confirm modal states the honest numbers before starting: `Gửi 28 tin nhắn
· khoảng 3 phút`, plus the pacing rationale and a reminder that 12 unmapped
contacts stay manual.

During the run, a sticky progress card: `ProgressBar` (mint) + `Đang gửi 12/28
· còn khoảng 2 phút`, `Tạm dừng` / `Dừng`, and the reassurance
**"Bạn có thể rời khỏi trang — hệ thống vẫn gửi tiếp."** That sentence is only
truthful if the run is server-side; see fork 3.

Card order stays **stable** during a run — re-sorting as rows complete is
disorienting. Triage happens through filter chips instead:
`Tất cả · Chưa gửi · Lỗi · Chưa liên kết`.

### 5. `MessageCard` states (extends today's sent/unsent)

| State | Visual | Actions |
|---|---|---|
| `Chưa liên kết Zalo` | sun-100 pill | `Liên kết` · `Sao chép` |
| `Chờ gửi` | ink-400 dot | `Sao chép` |
| `Đang gửi` | sky-300 pulse (respect `prefers-reduced-motion`) | — |
| `Đã gửi 14:32` | mint-50 ✓ | `Sao chép` |
| `Lỗi: <reason>` | coral-100 | `Thử lại` · `Sao chép` |

A failure is per-contact and recoverable; it never aborts the run.

## Forks needing your call

**1. Entry point** — notifications-page strip *(recommended, YAGNI)* vs. a new
`Cài đặt` nav destination.

**2. Mapping moment** — post-link review screen *(recommended: bulk auto-match is
where the leverage is)* vs. purely just-in-time per card *(less upfront work, but
the first run then interrupts the teacher 40 times)*.

**3. Who paces the run** — this is the one with real backend consequence:

- *Server-side goroutine per run (recommended):* survives tab close, phone
  lock, app switch; ledger rows persist as it advances; a restart leaves the
  remainder `queued` and the teacher taps `Tiếp tục`. Costs the API its first
  background component and makes the "you can leave" promise true.
- *Client-driven loop:* no backend concurrency at all, but a locked phone or
  switched app silently halts a collection run. Given the bottom tab bar, most
  runs will happen on a phone — this is likely a false economy.

Note either way: `BulkSend` currently calls `sender.Send` **inside** `WithinTx`.
A paced run must move sends out of the transaction — a multi-minute Postgres
transaction is not acceptable.

## Unresolved questions

1. Forks 1–3 above.
2. Does Zalo's `getfriends` response include a phone field goclaw doesn't decode?
   If yes, auto-match becomes exact and the "Cần kiểm tra"/"Chưa tìm thấy"
   buckets mostly vanish. Unverifiable without live credentials.
3. Pause/resume: is `Tạm dừng` worth building in v1, or is `Dừng` + `Tiếp tục` enough?
4. Should an unlink action wipe stored friend mappings, or keep them for a
   future re-link? (Keeping them is kinder; the UIDs stay valid per account.)
