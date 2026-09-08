---
title: "Backbone hardening: 5 phase, limiter phải decode như gin"
date: 2026-09-08
summary: Cook plan backbone-hardening trên feat/backbone-hardening; review 3 vòng vì khoá rate-limit lệch với JSON binding của gin
---

# Backbone hardening: 5 phase, limiter phải decode như gin

## Chuyện gì đã xảy ra

Cook plan `plans/260908-1009-backbone-hardening/` (finding 5, 6, 7, 8, 10 của review kiến trúc 260905) ở chế độ auto, 5 phase liên tục trên nhánh `feat/backbone-hardening`, mỗi phase một commit:

- Phase 1 `c91ff2f`: refresh reuse grace (`API_JWT_REFRESH_REUSE_GRACE`, mặc định 15s) cứu race hai tab cùng refresh, dựa trên bất biến "chỉ rotation gọi `Revoke` đơn lẻ" + `FamilyHasLive`.
- Phase 4 `f3f6701`, phase 5 `0204764`: web `SessionCacheReset` và enrollment mutation invalidate `sessionsKeys.all`.
- Phase 2 `71b9e9a`: `BodyLimit` toàn server (`API_HTTP_MAX_BODY_BYTES` 1 MiB), 413 `PAYLOAD_TOO_LARGE`, import roster miễn.
- Phase 3 `c4c7ab5`: login limiter theo phone chuẩn hoá 10/phút, `bcryptGate` GOMAXPROCS slot chờ 2s → 429, `API_HTTP_TRUSTED_PROXIES` opt-in mới mount IP limiter.
- `a8e3fc4`: harness integration policy dựng config tay nên MaxBodyBytes=0 chặn mọi body; phải set 1 MiB.

Review chạy 3 vòng sửa (8 → 8.5 → 9 → 9.5) và cả ba vòng đều xoay quanh một lớp lỗi: `JSONBodyKey` tự cài lại luật tìm field JSON, còn handler bind qua gin/encoding/json với luật khác.

1. Tra map chính xác → `{"Phone": ...}` qua limiter với khoá rỗng. Vá bằng `strings.EqualFold` (`e8a9c1c`).
2. Vẫn lệch với khoá trùng: encoding/json lấy khoá **cuối**, map lookup ưu tiên khớp chính xác. `{"phone":"rác","Phone":"nạn nhân"}` bucket rác, bcrypt nạn nhân. Test vòng 1 chọn đúng thứ tự mà hai cách tình cờ trùng. Vá bằng `reflect.StructOf` một field tag `json:"<field>"` rồi unmarshal (`1efaa0f`).
3. `json.Unmarshal` từ chối byte thừa, gin dùng `json.Decoder` chỉ đọc một giá trị: body `...}{}` qua limiter với khoá rỗng. Lỗi có sẵn trên master. Vá bằng `json.NewDecoder(...).Decode` (`735965f`).

Ngoài ra review kéo về: `Refresh` kiểm `Revoked()` trước `Expired()` (như master, replay token cũ hết hạn vẫn giết family); nhánh cứu trong grace revoke family khi account disabled/missing thay vì return sớm; ctx hủy khi chờ gate → 429 thay vì 500; swagger login có 429; test router pin exemption `/api/v1/imports/roster`.

## Quyết định

- Không cài lại luật của encoding/json ở bất kỳ đâu: khi cần "nhìn trước" một field của body, decode bằng chính đường mà handler sẽ dùng (struct + tag + Decoder). Đây là bài học chung cho mọi middleware đọc body.
- Số token anh em sinh trong grace 15s không có trần: chấp nhận, ghi tín hiệu xem lại trong phase-01 (log `refresh reuse within grace` lặp cùng `family_id`).
- Mọi lỗi từ `bcryptGate.run` (busy lẫn ctx) đều 429, không `LoginFailed`, vì credentials chưa được phán xét.

## Kiểm chứng

`make test-api-unit`, `make lint-api` 0 issues, `make api-docs` không diff, integration `-tags=integration -p 2` cho `internal/server` + `internal/features/auth` xanh trên `735965f`, web typecheck/test/lint xanh (5 warning lint có sẵn). Report: `plans/260908-1009-backbone-hardening/reports/`.

## Bước tiếp

- Commit thay đổi plan + 3 report (chưa commit), rồi push/PR khi user yêu cầu.
- Vận hành: chốt giá trị `API_HTTP_TRUSTED_PROXIES` cho homelab theo hướng dẫn mới trong `docs/deployment.md`; theo dõi 429 "busy" và 413 ngoài route import sau deploy.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
