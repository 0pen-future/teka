-- =============================================================
-- 000027 — Kho học liệu: chương trình mẫu → phiên bản → buổi học mẫu.
--
-- program_templates là tài sản chung của trung tâm (không thuộc lớp/giáo
-- viên nào). Mỗi template có nhiều phiên bản; chỉ tối đa MỘT bản nháp tại
-- một thời điểm (unique một phần theo status = 'draft'). Phiên bản đã
-- published bất biến — service khoá buổi học của nó (409 VERSION_LOCKED);
-- archived giữ nguyên dữ liệu để lớp đang dùng vẫn đọc được.
--
-- FK composite kèm center_id theo khuôn 000015/000022: một dòng không bao giờ
-- trỏ chéo trung tâm; CASCADE chỉ chạy khi xoá cứng dòng cha. created_by SET
-- NULL riêng cột thay vì CASCADE: xoá cứng tài khoản người soạn không được
-- kéo theo chương trình mẫu — nội dung chung của trung tâm.
-- (version_id, position) DEFERRABLE để đổi chỗ hai buổi trong cùng một
-- transaction mà không đi qua vị trí tạm.
-- =============================================================

CREATE TABLE program_templates (
    id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    center_id   UUID         NOT NULL REFERENCES centers (id) ON DELETE CASCADE,
    code        VARCHAR(20)  NOT NULL,
    name        VARCHAR(200) NOT NULL,
    subject     VARCHAR(100),
    level       VARCHAR(100),
    description TEXT,
    created_by  UUID,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ,
    UNIQUE (id, center_id),
    CONSTRAINT fk_program_templates_creator_center
        FOREIGN KEY (created_by, center_id) REFERENCES center_members (teacher_id, center_id)
        ON DELETE SET NULL (created_by)
);

-- Mã duy nhất trong trung tâm khi chưa xoá mềm; xoá mềm rồi có thể dùng lại mã.
CREATE UNIQUE INDEX uq_program_templates_code
    ON program_templates (center_id, code) WHERE deleted_at IS NULL;
CREATE INDEX idx_program_templates_center
    ON program_templates (center_id, name) WHERE deleted_at IS NULL;

CREATE TABLE program_template_versions (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    template_id  UUID        NOT NULL,
    center_id    UUID        NOT NULL,
    version_no   INT         NOT NULL CHECK (version_no > 0),
    status       VARCHAR(12) NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'published', 'archived')),
    changelog    TEXT,
    published_at TIMESTAMPTZ,
    created_by   UUID,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (template_id, version_no),
    UNIQUE (id, center_id),
    CONSTRAINT fk_program_template_versions_template_center
        FOREIGN KEY (template_id, center_id) REFERENCES program_templates (id, center_id)
        ON DELETE CASCADE,
    CONSTRAINT fk_program_template_versions_creator_center
        FOREIGN KEY (created_by, center_id) REFERENCES center_members (teacher_id, center_id)
        ON DELETE SET NULL (created_by)
);

-- Mỗi template tối đa một bản nháp.
CREATE UNIQUE INDEX uq_program_template_versions_draft
    ON program_template_versions (template_id) WHERE status = 'draft';
CREATE INDEX idx_program_template_versions_template
    ON program_template_versions (template_id, version_no DESC);

CREATE TABLE template_lessons (
    id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    version_id    UUID         NOT NULL,
    center_id     UUID         NOT NULL,
    position      INT          NOT NULL CHECK (position > 0),
    title         VARCHAR(200) NOT NULL,
    objectives    TEXT,
    duration_min  INT          CHECK (duration_min IS NULL OR duration_min > 0),
    homework_note TEXT,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    UNIQUE (id, center_id),
    CONSTRAINT uq_template_lessons_position
        UNIQUE (version_id, position) DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT fk_template_lessons_version_center
        FOREIGN KEY (version_id, center_id) REFERENCES program_template_versions (id, center_id)
        ON DELETE CASCADE
);

CREATE INDEX idx_template_lessons_version
    ON template_lessons (version_id, position);

-- Backfill quyền đọc: library.read là khoá mặc định (DefaultGrant) theo đúng
-- khuôn 000018/000022 — cả nhánh vai trò hệ thống VÀ thành viên không vai
-- trò của trung tâm đang sống. KHÔNG backfill library.edit / library.publish
-- (opt-in, owner gán tay qua ma trận quyền).
WITH ins AS (
    INSERT INTO center_role_permissions (role_id, permission_key)
    SELECT cr.id, 'library.read'
    FROM center_roles cr
    JOIN centers c ON c.id = cr.center_id AND c.deleted_at IS NULL
    WHERE cr.is_system
    ON CONFLICT DO NOTHING
    RETURNING role_id, permission_key
)
INSERT INTO rbac_backfill_rows (step, role_id, permission_key)
SELECT 'library_role_defaults', role_id, permission_key FROM ins;

-- ON CONFLICT DO NOTHING giữ nguyên dòng deny sẵn có: deny thắng grant.
WITH ins AS (
    INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed)
    SELECT cm.teacher_id, cm.center_id, 'library.read', TRUE
    FROM center_members cm
    JOIN centers c ON c.id = cm.center_id AND c.deleted_at IS NULL
        AND c.owner_id <> cm.teacher_id
    WHERE cm.left_at IS NULL AND cm.role_id IS NULL
    ON CONFLICT DO NOTHING
    RETURNING teacher_id, center_id, permission_key
)
INSERT INTO rbac_backfill_rows (step, teacher_id, center_id, permission_key, allowed)
SELECT 'library_member_defaults', teacher_id, center_id, permission_key, TRUE FROM ins;
