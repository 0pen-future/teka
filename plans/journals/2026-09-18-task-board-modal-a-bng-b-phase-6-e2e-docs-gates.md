---
title: "Task board Modal A + Bảng B: Phase 6 e2e, docs, gates"
date: 2026-09-18
summary: "Phase 6 hoàn tất: e2e dispatch (8 case), docs, toàn bộ gate xanh; push+PR để user; phát hiện a11y gap Xong-nhanh bằng bàn phím"
---

# Task board Modal A + Bảng B: Phase 6 e2e, docs, gates

## What happened

Hoàn tất Phase 6 (phase cuối) của plan `260917-2142-task-modal-a-board-b`. Phase 1–5 đã commit từ các phiên trước (`15c97dc`…`0aec7fb`); phiên này khoá chất lượng và bổ sung e2e + docs.

- **e2e mới** `apps/web/e2e/tasks-board-dispatch.spec.ts` (một test, 8 case, `setTimeout 90s`): chip Quá hạn (URL `?filter=overdue` + `today=`, chỉ việc quá hạn, reload giữ chip), chip giáo viên `Thầy Minh 2` → `?assignee=`, chip "Của tôi" hiện việc đã xong của mình, kéo khi đang lọc thả sau hàng xóm hiển thị (D9), Xong nhanh + Hoàn tác, thu gọn cột ghi URL + reload, đổi màu cột re-tint, và board của member không có `tasks.view_all`.
- **e2e bổ sung** `tasks-board.spec.ts`: dirty guard khi Escape ("Bỏ thay đổi?"), Xoá → toast Hoàn tác → khôi phục (`role=status` chứa "Đã khôi phục"), D12 assignee đổi chỉ "Cột" + Lưu → card sang cột (login qua user thứ hai Thầy Minh), và điều hướng bàn phím roving-tabindex + `[`/`]`.
- **Helpers**: `board.ts` thêm `filterChip`/`quickDone`/`waitForBoard`/`findMemberId`/`moveTask`/`deleteTask` và `TaskSeed` (due_on/assignee override); `auth.ts` thêm `loginAsMember`.
- **Docs**: `api-guidelines.md` (query `filter`/`assignee`/`today` + `counts` cho `GET /tasks/board`, `POST /tasks/:id/restore`, `color`), `architecture.md` (kanban lib generic column, ranh giới lọc phía server), `frontend-guidelines.md` ("Board filters are server-side" + `HvModal stickyFooter`). Hai README lib đã cập nhật ở Phase 1/5.

## Gates

Tất cả PASS: `lint-web` (0 error, 6 warning react-hooks có sẵn), `test-web` (864 pass/3 skip), `lint-api`/`test-api-unit`/`api-docs` (không drift swagger), `test-api` (đã xanh cho code đã commit — Phase 1 report 40/40 pkg, coverage 77.3%; không file .go nào đổi trong diff phase này), `e2e-isolated E2E_ARGS=tasks-board` (8/8 spec xanh trên `teka-e2e`, stack dọn sạch).

## Decision

- Hai commit theo phase: `test(e2e): cover dispatch board filters, modal guards, and keyboard nav` và `docs: document server-side board filters, restore, and column color`. Không tham chiếu AI, không phase ID trong message.
- Không chạy lại `test-api` integration: không file `.go` nào thay đổi so với commit Phase 1 đã gate; dựa vào bằng chứng đã ghi + CI (narrowest-useful-test).
- Push + PR để user tự xử lý (chọn "Chưa push gì cả") do auth split: SSH=cesc1802 write, gh=dev-fng pull-only — danh tính PR thuộc quyết định của user.

## a11y gap phát hiện (chưa vá)

Người dùng bàn phím/screen-reader không kích hoạt được checkbox "Xong nhanh" bằng Space/Enter: `TaskCard.onKeyDown` (task-card.tsx) chặn Enter/Space khi event bubble qua `CardControlBarrier` (vốn chỉ chặn click/mousedown/touchstart, không chặn keydown) và mở modal chi tiết thay vì để checkbox chạy Space mặc định. Lỗi có sẵn, ghi nhận để xử lý riêng, không mở rộng scope phase này.

## Next steps

1. User push branch `feat/task-modal-a-board-b` (7 commit) qua SSH và mở PR bằng danh tính mong muốn — nội dung PR + checklist a11y thủ công (375/1280px, Tab order, VoiceOver/NVDA) đã soạn sẵn.
2. Reviewer chạy checklist a11y thủ công.
3. Sau merge: cân nhắc issue riêng cho a11y gap Xong-nhanh bằng bàn phím; `ak plan archive` cho plan.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
