---
title: Sổ lớp mở rộng tại chỗ — Quản lý lớp học theo Phương án C
date: 2026-09-02
summary: Redesign trang /classbook thành bảng ledger mở rộng tại chỗ; sửa 11 finding code review; deploy web image lên homelab
---

# Sổ lớp mở rộng tại chỗ — Quản lý lớp học theo Phương án C

## What happened
- Plan `plans/260902-1523-classbook-ledger-option-c` (5 phase) hoàn tất: `ClassbookPage` bỏ tabs/side panel, chuyển sang bảng 8 cột với hàng mở rộng tại chỗ (`SessionsTable` + `SessionExpandRow`), toolbar chọn lớp + month stepper (`?class_id`, `?month`), dải KPI hairline, guard `UnsavedScoresGuard` cho đổi hàng/lớp/tháng/view.
- Tester xanh ngay lần đầu. Code review trả 1 blocker + 10 should-fix. Blocker: `flush()` của hàng mở rộng bỏ qua điểm chung (lớp không có bộ điểm) nên "Lưu và đóng" trong guard không lưu gì. Sửa bằng `saveGeneralScores()`/`saveNoteNow()` trả `Promise<boolean>` và `flush` gộp cả hai.
- Should-fix đã áp: `dirtyCount` gồm cả nhận xét; cleanup `onDirtyChange(0,0)` khi unmount; Escape trong hàng mở rộng đóng qua guard (bỏ qua dialog portal); focus quay về nút hàng khi đóng; roving tabindex theo hàng focus cuối; chip CHẤM ĐIỂM `none` khi roster chưa tải hoặc 0/0; `ProgressBar` nhận `aria-label` lên `role=progressbar`; hàng hủy hiện chip coral "Buổi hủy" ở GIÁO ÁN; trạng thái "Không tìm thấy lớp" khi `class_id` lạ; đổi tên biến `window` che global.
- Bỏ qua có chủ đích: xoá điểm bằng `score: null` là hành vi có sẵn ngoài scope.

## Gotchas
- Điểm chung lưu qua `PUT /sessions/:id/marks`, điểm thành phần qua `PUT /sessions/:id/scores`; test đếm nhầm endpoint lúc đầu.
- `jsx-a11y/no-noninteractive-element-interactions` báo tại phần tử `<section>`, nên `eslint-disable-next-line` phải đặt ngay trên dòng `<section`, không phải trên prop `onKeyDown`.
- Scout-block hook chặn mọi lệnh Bash chứa chuỗi node_modules; dùng `npx`/`npm run` là đủ.

## Decision
- Không thêm tên giáo viên vào nút chọn lớp (không có dữ liệu, non-goal); icon CSV dùng `arrow-down` vì hv-icon không có `download`.
- Chia 3 commit theo plan (scoring → classbook → plans/reports); commit scoring không tự build được vì file dùng chung chỉ vào commit classbook.

## Next steps
- Deploy homelab: chỉ rebuild `teka-web:local` (API không đổi từ 31/08).
- Cân nhắc sau: làm rõ hành vi xoá điểm `null` trong `ScoreEntryByStudent`.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
