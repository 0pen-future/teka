# Brainstorm: event bus cho notification module — đánh giá port từ goclaw

Status: khuyến nghị **không port bus**; dùng transactional outbox + worker
Date: 2026-08-06
Nguồn: `/ak:xia --port` trên `github.com/nextlevelbuilder/goclaw@dev` → `internal/bus` (7 files, ~10k tokens)
Nối tiếp: `brainstorm-260806-1626-zalo-personal-ux.md`

## Contract

- **Outcome:** đợt gửi Zalo có nhịp chạy nền, sống sót qua restart, tiến trình
  hiển thị được trên web — với kiến trúc nhỏ nhất đạt được điều đó.
- **Constraints:** Gin request/response, 1 instance, GORM/Postgres; app hiện
  **không có** goroutine nền nào (chỉ graceful shutdown ở `server.go`/`serve.go`);
  UI đã chốt dùng polling TanStack Query, không SSE/WS.
- **Non-goals:** message pump inbound/outbound, dedupe/debounce chat, WS event
  fan-out, multi-tenant cache invalidation.
- **Acceptance:** đợt gửi tiếp tục đúng chỗ sau khi API restart; không tin nào
  gửi hai lần; test chạy không cần `sleep` thật.

## goclaw `bus` thực chất là 3 thứ gộp lại

| Thành phần | Mục đích ở goclaw | Teka cần? |
|---|---|---|
| `inbound`/`outbound` chan (buffer 1000) | work queue cho daemon bơm tin giữa nhiều kênh live và agent runtime | **Không** — Teka không có luồng để bơm |
| `handlers map[string]MessageHandler` | router theo tên kênh | **Không** — `senderRegistry()` đã làm |
| `Subscribe`/`Broadcast` | fan-out **đồng bộ, in-memory, best-effort** cho WS subscribers + cache invalidation | **Không** — UI polling DB |
| `DedupeCache`, `InboundDebouncer` | gộp tin nhắn chat dồn dập trước khi chạy LLM | **Không** — không có bài toán này |

`Broadcast` không persistent, không retry, không thứ tự, mất sạch khi restart.

## Luận điểm quyết định

**Bảng `notifications` đã là outbox table.** Nó có `status`
(queued/sent/delivered/failed), `provider_msg_id`, `error_message`, `sent_at` —
đúng ngữ nghĩa hàng đợi bền vững. Một event bus in-memory đặt cạnh đó tạo
**nguồn sự thật thứ hai, yếu hơn**: mất khi restart, đúng lúc acceptance
criteria đòi "gửi tiếp sau restart".

Sự kiện "đã gửi xong 1 tin" ở Teka **là một dòng UPDATE**, và UI đọc bằng
polling. Bus không nằm trên đường đi đó.

Kiểm chứng tiêu chí đáng dùng pub/sub — cần ≥2 subscriber độc lập cho cùng một
sự kiện. Teka hiện có **một** consumer (sổ thông báo). Chưa đạt → YAGNI.

## Ba hướng

| | Việc | Rủi ro | Kết luận |
|---|---|---|---|
| **A. Port `bus`** | ~400 LoC + goroutine nền + nguồn sự thật thứ 2 | trạng thái bay khi restart | Bỏ |
| **B. Dispatcher nhỏ (in-memory observer)** | ~60 LoC | vẫn không bền vững; chưa có subscriber thứ 2 | Bỏ (YAGNI) |
| **C. Outbox + worker trên bảng sẵn có** ✅ | ~150 LoC, không schema mới ngoài `channel` CHECK | claim row phải atomic | **Chọn** |

## Hình dạng C

- `BulkSend` giữ nguyên vai trò: **chỉ queue** trong transaction.
  Bỏ `sender.Send` ra khỏi `WithinTx` (hiện đang gọi bên trong — đợt gửi có
  nhịp sẽ giữ transaction Postgres vài phút, không chấp nhận được).
- `Dispatcher` (mới, trong package `notifications`): 1 run/giáo viên, claim từng
  row `queued` → `sending`, gọi `Sender`, ghi kết quả ngay, ngủ theo nhịp có
  jitter, tôn trọng `ctx`.
- Restart giữa chừng: row `sending` mồ côi → job quét lùi về `queued` theo tuổi,
  hoặc giáo viên bấm "Tiếp tục".
- Tiến trình: `GET .../notifications` sẵn có + `refetchInterval`. Không endpoint mới.

Testability: `Dispatcher` nhận `Repository` + `Sender` + `sleep func(context.Context, time.Duration) error`
→ test bơm sleep no-op, chạy tức thì, không cần đồng hồ thật.

Scale: an toàn ở 1 instance. Nếu sau này ≥2 replica, `runs map` in-memory hết
tác dụng → đổi claim sang `SELECT … FOR UPDATE SKIP LOCKED`. Ghi chú sẵn, đừng
làm bây giờ.

## Khi nào nên xem lại quyết định này

Port bus (hoặc dựng dispatcher) khi xuất hiện **subscriber thứ hai thật sự**:
activity log/audit trail, thống kê realtime, hoặc webhook ra ngoài. Lúc đó
`Broadcast` mới trả đúng giá trị của nó.

## Câu hỏi còn mở

1. Row `sending` mồ côi sau restart: tự quét lùi về `queued` (theo tuổi > N phút)
   hay để giáo viên bấm "Tiếp tục" thủ công?
2. Dispatcher chạy trong process API (`serve`) hay thêm lệnh CLI riêng?
   Trong process là đủ cho 1 instance; CLI riêng dễ vận hành hơn về sau.
3. Có cần `status='sending'` trong CHECK constraint không, hay tái dùng `queued`
   + cột `claimed_at`? (`sending` rõ nghĩa hơn cho UI.)
