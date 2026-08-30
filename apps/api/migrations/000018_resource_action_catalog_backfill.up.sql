-- Backfill tương thích cho catalog resource-action: vai trò hệ thống và
-- stint không vai trò nhận baseline vận hành (trước catalog, chỉ cần là
-- thành viên là làm được mọi thao tác — giữ nguyên hành vi nghĩa là cấp đủ
-- các key vận hành). Key phạm vi (<resource>.view_all) KHÔNG nằm trong
-- baseline: tầm nhìn dữ liệu không được mở rộng. Các key định danh legacy
-- (reports.send, members.manage, ...) cũng không: chúng vốn đã bị chặn theo
-- permission, cấp mặc định là leo thang quyền.
--
-- Mọi dòng backfill chèn được ghi lại trong rbac_backfill_rows để down chỉ
-- xoá đúng dữ liệu do rollout tạo ra — dòng do owner tự gán không bị đụng.
-- rbac_backfill_ledger lưu checksum của mapping (khớp với catalog Go qua
-- test parity trong package migrations) và số dòng từng bước.

-- Cột phiên bản cho ghi-đè CAS: đọc trả về phiên bản, ghi thay-thế phải nộp
-- lại đúng phiên bản đó, lệch là 409. Mặc định 1 — 0 là "client cũ không gửi".
ALTER TABLE center_roles ADD COLUMN assignment_version BIGINT NOT NULL DEFAULT 1;
ALTER TABLE center_members ADD COLUMN assignment_version BIGINT NOT NULL DEFAULT 1;

CREATE TABLE rbac_backfill_rows (
    step           TEXT NOT NULL,
    role_id        UUID,
    teacher_id     UUID,
    center_id      UUID,
    permission_key VARCHAR(64) NOT NULL,
    allowed        BOOLEAN
);

CREATE TABLE rbac_backfill_ledger (
    applied_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    mapping_checksum    TEXT NOT NULL,
    role_default_rows   INTEGER NOT NULL,
    member_default_rows INTEGER NOT NULL,
    scope_role_rows     INTEGER NOT NULL,
    scope_member_rows   INTEGER NOT NULL
);

-- Baseline cho vai trò hệ thống của mọi trung tâm đang sống. ON CONFLICT DO
-- NOTHING giữ nguyên dòng đã tồn tại (idempotent, không ghi đè gán tay).
WITH ins AS (
    INSERT INTO center_role_permissions (role_id, permission_key)
    SELECT cr.id, k.key
    FROM center_roles cr
    JOIN centers c ON c.id = cr.center_id AND c.deleted_at IS NULL
    CROSS JOIN (VALUES
        -- teka:default-keys-begin
        ('classes.create'),
        ('classes.list'),
        ('classes.read'),
        ('classes.edit'),
        ('classes.delete'),
        ('classes.archive'),
        ('schedules.create'),
        ('schedules.edit'),
        ('schedules.delete'),
        ('contacts.create'),
        ('contacts.list'),
        ('contacts.read'),
        ('contacts.edit'),
        ('contacts.delete'),
        ('contacts.link_zalo'),
        ('students.create'),
        ('students.list'),
        ('students.read'),
        ('students.edit'),
        ('students.delete'),
        ('enrollments.create'),
        ('enrollments.list'),
        ('enrollments.read'),
        ('enrollments.delete'),
        ('enrollments.end'),
        ('sessions.create'),
        ('sessions.list'),
        ('sessions.read'),
        ('sessions.delete'),
        ('sessions.lifecycle'),
        ('attendance.read'),
        ('attendance.confirm'),
        ('scores.read'),
        ('scores.edit'),
        ('teaching.read'),
        ('teaching.edit'),
        ('billing.create'),
        ('billing.list'),
        ('billing.read'),
        ('billing.draft'),
        ('billing.close'),
        ('billing.void_invoice'),
        ('billing.adjust_invoice'),
        ('payments.create'),
        ('payments.list'),
        ('payments.read'),
        ('payments.allocate'),
        ('payments.reverse'),
        ('statements.list'),
        ('statements.read'),
        ('statements.generate'),
        ('statements.revoke'),
        ('notifications.mark_sent')
        -- teka:default-keys-end
    ) AS k(key)
    WHERE cr.is_system
    ON CONFLICT DO NOTHING
    RETURNING role_id, permission_key
)
INSERT INTO rbac_backfill_rows (step, role_id, permission_key)
SELECT 'role_defaults', role_id, permission_key FROM ins;

