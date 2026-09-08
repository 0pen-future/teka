---
phase: 3
title: "API: login rate limit + bcrypt gate + trusted proxies opt-in"
status: completed
priority: P1
effort: "1d"
dependencies: [2]
---

# Phase 3: API — login rate limit, bcrypt concurrency gate, trusted proxies opt-in (finding 7)

## Overview

Ba lớp bảo vệ cho `POST /api/v1/auth/login`: limiter theo số điện thoại đã chuẩn hoá (10/phút), cổng giới hạn số bcrypt chạy đồng thời (GOMAXPROCS slot, chờ tối đa 2s rồi 429) áp cho cả compare thật lẫn `burnPassword`, và limiter theo IP (60/phút) chỉ mount khi vận hành khai báo `API_HTTP_TRUSTED_PROXIES`. Mặc định (env rỗng) hành vi proxy không đổi.

## Requirements

- Functional:
  - `PhoneKey(field)` = `JSONBodyKey(field)` + `validation.NormalizePhone`; `0912…` và `+84912…` chung bucket; phone không parse được → key = chuỗi thô đã trim (vẫn limit) — không trả `""` để tránh bypass bằng phone lỗi định dạng.
  - `/auth/login`: `RateLimit(PhoneKey("phone"), 10, time.Minute)`; `/auth/forgot-password` chuyển từ `JSONBodyKey("phone")` sang `PhoneKey("phone")` (giữ 5/phút).
  - `bcryptGate`: semaphore `runtime.GOMAXPROCS(0)` slot (tối thiểu 1), chờ tối đa 2s hoặc tới `ctx.Done()`; hết chờ → `apperror.TooManyRequests("server busy, try again later")`; **không** publish `LoginFailed` (thông tin đăng nhập chưa được phán xét); áp cho cả `bcrypt.CompareHashAndPassword` lẫn `burnPassword` để không tạo timing oracle.
  - Trusted proxies: `HTTPConfig.TrustedProxies []string` (env `API_HTTP_TRUSTED_PROXIES`, phân cách dấu phẩy, CIDR hoặc IP). Rỗng → `SetTrustedProxies(nil)` như hiện tại và **không** mount IP limiter. Khác rỗng → `SetTrustedProxies(list)` và mount thêm `RateLimit(ClientIPKey(), 60, time.Minute)` trên login.
  - Web: trang login hiển thị message 429 từ server (`errors.root`) — xác nhận bằng test, không đổi code nếu đã đúng.
- Non-functional: timing-safe (mọi nhánh thất bại đều qua gate và bcrypt); không thêm dependency (channel semaphore); test không phụ thuộc thời gian thực ngoài gate wait ngắn (10ms) trong test gate.

## Architecture

```text
POST /auth/login
  → BodyLimit (phase 2)
  → RateLimit(ClientIPKey, 60/min)      [chỉ khi TrustedProxies != ∅]
  → RateLimit(PhoneKey("phone"), 10/min)
  → handler.login → Service.Login
        → gate.acquire(ctx, 2s) ── timeout → 429 TooManyRequests (no event)
        → bcrypt compare | burnPassword (trong slot)
        → gate.release
```

`RegisterRoutes(rg, h, loginLimits ...gin.HandlerFunc)`: `g.POST("/login", append(loginLimits, h.login)...)`. Router lắp mảng limiter theo config. `ratelimit.go` sửa comment "never `c.ClientIP()`" thành: chỉ dùng `ClientIPKey` khi router đã `SetTrustedProxies` từ config; nếu không, key là IP socket (Traefik) và limiter vô nghĩa.

Vì sao chỉ hash password khi login: `ResetPassword` tra token trước (cheap), token lạ bị từ chối trước khi bcrypt; `forgot-password` không bcrypt. Vậy gate chỉ cần ở `Login`.

**Cập nhật sau review — MINOR-5 (ctx cancel không còn thoát ra 500 giả):** khi caller bỏ đi trong lúc chờ gate (`ctx.Canceled`/`ctx.DeadlineExceeded`), bản đầu chỉ dịch `errBcryptBusy` sang 429 còn lỗi context thoát nguyên vẹn và bị `response.Err` biến thành `INTERNAL_ERROR` 500 kèm log error giả. Bản sửa cuối bắt cả hai loại lỗi (`errBcryptBusy` lẫn context) và trả `apperror.TooManyRequests` cho tất cả, có test `TestLoginRefusesWhenCallerLeavesWhileWaitingForBcrypt`.

