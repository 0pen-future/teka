---
phase: 5
title: "Cleanup scoping cũ"
status: done
priority: P2
effort: "0.5d"
dependencies: [4]
---

# Phase 5: Cleanup scoping cũ

## Overview

Khi P1–P4 đã deploy và soak (owner dùng staff UI, handoff chạy qua dual-write,
không regression), gỡ các nhánh scoping còn dựa `classes.teacher_id` /
`session.TeacherID` mà P2/P4 đã thay bằng assignment. KHÔNG drop cột (quyết
định D3 plan.md): `classes.teacher_id` vẫn là con trỏ GV chính denormalized,
handoff tiếp tục sync nó cùng assignment.

## Requirements

- Functional: hành vi bất biến — phase này chỉ xóa code path chết.
- Non-functional: sau cleanup, grep contract (dưới) chỉ còn các chỗ dùng
  hợp lệ.

## Architecture

- Gỡ nhánh join/so sánh `classes.teacher_id = sc.TeacherID` còn sót trong
  readScoped các repo. Lưu ý phân loại đúng (đồng bộ với P4 — bảng method
  attendance):
  - `<<table>>.teacher_id` với vai trò ATTRIBUTION (cột ghi công creator/
    last-writer) GIỮ — không phải scoping.
  - `<<table>>.teacher_id = $self` với vai trò ROW FILTER trên attendance
    write/tally đã bị P4 THAY THẾ — ở đây chỉ xác nhận không còn sót (khác
    bản plan cũ từng nói "own-rows GIỮ" — câu đó chỉ còn đúng cho các view
    cá nhân như notifications `runsOwnScoped`, dashboard drill-down).
- **Grep contract (mở rộng theo red-team F10):** sau cleanup, các pattern sau
  chỉ còn ở chỗ hợp lệ:
  - `classes.teacher_id`: handoff dual-write, classes model/DTO hiển thị GV
    chính, dashboard drill-down (own-rows có chủ đích), migrations lịch sử.
  - `session.TeacherID` trong service gates grading/teaching/sessions: 0 chỗ
    (đã thay bằng WriteExists ở P4).
  - `teacher_id = ?`/`sc.TeacherID` row-filter trong attendance repo
    write/tally: 0 chỗ.
- Xác nhận bất biến dual-write bằng query parity (chạy read-only trên prod
  trước khi merge):

```sql
SELECT count(*) FROM classes c
WHERE c.deleted_at IS NULL AND NOT EXISTS (
  SELECT 1 FROM class_staff cs WHERE cs.class_id = c.id
  AND cs.teacher_id = c.teacher_id AND cs.role_key = 'giao_vien'
  AND cs.ended_at IS NULL);
-- phải = 0
```

- **Remediation khi parity ≠ 0 (red-team F2):** KHÔNG bị chặn vĩnh viễn —
  chạy lại backfill idempotent của 000015 (SQL y hệt, ON CONFLICT DO NOTHING)
  như lệnh reconcile; hoặc owner handoff-về-chính-GV từng lớp lệch (P1 đã biến
  no-op path thành lệnh repair). Ghi số trước/sau vào delivery note. Nguồn
  drift kỳ vọng = 0 nhờ create-hook P1; nếu ≠ 0, tìm nguồn ghi lớp không qua
  service trước khi merge.
- Docs: `docs/api-guidelines.md` chốt mô hình cuối (class_staff là nguồn
  quyền lớp duy nhất; teacher_id các bảng = creator/last-writer attribution).
- Ghi chú tương lai (không làm): multi-GV = drop `uq_class_staff_one_gv` +
  bỏ dual-write + quyết định lại `classes.teacher_id`.

## Related Code Files

- Modify: các repo còn nhánh cũ (danh sách chính xác chốt lúc cook bằng grep
  contract trên + diff so với P2/P4), `docs/api-guidelines.md`

## Implementation Steps

1. Parity query trên prod; nếu ≠ 0 → reconcile (backfill idempotent) → về 0,
   ghi vào delivery note.
2. Grep sweep theo contract 3 pattern + gỡ nhánh chết, mỗi lần gỡ chạy
   integration suite feature đó.
3. Full `make test-api` + e2e stack + docs.

## Success Criteria

- [x] Parity = 0 ghi nhận trước merge (kể cả sau reconcile nếu cần).
      `classes.teacher_id` ↔ active `giao_vien` stint parity = 0 cả hai chiều
      (resource-action-rbac phase-08 prod inventory 2026-08-31).
- [x] Grep contract đạt cả 3 pattern. `session.TeacherID`/attendance row-filter
      đã thay ở P4; nhánh `classes.teacher_id` còn sót trong readScoped gỡ ở
      commit fa8cfc8 (class reads stint-only, dead creator arm removed).
- [x] Toàn bộ suite xanh, không đổi behavior nào observable. Build/vet clean,
      unit + integration xanh trên các package liên quan (classes, sessions,
      grading, teaching), `make lint` clean, web suite xanh.

Deploy 2026-08-31 ~11:46 xác nhận: binary provenance = HEAD 602a4cc;
`/readyz` 200; 0 error/fatal/panic log; denial baseline 403s=0/24h — không
regression quan sát được.

## Review carryover từ P1 (code review 2026-08-30)

- Kick member (`RemoveMember`) không đòi handoff lớp trước: CTE đóng stint
  giao_vien nhưng `classes.teacher_id` vẫn trỏ người bị kick → lớp sống với
  0 GV active. `uq_class_staff_one_gv` chỉ chặn 2 GV, không chặn 0. Đây là
  drift source đã biết: parity query của phase này phải đếm cả chiều
  "teacher_id không có stint active" (không chỉ chiều 2 stint), và reconcile
  bằng backfill idempotent / handoff no-op.

## Review carryover từ P2 (code review 2026-08-30)

- `classes.Service.GetReadable` luôn chạy batch query `RolesByClass` nhưng 4
  consumer ngoài classes handler (sessions, grading, teaching ×3 endpoint) vứt
  kết quả — một round-trip thừa mỗi GET theo lớp, classbook trả ~4 lần. Khi
  cleanup: tách `GetReadable` (không roles) khỏi `GetReadableWithRoles`
  (handler dùng).
- Vitest chưa exercise nhánh `canWrite=false` ở component attendance-page /
  classbook-page / session-detail-panel / component-score-grid (MSW
  `/centers/me` mặc định trả owner). Nhánh false hiện chỉ được pin bởi
  `class-permissions.test.ts`, `class-settings-page.test.tsx` (override member
  — pattern đúng để nhân bản) và e2e `class-staff-read.spec.ts`. P4 đụng các
  component này cho capability map — bổ sung override member khi sửa.

## Risk Assessment

- **Gỡ nhầm attribution stamp** → phân loại attribution vs row-filter theo
  bảng P4; review diff theo checklist.
- **Soak chưa đủ** → gate: phase này không ship cùng release với P4.
