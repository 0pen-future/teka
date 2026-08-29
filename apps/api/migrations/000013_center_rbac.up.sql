-- RBAC theo trung tâm: vai trò cấu hình được cho thành viên và override
-- grant/deny theo từng thành viên. Danh mục permission key sống trong code
-- (authctx/permissions.go) — DB chỉ lưu phép gán, không lưu định nghĩa.
-- Chủ trung tâm (centers.owner_id) là superuser ngầm định, đứng NGOÀI hệ
-- vai trò: không có role row, mọi kiểm tra Has() trả true theo owner bypass.

-- Vai trò theo trung tâm. Ba vai trò hệ thống (giao_vien, hoc_vu, tro_giang)
-- được gieo cho mỗi trung tâm khi tạo; is_system chặn xoá/tạo tuỳ ý về sau
-- (v1 chưa có CRUD vai trò tuỳ chỉnh).
CREATE TABLE center_roles (
    id         UUID PRIMARY KEY,
    center_id  UUID NOT NULL REFERENCES centers(id) ON DELETE CASCADE,
    key        VARCHAR(32)  NOT NULL,
    name       VARCHAR(100) NOT NULL,
    is_system  BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (center_id, key)
);

-- Tập permission của một vai trò. permission_key phải là key hợp lệ trong
-- danh mục code; key lạ bị bỏ qua khi đọc nên một lần rollback code không
-- làm hỏng scope resolution.
CREATE TABLE center_role_permissions (
    role_id        UUID NOT NULL REFERENCES center_roles(id) ON DELETE CASCADE,
    permission_key VARCHAR(64) NOT NULL,
    PRIMARY KEY (role_id, permission_key)
);

-- Vai trò của stint hiện tại. NULL là trạng thái hợp lệ, nghĩa là "không có
-- quyền theo vai trò" (tương đương giao_vien mặc định của v1): stint của
-- owner luôn NULL, và các đường insert thô (seed/fixture) được phép để NULL.
ALTER TABLE center_members ADD COLUMN role_id UUID REFERENCES center_roles(id);

-- Override theo thành viên: allowed=TRUE cấp thêm, FALSE chặn key bất kể vai
-- trò. Sống theo stint như can_send_reports — đóng/mở lại membership phải xoá
-- sạch các dòng này (app-enforced trong Open/CloseMembership).
CREATE TABLE center_member_permissions (
    teacher_id     UUID NOT NULL,
    center_id      UUID NOT NULL,
    permission_key VARCHAR(64) NOT NULL,
    allowed        BOOLEAN NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (teacher_id, center_id, permission_key),
    FOREIGN KEY (teacher_id, center_id)
        REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE
);

-- Backfill: mỗi trung tâm đang sống nhận đủ 3 vai trò hệ thống.
INSERT INTO center_roles (id, center_id, key, name)
SELECT gen_random_uuid(), c.id, r.key, r.name
FROM centers c
CROSS JOIN (VALUES
    ('giao_vien', 'Giáo viên'),
    ('hoc_vu',    'Học vụ'),
    ('tro_giang', 'Trợ giảng')
) AS r(key, name)
WHERE c.deleted_at IS NULL;

-- Stint đang sống của thành viên (không phải owner) nhận vai trò mặc định
-- giao_vien; stint của owner giữ NULL — owner đứng ngoài hệ vai trò.
UPDATE center_members cm
SET role_id = cr.id
FROM center_roles cr, centers c
WHERE c.id = cm.center_id
  AND cr.center_id = cm.center_id AND cr.key = 'giao_vien'
  AND cm.left_at IS NULL
  AND c.owner_id <> cm.teacher_id;

-- Cờ can_send_reports đang bật trở thành override grant reports.send, giữ
-- hai nguồn song song bằng nhau ngay từ đầu (cột vẫn là nguồn chính cho tới
-- khi bị gỡ ở migration sau).
INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed)
SELECT cm.teacher_id, cm.center_id, 'reports.send', TRUE
FROM center_members cm
WHERE cm.left_at IS NULL AND cm.can_send_reports;