**Cập nhật sau review — MINOR-6 (swagger thiếu 429 cho login):** `handler.go` annotation của `/auth/login` nay có thêm `@Failure 429` cạnh 401/422, khớp với `forgot-password`/`reset-password` đã có; `make api-docs` chạy lại không sinh diff ngoài dòng này.

**Cập nhật sau review (3 cycle):** thiết kế ban đầu của `JSONBodyKey` tra khoá trên `map[string]any` để lấy giá trị cho limiter. Review phát hiện điều đó tự cài lại luật khớp field của `encoding/json` và lệch với nó theo hai trục — hoa/thường, khoá trùng (MAJOR-1) và byte thừa sau JSON object do `json.Unmarshal` đòi input là đúng một giá trị còn gin bind bằng `json.Decoder` (MAJOR-11). Bản sửa cuối cùng bỏ hẳn việc tự cài luật: `JSONBodyKey` dựng một struct một field bằng `reflect.StructOf` mang đúng tag `json:"<field>"`, rồi decode qua `json.NewDecoder(...).Decode(...)` — cùng đường mà `ShouldBindJSON` dùng — nên limiter luôn thấy đúng giá trị handler sẽ bind, không có luật nào phải tự bảo trì. `PhoneKey` không đổi cấu trúc, chỉ thừa hưởng bản sửa qua `JSONBodyKey`.

## Related Code Files

- Create: `apps/api/internal/features/auth/bcrypt_gate.go`, `apps/api/internal/features/auth/bcrypt_gate_test.go`.
- Modify: `apps/api/internal/middleware/ratelimit.go` — `PhoneKey`, `ClientIPKey`, comment; `ratelimit_test.go` — test chuẩn hoá phone và ClientIPKey.
- Modify: `apps/api/internal/features/auth/service.go` — trường `gate *bcryptGate` khởi tạo trong `NewService` (hằng `bcryptGateWait = 2 * time.Second`); `Login` dùng gate; `burnPassword` qua gate.
- Modify: `apps/api/internal/features/auth/routes.go` — `RegisterRoutes` nhận limiter; `handler_test.go` — test 429 theo phone qua HTTP.
- Modify: `apps/api/internal/features/auth/service_test.go` — test gate bận → 429, không event.
- Modify: `apps/api/internal/config/config.go` — `TrustedProxies`; validate mỗi phần tử là IP hoặc CIDR hợp lệ (`net.ParseCIDR` / `net.ParseIP`).
- Modify: `.env.example` (root repo) — `API_HTTP_TRUSTED_PROXIES=` (rỗng, kèm comment ví dụ subnet docker).
- Modify: `apps/api/internal/server/router.go` — `SetTrustedProxies` theo config; lắp limiter login; forgot-password dùng `PhoneKey`.
- Modify: `apps/web/src/features/auth/__tests__/login-page.test.tsx` — case 429 hiển thị message.
- Modify: `docs/api-guidelines.md` (mục passwords/bcrypt ~454-456 và forgot-password ~474-481), `docs/deployment.md` (mục Traefik: cách điền `API_HTTP_TRUSTED_PROXIES`, cách xác minh XFF).
- Không đổi: `imports` limiter, `teachers.SetPassword` hashing, web interceptors.

## Implementation Steps

1. **`PhoneKey` + `ClientIPKey`** trong `ratelimit.go` (kiểm tra `middleware` chưa import `shared/validation` và `validation` không import `middleware` → không vòng). Test: hai dạng số → cùng key; phone rỗng → `""` (limiter bỏ qua, handler sẽ 422 như hiện tại); `ClientIPKey` trả `c.ClientIP()`.
2. **`bcryptGate`.**
   ```go
   type bcryptGate struct{ slots chan struct{}; wait time.Duration }
   func newBcryptGate(size int, wait time.Duration) *bcryptGate
   func (g *bcryptGate) run(ctx context.Context, fn func()) error // ErrBcryptBusy khi hết chờ / ctx done
   ```
   `NewService` tạo `newBcryptGate(runtime.GOMAXPROCS(0), bcryptGateWait)` (không cần bọc `max(1, …)`: `runtime.GOMAXPROCS(0)` luôn ≥ 1, review NIT-9 xác nhận lớp bọc là code phòng thủ chết). Test: size 1, wait 10ms; chiếm slot trong goroutine → `run` thứ hai trả `ErrBcryptBusy`; thả slot → chạy được; ctx cancel → trả `ctx.Err()`.
