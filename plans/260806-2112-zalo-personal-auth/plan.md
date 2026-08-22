---
title: "Zalo Personal Auth"
description: "Teacher links their personal Zalo account via QR from the profile page; credentials persist encrypted and re-login is automatic."
status: completed
priority: P1
effort: "4-6d"
tags: [zalo, auth, security, backend, web]
created: 2026-08-06
blockedBy: []
blocks: []
---

# Zalo Personal Auth

## Overview

Vertical slice that lets a teacher **link their own personal Zalo account** from
the profile page. Teacher taps `Đăng nhập với Zalo`, a QR modal opens, they scan
with the Zalo app, and on confirm the API persists the session **encrypted at
rest** and re-logins automatically on restart. This is the authentication
foundation for the personal-account send feature — sending, contact↔friend
mapping, and the paced run are **explicitly out of scope** here and remain in the
brainstorm reports below as the next milestone.

The Zalo personal protocol is unofficial/reverse-engineered (ported from
[goclaw@dev `internal/channels/zalo/personal`](https://github.com/nextlevelbuilder/goclaw/tree/dev/internal/channels/zalo), itself ported from zcago/MIT). The session
credentials (IMEI + cookie jar) are **full account-takeover material**, so
encryption, no-logging, and opt-in consent are acceptance criteria, not
nice-to-haves.

**Source reports (context, decisions already made):**
- `plans/reports/brainstorm-260806-1611-zalo-personal-invoice-send.md` — technique accepted (Approach A), risk guardrails.
- `plans/reports/brainstorm-260806-1626-zalo-personal-ux.md` + `.html` — UX design, consent+QR modal, mobile QR trap.

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Port the **auth-only** subset of the Zalo personal protocol (crypto, session, QR login, cookie re-login) as a quarantined Go package with its ported unit tests | P1 |
| 2 | Persist session credentials **encrypted (AES-GCM)** under a KEK from env; never returned by any endpoint, never logged | P1 |
| 3 | Server-driven QR link flow (poll-based, no SSE) with an in-process per-teacher session cache, automatic re-login from stored creds, and a periodic health probe that surfaces expired sessions proactively | P1 |
| 4 | Profile page `Kết nối Zalo` card drives the real consent+QR flow with a mobile-usable path and honest linked/expired states | P1 |

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Phase 1: Protocol auth port](./phase-01-protocol-auth-port.md) | Completed |
| 2 | [Phase 2: Credential encryption and storage](./phase-02-credential-encryption-and-storage.md) | Completed |
| 3 | [Phase 3: Session manager and QR link flow](./phase-03-session-manager-and-qr-link-flow.md) | Completed |
| 4 | [Phase 4: HTTP API endpoints and wiring](./phase-04-http-api-endpoints-and-wiring.md) | Completed |
| 5 | [Phase 5: Frontend profile Zalo connect](./phase-05-frontend-profile-zalo-connect.md) | Completed |

Phases are strictly sequential: 2 depends on 1, 3 on 1+2, 4 on 3, 5 on 4.

## Architecture at a glance

```
Profile page ── consent+QR modal (HvModal)
   │  POST /me/zalo/link/start  ──► link_id   (consent_version in body; QR arrives via first poll)
   │  GET  /me/zalo/link/status?id=  ◄── poll ~1.5s: pending|qr_ready|scanned|confirmed|linked|expired|error
   │  GET  /me/zalo   (status card)     DELETE /me/zalo (unlink)
   ▼
features/zalo (service)
   ├── LinkManager  ── in-memory map[link_id]→{state, qrPNG, goroutine}      (Phase 3)
   ├── SessionCache ── in-memory map[teacherID]→*protocol.Session            (Phase 3)
   ├── HealthProbe  ── periodic sweep: verify sessions, mark expired early   (Phase 3)
   ├── Repository   ── zalo_accounts (encrypted_credentials bytea + consent) (Phase 2)
   └── protocol/    ── quarantined reverse-engineered port                   (Phase 1)
            crypto.go · config.go · client.go · models.go · auth.go · doc.go
            (send-only helpers getServiceURL/encryptPayload/decryptDataField fold into client.go)
   shared/crypto    ── AES-GCM envelope, KEK from API_ZALO_CRED_KEY          (Phase 2)
```

## Non-goals (this plan)

- Sending messages (`SendMessage`), contact↔Zalo-friend mapping, the paced bulk
  run, `MessageCard` state changes — the next milestone; keep `zalo_manual` /
  copy-paste as the shipped path meanwhile.
- Inbound chat/WebSocket listener, media, groups — never ported.
- Horizontal scale of the API: the in-process session cache assumes the single
  homelab replica (documented constraint, not a defect).
- A general settings page or new nav destination.

## Key decisions (defaults chosen; reversible)

- **KEK delivery:** `API_ZALO_CRED_KEY` env var, mirroring the existing
  `API_STATEMENTS_TOKEN_KEY` pattern (hex/base64, ≥32 bytes). Not a file mount —
  resolves brainstorm 1611 Q4 with the established convention.
- **QR transport:** short-polling via TanStack Query, matching the app's
  "server state = TanStack Query, no SSE anywhere" constraint.
- **Package location:** feature at `internal/features/zalo`; the vendored
  reverse-engineered protocol quarantined under `internal/features/zalo/protocol`.
- **Storage shape:** new `zalo_accounts` table (one row per teacher, `teacher_id`
  as PK — one Zalo account per teacher is a permanent v1 constraint, reversible
  later by migration), not a column on `teachers` — keeps takeover-grade secrets
  in an isolated table with its own access path.
- **QR progress granularity:** thread an additive progress callback into the
  ported `LoginQR` so the client sees a distinct `scanned`/`confirmed` step, not
  only `qr_ready → linked`. (Validation session 1 — reverses Phase 3's original
  coarse-state recommendation.)
- **Consent audit trail:** persist `consent_at` + `consent_version` on
  `zalo_accounts`; `link/start` carries the acknowledged consent version. Consent
  is an auditable record, not only a UI gate. (Validation session 1.)
- **Expired-session detection:** a periodic in-process health probe verifies
  linked sessions and flips `status` to `expired` proactively, so the profile
  card is honest without waiting for the next send. This is the API's **second**
  background component. (Validation session 1 — reverses Phase 3's lazy-only
  recommendation.)

## Success Criteria

- [x] Teacher links their Zalo account by scanning a QR in the web app; on
      confirm the card shows `Đã kết nối · <tên Zalo>`. Verified:
      `zalo-connect-card.tsx` linked branch + `zalo-connect-card.test.tsx`;
      confirmed by phase-05 code review criterion 4.
- [x] Credentials persist AES-GCM encrypted; the plaintext IMEI/cookies never
      appear in any API response, log line, or error string. Verified:
      `internal/shared/secrets/secrets.go` (AES-256-GCM), `zalo_accounts.encrypted_credentials`
      is the only credential column (migration `000004`), response DTOs hand-built
      with a reflection test (`handler_test.go:409-429`). The one gap found —
      plaintext IMEI reaching a log line on a transport-error path (review finding
      C-1) — was fixed via `doRequest` in `protocol/client.go`, which strips the
      query string before the error is logged; regression-tested by
      `TestFetchers_TransportErrorOmitsCredentials`.
- [x] After an API restart, the next action that needs the session re-logins
      automatically from stored creds with no re-scan. Verified: `sessionFor` in
      `service.go` (cache miss → `LoginWithCredentials` from stored creds),
      `service_test.go` cache/reload branches (Phase 3, already completed).
- [x] Session-expired is surfaced as a distinct card state with a `Quét lại mã`
      action; the profile page never dead-ends. The periodic health probe flips a
      dead session to `expired` proactively, without waiting for a send. Verified:
      `health_probe.go` + `health_probe_test.go` (Phase 3), `zalo-connect-card.tsx`
      expired branch.
- [x] The link modal shows a distinct `Đã quét · chờ xác nhận` step after the QR
      is scanned, before the account is confirmed linked. Verified:
      `protocol.QRState` scanned/confirmed (`auth.go`), `LinkManager` progress
      wiring (Phase 3), `zalo-link-modal.tsx:145-157` + phase-05 review criterion 3.
- [x] Every linked account records `consent_at` + `consent_version`; the linked
      row cannot exist without an acknowledged consent version. Verified:
      `zalo_accounts.consent_version` is `NOT NULL` (migration `000004`),
      `Repository.Upsert` returns `ErrConsentVersionRequired` on a blank version
      (`repository.go:59-62`), `TestUpsertRejectsMissingConsentVersion`
      (`integration_test.go:117`), handler 400s on blank `consent_version`
      (`handler_test.go:240-259`).
- [x] On a phone, the modal offers `Lưu ảnh QR` (download) so a teacher on the
      same device can still complete the scan. Verified: `zalo-link-modal.tsx:188-194`,
      phase-05 review criterion 5. Note: iOS Safari ignores `download` on a
      `data:` URI (review finding M6); mitigated with `target="_blank"
      rel="noopener"` so the QR still opens in a new tab instead of replacing the
      running attempt. A full fix (Blob URL) was deferred as disproportionate to
      the remaining gap — see phase-05 review disposition.
- [x] `go test ./...` (API) and `npm test` (web) green for touched packages;
      ported protocol unit tests pass. Verified: `go build ./...`, `go vet ./...`
      (with and without `-tags=integration`), `go test ./...`, `go test -race
      -count=3` on `./internal/features/zalo/...`, and `golangci-lint v2.7.2` all
      clean. `go test -tags=integration ./...` was also run against real
      Postgres (every package, including `zalo`, `zalo/protocol`, and
      `shared/secrets`) and is clean, with `go vet -tags=integration ./...`
      clean too — this is the strongest evidence for the encrypted-credential
      storage path, since it exercises `integration_test.go` against a real
      database rather than a stub. Web: 30 files / 146 tests pass (final count
      after the phase-05 review round — see that report's disposition for the
      interim corrections), `eslint` 0 errors, `tsc -b --noEmit` exit 0,
      `prettier --check` clean, `npm run build` succeeds.

## Validation Log

### Verification Results (session 1)

- Tier: Full (5 phases).
- Claims checked: 11 load-bearing. Verified: 11 · Failed: 0 · Unverified: 0.
- Verified against the codebase:
  - `decodeTokenKey`, `validateStatements`, `StatementsConfig`, `API_STATEMENTS_TOKEN_KEY`, `minStatementTokenKeyLen = 32` — `internal/config/config.go` (Phase 2 mirror target).
  - Latest migration is `000003`; `000004` is free (Phase 2).
  - `authctx.TeacherID(c *gin.Context) (uuid.UUID, bool)` — `internal/shared/authctx/authctx.go:57` (note: it takes `*gin.Context` and returns `(uuid.UUID, bool)`, not `authctx.TeacherID(ctx)`; Phase 4 prose corrected).
  - notifications feature shape `handler.go`/`dto.go`/`routes.go`/`service.go`/`repository.go` present (Phase 4 template).
  - `golang.org/x/sync v0.22.0 // indirect` present in `go.mod` (Phase 1 promotes to direct).
  - Web: profile stub card `profile-page.tsx:135-141`, `apiClient` (`lib/api/client`), `parseData` (`lib/api/envelope`), `HvModal`, `confirm-dialog.tsx` (`components/shared/`), MSW `src/test/msw/handlers.ts` all present (Phase 5).

### Answers (session 1)

1. **QR progress granularity → thread progress callback (option b).** Expose distinct `scanned`/`confirmed`. Reverses Phase 3's original coarse-state recommendation. Propagated to Phase 1 (additive callback on `LoginQR`), Phase 3 (states + LinkManager), Phase 4 (status enum), Phase 5 (client state machine).
2. **One Zalo account per teacher → keep `teacher_id` as PK.** Confirms Phase 2 as written; no change.
3. **Consent → persist `consent_at` + `consent_version`.** Propagated to Phase 2 (schema/model/repo), Phase 4 (`link/start` request body + handler), Phase 5 (modal sends the version).
4. **Expired detection → proactive health probe.** Reverses Phase 3's lazy-only recommendation. Adds a second background component. Propagated to Phase 3 (probe + lifecycle), Phase 4 (`GET /me/zalo` reflects probe-updated status), success criteria.

### Whole-Plan Consistency Sweep (session 1)

- Reconciled `plan.md` diagram: `link/start` returns `link_id` only (QR arrives via the first `link/status` poll), matching Phase 3/4 — the earlier "link_id + QR PNG" wording was misleading and is fixed.
- Poll state enum unified across `plan.md`/Phase 3/Phase 4/Phase 5 to `pending|qr_ready|scanned|confirmed|linked|expired|error`.
- Phase 4 `authctx.TeacherID` usage corrected to the real `(*gin.Context) (uuid.UUID, bool)` signature.
- No unresolved contradictions remain.

### Execution (session 2)

All five phases implemented and merged. Backend package lives at
`apps/api/internal/features/zalo/` (+ `protocol/` subpackage); envelope
encryption landed at `internal/shared/secrets` instead of the planned
`internal/shared/crypto` (name collision with `crypto/rand` at the import
site — see Phase 2 Execution Notes). Frontend slice lives at
`apps/web/src/features/profile/{api,hooks,schemas,components}` per the
Phase 5 design.

**Backend verification, re-run at the end of session 2:** `go build ./...`,
`go vet ./...` (with and without `-tags=integration`), `go test ./...` (all
packages), `go test -race -count=3` on `./internal/features/zalo/...`, and
`golangci-lint v2.7.2` — all clean, 0 issues. Migration `000004_zalo_accounts`
is exercised by `migrations/migrations_test.go`'s up/down cycle.
`go test -tags=integration ./...` was additionally run against a real
Postgres instance and is clean across every package (`billing`, `classes`,
`collections`, `contacts`, `enrollments`, `notifications`, `payments`,
`sessions`, `statements`, `students`, `teachers`, `zalo`, `zalo/protocol`,
`middleware`, `server`, `shared/id`, `shared/secrets`, `shared/validation`,
`migrations`, `seeds`); `go vet -tags=integration ./...` is clean too. This
is the strongest evidence for the encrypted-credential storage path — it
exercises `internal/features/zalo/integration_test.go` against a real
database rather than a stub.

**Frontend verification:** 30 test files / 146 tests pass, `eslint` 0 errors
(4 pre-existing `react-hook-form` warnings unrelated to this feature),
`tsc -b --noEmit` exit 0, `prettier --check` clean, `npm run build` succeeds.

**Two code reviews were run against the finished implementation, each with a
disposition recording what was fixed vs. deliberately deferred:**

- `plans/reports/zalo-phase-04-code-review.md` (backend). Found one critical
  issue (C-1: a transport-error path put the plaintext IMEI into a log line
  via an unwrapped `*url.Error`) and one high issue (H-2: an in-flight link
  attempt could theoretically re-link an account the instant after `Unlink`
  deleted its row). Both fixed, each with a regression test written first and
  confirmed to fail against the pre-fix code. Also resolved: the `DELETE
  /me/zalo` contract — the review flagged a mismatch between the accepted
  plan (`204` unconditionally) and the shipped code (`404` when nothing was
  linked); decided in favor of the accepted plan, so `Unlink` is now
  idempotent and always `204`. A third test pinning the old `404` was found
  and fixed in `integration_test.go` after the review, and the OpenAPI spec
  (`apps/api/docs/*`) was regenerated so it documents `DELETE /me/zalo` as
  idempotent `204` with no `404` case — code, tests, and the generated spec
  now agree. A short `.env.example` placeholder that would have passed
  production validation was also fixed (L-6). Four low-severity
  items (a seed-service gap, an unindexed status column, a probe-restart
  edge case, and JSON-error quoting) were deferred as informational — none
  touch credential exposure or the auth contract.
- `plans/reports/zalo-phase-05-code-review.md` (frontend). Found three high
  issues, all in the unhappy path: a poll that never succeeds looped forever
  with no visible error (H1), a failed status query rendered as "not linked"
  to an already-linked teacher (H2), and the one error screen a teacher sees
  was in English (H3). All three fixed. Four of six medium issues fixed
  (stale card on close-while-linking, silent unlink failure, a retry that
  could no-op on a repeated `link_id`, no local countdown-expiry fallback);
  one partially mitigated (iOS Safari ignores `download` on a `data:` URI —
  worked around with `target="_blank"`, a full Blob-URL fix deferred as
  disproportionate); one left as documented, not a defect (`ATTEMPT_TTL_SECONDS`
  duplicating the server's configurable TTL — fixing it means an API contract
  change outside this phase's scope).
- `plans/reports/zalo-phase-05-test-report.md` carries its own correction: its
  original claim that TypeScript compiled (inferred from `vitest` running) was
  wrong — `vitest` does not typecheck, and the added test file in fact failed
  `tsc`, `eslint`, and `prettier`. The correction section documents the fix and
  the final verified count (143 tests at that point; the phase-05 review's own
  later fixes brought the suite to its final 146).

None of the deferred items above affect the plan's Success Criteria.

<!-- slug: zalo-personal-auth -->
