---
phase: 5
title: "Cleanup scoping cũ"
status: pending
priority: P2
effort: "0.5d"
dependencies: [4]
---

# Phase 5: Cleanup scoping cũ

## Overview

Khi P1–P4 đã deploy và soak (owner dùng staff UI, handoff chạy qua dual-write,
không regression), gỡ các nhánh scoping còn dựa `classes.teacher_id` mà P2/P4
đã thay bằng assignment. KHÔNG drop cột (quyết định D3 plan.md):
`classes.teacher_id` vẫn là con trỏ GV chính denormalized, handoff tiếp tục
sync nó cùng assignment.

## Requirements

- Functional: hành vi bất biến — phase này chỉ xóa code path chết.
- Non-functional: sau cleanup, grep `classes.teacher_id` trong features chỉ
  còn: handoff dual-write, classes model/DTO (hiển thị GV chính), backfill/
  migration lịch sử.

## Architecture

- Gỡ nhánh join/so sánh `classes.teacher_id = sc.TeacherID` còn sót trong
  readScoped các repo (nếu P2 giữ dạng "own OR assignment", own-rows GIỮ —
  chỉ gỡ các check theo classes.teacher_id, không gỡ creator anchor own-rows
  của enrollments/attendance).
- Xác nhận bất biến dual-write bằng query parity (chạy read-only trên prod
  trước khi merge):

```sql
SELECT count(*) FROM classes c
WHERE NOT EXISTS (SELECT 1 FROM class_staff cs WHERE cs.class_id = c.id
  AND cs.teacher_id = c.teacher_id AND cs.role_key = 'giao_vien'
  AND cs.ended_at IS NULL);
-- phải = 0
```

- Docs: `docs/api-guidelines.md` chốt mô hình cuối (class_staff là nguồn
  quyền lớp duy nhất; teacher_id các bảng = creator attribution).
- Ghi chú tương lai (không làm): multi-GV = drop `uq_class_staff_one_gv` +
  bỏ dual-write + quyết định lại `classes.teacher_id`.

## Related Code Files

- Modify: các repo còn nhánh cũ (danh sách chính xác chốt lúc cook bằng grep
  `classes.teacher_id` + `readScoped` diff so với P2), `docs/api-guidelines.md`

## Implementation Steps

1. Parity query trên prod = 0 (ghi vào delivery note).
2. Grep sweep + gỡ nhánh chết, mỗi lần gỡ chạy integration suite feature đó.
3. Full `make test-api` + e2e stack + docs.

## Success Criteria

- [ ] Parity = 0 ghi nhận trước merge.
- [ ] Grep contract đạt (chỉ còn 3 nhóm chỗ dùng hợp lệ).
- [ ] Toàn bộ suite xanh, không đổi behavior nào observable.

## Risk Assessment

- **Gỡ nhầm own-rows creator scoping** → chỉ gỡ check theo `classes.teacher_id`,
  không đụng `<<table>>.teacher_id = $self`; review diff theo checklist.
- **Soak chưa đủ** → gate: phase này không ship cùng release với P4.
