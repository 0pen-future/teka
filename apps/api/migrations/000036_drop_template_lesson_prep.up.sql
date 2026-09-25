-- Gỡ bỏ tính năng chuẩn bị tài liệu (lesson prep) mà 000032 đã thêm: bảng
-- Kanban 4 cột cố định, người được phân công, hạn hoàn thành và checklist
-- trên mỗi buổi học mẫu không còn được dùng.
DELETE FROM center_role_permissions
WHERE permission_key = 'prep.assign';

DELETE FROM center_member_permissions
WHERE permission_key = 'prep.assign';

DROP INDEX IF EXISTS idx_template_lessons_assignee;
ALTER TABLE template_lessons
    DROP CONSTRAINT IF EXISTS fk_template_lessons_assignee,
    DROP COLUMN IF EXISTS checklist,
    DROP COLUMN IF EXISTS due_date,
    DROP COLUMN IF EXISTS assignee_id,
    DROP COLUMN IF EXISTS prep_status;
