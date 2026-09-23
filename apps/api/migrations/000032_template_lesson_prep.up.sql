-- ===== 000032 — Chuẩn bị tài liệu cho buổi học mẫu =====
-- Mỗi buổi mẫu trong bản nháp mang trạng thái chuẩn bị (bảng Kanban 4 cột
-- cố định), người được phân công, hạn hoàn thành và checklist việc cần làm.
-- assignee_id theo khuôn tasks (000022): FK composite kèm center_id vào
-- center_members để không gán chéo trung tâm, và ON DELETE SET NULL riêng cột
-- đó — gỡ thành viên chỉ bỏ phân công, không xoá nội dung buổi mẫu.
ALTER TABLE template_lessons
    ADD COLUMN prep_status VARCHAR(10) NOT NULL DEFAULT 'todo'
        CHECK (prep_status IN ('todo', 'doing', 'review', 'done')),
    ADD COLUMN assignee_id UUID,
    ADD COLUMN due_date DATE,
    ADD COLUMN checklist JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD CONSTRAINT fk_template_lessons_assignee
        FOREIGN KEY (assignee_id, center_id) REFERENCES center_members (teacher_id, center_id) ON DELETE SET NULL (assignee_id);

CREATE INDEX idx_template_lessons_assignee ON template_lessons (assignee_id) WHERE assignee_id IS NOT NULL;
