-- =============================================================
-- 000026 — class_invitations: lời mời nhận vai trong lớp. Một lời mời là
-- ĐỀ XUẤT của chủ trung tâm: người được mời chấp nhận/từ chối trong app,
-- nhưng stint class_staff chỉ được ghi khi chủ trung tâm xác nhận
-- (status = assigned). Bảng riêng, không tái dùng invitations (onboarding
-- trung tâm, public token).
--
-- FK composite kèm center_id theo khuôn 000015: dòng không trỏ chéo trung
-- tâm; ON DELETE CASCADE chỉ chạy khi xoá cứng lớp/tài khoản. Rời trung tâm
-- (center_members.left_at) và xoá mềm lớp KHÔNG kích hoạt cascade — hủy lời
-- mời khi gỡ thành viên nằm trong service.
-- =============================================================

CREATE TABLE class_invitations (
    id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    center_id    UUID        NOT NULL,
    class_id     UUID        NOT NULL,
    teacher_id   UUID        NOT NULL,
    role_key     VARCHAR(20) NOT NULL CHECK (role_key IN ('giao_vien', 'tro_giang', 'hoc_vu')),
    status       VARCHAR(12) NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'accepted', 'declined', 'cancelled', 'assigned')),
    invited_by   UUID        NOT NULL,
    message      TEXT,
    sent_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    reminded_at  TIMESTAMPTZ,
    responded_at TIMESTAMPTZ,
    assigned_at  TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (class_id, center_id)   REFERENCES classes (id, center_id) ON DELETE CASCADE,
    FOREIGN KEY (teacher_id, center_id) REFERENCES center_members (teacher_id, center_id) ON DELETE CASCADE
);

-- Mỗi người tối đa một lời mời đang chờ cho một lớp.
CREATE UNIQUE INDEX uq_class_invitations_pending
    ON class_invitations (class_id, teacher_id) WHERE status = 'pending';

CREATE INDEX idx_class_invitations_center_status
    ON class_invitations (center_id, status, sent_at DESC);
CREATE INDEX idx_class_invitations_teacher
    ON class_invitations (teacher_id, status);
