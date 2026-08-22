---
phase: 3
title: "Paced run engine + kênh zalo_personal"
status: completed
priority: P1
effort: "2.5d"
dependencies: [1, 2]
---

# Phase 3: Paced run engine + kênh zalo_personal

## Overview

Phase lõi: kênh `zalo_personal` trong notifications. BulkSend queue rows trong
tx như cũ; sau commit, một background run gửi tuần tự qua `zalo.Service.SendDM`
với gap ngẫu nhiên 3–8s, cập nhật từng row `sent`/`failed`. Run persist trong
bảng `notification_runs` (Phase 2) nên sống sót restart; endpoint poll cho web
theo dõi tiến độ + endpoint resume thủ công cho run bị gián đoạn.

<!-- Updated: Validation Session 1 - run persistence + interrupted/resume thay snapshot thuần memory -->

## Requirements

- Functional:
  - Constant `ChannelZaloPersonal = "zalo_personal"` + entry registry.
  - BulkSend với `channel=zalo_personal`:
    1. Pre-check: `zalo.Service` có session lành (status linked, chưa expired) —
       expired/chưa link → 400/409 với message rõ, không ghi gì (giữ nguyên
       semantics "bad channel choice never leaves partial state").
    2. Load mapping `zalo_user_id` cho các contact target (1 query).
    3. Contact **đã map** → row `channel=zalo_personal`, `status=queued`, kèm
       run item (notification_id, to_uid, text). Contact **chưa map** → row
       `channel=zalo_manual` fallback (vẫn vào BulkText copy-paste), đếm
       `fallback_manual_count`. Không bao giờ lỗi vì unmapped.
    4. Insert batch trong tx (như hiện tại); **sau commit** start run.
  - RunManager (in-process pacing, theo mẫu LinkManager; state persist qua repo):
    - Tạo record `notification_runs` (status `running`) cùng tx với InsertBatch;
      rows gắn `run_id`. 1 run active/teacher; bulk send khi đang có run → 409.
    - Goroutine gửi tuần tự: `SendDM` → `sent` + `provider_msg_id`, lỗi →
      `failed` + `error_message`; giữa 2 send ngủ `rand(PaceMin..PaceMax)`.
      Xong → run status `completed` + `finished_at`.
    - `ErrLinkExpired` giữa run → mark **mọi row còn lại** `failed`
      ("phiên Zalo hết hạn"), run status `expired`. Không tự retry (guardrail).
    - Cap `MaxRunSize`: quá cap → 400 từ BulkSend, gợi ý gửi theo đợt.
    - Snapshot đọc từ DB: run record + COUNT rows theo `run_id` →
      `{run_id, period_id, purpose, status, total, sent, failed}` — sống sót
      restart, không giữ counter trong memory.
    - `Close()` dừng goroutine khi shutdown; **startup reconcile:** run nào còn
      `running` trong DB mà không có goroutine → mark `interrupted` (rows giữ
      `queued`).
    - **Resume thủ công:** `POST /billing-periods/:id/notifications/run/resume`
      cho run `interrupted` — re-render text các row `queued` còn lại qua
      `statements.Build`, tiếp tục pacing như run mới trên cùng `run_id`
      (status về `running`). KHÔNG auto-resume lúc boot: giữ guardrail "không
      tự gửi khi teacher không bấm" + tránh double-send tin đang bay dở.
  - `GET /billing-periods/:id/notifications/run` → snapshot run mới nhất của
    period (DB-backed); không có run → `{active: false}`. Ledger chi tiết vẫn
    qua `GET .../notifications` hiện có.
  - Response BulkSend thêm: `run_id`, `personal_queued_count`,
    `fallback_manual_count` (schema cũ giữ nguyên cho kênh manual).
- Non-functional:
  - Config `NotificationsConfig`: `PaceMinSeconds` (default 3), `PaceMaxSeconds`
    (default 8), `MaxRunSize` (default 50) — env `API_NOTIFICATIONS_*`,
    `.env.example` + compose cập nhật.
  - Send **ngoài** tx và ngoài request goroutine; race-clean (`go test -race`).

## Architecture

