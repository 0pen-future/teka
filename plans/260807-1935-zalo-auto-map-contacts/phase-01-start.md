---
phase: 1
title: "Protocol FindUser and SendFriendRequest port"
status: completed
priority: P1
effort: "1d"
dependencies: []
---

# Phase 1: Protocol FindUser and SendFriendRequest port

## Overview

Port `FindUser` (batch phone → Zalo account lookup) and `SendFriendRequest`
from upstream `zcago` into `apps/api/internal/features/zalo/protocol`,
following the request/crypto patterns `FetchFriends` and `SendMessage` already
use. No service-layer or HTTP surface changes.

<!-- Updated: Validation Session 1 - SendFriendRequest port added -->

## Requirements

- Functional: `FindUser(ctx, sess, phones []string)` returns a map keyed by
  phone with `UID`, `DisplayName`, `ZaloName`, `Avatar` for each phone Zalo can
  resolve; phones Zalo cannot resolve are simply absent from the map.
- Functional: `SendFriendRequest(ctx, sess, userID, message string)` sends one
  friend request to one UID; empty userID errors early without a network call.
- Functional: empty input errors early without a network call.
- Non-functional: package stays quarantined (no Teka imports); wire details
  mirror upstream so a future re-port stays mechanical.

## Architecture

Upstream reference (`zcago/api/find_user.go`, fetched 260807):

- Service base: zpw service **`friend`** (not `profile`).
- Endpoint: `GET {friend}/api/friend/profile/multiget` with AES-encrypted
  `params` of `{"phones": [...], "avatar_size": 240, "language": sess.Language}`.
- Response: standard encrypted envelope; decrypted body is a JSON object keyed
  by phone: `{"<phone>": {"uid": "...", "display_name": "...", "zalo_name": "...", "avatar": "...", ...}}`.

Our port already has every building block in `contacts.go`: `encryptPayload`,
`makeURL`, `newRequest`/`setDefaultHeaders`/`doRequest`, `Response[T]` envelope,
`decryptDataField`. `FindUser` is `FetchFriends` with a different service, path,
payload, and result type.

Upstream reference for the request send (`zcago/api/send_friend_request.go`,
fetched 260807):

- Endpoint: `POST {friend}/api/friend/sendreq` — same `friend` service as
  `FindUser`, form body with AES-encrypted `params`.
- Payload: `{"toid": userID, "msg": message, "reqsrc": 30, "imei": sess.IMEI,
  "language": ..., "srcParams": "{\"uidTo\":\"<userID>\"}"}` (srcParams is a
  JSON string, not a nested object).
- Response: standard encrypted envelope; the data field is an opaque string —
  success is a zero `error_code`, the body is not parsed further.
- Our `send.go` already has the POST-form + `imei` pattern to mirror.

Gap to close first: `ZpwServiceMapV3` (`models.go:14`) has no `Friend` field
and `ServiceURL` (`client.go:387`) has no `"friend"` case — the login response
carries the URLs, we just never parsed them. Existing sessions in the cache
were built before this field existed only in-process, so no migration concern:
the map is re-parsed on every login.

## Related Code Files

- Modify: `apps/api/internal/features/zalo/protocol/models.go` — add
  `Friend []string \`json:"friend"\`` to `ZpwServiceMapV3`; add a small
  `FoundUser` struct (UID, DisplayName, ZaloName, Avatar — only the fields the
  product needs, matching the `FriendInfo` precedent).
- Modify: `apps/api/internal/features/zalo/protocol/client.go` — `"friend"`
  case in `ServiceURL`.
- Modify: `apps/api/internal/features/zalo/protocol/contacts.go` — add
  `FindUser` and `SendFriendRequest` beside `FetchFriends` (same domain, same
  helpers; no new file).
- Modify: `apps/api/internal/features/zalo/protocol/contacts_test.go` — tests.
- Modify: `apps/api/internal/features/zalo/protocol/doc.go` — the scope comment
  still claims "authentication only" while `contacts.go`/`send.go` exist; fix
  the drift while touching the package.

## Implementation Steps (TDD)

1. Red: extend `contacts_test.go` with `FindUser` tests mirroring the existing
   `FetchFriends` golden-response style (fake HTTP server, encrypted fixture):
   - session without `friend` service URL → `zalo_personal: no friend service URL`;
   - empty phone slice → error, zero HTTP calls;
   - happy path: two phones requested, one resolved — decrypted map parsed,
     absent phone absent from result;
   - non-zero `error_code` envelope → error carrying code and message.
2. Red: `ServiceURL` test for the `"friend"` case.
3. Red: `SendFriendRequest` tests in the same golden style — no friend service
   URL error; empty userID → error, zero HTTP calls; happy path posts the
   encrypted form payload (`toid`, `reqsrc` 30, stringified `srcParams`) and a
   zero `error_code` envelope succeeds; non-zero `error_code` → error with
   code and message.
4. Green: add the `Friend` field + `ServiceURL` case + `FindUser` +
   `SendFriendRequest` implementations.
5. Fix `doc.go` scope paragraph.
6. Run package tests in docker (the session's established harness).

## Success Criteria

- [x] `FindUser` resolves phones through the `friend` service map with the
      shared crypto/envelope helpers; all new golden tests green.
- [x] `SendFriendRequest` posts one request per call through the same service
      map; golden tests green.
- [x] `go test ./internal/features/zalo/...` and `go vet -tags=integration ./internal/features/zalo/...` green.
- [x] `doc.go` describes the real scope (auth + friends + send + lookup + friend requests).

## Risk Assessment

- **Wire format drift**: zcago is a reverse-engineered port; `multiget` may
  change shape without notice. Mitigation: golden tests pin our parsing, and
  the live PoC in phase 2 validates against production Zalo before any UI work.
- **Response typing**: upstream response has many fields we ignore; keep
  `FoundUser` minimal so unknown fields cannot break decoding.
