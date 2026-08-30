---
phase: 4
title: "Writes theo capability map"
status: pending
priority: P1
effort: "2d"
dependencies: [2, 3]
---

# Phase 4: Writes theo capability map

## Overview

Chuyển các đường GHI artifacts của lớp từ creator-anchor (`teacher_id = $self`)
sang assignment ACTIVE + capability map (`authctx.StaffRolesFor`). GV được gán
(kể cả sau handoff) full write trên artifacts lớp; GV cũ mất write (chỉ còn
đọc); trợ giảng ghi điểm danh không cần duyệt; học vụ thuần đọc + gửi
statement per class. Audit các feature còn giả định GV-owns-rows.

## Requirements

- Functional: theo capability matrix v1 (plan.md / brainstorm):
  attendance.write = GV + trợ giảng; scores.write, remarks.write,
  lesson_plan.write, enrollment.write (end/delete — create đã mở P3) = GV;
  statement.send per class = học vụ; owner mọi thứ.
- Non-functional: write bằng role không có capability → 403; không assignment
  → 404/empty (không leak tồn tại lớp); mọi write path có test 5 vai.

## Architecture

**Fragment ghi** — bổ sung `classscope.WriteExists(alias, roles []string)`:

```sql
EXISTS (SELECT 1 FROM class_staff cs WHERE cs.class_id = <alias>.class_id
        AND cs.teacher_id = ? AND cs.center_id = ?
        AND cs.ended_at IS NULL AND cs.role_key = ANY(?))
```

Service tra `authctx.StaffRolesFor(cap)` rồi truyền role slice xuống repo
helper (`writeScoped(ctx, sc, roles)`) — repo không đọc capability map (giữ
scoping guard: repo chỉ nhận tham số, branch duy nhất trên `CenterWide()`).

**Semantics 403 vs 404**: caller CÓ assignment (kể cả ended) trên lớp nhưng
thiếu capability → 403 honest (thấy lớp, thiếu quyền — cùng triết lý send
exclusivity trong api-guidelines). Không có assignment nào → 404/empty.

**Điểm chuyển đổi:**

| Write path | Hiện tại | Capability |
|---|---|---|
| attendance upsert/confirm (attendance/repository.go `scoped` 80–85 + service) | teacher_id=self | attendance.write (GV, trợ giảng) |
| grading UpsertScores (grading/service.go, key class×session×student×component) | class-teacher/owner gate | scores.write (GV) |
| teaching lesson plans create/update/submit (teaching/service.go) | teacher_id=self | lesson_plan.write (GV); owner review loop giữ nguyên |
| teaching session notes + marks (nhận xét) | teacher_id=self | remarks.write (GV) |
| sessions lifecycle: hold/cancel/generate/pending confirm | teacher_id=self | attendance.write? KHÔNG — lifecycle buổi học thuộc GV: dùng capability riêng `sessions.write` (GV) thêm vào map |
| enrollments end/delete | creator/owner | enrollment.write (GV active của lớp) + owner |
| statements list/detail cho học vụ + gửi (bulk send, preview, resume per class) | `ReportsOversight()` center-wide | thêm nhánh học vụ: contact có enrollment active trong lớp học vụ gán (D1); send path notifications gate tương tự |

Lưu ý attendance: row attendance có creator `teacher_id` — GHI mới stamp
`teacher_id = $self` của người ghi (trợ giảng ghi thì ghi công trợ giảng);
bất biến "writes keep teacher_id = $self" trong api-guidelines GIỮ, chỉ điều
kiện ĐƯỢC ghi đổi sang assignment.

**Học vụ send per class** (D1): notifications bulk-send/preview/resume nhận
thêm đường gate: caller có assignment active `hoc_vu` trên lớp mà period/
statement target thuộc về (contact → enrollment active → class gán). Sender
attribution: rows ghi công học vụ (pattern thư ký đã có — sender = caller,
gửi từ zalo của caller). `reports.send` center-wide giữ nguyên song song.

**Audit GV-owns-rows** (làm trong phase này, sửa nếu lệch):

- dashboard (`dashboard.view`): số liệu theo teacher_id — quyết định: dashboard
  member vẫn theo own-rows + lớp gán (readScoped) — kiểm và test.
- notifications ledger/list các view own-rows (`runsOwnScoped`
  notifications/repository.go:203).
- imports điểm danh/điểm (nếu tồn tại đường import này) theo capability map.
- billing/payments/collections: owner + oversight domain — không mở cho GV;
  chỉ xác nhận không dùng classes.teacher_id ở write nào.

**Web**: gate nút write theo `my_staff_roles` (P2 đã có): attendance edit cho
GV + trợ giảng, điểm/nhận xét/giáo án chỉ GV, tab thống kê/statement + nút gửi
cho học vụ. UI học vụ: trang lớp read-only + action "Gửi báo cáo".

## Related Code Files

- Modify: `apps/api/internal/shared/classscope/classscope.go` (WriteExists),
  `apps/api/internal/shared/authctx/class_staff.go` (thêm `sessions.write`)
- Modify: `apps/api/internal/features/{attendance,grading,teaching,sessions,enrollments,statements,notifications}/{repository,service}.go` + integration tests
- Modify: `apps/api/internal/features/{dashboard nếu là package riêng — xác vị trí thật lúc cook}/…` audit
- Modify: `apps/web/src/features/teaching` classbook (attendance/score/remark gates), statements UI học vụ
- Modify: e2e specs: attendance theo trợ giảng, học vụ send, GV-cũ-mất-write
- Modify: `docs/api-guidelines.md` — capability map section

## Implementation Steps

1. `WriteExists` + `writeScoped` helpers + guard test mở rộng.
2. Đổi từng write path theo bảng; mỗi path integration test 5 vai + case
   GV-cũ-sau-handoff (ended assignment): write → 403, read → 200.
3. Học vụ statements read + send per class (API + attribution test).
4. Audit GV-owns-rows list trên — ghi kết quả vào delivery note của phase.
5. Web gates + vitest; e2e 3 kịch bản mới.
6. Full suites + swagger regen.

## Success Criteria

- [ ] Acceptance 2, 5, 6 của plan.md pass (handoff write-flip, học vụ read+send, trợ giảng attendance).
- [ ] GV cũ sau handoff: mọi write 403, đọc lịch sử 200 — integration + e2e.
- [ ] Trợ giảng ghi điểm danh thẳng, không endpoint duyệt nào tồn tại.
- [ ] Học vụ gửi statement lớp gán từ zalo của mình, attribution đúng; không gửi được lớp khác (404).

## Risk Assessment

- **Đây là phase behavior-flip lớn nhất** (GV cũ mất write): ship sau khi P1–P3
  soak; release note cho user. Rollback = revert code (schema không đổi trong
  phase này).
- **Học vụ thấy statement gia đình gồm phí lớp khác** (D1, đã chấp nhận ở
  brainstorm-level) — confirm lại ở validation trước khi cook.
- **Miss một write path** → grep sweep `teacher_id = ?` / `sc.TeacherID` trong
  các repo feature lớp trước khi đóng phase; guard test liệt kê write methods.