- **Consumer-defined interface** (mẫu `StatementsSource`): notifications khai báo
  `type ZaloSender interface { SendDM(ctx, teacherID uuid.UUID, toUID, text string) (string, error); Verify(ctx, teacherID uuid.UUID) error }`
  — `*zalo.Service` satisfy; container wire vào `notifications.NewService`.
  Không import cycle (notifications → zalo interface-free).
- **Sender registry vs run:** `Sender.Send` hiện chạy trong tx nên kênh
  personal KHÔNG giao việc gửi cho registry. `zaloPersonalSender.Send` chỉ là
  validation no-op (như manual); orchestration nằm trong `Service.BulkSend`
  (branch theo channel) + `RunManager`. Ghi chú doc comment giải thích ranh giới.
- **Repo mới:** `MarkOutcome(ctx, teacherID, id, status, providerMsgID, errMsg)`
  (1 UPDATE/row — mỗi row một lần khi đến lượt) + `FailQueuedInRun(ctx, ids, reason)`
  cho nhánh expired-giữa-run + CRUD `notification_runs` (create/update status,
  snapshot query COUNT theo `run_id`, reconcile `running`→`interrupted`).
- Run item giữ text đã render trong memory (DB không lưu message text — giữ
  nguyên thiết kế "never persisted"); restart giữa run → run `interrupted`,
  rows `queued` còn lại hiển thị trong ledger; resume re-render text từ
  `statements.Build` cho đúng các contact còn `queued` (không cần persist text).
- Lifecycle: RunManager thuộc `notifications.Service`, container gọi
  `Close()` lúc shutdown (thứ tự cùng chỗ `c.Zalo.Close()` trong
  `internal/app/container.go` / `app.go`).

## Related Code Files

- Create: `apps/api/internal/features/notifications/run_manager.go` (+ `run_manager_test.go`)
- Modify: `apps/api/internal/features/notifications/{model,sender,service,repository,handler,dto,routes}.go` + tests
- Modify: `apps/api/internal/config/config.go` (+ `config_test.go`), `.env.example`,
  `docker-compose*.yml` (env passthrough nếu file khai báo từng biến)
- Modify: `apps/api/internal/app/container.go`, `app.go` (wire ZaloSender + Close)
- Modify: `apps/api/docs/*` (swagger regen)

## Implementation Steps

1. Config: thêm 3 field + validate (min ≤ max, cap > 0), env mapping, defaults.
2. Repo: `MarkOutcome`, `FailQueuedInRun` + integration tests.
3. RunManager: struct + goroutine + DB-backed snapshot + Close + startup
   reconcile (`running`→`interrupted`); unit test với fake ZaloSender, sleep
   hook inject được (test không ngủ thật 3–8s).
4. BulkSend branch `zalo_personal` (pre-check, mapping load, fallback split,
   tạo run record trong tx, post-commit start); giữ nguyên đường
   `zalo_manual`/`zalo_zns` hiện có.
5. Resume: endpoint + service path (re-render qua `statements.Build`, chỉ rows
   `queued` của run `interrupted`).
6. Handler/DTO/route cho run snapshot + resume + response mới; swagger regen.
7. Wire container + shutdown + startup reconcile; `go test ./... && go test -race ./internal/features/notifications/...`.

## Success Criteria

- [x] Kênh manual/zns hành vi cũ không đổi (test hiện có xanh, không sửa test).
- [x] Run 3 contact (2 map, 1 không): 2 row personal sent với msgId, 1 row
      manual fallback; counts đúng; gap giữa 2 send ∈ [PaceMin, PaceMax] (test
      qua sleep hook).
- [x] Expired giữa run: row còn lại failed đúng lý do; run status `expired`;
      account expired (profile card hiển thị đúng).
- [x] Bulk send lần 2 khi run đang chạy → 409; sau khi run xong → cho phép.
- [x] Shutdown giữa run: goroutine dừng sạch (không leak — race test + Close
      test), rows còn lại vẫn `queued`; boot sau đó reconcile run →
      `interrupted`.
- [x] Resume run `interrupted`: chỉ gửi rows `queued` còn lại, text re-render
      đúng, không gửi lại rows đã `sent`.
