-- v5 của chương trình mẫu: buổi học mang mode/unit, bài tập theo nhóm
-- (template_exercise_groups, FK composite theo khuôn 000027/000028), 2 loại
-- trường nhật ký mới, và bộ điểm chuyển từ một mảng phẳng thành mảng các bộ
-- ({key, title, components}) để một phiên bản có thể mang nhiều bộ điểm.
--
-- Bọc score_set chỉ áp dụng cho hàng còn ở hình dạng phẳng (phần tử đầu
-- không có "components"); down chỉ giữ lại bộ đầu tiên, các bộ điểm khác
-- (nếu người dùng đã thêm sau khi lên v5) sẽ mất khi hạ cấp — đây là hành vi
-- có chủ đích, không phải lỗi.
ALTER TABLE template_lessons
    ADD COLUMN mode VARCHAR(12) NOT NULL DEFAULT 'scheduled'
        CHECK (mode IN ('scheduled', 'self_study')),
    ADD COLUMN unit VARCHAR(100);

CREATE TABLE template_exercise_groups (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    version_id UUID NOT NULL,
    center_id  UUID NOT NULL,
    name       VARCHAR(100) NOT NULL,
    position   INT NOT NULL CHECK (position > 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (id, center_id),
    UNIQUE (version_id, position) DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT fk_template_exercise_groups_version_center
        FOREIGN KEY (version_id, center_id) REFERENCES program_template_versions (id, center_id) ON DELETE CASCADE
);
CREATE INDEX idx_template_exercise_groups_version ON template_exercise_groups (version_id);

ALTER TABLE template_lesson_exercises
    ADD COLUMN group_id UUID,
    ADD CONSTRAINT fk_template_lesson_exercises_group_center
        FOREIGN KEY (group_id, center_id) REFERENCES template_exercise_groups (id, center_id) ON DELETE SET NULL (group_id);

-- Nhật ký: thêm 2 loại v5, giữ 2 loại cũ.
ALTER TABLE template_log_fields DROP CONSTRAINT template_log_fields_kind_check;
ALTER TABLE template_log_fields ADD CONSTRAINT template_log_fields_kind_check
    CHECK (kind IN ('text', 'long_text', 'checkbox', 'student', 'number', 'select'));

-- Bộ điểm: mảng phẳng [{key,label,max,weight}] -> mảng bộ [{key,title,components}].
-- Chỉ bọc khi phần tử đầu chưa có "components" (idempotent nếu chạy lại).
UPDATE program_template_versions
SET score_set = jsonb_build_array(jsonb_build_object(
        'key', 'main', 'title', 'Bộ điểm', 'components', score_set))
WHERE jsonb_typeof(score_set) = 'array'
  AND jsonb_array_length(score_set) > 0
  AND NOT (score_set -> 0 ? 'components');
UPDATE program_template_versions SET score_set = '[]'::jsonb
WHERE score_set IS NULL OR jsonb_typeof(score_set) <> 'array';
