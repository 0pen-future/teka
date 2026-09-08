---
phase: 2
title: "API: cap body toàn server + 413"
status: completed
priority: P1
effort: "0.5d"
dependencies: []
---

# Phase 2: API — cap body toàn server + 413 PAYLOAD_TOO_LARGE (finding 6)

## Overview

Thêm middleware `BodyLimit` toàn server (mặc định 1 MiB, cấu hình `API_HTTP_MAX_BODY_BYTES`) từ chối sớm request có `Content-Length` vượt cap và bọc `http.MaxBytesReader` cho phần còn lại, để `io.ReadAll` trong `JSONBodyKey` và mọi `ShouldBindJSON` không thể buffer body vô hạn. Route import roster giữ cap riêng 2 MiB đã có.

## Requirements

- Functional:
  - `Content-Length > cap` → 413 ngay trong middleware, envelope lỗi chuẩn, code `PAYLOAD_TOO_LARGE`; handler và limiter phía sau không chạy.
  - Body chunked/không khai báo độ dài vượt cap → đọc tới cap rồi lỗi `*http.MaxBytesError`; `validation.BindError` map thành 413 `PAYLOAD_TOO_LARGE`; `JSONBodyKey` gặp lỗi đọc phải trả lại body cho downstream sao cho lỗi được surface (không im lặng thành 400 "invalid request body").
  - `POST /api/v1/imports/roster` **không** bị bọc bởi cap 1 MiB (handler tự bọc 2 MiB); mọi route khác đều bị cap.
  - Request không có body (GET, DELETE, `http.NoBody`) đi qua không đổi.
- Non-functional: cap cấu hình được; không đổi hợp đồng của route import (vẫn 400 với message hiện tại — non-goal đổi sang 413); thêm mã lỗi mới vào docs API; web không cần đổi (mã lỗi lạ đã hiển thị message server qua `useApiFormErrors`).

## Architecture

```text
r.Use(RequestID, Logger, Recovery, CORS, BodyLimit(cfg.HTTP.MaxBodyBytes, "/api/v1/imports/roster"))
                                          │
        ┌─────────────────────────────────┴──────────────────────────────┐
        │ c.FullPath() ∈ exempt → c.Next()                               │
        │ Body nil / NoBody → c.Next()                                   │
        │ ContentLength > max → abort 413 PAYLOAD_TOO_LARGE              │
        │ else c.Request.Body = http.MaxBytesReader(c.Writer, Body, max) │
        └────────────────────────────────────────────────────────────────┘
                                          │
              RateLimit(JSONBodyKey) → io.ReadAll → MaxBytesError?
                 → đặt lại Body = reader replay lỗi → key "" → next
                                          │
              handler ShouldBindJSON → err → validation.BindError
                 → errors.As(*http.MaxBytesError) → 413
```

Gin resolve route trước khi chạy chain nên `c.FullPath()` đã có giá trị trong middleware toàn cục (tiền lệ: `request_events.go`, `route_policy_enforce.go`). Middleware đặt **sau** `Recovery` để panic trong xử lý body vẫn được recover, và sau `CORS` để preflight không bị ảnh hưởng.

## Related Code Files

- Create: `apps/api/internal/middleware/bodylimit.go`, `apps/api/internal/middleware/bodylimit_test.go`.
- Modify: `apps/api/internal/config/config.go` — `HTTPConfig.MaxBodyBytes int64 \`env:"HTTP_MAX_BODY_BYTES" envDefault:"1048576"\``; validate `> 0`.
- Modify: `.env.example` (root repo) — `API_HTTP_MAX_BODY_BYTES=1048576` cạnh `API_HTTP_PORT`.
- Modify: `apps/api/internal/server/router.go` — thêm vào `r.Use(...)` (dòng ~55-60); comment ngắn tại sao imports miễn.
- Modify: `apps/api/internal/shared/apperror/apperror.go` — `CodePayloadTooLarge = "PAYLOAD_TOO_LARGE"`, `func PayloadTooLarge(message string) *Error` (status 413).
- Modify: `apps/api/internal/shared/validation/validation.go` — `BindError` map `*http.MaxBytesError` trước nhánh mặc định.
- Modify: `apps/api/internal/middleware/ratelimit.go` — `JSONBodyKey`: khi `io.ReadAll` lỗi, gán `c.Request.Body = io.NopCloser(&errReader{err})` (reader trả lỗi đó mọi lần `Read`) rồi trả `""`.
- Modify: `apps/api/internal/middleware/ratelimit_test.go` — test body vượt cap qua limiter → 413 và không tiêu bucket.
- Modify: `docs/architecture.md` (request lifecycle ~dòng 33-42), `docs/api-guidelines.md` (mục mã lỗi / envelope nếu có bảng; và ghi chú cap body + ngoại lệ import).
- Không đổi: `imports/handler.go` (cap 2 MiB, `TestOversizeUploadIsRejected` giữ nguyên), `server/server.go` timeouts, Traefik labels, web.

## Implementation Steps

