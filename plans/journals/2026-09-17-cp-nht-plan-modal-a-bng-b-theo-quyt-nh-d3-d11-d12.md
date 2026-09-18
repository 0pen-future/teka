---
title: Cập nhật plan Modal A + Bảng B theo quyết định D3/D11/D12
date: 2026-09-17
summary: "Đảo D3 sang lọc + đếm phía server cho GET /tasks/board, bỏ segmented phạm vi theo report (D11), assignee đổi Cột trong modal (D12); 6 phase cập nhật, effort 7d."
---

# Cập nhật plan Modal A + Bảng B theo quyết định D3/D11/D12

## What happened

- Chủ dự án chốt ba câu hỏi mở của `plans/260917-2142-task-modal-a-board-b/`: lọc nhanh phải do backend làm, UI Bảng B theo report 100%, và người chỉ là assignee được đổi "Cột" ngay trong Modal A.
- Kiểm chứng trước khi sửa: handler `board` không đọc query param nào (`apps/api/internal/features/tasks/handler.go:56-67`) trong khi web vẫn gửi `?scope=` (`tasks-api.ts:19-21`); adapter luôn echo scope từ policy (`service.go:87-91`) nên segmented phạm vi hiện là control chết với người có `view_all`. `ListBoard` là subquery `row_number() OVER (PARTITION BY column_id)` (`task_repository.go:32-71`) nên predicate lọc chèn được vào `WHERE` bên trong và `has_more` vẫn đúng. `CanMoveTask` gồm assignee (`pkg/kanban/policy.go:47-49`), route move cần `tasks.edit` (`routespec.go:380`) → server đã sẵn sàng cho D12.
- Cập nhật `plan.md` (D3, D4, D9 bãi bỏ, D11, D12 mới, effort 7d, risk/success criteria) và 6 phase: Phase 1 thêm hợp đồng `filter|assignee|today` + `counts`, core `BoardFilter`/`CountBoard`; Phase 4 gỡ segmented + phụ đề từ `counts` (deps thêm Phase 1); Phase 5 bỏ selector client, `toFullIndex`, `hasMore`; Phase 3 nhánh read-only chỉ gọi move; Phase 6 e2e/docs theo hợp đồng mới.

## Decision

- Lọc và đếm đi qua core (`Policy.Visibility` vẫn là luật đọc duy nhất); `mine` suy ra ở adapter từ `by_assignee[actor]`.
- `today` do client gửi (ngày lịch địa phương, nhất quán D8) thay vì đọc `teachers.Timezone` cross-feature như `sessions`.
- Quy tắc đếm giả định: mọi `counts` chỉ đếm việc chưa xong; lọc `mine`/`assignee` vẫn trả việc đã xong của người đó (đúng demo report). Cần chủ dự án xác nhận.

## Next steps

- `/ak:cook plans/260917-2142-task-modal-a-board-b/plan.md`, Phase 1 (API) và Phase 2 (kit) song song; Phase 4 nay chờ Phase 1.
- Xác nhận quy tắc đếm `counts` trước khi viết integration test (g) của Phase 1.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
