---
title: "Lập kế hoạch Kanban trung tâm công việc (lib trong app, hard mode)"
date: 2026-09-13
summary: "Plan 260913-1102-task-center-kanban: 6 phase, core kanban Go/React tách khỏi Teka bằng ports, red-team 4 lens áp 15 finding, members.list opt-in"
---

# Lập kế hoạch Kanban trung tâm công việc (lib trong app, hard mode)

## What happened
- Lập kế hoạch từ brief brainstorm rev3 (`plans/reports/brainstorm-260913-1725-task-center-kanban.html`) với hai yêu cầu thêm: SOLID + design pattern, và thiết kế như lib tái dùng được cho dự án khác.
- Chạy hard mode: 2 scout (api/web), 2 researcher (Go kanban lib patterns, React headless kanban), planner, 4 red-team lens (security, failure, assumption, scope) → 32 finding thô, gộp còn 16, áp 15, 1 quyết định user.
- Deliverable: `plans/260913-1102-task-center-kanban/plan.md` + 6 phase file. `ak plan validate` OK, `ak plan status` đọc 6 phase / 64 task.

## Decision
- Kiến trúc "lib trong app, biên giới cứng": `apps/api/pkg/kanban` (stdlib + uuid) và `apps/web/src/lib/kanban` (chỉ react). Không go.mod/package.json riêng. Biên giới enforce bằng test `go list -deps` và ESLint `no-restricted-imports`.
- Luật read nằm ở `DefaultPolicy` trong core (trả `Visibility`), repository nhận `Visibility` chứ không nhận `authctx.Scope`. Lệch có chủ đích so với prose "quy tắc scope trong repository" của brief; ghi ở D3.
- Core chỉ publish event khi tự mở `uow.Within` ngoài cùng (v1: `ColumnDeleted`). Bàn giao khi rời trung tâm do `centers.RemoveMember` publish sau commit; audit qua bus, không ghi trực tiếp.
- Literal tên cột mặc định tiếng Việt nằm ở `centers/default_columns.go`, core chỉ có `DefaultColumns(specs)`; `container.go` và `NewRouter` không đổi, wiring `TaskHandover` ở `registerFeatures` nil-safe.
- Catalog: 8 khoá mới, 5 CRUD backfill cho 3 vai trò và stint không role; `tasks.manage_board`, `tasks.view_all`, `members.list` opt-in (user chọn 13/09/2026). `DefaultRoleKeys` 53 → 58.
- Web: một nguồn sự thật trong cache TanStack, reducer là helper thuần cho `setQueryData`, lỗi thì invalidate prefix boards, 9 mutation dùng chung `scope`.
- Rollback tách 2 chế độ: trước ship `migrate-down` xoá 8 khoá; sau ship không drop bảng, gỡ route + revert `CatalogVersion`.

## Gotchas
- Hook `set-active-plan.cjs` fail (thiếu module) → báo, không sửa; dùng `ak plan use` thay thế.
- `ak plan validate`/`status` cần path `plans/<id>`, id trần không resolve khi CWD là repo root.
- Brief đếm sai số khoá ở AC8; plan chốt 8/5/3.
- scopelint R1 không bắn cho repo tasks (không nhận `Scope`) → thay bằng integration test 2 trung tâm.

## Next steps
- `/ak:cook` theo thứ tự phase 1 → 6; phase 1 cần backup DB trước migration.
- Khi bàn giao UX: "Công việc" nằm trong sheet "Thêm" trên mobile; select "Giao cho" trống cho đến khi owner tick `members.list`.
- AgentWiki publish skipped.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
