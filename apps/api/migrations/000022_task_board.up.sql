-- Trung tâm công việc: bảng Kanban theo trung tâm với cột tự cấu hình.
-- task_columns/tasks theo khuôn toàn vẹn 000007/000015 — FK composite kèm
-- center_id để một dòng không bao giờ trỏ chéo trung tâm. RESTRICT trên
-- column_id buộc service di dời việc trước khi xoá cột (cột là tài nguyên
-- chung, xoá ảnh hưởng mọi người); CASCADE trên created_by theo khuôn
-- center_members hiện có — rời trung tâm không khoá được xoá cứng
-- teacher/center (PII) trong một transaction, nên task của họ xoá theo.
-- assignee_id thì SET NULL riêng cột đó thay vì CASCADE cả dòng: bị gỡ khỏi
-- trung tâm chỉ nên bỏ gán việc, không xoá luôn task do người khác tạo.
CREATE TABLE task_columns (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    center_id  UUID NOT NULL REFERENCES centers(id) ON DELETE CASCADE,
    name       VARCHAR(40) NOT NULL,
    position   INT NOT NULL,
    is_done    BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (id, center_id) -- đích cho FK composite của tasks.column_id
);

-- Tên cột duy nhất trong trung tâm, không phân biệt hoa thường — chặn ở DB,
-- không chỉ ở UI.
CREATE UNIQUE INDEX uq_task_columns_name ON task_columns (center_id, lower(name));
CREATE INDEX idx_task_columns_order ON task_columns (center_id, position);

CREATE TABLE tasks (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    center_id    UUID NOT NULL,
    column_id    UUID NOT NULL,
    created_by   UUID NOT NULL,
    assignee_id  UUID,
    title        VARCHAR(200) NOT NULL,
    description  TEXT NOT NULL DEFAULT '',
    priority     VARCHAR(8) NOT NULL DEFAULT 'none' CHECK (priority IN ('none', 'low', 'medium', 'high')),
    due_on       DATE,
    position     DOUBLE PRECISION NOT NULL DEFAULT 0,
    completed_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at   TIMESTAMPTZ,
    CONSTRAINT fk_tasks_column_center
        FOREIGN KEY (column_id, center_id) REFERENCES task_columns (id, center_id) ON DELETE RESTRICT,
    CONSTRAINT fk_tasks_creator_center
        FOREIGN KEY (created_by, center_id) REFERENCES center_members (teacher_id, center_id) ON DELETE CASCADE,
    CONSTRAINT fk_tasks_assignee_center
        FOREIGN KEY (assignee_id, center_id) REFERENCES center_members (teacher_id, center_id) ON DELETE SET NULL (assignee_id)
);

CREATE INDEX idx_tasks_board ON tasks (center_id, column_id, position) WHERE deleted_at IS NULL;
CREATE INDEX idx_tasks_assignee ON tasks (center_id, assignee_id);
CREATE INDEX idx_tasks_creator ON tasks (center_id, created_by);

-- Backfill cột mặc định: mỗi trung tâm đang sống nhận đúng 3 cột. Literal
-- tên/thứ tự/is_done phải khớp centers/default_columns.go (Phase 3) — parity
-- test trong package migrations so khối này với một literal đóng băng.
-- ON CONFLICT DO NOTHING theo unique index tên: chạy lại migration (idempotent
-- theo schema_migrations) không tạo cột trùng nếu tên mặc định đã tồn tại.
INSERT INTO task_columns (center_id, name, position, is_done)
SELECT c.id, v.name, v.pos, v.done
FROM centers c
CROSS JOIN (VALUES
    -- teka:default-columns-begin
    ('Cần làm', 0, FALSE),
    ('Đang làm', 1, FALSE),
    ('Hoàn thành', 2, TRUE)
    -- teka:default-columns-end
) AS v(name, pos, done)
WHERE c.deleted_at IS NULL
ON CONFLICT DO NOTHING;

-- Backfill quyền: 5 khoá CRUD (tasks.create/list/read/edit/delete) theo đúng
-- khuôn 000018 — cả hai nhánh vai trò VÀ thành viên không vai trò, nếu không
-- stint không vai trò của trung tâm có sẵn mất toàn bộ tasks.*.
-- KHÔNG backfill tasks.manage_board / tasks.view_all / members.list — ba khoá
-- opt-in (DefaultGrant: false), owner gán tay qua ma trận.
WITH ins AS (
    INSERT INTO center_role_permissions (role_id, permission_key)
    SELECT cr.id, k.key
    FROM center_roles cr
    JOIN centers c ON c.id = cr.center_id AND c.deleted_at IS NULL
    CROSS JOIN (VALUES
        -- teka:task-keys-begin
        ('tasks.create'),
        ('tasks.list'),
        ('tasks.read'),
        ('tasks.edit'),
        ('tasks.delete')
        -- teka:task-keys-end
    ) AS k(key)
    WHERE cr.is_system
    ON CONFLICT DO NOTHING
    RETURNING role_id, permission_key
)
INSERT INTO rbac_backfill_rows (step, role_id, permission_key)
SELECT 'task_role_defaults', role_id, permission_key FROM ins;

-- Stint đang sống không có vai trò (role_id NULL, không phải owner) nhận 5
-- khoá CRUD qua grant theo thành viên — backfill vai trò không chạm tới họ.
-- ON CONFLICT DO NOTHING giữ nguyên dòng deny sẵn có: deny thắng grant.
WITH ins AS (
    INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed)
    SELECT cm.teacher_id, cm.center_id, k.key, TRUE
    FROM center_members cm
    JOIN centers c ON c.id = cm.center_id AND c.deleted_at IS NULL
        AND c.owner_id <> cm.teacher_id
    CROSS JOIN (VALUES
        -- teka:task-keys-begin
        ('tasks.create'),
        ('tasks.list'),
        ('tasks.read'),
        ('tasks.edit'),
        ('tasks.delete')
        -- teka:task-keys-end
    ) AS k(key)
    WHERE cm.left_at IS NULL AND cm.role_id IS NULL
    ON CONFLICT DO NOTHING
    RETURNING teacher_id, center_id, permission_key
)
INSERT INTO rbac_backfill_rows (step, teacher_id, center_id, permission_key, allowed)
SELECT 'task_member_defaults', teacher_id, center_id, permission_key, TRUE FROM ins;
