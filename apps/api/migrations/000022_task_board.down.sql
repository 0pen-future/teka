-- CẢNH BÁO: xoá KHÔNG điều kiện cả 5 khoá backfill mặc định lẫn 3 khoá
-- opt-in (tasks.manage_board, tasks.view_all, members.list) — kể cả những
-- dòng owner đã tự tay gán qua ma trận sau khi lên bảng. Chỉ an toàn để
-- rollback TRƯỚC khi tính năng lên production; sau đó revert bằng migration
-- mới, không dùng file down này.
DELETE FROM center_role_permissions
WHERE permission_key IN (
    'tasks.create', 'tasks.list', 'tasks.read', 'tasks.edit', 'tasks.delete',
    'tasks.manage_board', 'tasks.view_all', 'members.list'
);

DELETE FROM center_member_permissions
WHERE permission_key IN (
    'tasks.create', 'tasks.list', 'tasks.read', 'tasks.edit', 'tasks.delete',
    'tasks.manage_board', 'tasks.view_all', 'members.list'
);

DELETE FROM rbac_backfill_rows
WHERE step IN ('task_role_defaults', 'task_member_defaults');

DROP TABLE IF EXISTS tasks;
DROP TABLE IF EXISTS task_columns;
