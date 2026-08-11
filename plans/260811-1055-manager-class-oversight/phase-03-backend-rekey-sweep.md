---
phase: 3
title: "Backend Re-key Sweep"
status: pending
priority: P1
effort: "3d"
dependencies: [2]
---

# Phase 3: Backend Re-key Sweep

<!-- Updated: Validation Session 1 - owner không tạo hộ; create luôn gán chính caller -->


## Overview

Sweep cơ học nhưng lớn nhất plan: chuyển toàn bộ feature packages từ scope `teacher_id` sang `authctx.Scope` (center + role). Teacher giữ nguyên hành vi (thấy/ghi đúng dữ liệu của mình); owner đọc + ghi toàn center. Kết thúc sweep: `authctx.TeacherID` bị xoá — `ScopeFrom` là accessor duy nhất.

## Requirements

- Functional: owner CRUD được mọi resource trong center; teacher không đổi một li hành vi (mọi integration test cũ pass với fixture bổ sung center).
- Non-functional: không endpoint nào nhận `center_id` hay `teacher_id` từ request — create luôn gán chính caller (quyết định validate 260811: owner không tạo hộ).

## Architecture

### Quy tắc scope thống nhất (áp vào từng repo)

```go
// scoped: mọi read bound vào tenant center; role teacher siết thêm teacher_id.
// Copy helper này per-repo như pattern hiện có (teachers/repository.go:48-53);
// raw/Table() query phải tự thêm deleted_at IS NULL bằng tay.
func (r *gormRepository) scoped(ctx context.Context, sc authctx.Scope) *gorm.DB {
    q := database.FromContext(ctx, r.db).Where("center_id = ?", sc.CenterID)
    if !sc.IsOwner {
        q = q.Where("teacher_id = ?", sc.TeacherID)
    }
    return q
}
```

- **Write**: update/delete đi qua cùng `scoped()` (WHERE mang scope — không fetch-then-check): owner sửa/xoá được row của mọi teacher trong center, teacher chỉ row của mình. Create: row luôn nhận `CenterID = sc.CenterID` và `TeacherID = sc.TeacherID` — **mọi role, kể cả owner** (quyết định validate 260811: owner không tạo hộ; owner cũng là teacher bình thường với resource của mình). Các bảng con (enrollments, sessions, attendance, billing...) suy ra `teacher_id` từ parent row. Không DTO nào nhận `teacher_id`/`center_id`; composite FK `(teacher_id, center_id)` vẫn là chốt toàn vẹn DB.
- **Models**: thêm field `CenterID`; giữ `TeacherID`.
- **Services**: chữ ký nhận `authctx.Scope` thay `teacherID uuid.UUID` (đổi kiểu tham số, logic giữ nguyên trừ điểm create ở trên).
- **Handlers**: `authctx.ScopeFrom(c)` thay `authctx.TeacherID(c)`.

### Phạm vi package (sweep theo thứ tự dependency)

`teachers` (profile), `contacts`, `students`, `classes`, `enrollments`, `sessions`, `attendance`, `billing`, `payments`, `collections`, `statements`, `notifications`, `zalo`.

Ngoại lệ: `zalo` — `zalo_accounts` key theo teacher (tài khoản cá nhân, Phase 1 không re-key); các đường notification runs của zalo scope theo center như thường. `auth` — không tenant scope, đã đụng ở Phase 2.

### Điểm không-cơ-học phải soi tay khi sweep

- Query raw/Table()/JOIN nhiều bảng (collections, statements, sessions pending, views): thêm `center_id` vào **từng bảng tham gia** + `deleted_at IS NULL` viết tay — house rule `teachers/repository.go:48-53`.
- `sessions` generate path (`service.go:75-131`): generate session mới phải mang `center_id` của class; index pending 000003/000006 đã rà ở Phase 1.
- Public/unauthenticated đường `statements.public_handler.go`: scope theo token riêng của nó — kiểm không lộ cross-center.
- Uniqueness nghiệp vụ per-teacher (billing period, contact phone) giữ per-teacher — Phase 1 đã chốt; service message giữ nguyên.

## Related Code Files

- Modify: `apps/api/internal/shared/authctx/authctx.go` — xoá `TeacherID()` (cuối sweep)
- Modify: mọi `apps/api/internal/features/<pkg>/{model,repository,service,handler}.go` của 13 packages trên + integration/service tests từng package
- Modify: `apps/api/internal/server/router.go` nếu chữ ký constructor đổi
- Modify: swagger regenerate

## Implementation Steps

1. Sweep từng package theo thứ tự dependency (teachers → contacts → students → classes → ... → collections/statements); mỗi package: model + repo `scoped()` + service signature + handler + test fixture (thêm center), chạy test package đó xanh rồi mới sang package kế.
2. Thêm test mới per package: (i) owner đọc/sửa/xoá resource của teacher khác cùng center — và create của owner luôn gán chính owner; (ii) teacher A **không** thấy resource teacher B cùng center; (iii) cross-center → 403/404/rỗng.
3. Xoá `authctx.TeacherID`; `grep -rn "authctx.TeacherID" apps/api` = 0.
4. Full test suite + swagger regenerate.

## Success Criteria

- [ ] Toàn bộ test suite cũ pass (fixture thêm center — hành vi teacher không đổi)
- [ ] Ba case mới (owner-cross-teacher, teacher-isolation-trong-center, cross-center-deny) pass ở từng package
- [ ] `authctx.TeacherID` không còn tồn tại; không query nào thiếu vế `center_id`
- [ ] Không endpoint nào nhận `center_id`/`teacher_id` từ request; create của mọi role gán chính caller
- [ ] Swagger cập nhật

## Risk Assessment

- **Bỏ sót query ngoài `scoped()`** (raw SQL, Table(), preload): grep `Table(\|Raw(\|Joins(` per package làm checklist; test cross-center per package là chốt chặn hành vi.
- **Nới bất biến ghi cho owner mở lối confused-deputy** (owner center A ghi vào center B qua id): mọi write WHERE mang `center_id` — test (iii) phủ cả write.
- **Sweep dở dang khó review**: đi từng package, mỗi package một commit xanh test — không commit nửa package.
- **Đụng độ với Phase 4** trên `sessions`: sweep xong package sessions trước khi Phase 4 thêm `ListRangeReadOnly` (dependency 4←3 đã phản ánh).
