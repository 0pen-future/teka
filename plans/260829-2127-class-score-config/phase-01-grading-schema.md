---
phase: 1
title: "Grading schema (migration 000014)"
status: done
priority: P1
effort: "0.5d"
dependencies: []
---

# Phase 1: Grading schema (migration 000014)

## Overview

Migration `000014_grading` tạo 4 bảng cho bộ điểm template + snapshot per lớp
+ điểm học sinh theo buổi. Không đụng bảng hiện có.

## Requirements

- Functional: 4 bảng dưới đây, up/down sạch, FK/index theo pattern
  `000009_teaching.up.sql`.
- Non-functional: không sửa migration cũ; embed qua `embed.go` tự động
  (glob); `migrations_test.go` phải pass.

## Architecture

Đối chiếu tên bảng/FK thực tế trong `000009_teaching.up.sql` và
`000001_baseline_schema.up.sql` (classes, class_sessions, students, centers)
trước khi viết — không đoán tên.

```sql
-- Bộ điểm template cấp trung tâm (soft delete kiểu gorm deleted_at).
CREATE TABLE score_sets (
    id         UUID PRIMARY KEY,
    center_id  UUID NOT NULL REFERENCES centers(id) ON DELETE CASCADE,
    name       VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
-- Tên bộ không trùng trong 1 trung tâm giữa các bộ còn sống.
CREATE UNIQUE INDEX score_sets_center_name_live
    ON score_sets (center_id, lower(name)) WHERE deleted_at IS NULL;

-- Thành phần của bộ: chỉ tên + thứ tự. Sửa bộ = replace toàn bộ danh sách
-- (hard delete + insert) — snapshot per lớp không bị ảnh hưởng.
CREATE TABLE score_set_components (
    id       UUID PRIMARY KEY,
    set_id   UUID NOT NULL REFERENCES score_sets(id) ON DELETE CASCADE,
    name     VARCHAR(50) NOT NULL,
    position SMALLINT NOT NULL,
    UNIQUE (set_id, name),
    UNIQUE (set_id, position)
);

-- Bản snapshot per lớp lúc gán. source_set_id chỉ truy vết, không ràng buộc
-- ngữ nghĩa (SET NULL khi bộ gốc bị xóa cứng — bộ gốc soft-delete nên
-- thực tế hiếm).
CREATE TABLE class_score_components (
    id            UUID PRIMARY KEY,
    class_id      UUID NOT NULL,
    center_id     UUID NOT NULL,
    name          VARCHAR(50) NOT NULL,
    position      SMALLINT NOT NULL,
    source_set_id UUID REFERENCES score_sets(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Composite FK theo invariant tenancy 000009: con trỏ về cha phải kèm
    -- center_id để dòng không bao giờ trỏ chéo trung tâm.
    FOREIGN KEY (class_id, center_id)
        REFERENCES classes(id, center_id) ON DELETE CASCADE,
    UNIQUE (class_id, name),
    UNIQUE (class_id, position),
    -- Cho student_scores FK composite (component phải thuộc đúng lớp).
    UNIQUE (id, class_id)
);

-- 1 điểm / thành phần / học sinh / buổi. teacher_id + center_id là anchor
-- tenancy/own-rows như session_marks. Score 0–10, 1 chữ số thập phân.
CREATE TABLE student_scores (
    id           UUID PRIMARY KEY,
    class_id     UUID NOT NULL,
    session_id   UUID NOT NULL,
    component_id UUID NOT NULL,
    student_id   UUID NOT NULL,
    teacher_id   UUID NOT NULL,
    center_id    UUID NOT NULL,
    score        NUMERIC(4,1) NOT NULL CHECK (score >= 0 AND score <= 10),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Composite FK tenancy như session_marks (000009:92-94).
    FOREIGN KEY (class_id, center_id)
        REFERENCES classes(id, center_id) ON DELETE CASCADE,
    FOREIGN KEY (session_id, center_id)
        REFERENCES class_sessions(id, center_id) ON DELETE CASCADE,
    FOREIGN KEY (student_id, center_id)
        REFERENCES students(id, center_id) ON DELETE CASCADE,
    -- Component phải thuộc đúng lớp của dòng điểm — DB tự chặn component
    -- lớp A gắn vào điểm lớp B.
    FOREIGN KEY (component_id, class_id)
        REFERENCES class_score_components(id, class_id) ON DELETE CASCADE,
    -- Guard membership như session_marks: remove teacher khỏi center
    -- CASCADE xóa điểm của teacher đó (kể cả dòng owner ghi hộ) — nhất
    -- quán với session_marks, trade-off chấp nhận có chủ đích.
    FOREIGN KEY (teacher_id, center_id)
        REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE,
    UNIQUE (session_id, component_id, student_id)
);
-- Index phục vụ "lớp đã có điểm?" và đọc điểm theo buổi.
CREATE INDEX student_scores_class ON student_scores (class_id);
```

Lưu ý: `class_sessions` không có `UNIQUE (id, class_id)` nên "session thuộc
đúng class" không ép được bằng FK — validate ở service (phase 3 đã có).
Trước khi viết vẫn đối chiếu 000009 để khớp chính xác tên cột/constraint
của composite FK (không đoán).

## Related Code Files

- Create: `apps/api/migrations/000014_grading.up.sql`
- Create: `apps/api/migrations/000014_grading.down.sql`

## Implementation Steps

1. Soi `000009_teaching.up.sql` + `000001_baseline_schema.up.sql` để khớp
   tên bảng, kiểu cột, style comment tiếng Việt, FK teachers.
2. Viết up theo SQL trên (điều chỉnh theo bước 1); down drop 4 bảng theo
   thứ tự ngược.
3. `make test-api` (hoặc chạy riêng `migrations_test.go`) xác nhận up/down.

## Success Criteria

- [ ] `migrations_test.go` pass (up hết rồi down hết sạch).
- [ ] Unique `(session_id, component_id, student_id)` và CHECK 0–10 có mặt.
- [ ] Mọi FK về bảng cha tenancy là composite kèm `center_id` (invariant
      000009); guard `(teacher_id, center_id) → center_members` có mặt.
- [ ] Không migration cũ nào bị sửa.

## Risk Assessment

- Tên bảng/FK đoán sai → bước 1 bắt buộc đối chiếu file thật.
- Cascade theo teacher guard: remove teacher khỏi center xóa điểm thành
  phần của teacher đó — ngữ nghĩa session_marks hiện có, đã chấp nhận;
  không được "sửa" thành RESTRICT mà không hỏi user.
- `lower(name)` unique index có thể lệch style repo → nếu 000013 không dùng
  functional index, hạ xuống UNIQUE (center_id, name) WHERE deleted_at IS
  NULL cho đồng bộ.
