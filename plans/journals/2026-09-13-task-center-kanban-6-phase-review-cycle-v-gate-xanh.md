---
title: "Task-center Kanban: 6 phase, review cycle và gate xanh"
date: 2026-09-13
summary: "Board Kanban theo trung tâm: catalog v4 + migration 000022, core Go/React hard-boundary, feature adapter hai đầu; review 0 blocker đã sửa, 5/5 gate xanh; còn deploy prod"
---

# Task-center Kanban: 6 phase, review cycle và gate xanh

## Chuyện gì đã xảy ra

Triển khai trọn plan `plans/260913-1102-task-center-kanban/` ở chế độ auto:
6 phase (catalog + migration, core Go `pkg/kanban`, feature adapter API,
lib headless `src/lib/kanban`, feature web `features/tasks`, ma trận + e2e +
docs). Sau đó chạy vòng review: 0 blocker / 5 major / 14 minor, sửa toàn bộ
major và 13/14 minor (một minor chỉ ghi nhận), retest 5/5 gate.

Lỗi thật gặp trên đường:

- E2E lần đầu fail vì `task_columns` trống: seeder ghi center thẳng vào DB,
  bỏ qua `CreateCenter` nên không có 3 cột mặc định. Sửa bằng cách export
  `centers.DefaultColumns` và cho seeder insert cùng bất biến.
- Toast thành công lặp lại tiêu đề task nên assertion "task biến mất" phải
  scope vào listbox cột thay vì `getByText` toàn trang.
- Review phát hiện migration thiếu CHECK cho `priority` và FK `assignee_id`
  dùng CASCADE (xoá nhầm cả task khi người được gán rời trung tâm) → chuyển
  sang `ON DELETE SET NULL (assignee_id)`; xác minh sống trên stack e2e.
- Map lỗi: sentinel con `ErrEmptyName/ErrEmptyTitle/ErrMoveToSelf` giờ trả
  422 kèm `fields` thay vì 400 chung; `GET /tasks/:id` 403 trong tenant,
  404 khác tenant (contract cập nhật).
- Gate `test-api` "fail do hạ tầng" hoá ra do tester spawn 3 bản
  `go test -tags=integration` chạy trùng, cùng ghi `coverage.out`. Dừng hết,
  chạy đúng một bản `-p 1`: 40/40 package OK, coverage 77.1%.

## Quyết định

- Hai lib hard-boundary giữ đúng ràng buộc import (test `go list -deps`,
  ESLint `no-restricted-imports`); feature chỉ là adapter.
- Cache TanStack Query là nguồn sự thật duy nhất cho board; lib headless
  không giữ state server.
- Ghi vào memory dự án: chỉ chạy một bản test tích hợp API tại một thời điểm.

## Bước tiếp theo

- Deploy prod theo `docs/deployment.md`: snapshot image, backup DB, build,
  compose `-p teka` up (migration 000022 chạy qua service `migrate`), verify
  `readyz`/web, rồi mới cấp khoá opt-in `tasks.manage_board`/`tasks.view_all`/
  `members.list` qua ma trận.
- Đóng tiêu chí "Prod: tài khoản có khoá thao tác được, không có khoá nhận 403".

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
