---
title: "Hoàn tất audit log toàn hệ thống (plan 260826-1228, 6 phases TDD)"
date: 2026-08-26
summary: "Event bus in-process generic + audit capture + owner read API + trang web audit + e2e Playwright + docs; toàn bộ gates xanh, plan 72/72 tasks"
---

# Hoàn tất audit log toàn hệ thống (plan 260826-1228, 6 phases TDD)

## What happened

Hoàn tất phase 5 và 6, đóng plan "System-wide audit log" (6/6 phases, 72/72
tasks). Session này:

- **Phase 5 (web)**: feature `apps/web/src/features/audit` — zod schemas,
  `useInfiniteQuery` với filters trong queryKey + enabled-gate theo owner,
  bảng có expand chi tiết, filter actor/nhóm hành động/free-text/từ-đến ngày,
  guard 3 lớp (nav entry, redirect deep-link, query không fire cho member).
  TDD: 8 page tests + layout tests đỏ trước, xanh sau. Review 2H/4M: fix H1
  (isError không được blank rows đã load — TanStack v5 isError coexist với
  data), H2 (pin TZ=Asia/Ho_Chi_Minh vào vitest + literal cases cho
  formatDateTime), M1–M3; M4 (date input write-only state) chấp nhận.
- **Phase 6 (e2e + docs)**: `apps/web/e2e/audit.spec.ts` — owner tạo + đổi tên
  contact rồi thấy row `contact.update` bind theo contact id của chính run
  (chống xanh giả trên DB tái sử dụng); member không có nav, deep-link bị
  redirect. Chạy trên stack isolated `compose -p teka-e2e` fresh seed: 2/2,
  full suite 24/24; chạy lại lần 2 không reseed vẫn pass. Docs mới
  `docs/event-bus.md` (bus contract, at-most-once, event catalog 5 events
  link source, action-map convention, blind spots, tunables `API_AUDIT_*`,
  grace ≥30s) + cập nhật architecture.md, deployment.md. Review phase 6:
  0H/2M/7L — fix hết, trong đó M2 phát hiện `collection.` trong filter groups
  là copy nhầm từ fixture test, thay bằng `billing.`/`teacher.` khớp 18
  prefix thực của action map.

## Lessons

- jsdom không implicit-submit form thiếu submit button → thêm nút "Lọc"
  (vừa fix test vừa tốt cho a11y). Radix Select render native select ẩn chứa
  text trùng → scope assertion bằng `within(table)`.
- E2e assertion phải bind với dữ liệu của chính run (id trong path/entity),
  không chỉ match action name — DB tái sử dụng làm test xanh giả khi pipeline
  hỏng. Mutation dạng `:id` (update/delete) mới mang entity id; create thì
  không (không có route param).
- Docs claims phải đối chiếu source từng câu: action row `auth.login_fail`
  khác event name `auth.login_failed`; logout idempotent vẫn revoke nhưng
  skip publish.

## Next steps

- Backlog (user đã quyết): rate limit `POST /auth/login` theo IP.
- ~49 file chưa commit trên master (user chọn gộp commit sau) — chờ user
  quyết định commit qua git-manager.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
