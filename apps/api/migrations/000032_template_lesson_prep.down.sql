-- Gỡ chỉ mục, ràng buộc và bốn cột chuẩn bị tài liệu khỏi template_lessons.
DROP INDEX IF EXISTS idx_template_lessons_assignee;
ALTER TABLE template_lessons
    DROP CONSTRAINT IF EXISTS fk_template_lessons_assignee,
    DROP COLUMN IF EXISTS checklist,
    DROP COLUMN IF EXISTS due_date,
    DROP COLUMN IF EXISTS assignee_id,
    DROP COLUMN IF EXISTS prep_status;
