-- Restores the original baseline predicate (status = 'held' only).
DROP INDEX idx_class_sessions_pending;
CREATE INDEX idx_class_sessions_pending
    ON class_sessions(teacher_id, session_date)
    WHERE status = 'held' AND attendance_confirmed_at IS NULL AND deleted_at IS NULL;
