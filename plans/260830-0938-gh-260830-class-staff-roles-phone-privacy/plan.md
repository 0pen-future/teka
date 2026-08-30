---
title: "Class staff roles + phone privacy"
description: "Bảng class_staff chung cho GV/học vụ/trợ giảng với capability map code-owned; scoping đọc/ghi theo assignment (soft-close giữ quyền đọc lịch sử); SĐT liên hệ chỉ owner + học vụ được gán; contact/student CRUD owner-only + migration anchor về owner"
status: pending
priority: P1
effort: "7d"
tags: [api, web, db, security, authz, migration]
created: 2026-08-30
blockedBy: []
blocks: []
---

# Class staff roles + phone privacy

Contract source: [brainstorm report](../reports/brainstorm-260830-0825-GH-260830-class-staff-roles-phone-privacy.md)
(4 rounds decisions, complete 2026-08-30). Decision log R1–R4 trong report là
user decisions — không đảo ngược trong plan này.

## Overview

Mỗi lớp có staff theo role (`giao_vien`, `hoc_vu`, `tro_giang`, mở rộng bằng
code) gán qua bảng chung `class_staff` (soft-close `ended_at`). Đọc dữ liệu lớp
đi theo MỌI assignment (kể cả đã đóng — lịch sử); ghi đòi assignment ACTIVE +
role nằm trong capability map code-owned. SĐT người liên hệ thành dữ liệu bảo
mật của trung tâm: chỉ owner, caller `ReportsOversight()` (thư ký gửi báo cáo)
và học vụ được gán vào lớp liên quan thấy. Owner sở hữu toàn bộ dữ liệu gốc:
contact + student CRUD owner-only, migration chuyển anchor `teacher_id` của
contacts và students hiện có về owner của center.

## Cross-plan coordination

- `260830-0714-GH-260830-handoff-roster-visibility` (done): phần widening đọc
  roster GIỮ; riêng việc GV thấy `contact_phone` bị plan này ĐẢO ở Phase 3
  (mask). `docs/api-guidelines.md` đoạn "Class-teacher roster reads" sẽ viết lại.
- `260829-1640-gh-260829-flexible-center-rbac` phase-04 cleanup (pending, soak-
  gated): file phase đó ghi migration `000014_drop_can_send_reports` nhưng
  `000014_grading` đã chiếm số — cleanup đó phải tự renumber (≥ kế tiếp còn
  trống lúc chạy). Plan này lấy `000015_class_staff` (P1) và
  `000016_owner_data_anchor` (P3). Không block nhau.
- `260829-1020-secretary-report-sender` (done): `reports.send` center-wide GIỮ
  NGUYÊN, cộng gộp với quyền học vụ per-class (quyết định D1 dưới).

## Plan-level decisions

- **D1 — reports.send vs học vụ**: cộng gộp, không thay thế. Thư ký
  (`ReportsOversight()`) giữ đọc/gửi center-wide + thấy phone để address send.
  Học vụ được gán chỉ đọc/gửi statement của contact có học sinh ghi danh active
  trong lớp gán. Statement là đơn vị theo gia đình (contact×period) — học vụ
  gán vào 1 lớp của gia đình đó thấy nguyên statement (gồm phí lớp khác).
- **D2 — giao_vien chỉ đổi qua handoff** trong suốt cửa sổ dual-write (P1→P5):
  staff API từ chối gán/gỡ `giao_vien` (409 trỏ sang handoff). DB enforce
  1 GV active/lớp bằng partial unique index — giữ bất biến
  `classes.teacher_id` = assignment `giao_vien` active duy nhất.
- **D3 — `classes.teacher_id` sống sót**: sau P5 cột vẫn là con trỏ GV chính
  (denormalized, handoff sync); chỉ các đường SCOPING theo nó bị gỡ.
- **D4 — vocabulary role_key**: cùng chuỗi với `center_roles.key` nhưng là trục
  độc lập (role trung tâm ≠ vai trong lớp); validate bằng code trong
  `authctx`, không FK sang `center_roles`, không CHECK cứng.
- **D5 — migration anchor có đường lùi**: `000016` lưu mapping cũ vào bảng
  audit `owner_anchor_backfill` để down khôi phục được (creator gốc không mất).

## Phases

| # | Phase | Status | Depends |
|---|-------|--------|---------|
| 1 | [Schema `class_staff` + quản lý staff + handoff dual-write](./phase-01-class-staff-schema-and-management.md) | Pending | — |
| 2 | [Read scoping theo assignment](./phase-02-read-scoping-by-assignment.md) | Pending | 1 |
| 3 | [Phone privacy + data ownership](./phase-03-phone-privacy-and-data-ownership.md) | Pending | 2 |
| 4 | [Writes theo capability map](./phase-04-capability-map-writes.md) | Pending | 2 (3 cho phần phone của học vụ send) |
| 5 | [Cleanup scoping cũ](./phase-05-cleanup-legacy-scoping.md) | Pending | 1–4 deployed + soak |

Mỗi phase shippable riêng; P3 và P4 có thể đảo thứ tự nội bộ một phần nhưng
acceptance của P4 (học vụ send) cần mask của P3 đã có mặt.

## Acceptance criteria (từ contract, observable)

1. Owner gán/gỡ staff per class qua API + UI; `role_key` ngoài danh mục code →
   422; lớp thiếu role chỉ cảnh báo mềm trên UI.
2. Handoff = đóng assignment `giao_vien` cũ + mở mới (owner-only, cùng center,
   1 tx, dual-write `classes.teacher_id`); GV mới ghi được điểm danh/điểm/nhận
   xét/giáo án/ghi danh; GV cũ chỉ còn ĐỌC lịch sử lớp (mọi write → 403/404).
3. GV & trợ giảng: mọi response + UI roster/attendance KHÔNG chứa
   `contact_phone`; học vụ được gán + owner thấy đủ. Integration test per role.
4. Contact + student create/update/delete: member → 403; owner → OK; import
   path cũng vậy. Sau migration: mọi contact + student anchor thuộc owner.
5. Học vụ: đọc điểm danh/điểm/nhận xét/statement lớp được gán (mọi write →
   403), gửi báo cáo cho contact lớp đó; lớp không gán → 404/empty.
6. Trợ giảng: đọc roster + read/write điểm danh lớp gán (không duyệt); lớp
   khác → 404/empty.
7. Peer cùng center không gán: không thấy gì (404/empty, không phải 403 leak).

## Success criteria

- [ ] 7 acceptance criteria trên có integration/e2e test tương ứng, xanh trên
      isolated e2e stack (`teka-e2e`).
- [ ] `make test-api` + web vitest xanh; coverage floor giữ (60%).
- [ ] `scoping_guard_test.go` mở rộng cover helper assignment mới; không repo
      nào branch trên `IsOwner`/`Has` trực tiếp.
- [ ] `docs/api-guidelines.md` cập nhật: mục scoping theo `class_staff`, phone
      privacy, ownership model mới.

## Risks (tổng hợp — chi tiết per phase)

- Blast radius ~8 feature (classes, sessions, attendance, enrollments,
  students, grading, teaching, statements/notifications) + imports + seeds +
  e2e → phasing bắt buộc, mỗi phase một PR.
- Dual-write invariant (D2) gãy → GV mới mất quyền hoặc 2 GV active. Index DB
  + test tx handoff chặn.
- Migration 000016 đổi anchor ảnh hưởng mọi query `teacher_id = $self` trên
  contacts/students của member → phải ship CÙNG code owner-only (P3 atomically).
- UX regression: member mất đường tạo contact/student — cần copy/hướng dẫn UI
  rõ ràng trong P3.
