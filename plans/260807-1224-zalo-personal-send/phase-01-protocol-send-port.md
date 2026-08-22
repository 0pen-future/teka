---
phase: 1
title: "Protocol send port"
status: completed
priority: P1
effort: "1d"
dependencies: []
---

# Phase 1: Protocol send port

## Overview

Port send-only subset của Zalo personal protocol từ goclaw vào
`internal/features/zalo/protocol` (quarantine như phần auth đã port), rồi expose
2 method mới trên `zalo.Service`: gửi DM và liệt kê bạn bè. Không đụng
notifications ở phase này.

## Requirements

- Functional:
  - `protocol.SendMessage(ctx, sess, toUID, text) (msgID string, err error)` —
    DM path only (`POST {chat_service}/api/message/sms`, payload
    `{message, toid, imei, clientId, ttl}`, AES-CBC encrypted `params=`).
  - `protocol.FetchFriends(ctx, sess) ([]FriendInfo, error)` — decode tối thiểu
    `userId`, `displayName`, `avatar` (bỏ mọi hàm group).
  - `zalo.Service.SendDM(ctx, teacherID, toUID, text) (string, error)` và
    `zalo.Service.ListFriends(ctx, teacherID) ([]Friend, error)` dùng
    `sessionFor` (tự re-login, trả `ErrNotLinked`/`ErrLinkExpired` như hiện có).
- Non-functional:
  - Credentials (IMEI, cookies, secret key) không bao giờ vào log/error string —
    tái dùng chuẩn `doRequest` (đã strip query string, review C-1 plan auth).
  - Port trung thực từ source, giữ quarantine: không import protocol ra ngoài
    package `zalo`.

## Architecture

- Source port: `github.com/nextlevelbuilder/goclaw@dev` →
  `internal/channels/zalo/protocol/send.go`, `send_helpers.go`, `contacts.go`
  (DM only; drop group/typing/media/listener).
- `client.go` hiện có `ServiceURL`, `getEncryptParam`, `encryptParams`,
  `buildFormBody`, `doRequest` — send port dùng lại; chỉ thêm helper còn thiếu
  (vd. decrypt data field của response) vào `client.go`/`crypto.go` theo ghi chú
  kiến trúc plan auth ("send-only helpers fold into client.go").
- `Session` đã giữ `UID`, secret key, service map sau login — `SendMessage`
  chỉ nhận `*Session`, không đọc DB.

## Related Code Files

- Create: `apps/api/internal/features/zalo/protocol/send.go` (+ `send_test.go`)
- Create: `apps/api/internal/features/zalo/protocol/contacts.go` (+ `contacts_test.go`)
- Modify: `apps/api/internal/features/zalo/protocol/client.go`, `crypto.go`
  (chỉ khi thiếu helper), `models.go` (FriendInfo, envelope cho send/friends)
- Modify: `apps/api/internal/features/zalo/service.go` (SendDM, ListFriends),
  `errors.go` (lỗi send-specific nếu cần), `service_test.go`

## Implementation Steps

1. Đọc source goclaw send/contacts, xác định phần DM-only + helper thiếu.
2. Port `send.go` + `send_helpers` (fold vào `client.go` nếu nhỏ), port test
   tương ứng (fake HTTP server như `client_test.go`/`auth_test.go` hiện có).
3. Port `FetchFriends` (bỏ group), thêm `FriendInfo` vào `models.go`.
4. Thêm `SendDM`/`ListFriends` vào `zalo.Service` qua `sessionFor`; lỗi Zalo
   reject session → `expire` + `ErrLinkExpired` (cùng semantics VerifyAccount).
5. Unit test service với fake login/relogin như `service_test.go` hiện có.

## Success Criteria

- [x] `go test ./internal/features/zalo/...` pass, kể cả `-race`.
- [x] Test chứng minh transport error không chứa IMEI/cookie (mở rộng
      `TestFetchers_TransportErrorOmitsCredentials` cho send path).
- [x] `SendDM` với session chết → account expired + `ErrLinkExpired`, không panic.
- [x] Không file nào ngoài package `zalo` import `zalo/protocol`.

## Risk Assessment

- **Protocol drift:** endpoint Zalo đổi bất kỳ lúc nào, không SLA — giữ port
  mỏng, test bằng fake server; lỗi runtime map về `failed` row (phase 3), không crash.
- **Payload lệch source:** so từng field với goclaw trước khi viết; không "cải tiến".
