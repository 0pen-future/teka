ALTER TABLE program_template_versions DROP COLUMN IF EXISTS score_set;

DROP TABLE IF EXISTS template_log_fields;
DROP TABLE IF EXISTS template_lesson_exercises;
DROP TABLE IF EXISTS template_lesson_materials;
DROP TABLE IF EXISTS library_exercises;
DROP TABLE IF EXISTS library_materials;
