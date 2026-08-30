---
title: "Deep plan: class staff roles + phone privacy"
date: 2026-08-30
summary: "ak-plan --deep hoàn tất: 5 phase / 11d, red-team 15 finding đã áp, validation đảo 3 quyết định (D6 import grantable, D9 per-class run, D1 statement bản theo lớp)"
---

# Deep plan: class staff roles + phone privacy

## What happened

Chạy `/ak-plan --deep` từ brainstorm report
`plans/reports/brainstorm-260830-0825-GH-260830-class-staff-roles-phone-privacy.md`.
Kết quả: plan `plans/260830-0938-gh-260830-class-staff-roles-phone-privacy/`
(plan.md + 5 phase files, tổng 11d, `ak plan validate` OK).

- 3 Explore researchers khảo sát RBAC/scoping/migration hiện trạng; 1
  researcher claim sai (lesson plans "chưa có") — tự verify lại: teaching
  feature chính là giáo án.
- Red-team 3 reviewer thù địch: 30 finding thô → 15 sau dedupe (8 Crit,
  6 High, 1 Med), user accept cả 15, đã áp vào toàn bộ phase files. Nặng
  nhất: 000016 đâm unique per-teacher (phải merge + re-key trước anchor),
  tạo lớp mới không sinh assignment, tally attendance own-rows làm invoice
  thiếu tiền, `classes.Get` là write-auth port dùng chung không được nới
  in-place, TargetContacts thiếu chiều class → leak phone toàn center.
- Red-team cũng bắt 2 lỗi facts trong plan gốc: imports gate thật là
  `Has(PermImportsRun)` (service.go:207, handler.go:76 chỉ là swagger
  comment); grading gate là session-teacher chứ không phải class-teacher.
- Consistency sweep 6 file: 3 lệch nhỏ (dependency P4, comment capability
  map P1) — đã sửa, 0 mâu thuẫn còn lại.

## Decision

Validation interview 7 câu — 4 theo đề xuất, 3 ĐẢO (bảng đầy đủ trong
plan.md § Validation Interview):

- **D6 đảo**: giữ `imports.run` grantable (member được grant vẫn import),
  nhưng mọi row import anchor owner cứng + dedupe scope owner.
- **D9 mới**: notification run per-class ngay trong plan này (không giữ 409
  per-period) — migration 000017 thêm `class_id` vào notification_runs.
- **D1 đảo (lớn nhất)**: học vụ làm việc trên statement BẢN THEO LỚP —
  thấy và gửi chỉ dòng phí lớp gán; phụ huynh gia đình nhiều lớp nhận nhiều
  tin. `statements.class_id` nullable + token derive theo class trong
  000017; bản gia đình vẫn là đơn vị của owner/oversight. P4: 2.5d → 4d.

Giữ theo đề xuất: zalo match/mapping mở cho học vụ giới hạn lớp gán; D8
(GV mới sửa được lịch sử, GV cũ mất write); D7 survivor = created_at sớm
nhất, dry-run collision trên prod trước khi cook P3.

## Next steps

- User chọn kết thúc session, chưa cook. Khi triển khai:
  `/ak:cook plans/260830-0938-gh-260830-class-staff-roles-phone-privacy`
  bắt đầu Phase 1 (000015_class_staff + 4 lifecycle hook).
- Trước cook P3: chạy dry-run đếm collision (center_id, phone)/(zalo) trên
  prod; xác nhận account e2e là owner (roster.spec.ts helper dòng 7–8).
- P3 deploy theo runbook (code trước → smoke 403 → migrate 000016); P5 chỉ
  ship sau soak P1–P4.

> Historical work record — not durable authority. Prefer docs/specs/ADRs for current decisions.
