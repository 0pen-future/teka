---
phase: 2
title: "Read scoping theo assignment"
status: completed
priority: P1
effort: "1.5d"
dependencies: [1]
---

# Phase 2: Read scoping theo assignment

## Overview

Chuyển các đường ĐỌC dữ liệu lớp từ check `classes.teacher_id = $self` sang
check assignment `class_staff` (mọi assignment, kể cả ended — quyền đọc lịch
sử R4.1). Học vụ + trợ giảng lần đầu THẤY lớp được gán: class list/detail,
sessions, roster, attendance, điểm, nhận xét/giáo án (read-only — write vẫn
theo scoping cũ cho tới Phase 4, và phase này phải CHỨNG MINH điều đó bằng
test). Phone chưa đụng (Phase 3).

## Requirements

- Functional: caller có assignment trên lớp đọc được: class + schedules +
  sessions, enrollments, students của lớp, attendance, scores (grading),
  teaching (lesson plans, session notes/marks) của lớp đó. Assignment ended →
  vẫn đọc; không assignment → 404/empty như hiện tại; lớp soft-deleted →
  assignment KHÔNG cấp đọc.
- Non-functional: giữ discipline repo-helper; fragment dùng chung có chữ ký
  khớp đủ mọi surface (không copy-paste EXISTS mỗi nơi mỗi kiểu); KHÔNG nới
  bất kỳ write path nào trong phase này.

**Review carryover từ P1 (code review 2026-08-30):**

- `classstaff/service.go` gate đọc staff list bằng `IsOwner`, trong khi
  `classes/repository.go` mở đọc lớp cho `CenterWide()`: member cầm
  `data.view_center_wide` GET được lớp nhưng nhận 404 ở `/staff` (fail-closed,
  không leak, nhưng lệch). Phase này khi định nghĩa fragment đọc chung phải
  quyết: staff list đi theo cùng luật đọc lớp hay giữ owner-only — và test rõ.
- Fixture `testutil.JoinCenter` đóng membership bằng `UPDATE center_members
  SET left_at = now()` trần — KHÔNG đóng stint như production
  `CloseMembership`. Test P2 mô phỏng rời trung tâm phải dùng đường
  production (hoặc fixture phải đóng stint) kẻo pass vì lý do sai.

## Architecture

**Helper dùng chung** — package `apps/api/internal/shared/classscope` (thuần
SQL string + args, không import gorm; lưu ý: `authctx` thực tế KHÔNG import
gorm — tách package vẫn đúng để giữ authctx thuần vocabulary, nhưng lý do
"import cycle" không tồn tại):

```go
// ReadExists nhận BIỂU THỨC cột class-id của query gọi (không giả định bảng
// nào cũng có cột class_id — red-team F9: students/StudentNames không có):
//   classscope.ReadExists("enrollments.class_id")
//   classscope.ReadExists("classes.id")
// Fragment LUÔN join classes và lọc soft-delete bên trong EXISTS —
// các readScoped hiện tại đều mang classes.deleted_at IS NULL, đánh rơi là
// widening lớp đã xoá:
//   EXISTS (SELECT 1 FROM class_staff cs
//           JOIN classes c2 ON c2.id = cs.class_id AND c2.deleted_at IS NULL
//           WHERE cs.class_id = <classIDExpr>
//             AND cs.teacher_id = ? AND cs.center_id = ?)
func ReadExists(classIDExpr string) (sql string, argCount int)

// Cho bảng không có class_id trực tiếp (students, attendance.StudentNames):
// nested qua enrollments active-or-any theo đúng subquery hiện có của từng repo.
func ReadExistsViaEnrollment(studentIDExpr string) (sql string, argCount int)
```

Mỗi repo giữ helper riêng (`readScoped`) theo convention hiện có, thân helper
gọi fragment chung:

