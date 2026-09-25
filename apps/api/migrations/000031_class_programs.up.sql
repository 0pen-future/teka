-- =============================================================
-- 000031 — Chương trình học của lớp, chat nội bộ lớp, phả hệ lớp và chỉ mục
-- tra cứu lịch sử theo thực thể.
--
-- class_programs: mỗi lớp gắn tối đa MỘT phiên bản chương trình mẫu đã
-- xuất bản (program_template_versions). Áp dụng chương trình chép danh sách
-- tên bài vào class_curricula.lessons (sổ đầu bài); gỡ chương trình chỉ xoá
-- dòng này, giáo trình và giáo án giữ nguyên. Lớp bị xoá cứng kéo theo
-- dòng chương trình (CASCADE); phiên bản mẫu đang được lớp tham chiếu thì
-- không xoá được (RESTRICT — service trả 409 trước đó).
--
-- class_messages: tin nhắn nội bộ giữa chủ trung tâm và nhân sự lớp, không
-- đồng bộ Zalo. Thân tin tối đa 2000 ký tự; xoá mềm bằng deleted_at để
-- người viết hoặc chủ trung tâm rút lại tin. Lớp bị xoá cứng kéo theo tin.
--
-- classes.parent_class_id / lineage_note: lớp "tiếp nối" từ lớp nào (ví dụ
-- Toán 8 → Toán 9). Chỉ trỏ tới lớp cùng trung tâm (FK composite kèm
-- center_id); xoá cứng lớp cha chỉ xoá con trỏ, không xoá lớp con.
--
-- idx_audit_logs_entity: màn chi tiết lớp hỏi "sự kiện nào đã xảy ra với
-- lớp này" — lọc theo (center_id, entity_type, entity_id) rồi mới đến thời
-- gian, chỉ mục hiện có (center_id, occurred_at) không phục vụ được.
--
-- FK composite kèm center_id theo khuôn 000027/000029/000030: không dòng nào
-- trỏ sang trung tâm khác.
-- =============================================================

CREATE TABLE class_programs (
    class_id            UUID        PRIMARY KEY,
    center_id           UUID        NOT NULL,
    template_version_id UUID        NOT NULL,
    applied_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    applied_by          UUID        NOT NULL,
    CONSTRAINT fk_class_programs_class_center
        FOREIGN KEY (class_id, center_id) REFERENCES classes (id, center_id)
        ON DELETE CASCADE,
    CONSTRAINT fk_class_programs_version_center
        FOREIGN KEY (template_version_id, center_id)
        REFERENCES program_template_versions (id, center_id),
    CONSTRAINT fk_class_programs_applier_center
        FOREIGN KEY (applied_by, center_id) REFERENCES center_members (teacher_id, center_id)
);

-- Thư viện hỏi "phiên bản này đang được lớp nào dùng" trước khi xoá mẫu.
CREATE INDEX idx_class_programs_version
    ON class_programs (template_version_id);

CREATE TABLE class_messages (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    center_id  UUID        NOT NULL,
    class_id   UUID        NOT NULL,
    author_id  UUID        NOT NULL,
    body       TEXT        NOT NULL CHECK (length(body) <= 2000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT fk_class_messages_class_center
        FOREIGN KEY (class_id, center_id) REFERENCES classes (id, center_id)
        ON DELETE CASCADE,
    CONSTRAINT fk_class_messages_author_center
        FOREIGN KEY (author_id, center_id) REFERENCES center_members (teacher_id, center_id)
);

-- Khung chat đọc trang mới nhất rồi cuộn ngược bằng con trỏ (created_at, id).
CREATE INDEX idx_class_messages_class_time
    ON class_messages (class_id, created_at DESC, id DESC);

ALTER TABLE classes
    ADD COLUMN parent_class_id UUID,
    ADD COLUMN lineage_note    TEXT,
    ADD CONSTRAINT fk_classes_parent_center
        FOREIGN KEY (parent_class_id, center_id) REFERENCES classes (id, center_id)
        ON DELETE SET NULL (parent_class_id);

CREATE INDEX idx_classes_parent
    ON classes (parent_class_id) WHERE parent_class_id IS NOT NULL;

CREATE INDEX idx_audit_logs_entity
    ON audit_logs (center_id, entity_type, entity_id, occurred_at DESC, id DESC);

-- Backfill quyền chat: class_messages.post là khoá mặc định (DefaultGrant)
-- theo đúng khuôn 000029/000030 — cả nhánh vai trò hệ thống VÀ thành viên
-- không vai trò của trung tâm đang sống. Cổng "đang có nhiệm kỳ mở trên
-- lớp" do service kiểm tra thêm khi gửi tin.
WITH ins AS (
    INSERT INTO center_role_permissions (role_id, permission_key)
    SELECT cr.id, 'class_messages.post'
    FROM center_roles cr
    JOIN centers c ON c.id = cr.center_id AND c.deleted_at IS NULL
    WHERE cr.is_system
    ON CONFLICT DO NOTHING
    RETURNING role_id, permission_key
)
INSERT INTO rbac_backfill_rows (step, role_id, permission_key)
SELECT 'class_messages_role_defaults', role_id, permission_key FROM ins;

-- ON CONFLICT DO NOTHING giữ nguyên dòng deny sẵn có: deny thắng grant.
WITH ins AS (
    INSERT INTO center_member_permissions (teacher_id, center_id, permission_key, allowed)
    SELECT cm.teacher_id, cm.center_id, 'class_messages.post', TRUE
    FROM center_members cm
    JOIN centers c ON c.id = cm.center_id AND c.deleted_at IS NULL
        AND c.owner_id <> cm.teacher_id
    WHERE cm.left_at IS NULL AND cm.role_id IS NULL
    ON CONFLICT DO NOTHING
    RETURNING teacher_id, center_id, permission_key
)
INSERT INTO rbac_backfill_rows (step, teacher_id, center_id, permission_key, allowed)
SELECT 'class_messages_member_defaults', teacher_id, center_id, permission_key, TRUE FROM ins;
