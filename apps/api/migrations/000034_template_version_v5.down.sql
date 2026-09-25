-- Down mất mọi bộ điểm ngoài bộ đầu tiên (data loss có chủ đích, xem up.sql).
UPDATE program_template_versions
SET score_set = COALESCE(score_set -> 0 -> 'components', '[]'::jsonb)
WHERE jsonb_typeof(score_set) = 'array' AND jsonb_array_length(score_set) > 0
  AND (score_set -> 0 ? 'components');

ALTER TABLE template_log_fields DROP CONSTRAINT template_log_fields_kind_check;
UPDATE template_log_fields SET kind = 'text' WHERE kind IN ('long_text', 'student');
ALTER TABLE template_log_fields ADD CONSTRAINT template_log_fields_kind_check
    CHECK (kind IN ('text', 'number', 'select', 'checkbox'));

ALTER TABLE template_lesson_exercises DROP CONSTRAINT fk_template_lesson_exercises_group_center, DROP COLUMN group_id;
DROP TABLE template_exercise_groups;

ALTER TABLE template_lessons DROP COLUMN unit, DROP COLUMN mode;