- [x] Poll endpoint trả snapshot đúng qua vòng đời run, kể cả sau restart
      (DB-backed).

## Risk Assessment

- **Tx boundary regression:** nhánh manual giữ nguyên `sender.Send` trong tx —
  chỉ thêm branch mới, có test chốt hành vi cũ.
- **Goroutine leak / double-run:** RunManager mutex + test `-race -count=3`
  (chuẩn plan auth).
- **Ban risk:** pacing + cap + friends-only là guardrail bắt buộc; giá trị
  3–8s là guess có chủ đích (không có docs chính thức) → configurable.
- **Restart giữa run:** run `interrupted` + resume thủ công (quyết định
  Validation Session 1). Không auto-resume lúc boot — tránh double-send tin
  đang bay dở lúc crash và giữ nguyên tắc teacher chủ động bấm gửi.
- **Double-send khi resume:** row đang in-flight lúc crash có thể đã tới Zalo
  nhưng chưa MarkOutcome → resume sẽ gửi lại row đó. Chấp nhận rủi ro 1 tin
  trùng tối đa (MarkOutcome ngay sau mỗi send để thu hẹp cửa sổ); ghi nhận
  trong doc comment.

## Carry-over từ review phase 1 (protocol)

- `SendMessage`/`FetchFriends` không check `resp.StatusCode`: 5xx hiện ra như
  lỗi parse. Retry/pacing của phase này cần phân biệt 5xx (retry được) với
  error code Zalo (không retry) → cân nhắc thêm status-code check khi thiết kế
  phân loại lỗi per-row.
- `clientId` trong send payload lấy từ `time.Now().UnixMilli()` — là dedup key
  phía Zalo. Gửi hàng loạt có thể trùng ms → Zalo lặng lẽ drop. Sender phase
  này nên cấp giá trị monotonic duy nhất nếu quan sát thấy drop.
- `error_code: 0` + `data: null` từ Zalo = gửi thành công nhưng không có
  msgId (đã có test chốt). Outbox không được coi msgId rỗng là send fail.
- Zalo trả inner `error_code: -3` cho "not logged in"; send path hiện không
  evict session cache khi gặp nó (health probe ~15p sẽ dọn). Quyết định có
  chủ đích ở phase này: có đáng map `-3` thành evict + `ErrLinkExpired` không.

## Carry-over từ review phase 2 (mapping backend)

- `GET /me/zalo/friends` là một cú gọi Zalo live mỗi request (`count: 20000`,
  không cache server-side; cache miss ở session còn kèm relogin). Nếu run
  engine hoặc UI gọi lặp, cân nhắc cache TTL ngắn phía server; tối thiểu là
  `staleTime` phía web (ghi ở phase 4). Liên quan: `doRequest` chưa check
  `resp.StatusCode` nên 429 từ Zalo hiện ra thành 500 generic.
- `notifications.run_id` giờ là composite FK `(run_id, teacher_id)` →
  `notification_runs(id, teacher_id)` `ON DELETE SET NULL (run_id)` — mọi
  insert row của run phải mang đúng `teacher_id` của run, DB sẽ chặn link chéo
  tenant.

## Kết quả review phase 3 (đã xử lý)

- **H1 (race resume/bulk-send đồng thời):** đã sửa bằng mẫu reservation —
  `RunManager.Reserve` giữ slot theo teacher TRƯỚC transaction; caller thua
  nhận 409 trước khi ghi gì vào DB. `startRun` cũ bị xoá (đồng thời sửa M2).
- **M1 (user duyệt: thêm ngay):** migration `000006` thêm partial unique index
  `notification_runs(teacher_id) WHERE status='running'` — backstop cross-process;
  repo map vi phạm thành `ErrRunActive`, service trả 409.
- **M4 (user duyệt: thêm cầu dao):** run dừng sau 3 lỗi gửi liên tiếp
  (`runBreakerThreshold`), row chưa gửi giữ `queued`, run chuyển `interrupted`
  để resume thủ công — tránh đốt cả danh sách khi tài khoản bị chặn tạm.
- **M3:** docs/deployment.md ghi rõ yêu cầu chạy đúng 1 instance API
  (stop-then-start khi deploy).
