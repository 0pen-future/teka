-- Bản lớp là dữ liệu chỉ tồn tại nhờ cột class_id: down xoá hẳn các rows
-- class-scoped rồi trả schema về dạng cũ. Rows gia đình (class_id IS NULL)
-- không bị đụng tới.

DELETE FROM statements WHERE class_id IS NOT NULL;
DROP INDEX uq_statements_class;
DROP INDEX uq_statements;
CREATE UNIQUE INDEX uq_statements
    ON statements(contact_id, period_id) WHERE deleted_at IS NULL;
ALTER TABLE statements DROP CONSTRAINT fk_statements_class_center;
ALTER TABLE statements DROP COLUMN class_id;

DELETE FROM notification_runs WHERE class_id IS NOT NULL;
DROP INDEX uq_notification_runs_one_active_period_class;
DROP INDEX uq_notification_runs_one_active_period;
CREATE UNIQUE INDEX uq_notification_runs_one_active_period
    ON notification_runs(billing_period_id)
    WHERE status = 'running';
ALTER TABLE notification_runs DROP CONSTRAINT fk_notification_runs_class_center;
ALTER TABLE notification_runs DROP COLUMN class_id;
