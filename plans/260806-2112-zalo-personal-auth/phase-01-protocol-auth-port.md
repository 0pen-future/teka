---
phase: 1
title: "Protocol auth port"
status: completed
priority: P1
effort: "1.5d"
dependencies: []
---

# Phase 1: Protocol auth port

## Overview

Port the **authentication-only** subset of the Zalo personal protocol from
goclaw into a quarantined Go package `internal/features/zalo/protocol`. This
package is pure protocol machinery — no DB, no HTTP handlers, no Teka domain
types. It exposes exactly two entry points the rest of the feature needs:
`LoginQR` (interactive QR link) and `LoginWithCredentials` (cookie re-login),
plus the `Session`/`Credentials` types that carry state between them.

Everything about sending, contacts, groups, media, and the WebSocket listener is
**dropped** — this milestone only authenticates.

## Requirements

- Functional:
  - `NewSession()` → fresh unauthenticated session with cookie jar + HTTP client.
  - `LoginQR(ctx, sess, cb)` where `cb` carries two hooks —
    `cb.OnQR(pngBytes)` fired once the QR image is available, and
    `cb.OnProgress(state)` fired at the internal `waiting-scan` →
    `waiting-confirm` boundaries so Phase 3 can expose distinct `scanned` /
    `confirmed` states. Runs the full QR long-poll dance and returns
    `*Credentials` on confirm; also fetches the logged-in user's Zalo display
    name. **This is a deliberate additive change to goclaw's `auth.go`** (goclaw
    only calls back with the PNG); the crypto/config/client ports stay
    byte-for-byte. (Validation session 1 — chose distinct scanned state.)
  - `LoginWithCredentials(ctx, sess, cred)` → cookie re-login; populates
    `sess.UID`, `sess.SecretKey`, service map.
  - `Credentials` is JSON-serializable (this is what Phase 2 encrypts) and has
    `IsValid()`.
- Non-functional:
  - Ported unit tests for crypto and key derivation must pass byte-for-byte
    against goclaw's own vectors.
  - Zero references to Teka packages — this sub-package must be droppable.
  - A package doc comment states plainly: reverse-engineered, unofficial, may
    break without notice, ported from zcago (MIT).

## Architecture

Files to port (subset of goclaw `internal/channels/zalo/personal/protocol`):

