-- Rows already sent over zalo_personal must survive the rollback; zalo_manual
-- is the closest older meaning (a teacher-sent personal message), so they fold
-- into it before the old CHECK is restored.
ALTER TABLE notifications
    DROP COLUMN run_id;
DROP TABLE notification_runs;

UPDATE notifications SET channel = 'zalo_manual' WHERE channel = 'zalo_personal';
ALTER TABLE notifications
    DROP CONSTRAINT notifications_channel_check;
ALTER TABLE notifications
    ADD CONSTRAINT notifications_channel_check
        CHECK (channel IN ('zalo_zns', 'zalo_manual', 'sms'));

ALTER TABLE contacts
    DROP COLUMN zalo_user_id,
    DROP COLUMN zalo_name;
