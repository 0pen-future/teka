---
title: "Phase 6: E2e And Docs"
status: done
priority: P2
effort: "0.5d"
dependencies: [5]
---

# Phase 6: E2e And Docs

## Overview

Khép vòng: Playwright e2e xuyên stack (mutation → thấy trong trang audit),
docs cập nhật (event bus + event catalog + quy ước action map), sweep
acceptance criteria toàn plan.

## Requirements

- [x] E2e: owner login → tạo 1 mutation (vd tạo class) → mở trang audit →
      thấy dòng tương ứng; member login → không thấy nav audit
- [x] Chạy trên isolated e2e stack (`docker compose -p teka-e2e` với port +
      URL overrides; specs cần fresh seed — theo quy ước e2e hiện có của repo)
- [x] Docs: kiến trúc event bus (1 trang ngắn trong `docs/` theo docs nav
      hiện có): Bus API, at-most-once guarantee, cách thêm subscriber mới,
      event catalog (RequestCompleted + 3 auth events, field list), quy ước
      thêm action map entry khi thêm route mutating
- [x] README/docs nav trỏ tới trang mới nếu docs có index

## Related Code Files

- Create: `apps/web/e2e/audit.spec.ts` (theo pattern spec e2e hiện có)
- Create/Modify: `docs/` — xác định file đích qua docs nav hiện có lúc
  implement (không bịa tên file trước; docs.maxLoc 800)
- Modify: seed e2e nếu cần user member (đối chiếu seeds hiện có)

## Implementation Steps

1. Đọc 1 spec e2e hiện có + cách seed; viết `audit.spec.ts` (owner flow +
   member negative).
2. Chạy e2e stack isolated, xác nhận pass.
3. Viết docs event bus + event catalog; verify claims khớp code (field names,
   buffer size, drop policy).
4. **Whole-plan sweep:** đối chiếu 8 success criteria trong `plan.md`,
   chạy `go test ./... -race`, `npm run lint/typecheck/test/build`, đánh dấu
   checklist. Ghi nhận follow-ups (retention, login-fail visibility) vào
   docs hoặc report.

Carry-forward từ review Phase 3 (user quyết 260826):

- Docs vận hành: `stop_grace_period: 30s` đã thêm vào compose dev/prod (M2 —
  shutdown xấu nhất ~20s: HTTP 10s + bus drain 5s + flush 5s); manifest deploy
  ngoài repo phải đặt grace ≥ 30s. Kèm docs env `API_AUDIT_*` (L6).
- Backlog (ngoài scope plan này): rate limit `POST /auth/login` theo IP (M3 —
  login sai không giới hạn bơm dòng audit; kích thước dòng đã bị clip 512B/1KB).
- Blind spots ghi vào docs: mutation bị 401 không để lại dấu vết (L4, theo
  quyết định skip principal-less); logout bằng token cũ của family đã revoke
  không publish event (L2, per-token check).

## Todo

- [x] audit.spec.ts pass trên e2e stack isolated
- [x] Docs event bus + catalog + action-map convention
- [x] Full verification sweep + checklist plan.md

## Success Criteria

- [x] Toàn bộ acceptance criteria plan.md được tick với bằng chứng (test
      name / lệnh đã chạy)
- [x] Docs không copy chi tiết máy-own (schema cột đầy đủ link tới migration
      thay vì chép lại)

## Risk Assessment

- E2e flaky do async flush → trang audit đọc sau khi flush interval 1s; spec
  dùng polling expect (Playwright auto-retry) thay vì sleep cứng.
- Docs drift — event catalog chỉ liệt kê tên + mục đích, field chi tiết link
  sang source.

## Ghi chú hoàn thành (260826)

- `apps/web/e2e/audit.spec.ts`: owner (Cô Lan) tạo contact → thấy dòng
  `contact.create` (actor + badge 201 + chi tiết `POST /api/v1/contacts`) trên
  trang audit, polling reload thay vì sleep cứng; member (Thầy Minh) không có
  nav entry và bị redirect khỏi `/audit`. Chạy trên stack isolated
  `docker compose -p teka-e2e` (fresh seed): 2/2 pass, full suite 24/24 pass
  (2.3m). Stack đã `down -v`, port trả sạch.
- Seed có sẵn cặp owner+member (`apps/api/seeds/seed.go`) — không cần sửa seed.
- Docs: trang mới `docs/event-bus.md` (Bus API, at-most-once, thêm subscriber,
  event catalog 5 events link source, action-map convention, blind spots L4/L2,
  backlog rate-limit login, tunables `API_AUDIT_*`, grace ≥30s);
  `docs/architecture.md` thêm mục In-process events + request-events trong
  lifecycle; `docs/deployment.md` thêm hàng env `API_AUDIT_*` + đoạn stop
  grace ≥30s. Claims đối chiếu source (fallback "METHOD route" tại
  `features/audit/action.go`, tunables default tại `internal/config/config.go`,
  `stop_grace_period: 30s` ở cả 2 compose). README không có index per-file nên
  không cần trỏ thêm.
- Sweep toàn plan: `go test ./... -race` pass; web lint 0 error (4 warning có
  sẵn), typecheck sạch, vitest 387 passed/3 skipped (60 files), build pass.

## Ghi chú sau review (260826)

Review report: `plans/reports/review-260826-phase-06-e2e-and-docs.md` —
DONE_WITH_CONCERNS, 0 High / 2 Medium / 7 Low; toàn bộ đã fix trong session:
spec bind row audit với contact id của run hiện tại qua `contact.update`
(chống xanh giả trên DB tái sử dụng — đã chạy 2 lần liên tiếp pass), filter
groups đồng bộ 18 prefix của action map (`billing.`/`teacher.` thay
`collection.`), docs chỉnh câu chữ khớp source (login_fail, logout idempotent,
unmatched-route skip, "at least ~20s").
