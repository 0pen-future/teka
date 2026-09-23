-- Gỡ đúng những dòng quyền mà 000030 đã backfill (theo sổ rbac_backfill_rows),
-- không chạm dòng owner tự gán sau đó; rồi bỏ bảng theo thứ tự ngược.
DELETE FROM center_role_permissions rp
USING rbac_backfill_rows b
WHERE b.step = 'paths_role_defaults'
  AND rp.role_id = b.role_id
  AND rp.permission_key = b.permission_key;

DELETE FROM center_member_permissions mp
USING rbac_backfill_rows b
WHERE b.step = 'paths_member_defaults'
  AND mp.teacher_id = b.teacher_id
  AND mp.center_id = b.center_id
  AND mp.permission_key = b.permission_key;

DELETE FROM rbac_backfill_rows
WHERE step IN ('paths_role_defaults', 'paths_member_defaults');

DROP TABLE IF EXISTS path_stage_courses;
DROP TABLE IF EXISTS path_stages;
DROP TABLE IF EXISTS learning_paths;
