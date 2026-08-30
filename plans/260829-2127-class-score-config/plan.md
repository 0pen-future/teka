---
title: "class-score-config"
description: "Owner cấu hình bộ điểm thành phần cho lớp; giáo viên + owner nhập điểm 0–10 theo buổi học"
status: done
priority: P1
effort: "4-5d"
tags: [api, web, migration, rbac]
created: 2026-08-29
---

# class-score-config

## Overview

Owner có menu owner-only mới trong nhóm sidebar "Trung tâm" để cấu hình lớp
học: CRUD "bộ điểm" đặt tên (VD "IELTS": Listening, Speaking, Reading,
Writing) và gán bộ cho lớp theo ngữ nghĩa **snapshot** (copy thành phần vào
lớp lúc gán). Giáo viên của lớp và owner nhập điểm 0–10 (1 chữ số thập phân)
cho từng học sinh × thành phần × buổi học, trong tab "Điểm" của
session-detail-panel (classbook).

Nguồn contract: `plans/reports/brainstorm-260829-2114-class-score-config.md`
(quyết định đã chốt: chặn re-apply khi đã có điểm; key điểm =
(class, session, student, component); owner gate = `Scope.IsOwner`, không
thêm perm key).

## Goals

| # | Goal | Priority |
|---|------|----------|
| 1 | Schema grading 4 bảng, migration 000014 up/down sạch | P1 |
| 2 | API `grading`: score-set CRUD (owner) + apply snapshot cho lớp | P1 |
| 3 | API điểm thành phần theo buổi: GET/PUT, teacher-của-lớp + owner | P1 |
| 4 | Web: trang cấu hình owner-only + entry sidebar "Trung tâm" | P1 |
| 5 | Web: nhập điểm thành phần trong session-detail-panel | P1 |

## Phases

| # | Phase | Status | Depends on |
|---|-------|--------|------------|
| 1 | [Grading schema (migration 000014)](./phase-01-grading-schema.md) | Done | — |
| 2 | [Score-set API (owner CRUD + apply)](./phase-02-score-set-api.md) | Done | 1 |
| 3 | [Student-scores API (GET/PUT theo buổi)](./phase-03-student-scores-api.md) | Done | 1, 2 |
| 4 | [Web owner config page](./phase-04-web-owner-config.md) | Done | 2 |
| 5 | [Web score entry (classbook)](./phase-05-web-score-entry.md) | Done | 3 |

## Key decisions

1. **Snapshot 2 tầng:** `score_sets` + `score_set_components` là template cấp
   trung tâm; `class_score_components` là bản copy per lớp lúc gán
   (`source_set_id` nullable chỉ để truy vết). Sửa/xóa bộ gốc không ảnh
   hưởng lớp đã gán.
2. **Chặn re-apply khi có điểm:** tồn tại ≥1 dòng `student_scores` của lớp
   (kể cả buổi đã qua) ⇒ apply/clear trả 409 + message rõ; UI disable kèm
   giải thích.
3. **Owner gate = `Scope.IsOwner` trong service/handler**, không perm key
   mới. Repo chỉ scope qua `sc.CenterWide()` (guard test
   `scoping_guard_test.go` cấm `sc.IsOwner`/`.Has(` trong repository.go).
4. **Divergence có chủ đích với teaching:** `teaching.PutMarks` là
   session-teacher-only (owner không ghi hộ); grading cho phép **owner ghi
   điểm thành phần** theo contract brainstorm (AC4). Comment tại gate phải
   nêu rõ đây là chủ đích, không copy nhầm pattern teaching.
5. **UI thay thế khi có components (user chốt, validation 260829):** lớp
   có `class_score_components` ⇒ tab "scores" render grid điểm thành phần
   **thay cho** input điểm chung; lớp không có components giữ nguyên UI
   hiện tại (zero change). SessionMark model/API không đổi — chỉ ẩn lối
   nhập ở lớp dùng components. Trade-off user đã chấp nhận: lớp dùng
   components sẽ không có điểm chung mới ⇒ score-bar-chart/month-marks/báo
   cáo phụ huynh không nhận thêm dữ liệu từ các lớp đó; đưa điểm thành
   phần vào chart/báo cáo là follow-up (non-goal phase này). UI kèm helper
   text "điểm thành phần chưa vào báo cáo phụ huynh".
