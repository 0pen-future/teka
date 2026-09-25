-- Dựng lại đúng cấu trúc cột/FK/index mà 000032 đã thêm, để binary cũ chạy
-- lại được. Dữ liệu chuẩn bị tài liệu (trạng thái, người được phân công,
-- hạn hoàn thành, checklist) và quyền prep.assign đã xóa ở up thì KHÔNG
-- được khôi phục — cột mới sinh ra sẽ mang giá trị mặc định.
ALTER TABLE template_lessons
    ADD COLUMN prep_status VARCHAR(10) NOT NULL DEFAULT 'todo'
        CHECK (prep_status IN ('todo', 'doing', 'review', 'done')),
    ADD COLUMN assignee_id UUID,
    ADD COLUMN due_date DATE,
    ADD COLUMN checklist JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD CONSTRAINT fk_template_lessons_assignee
        FOREIGN KEY (assignee_id, center_id) REFERENCES center_members (teacher_id, center_id) ON DELETE SET NULL (assignee_id);

CREATE INDEX idx_template_lessons_assignee ON template_lessons (assignee_id) WHERE assignee_id IS NOT NULL;
