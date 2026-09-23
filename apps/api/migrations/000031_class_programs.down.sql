-- Gỡ đúng những dòng quyền mà 000031 đã backfill (theo sổ rbac_backfill_rows),
-- không chạm dòng owner tự gán sau đó; rồi bỏ chỉ mục, cột và bảng theo thứ
-- tự ngược.
DELETE FROM center_role_permissions rp
USING rbac_backfill_rows b
WHERE b.step = 'class_messages_role_defaults'
  AND rp.role_id = b.role_id
  AND rp.permission_key = b.permission_key;

DELETE FROM center_member_permissions mp
USING rbac_backfill_rows b
WHERE b.step = 'class_messages_member_defaults'
  AND mp.teacher_id = b.teacher_id
  AND mp.center_id = b.center_id
  AND mp.permission_key = b.permission_key;

DELETE FROM rbac_backfill_rows
WHERE step IN ('class_messages_role_defaults', 'class_messages_member_defaults');

DROP INDEX IF EXISTS idx_audit_logs_entity;

DROP INDEX IF EXISTS idx_classes_parent;
ALTER TABLE classes
    DROP CONSTRAINT IF EXISTS fk_classes_parent_center,
    DROP COLUMN IF EXISTS lineage_note,
    DROP COLUMN IF EXISTS parent_class_id;

DROP TABLE IF EXISTS class_messages;
DROP TABLE IF EXISTS class_programs;
