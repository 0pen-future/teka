-- Gỡ đúng những dòng quyền mà 000029 đã backfill (theo sổ rbac_backfill_rows),
-- không chạm dòng owner tự gán sau đó; rồi bỏ cột/bảng theo thứ tự ngược.
DELETE FROM center_role_permissions rp
USING rbac_backfill_rows b
WHERE b.step = 'courses_role_defaults'
  AND rp.role_id = b.role_id
  AND rp.permission_key = b.permission_key;

DELETE FROM center_member_permissions mp
USING rbac_backfill_rows b
WHERE b.step = 'courses_member_defaults'
  AND mp.teacher_id = b.teacher_id
  AND mp.center_id = b.center_id
  AND mp.permission_key = b.permission_key;

DELETE FROM rbac_backfill_rows
WHERE step IN ('courses_role_defaults', 'courses_member_defaults');

DROP INDEX IF EXISTS idx_classes_course;
ALTER TABLE classes
    DROP CONSTRAINT IF EXISTS fk_classes_course,
    DROP COLUMN IF EXISTS course_id;

DROP TABLE IF EXISTS course_tuition_packs;
DROP TABLE IF EXISTS courses;
