-- =============================================================
-- 000015 — class_staff: nhân sự theo vai trong lớp (giáo viên chính, học vụ,
-- trợ giảng). Một dòng = một nhiệm kỳ (stint) của một người trong một lớp:
-- ended_at NULL là đang hoạt động; NOT NULL là đã kết thúc nhưng vẫn cấp
-- quyền ĐỌC lịch sử lớp (soft-close). role_key validate trong code (authctx)
-- — không CHECK cứng, để thêm vai mới không cần migration.
--
-- Theo khuôn toàn vẹn 000007/000009: FK composite kèm center_id để dòng không
-- trỏ chéo trung tâm; ON DELETE CASCADE bắt buộc để tx xoá cứng
-- teacher/center (PII) qua center_members vẫn chạy một lần commit.
-- =============================================================

CREATE TABLE class_staff (
    id         UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    class_id   UUID        NOT NULL,
    center_id  UUID        NOT NULL,
    teacher_id UUID        NOT NULL,
    role_key   VARCHAR(32) NOT NULL,
    started_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at   TIMESTAMPTZ,
    FOREIGN KEY (class_id, center_id)  REFERENCES classes (id, center_id) ON DELETE CASCADE,
    FOREIGN KEY (teacher_id, center_id) REFERENCES center_members (teacher_id, center_id) ON DELETE CASCADE
);

-- Một người tối đa một nhiệm kỳ đang hoạt động trong một lớp.
CREATE UNIQUE INDEX uq_class_staff_active
    ON class_staff (class_id, teacher_id) WHERE ended_at IS NULL;

-- Bất biến dual-write với classes.teacher_id: đúng một giáo viên chính đang
-- hoạt động mỗi lớp. Hai handoff chạy song song đâm vào index này thành lỗi
-- tx (retry được) thay vì âm thầm sinh hai giáo viên chính.
CREATE UNIQUE INDEX uq_class_staff_one_gv
    ON class_staff (class_id) WHERE ended_at IS NULL AND role_key = 'giao_vien';

CREATE INDEX idx_class_staff_teacher ON class_staff (teacher_id, center_id);
CREATE INDEX idx_class_staff_class ON class_staff (class_id);

-- Backfill: mỗi lớp còn sống → một nhiệm kỳ giao_vien đang hoạt động cho
-- teacher_id hiện tại. Lọc deleted_at vì classes chỉ soft-delete (CASCADE
-- không bao giờ kích hoạt) — lớp đã xoá không được chiếm uq_class_staff_one_gv.
-- Idempotent (ON CONFLICT trên index partial) để chạy lại được như lệnh
-- reconcile khi dọn dual-write.
INSERT INTO class_staff (class_id, center_id, teacher_id, role_key)
SELECT c.id, c.center_id, c.teacher_id, 'giao_vien'
FROM classes c
WHERE c.deleted_at IS NULL
ON CONFLICT (class_id, teacher_id) WHERE ended_at IS NULL DO NOTHING;
