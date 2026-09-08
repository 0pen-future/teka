---
title: "Backbone hardening: refresh grace, body cap, login limit, web cache boundary, roster invalidation"
description: "Xử lý finding 5, 6, 7, 8, 10 của review kiến trúc 2026-09-05: refresh reuse có grace window, cap body toàn server, rate limit + bcrypt gate cho login, xoá query cache khi hết phiên, ghi danh invalidate roster điểm danh."
status: in-progress
priority: P1
effort: "3.5d"
tags: [api, web, security, auth, resilience, cache]
created: 2026-09-08
source: plans/reports/review-260905-2208-architecture-review.md
blockedBy: []
blocks: []
---

# Backbone hardening: refresh grace, body cap, login limit, web cache boundary, roster invalidation

Status: in-progress · Nguồn: `plans/reports/review-260905-2208-architecture-review.md` finding **5, 6, 7, 8, 10** (tất cả CONFIRMED). Finding 1–4 và 9 đã có plan riêng (`plans/260906-0627-authz-write-scope-root-cause`, completed) hoặc ngoài phạm vi plan này.

## Overview

Năm lỗi độc lập nhau, cùng thuộc nhóm "backbone / vận hành / web state" của review:

| Finding | Triệu chứng | Nguyên nhân đã xác minh trong code |
|---|---|---|
| 5 | Hai tab cùng refresh → cả hai bị đăng xuất | `auth/service.go` `Refresh`: token đã revoke (hoặc thua race `Revoke`) → `RevokeFamily` ngay, không phân biệt replay tấn công với tab thua race vài ms. Cookie `refresh_token` dùng chung mọi tab; single-flight web chỉ in-memory per tab. |
| 6 | POST body hàng trăm MB → buffer toàn bộ vào RAM → OOM | Không có `http.MaxBytesReader` toàn server (`server/router.go`, `server/server.go`); `middleware.JSONBodyKey` `io.ReadAll` trước rate limit; chỉ `imports/handler.go` `readUpload` tự cap 2 MiB. |
| 7 | 50 req/s login → bão hoà CPU bcrypt cost 12 | `auth/routes.go` mount `/login` không limiter (chỉ forgot/reset có); mọi nhánh thất bại vẫn chạy `burnPassword`; không có giới hạn đồng thời cho bcrypt; `SetTrustedProxies(nil)` nên không có identity IP để limit. |
| 8 | Người dùng B thấy dữ liệu cache của A trên cùng tab | `interceptors.ts` refresh thất bại chỉ `clearSession()`; `queryClient.clear()` chỉ ở `useLogout`; query key không chứa user id; gcTime mặc định 5 phút. |
| 10 | Ghi danh mới không xuất hiện trên sheet điểm danh đã mở (30s staleTime) → xác nhận thiếu em | `use-enrollments.ts` chỉ invalidate key roster (`enrollments`, `students`, `classes`), không chạm `sessionsKeys`; `use-roster-import.ts`, `use-students.ts` tương tự. |

## Contract

- **Outcome:** (5) Hai tab (hoặc một request retry) refresh cùng một cookie trong cửa sổ ngắn đều nhận phiên hợp lệ; replay ngoài cửa sổ hoặc sau logout vẫn giết family. (6) Mọi request vượt cap body bị từ chối 413 trước khi buffer; route import giữ cap riêng 2 MiB. (7) `/auth/login` bị giới hạn theo số điện thoại đã chuẩn hoá, bcrypt chạy có giới hạn đồng thời, và có limiter theo IP khi vận hành bật trusted proxies. (8) Mọi kết thúc phiên hoặc đổi danh tính người dùng xoá sạch TanStack cache, không cần đổi query key. (10) Mọi mutation tạo/kết thúc/xoá ghi danh (kể cả import và anonymize) làm roster/session điểm danh refetch.
- **Constraints:** Giữ hợp đồng API hiện có (envelope, mã lỗi, cookie path); chỉ thêm mã lỗi mới `PAYLOAD_TOO_LARGE`. Không sửa migration đã có; không cần migration mới. Không thêm dependency Go/npm. Swagger regenerate không diff trừ khi thêm annotation có chủ đích. Test hiện có chỉ được **bổ sung hoặc điều chỉnh mốc thời gian**, không bỏ assertion bảo mật. Tuân `docs/api-guidelines.md`, `docs/frontend-guidelines.md`, `apps/web/CLAUDE.md`.
- **Non-goals:** Khoá cross-tab phía web (Web Locks/BroadcastChannel); Traefik buffering label; đưa user/center id vào 19 key factory web; key registry dùng chung; đổi 400 → 413 cho upload import; hai chỗ thiếu invalidation khác review nêu (reassign→classStaff, attendance→dashboard periodPreview); cột `ip` audit_logs và cắt độ dài `X-Request-ID` (review "ngoài top 10"); finding 1–4, 9.
- **Acceptance criteria:** mục *Success Criteria* dưới; mỗi phase có tiêu chí riêng.

## Quyết định thiết kế đã chốt

