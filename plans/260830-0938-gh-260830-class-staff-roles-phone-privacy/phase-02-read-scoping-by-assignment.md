---
phase: 2
title: "Read scoping theo assignment"
status: pending
priority: P1
effort: "1d"
dependencies: [1]
---

# Phase 2: Read scoping theo assignment

## Overview

Chuyển các đường ĐỌC dữ liệu lớp từ check `classes.teacher_id = $self` sang
check assignment `class_staff` (mọi assignment, kể cả ended — quyền đọc lịch
sử R4.1). Học vụ + trợ giảng lần đầu THẤY lớp được gán: class list/detail,
sessions, roster, attendance, điểm, nhận xét/giáo án (read-only — write vẫn
theo scoping cũ cho tới Phase 4). Phone chưa đụng (Phase 3).

## Requirements

- Functional: caller có assignment trên lớp đọc được: class + schedules +
  sessions, enrollments, students của lớp, attendance, scores (grading),
  teaching (lesson plans, session notes/marks) của lớp đó. Assignment ended →
  vẫn đọc; không assignment → 404/empty như hiện tại.
- Non-functional: giữ discipline repo-helper; một SQL fragment dùng chung,
  không copy-paste EXISTS mỗi nơi mỗi kiểu.

## Architecture

**Helper dùng chung** — `apps/api/internal/shared/authctx` (hoặc
`internal/shared/classscope` nếu import cycle với gorm — authctx hiện không
import gorm, giữ authctx thuần: đặt fragment ở dạng chuỗi SQL + args):

```go
// classscope.ReadExists trả fragment EXISTS cho cột class_id của bảng alias.
// Đọc: MỌI assignment (active + ended).
//   EXISTS (SELECT 1 FROM class_staff cs WHERE cs.class_id = <alias>.class_id
//           AND cs.teacher_id = ? AND cs.center_id = ?)
```

Mỗi repo giữ helper riêng (`readScoped`) theo convention hiện có, thân helper
gọi fragment chung. Pattern chuẩn hoá (thay thân các `readScoped` hiện tại):

```go
// readScoped: center filter + (own rows OR lớp có assignment của caller).
q.Where("center_id = ?", sc.CenterID)
if !sc.CenterWide() {
    q = q.Where("(teacher_id = ? OR "+classscope.ReadExists("enrollments")+")",
        sc.TeacherID, sc.TeacherID, sc.CenterID)
}
```

**Điểm chuyển đổi** (thay `classes.teacher_id = ?` / thêm nhánh assignment):

| Feature | Hiện tại | Đổi thành |
|---|---|---|
| enrollments `readScoped` (repository.go:93–104) | join classes assigned | EXISTS class_staff |
| students `readScoped` (repository.go:81–95, working tree) | enrollment→class assigned | EXISTS class_staff qua enrollments join sẵn có |
| attendance `StudentNames` (repository.go:130–172) | mirror students | EXISTS class_staff |
| attendance reads (list theo session/class) | `scoped` teacher_id=self | thêm `readScoped` assignment cho GET paths |
| classes `scoped`/`scopedSchedules` (repository.go:71–86) — GET list/detail/schedules | teacher_id=self | thêm `readScoped`: own OR assignment (cột class_id = classes.id) |
| sessions reads (classbook) | theo teacher/class cũ | `readScoped` assignment |
| grading reads (scores theo class/session) | center-scoped + service gate class-teacher | service gate mở cho assignment holder (read) |
| teaching reads (lesson plan, session notes/marks) | theo teacher_id/class | `readScoped` assignment (read-only cho học vụ/trợ giảng theo capability... đọc: mọi role) |

Ghi chú: contract cho trợ giảng v1 chỉ nói roster + điểm danh; nhưng quyền đọc
đồng nhất "mọi assignment đọc được dữ liệu lớp" (brainstorm R4.1 áp chung mọi
role). Giữ nguyên tắc một luật đọc duy nhất — đơn giản hơn per-role read matrix
và khớp "GV cũ giữ quyền đọc lịch sử" cho mọi role bị gỡ.

**`my_staff_roles` cho web**: class list/detail response (classes/dto.go) thêm
`my_staff_roles []string` — role active của caller trên lớp (owner: `[]` +
web đã có owner flag). Web dùng từ Phase 4 để gate nút write; thêm ngay ở đây
để tránh đổi contract 2 lần.

**Web**: học vụ/trợ giảng đăng nhập thấy lớp được gán trong class picker +
trang lớp (read-only UI tự nhiên vì các nút write bị API 403 — nhưng ẨN nút
write cho role không có capability, dựa `my_staff_roles`, ngay phase này để
khỏi ship UI bấm-ra-403).

## Related Code Files

- Create: `apps/api/internal/shared/classscope/classscope.go` (+ unit test)
- Modify: `apps/api/internal/features/{enrollments,students,attendance,classes,sessions,grading,teaching}/repository.go` (+ service gates grading/teaching) + integration tests từng feature
- Modify: `apps/api/internal/features/classes/dto.go` + service (my_staff_roles)
- Modify: `apps/api/internal/features/scoping_guard_test.go` — cho phép/enforce pattern mới (repo chỉ branch trên `CenterWide()`; fragment nhận args, không đọc Scope sâu)
- Modify: `apps/web/src/features/teaching|roster` — class schemas (`my_staff_roles`), ẩn nút write theo role
- Modify: `docs/api-guidelines.md` — mục "Class-teacher roster reads" viết lại thành "Class-staff reads"

## Implementation Steps

1. `classscope` fragment + unit test.
2. Đổi từng feature theo bảng trên, mỗi feature kèm integration test 4 vai:
   owner / GV assigned / học vụ assigned / peer không gán (+ assignment ended
   vẫn đọc được).
3. `my_staff_roles` vào classes responses + swagger regen.
4. Web: schemas + ẩn write theo role; vitest.
5. Cập nhật docs/api-guidelines.md.
6. `make test-api` + e2e roster/attendance specs.

## Success Criteria

- [ ] Học vụ + trợ giảng thấy lớp gán (list, roster, điểm danh, classbook) — e2e per role.
- [ ] GV cũ (assignment ended sau handoff) vẫn đọc lịch sử lớp cũ — integration test.
- [ ] Peer không gán: 404/empty mọi endpoint trên — không leak qua 403.
- [ ] Behavior GV hiện tại + owner không đổi (regression xanh).

## Risk Assessment

- **N+1 / perf**: EXISTS trên class_staff mỗi list query — index
  `idx_class_staff_teacher` + `idx_class_staff_class` (P1) cover; bảng nhỏ
  (staff × class), chấp nhận.
- **Widening ngoài ý muốn**: fragment đọc áp cả assignment ended — đúng theo
  R4.1, nhưng phải chắc mọi WRITE path không dùng nhầm `readScoped` (audit
  từng repo khi đổi; test write-bằng-role-đọc → 403).
- **Import cycle authctx↔gorm**: né bằng package `classscope` thuần SQL string.