-- Stint đang sống không có vai trò (role_id NULL, không phải owner) nhận
-- baseline qua grant theo thành viên — backfill vai trò không chạm tới họ.
-- ON CONFLICT DO NOTHING giữ nguyên dòng deny sẵn có: deny thắng grant.
WITH ins AS (
    INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed)
    SELECT cm.teacher_id, cm.center_id, k.key, TRUE
    FROM center_members cm
    JOIN centers c ON c.id = cm.center_id AND c.deleted_at IS NULL
        AND c.owner_id <> cm.teacher_id
    CROSS JOIN (VALUES
        -- teka:default-keys-begin
        ('classes.create'),
        ('classes.list'),
        ('classes.read'),
        ('classes.edit'),
        ('classes.delete'),
        ('classes.archive'),
        ('schedules.create'),
        ('schedules.edit'),
        ('schedules.delete'),
        ('contacts.create'),
        ('contacts.list'),
        ('contacts.read'),
        ('contacts.edit'),
        ('contacts.delete'),
        ('contacts.link_zalo'),
        ('students.create'),
        ('students.list'),
        ('students.read'),
        ('students.edit'),
        ('students.delete'),
        ('enrollments.create'),
        ('enrollments.list'),
        ('enrollments.read'),
        ('enrollments.delete'),
        ('enrollments.end'),
        ('sessions.create'),
        ('sessions.list'),
        ('sessions.read'),
        ('sessions.delete'),
        ('sessions.lifecycle'),
        ('attendance.read'),
        ('attendance.confirm'),
        ('scores.read'),
        ('scores.edit'),
        ('teaching.read'),
        ('teaching.edit'),
        ('billing.create'),
        ('billing.list'),
        ('billing.read'),
        ('billing.draft'),
        ('billing.close'),
        ('billing.void_invoice'),
        ('billing.adjust_invoice'),
        ('payments.create'),
        ('payments.list'),
        ('payments.read'),
        ('payments.allocate'),
        ('payments.reverse'),
        ('statements.list'),
        ('statements.read'),
        ('statements.generate'),
        ('statements.revoke'),
        ('notifications.mark_sent')
        -- teka:default-keys-end
    ) AS k(key)
    WHERE cm.left_at IS NULL AND cm.role_id IS NULL
    ON CONFLICT DO NOTHING
    RETURNING teacher_id, center_id, permission_key
)
INSERT INTO rbac_backfill_rows (step, teacher_id, center_id, permission_key, allowed)
SELECT 'member_defaults', teacher_id, center_id, permission_key, TRUE FROM ins;

-- Mở rộng đối xứng key phạm vi legacy ở vai trò: mỗi dòng
-- data.view_center_wide sinh đủ 12 dòng <resource>.view_all. Dòng legacy giữ
-- nguyên — nó vẫn hiệu lực qua alias expansion cho tới cutover Phase 4.
WITH ins AS (
    INSERT INTO center_role_permissions (role_id, permission_key)
    SELECT rp.role_id, v.key
    FROM center_role_permissions rp
    CROSS JOIN (VALUES
        -- teka:scope-keys-begin
        ('classes.view_all'),
        ('contacts.view_all'),
        ('students.view_all'),
        ('enrollments.view_all'),
        ('sessions.view_all'),
        ('attendance.view_all'),
        ('scores.view_all'),
        ('teaching.view_all'),
        ('billing.view_all'),
        ('payments.view_all'),
        ('statements.view_all'),
        ('notifications.view_all')
        -- teka:scope-keys-end
    ) AS v(key)
    -- teka:legacy-scope-key-begin
    WHERE rp.permission_key = 'data.view_center_wide'
    -- teka:legacy-scope-key-end
    ON CONFLICT DO NOTHING
    RETURNING role_id, permission_key
)
INSERT INTO rbac_backfill_rows (step, role_id, permission_key)
SELECT 'scope_role', role_id, permission_key FROM ins;

-- Mở rộng đối xứng ở thành viên: grant sinh 12 grant, deny sinh 12 deny
-- (copy nguyên allowed). Dòng canonical sẵn có (kể cả deny owner đã gán)
-- được giữ nguyên nhờ ON CONFLICT DO NOTHING.
WITH ins AS (
    INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed)
    SELECT mp.teacher_id, mp.center_id, v.key, mp.allowed
    FROM center_member_permissions mp
    CROSS JOIN (VALUES
        -- teka:scope-keys-begin
        ('classes.view_all'),
        ('contacts.view_all'),
        ('students.view_all'),
        ('enrollments.view_all'),
        ('sessions.view_all'),
        ('attendance.view_all'),
        ('scores.view_all'),
        ('teaching.view_all'),
        ('billing.view_all'),
        ('payments.view_all'),
        ('statements.view_all'),
        ('notifications.view_all')
        -- teka:scope-keys-end
    ) AS v(key)
    -- teka:legacy-scope-key-begin
    WHERE mp.permission_key = 'data.view_center_wide'
    -- teka:legacy-scope-key-end
    ON CONFLICT DO NOTHING
    RETURNING teacher_id, center_id, permission_key, allowed
)
INSERT INTO rbac_backfill_rows (step, teacher_id, center_id, permission_key, allowed)
SELECT 'scope_member', teacher_id, center_id, permission_key, allowed FROM ins;

-- Sổ cái: checksum mapping (pin bằng test parity với catalog Go) + số dòng
-- từng bước, để vận hành biết database đã backfill dưới thế hệ catalog nào.
INSERT INTO rbac_backfill_ledger (
    mapping_checksum, role_default_rows, member_default_rows,
    scope_role_rows, scope_member_rows)
VALUES (
    'sha256:76a7390d88b0c63bb608f0fc3dfb509240c154a521e6afb6b9c4a5543d8656f8',
    (SELECT count(*) FROM rbac_backfill_rows WHERE step = 'role_defaults'),
    (SELECT count(*) FROM rbac_backfill_rows WHERE step = 'member_defaults'),
    (SELECT count(*) FROM rbac_backfill_rows WHERE step = 'scope_role'),
    (SELECT count(*) FROM rbac_backfill_rows WHERE step = 'scope_member')
);