6. **Web đặt trong feature `center`** (trang config) + feature `teaching`
   (nhập điểm); không mở web feature `grading` mới cho 1 page + 1 section.
7. **Audit registry bắt buộc:** audit là route-based (`audit/action.go`);
   mọi route grading mới phải đăng ký action + test capture. Đây là premise
   của quyết định không thêm cột `entered_by` — dấu vết owner ghi hộ nằm
   trong audit log.
8. **Composite-FK tenancy (invariant 000009):** mọi FK về bảng cha kèm
   `center_id`; guard `(teacher_id, center_id) → center_members ON DELETE
   CASCADE` — hệ quả: remove teacher khỏi center xóa điểm thành phần của
   teacher đó (kể cả dòng owner ghi hộ), nhất quán session_marks, chấp
   nhận có chủ đích.

## Success Criteria (acceptance từ brainstorm)

- [x] Sidebar "Trung tâm" có entry owner-only mở trang cấu hình lớp học; ẩn
      với member.
- [x] Owner CRUD bộ điểm; tên thành phần trong 1 bộ không trùng.
- [x] Gán bộ cho lớp = snapshot; sửa/xóa bộ gốc không đổi thành phần lớp.
- [x] Teacher của lớp + owner nhập/sửa điểm 0–10 (1 lẻ) per học sinh ×
      thành phần × buổi; member khác 403.
- [x] Lớp đã có ≥1 điểm ⇒ API 409 khi gán bộ khác; UI báo rõ.
- [x] Migration 000014 up/down sạch; integration test repo + handler; web
      test cho page/section mới.

## Open questions

None — chốt qua advisory review (kongming) + validation interview (user,
260829). Xem `## Validation Log`.

## Validation Log

### Verification Results (260829)
- Claims checked: ~20 (kongming advisory + fact-check pass)
- Verified: all | Failed: 0 | Unverified: 0
- Tier: Full (5 phases)
- Evidence chính: composite-FK invariant + guard center_members
  (`000009_teaching.up.sql:7-9,29-30,92-94`), audit route-based
  (`audit/action.go`, `capture_integration_test.go`), classes center-wide
  qua `CenterWide()` (`classes/repository.go:73,82,174`), guard test
  (`scoping_guard_test.go:33`), embed glob `*.sql` (`migrations/embed.go:9`).

### Interview Decisions (user, 260829)
1. **Cascade chấp nhận:** guard `(teacher_id, center_id) → center_members
   ON DELETE CASCADE` — remove teacher xóa điểm thành phần của teacher đó,
   nhất quán session_marks.
2. **Step nhập điểm = 0.5 cả hai:** grid điểm thành phần dùng
   `step={0.5}` như input điểm chung hiện tại (không nâng lên 0.1). DB/API
   giữ NUMERIC(4,1), validate 0–10 tối đa 1 chữ số thập phân — mirror đúng
   session_marks; 0.5 chỉ là bước UI.
3. **Attribution qua audit log:** không thêm cột `entered_by`; đăng ký
   route grading vào `audit/action.go` là bắt buộc (decision 7).
4. **UI thay thế:** grid điểm thành phần thay input điểm chung khi lớp có
   components (đảo đề xuất "sống cạnh" ban đầu — decision 5 đã cập nhật).

### Whole-Plan Consistency Sweep (260829)
Đã rà toàn bộ plan.md + phase-01..05 sau propagation: không còn thuật ngữ
"sống cạnh"/step 0.1 ngoài ngữ cảnh lịch sử; phase 3 làm rõ 0.1 là
precision API/DB còn 0.5 là step UI; phase 5 cập nhật replace-mode ở
overview/requirements/architecture/steps/tests/risks. Không còn mâu thuẫn
chưa giải quyết.

<!-- slug: class-score-config -->
