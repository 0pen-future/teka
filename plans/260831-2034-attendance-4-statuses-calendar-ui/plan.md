---
title: "Điểm danh 4 trạng thái + chọn buổi/lịch + UI responsive"
description: "Mở rộng điểm danh từ present/absent sang 4 trạng thái (đúng giờ, muộn, vắng, có lý do), đổi contract API confirm, thêm số liệu điểm danh vào danh sách buổi, và dựng lại UI điểm danh + chọn buổi theo artifact đã chốt."
status: completed
priority: P1
effort: "4-5d"
tags: [api, web, db, attendance, sessions]
created: 2026-08-31
blockedBy: []
blocks: []
---

# Điểm danh 4 trạng thái + chọn buổi/lịch + UI responsive

## Overview

Triển khai theo design canvas đã chốt (artifact "Điểm danh 4 trạng thái",
https://claude.ai/code/artifact/4de5a29b-5df9-4ad8-9d05-b9e3dfdff169 — phương án A):

- **DB**: `attendance_records.status` hiện CHECK `present|absent|excused` → thêm `late`
  (mapping UI: present=Đúng giờ, late=Muộn, absent=Vắng, excused=Có lý do).
- **API**: `POST /sessions/:id/attendance` hiện nhận `absent_student_ids` →
  nhận danh sách `{student_id, status, note?}` (học sinh không liệt kê mặc định
  `present`); danh sách buổi trả thêm số liệu đếm theo trạng thái.
- **UI**: bảng điểm danh dạng table với 4 cột trạng thái (nút tròn 1 chạm,
  mặc định cả lớp Đúng giờ → buổi bình thường vẫn 1 chạm Xác nhận); bộ chọn
  buổi 3 thẻ TRƯỚC · HÔM NAY · KẾ TIẾP + mũi tên ‹ ›, "Mở lịch tháng" là lối
  tắt phụ; responsive mobile (390px) và desktop hai cột (1440px).

**Quyết định billing (giữ nguyên hành vi):** cả 4 trạng thái đều `billable=true`
như V1 hiện tại — `late` được coi là có mặt và tính tiền bình thường (đúng ghi
chú trong artifact); `excused` tiếp tục tính tiền như hiện nay. Thay đổi
billability của `excused` là quyết định sản phẩm riêng, nằm ngoài scope
(xem Open questions). Nhờ đó `TallyByEnrollment` và toàn bộ billing/chốt sổ
không đổi hành vi.

**Không cần endpoint lịch mới:** `GET /classes/:id/sessions?from&to` đã
materialize buổi từ `class_schedules` (generator `Expand()`); UI prev/today/next
và lịch tháng chỉ cần gọi endpoint này với cửa sổ ngày phù hợp.

## Non-goals

- Không đổi công thức billing, kỳ chốt sổ, hay logic `invoice_adjustments`.
- Không thêm endpoint calendar riêng; không đổi cơ chế sinh buổi từ schedule.
- Không đổi permission keys/catalog RBAC của các endpoint attendance/sessions
  (plan `260829-1640-gh-260829-flexible-center-rbac` đang in-progress — chỉ giữ
  nguyên middleware hiện có, phối hợp khi merge).
- Không làm màn hình thống kê chuyên cần/báo cáo mới.

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | DB + API attendance hỗ trợ 4 trạng thái, contract confirm mới có tương thích ngược | P1 |
| 2 | Danh sách buổi trả số liệu điểm danh (badge "21 đúng giờ · 1 muộn · 2 vắng") | P1 |
| 3 | UI bảng điểm danh 4 cột theo phương án A, responsive mobile + desktop | P1 |
| 4 | Bộ chọn buổi 3 thẻ + mũi tên + lịch tháng phụ | P1 |
| 5 | E2E + regen swagger + cập nhật docs | P2 |

## Phases

| # | Phase | Status |
|---|-------|--------|
| 1 | [Phase 1: DB migration + attendance API 4 trạng thái](./phase-01-db-attendance-api.md) | Completed |
| 2 | [Phase 2: Số liệu điểm danh trong sessions API](./phase-02-sessions-summary-counts.md) | Completed |
| 3 | [Phase 3: Web — bảng điểm danh 4 cột](./phase-03-web-attendance-table.md) | Completed |
| 4 | [Phase 4: Web — bộ chọn buổi + lịch tháng](./phase-04-web-session-picker-calendar.md) | Completed |
| 5 | [Phase 5: E2E, docs, verification](./phase-05-e2e-docs-verification.md) | Completed |

Dependencies: 2 phụ thuộc 1; 3 phụ thuộc 1; 4 phụ thuộc 2; 5 phụ thuộc 3+4.
Phase 1+2 (backend) có thể chạy song song với việc chuẩn bị UI, nhưng phase 3
chỉ merge sau khi contract phase 1 chốt.

## Success Criteria

- [x] Migration `000021` thêm `late` vào CHECK của `attendance_records.status`; `make test-api` xanh.
- [x] `POST /sessions/:id/attendance` nhận `marks: [{student_id, status, note?}]`, mặc định present cho học sinh không liệt kê; vẫn chấp nhận `absent_student_ids` (deprecated) trong giai đoạn chuyển tiếp.
- [x] `GET /sessions/:id/attendance` trả đủ 4 trạng thái + note từng học sinh.
- [x] Session DTO (list/detail/pending) có `attendance_summary {present, late, absent, excused}` (null khi chưa điểm danh); billing output không đổi (test chứng minh).
- [x] UI mobile 390px: bảng 4 cột nút tròn, chip đếm trạng thái, hàng nhuộm màu theo trạng thái, confirm bar "XÁC NHẬN · n VẮNG · n MUỘN"; desktop 1440px: hai cột với 3 thẻ buổi dọc.
- [x] Chọn buổi: 3 thẻ TRƯỚC/HÔM NAY/KẾ TIẾP + mũi tên ‹ › + modal lịch tháng; buổi chưa điểm danh nhuộm coral.
- [x] Các state buổi giữ nguyên hành vi: đã xác nhận (mở lại sửa), đã huỷ, kỳ chốt sổ (LƯU VÀ TẠO ĐIỀU CHỈNH).
- [x] `npm run typecheck`, `npm run test`, `make test-api` xanh; e2e attendance cập nhật và pass.

## Open questions

1. **"Muộn" có ảnh hưởng tính tiền/báo cáo riêng không?** Plan này coi muộn =
   có mặt, tính tiền bình thường (theo artifact). Nếu sau này cần khác, chỉ đổi
   ở tầng billing, không đổi schema.
2. **"Vắng có lý do" (excused) có tính tiền không?** Plan này giữ nguyên V1
   (billable=true). Nếu chốt "không tính tiền", cần một thay đổi billing riêng
   (billable=false cho excused + reconciliation cho kỳ đã chốt) — ngoài scope.
3. **Thời gian giữ tương thích `absent_student_ids`:** đề xuất giữ 1 release
   rồi gỡ (client web là consumer duy nhất).

<!-- slug: attendance-4-statuses-calendar-ui -->
