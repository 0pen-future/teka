---
title: "Zalo Personal Send hoàn tất: Phase 5 web channel + run progress"
date: 2026-08-07
summary: "Hoàn tất plan zalo-personal-send: Phase 5 (kênh gửi + banner tiến độ run) TDD, 11 fix từ review, gates xanh toàn bộ, plan sync-back"
---

# Zalo Personal Send hoàn tất: Phase 5 web channel + run progress

## What happened

Chốt plan `plans/260807-1224-zalo-personal-send/` bằng Phase 5 (web channel + run progress, TDD): radio chọn kênh gửi (personal/manual) với `effectiveChannel` chặn kênh disabled, confirm dialog "X tự động · Y thủ công", banner tiến độ run poll 2s (`run-progress-banner.tsx`), fallback/failed rows theo run, resume run gián đoạn.

Review loop bắt được 1 lỗi Critical thật: ledger `GET /billing-periods/:id/notifications` trả `response.OK` không có `meta`, còn web parse bằng `parseList` (bắt buộc `meta`) — MSW mock tự bịa `meta` nên test không lộ. Fix: thêm `parseArray` trong `src/lib/api/envelope.ts` cho endpoint list không phân trang, sửa mock khớp wire thật.

6 concern user chọn đã xử lý TDD: counts confirm dialog tính từ số dư kỳ (`useContactCollectionsList`) giao với mapping Zalo (test nhắc nợ chứng minh "2 tự động · 1 thủ công"); `run_id` xuyên suốt Go DTO → zod → UI để pin failed rows đúng run (lỗi run cũ không lẫn banner run mới); cảnh báo gửi trùng khi "Tạo lại" thủ công sau run tự động; nút "Ẩn" banner run kết thúc; gửi `channel: "zalo_manual"` tường minh; a11y `aria-describedby` cho radio disabled + fallback khi `/me/zalo` lỗi.

Gates cuối: vitest 170/170, eslint 0 lỗi, tsc/prettier sạch, build OK; Go test notifications (docker, `-tags=integration`) xanh, `go build ./...` sạch.

## Decision

- Fix envelope phía web (`parseArray`) thay vì đổi backend sang `response.List`: endpoint thật sự không phân trang, blast radius nhỏ nhất.
- Giữ test polling trên real timers (user chọn, không convert fake timers) — chấp nhận ~9 test real-timer với budget 15s/test.
- Hoãn có ghi nhận: mã lỗi 409 riêng cho expired-session (hiện chỉ substring `EXPIRED_SESSION_409`), gate 2 query đếm theo `personalReady`, ghi chú deployment về `NOTIFICATIONS_DEFAULT_CHANNEL` (đã sửa comment `.env.example`: default chỉ áp dụng cho caller ngoài UI).

## Next steps

- User tự commit ~81 file đang chờ trên master.
- Cân nhắc backlog: error code 409 riêng, gate query đếm, trần số row ledger khi "Tạo lại" nhiều lần.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
