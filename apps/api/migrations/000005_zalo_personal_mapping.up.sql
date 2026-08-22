-- Teachers send statement notifications as DMs from their own Zalo account.
-- That needs three things: contacts must remember which Zalo friend they are,
-- notifications must accept the new channel, and a batch of paced sends must
-- survive a restart as one durable run.

-- Which Zalo friend this contact is, chosen by the teacher in the picker.
-- zalo_name is the friend's name at mapping time so lists render without
-- refetching the live friend list. No FK to zalo_accounts: the mapping stays
-- meaningful across unlink/relink of the same account.
ALTER TABLE contacts
    ADD COLUMN zalo_user_id VARCHAR(32),
    ADD COLUMN zalo_name    VARCHAR(100);

-- One Zalo friend maps to at most one live contact per teacher. A duplicate
-- mapping would send one person the statement links — and the debt data — of
-- two families. Partial like uq_contacts_phone: per-teacher, and a deleted
-- contact releases its friend.
CREATE UNIQUE INDEX uq_contacts_zalo_user
    ON contacts(teacher_id, zalo_user_id)
    WHERE zalo_user_id IS NOT NULL AND deleted_at IS NULL;

ALTER TABLE notifications
    DROP CONSTRAINT notifications_channel_check;
ALTER TABLE notifications
    ADD CONSTRAINT notifications_channel_check
        CHECK (channel IN ('zalo_zns', 'zalo_manual', 'sms', 'zalo_personal'));

-- One paced sending batch. Counters (total/sent/failed) are always derived by
-- counting notifications per run_id — storing them here would be a second
-- source of truth that drifts. status semantics: 'interrupted' means the run
-- stopped with rows still queued — the process died, or sending was cut short
-- after repeated failures — and the teacher may resume manually; 'expired'
-- means the Zalo session died mid-run (remaining rows are failed).
CREATE TABLE notification_runs (
    id                 UUID PRIMARY KEY,
    teacher_id         UUID        NOT NULL REFERENCES teachers(id) ON DELETE CASCADE,
    billing_period_id  UUID        NOT NULL,
    purpose            VARCHAR(20) NOT NULL DEFAULT 'statements'
                           CHECK (purpose IN ('statements', 'reminder')),
    status             VARCHAR(20) NOT NULL DEFAULT 'running'
                           CHECK (status IN ('running', 'completed', 'interrupted', 'expired')),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at        TIMESTAMPTZ,
    FOREIGN KEY (billing_period_id, teacher_id)
        REFERENCES billing_periods(id, teacher_id) ON DELETE CASCADE,
    CONSTRAINT uq_notification_runs_tid UNIQUE (id, teacher_id)
);
CREATE INDEX idx_notification_runs_teacher ON notification_runs(teacher_id);

-- A notification row optionally belongs to a run. SET NULL, not CASCADE: the
-- notification is the audit record of a message that reached (or failed to
-- reach) a parent, and it outlives its batch. The FK is composite over
-- (run_id, teacher_id) so the database itself refuses a notification pointing
-- at another teacher's run — run progress is derived by counting these rows,
-- and a cross-tenant link would silently mix tenants' numbers. The SET NULL
-- column list keeps teacher_id intact when the run goes away (needs PG >= 15).
ALTER TABLE notifications
    ADD COLUMN run_id UUID;
ALTER TABLE notifications
    ADD FOREIGN KEY (run_id, teacher_id)
        REFERENCES notification_runs(id, teacher_id) ON DELETE SET NULL (run_id);
CREATE INDEX idx_notifications_run ON notifications(run_id) WHERE run_id IS NOT NULL;
