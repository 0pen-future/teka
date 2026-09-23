-- =============================================================
-- 000030 — Lộ trình học: learning_paths, path_stages, path_stage_courses.
--
-- Lộ trình là chuỗi giai đoạn có thứ tự; mỗi giai đoạn gồm một hoặc nhiều
-- khóa học (courses). Dùng để tư vấn tuyển sinh và hiển thị "khóa tiếp
-- theo" trong chi tiết khóa học. Một khóa học có thể nằm trong nhiều lộ
-- trình. Không gắn học viên vào lộ trình.
--
-- FK composite kèm center_id theo khuôn 000027/000029: giai đoạn không bao
-- giờ thuộc lộ trình của trung tâm khác, giai đoạn không bao giờ liệt kê
-- khóa học của trung tâm khác. Giai đoạn và dòng khóa-trong-giai-đoạn do
-- lộ trình sở hữu nên CASCADE; xoá cứng khóa học cũng rút nó khỏi mọi giai
-- đoạn (service chặn xoá khi khóa còn trong lộ trình đang sống — 409).
--
-- UNIQUE (path_id, position) DEFERRABLE để sắp xếp lại giai đoạn trong một
-- giao dịch mà không cần vị trí tạm, như 000027 (buổi học mẫu) và 000029
-- (gói học phí).
-- =============================================================

CREATE TABLE learning_paths (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    center_id   UUID         NOT NULL REFERENCES centers (id) ON DELETE CASCADE,
    code        VARCHAR(20)  NOT NULL,
    name        VARCHAR(200) NOT NULL,
    description TEXT,
    status      VARCHAR(12)  NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'active', 'archived')),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ,
    UNIQUE (id, center_id)
);

-- Mã duy nhất trong trung tâm khi chưa xoá mềm; xoá mềm rồi có thể dùng lại mã.
CREATE UNIQUE INDEX uq_learning_paths_code
    ON learning_paths (center_id, code) WHERE deleted_at IS NULL;
CREATE INDEX idx_learning_paths_center
    ON learning_paths (center_id, status, name) WHERE deleted_at IS NULL;

CREATE TABLE path_stages (
    id        UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    path_id   UUID         NOT NULL,
    center_id UUID         NOT NULL,
    position  INT          NOT NULL CHECK (position > 0),
    name      VARCHAR(200) NOT NULL,
    goal      TEXT,
    UNIQUE (id, center_id),
    CONSTRAINT uq_path_stages_position
        UNIQUE (path_id, position) DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT fk_path_stages_path_center
        FOREIGN KEY (path_id, center_id) REFERENCES learning_paths (id, center_id)
        ON DELETE CASCADE
);

CREATE INDEX idx_path_stages_path
    ON path_stages (path_id, position);

CREATE TABLE path_stage_courses (
    stage_id  UUID NOT NULL,
    course_id UUID NOT NULL,
    center_id UUID NOT NULL,
    position  INT  NOT NULL CHECK (position > 0),
    PRIMARY KEY (stage_id, course_id),
    CONSTRAINT fk_path_stage_courses_stage_center
        FOREIGN KEY (stage_id, center_id) REFERENCES path_stages (id, center_id)
        ON DELETE CASCADE,
    CONSTRAINT fk_path_stage_courses_course_center
        FOREIGN KEY (course_id, center_id) REFERENCES courses (id, center_id)
        ON DELETE CASCADE
);

-- Chi tiết khóa học hỏi "khóa này nằm trong lộ trình nào".
CREATE INDEX idx_path_stage_courses_course
    ON path_stage_courses (course_id);

-- Backfill quyền đọc: paths.read là khoá mặc định (DefaultGrant) theo đúng
-- khuôn 000029 — cả nhánh vai trò hệ thống VÀ thành viên không vai trò của
-- trung tâm đang sống. KHÔNG backfill paths.edit (opt-in, owner gán tay).
WITH ins AS (
    INSERT INTO center_role_permissions (role_id, permission_key)
    SELECT cr.id, 'paths.read'
    FROM center_roles cr
    JOIN centers c ON c.id = cr.center_id AND c.deleted_at IS NULL
    WHERE cr.is_system
    ON CONFLICT DO NOTHING
    RETURNING role_id, permission_key
)
INSERT INTO rbac_backfill_rows (step, role_id, permission_key)
SELECT 'paths_role_defaults', role_id, permission_key FROM ins;

-- ON CONFLICT DO NOTHING giữ nguyên dòng deny sẵn có: deny thắng grant.
WITH ins AS (
    INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed)
    SELECT cm.teacher_id, cm.center_id, 'paths.read', TRUE
    FROM center_members cm
    JOIN centers c ON c.id = cm.center_id AND c.deleted_at IS NULL
        AND c.owner_id <> cm.teacher_id
    WHERE cm.left_at IS NULL AND cm.role_id IS NULL
    ON CONFLICT DO NOTHING
    RETURNING teacher_id, center_id, permission_key
)
INSERT INTO rbac_backfill_rows (step, teacher_id, center_id, permission_key, allowed)
SELECT 'paths_member_defaults', teacher_id, center_id, permission_key, TRUE FROM ins;
