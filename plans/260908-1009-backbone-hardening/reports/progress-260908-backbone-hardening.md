---
date: 2026-09-08
branch: feat/backbone-hardening
range: master..feat/backbone-hardening
---

# Tiến độ cuối — Backbone hardening (finding 5, 6, 7, 8, 10)

## Trạng thái: hoàn thành, sẵn merge

Cả 5 phase implement + commit, review 3 cycle đạt DONE (không finding mở), test/lint/typecheck/docs xanh.

## Theo phase

| # | Phase | Commit | Kết quả |
|---|-------|--------|---------|
| 1 | Refresh reuse grace | `c91ff2f` | Sibling token trong grace 15s; replay ngoài grace/sau logout/tài khoản disable → 401 + family chết; `grace=0` giữ hành vi cũ. |
| 2 | Body cap toàn server | `71b9e9a` | 413 `PAYLOAD_TOO_LARGE` trước handler (Content-Length khai gian và chunked); import roster giữ cap riêng 2 MiB, có test router-level pin exemption (NIT-8). |
| 3 | Login rate limit + bcrypt gate + trusted proxies | `c4c7ab5` | Phone limiter 10/phút, gate bcrypt GOMAXPROCS slot 2s timeout → 429, IP limiter opt-in qua `API_HTTP_TRUSTED_PROXIES`. |
| 4 | Web SessionCacheReset | `f3f6701` | Component quan sát `useAuthStore`, `queryClient.clear()` tại mọi ranh giới phiên (logout, refresh chết, đổi user). |
| 5 | Enrollment invalidate roster session | `0204764` | 5 đường mutation ghi danh (create/end/delete/import commit/anonymize) invalidate `sessionsKeys.all`. |

Fix bổ sung Docker integration harness: `a8e3fc4`. Review fixes: `e8a9c1c`, `1efaa0f`, `735965f`.

## Review — 3 cycle + 1 xác nhận

| Cycle | Điểm | Ghi chú |
|---|---|---|
| 1 | 8/10 | MAJOR-1 (rate limit login/forgot-password bị vòng qua bằng đổi hoa/thường tên field JSON) + 6 MINOR + 2 NIT. |
| 2 (`e8a9c1c`) | 8.5/10 | 8/9 finding cycle 1 đã sửa đúng; MAJOR-1 mới vá nửa (còn lệch qua khoá trùng thứ tự). |
| 3 (`1efaa0f`) | 9/10 | MAJOR-1 đóng đúng cách (`reflect.StructOf` + `encoding/json`); phát hiện MAJOR-11 mới (có sẵn trên master): `json.Unmarshal` vs `json.Decoder` cho phép byte thừa sau JSON object vòng qua limiter. |
| Xác nhận (`735965f`) | **9.5/10 — DONE** | MAJOR-11 đóng bằng một dòng (`json.NewDecoder(...).Decode(...)`); không còn finding mở. |

Report đầy đủ: `reports/code-review-260908-backbone-hardening.md`.

## Verification (chạy sau commit cuối, bởi lead)

- `make test-api-unit` xanh, `make lint-api` 0 issues, `make api-docs` không diff.
- Integration Docker: `go test -tags=integration -p 2 ./internal/server/ ./internal/features/auth/` xanh (bao gồm 2 test tích hợp phase 1 mà cả 3 cycle review chưa chạy được vì suite Docker bận).
- Web: typecheck/test/lint xanh (5 warning `react-hooks/incompatible-library` đã biết, không liên quan branch).

Report chi tiết: `reports/test-report-260908-backbone-hardening.md` (chạy trước commit review-fix cuối; các con số trên đã được lead xác nhận lại sau `735965f`).

## Quyết định/rủi ro đã chấp nhận

- **Không giới hạn số token anh em sinh trong grace 15s** (MINOR-4, ghi trong `phase-01` Risk Assessment): chấp nhận vì cửa sổ ngắn và mọi token anh em cùng gãy khi family bị revoke. Tín hiệu cần xem lại: log `refresh reuse within grace` lặp nhiều lần cùng `family_id`.
- **Nới lỏng phát hiện replay trong 15s**: chấp nhận có chủ đích (D1), cấu hình được về 0.
- **Limiter/gate in-memory per-instance**: chấp nhận cho compose single-replica hiện tại; ghi trong `docs/deployment.md`.

## Thay đổi hành vi do review (đã đồng bộ vào phase docs)

- `JSONBodyKey`/`PhoneKey`: bỏ tra `map[string]any`, decode qua `encoding/json` với `reflect.StructOf` probe + `json.Decoder` — khớp đúng luật bind của gin (hoa/thường, khoá trùng lấy cái cuối, bỏ qua byte thừa). Ghi ở `phase-03`.
- `Refresh`: kiểm tra `Revoked()` trước `Expired()` — replay token cũ đã hết hạn vẫn giết family (khớp Requirements gốc, MINOR-2).
- Nhánh cứu (`reuseAfterRotation`) chỉ `RevokeFamily` khi lỗi là 401 thật (tài khoản mất/không active); lỗi transient (500) propagate nguyên vẹn, không giết family oan. Ghi ở `phase-01` (MINOR-3).
- `bcryptGate`: lỗi `ctx.Canceled`/`DeadlineExceeded` khi caller bỏ đi cũng map về 429, không còn lọt thành 500 giả trong log (MINOR-5).
- Swagger `/auth/login` thêm `@Failure 429` (MINOR-6); `docs/architecture.md` sửa dòng "no trusted proxies" đã lỗi thời (MINOR-7).
- `bcrypt_gate.go`: bỏ lớp bọc `max(1, GOMAXPROCS(0))` thừa (NIT-9) — `runtime.GOMAXPROCS(0)` luôn ≥ 1. Đã sửa `phase-03` step 2.
- Test router-level `TestRosterImportIsExemptFromGlobalBodyCap` pin liên kết giữa hằng exempt và route thật (NIT-8). Đã ghi vào `phase-02`.

## Việc còn lại (không chặn merge)

- Không có follow-up bắt buộc. Review cycle 3 lưu ý MAJOR-11 "có sẵn trên master, không do branch tạo ra" nhưng branch đã tiện tay sửa luôn.
- Docker integration được lead chạy, không phải reviewer (3 cycle review đều note "chưa chạy được vì suite Docker bận process khác") — đã xác minh xanh sau commit cuối, không còn khoảng trống evidence.

## Câu hỏi chưa giải quyết

1. Grace mặc định 15s có phù hợp độ trễ mạng di động thực tế không? Đo bằng log `refresh reuse within grace` sau khi deploy (câu hỏi mở #2 của plan gốc).
2. `API_HTTP_TRUSTED_PROXIES` nên đặt giá trị gì trong homelab (Traefik/cloudflared có forward XFF không, subnet mạng `homelab`)? Câu hỏi mở #1 của plan gốc, mặc định để trống không chặn merge.