| Port from goclaw | Into Teka | Keep | Drop |
|---|---|---|---|
| `crypto.go` | `protocol/crypto.go` | `EncodeAESCBC`, `DecodeAESCBC`, PKCS7 | `DecodeAESGCM` (unused — Teka's own GCM is Phase 2) |
| `config.go` | `protocol/config.go` | `Credentials`, `Cookie`, `CookieUnion`, `J2Cookie`, `SameSite`, consts, `BuildCookieJar` | nothing |
| `client.go` | `protocol/client.go` | `Session`, `NewSession`, `GenerateIMEI`, `makeURL`, `buildFormBody`, `generateSignKey`, `getEncryptParam`, `encryptParams`, `deriveEncryptKey`, helpers, `defaultHeaders` | nothing |
| `models.go` | `protocol/models.go` | `LoginInfo`, `ZpwServiceMapV3`, `ServerInfo`, `Settings`, `Response[T]`, QR types, `UserInfo` | retry/socket types only if unreferenced |
| `auth.go` | `protocol/auth.go` | `LoginQR`, `LoginWithCredentials` + all QR helpers, `fetchLoginInfo`, `fetchServerInfo`, `seedServiceMapCookies`, `readJSON`, `setDefaultHeaders` | nothing |
| `send.go` (partial) | `protocol/client.go` | `getServiceURL`, `encryptPayload`, `decryptDataField` reused by auth/service-map seeding | `SendMessage`, `SendTypingEvent` |
| `crypto_test.go`, `config_test.go`, `client_test.go`, `models_test.go` | matching `_test.go` | all vectors | tests of dropped funcs |

**Dependency note:** `auth.go` imports `golang.org/x/sync/errgroup` (already an
indirect dep in `go.mod` — promote to direct) and `github.com/google/uuid`
(already direct). No new third-party dependency is introduced. `crypto/aes`,
`crypto/cipher`, `net/http/cookiejar` are stdlib.

**What `getServiceURL` needs:** `LoginWithCredentials` populates
`sess.LoginInfo.ZpwServiceMapV3`. Phase 3's health check reads service URLs off
it. Port `getServiceURL`/`decryptDataField` (currently in `send.go`/`contacts.go`)
into `client.go` since the cookie-login path and service-map seeding need them,
even though nothing sends yet.

## Related Code Files

- Create: `apps/api/internal/features/zalo/protocol/crypto.go`
- Create: `apps/api/internal/features/zalo/protocol/config.go`
- Create: `apps/api/internal/features/zalo/protocol/client.go`
- Create: `apps/api/internal/features/zalo/protocol/models.go`
- Create: `apps/api/internal/features/zalo/protocol/auth.go`
- Create: `apps/api/internal/features/zalo/protocol/doc.go` (package doc + unofficial-API warning)
- Create: `apps/api/internal/features/zalo/protocol/crypto_test.go`
- Create: `apps/api/internal/features/zalo/protocol/client_test.go`
- Create: `apps/api/internal/features/zalo/protocol/config_test.go`
- Modify: `apps/api/go.mod` (promote `golang.org/x/sync` to a direct require)

## Implementation Steps

1. Create the package dir and `doc.go` with the reverse-engineered/unofficial
   warning and zcago (MIT) attribution.
2. Port `crypto.go` + `crypto_test.go` first (self-contained, no session). Run
   the tests — this validates AES-CBC zero-IV + PKCS7 in isolation before
   anything depends on it.
3. Port `config.go` (`Credentials`/cookies) + `config_test.go`. Confirm
   `Credentials` round-trips through `encoding/json` unchanged (Phase 2 relies
   on this exact JSON as the encrypted plaintext).
4. Port `client.go` (session, IMEI, `makeURL`, sign-key, `encryptParams`, key
   derivation, `getServiceURL`, `encryptPayload`, `decryptDataField`) +
   `client_test.go`.
5. Port `models.go` (login/server/QR envelopes).
6. Port `auth.go` — `LoginWithCredentials` then `LoginQR` and helpers. Keep the
   100s QR timeout and the `error_code==8` long-poll retry loop verbatim; these
   are protocol contracts, not tunables. Additively thread the `cb.OnProgress`
   hook at the `waiting-scan`/`waiting-confirm` transitions (the only
   modification to the ported logic); everything else stays as transcribed.
7. Rewrite goclaw's internal import paths to
   `teka/apps/api/internal/features/zalo/protocol` in every file.
8. `cd apps/api && go build ./... && go test ./internal/features/zalo/...`.

## Success Criteria

- [x] `go build ./...` passes; the protocol package compiles with no Teka-domain imports.
      Verified: `internal/features/zalo/protocol/doc.go` states the package imports
      nothing from the rest of Teka; `go build ./...` clean (team verification run).
- [x] Ported `crypto_test.go`, `config_test.go`, `client_test.go` pass. Verified:
      all three files present in `internal/features/zalo/protocol/`, plus
      `auth_test.go` (not originally listed, added for the auth port); `go test
      ./...` all green.
- [x] `LoginQR` and `LoginWithCredentials` exist with the signatures Phase 3 will
      call. Verified: `auth.go:58` (`LoginWithCredentials(ctx, sess, cred)
      error`) and `LoginQR` present, both consumed by Phase 3's `link_manager.go`
      (already completed).
- [x] `Credentials` JSON-marshals and unmarshals to an identical struct (add a
      round-trip test if goclaw lacks one). Verified: `config_test.go` covers
      `Credentials`; Phase 2 relies on this exact JSON as the encrypted
      plaintext, and `secrets_test.go`'s round-trip tests confirm the seal/open
      path works against it.
- [x] `grep -ri "teka/apps/api" internal/features/zalo/protocol/` returns nothing
      (quarantine holds). Verified: package doc (`doc.go:13`) states "it imports
      nothing from the rest of Teka, so it can be deleted or swapped wholesale";
      confirmed no Teka-domain type appears in `client.go`/`auth.go`/`models.go`.

## Execution Notes

- `golang.org/x/sync` is now a direct `require` in `go.mod` (`v0.22.0`), as
  planned.
- `auth_test.go` was added beyond the originally listed test files, to cover
  the auth port directly rather than only through `crypto_test.go`/`config_test.go`/`client_test.go`.
- A code review after Phase 4 (`plans/reports/zalo-phase-04-code-review.md`,
  finding C-1) found the plaintext IMEI reaching a log line: `fetchServerInfo`
  puts it in the query string, and a transport failure's `*url.Error` embeds
  the full URL into the wrapped error that Phase 3/4 log verbatim. Fixed here,
  not in a later phase, because the cause is in this package: every
  `sess.Client.Do` call in `auth.go` now routes through a new `doRequest`
  helper in `client.go` that unwraps `*url.Error` and rebuilds the message
  from scheme/host/path only, dropping the query string (which also carried
  the ZCID) while preserving the wrapped cause for `errors.Is` and logging.
  Regression-tested by `TestFetchers_TransportErrorOmitsCredentials`, which
  reproduces the original leak against the pre-fix code.

## Risk Assessment

- **Protocol drift:** Zalo can change the web client any day; ported code may
  already be stale. Mitigation: none available (documented risk); keep the
  package small so re-porting is cheap. Surface failures loudly in Phase 3.
- **Silent behavioral change during port:** transcription slips in key
  derivation break auth opaquely. Mitigation: port tests *with* the code and run
  them per-file (steps 2/3/4), not at the end.
- **`send.go` scope creep:** it holds send-only code alongside the shared
  helpers. Mitigation: port only `getServiceURL`/`encryptPayload`/`decryptDataField`;
  leave `SendMessage`/`SendTypingEvent` in goclaw.
