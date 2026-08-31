---
phase: 2
title: "Số liệu điểm danh trong sessions API"
status: completed
priority: P1
effort: "0.5d"
dependencies: [1]
---

# Phase 2: Số liệu điểm danh trong sessions API

## Overview

Thêm `attendance_summary` (đếm theo 4 trạng thái) vào response của danh sách
buổi, chi tiết buổi và danh sách pending — phục vụ badge
"21 đúng giờ · 1 muộn · 2 vắng" và tô màu thẻ buổi trong UI chọn buổi.

## Requirements

- Functional:
  - [x] Session DTO thêm `attendance_summary: {present, late, absent, excused} | null` — null khi `attendance_confirmed_at` IS NULL.
  - [x] Áp dụng cho `GET /classes/:id/sessions`, `GET /sessions/:id`, `GET /sessions/pending`.
  - [x] Chỉ đếm record `deleted_at IS NULL`.
- Non-functional:
  - [x] Một query gộp (JOIN + FILTER hoặc LATERAL), không N+1 theo buổi.
  - [x] Kiểm tra giới hạn from/to của list endpoint: UI lịch tháng cần cửa sổ ≥ 62 ngày (nếu service đang cap thấp hơn, nới cap và ghi rõ trong swag).

## Architecture

- Repository sessions: mở rộng SELECT với aggregate
  `COUNT(*) FILTER (WHERE ar.status = 'present')` … per status, LEFT JOIN
  `attendance_records ar ON ar.session_id = s.id AND ar.deleted_at IS NULL`,
  GROUP BY session. Buổi chưa confirm không có record → service map thành null
  (phân biệt với confirm 0 học sinh nhờ `attendance_confirmed_at`).
- `student_count` giữ nguyên nghĩa (sĩ số roster) — không đổi field cũ.

## Related Code Files

- Modify: `apps/api/internal/features/sessions/dto.go` (thêm `attendance_summary`)
- Modify: `apps/api/internal/features/sessions/repository.go` (aggregate query cho list/detail/pending)
- Modify: `apps/api/internal/features/sessions/service.go`, `handler.go` (map + swag)
- Modify: `apps/api/internal/features/sessions/*_test.go`, `*_integration_test.go`

## Implementation Steps

1. Viết aggregate trong repository (list, detail, pending dùng chung helper).
2. Map sang DTO trong service; null khi chưa confirm.
3. Regen swagger (`make api-docs`).
4. Tests: buổi chưa confirm → null; buổi confirm đủ 4 trạng thái → đếm đúng; record soft-deleted không được đếm; không N+1 (assert số query nếu testutil hỗ trợ, hoặc dùng 1 query gộp có test tích hợp).

## Success Criteria

- [x] `make test-api` xanh; response mẫu trong swagger có `attendance_summary`.
- [x] List 60+ buổi trả về trong một round-trip DB cho phần summary.

## Risk Assessment

- **Hiệu năng list dài** — aggregate trên index `(session_id)` của
  attendance_records là đủ ở quy mô lớp học (≤ vài chục học sinh × vài chục
  buổi); nếu chậm, thêm partial index theo `session_id WHERE deleted_at IS NULL`
  (đã có unique partial index tương tự).
- **Nhầm nghĩa `student_count`** — giữ nguyên field, chỉ thêm field mới; UI đổi
  nguồn dữ liệu cho badge ở Phase 4.