3. **`Login`.** Bọc cả hai đường trong `gate.run`; `ErrBcryptBusy` → `apperror.TooManyRequests(...)`, return trước khi publish event. Test service: `svc.gate = newBcryptGate(1, 10*time.Millisecond)`, chiếm slot, `Login` → `apperror` code `TOO_MANY_REQUESTS`, `fakeBus` không nhận `LoginFailed`; đường phone lạ cũng 429 (không lộ tài khoản tồn tại hay không).
4. **Routes + router.** Chữ ký `RegisterRoutes`; router:
   ```go
   loginLimits := []gin.HandlerFunc{middleware.RateLimit(middleware.PhoneKey("phone"), 10, time.Minute)}
   if len(cfg.HTTP.TrustedProxies) > 0 {
       _ = r.SetTrustedProxies(cfg.HTTP.TrustedProxies)
       loginLimits = append([]gin.HandlerFunc{middleware.RateLimit(middleware.ClientIPKey(), 60, time.Minute)}, loginLimits...)
   } else { _ = r.SetTrustedProxies(nil) }
   ```
   Cập nhật mọi nơi gọi `RegisterRoutes` (grep, kể cả test harness trong `handler_test.go`).
5. **Handler test.** `TestLoginRateLimitedPerNormalizedPhone`: 10 lần sai mật khẩu xen kẽ `0…`/`+84…` → 401; lần 11 → 429 envelope; số khác → 401 bình thường. Nếu `server` có router test, thêm `TestRouterWithoutTrustedProxiesSkipsIPLimiter` (cấu hình rỗng: 61 request login khác phone không 429 theo IP).
6. **Config.** Trường + validate; test config nếu có (`config_test.go`): `"10.0.0.0/8, 172.18.0.0/16"` parse 2 phần tử; `"abc"` → lỗi.
7. **Web test.** Trang login: MSW trả 429 envelope `{ error: { code: "TOO_MANY_REQUESTS", message } }` → message hiển thị ở `errors.root`. Không đổi code nếu pass.
8. **Docs.** `api-guidelines.md`: login limit 10/phút theo phone chuẩn hoá, gate bcrypt và ý nghĩa 429 "busy", IP limiter điều kiện; `deployment.md`: mục mới ngắn "Trusted proxies": Traefik Docker provider forward `X-Forwarded-For` mặc định; nếu qua cloudflared, đặt subnet chứa **cả** Traefik và cloudflared; cách xác minh: bật env, gửi 1 request, so IP trong log request với IP thật. Ghi rõ limiter in-memory per replica.
9. **Verify.** `make test-api-unit`, `make lint-api`, `cd apps/web && npm run test -- login`, `make api-docs` không diff.

## Success Criteria

- [x] Lần login sai thứ 11 trong 1 phút cùng số (mọi định dạng) → 429; số khác không bị ảnh hưởng.
- [x] Gate bận → 429 `TOO_MANY_REQUESTS`, không `LoginFailed`, cả nhánh phone lạ lẫn phone đúng đều 429 (timing-safe).
- [x] Env trusted proxies rỗng → `SetTrustedProxies(nil)` và không có IP limiter; khác rỗng → có; giá trị sai → lỗi khởi động.
- [x] forgot-password dùng `PhoneKey`; test hiện có của forgot vẫn pass.
- [x] Web login hiển thị message 429; docs API + deployment cập nhật; lint/test xanh.

## Risk Assessment

- **Gate quá chặt trên máy ít core** (GOMAXPROCS=1 → 1 bcrypt/lúc, ~250ms mỗi login): 8 giáo viên login cùng giây → người cuối chờ ~2s, còn trong ngưỡng. Tín hiệu: 429 "busy" xuất hiện trong log giờ cao điểm → nâng size gate (hằng → env nếu cần) hoặc giảm cost bcrypt (ngoài phạm vi).
- **Limiter theo phone chặn người dùng thật bị brute-force số của mình** (lockout 1 phút): chấp nhận, cửa sổ ngắn; khác với khoá tài khoản vĩnh viễn.
- **IP limiter với NAT trường học** (nhiều giáo viên chung IP): 60/phút gồm cả login thành công — đủ rộng; tín hiệu: 429 theo IP ở giờ vào lớp → nâng ngưỡng.
- **Cấu hình trusted proxies sai** → XFF giả mạo bypass IP limiter (nhưng phone limiter và gate vẫn hoạt động) — mặc định tắt, doc hướng dẫn xác minh. Phụ thuộc câu hỏi mở #1 của plan.
- **Đổi chữ ký `RegisterRoutes`** ảnh hưởng test harness — grep toàn bộ nơi gọi trước khi sửa.
