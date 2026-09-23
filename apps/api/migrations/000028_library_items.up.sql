-- =============================================================
-- 000028 — Kho học liệu: học liệu, bài tập, trường nhật ký, bộ điểm.
--
-- library_materials / library_exercises là tài sản chung của trung tâm,
-- xoá mềm (deleted_at) như program_templates; học liệu chỉ là link + mô tả,
-- không có upload file. Hai bảng nối gắn chúng vào từng buổi học mẫu và
-- được ghi đè toàn bộ mỗi lần lưu, nên khoá chính (lesson_id, material_id)
-- là đủ — không cần id riêng.
--
-- template_log_fields (trường nhật ký buổi) và cột score_set (bộ điểm)
-- thuộc phiên bản, nên cùng chịu khoá publish với buổi học: service giữ
-- row lock của phiên bản khi ghi. (version_id, position) DEFERRABLE để
-- ghi đè danh sách trong một transaction mà không đi qua vị trí tạm.
--
-- FK composite kèm center_id theo khuôn 000027: một dòng nối không bao giờ
-- trỏ chéo trung tâm; CASCADE chỉ chạy khi xoá cứng dòng cha. Xoá mềm học
-- liệu đang được gắn bị service chặn (409), nên CASCADE ở FK không phải là
-- đường xoá thường ngày.
-- =============================================================

CREATE TABLE library_materials (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    center_id   UUID         NOT NULL REFERENCES centers (id) ON DELETE CASCADE,
    title       VARCHAR(200) NOT NULL,
    kind        VARCHAR(10)  NOT NULL DEFAULT 'link'
        CHECK (kind IN ('link', 'doc', 'video', 'other')),
    url         TEXT,
    description TEXT,
    tags        JSONB        NOT NULL DEFAULT '[]',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ,
    UNIQUE (id, center_id)
);

CREATE INDEX idx_library_materials_center
    ON library_materials (center_id, title) WHERE deleted_at IS NULL;

CREATE TABLE library_exercises (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    center_id   UUID         NOT NULL REFERENCES centers (id) ON DELETE CASCADE,
    title       VARCHAR(200) NOT NULL,
    description TEXT,
    difficulty  SMALLINT     CHECK (difficulty BETWEEN 1 AND 5),
    tags        JSONB        NOT NULL DEFAULT '[]',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ,
    UNIQUE (id, center_id)
);

CREATE INDEX idx_library_exercises_center
    ON library_exercises (center_id, title) WHERE deleted_at IS NULL;

CREATE TABLE template_lesson_materials (
    lesson_id            UUID    NOT NULL,
    material_id          UUID    NOT NULL,
    center_id            UUID    NOT NULL,
    shared_with_students BOOLEAN NOT NULL DEFAULT FALSE,
    position             INT     NOT NULL CHECK (position > 0),
    PRIMARY KEY (lesson_id, material_id),
    CONSTRAINT fk_template_lesson_materials_lesson_center
        FOREIGN KEY (lesson_id, center_id) REFERENCES template_lessons (id, center_id)
        ON DELETE CASCADE,
    CONSTRAINT fk_template_lesson_materials_material_center
        FOREIGN KEY (material_id, center_id) REFERENCES library_materials (id, center_id)
        ON DELETE CASCADE
);

CREATE INDEX idx_template_lesson_materials_material
    ON template_lesson_materials (material_id);

CREATE TABLE template_lesson_exercises (
    lesson_id   UUID NOT NULL,
    exercise_id UUID NOT NULL,
    center_id   UUID NOT NULL,
    position    INT  NOT NULL CHECK (position > 0),
    PRIMARY KEY (lesson_id, exercise_id),
    CONSTRAINT fk_template_lesson_exercises_lesson_center
        FOREIGN KEY (lesson_id, center_id) REFERENCES template_lessons (id, center_id)
        ON DELETE CASCADE,
    CONSTRAINT fk_template_lesson_exercises_exercise_center
        FOREIGN KEY (exercise_id, center_id) REFERENCES library_exercises (id, center_id)
        ON DELETE CASCADE
);

CREATE INDEX idx_template_lesson_exercises_exercise
    ON template_lesson_exercises (exercise_id);

CREATE TABLE template_log_fields (
    id         UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    version_id UUID         NOT NULL,
    center_id  UUID         NOT NULL,
    position   INT          NOT NULL CHECK (position > 0),
    label      VARCHAR(100) NOT NULL,
    kind       VARCHAR(10)  NOT NULL
        CHECK (kind IN ('text', 'number', 'select', 'checkbox')),
    options    JSONB        NOT NULL DEFAULT '[]',
    required   BOOLEAN      NOT NULL DEFAULT FALSE,
    UNIQUE (id, center_id),
    UNIQUE (version_id, position) DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT fk_template_log_fields_version_center
        FOREIGN KEY (version_id, center_id) REFERENCES program_template_versions (id, center_id)
        ON DELETE CASCADE
);

-- Bộ điểm của phiên bản: [{key, label, max, weight}], đọc/ghi nguyên khối.
ALTER TABLE program_template_versions
    ADD COLUMN score_set JSONB NOT NULL DEFAULT '[]';
