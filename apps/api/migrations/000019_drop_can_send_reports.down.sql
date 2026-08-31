-- Restore the legacy column and rebuild it from the reports.send override
-- rows — the exact source the dual-write kept in lockstep with the column.
-- Only live stints are flagged: closing a membership always reset the column
-- to FALSE and deleted the stint's override rows.
ALTER TABLE center_members ADD COLUMN can_send_reports BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE center_members cm
SET can_send_reports = TRUE
FROM center_member_permissions mp
WHERE mp.teacher_id = cm.teacher_id
  AND mp.center_id = cm.center_id
  AND mp.permission_key = 'reports.send'
  AND mp.allowed
  AND cm.left_at IS NULL;
