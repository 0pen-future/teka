# Brainstorm: System-wide audit log

## Contract

- **Outcome:** Center owner xem được audit log mọi hành động của user trong
  center: toàn bộ mutations (POST/PUT/PATCH/DELETE) + sự kiện auth
  (login/logout/login fail), qua trang web owner-only, có filter + phân trang.
- **Constraints:**
  - Zero UX impact: ghi audit bất đồng bộ, không thêm latency vào request path.
  - Multi-tenant: mọi bản ghi gắn `center_id` từ Scope; owner chỉ thấy log
    center của mình. Auth events (chưa có scope) gắn theo user.
  - Không lưu request body (password, PII); chỉ actor, route, entity id,
    status, request-id, ip, user-agent, metadata chọn lọc.
  - Theo kiến trúc hiện có: feature `internal/features/audit`, shared infra
    không chứa business logic, GORM + migration golang-migrate.
- **Non-goals:**
  - Không log reads (GET).
  - Không diff dữ liệu trước/sau (row-level change capture) — billing
    adjustments đã có trail riêng, giữ nguyên.
  - Không retention/archival tự động ở V1 (ghi nhận là follow-up).
  - Không realtime stream/websocket cho trang log.
- **Acceptance criteria:**
  1. Mọi request mutating qua `/api/v1` của user đã đăng nhập tạo đúng 1 dòng
     `audit_logs` (kể cả 4xx/5xx, kèm status code).
  2. Login thành công/thất bại và logout tạo bản ghi auth event.
  3. Benchmark/inspect xác nhận write path không chặn response (publish
     non-blocking; overflow thì drop + log warning, không chặn).
  4. `GET .../audit-logs` trả 403 cho member không phải owner; owner thấy
     log đúng center, filter theo actor/action/khoảng thời gian, keyset
     pagination.
  5. Trang web audit hiển thị cho owner; member không thấy entry điều hướng.
  6. Graceful shutdown: `events.Bus.Close` drain queue của mọi subscriber
     trước khi đóng DB pool.
  7. Event bus tái sử dụng được: feature auth và middleware chỉ phụ thuộc
     `shared/events`, không import `features/audit`; thêm subscriber mới
     không cần sửa publisher.

## Decisions (user-confirmed)

1. Phạm vi: mutations + auth events (không reads).
2. Quyền xem: chỉ center owner.
3. Capture: **HTTP middleware + observer pattern** — middleware là publisher,
   async worker (buffered channel + worker goroutine, batch insert) là
   observer; feature auth publish sự kiện login/logout qua cùng đường.
4. Observer infra **tách generic** khỏi audit: event bus in-process tại
   `internal/shared/events` (subject dùng chung, fan-out per-subscriber).
   Audit chỉ là subscriber đầu tiên; module khác (notifications, webhook,
   analytics) subscribe sau mà không sửa middleware/publisher.

## Approaches compared

| Approach | Ưu | Nhược | Verdict |
|---|---|---|---|
| Middleware HTTP (+async observer) | 1 điểm chạm, phủ 100% mutations, không sửa 19 features, không ảnh hưởng latency | Ngữ nghĩa mức endpoint, cần map route→action name | **Chọn** |
| Service-layer events | Ngữ nghĩa domain giàu | Sửa mọi feature, dễ bỏ sót, khó đảm bảo phủ đủ | Loại (V1) |
| DB trigger/CDC | Bắt được diff dữ liệu | Không biết actor nếu không SET LOCAL, phức tạp với pool GORM, không bắt auth event | Loại |

Đường mở rộng: event bus generic cho phép thêm explicit domain events
(hybrid) và subscriber mới (notification, webhook, analytics) mà không đổi
hạ tầng capture.

### Abstraction của observer (so sánh bổ sung)

| Approach | Ưu | Nhược | Verdict |
|---|---|---|---|
| Generic in-process event bus (`shared/events`): `Publish` non-blocking, mỗi subscriber có queue + worker riêng | Tái sử dụng cho mọi module; slow consumer không chặn nhau; auth không phụ thuộc audit | Thêm 1 lớp indirection; cần quy ước event type | **Chọn** |
| Giữ `audit.Recorder` chuyên dụng, refactor khi có consumer thứ 2 | Ít code nhất hôm nay | Auth/feature phụ thuộc thẳng audit; refactor sau đắt hơn khi callsite lan rộng | Loại (user đã quyết cần reuse) |
| Domain-event framework đầy đủ (topic registry, generics, retry, outbox) | Guarantee mạnh | Over-engineering cho nhu cầu in-process at-most-once | Loại (YAGNI) |

## Design sketch

- `internal/shared/events` (infra thuần, không business logic):

  ```go
  type Event interface{ EventName() string }
  type Handler func(ctx context.Context, e Event)

  type Bus interface {
      Publish(e Event)                            // non-blocking; queue đầy → drop + log warning
      Subscribe(name string, buf int, h Handler)  // mỗi subscriber: queue + worker goroutine riêng
      Close(ctx context.Context) error            // drain mọi queue trước khi đóng DB pool
  }
  ```

  Kèm `SyncBus` (deliver đồng bộ) cho unit/integration test. Event struct
  sống cạnh publisher: `middleware.RequestCompleted{...}`,
  `auth.LoginSucceeded/LoginFailed/LoggedOut{...}` — subscriber type-switch,
  không import ngược.
- Migration `000010_audit_logs`: `id, occurred_at, center_id (nullable),
  actor_user_id, actor_role, action, method, path, entity_type, entity_id
  (nullable), status_code, request_id, ip, user_agent, metadata jsonb`.
  Index `(center_id, occurred_at DESC)`; partition chỉ khi volume đòi hỏi
  (YAGNI).
- `features/audit`: model, repository (batch insert), subscriber
  (`bus.Subscribe("audit", 1024, h)` — handler tự batch theo size/interval),
  handler + routes cho owner read API, service map route→action.
- Middleware `middleware.RequestEvents(bus)` mount sau auth+scope trong nhóm
  `/api/v1`; publish `RequestCompleted` sau `c.Next()` (đã có status), chỉ
  method mutating. Middleware không biết audit tồn tại.
- Auth feature `bus.Publish(auth.LoginFailed{...})` cho login/logout/login-fail
  (login-fail: không lưu password, cân nhắc chỉ lưu email đã thử + ip).
  Auth phụ thuộc `shared/events`, không phụ thuộc `features/audit`.
- Web: feature `audit` (hoặc trong `center`), trang owner-only, table +
  filter + infinite scroll/keyset.
- Test: dùng `SyncBus` cho unit/integration test (deliver đồng bộ, assert
  ngay); integration test khẳng định mutation ghi đúng 1 dòng.

## Risks / unresolved

- Async ⇒ có thể mất vài event cuối nếu crash cứng (chấp nhận được cho
  ops-audit; không dùng cho chứng từ tài chính).
- Action naming: cần bảng map route→động từ đọc được (vd
  `billing.adjustment.create`); fallback `METHOD path`.
- Retention & dung lượng: chưa quyết — theo dõi sau khi có số liệu thực.
- Login-fail lưu email hay không (PII trade-off) — quyết ở bước plan.
- Event contract: `EventName()` string là hợp đồng lỏng; khi có subscriber
  thứ 2 cần bảng event catalog ngắn trong docs để tránh trôi tên/field.
- Drop policy per-subscriber: audit chấp nhận at-most-once; subscriber tương
  lai cần guarantee mạnh hơn (vd webhook) phải tự thêm persistence/outbox —
  bus không hứa điều đó.

## Next

Handoff sang `/ak:plan` → `/ak:cook` với contract này.
