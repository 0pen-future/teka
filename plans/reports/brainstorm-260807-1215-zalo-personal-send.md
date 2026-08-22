# Brainstorm: Notifications gửi statement qua Zalo cá nhân (zalo_personal)

Status: contract accepted, ready for `/ak:plan`
Date: 2026-08-07
Builds on: `brainstorm-260806-1611-zalo-personal-invoice-send.md` (direction A accepted),
plan `260806-2112-zalo-personal-auth` (completed — auth/session/encryption foundation shipped).

## Contract

- **Outcome:** từ trang notifications, giáo viên chọn kênh `zalo_personal`; hệ
  thống gửi tin nhắn statement (1 tin/contact/kỳ, text hiện có từ
  `statements.Build`) như DM từ chính tài khoản Zalo của giáo viên tới các
  contact đã map. Run được pace tự động; ledger rows chuyển
  queued → sent/failed với msgId/error. Contact chưa map → fallback
  `zalo_manual` (copy-paste), không bao giờ lỗi.
- **Constraints:**
  - Tái dùng nền tảng đã ship: `features/zalo` (session cache, `sessionFor`,
    health probe, encrypted creds), `notifications.Sender` registry,
    `statements.Build`. Không refactor lại các phần này.
  - Send phải nằm **ngoài** DB transaction (BulkSend hiện gọi `sender.Send`
    trong `WithinTx` — phải tách). Pacing serial, gap ngẫu nhiên 3–8s, cap
    per-run.
  - Single replica homelab (in-process run state chấp nhận được; rows persist
    nên restart không mất ledger).
  - Credentials không bao giờ xuất hiện trong log/response/git (tiêu chuẩn đã
    thiết lập ở plan auth).
  - Web: server state = TanStack Query, poll (không SSE) — cùng pattern QR link.
- **Non-goals:** ZNS/OA/Bot API, SMS, per-invoice message (giữ 1 tin/contact),
  auto-match theo phone (để sau nếu quan sát thấy friends list có phone),
  inbound chat/media/group, multi-replica scale.
- **Acceptance criteria:**
  1. Port send-only subset vào `zalo/protocol`: `SendMessage` (DM) +
     `FetchFriends` (bỏ group/media/listener), quarantine như phần auth.
  2. Contact có cột mapping Zalo UID (nullable); picker UI cho giáo viên chọn
     bạn Zalo cho từng contact một lần; unmap được.
  3. Channel `zalo_personal` trong registry + migration CHECK constraint
     `notifications.channel`; BulkSend với kênh này queue rows trong tx, một
     background paced run gửi tuần tự ngoài tx, cập nhật từng row
     sent (kèm `provider_msg_id`) / failed (kèm `error_message`).
  4. Web poll được tiến độ run (đang gửi x/y, row failed hiển thị lý do); đóng
     tab không dừng run.
  5. Contact chưa map hoặc session expired trước run → rơi về `zalo_manual`
     flow hiện có, có đếm/hiển thị rõ; session expired giữa run → các row còn
     lại failed với lý do rõ, không retry tự động.
  6. `go test ./...` + web test/lint/build xanh; race test cho run goroutine.

## Decisions (session này)

1. **Mapping = picker thủ công.** FriendInfo không có phone (goclaw chỉ decode
   userId/displayName/avatar/...); auto-match không xác minh được nếu chưa có
   creds live. Lưu `zalo_user_id` trên contacts. Auto-match là follow-up, không
   phải scope này.
2. **Execution = background run + poll.** Queue trong tx (giữ nguyên semantics
   BulkSend), goroutine paced-run ngoài tx, web poll qua TanStack Query. Cùng
   họ pattern với LinkManager/health probe đã có. Đồng bộ client-driven bị loại
   vì đóng tab là chết run.
3. **Granularity = 1 tin/contact/kỳ.** Tái dùng nguyên `statements.Build`; ít
   tin hơn = ít tín hiệu anti-spam; không đụng FK notifications. Ghi nhận
   diverge PRD R5 (per-invoice) là quyết định có chủ đích.

## Guardrails kế thừa (vẫn là acceptance, không phải nice-to-have)

- Pacing bắt buộc (source gốc không có throttling — 30 DM/2s ≈ spam flag).
- Friends-only delivery; unmapped ⇒ manual fallback tự động.
- Health probe chặn run khi session expired (probe đã ship, chỉ cần check
  trước run).
- Ban risk đã được user chấp nhận từ brainstorm 1611 + consent flow đã ship.

## Evidence gaps (không chặn plan)

- Ngưỡng rate thật của Zalo không có tài liệu — 3–8s là guess, cần configurable.
- `getfriends` có trả phone hay không — chỉ xác minh được khi có creds live;
  nếu có thì mở follow-up auto-match.

## Handoff

→ `/ak:plan` với contract trên. Phạm vi dự kiến: protocol send port →
migrations (contacts.zalo_user_id + notifications CHECK) → friend
picker API/UI → sender + paced run + poll endpoint → notifications page UI.
