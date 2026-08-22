---
title: "Zalo Personal Send"
description: "Notifications gửi tin statement như DM từ tài khoản Zalo cá nhân của giáo viên: port send protocol, mapping contact↔bạn Zalo, paced background run, UI tiến độ."
status: completed
priority: P1
effort: "6-7d"
tags: [zalo, notifications, backend, web, security]
created: 2026-08-07
blockedBy: []
blocks: []
---

# Zalo Personal Send

> **Artifact chính cho người đọc: [`plan.html`](./plan.html)** (self-contained,
> mở trực tiếp từ disk). File này là index cho CLI/cook; chi tiết thực thi nằm
> trong từng phase file.

## Overview

Nối milestone `260806-2112-zalo-personal-auth` (đã ship: QR link, creds mã hóa,
session cache, health probe): từ trang notifications, giáo viên chọn kênh
`zalo_personal`, hệ thống gửi tin statement (1 tin/contact/kỳ, text từ
`statements.Build`) như DM từ chính tài khoản Zalo của giáo viên tới các contact
đã map, pace 3–8s/tin trong background run; contact chưa map rơi về
`zalo_manual` copy-paste. Contract đã chốt tại
`plans/reports/brainstorm-260807-1215-zalo-personal-send.md`.

Quyết định đã chốt (không mở lại khi thực thi): mapping = picker thủ công;
execution = background run + poll; granularity = 1 tin/contact/kỳ.

**Hoàn thành 2026-08-07:** cả 5 phase done, mọi Success Criteria đạt; web +
backend gates xanh.

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Port send-only subset (`SendMessage` DM + `FetchFriends`) vào `zalo/protocol`, quarantine như phần auth | P1 |
| 2 | Mapping contact↔bạn Zalo: cột `zalo_user_id`/`zalo_name` trên contacts + picker API/UI, unmap được | P1 |
| 3 | Kênh `zalo_personal`: queue trong tx, paced run ngoài tx (gap ngẫu nhiên 3–8s, cap per-run), rows → sent/failed với msgId/error | P1 |
| 4 | Web: chọn kênh, poll tiến độ run, hiển thị fallback/failed rõ ràng; đóng tab không dừng run | P1 |

## Phases

| # | Phase | Status | Depends on |
|---|-------|--------|------------|
| 1 | [Phase 1: Protocol send port](./phase-01-protocol-send-port.md) | Completed | — |
| 2 | [Phase 2: Migration + mapping backend](./phase-02-migration-and-mapping-backend.md) | Completed | 1 |
| 3 | [Phase 3: Paced run engine + kênh zalo_personal](./phase-03-paced-run-and-personal-channel.md) | Completed | 1, 2 |
| 4 | [Phase 4: Web friend picker](./phase-04-web-friend-picker.md) | Completed | 2 |
| 5 | [Phase 5: Web channel + run progress](./phase-05-web-channel-and-run-progress.md) | Completed | 3, 4 |

## Success Criteria

- [x] Send-only protocol port có unit test ported pass; credentials không xuất
      hiện trong log/response/error (chuẩn `doRequest` strip đã có).
- [x] Giáo viên map/unmap được contact với bạn Zalo qua picker; mapping persist
      trên `contacts.zalo_user_id`.
- [x] Bulk send kênh `zalo_personal`: rows queue trong tx; run nền gửi tuần tự
      gap 3–8s; từng row → `sent` (+`provider_msg_id`) / `failed`
      (+`error_message`); contact chưa map → row `zalo_manual` fallback, không lỗi.
- [x] Session expired trước run → chặn với thông báo rõ; expired giữa run →
      các row còn lại `failed` lý do rõ, không tự retry.
- [x] Web poll tiến độ x/y, hiển thị failed + lý do, đếm fallback; đóng tab
      không dừng run.
- [x] Run persist trong bảng `notification_runs` (+ `notifications.run_id`):
      restart giữa run → run đánh dấu `interrupted`, rows còn lại giữ `queued`,
      teacher bấm "Gửi tiếp" để resume (không auto-resume lúc boot).
- [x] Trước khi gửi kênh personal: confirm dialog "X gửi tự động · Y copy thủ
      công" (đếm từ mapping đã có trên contact data phía client).
- [x] `go test ./...` + `go test -race` cho run engine, web
      test/lint/typecheck/build xanh; swagger + `docs/schema_design.sql` cập nhật.

## Nguồn & tham chiếu

- Contract: `plans/reports/brainstorm-260807-1215-zalo-personal-send.md`
- Kỹ thuật + rủi ro: `plans/reports/brainstorm-260806-1611-zalo-personal-invoice-send.md`
- Nền tảng auth: `plans/260806-2112-zalo-personal-auth/plan.md`
- Source port: `github.com/nextlevelbuilder/goclaw@dev` → `internal/channels/zalo/protocol/{send,send_helpers,contacts}.go`