```go
// readScoped: center filter + (own rows OR lớp có assignment của caller).
q.Where("center_id = ?", sc.CenterID)
if !sc.CenterWide() {
    frag, _ := classscope.ReadExists("enrollments.class_id")
    q = q.Where("(teacher_id = ? OR "+frag+")", sc.TeacherID, sc.TeacherID, sc.CenterID)
}
```

**`classes.Get` là authorization port dùng chung — KHÔNG nới in-place**
(red-team F6 Critical): teaching/sessions/grading/dashboard resolve class qua
`classes.Get`/`scoped` làm write gate (`teaching/repository.go:34-37` ghi rõ
"class resolution through classes.Get IS the authorization gate"). Nới nó là
mở write cho trợ giảng/học vụ TRƯỚC khi capability map tồn tại. Tách 2 port:

- `Get`/`scoped` — GIỮ NGUYÊN own-rows, tiếp tục phục vụ write gate của:
  `teaching/service.go:557` (resolveClass cho curriculum/lesson-plan write),
  `sessions/service.go:88,235` (generate/hold), `grading/service.go:526`,
  `centers/dashboard.go:216`. Không đổi call site nào trong số này ở P2.
- `GetReadable`/`readScoped` — port MỚI (own OR assignment) chỉ cho GET
  list/detail/schedules của chính feature classes và các read path bảng dưới.

**Điểm chuyển đổi** (thay `classes.teacher_id = ?` / thêm nhánh assignment):

| Feature | Hiện tại | Đổi thành |
|---|---|---|
| enrollments `readScoped` (repository.go:93–104) | join classes assigned | `ReadExists("enrollments.class_id")` |
| students `readScoped` (repository.go:81–95, working tree) | enrollment→class assigned | `ReadExistsViaEnrollment("students.id")` (giữ nested + deleted_at) |
| attendance `StudentNames` (repository.go:130–172) | mirror students | `ReadExistsViaEnrollment` |
| attendance reads (list theo session/class) | `scoped` teacher_id=self | thêm `readScoped` assignment cho GET paths |
| classes GET list/detail/schedules | `scoped` teacher_id=self | port mới `readScoped` (cột `classes.id`); write paths giữ `scoped` |
| sessions reads (classbook) | theo teacher/class cũ | `readScoped` assignment — CHỈ GET; generate/hold giữ port cũ |
| grading reads (scores theo class/session) | center-scoped + service gate theo `session.TeacherID` (grading/service.go:323 — plan cũ ghi nhầm "class-teacher") | mở READ cho assignment holder; write gate session-teacher GIỮ NGUYÊN tới P4 |
| teaching reads (lesson plan, session notes/marks) | theo teacher_id/class | `readScoped` assignment (đọc: mọi role) |

Ghi chú: contract cho trợ giảng v1 chỉ nói roster + điểm danh; nhưng quyền đọc
đồng nhất "mọi assignment đọc được dữ liệu lớp" (brainstorm R4.1 áp chung mọi
role). Giữ nguyên tắc một luật đọc duy nhất.

**Dashboard audit (kéo từ P4 về đây — red-team F13):** dashboard KHÔNG dùng
scope caller: `centers/dashboard.go:82-84` forge
`authctx.Scope{TeacherID, CenterID}` (IsOwner=false, Perms=nil) rồi bơm vào
`classes.List`/`classes.Get`/`sessions.ListRangeReadOnly`. Quyết định:
drill-down "lớp của GV X" giữ nghĩa own-rows (`classes.teacher_id`) — dashboard
tiếp tục dùng port `scoped` cũ, KHÔNG chuyển sang readScoped (nếu không KPI
cộng lớp trợ giảng + lớp đã bàn giao, đếm trùng giữa 2 GV). Cấm mọi logic mask
(P3 `PhoneVisible`) chạy trên scope giả — mask luôn nhận `sc` thật của caller.

**`my_staff_roles` cho web** (red-team F14 — `FromModel` là mapper thuần, dùng
chung với dashboard `centers/handler.go:468`, không nhét per-caller field vào):

