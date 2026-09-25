---
title: "Menu KHO HỌC LIỆU theo So Lop v5 (web)"
status: completed
created: 2026-09-25
branch: feat/giang-day-menu
---

# Menu KHO HỌC LIỆU

## Outcome
- "Giảng dạy" keeps only Danh sách lớp học, Lớp cần tuyển sinh, Lời mời nhận lớp.
- New group "Kho học liệu" (prototype nav): Lộ trình học (/paths), Khóa học
  (/courses), Kho học liệu (/library), Chuẩn bị tài liệu (/prep — kept by user
  decision). Ngân hàng nội dung / bài tập are reached from the Kho học liệu hub.
- Lộ trình học, Khóa học and Chi tiết khóa học screens follow the prototype
  (`screen 'paths' | 'courses' | 'coursedetail'`) — Kho học liệu hub is already v5.

## Design source
`So Lop - Prototype v5.dc.html` (DesignSync, project 4a7e6c77…) + logic.js.

## Decisions
- Web only: path list embeds stages + courses; CHẶNG column derives from it.
- Status copy: course active "Đang hoạt động", archived "Dừng hoạt động",
  draft "Nháp"; path archived "Ngừng hoạt động".
- Lịch sử thay đổi reads `/audit-logs?entity_type=course&entity_id=…` when the
  viewer holds audit.read.
- Ops tabs without a repo screen show the prototype placeholder copy.

## Acceptance
- [x] Nav + layout tests updated.
- [x] Paths / courses / course detail pages match v5; tests updated.
- [x] typecheck, lint, vitest, isolated e2e pass.
- [x] Deployed with prod compose; /readyz 200 (teka-{api,web}:5d4f3a2-dirty-260925-0151; rollback …-0110).