## Validation Log

### Session 1 — 2026-08-07
**Trigger:** `/ak:plan validate` sau khi tạo plan (user chọn ở Post-Plan Handoff).
**Questions asked:** 4

#### Verification Results
- **Tier:** Full (5 phases — Fact Checker, Flow Tracer, Scope Auditor, Contract Verifier)
- **Claims checked:** ~30 (symbols, file paths, tx boundary, quarantine, CHECK constraint, web layout)
- **Verified:** ~30 | **Failed:** 0 | **Unverified:** 0
- Điểm tựa chính: `WithinTx` bọc `InsertBatch` tại `notifications/service.go:68→152`
  (xác nhận send phải nằm ngoài tx); không import `zalo/protocol` ngoài package
  zalo; CHECK channel tại `docs/schema_design.sql:439`; toàn bộ file web
  phase 4–5 tồn tại đúng đường dẫn.

#### Questions & Answers

1. **[Assumptions]** MaxRunSize mặc định 50 với pacing 3–8s (~4.5 phút/run tối đa) — giữ ngưỡng?
   - Options: Giữ 50 (Recommended) | Nâng 100 | Bỏ cap
   - **Answer:** Other — "make it configable"
   - **Rationale:** Plan đã thiết kế cap qua env `API_NOTIFICATIONS_MAX_RUN_SIZE`
     (default 50); đáp án xác nhận yêu cầu configurable, không đổi thiết kế.

2. **[Architecture]** Run state in-memory hay persist DB?
   - Options: In-memory (Recommended) | Persist bảng runs
   - **Answer:** Persist bảng runs
   - **Rationale:** Đảo khuyến nghị — restart không được làm mất tiến độ.
     Thiết kế chốt: bảng `notification_runs` + cột `notifications.run_id`;
     counters (total/sent/failed) derive bằng COUNT trên rows theo `run_id`,
     không duplicate state. Restart → run `running` cũ đánh dấu `interrupted`,
     rows giữ `queued`; resume là **hành động thủ công** của teacher (endpoint
     resume re-render text qua `statements.Build`) — không auto-resume lúc boot
     để giữ guardrail "không tự gửi khi teacher không bấm" và tránh double-send.

3. **[Tradeoffs]** Contact chưa map: gửi ngay + báo sau, hay confirm trước?
   - Options: Gửi ngay, báo sau (Recommended) | Confirm trước khi gửi
   - **Answer:** Confirm trước khi gửi
   - **Rationale:** Dialog "X gửi tự động · Y copy thủ công — tiếp tục?" trước
     một hành động gửi tin thật. Counts tính client-side từ mapping đã có trong
     contact data (Phase 2 DTO) — không cần endpoint preview mới.

4. **[Risks]** Expired giữa run: fail toàn bộ rows còn lại hay giữ queued?
   - Options: Fail toàn bộ (Recommended) | Giữ queued để resend
   - **Answer:** Fail toàn bộ (Recommended)
   - **Rationale:** Trạng thái cuối rõ ràng, nhất quán guardrail không-tự-retry.
     Với bảng runs: run kết thúc với status `expired`. Phân biệt với
     `interrupted` (restart): expired = failed rows, interrupted = queued rows.

#### Confirmed Decisions
- Cap per-run: configurable qua env, default 50 — giữ nguyên thiết kế.
- Run persistence: bảng `notification_runs` + `notifications.run_id`; manual
  resume, không auto-resume.
- Pre-send confirm dialog cho kênh personal (client-side counts).
- Expired giữa run → fail toàn bộ rows còn lại, run status `expired`.

#### Action Items
- [x] Phase 2: migration 000005 thêm bảng `notification_runs` + cột `notifications.run_id`.
- [x] Phase 3: RunManager persist run record, mark-interrupted lúc boot, endpoint resume; effort 2d → 2.5d.
- [x] Phase 5: confirm dialog trước khi gửi; banner trạng thái interrupted + nút "Gửi tiếp".
- [x] plan.html cập nhật các phase card/modal + risk "Restart giữa run" tương ứng.

#### Impact on Phases
- Phase 2: schema mở rộng (bảng runs) — down migration drop bảng + cột.
- Phase 3: persistence + resume thay cho snapshot thuần memory; snapshot đọc từ DB.
- Phase 5: thêm confirm dialog + interrupted/resume UI.

### Whole-Plan Consistency Sweep
- Files reread: plan.md, phase-01…phase-05, plan.html
- Decision deltas checked: 3 (run persistence, confirm dialog, expired-vs-interrupted status)
- Reconciled stale references: plan.md success criteria; phase-02/03/05 requirements,
  architecture, steps, success criteria, risks; plan.html contract/phase cards/modals/risks
- Unresolved contradictions: 0

<!-- slug: zalo-personal-send -->
