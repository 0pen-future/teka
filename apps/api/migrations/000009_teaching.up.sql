-- =============================================================
-- 000009 — Dữ liệu giảng dạy: giáo trình, giáo án (kèm vòng duyệt),
-- nhận xét buổi và điểm/ghi chú riêng từng học sinh.
--
-- Chuyển phần dữ liệu trước đây chỉ nằm ở localStorage của web (store
-- teaching UI-first) về Postgres. Toàn bộ bảng theo đúng khuôn toàn vẹn
-- của 000007: center_id NOT NULL, FK composite về UNIQUE (id, center_id)
-- của bảng cha, và FK guard (teacher_id, center_id) → center_members —
-- teacher trên row phải đã/đang là thành viên center của row.
--
-- KHÔNG có deleted_at: dữ liệu giảng dạy không dính tiền, thay được;
-- xoá cứng theo cha (class/session/student) là đủ — khác các bảng
-- tài chính vẫn giữ soft delete.
-- =============================================================

-- Giáo trình: một row cho mỗi lớp. lessons là mảng JSONB tên bài theo thứ
-- tự — UI sửa cả danh sách một lần (modal thay nguyên list) nên không tách
-- bảng con; giáo án trỏ bài học bằng chỉ số (lesson_index), sửa giáo trình
-- làm lệch chỉ số là ngữ nghĩa đã chấp nhận từ prototype.
CREATE TABLE class_curricula (
    id            UUID PRIMARY KEY,
    class_id      UUID        NOT NULL UNIQUE,
    teacher_id    UUID        NOT NULL,
    center_id     UUID        NOT NULL,
    lessons       JSONB       NOT NULL DEFAULT '[]',
    current_index INT         NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (class_id, center_id)   REFERENCES classes(id, center_id)                ON DELETE CASCADE,
    CONSTRAINT fk_class_curricula_teacher_center FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE
);

-- Giáo án theo (lớp, chỉ số bài). Không có status 'none': chưa soạn = chưa
-- có row (web tự map row thiếu → "none"). submitted_by tách khỏi teacher_id
-- để giữ đúng ai bấm nộp, kể cả khi lớp đổi giáo viên phụ trách.
CREATE TABLE lesson_plans (
    id            UUID        PRIMARY KEY,
    class_id      UUID        NOT NULL,
    lesson_index  INT         NOT NULL CHECK (lesson_index >= 0),
    teacher_id    UUID        NOT NULL,
    center_id     UUID        NOT NULL,
    goal          TEXT        NOT NULL DEFAULT '',
    activities    JSONB       NOT NULL DEFAULT '[]',
    homework      TEXT        NOT NULL DEFAULT '',
    -- Chỉ metadata tên file (quyết định 260814): chưa có upload thật.
    file_name     TEXT,
    status        VARCHAR(20) NOT NULL
                      CHECK (status IN ('draft', 'pending', 'approved', 'redo')),
    redo_note     TEXT,
    owner_comment TEXT,
    submitted_by  UUID        REFERENCES teachers(id),
    submitted_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_lesson_plans_class_lesson UNIQUE (class_id, lesson_index),
    FOREIGN KEY (class_id, center_id)   REFERENCES classes(id, center_id)                ON DELETE CASCADE,
    CONSTRAINT fk_lesson_plans_teacher_center FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE
);
-- Hàng đợi duyệt của owner + chấm đỏ trên nav: đếm/liệt kê pending theo
-- center, O(số pending) bất kể lịch sử giáo án dày đến đâu.
CREATE INDEX idx_lesson_plans_pending ON lesson_plans(center_id) WHERE status = 'pending';

-- Nhận xét chung cả lớp cho một buổi — 1:1 với buổi nên session_id là PK
-- luôn, không cần id thay thế.
CREATE TABLE session_notes (
    session_id UUID        PRIMARY KEY,
    teacher_id UUID        NOT NULL,
    center_id  UUID        NOT NULL,
    body       TEXT        NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (session_id, center_id) REFERENCES class_sessions(id, center_id)         ON DELETE CASCADE,
    CONSTRAINT fk_session_notes_teacher_center FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE
);

-- Điểm + ghi chú riêng của một học sinh trong một buổi, gộp một row vì cùng
-- khoá (session, student) và cùng một đường upsert. Cả hai cột NULL được:
-- row mà score lẫn personal_note đều NULL thì service xoá luôn, giữ bảng
-- không có row rỗng. Khác attendance_records.note (ngữ nghĩa điểm danh).
CREATE TABLE session_marks (
    id            UUID         PRIMARY KEY,
    session_id    UUID         NOT NULL,
    student_id    UUID         NOT NULL,
    teacher_id    UUID         NOT NULL,
    center_id     UUID         NOT NULL,
    -- Thang 0–10 bước 0.1 như UI; NUMERIC chính xác, không trôi float.
    score         NUMERIC(4,1) CHECK (score >= 0 AND score <= 10),
    personal_note TEXT,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT uq_session_marks_session_student UNIQUE (session_id, student_id),
    FOREIGN KEY (session_id, center_id) REFERENCES class_sessions(id, center_id)         ON DELETE CASCADE,
    FOREIGN KEY (student_id, center_id) REFERENCES students(id, center_id)               ON DELETE CASCADE,
    CONSTRAINT fk_session_marks_teacher_center FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE
);
