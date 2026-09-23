-- =============================================================
-- 000029 — Danh mục khóa học: courses, gói học phí, và lớp gắn khóa học.
--
-- courses là "sản phẩm" của trung tâm: mã, tên, môn, cấp, học phí mặc định
-- và chương trình mẫu mặc định. Lớp học (classes) gắn vào khóa học qua
-- course_id (NULL với lớp cũ chưa gắn) và kế thừa default_unit_price khi
-- tạo lớp mà không nhập giá. Khóa học có ba trạng thái: draft (đang soạn),
-- active (đang mở tuyển), archived (ngừng tuyển) — archive KHÔNG chạm lớp
-- đang chạy.
--
-- course_tuition_packs là các gói học phí (n buổi = x đồng) hiển thị trong
-- tab Thiết lập; ghi đè toàn bộ mỗi lần lưu, thứ tự bằng position.
--
-- FK composite kèm center_id theo khuôn 000027: khóa học không bao giờ
-- trỏ sang chương trình mẫu của trung tâm khác, lớp không bao giờ trỏ sang
-- khóa học của trung tâm khác. default_template_version_id và
-- classes.course_id là tham chiếu (không phải sở hữu) nên SET NULL thay vì
-- CASCADE: xoá cứng phiên bản/khóa học không kéo theo khóa học/lớp.
-- Điều kiện "phiên bản phải là published" do service kiểm (422).
-- =============================================================

CREATE TABLE courses (
    id                          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    center_id                   UUID         NOT NULL REFERENCES centers (id) ON DELETE CASCADE,
    code                        VARCHAR(20)  NOT NULL,
    name                        VARCHAR(200) NOT NULL,
    subject                     VARCHAR(100),
    level                       VARCHAR(100),
    description                 TEXT,
    status                      VARCHAR(12)  NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'active', 'archived')),
    default_template_version_id UUID,
    default_unit_price          BIGINT       NOT NULL DEFAULT 0 CHECK (default_unit_price >= 0),
    total_sessions              INT          CHECK (total_sessions IS NULL OR total_sessions > 0),
    duration_min                INT          CHECK (duration_min IS NULL OR duration_min > 0),
    created_at                  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at                  TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at                  TIMESTAMPTZ,
    UNIQUE (id, center_id),
    CONSTRAINT fk_courses_template_version_center
        FOREIGN KEY (default_template_version_id, center_id)
        REFERENCES program_template_versions (id, center_id)
        ON DELETE SET NULL (default_template_version_id)
);

-- Mã duy nhất trong trung tâm khi chưa xoá mềm; xoá mềm rồi có thể dùng lại mã.
CREATE UNIQUE INDEX uq_courses_code
    ON courses (center_id, code) WHERE deleted_at IS NULL;
CREATE INDEX idx_courses_center
    ON courses (center_id, status, name) WHERE deleted_at IS NULL;

CREATE TABLE course_tuition_packs (
    id        UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    course_id UUID         NOT NULL,
    center_id UUID         NOT NULL,
    name      VARCHAR(100) NOT NULL,
    sessions  INT          NOT NULL CHECK (sessions > 0),
    price     BIGINT       NOT NULL CHECK (price >= 0),
    position  INT          NOT NULL CHECK (position > 0),
    CONSTRAINT uq_course_tuition_packs_position
        UNIQUE (course_id, position) DEFERRABLE INITIALLY DEFERRED,
    CONSTRAINT fk_course_tuition_packs_course_center
        FOREIGN KEY (course_id, center_id) REFERENCES courses (id, center_id)
        ON DELETE CASCADE
);

CREATE INDEX idx_course_tuition_packs_course
    ON course_tuition_packs (course_id, position);

ALTER TABLE classes
    ADD COLUMN course_id UUID,
    ADD CONSTRAINT fk_classes_course
        FOREIGN KEY (course_id, center_id) REFERENCES courses (id, center_id)
        ON DELETE SET NULL (course_id);

CREATE INDEX idx_classes_course
    ON classes (course_id) WHERE deleted_at IS NULL;

-- Backfill quyền đọc: courses.read là khoá mặc định (DefaultGrant) theo đúng
-- khuôn 000027 — cả nhánh vai trò hệ thống VÀ thành viên không vai trò của
-- trung tâm đang sống. KHÔNG backfill courses.edit (opt-in, owner gán tay).
WITH ins AS (
    INSERT INTO center_role_permissions (role_id, permission_key)
    SELECT cr.id, 'courses.read'
    FROM center_roles cr
    JOIN centers c ON c.id = cr.center_id AND c.deleted_at IS NULL
    WHERE cr.is_system
    ON CONFLICT DO NOTHING
    RETURNING role_id, permission_key
)
INSERT INTO rbac_backfill_rows (step, role_id, permission_key)
SELECT 'courses_role_defaults', role_id, permission_key FROM ins;

-- ON CONFLICT DO NOTHING giữ nguyên dòng deny sẵn có: deny thắng grant.
WITH ins AS (
    INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed)
    SELECT cm.teacher_id, cm.center_id, 'courses.read', TRUE
    FROM center_members cm
    JOIN centers c ON c.id = cm.center_id AND c.deleted_at IS NULL
        AND c.owner_id <> cm.teacher_id
    WHERE cm.left_at IS NULL AND cm.role_id IS NULL
    ON CONFLICT DO NOTHING
    RETURNING teacher_id, center_id, permission_key
)
INSERT INTO rbac_backfill_rows (step, teacher_id, center_id, permission_key, allowed)
SELECT 'courses_member_defaults', teacher_id, center_id, permission_key, TRUE FROM ins;
