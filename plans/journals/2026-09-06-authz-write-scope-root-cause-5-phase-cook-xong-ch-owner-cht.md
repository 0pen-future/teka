---
title: "Authz write-scope root cause: 5 phase cook xong, chờ owner chốt D9/OQ6"
date: 2026-09-06
summary: "Đóng lỗ hổng *.view_all mở quyền ghi; Anchor tách khỏi Scope; reports.send suy ra key đọc; scopelint go/analysis; routespec manifest. Chưa commit, chưa CI."
---

# Authz write-scope root cause: 5 phase cook xong, chờ owner chốt D9/OQ6

## What happened
Cook `plans/260906-0627-authz-write-scope-root-cause/plan.md` (`--auto --tdd --advise`), 5 phase trên một cây làm việc, chưa commit (118 file so với master 57986c5).

- Phase 1: `CenterWideFor(key)` chỉ nới đọc; `WriteWide()` owner-only. Hotfix H-1 trong payments (allocation bỏ sót invoice của giáo viên không phải owner).
- Phase 2: `authctx.Anchor`/`OwnerAnchor` là kiểu riêng, không dựng được `Scope{IsOwner:true}` ngoài `centers`.
- Phase 3: `reports.send` suy ra `billing/statements/notifications/contacts.view_all` (`impliedKeys`, thêm sau bước deny). Kéo theo `zalo.MatchFriendsScoped` chuyển sang `contacts.view_all`. `MarkSent` đếm hàng ghi được theo id distinct → 404 khi có id ngoài tầm.
- Phase 4: `apps/api/tools/scopelint` (go/analysis): R1 scope witness theo hình dạng kiểu (receiver có `*gorm.DB`, helper cùng receiver nhận Scope/Anchor, hoặc selector `CenterID`), R2 cấm `Unscoped()`/`Session{NewDB:true}`, R3 authority ở lại authctx. `tree_test.go` load cả cây, sàn 18 repository package (thực tế 21). Chạy trong `go test` → CI qua `make test-api`; `lint-api`/`test-api-unit` có prerequisite `scopelint`. Xoá `scoping_guard_test.go`.
- Phase 5: `internal/shared/routespec` (128 spec) cấp cho route policy, audit action, middleware skip set.

Spec phase 4 ban đầu (raw-root = vi phạm) suy ra ~180 directive so với ngân sách 8 → viết lại thành witness theo kongming; witness là "có nhắc", không phải dataflow, RLS là backstop.

Sự cố: một subagent chạy `git stash` trái chỉ dẫn (stash list rỗng, cây nguyên); một lần revert probe mất newline cuối `billing/repository.go`, đã khôi phục và so byte với snapshot.

## Verification
`make test-api` 37 package xanh, coverage 77.8%; `make lint-api` 0; `make scopelint` 0 diagnostic/0 directive; `make api-docs` không drift. CI chưa chạy (0 push).

## Decision
- Merge một lần, 5 commit trên branch `authz-write-scope` (công thức: `plans/reports/kongming-final-checkpoint-260907.md` §3). Rollback = revert, không migration.
- Cần owner chốt D9 (member có `payments.create` ghi thanh toán; chỉ owner reverse/reallocate; member không xem lại phiếu nếu thiếu `payments.view_all`) và OQ6 (`students.Create` owner-only?) trước merge.

## Next steps
- User quyết định commit; mở PR sớm để CI chạy trước khi chờ owner.
- Sau deploy: kiểm kê lại D4 + payment dư của member, owner chạy `POST /payments/:id/allocations/auto`.
- Follow-up: web `member-permissions-dialog.tsx` chưa hiện key suy ra; comment `D8`/`D4`/"phase 2" có sẵn trên master; test chống chuỗi `impliedKeys`; câu về `*Anchored` trong docs Tenancy; `tree_test` chưa truyền `-tags=integration`.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
