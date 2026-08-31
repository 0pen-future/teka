-- Restore the legacy column and rebuild it from the effective reports.send
-- verdict — (role grant ∪ member grant) − member deny — not just the member
-- override rows: role-level reports.send grants became possible once the
-- catalog stopped blocking them, so a member-rows-only rebuild would silently
-- strip every role-granted sender on rollback. Only live stints are flagged:
-- closing a membership always reset the column to FALSE and deleted the
-- stint's override rows.
ALTER TABLE center_members ADD COLUMN can_send_reports BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE center_members cm
SET can_send_reports = TRUE
WHERE cm.left_at IS NULL
  AND (
    EXISTS (
      SELECT 1 FROM center_member_permissions mp
      WHERE mp.teacher_id = cm.teacher_id
        AND mp.center_id = cm.center_id
        AND mp.permission_key = 'reports.send'
        AND mp.allowed
    )
    OR (
      EXISTS (
        SELECT 1 FROM center_role_permissions rp
        WHERE rp.role_id = cm.role_id
          AND rp.permission_key = 'reports.send'
      )
      AND NOT EXISTS (
        SELECT 1 FROM center_member_permissions mp
        WHERE mp.teacher_id = cm.teacher_id
          AND mp.center_id = cm.center_id
          AND mp.permission_key = 'reports.send'
          AND NOT mp.allowed
      )
    )
  );