- GIỮ `FromModel` nguyên vẹn; thêm `FromModelWithRoles(class, roles []string)`.
- Service classes load role bằng MỘT query batch
  `WHERE class_id IN (...) AND teacher_id = ?` rồi map — cấm N+1 per-row.
- Dashboard path trả `[]` (không gọi WithRoles).
- Web `classSchema` khai `my_staff_roles: z.array(z.string()).default([])` —
  response dashboard thiếu field không được vỡ parse.

**Web**: học vụ/trợ giảng đăng nhập thấy lớp được gán trong class picker +
trang lớp; ẨN nút write cho role không có capability (dựa `my_staff_roles`)
ngay phase này để khỏi ship UI bấm-ra-403.

## Related Code Files

- Create: `apps/api/internal/shared/classscope/classscope.go` (+ unit test)
- Modify: `apps/api/internal/features/{enrollments,students,attendance,classes,sessions,grading,teaching}/repository.go` (+ service gates grading/teaching READ) + integration tests từng feature
- Modify: `apps/api/internal/features/classes/{dto,service}.go` (port GetReadable + my_staff_roles batch)
- Modify: `apps/api/internal/features/scoping_guard_test.go` — cho phép/enforce pattern mới (repo chỉ branch trên `CenterWide()`; fragment nhận args, không đọc Scope sâu)
- Modify: `apps/web/src/features/teaching|roster` — class schemas (`my_staff_roles` với `.default([])`), ẩn nút write theo role
- Modify: `docs/api-guidelines.md` — mục "Class-teacher roster reads" viết lại thành "Class-staff reads"

## Implementation Steps

1. `classscope` fragment (ReadExists + ReadExistsViaEnrollment, kèm
   deleted_at) + unit test, gồm test "lớp soft-deleted → không đọc".
2. Port split classes: `GetReadable` mới; xác nhận 4 call site write gate
   (teaching:557, sessions:88+235, grading:526, dashboard:216) vẫn trên port
   cũ — ghi thành checklist trong PR.
3. Đổi từng feature theo bảng trên, mỗi feature kèm integration test 4 vai:
   owner / GV assigned / học vụ assigned / peer không gán (+ assignment ended
   vẫn đọc được).
4. **Write-freeze test**: trợ giảng/học vụ được gán → POST curriculum, PUT
   scores, generate sessions, upsert attendance → 403/404 y như trước phase
   (chốt tuyên bố "P2 chưa đụng write").
5. `my_staff_roles` batch + swagger regen; web schemas `.default([])`.
6. Web: ẩn write theo role; vitest.
7. Cập nhật docs/api-guidelines.md.
8. `make test-api` + e2e roster/attendance specs.

## Success Criteria

- [x] Học vụ + trợ giảng thấy lớp gán (list, roster, điểm danh, classbook) — e2e per role.
- [x] GV cũ (assignment ended sau handoff) vẫn đọc lịch sử lớp cũ — integration test.
- [x] Peer không gán: 404/empty mọi endpoint trên — không leak qua 403.
- [x] Write-freeze: mọi write path của học vụ/trợ giảng vẫn bị chặn như trước.
- [x] Lớp soft-deleted: assignment holder 404/empty.
- [x] Dashboard KPI không đổi số (regression trên seed data).
- [x] Behavior GV hiện tại + owner không đổi (regression xanh).

## Risk Assessment

- **N+1 / perf**: EXISTS trên class_staff mỗi list query — index
  `idx_class_staff_teacher` + `idx_class_staff_class` (P1) cover; bảng nhỏ
  (staff × class), chấp nhận. `my_staff_roles` bắt buộc batch query.
- **Widening ngoài ý muốn**: fragment đọc áp cả assignment ended — đúng theo
  R4.1; write-freeze test + port split chặn rò sang write.
- **`data.view_center_wide`**: member được grant key này đi đường
  `CenterWide()` bỏ qua toàn bộ class_staff — hành vi RBAC hiện có, giữ
  nguyên, ghi vào docs (plan.md D4).
