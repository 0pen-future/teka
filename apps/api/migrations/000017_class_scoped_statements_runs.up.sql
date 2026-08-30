-- Statement theo lớp cho đường học vụ + notification run song song per lớp.
--
-- statements.class_id / notification_runs.class_id:
--   NULL     = bản GIA ĐÌNH / run center-wide — đơn vị hiện tại, hành vi cũ
--              giữ nguyên từng bit (index partial WHERE class_id IS NULL phủ
--              đúng tập rows cũ).
--   NOT NULL = bản THEO LỚP — học vụ generate/gửi statement chỉ chứa phí của
--              lớp mình được gán; run học vụ stamp lớp để hai học vụ hai lớp
--              cùng kỳ gửi song song không đâm unique nào.

ALTER TABLE statements ADD COLUMN class_id UUID;
-- Composite FK theo center: một statement không bao giờ trỏ sang lớp của
-- center khác. NO ACTION thay vì CASCADE: xoá lớp không được lặng lẽ xoá
-- chứng từ công nợ đã phát hành (lớp soft-delete là đường chính, hard delete
-- lớp còn statement là lỗi vận hành cần nổ FK).
ALTER TABLE statements
    ADD CONSTRAINT fk_statements_class_center
    FOREIGN KEY (class_id, center_id) REFERENCES classes(id, center_id);

-- Re-key unique gia đình: thêm chiều class_id IS NULL, nếu không bản lớp đầu
-- tiên của một contact/kỳ sẽ đâm vào unique gia đình đang có.
DROP INDEX uq_statements;
CREATE UNIQUE INDEX uq_statements
    ON statements(contact_id, period_id)
    WHERE class_id IS NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX uq_statements_class
    ON statements(contact_id, period_id, class_id)
    WHERE class_id IS NOT NULL AND deleted_at IS NULL;

ALTER TABLE notification_runs ADD COLUMN class_id UUID;
ALTER TABLE notification_runs
    ADD CONSTRAINT fk_notification_runs_class_center
    FOREIGN KEY (class_id, center_id) REFERENCES classes(id, center_id);

-- Một active run per kỳ (center-wide) VÀ một active run per (kỳ, lớp): run
-- học vụ hai lớp khác nhau cùng kỳ chạy song song; hai học vụ CÙNG lớp cùng
-- kỳ vẫn 409 như cũ. Reconciler xử lý run kẹt per-row, không phụ thuộc index.
DROP INDEX uq_notification_runs_one_active_period;
CREATE UNIQUE INDEX uq_notification_runs_one_active_period
    ON notification_runs(billing_period_id)
    WHERE status = 'running' AND class_id IS NULL;
CREATE UNIQUE INDEX uq_notification_runs_one_active_period_class
    ON notification_runs(billing_period_id, class_id)
    WHERE status = 'running' AND class_id IS NOT NULL;
