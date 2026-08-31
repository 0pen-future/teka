-- The delegated send-reports permission now lives solely in the permission
-- assignment tables as reports.send; the dual-written legacy column retires.
-- Effective access is unchanged: every live can_send_reports = TRUE stint has
-- carried a mirrored reports.send grant row since the dual-write shipped.
ALTER TABLE center_members DROP COLUMN can_send_reports;
