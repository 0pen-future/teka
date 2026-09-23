---
phase: 4
title: "Kho học liệu — Học liệu, Bài tập, Trường nhật ký, Bộ điểm"
status: pending
priority: P2
effort: "2d"
dependencies: [3]
---

# Phase 4: Kho học liệu — Học liệu, Bài tập, Trường nhật ký, Bộ điểm

> Outline (deep mode). **Scout lại**: Phase 3 `library/` sau khi merge; `teaching` marks/score để không trùng bộ điểm.

## Goal
Bổ sung vào phiên bản chương trình mẫu: học liệu (link/tên), bài tập, trường nhật ký buổi (log fields) và bộ điểm;
gắn học liệu/bài tập vào từng buổi mẫu với cờ chia sẻ cho học viên.

## Key decisions
- Học liệu là **link + mô tả**, không upload file (không có object storage trong repo — non-goal ghi rõ).
- Gắn buổi↔học liệu/bài tập là bảng nối, ghi đè toàn bộ bằng `PUT` (idempotent, đơn giản cho UI checkbox).
- `log_fields` và `score_set` gắn ở **phiên bản** (cùng lock publish).
- Không key mới: đọc `library.read`, ghi `library.edit` (optIn — kể cả `score-set`). <!-- Red Team S1 F6 -->

## Migration sketch — `000028_library_items` <!-- Red Team S1 F5, F8 -->
```sql
library_materials (id PK, center_id NOT NULL, title, kind CHECK (kind IN ('link','doc','video','other')), url TEXT, description, tags JSONB,
  created_at, updated_at, deleted_at)  UNIQUE (id, center_id)
library_exercises (id PK, center_id NOT NULL, title, description, difficulty SMALLINT, tags JSONB, created_at, updated_at, deleted_at)  UNIQUE (id, center_id)
template_lesson_materials (lesson_id, material_id, center_id NOT NULL, shared_with_students BOOL DEFAULT false, position INT,
  PRIMARY KEY (lesson_id, material_id),
  FOREIGN KEY (lesson_id, center_id)   REFERENCES template_lessons (id, center_id),
  FOREIGN KEY (material_id, center_id) REFERENCES library_materials (id, center_id))
template_lesson_exercises (lesson_id, exercise_id, center_id NOT NULL, position INT, PRIMARY KEY (lesson_id, exercise_id),
  FOREIGN KEY (lesson_id, center_id)   REFERENCES template_lessons (id, center_id),
  FOREIGN KEY (exercise_id, center_id) REFERENCES library_exercises (id, center_id))
template_log_fields (id PK, version_id NOT NULL, center_id NOT NULL, position INT, label, kind CHECK (kind IN ('text','number','select','checkbox')),
  options JSONB, required BOOL, FOREIGN KEY (version_id, center_id) REFERENCES program_template_versions (id, center_id),
  UNIQUE (version_id, position) DEFERRABLE INITIALLY DEFERRED)
ALTER TABLE program_template_versions ADD COLUMN score_set JSONB NOT NULL DEFAULT '[]'; -- [{key,label,max,weight}]
-- down: DROP COLUMN score_set; DROP 5 bảng theo thứ tự ngược
```
Không backfill quyền (không key mới).

## API (thêm vào `library`)
- `GET/POST /library/materials`, `GET/PUT/DELETE /library/materials/:id`; tương tự `/library/exercises`.
- `PUT /library/lessons/:lid/materials` body `[{material_id, shared_with_students}]`; `PUT /library/lessons/:lid/exercises`.
- `PUT /library/versions/:vid/log-fields`, `PUT /library/versions/:vid/score-set` (`library.edit`).
- `GET /library/versions/:vid` trả đầy đủ lessons + materials + exercises (một payload cho Phase 7 read-through).
- Mọi route mutating khai `req(action, entity, id)` (entity `library_material`, `library_exercise`, `template_lesson`,
  `template_version`); `route_policy_snapshot_test.go` (+12), `audit/action_test.go` (+10). <!-- Red Team S1 F7 -->
- Service kiểm `material_id`/`exercise_id` cùng `center_id` với lesson trước khi ghi bảng nối (FK composite là hàng rào thứ hai).

## Web (feature `library`)
- Tab Học liệu / Bài tập trong `library-page.tsx`: bảng + dialog tạo/sửa (gate `has("library.edit")`).
- `template-lesson-page.tsx`: hai khối chọn học liệu/bài tập (`HvSelect` multi hoặc checklist), cờ chia sẻ.
- `template-detail-page.tsx`: tab phụ "Nhật ký & Điểm" (log fields builder, score set editor).

## Verification
- Integration: PUT nối idempotent; material khác trung tâm → 422; xoá material đang gắn → 409 `MATERIAL_IN_USE`.
- Migration 000028 up/down/up sạch.
- Vitest cho dialog + tab; cập nhật e2e `library.spec.ts`.

## Risks
- Score set JSONB vs bảng — chọn JSONB vì chỉ đọc nguyên khối; nếu Phase 7 cần chấm theo key thì vẫn đủ.
