---
title: "Phase 1 kéo-thả: API move có after_task_id, renormalize và khoá cột"
date: 2026-09-17
summary: "Mở rộng POST /tasks/:id/move nhận after_task_id (midpoint + renormalize khi gap/2 < 1e-6) trong một tx với pg_advisory_xact_lock theo cột; reviewer tìm ra nhánh top thiếu khoá gây lost update, đã sửa."
---

# Phase 1 kéo-thả: API move có after_task_id, renormalize và khoá cột

## Bối cảnh

Phase 1 của plan `plans/260917-1515-task-dnd-rich-text` (kéo-thả + rich text),
chạy qua `/ak:cook` code mode. Mục tiêu: `POST /tasks/:id/move` nhận
`after_task_id` (Optional, nullable) để sắp thứ tự trong cột mà vẫn tương thích
body v1 `{ column_id }` (lên đầu cột).

## Đã làm

- `pkg/kanban`: `MoveTask(..., after *TaskID)` chạy trong `uow.Within`;
  `positionAfter` thuần (midpoint với successor, bỏ qua chính việc đang di
  chuyển; anchor cuối → +1); `minPositionGap = 1e-6`, renormalize khi
  `gap/2 < minPositionGap` để mọi cặp kề luôn cách ≥ 1e-6. Port mới
  `ListColumnPositions` + `RenormalizeColumn`; `ErrInvalidAfterTask` wrap
  `ErrInvalidInput`.
- Feature `tasks`: `MoveTaskRequest.AfterTaskID Optional[uuid.UUID]`; anchor sai
  → 422 `fields.after_task_id`; chuỗi không UUID → 400 (BindError sẵn có);
  `Board` dùng `sort.SliceStable`. Repo Postgres: `ListColumnPositions` lấy
  `pg_advisory_xact_lock(hashtext(tenant:col))` trước khi select.
- Swagger sinh lại bằng `make api-docs`; README `pkg/kanban` cập nhật "Position
  strategy" + bảng use-case.
- Test: bảng `positionAfter` 9 case, unit core/feature, 7 integration test mới
  (`move_integration_test.go`) gồm 45 lần chèn cùng khe (renormalize nhiều lần),
  2 goroutine cùng anchor, tenant khác, top-move đan xen renormalize.

## Vấn đề gặp phải

- `make test-api` chạy song song nhiều gói → timeout tạo Docker systemd scope
  (tranh chấp, không phải lỗi code). Chạy lại `-p 1`: 40/40 gói ok, coverage
  77.2%.
- Reviewer H-1 (đúng): nhánh `after == nil` return sớm qua
  `MinPositionInColumn`, không giữ khoá cột. Tx renormalize (ghi 0..n-1 theo
  order đã chụp) có thể ghi đè position min−1 của top-move vừa commit → thao
  tác kéo lên đầu bị âm thầm hoàn tác. Sửa: mọi placement đều đi qua
  `ListColumnPositions` (`topOf(rows)`); thêm test unit đếm số lần list và test
  tích hợp `TestConcurrentTopMoveSurvivesRenormalization`.
- Reviewer M-1: `RenormalizeColumn` trả `ErrTaskNotFound` khi `RowsAffected == 0`
  nhưng DeleteTask không lấy khoá → soft-delete xen giữa làm move hợp lệ nhận
  404. Sửa: bỏ qua dòng không còn (đã xoá thì không cần position).
- `ak plan phase close` / `ak plan update` báo "plan not found" dù `ak plan show`
  thấy plan; sync trạng thái bằng tay (frontmatter + bảng phases) rồi
  `ak plan reindex --yes` + `ak plan check` cho checkbox.

## Quyết định

- Không tách endpoint riêng cho "đặt cuối cột"; cột chạm cap 50 việc thả dưới
  thẻ 50 sẽ nằm trên phần ẩn — ghi vào Risk phase 4.
- Giữ ngữ nghĩa v1 cho top-move (min−1 tính cả chính việc đang di chuyển).

## Bước tiếp

- Commit Phase 1 (API + docs + plan + reports) qua git-manager, không tham
  chiếu AI.
- Phase 2 (rich text API: bluemonday, migration 000023) — **backup DB prod
  trước** khi chạy migration. Phase 3–5 đã phản ánh red-team web H1–H4/M1–M8.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
