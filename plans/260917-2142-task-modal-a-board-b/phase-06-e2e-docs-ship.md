---
phase: 6
title: "E2E, a11y, docs, gates và ship"
status: done
priority: P1
effort: "1d"
dependencies: [3, 4, 5]
---

# Phase 6: E2E, a11y, docs, gates và ship

## Context Links

- `apps/web/e2e/tasks-board.spec.ts`, `tasks-board-dnd.spec.ts`, `tasks-board-mobile.spec.ts`, `tasks-board-rich-text.spec.ts`; helpers `e2e/helpers/{board,drag,auth}.ts`
- `make e2e-isolated E2E_ARGS="tasks-board"` (compose project `teka-e2e`, seed mới)
- Docs: `docs/frontend-guidelines.md` (mục Drag-and-drop, Rich text ~dòng 122–145; mục e2e ~dòng 187), `docs/api-guidelines.md`, `docs/architecture.md`, `apps/api/pkg/kanban/README.md`, `apps/web/src/lib/kanban/README.md`
- Plan index: [plan.md](./plan.md)
- Quy tắc commit: conventional, không tham chiếu AI; branch `feat/task-modal-a-board-b`
- Quyết định D3, D11, D12 trong [plan.md](./plan.md) (lọc server, không segmented, assignee đổi Cột trong modal)

## Overview

Khoá chất lượng toàn plan: e2e cho luồng mới, kiểm a11y có chủ đích, cập nhật
docs đúng "bề mặt sở hữu nhỏ nhất", chạy đủ gate, chia commit theo phase.

## Key Insights

- Seed e2e tạo center mới → cột mặc định đã có `color` (Phase 1) → không cần
  seed thêm cho tint.
- e2e cần user có `tasks.view_all` để thấy dải lọc: dùng vai owner/manager
  trong `helpers/auth.ts` (kiểm vai nào có quyền; `docs/adding-permissions.md`).
- Lọc là request thật (D3) → e2e có thể khẳng định bằng URL (`?filter=overdue`)
  và bằng `page.waitForResponse(/\/tasks\/board\?.*filter=overdue/)` thay vì chỉ
  đếm card; số trên chip đến từ `counts` nên seed > 50 việc một cột là case đáng
  có nếu seed cho phép (tạo qua API trong `beforeAll`, không bắt buộc).
- Playwright `toHaveCSS("background-color", ...)` đủ để kiểm tint; contrast đã
  đo bằng script ở Phase 5, e2e không cần axe (không thêm dependency).

## Requirements

- e2e mới `tasks-board-dispatch.spec.ts`: (1) chip "Quá hạn" → URL `?filter=overdue`, request board có `filter=overdue&today=`, chỉ còn việc quá hạn; reload giữ chip bật; (2) chip giáo viên `Tên · N` khớp số việc chưa xong đã seed, bấm → `?assignee=`; (3) chip "Của tôi" hiện cả việc đã xong của mình ở cột Hoàn thành; (4) kéo card khi đang lọc → thả sau card hiển thị liền trước, tắt lọc vẫn đúng thứ tự tương đối; (5) checkbox Xong → cột Hoàn thành → Hoàn tác trả về; (6) thu gọn cột → URL `collapsed=` → reload vẫn thu gọn; (7) đổi màu cột trong Cấu hình → nền đổi; (8) đăng nhập vai không có `view_all` → không có nhóm "Bộ lọc nhanh", phụ đề Bảng A.
- e2e sửa `tasks-board.spec.ts`: modal — sửa tiêu đề rồi Escape → dialog "Bỏ thay đổi?"; Xoá → toast Hoàn tác → việc quay lại; (D12) đăng nhập vai assignee (không phải creator) mở việc → chỉ Cột enable, đổi Cột + Lưu → card sang cột mới (cần seed 2 user: creator giao cho assignee — kiểm `helpers/auth.ts`).
- Docs: `frontend-guidelines.md` thêm đoạn "Board filters are server-side" (`filter`/`assignee`/`today` là query param, `counts` từ server, URL state, board trong cache đã lọc nên không ánh xạ index, quick-done qua move) và "HvModal stickyFooter"; `api-guidelines.md` thêm query `filter|assignee|today` + `counts` cho `GET /tasks/board`, `POST /tasks/:id/restore`, `color` vào bảng endpoint/DTO; `architecture.md` cập nhật sơ đồ kanban (Column.color opaque, `BoardFilter`/`CountBoard`, RestoreTask); hai README lib cập nhật port/type mới.
- Gates đầy đủ; commit theo phase; PR mô tả + checklist thủ công (375px/1280px, bàn phím, VoiceOver/NVDA một lượt).

## Related Code Files

