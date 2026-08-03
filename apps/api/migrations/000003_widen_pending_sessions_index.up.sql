-- Widens idx_class_sessions_pending to also cover the 'planned' half of the
-- pending-attendance predicate: a session whose date has passed but was
-- never explicitly marked held is exactly the case the R2 warning feed
-- exists for, and the original baseline index only covered status='held'.
DROP INDEX idx_class_sessions_pending;
CREATE INDEX idx_class_sessions_pending
    ON class_sessions(teacher_id, session_date)
    WHERE status IN ('held', 'planned') AND attendance_confirmed_at IS NULL AND deleted_at IS NULL;