| # | Quyết định | Lý do | Phương án đã loại |
|---|-----------|-------|-------------------|
| D1 | **Grace window phía server, không cần cột mới.** Token bị revoke bởi rotation trong `API_JWT_REFRESH_REUSE_GRACE` (mặc định `15s`, `0` = tắt) **và family vẫn còn token sống** → cấp token *anh em* (sibling) trong cùng family thay vì giết family. Cả hai đường thua race (`t.Revoked()` khi đọc, `ErrTokenAlreadyRevoked` khi update) đi qua một helper. | `Revoke` đơn lẻ chỉ được gọi bởi rotation; `RevokeFamily`/`RevokeAllForUser` giết cả family. Vì vậy "token revoke gần đây + family còn sống" ⇔ race rotation, không cần `replaced_by`. Server-only, web không đổi. Mẫu "reuse interval" của Auth0. | (a) Chỉ sửa web bằng Web Locks: không cứu được request đã bay đi với cookie cũ, Safari <15.4 không hỗ trợ. (b) 401 mã riêng + client retry: phụ thuộc thứ tự hai response về trình duyệt, nhiều bộ phận chuyển động hơn. |
| D2 | **`middleware.BodyLimit` toàn server** trong `r.Use` sau `Recovery`, cap mặc định 1 MiB (`API_HTTP_MAX_BODY_BYTES`), từ chối sớm 413 khi `Content-Length > cap`, bọc `http.MaxBytesReader` cho body không khai báo độ dài. `POST /api/v1/imports/roster` nằm trong danh sách miễn vì handler tự cap 2 MiB trước khi parse form. `validation.BindError` map `*http.MaxBytesError` → `apperror.PayloadTooLarge` (413). | Một chốt chặn, cấu hình được, có test; imports giữ cap riêng đã có test. | Cap tại Traefik: không có ở dev, ngoài repo, chỉ là lớp phụ. |
| D3 | **Ba lớp cho login:** (a) `RateLimit(PhoneKey("phone"), 10/phút)` với `PhoneKey` chuẩn hoá `0xxx`/`+84xxx` về một bucket (dùng luôn cho forgot-password); (b) `bcryptGate` semaphore `GOMAXPROCS` slot, chờ tối đa 2s → 429, áp cho cả compare thật lẫn `burnPassword`; (c) `RateLimit(ClientIPKey(), 60/phút)` **chỉ mount khi** `API_HTTP_TRUSTED_PROXIES` khác rỗng (opt-in, mặc định không đổi hành vi). | (a) chặn brute-force một tài khoản; (b) chặn cạn CPU với số điện thoại ngẫu nhiên — đây là nguyên nhân gốc của kịch bản review; (c) cần identity IP, phụ thuộc Traefik forward XFF (câu hỏi mở #2 của review) nên gated bằng config. | Bỏ `burnPassword` cho phone lạ: lộ timing oracle. Limiter toàn cục một bucket: một kẻ tấn công khoá login mọi người. |
| D4 | **`SessionCacheReset` component** (features/auth, render null, mount trong `Providers`) quan sát `useAuthStore` user id: `non-null → null` hoặc `A → B` ⇒ `queryClient.clear()`. `useLogout` bỏ `queryClient.clear()` trùng. `renderWithProviders` cũng mount component này. | Một điểm chốt theo *trạng thái* thay vì theo *call site*: mọi đường kết thúc phiên hiện tại và tương lai (logout, refresh chết, đổi user) đều được phủ; test được với queryClient của từng test. | Đưa user id vào mọi query key: 19 factory, dễ sót, cache cũ vẫn chiếm RAM tới gcTime. `queryClient.clear()` trong `auth-store`: store phải import singleton, test không quan sát được. |
| D5 | Mọi mutation ghi danh invalidate **`sessionsKeys.all`** (import từ barrel `@/features/attendance` như `use-classes.ts` đã làm): `useCreateEnrollment`, `useEndEnrollment`, `useDeleteEnrollment`, `useImportRoster` (committed), và mutation ở `use-students.ts` đang invalidate `enrollmentsKeys.all`. | `sessionSchema` có `student_count` trên cả list và detail, roster là key riêng → ba nhánh; `sessionsKeys.all` gọn, cùng tiền lệ `useReassignTeacher`, chi phí chỉ là refetch các query đang mount. | Key registry dùng chung: ngoài phạm vi finding 10. |

## Phases

| # | Phase | Status | Phụ thuộc | Ước lượng |
|---|-------|--------|-----------|-----------|
| 1 | [API: refresh reuse grace window](./phase-01-refresh-reuse-grace.md) | Completed | — | 1d |
| 2 | [API: cap body toàn server + 413](./phase-02-body-limit.md) | Completed | — | 0.5d |
| 3 | [API: login rate limit + bcrypt gate + trusted proxies opt-in](./phase-03-login-rate-limit-bcrypt-gate.md) | Pending | 2 (cùng sửa `ratelimit.go`) | 1d |
| 4 | [Web: xoá query cache tại ranh giới phiên](./phase-04-web-session-cache-reset.md) | Completed | — | 0.5d |
| 5 | [Web: ghi danh invalidate roster điểm danh](./phase-05-enrollment-roster-invalidation.md) | Completed | — | 0.5d |

Nhóm chạy song song được: `{1}`, `{2 → 3}`, `{4}`, `{5}` — khác file, khác app. Phase 3 sau phase 2 vì cả hai chạm `apps/api/internal/middleware/ratelimit.go`, `server/router.go`, `config/config.go`, `.env.example`.

## Files (tóm tắt — chi tiết trong từng phase)

- **Mới (api):** `internal/middleware/bodylimit.go` (+test), `internal/features/auth/bcrypt_gate.go` (+test).
- **Sửa (api):** `config/config.go`, `.env.example`, `server/router.go`, `middleware/ratelimit.go` (+test), `shared/apperror/apperror.go`, `shared/validation/validation.go`, `features/auth/{service.go,repository.go,routes.go,tokens.go}` (+ `service_test.go`, `integration_test.go`, `handler_test.go`).
- **Mới (web):** `src/features/auth/session-cache-reset.tsx` (+test).
- **Sửa (web):** `src/app/providers.tsx`, `src/features/auth/index.ts`, `src/features/auth/hooks/use-auth.ts`, `src/test/utils.tsx`, `src/features/roster/hooks/{use-enrollments.ts,use-roster-import.ts,use-students.ts}` (+test).
- **Docs:** `docs/api-guidelines.md` (refresh grace, 413, login limit), `docs/architecture.md` (request lifecycle), `docs/deployment.md` (trusted proxies), `docs/frontend-guidelines.md` (ranh giới cache).
- **Không đụng:** migrations, `imports/handler.go` cap 2 MiB, `interceptors.ts` (hành vi giữ nguyên), query key factories, `components/ui/*`.

## Success Criteria

- [ ] F5: test unit + integration chứng minh hai refresh đồng thời cùng token → cả hai 200, family còn sống với 2 token live; replay sau grace hoặc sau logout → 401 + family chết; `API_JWT_REFRESH_REUSE_GRACE=0` khôi phục hành vi cũ nguyên vẹn.
- [ ] F6: `POST /api/v1/auth/forgot-password` với `Content-Length` 100 MB trả 413 `PAYLOAD_TOO_LARGE` mà handler và limiter không chạy; body chunked vượt cap cũng 413; `POST /imports/roster` 2 MiB vẫn qua cap toàn server và giữ hành vi cũ.
- [ ] F7: 11 lần login sai cùng số (`0…` và `+84…` xen kẽ) → lần 11 là 429; số khác không ảnh hưởng; gate 1 slot bận → 429 không publish `LoginFailed`; env trusted proxies rỗng → `SetTrustedProxies(nil)` và không mount IP limiter (router test).
- [ ] F8: cache có dữ liệu, `clearSession()` (refresh chết hoặc logout) → `queryCache` rỗng; đổi user A→B → rỗng; `setAccessToken`/`setUser` cùng id → giữ nguyên; test MSW: query 401 → refresh 401 → cache rỗng.
- [ ] F10: sau `mutateAsync` của create/end/delete/import/anonymize, `queryClient.getQueryState(sessionsKeys.roster(id)).isInvalidated === true`.
- [ ] `make test-api` (kể cả integration Docker), `make lint-api`, `cd apps/web && npm run typecheck && npm run test`, `make lint-web` đều xanh; `make api-docs` không diff ngoài ý định.
- [ ] Docs bốn file trên phản ánh hành vi mới; review cross-module (`/ak:code-review` hoặc reviewer) trước merge vì chạm auth và middleware toàn server.

## Rủi ro chung

- **Phase 1 nới lỏng phát hiện replay trong 15s** (kẻ trộm cookie replay trong grace nhận token sống). Chấp nhận: cửa sổ ngắn, cấu hình được về 0; ghi rõ trong docs. Tín hiệu vỡ giả định: audit thấy nhiều family có >2 token live cùng lúc → giảm grace hoặc tắt.
- **Limiter và gate in-memory per-instance** (câu hỏi mở #3 của review: prod single replica). Nếu scale-out, giới hạn nhân theo số replica; chấp nhận vì compose hiện single replica; ghi trong docs.
- **Trusted proxies cấu hình sai** (CIDR quá rộng) → XFF giả mạo → limiter IP bị vượt và audit ip sai. Mặc định tắt; deployment doc hướng dẫn xác minh bằng log.

## Câu hỏi chưa giải quyết

1. Traefik/cloudflared trong homelab có forward `X-Forwarded-For` tới container API không, và subnet của mạng `homelab` là gì? Quyết định giá trị `API_HTTP_TRUSTED_PROXIES` khi bật lớp (c) của phase 3. Không chặn plan: mặc định để trống.
2. Giá trị grace mặc định 15s có phù hợp với độ trễ mạng di động của người dùng thực không? Có thể đo bằng log `refresh reuse within grace` (phase 1 thêm log Info) sau khi deploy.
