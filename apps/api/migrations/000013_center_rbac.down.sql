-- An toàn để gỡ: cột can_send_reports vẫn là nguồn chính cho reports.send,
-- các phép gán vai trò/override khác đều dựng lại được từ UI.
ALTER TABLE center_members DROP COLUMN role_id;
DROP TABLE center_member_permissions;
DROP TABLE center_role_permissions;
DROP TABLE center_roles;
