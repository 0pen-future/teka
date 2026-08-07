# Brainstorm: Zalo **personal-account** invoice send (supersedes 1532 Bot-API direction)

Status: direction proposed, **needs user decision on risk acceptance** before `/ak:plan`
Date: 2026-08-06
Supersedes: `brainstorm-260806-1532-zalo-bot-invoice-send.md` (Bot API direction)
Source studied: `/ak:xia --compare` on
`github.com/nextlevelbuilder/goclaw@dev` → `internal/channels/zalo/**` (44 files, ~69k tokens)

## Contract (revised)

- **Outcome:** teacher sends the per-invoice tuition message to a student's
  contact over Zalo, delivered **from the teacher's own personal Zalo account**
  (parent sees a normal DM from their teacher), triggered from the notifications
  page. `zalo_manual` stays as fallback.
- **Constraints:**
  - Auth = personal account via QR login (zcago-style protocol port). **No OA,
    no ZNS, no Bot API** — user decision.
  - Session credentials (IMEI + cookie jar) are full account-takeover material →
    must be encrypted at rest; Teka has no crypto helper today.
  - Keep the `Sender` interface/registry (`notifications/sender.go`); new
    `zalo_personal` channel slots beside `zalo_manual`/`zalo_zns`.
  - Single API instance (homelab, Traefik, one `api` service) — a process-held
    session per teacher is viable; horizontal scale would break it.
- **Non-goals:** ZNS/OA, SMS, group send, inbound chat/replies, media, parent
  login, multi-device session sharing.
- **Acceptance criteria:**
  1. Teacher links their Zalo account by scanning a QR in the web app; creds
     persist encrypted; re-login on restart is automatic (no re-scan).
  2. Teacher maps each contact to a Zalo friend once (picker UI); mapping stored
     on `contacts`.
  3. Per-invoice send delivers a DM to mapped contacts, paced (throttled) to
     avoid anti-spam flags; ledger rows go queued → sent/failed with Zalo msgId
     or error; unmapped contacts fall back to manual copy.
  4. Zalo credentials never appear in logs, API responses, or git.

## Source anatomy (goclaw zalo/personal)

Send-only subset actually needed (~1,400 LoC of reverse-engineered Go):

| File | Purpose | Needed |
|---|---|---|
| `protocol/config.go` | `Credentials`, cookie union, jar building, API constants | yes |
| `protocol/crypto.go` | AES-CBC payload encrypt/decrypt | yes |
| `protocol/client.go` | session, `makeURL`, signkey, IMEI, headers | yes |
| `protocol/models.go` | `LoginInfo`, `Response[T]` envelope, service map | yes |
| `protocol/auth.go` | `LoginWithCredentials` + `LoginQR` (long-poll scan/confirm) | yes |
| `protocol/send.go` | `SendMessage` (DM path only; drop group + typing) | yes |
| `protocol/send_helpers.go` | form body, JSON read | yes |
| `protocol/contacts.go` | `FetchFriends` (drop all group functions) | partial |
| `protocol/listener*.go`, `ws_client.go`, `message*.go` | inbound WS chat | **no** |
| `send_file.go`, `send_image.go`, `content-media.go` | media | **no** |
| `personal/channel.go`, `handlers.go`, `policy.go`, `factory.go`, `zalo.go` | goclaw bus/agent plumbing | **no** |

Auth chain: QR (`id.zalo.me/account/authen/qr/*`, ~100s long-poll) → cookies +
generated IMEI → `Credentials` JSON → `LoginWithCredentials` yields
`zpw_enk` secret key + service-map URLs → every call is AES-CBC-encrypted
`params=` over form POST.

Send: `POST {chat_service}/api/message/sms`, payload `{message, toid, imei,
clientId, ttl}`, encrypted; returns encrypted `{msgId}`.

goclaw itself logs `security.unofficial_api` on start: *"Zalo Personal is
unofficial and reverse-engineered. Account may be locked/banned."*

## Dependency matrix (source → Teka)

| Source concern | Teka today | Verdict |
|---|---|---|
| Channel registry | `senderRegistry()` map | **EXISTS** — add `zalo_personal` |
| Credential store | none (no encryption helper anywhere in `internal/`) | **NEW** — encrypted column + KEK from env |
| Long-lived session | app is pure request/response, no workers | **NEW** — lazy per-teacher session cache |
| QR interactive flow | no SSE/WS anywhere | **NEW** — poll endpoint or SSE |
| Recipient identity | `contacts.phone` + `full_name` | **CONFLICT** — `FriendInfo` has **no phone**; needs manual mapping |
| Message text | `statements.Build` (pure, length-collapse) | **EXISTS** — needs per-invoice variant |
| Ledger | `notifications` FK = `statement_id` | **CONFLICT** — per-invoice send needs invoice ref + `channel` CHECK migration |
| Rate limiting | none | **NEW** — source has *zero* throttling |

