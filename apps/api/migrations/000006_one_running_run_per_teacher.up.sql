-- The API keeps one sending pass per teacher with an in-process guard, but
-- that guard cannot see a second API instance (overlapping deploy, accidental
-- scale-out). Two concurrent passes would DM the same parents twice from the
-- teacher's personal account, so the database backstops the invariant itself.
CREATE UNIQUE INDEX uq_notification_runs_one_active
    ON notification_runs(teacher_id)
    WHERE status = 'running';
