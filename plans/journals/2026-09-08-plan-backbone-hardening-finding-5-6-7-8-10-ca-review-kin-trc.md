---
title: "Plan backbone hardening: finding 5/6/7/8/10 của review kiến trúc"
date: 2026-09-08
summary: "Plan 5 phase tại plans/260908-1009-backbone-hardening: refresh reuse grace không cần migration, BodyLimit toàn server 413, login limit + bcrypt gate + trusted proxies opt-in, SessionCacheReset web, enrollment invalidate sessionsKeys"
---

# Plan backbone hardening: finding 5/6/7/8/10 của review kiến trúc

## What happened

Chạy `/ak:plan` trên `plans/reports/review-260905-2208-architecture-review.md` cho finding 5, 6, 7, 8, 10 (đều CONFIRMED). Không spawn researcher; đọc trực tiếp `auth/service.go`, `repository.go`, `middleware/ratelimit.go`, `server/router.go`, `config.go`, `interceptors.ts`, `auth-store.ts`, `use-enrollments.ts`, `use-sessions.ts` và các test liên quan để chốt thiết kế. Kết quả: `plans/260908-1009-backbone-hardening/` với `plan.md` + 5 phase, validate OK, đã `ak plan use` và set active plan.

Sự cố nhỏ: `ak plan create` sinh thư mục theo giờ UTC với slug dài (`260908-0315-…`), không khớp naming của hook (`260908-1009-{slug}`); `mv` rồi `ak plan reindex --apply` để store nhận đường dẫn mới (store id `teka/260908-0327`). Hook scout-block chặn lệnh `find` có chuỗi `node_modules`, script `set-active-plan.cjs` nằm ở `~/.agentkit/cache/kits/engineer/…/scripts/`. `TaskCreate` không có trong runtime này nên bỏ bước hydrate task.

## Decision

- Finding 5: grace window phía server (`API_JWT_REFRESH_REUSE_GRACE`, 15s, 0 = tắt), **không migration**. Lý do: `Revoke(id)` đơn lẻ chỉ được gọi bởi rotation, mọi đường ác ý dùng `RevokeFamily`/`RevokeAllForUser`, nên "revoke gần đây + family còn token sống" ⇔ race rotation. Cấp sibling token cùng family; cần repo method `FamilyHasLive`. Loại Web Locks (không cứu request đã bay) và mã 401 riêng + client retry (phụ thuộc thứ tự response).
- Finding 6: `middleware.BodyLimit` toàn server 1 MiB, miễn `POST /api/v1/imports/roster` (tự cap 2 MiB), mã lỗi mới `PAYLOAD_TOO_LARGE` 413 qua `validation.BindError`; `JSONBodyKey` phải replay lỗi đọc thay vì để 400 mù mờ.
- Finding 7: ba lớp — `PhoneKey` chuẩn hoá số (dùng cả cho forgot-password), `bcryptGate` semaphore GOMAXPROCS chờ 2s → 429 áp cho cả `burnPassword` (giữ timing-safe), IP limiter chỉ khi `API_HTTP_TRUSTED_PROXIES` khác rỗng vì phụ thuộc Traefik forward XFF (câu hỏi mở của review).
- Finding 8: `SessionCacheReset` quan sát user id trong `useAuthStore` (A→null, A→B ⇒ `queryClient.clear()`), bỏ clear trùng trong `useLogout`, không đưa user id vào 19 key factory.
- Finding 10: mọi mutation ghi danh (create/end/delete/import/anonymize) invalidate `sessionsKeys.all` qua barrel attendance, theo tiền lệ `useReassignTeacher`.

## Next steps

- Handoff: đề xuất `/ak:plan red-team` trước khi cook vì chạm auth + middleware toàn server; rồi `/ak:cook plans/260908-1009-backbone-hardening/plan.md`.
- Câu hỏi mở còn lại: Traefik/cloudflared có forward XFF và subnet mạng `homelab` để điền trusted proxies; giá trị grace 15s có đủ cho mạng di động (đo bằng log `refresh reuse within grace` sau deploy).

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