- `apps/web/e2e/tasks-board-dispatch.spec.ts` (mới), `apps/web/e2e/tasks-board.spec.ts`, `apps/web/e2e/helpers/board.ts` (thêm `filterChip(page, name)`, `quickDone(card)`, `waitForBoard(page, { filter?, assignee? })`)
- `docs/frontend-guidelines.md`, `docs/api-guidelines.md`, `docs/architecture.md`, `apps/api/pkg/kanban/README.md`, `apps/web/src/lib/kanban/README.md`

## Implementation Steps

1. Viết `tasks-board-dispatch.spec.ts` dùng `createTasks` (helper) với `due_on` hôm qua/hôm nay/không, assignee khác nhau, một việc đã xong (cần ≥2 giáo viên trong seed — kiểm `helpers/auth.ts`; nếu chỉ có 1, tạo thêm qua API invite trong `beforeAll` hoặc giảm case (2) về 1 giáo viên). `today` trong assertion request = ngày lịch của trình duyệt test (Playwright chạy cùng máy → `new Date()` trong spec).
2. Sửa `tasks-board.spec.ts`: dirty guard + Hoàn tác sau xoá (đợi `getByRole("status")` chứa "Đã khôi phục"); case assignee đổi Cột (D12) — nếu seed không có user thứ hai đăng nhập được, ghi rõ trong PR và giữ unit test (h) của Phase 3 làm bằng chứng.
3. Chạy `make e2e-isolated E2E_ARGS="tasks-board"`; sửa flake bằng `expect.poll`/`toBeVisible`, không `waitForTimeout`.
4. Kiểm a11y thủ công theo checklist: Tab qua card → ⋯ → checkbox Xong → chip lọc; `[`/`]` với cột thu gọn; SR đọc "Đánh dấu xong X", "Thao tác", chip "Quá hạn, đã nhấn".
5. Docs như Requirements; xác minh mỗi câu có nguồn (file/test).
6. Gates: `make lint-api test-api-unit api-docs` → `make test-api` (riêng) → `make lint-web test-web` → e2e.
7. Commit theo phase (ví dụ `feat(api): column color and task restore`, `feat(web): sticky modal footer and chips in hv kit`, `feat(web): compact task modal with undo delete`, `feat(web): board card and column fixes`, `feat(web): dispatch board filters, quick done, column tint`, `test(e2e): cover dispatch board`, `docs: task board dispatch and restore contracts`). Không tham chiếu AI trong message.
8. Mở PR (`gh pr create`) với tóm tắt + checklist thủ công; đợi CI.

## Todo

- [x] e2e dispatch spec (8 case) + sửa tasks-board.spec (dirty guard, Hoàn tác, assignee đổi Cột, điều hướng bàn phím)
- [x] e2e xanh trên stack isolated (8/8 spec `tasks-board*` PASS trên `teka-e2e`)
- [~] a11y checklist thủ công — phần tự động hoá được đã có e2e (roving tabindex, `[`/`]`, live region); VoiceOver/NVDA + 375/1280px còn chờ reviewer (checklist trong PR). Phát hiện a11y gap có sẵn: không "Xong nhanh" được bằng bàn phím (`TaskCard.onKeyDown` chặn Space/Enter) — nêu trong PR, không vá ở phase này.
- [x] Docs 5 file cập nhật (frontend-guidelines, api-guidelines, architecture sửa lần này; 2 README lib đã commit ở Phase 1/5), link kiểm
- [x] Toàn bộ gate xanh (lint-api, test-api-unit, api-docs, test-api, lint-web, test-web, e2e-isolated)
- [~] Commit theo phase (2 commit test+docs đã tạo, qua lefthook) — push + PR user tự xử lý (auth split, đã hỏi 2026-09-18)

## Success Criteria

- 5 spec `tasks-board*` xanh trên `teka-e2e`.
- Docs không mô tả hành vi chưa có; mỗi endpoint/DTO mới (`filter`/`assignee`/`today`, `counts`, restore, color) có mặt trong `api-guidelines.md`; không còn câu nào nói "lọc phía client"/segmented phạm vi.
- PR xanh CI; không AI reference trong lịch sử commit của branch.

## Risk Assessment

- **Seed thiếu giáo viên thứ hai** → bước 1 có fallback; case D12 cũng cần user thứ hai đăng nhập được.
- **Ngày lịch e2e khác server** (stack chạy UTC, trình duyệt giờ máy) → assertion dựa vào `today` client gửi, seed `due_on` tính từ cùng `new Date()` của spec.
- **Toast 6s trong e2e** → click Hoàn tác ngay sau khi toast hiện; nếu flake, tăng duration chỉ trong test qua `data-testid`? Không — dùng `getByRole("button", { name: "Hoàn tác" })` với timeout mặc định 5s là đủ.

## Security Considerations

- Không mô tả nội bộ quyền trong docs công khai ngoài những gì đã có trong `adding-permissions.md`.

## Next Steps

Merge → `ak plan archive` sau khi ship; ghi journal.
