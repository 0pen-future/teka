---
phase: 3
title: "Student-scores API (GET/PUT theo buổi)"
status: done
priority: P1
effort: "1d"
dependencies: [1, 2]
---

# Phase 3: Student-scores API (GET/PUT theo buổi)

## Overview

Phần điểm học sinh của feature `grading`: đọc/ghi điểm thành phần theo buổi
học. Key = (session, component, student); ghi đè được; teacher của lớp +
owner được ghi.

## Requirements

- Functional:
  - `GET /sessions/:id/scores` — toàn bộ điểm thành phần của buổi (mọi học
    sinh × component), kèm danh sách component của lớp để UI dựng cột.
  - `PUT /sessions/:id/scores` — batch upsert
    `[{student_id, component_id, score}]`; `score: null` xóa dòng (bảng
    không giữ dòng rỗng, như session_marks).
- Non-functional: score 0–10, tối đa 1 chữ số thập phân (NUMERIC(4,1);
  step 0.5 chỉ là ràng buộc UI phase 5, API không ép bội số 0.5 — mirror
  session_marks); reject component
  không thuộc lớp của session; reject student ngoài roster? — mirror đúng
  cách `teaching.PutMarks` xử lý (soi trước, làm giống).

## Architecture

- **Gate ghi (divergence có chủ đích):** teacher của session **hoặc owner**.
  Khác `teaching.PutMarks` (session-teacher-only, owner bị chặn) — contract
  brainstorm AC4 chốt owner được nhập. Comment tại gate nêu rõ chủ đích để
  reviewer không "sửa cho giống teaching".
- **teacher_id trên dòng điểm:** luôn = teacher của session (anchor own-rows
  ổn định), kể cả khi owner là người ghi. Truy vết "ai ghi" đi qua audit
  log — vì vậy **bắt buộc** đăng ký `PUT /sessions/:id/scores` vào
  `audit/action.go` + test capture; audit là route-based, không tự capture.
  Quên mục này = owner ghi hộ không truy vết được ở đâu (premise của quyết
  định bỏ cột `entered_by` sụp đổ).
- **Cascade lifecycle:** dòng điểm thuộc teacher qua guard FK
  `(teacher_id, center_id) → center_members ON DELETE CASCADE` (phase 1) —
  remove teacher khỏi center xóa cả điểm owner ghi hộ; nhất quán
  session_marks, đã chấp nhận.
- **Gate đọc:** teacher của session; member khác chỉ khi `sc.CenterWide()`;
  owner mặc nhiên qua CenterWide.
- **Resolve session** → class → components: tái dùng pattern
  `teaching.resolveSession`; scores upsert theo unique
  `(session_id, component_id, student_id)` (ON CONFLICT DO UPDATE).
- **Routes:** join group `/sessions/:id` trong `grading/routes.go` (đã tạo
  phase 2).

## Related Code Files

- Modify: `apps/api/internal/features/grading/{model,dto,repository,service,handler,routes}.go`
- Modify: `apps/api/internal/features/grading/grading_integration_test.go`
- Generated: `apps/api/docs/*` qua `make api-docs`

## Implementation Steps

1. Soi `teaching.PutMarks` + `resolveSession` (service.go:294-360, 466) và
   mirror cấu trúc batch entry + tri-state.
2. Model `StudentScore` + repo: `ListBySession`, `UpsertBatch` (tx),
   `DeleteWhereNull`; scope qua `sc.CenterWide()`.
3. Service: gate teacher-or-owner, validate score range/component thuộc
   lớp, orchestrate.
4. Handler + dto + swag; wire routes; đăng ký route vào `audit/action.go`
   + test capture.
5. Integration tests: teacher ghi/sửa/xóa (null) OK; owner ghi OK; member
   center-wide đọc OK nhưng ghi 403; member thường 403 cả đọc;
   component lớp khác → 4xx; score 10.5 → 422.
6. `make test-api`.

## Success Criteria

- [ ] AC4: teacher lớp + owner nhập/sửa 0–10 một chữ số thập phân; member
      không phải teacher lớp → 403.
- [ ] Upsert idempotent, sửa đè được; null xóa dòng.
- [ ] GET trả components + scores đủ cho UI dựng grid một round-trip.
- [ ] `PUT /sessions/:id/scores` được audit capture (test) — dấu vết owner
      ghi hộ nằm ở đây.
- [ ] `make test-api` pass.

## Risk Assessment

- Owner-ghi đi ngược comment gate teaching → docs/comment phải nêu chủ
  đích (decision 4 plan.md) tránh bị "sửa lỗi" về sau.
- Batch lớn (roster × components): giới hạn entries ≤ 500/request là đủ
  (roster ~30 × 10 components); reject lớn hơn.
