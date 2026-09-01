---
title: "Lớp & học sinh chuyển sang Trung tâm, owner-only, 3 tab"
date: 2026-09-01
summary: "Hoàn tất plan class-students-center-tabs: nav sang Trung tâm, guard owner-only, 3 tab, quét điểm chạm member, viết lại 3 e2e spec; phát hiện CI e2e master đỏ từ PR #39"
---

# Lớp & học sinh chuyển sang Trung tâm, owner-only, 3 tab

## What happened

Thực thi trọn 4 phase của `plans/260901-2035-class-students-center-tabs/` trên
nhánh `teka/260901-2035`: trang `/students` chuyển vào nhóm nav "Trung tâm",
owner-only (shell guard theo pattern `center-permissions-page`, non-owner
redirect `/` với zero request roster), tái cấu trúc 3 tab Lớp học | Học sinh |
Chưa ghi danh (state `?tab=`, suy tab từ `class_id` legacy), quét sạch điểm
chạm member (ẩn card "Lớp mới", copy trang nhập Excel, back-link cài đặt lớp
theo vai trò), viết lại 3 e2e spec với assert member chuyển sang `/records`.
API zero-diff, `CATALOG_VERSION` giữ 3.

## Decision

- User chốt giữa phase 4: **chấp nhận bỏ workflow UI gửi báo cáo class-scoped
  của học vụ** — nút "Gửi báo cáo" trên roster là entry duy nhất; xoá
  `ClassSendPeriodsDialog` (dead code), trim biến thể `classId` của
  `useReportPeriods`/`listReportPeriods`, bỏ e2e test tương ứng. API vẫn cho
  phép, chỉ cắt đường UI.
- Review gate: sửa ngay finding Medium — tab "Lớp học" hiển thị lỗi tải
  `/classes` như danh sách rỗng; giờ render "Không tải được danh sách lớp"
  (pattern `class-overview-cards`) kèm unit test. Hai finding Low ghi nhận
  non-issue có chủ đích trong phase 4.

## Verification

Typecheck sạch; vitest 68 files 480 pass/3 skip; lint 0 lỗi; e2e trên stack
`teka-e2e` seed tươi: 3 spec viết lại xanh. 5 spec fail (billing, collections,
3 statement) được chứng minh là breakage CÓ SẴN trên master — Web CI e2e của
master@2efc779 (base nhánh) fail đúng bộ đó cộng test hoc_vu send cũ; CI đỏ
từ PR #39 (b915a50, 2026-08-31), lần xanh cuối a0704ed (2026-08-30).

## Next steps

- Bugfix riêng cho e2e master (billing/collections/statement fail trên seed
  tươi từ PR #39) — ngoài scope plan này.
- Follow-up đã ghi ở Non-goals: cân nhắc extract `HvTabs` khi có usage thứ 3.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
