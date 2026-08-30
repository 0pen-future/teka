-- Gỡ backfill: chỉ xoá đúng những dòng đã ghi trong rbac_backfill_rows.
-- Dòng do owner tự gán (không có trong sổ) không bị đụng. Với dòng thành
-- viên, allowed phải còn khớp giá trị backfill đã ghi — owner đã lật
-- grant/deny thì dòng đó sống sót qua down (quyết định của owner thắng).
DELETE FROM center_role_permissions rp
USING rbac_backfill_rows b
WHERE b.step IN ('role_defaults', 'scope_role')
  AND b.role_id = rp.role_id
  AND b.permission_key = rp.permission_key;

DELETE FROM center_member_permissions mp
USING rbac_backfill_rows b
WHERE b.step IN ('member_defaults', 'scope_member')
  AND b.teacher_id = mp.teacher_id
  AND b.center_id = mp.center_id
  AND b.permission_key = mp.permission_key
  AND b.allowed = mp.allowed;

DROP TABLE rbac_backfill_ledger;
DROP TABLE rbac_backfill_rows;

ALTER TABLE center_members DROP COLUMN assignment_version;
ALTER TABLE center_roles DROP COLUMN assignment_version;