## Challenge (Phase 4)

| # | Question | Source answer | Teka answer | Risk if wrong |
|---|---|---|---|---|
| 1 | What happens if Zalo bans the account? | Warns in a log line, no handling | Teacher's **primary business comms channel** dies | Catastrophic, non-recoverable by us. Bulk DM bursts are the exact anti-spam signature. |
| 2 | Where do cookies live? | Plaintext JSON, file `0600` or DB column | Teka DB has no encryption; homelab Postgres + backups | Stolen row = full Zalo account takeover, incl. teacher's private chats |
| 3 | How is a recipient resolved? | `FetchFriends` → `userId`+`displayName` only | Teka keys contacts by **phone** | No auto-mapping. Teacher must hand-link every contact. Name collisions likely ("Mẹ Bảo"). |
| 4 | How is the session refreshed? | Retry cookie login, else QR | No worker to detect expiry; teacher discovers on send failure | Silent failure mid-collection-run unless expiry is surfaced proactively |
| 5 | What pins protocol correctness? | Nothing — Zalo web client can change any day | Same | Feature breaks with no deprecation notice; no vendor SLA, no support path |
| 6 | Send pacing? | None; sends as fast as called | Bulk send loops all contacts in one tx | 30 DMs in 2s ≈ spam flag. Throttling is **not optional**. |
| 7 | Multi-instance? | goclaw is single-process | Homelab = 1 replica today | Fine now; blocks any future scale-out |

## Approaches

**A. Port the send-only personal subset into the API** *(user's stated direction)*
- +No parent action ever; message comes from the teacher's real account (best read rate); no per-message fee; **no inbound webhook needed** (send-only — this is the one place personal beats Bot API on complexity).
- −ToS violation, ban risk on the teacher's own account, ~1,400 LoC unversioned protocol, new crypto + session-cache + QR-flow subsystems, manual friend mapping.
- Cost: large. Risk: **high (7/10)**.

**B. Official Zalo Bot API** *(previously accepted, 1532 report)*
- +Official, token-based, no ban risk, no reverse engineering, ~150 LoC. Note: a Bot-API token is **not** an OA/ZNS — no business verification, no template approval, no fee.
- −Parent must DM the bot once to pair; message arrives from a bot, not the teacher. For non-technical parents that pairing step is a real adoption blocker.
- −Requires an inbound path (webhook needs public HTTPS, or polling).
- Cost: small. Risk: low (2/10).

**C. Assisted manual (deep link)**
- +Near-zero cost/risk: keep `zalo_manual`, add a per-contact `https://zalo.me/<phone>` button + auto-copied text.
- −Teacher taps once per contact; not automation.
- Cost: tiny. Risk: 1/10.

## Recommendation

Proceed with **A**, but only under hard guardrails, and **keep C shipped as the
always-available fallback**. A's guardrails are acceptance criteria, not
nice-to-haves:

1. Opt-in per teacher with an explicit in-app risk acknowledgement (ban wording).
2. Credentials encrypted with AES-GCM under a KEK from env (`ZALO_CRED_KEY`),
   never returned by any endpoint, never logged.
3. Send pacing: serial sends, randomized 3–8s gap, per-run cap; sends move
   **out of the DB transaction** (current `BulkSend` calls `sender.Send` inside
   `WithinTx` — a 30-contact throttled run would hold a Postgres tx for minutes).
4. Friends-only delivery; unmapped contact ⇒ automatic `zalo_manual` fallback,
   never an error.
5. Health probe surfaces session-expired before a collection run, not during.

Honest counsel: **B is the lower-risk engineering answer** and I'd pick it if the
pairing barrier weren't a product blocker. The deciding factor is whether the
teacher accepts that a Zalo lockout would hit their personal account. That is
your call, not mine — flagging it because it is irreversible and the source repo
itself warns about it.

## Evidence gaps

- Whether Zalo's `getfriends` response carries `phoneNumber` (goclaw decodes
  only 4 fields). If it does, contact→UID mapping can auto-match on phone and
  challenge #3 mostly dissolves. **Unverifiable without live credentials.**
- Real-world Zalo rate thresholds — no public documentation; pacing values are
  a guess until observed.

## Unresolved questions

1. Accept the personal-account ban risk on the teacher's own Zalo (A), or
   revert to the official Bot API with parent pairing (B)?
2. Per-invoice vs per-contact family message — still open from the 1532 report
   (diverges from PRD R5, multi-child contacts get N messages).
3. Contact↔Zalo-friend mapping UX: manual picker (safe) vs phone auto-match
   (depends on gap #1).
4. `ZALO_CRED_KEY` management on the homelab deploy — env var, or a file mount?
