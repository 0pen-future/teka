-- =============================================================
-- 000014 — Bộ điểm thành phần: template cấp trung tâm, snapshot per lớp,
-- và điểm từng học sinh × thành phần × buổi.
--
-- Owner cấu hình "bộ điểm" đặt tên (VD IELTS: Listening/Speaking/Reading/
-- Writing) rồi gán cho lớp theo ngữ nghĩa SNAPSHOT: lúc gán copy thành phần
-- vào class_score_components, nên sửa/xóa bộ gốc không đụng lớp đã gán.
--
-- Theo đúng khuôn toàn vẹn 000007/000009: center_id NOT NULL, FK composite
-- về UNIQUE (id, center_id) của bảng cha, và FK guard (teacher_id, center_id)
-- → center_members trên bảng có điểm.
-- =============================================================

-- Bộ điểm template cấp trung tâm. Soft delete kiểu deleted_at: bộ có thể đã
-- gán vào lớp (dạng snapshot), xóa cứng mất truy vết source_set_id.
CREATE TABLE score_sets (
    id         UUID PRIMARY KEY,
    center_id  UUID         NOT NULL REFERENCES centers(id) ON DELETE CASCADE,
    name       VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ
);
-- Tên bộ không trùng (case-insensitive) giữa các bộ còn sống trong 1 trung tâm.
CREATE UNIQUE INDEX score_sets_center_name_live
    ON score_sets (center_id, lower(name)) WHERE deleted_at IS NULL;

-- Thành phần của bộ: chỉ tên + thứ tự. Sửa bộ = replace toàn bộ danh sách
-- (hard delete + insert) — snapshot per lớp đã copy giá trị nên không lệ thuộc.
CREATE TABLE score_set_components (
    id       UUID        PRIMARY KEY,
    set_id   UUID        NOT NULL REFERENCES score_sets(id) ON DELETE CASCADE,
    name     VARCHAR(50) NOT NULL,
    position SMALLINT    NOT NULL,
    UNIQUE (set_id, name),
    UNIQUE (set_id, position)
);

-- Bản snapshot per lớp lúc gán. source_set_id chỉ truy vết, không ràng buộc
-- ngữ nghĩa (SET NULL khi bộ gốc bị xóa cứng — bộ gốc soft-delete nên hiếm).
CREATE TABLE class_score_components (
    id            UUID        PRIMARY KEY,
    class_id      UUID        NOT NULL,
    center_id     UUID        NOT NULL,
    name          VARCHAR(50) NOT NULL,
    position      SMALLINT    NOT NULL,
    source_set_id UUID        REFERENCES score_sets(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Composite FK theo invariant tenancy 000007/000009: con trỏ về cha kèm
    -- center_id để dòng không bao giờ trỏ chéo trung tâm.
    FOREIGN KEY (class_id, center_id) REFERENCES classes(id, center_id) ON DELETE CASCADE,
    UNIQUE (class_id, name),
    UNIQUE (class_id, position),
    -- Target cho FK composite của student_scores (component phải thuộc đúng lớp).
    UNIQUE (id, class_id)
);

-- 1 điểm / thành phần / học sinh / buổi. teacher_id + center_id là anchor
-- tenancy/own-rows như session_marks. Score 0–10, tối đa 1 chữ số thập phân.
CREATE TABLE student_scores (
    id           UUID         PRIMARY KEY,
    class_id     UUID         NOT NULL,
    session_id   UUID         NOT NULL,
    component_id UUID         NOT NULL,
    student_id   UUID         NOT NULL,
    teacher_id   UUID         NOT NULL,
    center_id    UUID         NOT NULL,
    -- Thang 0–10; NUMERIC chính xác, không trôi float (mirror session_marks).
    score        NUMERIC(4,1) NOT NULL CHECK (score >= 0 AND score <= 10),
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    -- Composite FK tenancy như session_marks (000009).
    FOREIGN KEY (class_id, center_id)   REFERENCES classes(id, center_id)        ON DELETE CASCADE,
    FOREIGN KEY (session_id, center_id) REFERENCES class_sessions(id, center_id) ON DELETE CASCADE,
    FOREIGN KEY (student_id, center_id) REFERENCES students(id, center_id)       ON DELETE CASCADE,
    -- Component phải thuộc đúng lớp của dòng điểm — DB tự chặn component lớp A
    -- gắn vào điểm lớp B.
    FOREIGN KEY (component_id, class_id) REFERENCES class_score_components(id, class_id) ON DELETE CASCADE,
    -- Guard membership như session_marks: remove teacher khỏi center CASCADE
    -- xóa điểm của teacher đó (kể cả dòng owner ghi hộ) — nhất quán
    -- session_marks, trade-off chấp nhận có chủ đích.
    CONSTRAINT fk_student_scores_teacher_center FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE,
    CONSTRAINT uq_student_scores_session_component_student UNIQUE (session_id, component_id, student_id)
);
-- Phục vụ "lớp đã có điểm?" (guard chặn re-apply) và đọc điểm theo buổi.
CREATE INDEX student_scores_class ON student_scores (class_id);
