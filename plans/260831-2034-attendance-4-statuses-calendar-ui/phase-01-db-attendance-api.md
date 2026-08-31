---
phase: 1
title: "DB migration + attendance API 4 trạng thái"
status: completed
priority: P1
effort: "1d"
dependencies: []
---

# Phase 1: DB migration + attendance API 4 trạng thái

## Overview

Thêm trạng thái `late` vào schema, mở rộng contract confirm từ
`absent_student_ids` sang danh sách `{student_id, status, note?}`, giữ tương
thích ngược một release. Billing không đổi hành vi (mọi trạng thái vẫn
`billable=true`).

## Requirements

- Functional:
  - [x] CHECK constraint `attendance_records.status` chấp nhận `present|late|absent|excused`.
  - [x] `POST /sessions/:id/attendance` nhận body mới `marks`; học sinh trên roster không có trong `marks` → `present`; ghi `note` per-student (dùng cho "Vắng có phép — mẹ báo ốm").
  - [x] Body cũ `absent_student_ids` vẫn hoạt động (map thành `marks` với status `absent`); nếu gửi cả hai → 400.
  - [x] Validate: `status` ∈ {late, absent, excused} trong `marks` (present là mặc định, không cần gửi); student không thuộc roster → 422 (đổi từ giả định 404 ban đầu — nhất quán với validate còn lại của `marks`); trùng student_id trong `marks` → 400.
  - [x] `GET /sessions/:id/attendance` trả status 4 giá trị + note per-row (DTO hiện có sẵn field `status`, `note` — chỉ cần bảo đảm pass-through).
- Non-functional:
  - [x] Mọi record vẫn `billable=true` — thêm integration test khẳng định `TallyByEnrollment`/billing preview không đổi khi có `late`/`excused`.
  - [x] Confirm sau kỳ chốt sổ vẫn đi qua `BillingReconciler` như cũ.

## Architecture

- Vocabulary: `model.go` đã có `StatusPresent/StatusAbsent/StatusExcused` — thêm
  `StatusLate = "late"`. Semantic: present=đúng giờ, late=muộn (coi là có mặt),
  absent=vắng, excused=vắng có lý do.
- Contract confirm mới:

```json
POST /sessions/:id/attendance
{
  "marks": [
    {"student_id": "uuid", "status": "late"},
    {"student_id": "uuid", "status": "absent"},
    {"student_id": "uuid", "status": "excused", "note": "mẹ báo ốm"}
  ],
  "note": "ghi chú buổi học (optional)"
}
```

- Service `Confirm`: build record cho toàn roster như hiện tại; lookup map
  student_id→mark; default `present`; giữ nguyên soft-delete off-roster,
  idempotent-replace khi confirm lại, reconciliation kỳ đã chốt.
- Down migration: `UPDATE attendance_records SET status='present' WHERE status='late'`
  trước khi khôi phục CHECK cũ (chấp nhận mất phân biệt muộn khi rollback — ghi ở Risk).

## Related Code Files

- Create: `apps/api/migrations/000021_attendance_status_late.up.sql` + `.down.sql`
- Modify: `apps/api/internal/features/attendance/model.go` (thêm `StatusLate`)
- Modify: `apps/api/internal/features/attendance/dto.go` (`ConfirmRequest.Marks`, giữ `AbsentStudentIDs` deprecated)
- Modify: `apps/api/internal/features/attendance/service.go` (logic Confirm ~dòng 170–220)
- Modify: `apps/api/internal/features/attendance/handler.go` (validation, swag annotations)
- Modify: `apps/api/internal/features/attendance/*_test.go`, `*_integration_test.go`
- Không sửa: `apps/api/internal/features/billing/**` (chỉ thêm test khẳng định hành vi)

## Implementation Steps

1. Viết migration 000021 (ALTER CHECK; down: convert late→present rồi restore CHECK); chạy `migrations_test.go`.
2. Thêm `StatusLate` vào model; cập nhật DTO `ConfirmRequest` (marks + legacy field, exclusive-or).
3. Sửa `service.Confirm` sang map-based; giữ billable=true; giữ reconciliation.
4. Cập nhật handler validation + swag annotations; `make api-docs`.
5. Unit tests: default present, mix 4 trạng thái, legacy body, cả hai body → 400, dup student → 400, off-roster → 404.
6. Integration tests: confirm 4 trạng thái rồi gọi billing preview — số tiền không đổi so với all-present; confirm lại (sửa) thay thế đúng.

## Success Criteria

- [x] `make test-api-unit` và `make test-api` xanh.
- [x] Swagger regen không diff ngoài phần attendance.
- [x] Confirm với body cũ từ web bundle cũ vẫn hoạt động.

## Risk Assessment

- **Rollback mất dữ liệu `late`** — down migration convert late→present; chấp
  nhận được vì late = có mặt về mặt billing; ghi rõ trong file down.
- **RBAC in-progress (plan 260829-1640)** chạm cùng handler/routes — giữ nguyên
  permission middleware và keys; nếu nhánh RBAC merge trước, rebase và chỉ đổi
  phần body/service.
- **Contract drift với e2e** — e2e specs dùng body cũ sẽ vẫn pass nhờ legacy
  path; cập nhật dần ở Phase 5.