1. **apperror + validation.** Thêm code/constructor 413. Trong `BindError`: `var tooLarge *http.MaxBytesError; if errors.As(err, &tooLarge) { return apperror.PayloadTooLarge("request body too large") }`. Viết test trong `validation_test.go` (nếu có) hoặc test router-level ở bước 5 để chứng minh gin `ShouldBindJSON` propagate `MaxBytesError` nguyên vẹn (json.Decoder trả lỗi reader trực tiếp; nếu thực tế bị bọc, so thêm bằng `strings.Contains(err.Error(), "http: request body too large")` và ghi lý do).
2. **Middleware `BodyLimit`.**
   ```go
   func BodyLimit(max int64, exemptFullPaths ...string) gin.HandlerFunc {
       exempt := make(map[string]struct{}, len(exemptFullPaths)) // fill
       return func(c *gin.Context) {
           if _, ok := exempt[c.FullPath()]; ok || c.Request.Body == nil || c.Request.Body == http.NoBody {
               c.Next(); return
           }
           if c.Request.ContentLength > max {
               response.Error(c, apperror.PayloadTooLarge("request body too large")) // dùng helper response hiện có (xem cách RateLimit trả 429)
               c.Abort(); return
           }
           c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, max)
           c.Next()
       }
   }
   ```
   Dùng đúng helper trả lỗi mà `RateLimit` đang dùng để envelope đồng nhất.
3. **Config + router.** Trường/validate/env; `r.Use(..., middleware.BodyLimit(cfg.HTTP.MaxBodyBytes, "/api/v1/imports/roster"))`. `NewRouter` đã nhận `cfg *config.Config`, đọc `cfg.HTTP.MaxBodyBytes` trực tiếp.
4. **`JSONBodyKey`.** Thêm `errReader`; test `TestJSONBodyKeyPreservesBodyForDownstreamBinding` giữ nguyên; thêm `TestJSONBodyKeyOversizedBodySurfacesAsPayloadTooLarge`: chain `BodyLimit(16) → RateLimit(JSONBodyKey("phone"),1,min) → handler ShouldBindJSON + BindError`; gửi body 64 byte chunked (không set `Content-Length`, dùng `req.ContentLength = -1` với `io.Pipe` hoặc `httptest.NewRequest` rồi ghi đè) → 413; gửi lần 2 hợp lệ → 200 (bucket không bị tiêu bởi request 413).
5. **Test `bodylimit_test.go`.** (a) `Content-Length` > max → 413, handler không chạy (flag). (b) body chunked > max → handler nhận `MaxBytesError` khi `io.ReadAll` → qua `BindError` 413. (c) body ≤ max → 200. (d) route exempt: đăng ký `POST /api/v1/imports/roster` trong engine test, body > max → handler đọc được toàn bộ. (e) GET không body → 200. Nếu `apps/api/internal/server` có test harness cho `NewRouter` (grep `router_test.go`), thêm một test end-to-end `POST /api/v1/auth/forgot-password` với `Content-Length: 104857600` → 413; nếu không có harness, test middleware là đủ, ghi rõ trong report.
6. **Docs.** `docs/architecture.md` sơ đồ request lifecycle: thêm bước "body-limit (1 MiB mặc định, import roster tự cap 2 MiB)". `docs/api-guidelines.md`: thêm `PAYLOAD_TOO_LARGE` 413 vào danh sách mã lỗi (tìm nơi liệt kê `TOO_MANY_REQUESTS`) và một câu ở mục middleware/limits. `docs/deployment.md`: dòng env nếu có bảng env.
7. **Verify.** `make test-api-unit`, `make lint-api`, `make api-docs` (không diff mong đợi vì không thêm annotation swagger — non-goal).

## Success Criteria

- [x] `POST /api/v1/auth/forgot-password` với `Content-Length: 104857600` → 413 `PAYLOAD_TOO_LARGE`, handler không chạy.
- [x] Body chunked vượt cap → 413 (qua `JSONBodyKey` lẫn qua handler thường), không phải 400.
- [x] `POST /api/v1/imports/roster` với file 1.5 MiB vẫn qua cap toàn server; `TestOversizeUploadIsRejected` không đổi.
- [x] `API_HTTP_MAX_BODY_BYTES` mặc định 1 MiB, `<= 0` lỗi khởi động.
- [x] Docs kiến trúc + API guidelines cập nhật; `make test-api-unit`, `make lint-api` xanh.

## Risk Assessment

- **Cap 1 MiB quá thấp cho một route JSON hợp lệ** (ví dụ bulk attendance / import contacts qua JSON). Kiểm tra trước khi chốt: grep handler nhận mảng lớn (`attendance` bulk mark, `students` bulk) và ước lượng kích thước tối đa thực tế; nếu có route cần hơn, nâng default hoặc thêm vào exempt với cap riêng. Tín hiệu sau deploy: log 413 trên route không phải import → nâng cap qua env, không cần release.
- **`MaxBytesReader` đóng kết nối khi vượt cap** (`Connection: close`) — hành vi chuẩn Go, chấp nhận.
- **Gin bọc lỗi decoder** khiến `errors.As` không khớp → test bước 1 bắt sớm; fallback so chuỗi có ghi lý do.
- **Exempt bằng `FullPath` lệ thuộc chuỗi route** — nếu route import đổi path, cap 1 MiB áp lên và test import 1.5 MiB (bước 5d) sẽ đỏ, đủ làm lưới đỡ.
