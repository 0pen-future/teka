-- =============================================================
-- Sổ Lớp — Database Schema V1 (PostgreSQL 15+)
-- Bám theo PRD-quan-ly-lop-day-them-v1.md
--
-- QUY ƯỚC KIỂU DỮ LIỆU
--   VARCHAR(n) cho mọi chuỗi có ngữ nghĩa giới hạn.
--   TEXT chỉ giữ lại ở các trường tự do thật sự: note, reason,
--   cancel_reason, error_message.
--
--   Lưu ý kỹ thuật để tránh hiểu nhầm: trong PostgreSQL, VARCHAR(n) và TEXT
--   lưu trữ và chạy y hệt nhau — VARCHAR(n) chỉ là TEXT kèm một CHECK độ dài.
--   Không có lợi ích hiệu năng (khác với MySQL/SQL Server). Lý do dùng ở đây
--   là: chặn dữ liệu rác, tài liệu hoá schema cho codegen (sqlc/gorm), và
--   đồng bộ giới hạn validation giữa Go và DB.
--
-- QUY ƯỚC SOFT DELETE
--   deleted_at có ở các bảng DANH MỤC do giáo viên tự quản lý.
--   KHÔNG có ở 4 bảng tài chính lõi (invoices, invoice_lines, payments,
--   payment_allocations) — lý do ở ghi chú (i) cuối file.
--   Mọi UNIQUE trên bảng có deleted_at đều chuyển thành partial unique index
--   WHERE deleted_at IS NULL, nếu không xoá rồi tạo lại sẽ bị chặn.
--
-- Nguyên tắc khác:
--   1. Tiền lưu BIGINT đơn vị đồng. Không FLOAT/DOUBLE ở bất kỳ đâu.
--   2. Trạng thái dùng VARCHAR + CHECK thay vì native ENUM.
--   3. UUID khoá chính, khuyến nghị sinh UUIDv7 ở tầng Go.
--   4. Công nợ ghi theo HỌC SINH, thu tiền ghi theo NGƯỜI LIÊN HỆ.
--   5. TENANT = CENTER (từ migration 000007). Composite FK (id, center_id)
--      chống ghép dữ liệu chéo trung tâm. teacher_id GIỮ LẠI trên mọi bảng
--      nghiệp vụ làm attribution (ai dạy/ai quản) và scope phụ cho role
--      teacher; FK guard (teacher_id, center_id) → center_members đảm bảo
--      teacher trên row đã/đang là thành viên center của row — neo vào lịch
--      sử membership để giáo viên rời center mà dữ liệu Ở LẠI center cũ.
-- =============================================================

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- =============================================================
-- 1. ĐỊNH DANH, GIÁO VIÊN & TRUNG TÂM
--
-- Tách IDENTITY (ai đăng nhập được) khỏi HỒ SƠ NGHIỆP VỤ (ai xuất hiện
-- trong dữ liệu). V1 chỉ giáo viên đăng nhập; phụ huynh dùng link token (R5),
-- học sinh không có mặt trong sản phẩm (Non-goals).
-- =============================================================

CREATE TABLE user_accounts (
    id              UUID PRIMARY KEY,
    -- Chỉ quyết định mở app vào giao diện nào. KHÔNG dùng để phân quyền dữ
    -- liệu — phạm vi dữ liệu luôn suy ra từ teachers.id / contacts.user_id.
    role            VARCHAR(16) NOT NULL
                        CHECK (role IN ('teachers', 'parent', 'students')),
    phone           VARCHAR(20) NOT NULL,   -- định danh đăng nhập, E.164
    -- 255 là có chủ ý: bcrypt 60 ký tự, argon2id encoded ~100.
    -- Đặt sát 60 sẽ vỡ khi đổi thuật toán băm.
    password_hash   VARCHAR(255),           -- NULL nếu chỉ đăng nhập OTP
    status          VARCHAR(20) NOT NULL DEFAULT 'active'
                        CHECK (status IN ('active', 'disabled')),
    last_login_at   TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    CONSTRAINT uq_users_role UNIQUE (id, role)
);
CREATE UNIQUE INDEX uq_users_phone ON user_accounts(phone) WHERE deleted_at IS NULL;
CREATE INDEX idx_users_role ON user_accounts(role) WHERE deleted_at IS NULL;

-- Hồ sơ giáo viên. id chính là user_accounts.id nên toàn bộ FK teacher_id
-- ở các bảng phía dưới không phải sửa. center_id thêm bằng ALTER phía dưới
-- vì centers.owner_id và teachers.center_id tham chiếu vòng nhau.
CREATE TABLE teachers (
    id              UUID PRIMARY KEY REFERENCES user_accounts(id) ON DELETE CASCADE,
    full_name       VARCHAR(100) NOT NULL,
    timezone        VARCHAR(64)  NOT NULL DEFAULT 'Asia/Ho_Chi_Minh',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ
);

-- Tenant của hệ thống (000007). Mọi bảng nghiệp vụ key theo center_id;
-- teacher_id chỉ còn là attribution trong center. owner là teacher có toàn
-- quyền đọc/ghi trong center. Bất biến owner.center_id = centers.id là
-- app-enforced (FK vòng centers.owner_id ↔ teachers.center_id không khai báo
-- được sạch). owner_id NO ACTION DEFERRABLE (RESTRICT không hoãn được trong
-- PG): center và owner đầu tiên sinh ra trong cùng một transaction (đăng ký
-- teacher mới), và chiều ngược — xoá cứng teacher kèm center cá nhân — cũng
-- đi trọn một transaction; mọi kiểm tra dồn về commit.
CREATE TABLE centers (
    id          UUID PRIMARY KEY,
    name        VARCHAR(255) NOT NULL,
    owner_id    UUID NOT NULL REFERENCES teachers(id)
                    ON DELETE NO ACTION DEFERRABLE INITIALLY DEFERRED,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at  TIMESTAMPTZ
);
-- Một teacher chỉ own tối đa một center sống (center cá nhân hoặc trung tâm).
CREATE UNIQUE INDEX uq_centers_owner ON centers(owner_id) WHERE deleted_at IS NULL;

-- Membership HIỆN TẠI là cột trên teachers: một teacher thuộc đúng một center
-- tại một thời điểm.
ALTER TABLE teachers ADD COLUMN center_id UUID NOT NULL REFERENCES centers(id);
CREATE INDEX idx_teachers_center ON teachers(center_id);

-- Lịch sử membership — anchor cho FK guard ở mọi bảng nghiệp vụ. Row sống
-- (left_at IS NULL) là membership hiện tại; row đã đóng giữ chân dữ liệu cũ:
-- giáo viên rời center thì dữ liệu Ở LẠI center cũ và vẫn ghi công họ.
-- Rời center = UPDATE left_at, KHÔNG BAO GIỜ DELETE row membership khi còn
-- dữ liệu — guard FK sẽ CASCADE xoá toàn bộ dữ liệu của cặp (teacher,
-- center); DELETE chỉ dành cho đường xoá cứng tài khoản.
CREATE TABLE center_members (
    teacher_id  UUID NOT NULL REFERENCES teachers(id) ON DELETE CASCADE,
    center_id   UUID NOT NULL REFERENCES centers(id) ON DELETE CASCADE,
    joined_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    left_at     TIMESTAMPTZ,
    PRIMARY KEY (teacher_id, center_id)
);
-- Một teacher chỉ có một membership sống tại một thời điểm.
CREATE UNIQUE INDEX uq_center_members_active ON center_members(teacher_id) WHERE left_at IS NULL;
CREATE INDEX idx_center_members_center ON center_members(center_id);

-- Center hiện tại của teacher phải có row membership tương ứng (sống hay đã
-- đóng là việc của query layer). DEFERRABLE: đăng ký teacher mới chèn
-- teachers trước, center_members ngay sau trong cùng transaction.
ALTER TABLE teachers ADD CONSTRAINT fk_teachers_membership
    FOREIGN KEY (id, center_id) REFERENCES center_members(teacher_id, center_id)
    DEFERRABLE INITIALLY DEFERRED;

-- =============================================================
-- 2. NGƯỜI LIÊN HỆ & HỌC SINH
-- Quan hệ 1:n (không tạo thực thể "gia đình").
-- =============================================================

CREATE TABLE contacts (
    id              UUID PRIMARY KEY,
    teacher_id      UUID         NOT NULL,
    center_id       UUID         NOT NULL,
    -- NULL ở V1: phụ huynh không đăng nhập, chỉ mở link token (R5).
    user_id         UUID         REFERENCES user_accounts(id) ON DELETE SET NULL,
    full_name       VARCHAR(100) NOT NULL,
    phone           VARCHAR(20)  NOT NULL,
    -- Mapping sang bạn Zalo của giáo viên cho kênh zalo_personal. Giáo viên tự
    -- chọn từ picker; backend không đối chiếu với live friends list. Cả hai cột
    -- NULL khi chưa map, luôn set/clear cùng nhau. zalo_name là tên hiển thị
    -- tại thời điểm map để UI không phải refetch friends.
    zalo_user_id    VARCHAR(32),
    zalo_name       VARCHAR(100),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    CONSTRAINT fk_contacts_teacher_center
        FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE,
    CONSTRAINT uq_contacts_cid UNIQUE (id, center_id)
);
-- Trùng số trong cùng giáo viên sẽ làm vỡ việc gộp thông báo và gộp công nợ.
-- Partial: xoá rồi tạo lại cùng số phải được phép.
-- GIỮ per-teacher sau 000007 (quyết định 260811): mapping Zalo và gộp thông
-- báo đi theo tài khoản Zalo cá nhân của từng giáo viên, không theo center.
-- KHÔNG unique toàn cục — một phụ huynh có con học nhiều thầy là nhiều
-- bản ghi contacts độc lập.
CREATE UNIQUE INDEX uq_contacts_phone
    ON contacts(teacher_id, phone) WHERE deleted_at IS NULL;
-- Một bạn Zalo chỉ map vào một contact sống per teacher — map trùng nghĩa là
-- một người nhận statement link (kèm dữ liệu công nợ) của hai gia đình.
CREATE UNIQUE INDEX uq_contacts_zalo_user
    ON contacts(teacher_id, zalo_user_id)
    WHERE zalo_user_id IS NOT NULL AND deleted_at IS NULL;
CREATE INDEX idx_contacts_teacher ON contacts(teacher_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_contacts_center ON contacts(center_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_contacts_user ON contacts(user_id) WHERE user_id IS NOT NULL;

CREATE TABLE students (
    id              UUID PRIMARY KEY,
    teacher_id      UUID         NOT NULL,
    center_id       UUID         NOT NULL,
    contact_id      UUID         NOT NULL,
    -- NULL ở V1 và còn NULL rất lâu: học sinh không có mặt trong sản phẩm.
    user_id         UUID         REFERENCES user_accounts(id) ON DELETE SET NULL,
    -- Danh sách trường ĐÓNG theo Mục R1 của PRD. Không thêm birth_date,
    -- address, school, photo... nếu chưa rà soát nghĩa vụ Nghị định 13/2023.
    full_name       VARCHAR(100) NOT NULL,
    -- Nhãn phân biệt khi hai anh em cùng lớp trùng họ tên (vd "An lớp 9A").
    display_note    VARCHAR(50),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    -- KHÁC deleted_at: đây là xoá thật PII theo yêu cầu của chủ thể dữ liệu.
    -- deleted_at chỉ là ẩn khỏi danh sách, dữ liệu vẫn còn nguyên.
    -- Nghị định 13/2023 yêu cầu xoá thật, nên cần cả hai cột.
    anonymized_at   TIMESTAMPTZ,
    CONSTRAINT fk_students_teacher_center
        FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE,
    CONSTRAINT fk_students_contact_center
        FOREIGN KEY (contact_id, center_id) REFERENCES contacts(id, center_id) ON DELETE RESTRICT,
    CONSTRAINT uq_students_cid UNIQUE (id, center_id)
);
CREATE INDEX idx_students_teacher ON students(teacher_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_students_center ON students(center_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_students_contact ON students(contact_id);

-- =============================================================
-- 3. LỚP & LỊCH
-- =============================================================

CREATE TABLE classes (
    id                  UUID PRIMARY KEY,
    teacher_id          UUID         NOT NULL,
    center_id           UUID         NOT NULL,
    name                VARCHAR(100) NOT NULL,
    start_date          DATE         NOT NULL,   -- ngày khai giảng, mỗi lớp một khác
    end_date            DATE,
    -- Đơn giá MẶC ĐỊNH của lớp. Đơn giá thực tế nằm ở enrollments.
    default_unit_price  BIGINT       NOT NULL CHECK (default_unit_price >= 0),
    status              VARCHAR(20)  NOT NULL DEFAULT 'active'
                            CHECK (status IN ('active', 'archived')),
    created_at          TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at          TIMESTAMPTZ,
    CONSTRAINT fk_classes_teacher_center
        FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE,
    CONSTRAINT uq_classes_cid UNIQUE (id, center_id)
);
CREATE INDEX idx_classes_teacher ON classes(teacher_id)
    WHERE deleted_at IS NULL AND status = 'active';
CREATE INDEX idx_classes_center ON classes(center_id) WHERE deleted_at IS NULL;

CREATE TABLE class_schedules (
    id              UUID PRIMARY KEY,
    teacher_id      UUID        NOT NULL,
    center_id       UUID        NOT NULL,
    class_id        UUID        NOT NULL,
    weekday         SMALLINT    NOT NULL CHECK (weekday BETWEEN 0 AND 6), -- 0 = CN
    start_time      TIME        NOT NULL,
    duration_min    SMALLINT    NOT NULL DEFAULT 90 CHECK (duration_min > 0),
    effective_from  DATE        NOT NULL,
    effective_to    DATE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    CONSTRAINT fk_class_schedules_teacher_center
        FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE,
    CONSTRAINT fk_class_schedules_class_center
        FOREIGN KEY (class_id, center_id) REFERENCES classes(id, center_id) ON DELETE CASCADE
);
CREATE INDEX idx_class_schedules_class ON class_schedules(class_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_class_schedules_center ON class_schedules(center_id) WHERE deleted_at IS NULL;

-- =============================================================
-- 4. GHI DANH — nơi đặt ĐƠN GIÁ
-- Quyết định kiến trúc quan trọng nhất của PRD: đơn giá nằm ở đây,
-- không nằm ở classes. V1 luôn kế thừa default_unit_price, không cho sửa,
-- nhưng cấu trúc sẵn sàng cho giảm giá anh chị em ở P1.
-- =============================================================

CREATE TABLE enrollments (
    id              UUID PRIMARY KEY,
    teacher_id      UUID        NOT NULL,
    center_id       UUID        NOT NULL,
    student_id      UUID        NOT NULL,
    class_id        UUID        NOT NULL,
    started_on      DATE        NOT NULL,   -- ngày nhập học, có thể giữa chu kỳ
    ended_on        DATE,                   -- NULL = đang học
    unit_price      BIGINT      NOT NULL CHECK (unit_price >= 0),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    CONSTRAINT fk_enrollments_teacher_center
        FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE,
    CONSTRAINT fk_enrollments_student_center
        FOREIGN KEY (student_id, center_id) REFERENCES students(id, center_id) ON DELETE CASCADE,
    CONSTRAINT fk_enrollments_class_center
        FOREIGN KEY (class_id, center_id)   REFERENCES classes(id, center_id)  ON DELETE CASCADE,
    CHECK (ended_on IS NULL OR ended_on >= started_on),
    CONSTRAINT uq_enrollments_cid UNIQUE (id, center_id)
);
-- Một học sinh chỉ có MỘT ghi danh đang mở trong một lớp.
CREATE UNIQUE INDEX uq_enrollments_active
    ON enrollments(student_id, class_id) WHERE ended_on IS NULL AND deleted_at IS NULL;
CREATE INDEX idx_enrollments_class ON enrollments(class_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_enrollments_center ON enrollments(center_id) WHERE deleted_at IS NULL;

-- =============================================================
-- 5. BUỔI HỌC & ĐIỂM DANH
-- Dữ liệu gốc của toàn bộ hệ thống (North Star G4).
-- =============================================================

CREATE TABLE class_sessions (
    id                      UUID PRIMARY KEY,
    teacher_id              UUID        NOT NULL,
    center_id               UUID        NOT NULL,
    class_id                UUID        NOT NULL,
    session_date            DATE        NOT NULL,
    start_time              TIME,
    status                  VARCHAR(20) NOT NULL DEFAULT 'planned'
                                CHECK (status IN ('planned', 'held', 'cancelled')),
    cancel_reason           TEXT,
    -- Buổi chỉ được tính tiền khi đã điểm danh xong.
    attendance_confirmed_at TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Huỷ buổi dùng status='cancelled' (giữ được lý do, hiện cho phụ huynh).
    -- deleted_at chỉ dành cho buổi tạo nhầm.
    deleted_at              TIMESTAMPTZ,
    CONSTRAINT fk_class_sessions_teacher_center
        FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE,
    CONSTRAINT fk_class_sessions_class_center
        FOREIGN KEY (class_id, center_id) REFERENCES classes(id, center_id) ON DELETE CASCADE,
    CONSTRAINT uq_class_sessions_cid UNIQUE (id, center_id),
    CHECK (status <> 'cancelled' OR attendance_confirmed_at IS NULL)
);
CREATE UNIQUE INDEX uq_class_sessions_per_day
    ON class_sessions(class_id, session_date) WHERE deleted_at IS NULL;
-- Truy vấn nóng nhất: "buổi nào đã qua mà chưa điểm danh"
-- (cảnh báo ở R2, điều kiện chặn chốt sổ ở R4). Bao gồm cả 'planned' lẫn
-- 'held' vì buổi bị quên xác nhận vẫn còn ở 'planned' — đúng trường hợp
-- cảnh báo tồn tại để bắt (widened bởi migration 000003).
-- GIỮ theo teacher_id: đây là cảnh báo tác nghiệp của từng giáo viên.
CREATE INDEX idx_class_sessions_pending
    ON class_sessions(teacher_id, session_date)
    WHERE status IN ('held', 'planned') AND attendance_confirmed_at IS NULL AND deleted_at IS NULL;
CREATE INDEX idx_class_sessions_class_date ON class_sessions(class_id, session_date);
CREATE INDEX idx_class_sessions_center ON class_sessions(center_id) WHERE deleted_at IS NULL;

-- Ghi nhận ĐẦY ĐỦ mọi học sinh của buổi, kể cả người có mặt.
-- Lý do không chỉ lưu người vắng: cần phân biệt "có mặt" với "chưa điểm danh".
-- UI vẫn là 1 chạm — server tự sinh toàn bộ dòng 'present' khi xác nhận.
CREATE TABLE attendance_records (
    id              UUID PRIMARY KEY,
    teacher_id      UUID        NOT NULL,
    center_id       UUID        NOT NULL,
    session_id      UUID        NOT NULL,
    student_id      UUID        NOT NULL,
    enrollment_id   UUID        NOT NULL,
    status          VARCHAR(20) NOT NULL
                        CHECK (status IN ('present', 'absent', 'excused')),
    -- V1: present và absent đều tính tiền; 'excused' (nghỉ phép) dành cho P1.
    billable        BOOLEAN     NOT NULL DEFAULT true,
    note            TEXT,
    recorded_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- CẢNH BÁO: chỉ dùng khi học sinh bị thêm nhầm vào buổi. Học sinh vắng
    -- KHÔNG phải xoá mềm — dùng status='absent'. Xoá mềm bản ghi điểm danh
    -- làm lệch số buổi tính tiền, tức là làm sai tiền gửi cho phụ huynh.
    deleted_at      TIMESTAMPTZ,
    CONSTRAINT fk_attendance_records_teacher_center
        FOREIGN KEY (teacher_id, center_id)    REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE,
    CONSTRAINT fk_attendance_records_session_center
        FOREIGN KEY (session_id, center_id)    REFERENCES class_sessions(id, center_id) ON DELETE CASCADE,
    CONSTRAINT fk_attendance_records_student_center
        FOREIGN KEY (student_id, center_id)    REFERENCES students(id, center_id)       ON DELETE CASCADE,
    CONSTRAINT fk_attendance_records_enrollment_center
        FOREIGN KEY (enrollment_id, center_id) REFERENCES enrollments(id, center_id)    ON DELETE CASCADE
);
CREATE UNIQUE INDEX uq_attendance_records
    ON attendance_records(session_id, student_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_attendance_records_enrollment ON attendance_records(enrollment_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_attendance_records_student ON attendance_records(student_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_attendance_records_center ON attendance_records(center_id) WHERE deleted_at IS NULL;

-- =============================================================
-- 6. KỲ CHỐT SỔ & CÔNG NỢ
-- =============================================================

CREATE TABLE billing_periods (
    id              UUID        PRIMARY KEY,
    teacher_id      UUID        NOT NULL,
    center_id       UUID        NOT NULL,
    year            SMALLINT    NOT NULL,
    month           SMALLINT    NOT NULL CHECK (month BETWEEN 1 AND 12),
    period_start    DATE        NOT NULL,
    period_end      DATE        NOT NULL,
    status          VARCHAR(20) NOT NULL DEFAULT 'open'
                        CHECK (status IN ('open', 'closed')),
    closed_at       TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    CONSTRAINT fk_billing_periods_teacher_center
        FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE,
    CONSTRAINT uq_billing_periods_cid UNIQUE (id, center_id)
);
-- GIỮ per-teacher sau 000007 (quyết định 260811): chu kỳ chốt sổ hiện hành
-- theo từng giáo viên; đổi sang per-center là đổi ngữ nghĩa nghiệp vụ.
CREATE UNIQUE INDEX uq_billing_periods
    ON billing_periods(teacher_id, year, month) WHERE deleted_at IS NULL;
CREATE INDEX idx_billing_periods_center ON billing_periods(center_id) WHERE deleted_at IS NULL;

-- Công nợ theo HỌC SINH. Snapshot bất biến sau khi kỳ đóng.
-- KHÔNG có deleted_at — huỷ phiếu thu dùng status='void'. Xem ghi chú (i).
CREATE TABLE invoices (
    id                  UUID PRIMARY KEY,
    teacher_id          UUID        NOT NULL,
    center_id           UUID        NOT NULL,
    period_id           UUID        NOT NULL,
    student_id          UUID        NOT NULL,
    -- Contact tại thời điểm chốt. Học sinh có thể đổi người liên hệ sau này
    -- nhưng phiếu thu cũ phải giữ nguyên đầu mối gốc.
    contact_id          UUID        NOT NULL,
    -- Snapshot tên tại thời điểm chốt. Bắt buộc, không phải tiện lợi:
    -- khi job retention xoá cứng students/contacts, sổ sách tài chính vẫn phải
    -- đọc được. Nếu chỉ có FK thì xoá xong phiếu thu trở thành vô danh.
    student_name        VARCHAR(100) NOT NULL,
    contact_name        VARCHAR(100) NOT NULL,
    opening_balance     BIGINT      NOT NULL DEFAULT 0,  -- nợ cũ mang sang
    current_charge      BIGINT      NOT NULL DEFAULT 0,  -- phát sinh kỳ này
    adjustment_total    BIGINT      NOT NULL DEFAULT 0,  -- điều chỉnh tay (+/-)
    total_due           BIGINT      NOT NULL DEFAULT 0,
    paid_amount         BIGINT      NOT NULL DEFAULT 0,
    status              VARCHAR(20) NOT NULL DEFAULT 'draft'
                            CHECK (status IN ('draft','issued','partially_paid','paid','void')),
    void_reason         TEXT,
    voided_at           TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_invoices_teacher_center
        FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE,
    CONSTRAINT fk_invoices_period_center
        FOREIGN KEY (period_id, center_id)  REFERENCES billing_periods(id, center_id) ON DELETE CASCADE,
    CONSTRAINT fk_invoices_student_center
        FOREIGN KEY (student_id, center_id) REFERENCES students(id, center_id)        ON DELETE RESTRICT,
    CONSTRAINT fk_invoices_contact_center
        FOREIGN KEY (contact_id, center_id) REFERENCES contacts(id, center_id)        ON DELETE RESTRICT,
    CONSTRAINT uq_invoices     UNIQUE (period_id, student_id),
    CONSTRAINT uq_invoices_cid UNIQUE (id, center_id),
    CHECK (paid_amount >= 0),
    CHECK (total_due = opening_balance + current_charge + adjustment_total),
    CHECK (status <> 'void' OR voided_at IS NOT NULL)
);
CREATE INDEX idx_invoices_contact_period ON invoices(contact_id, period_id);
CREATE INDEX idx_invoices_unpaid
    ON invoices(teacher_id, period_id)
    WHERE status IN ('issued', 'partially_paid');
CREATE INDEX idx_invoices_center ON invoices(center_id);

-- Một dòng cho mỗi ghi danh. Học sinh học 2 lớp -> 2 dòng, 1 invoices.
-- KHÔNG có deleted_at: xoá mềm một dòng làm tổng invoices không còn khớp
-- với chi tiết, tức là con số gửi phụ huynh không giải thích được.
-- Sửa số liệu dùng invoice_adjustments (có lý do, có vết).
CREATE TABLE invoice_lines (
    id              UUID PRIMARY KEY,
    teacher_id      UUID         NOT NULL,
    center_id       UUID         NOT NULL,
    invoice_id      UUID         NOT NULL,
    enrollment_id   UUID         NOT NULL,
    class_name      VARCHAR(100) NOT NULL,  -- snapshot, phòng khi lớp đổi tên
    billable_count  INT          NOT NULL CHECK (billable_count >= 0),
    absent_count    INT          NOT NULL DEFAULT 0,
    unit_price      BIGINT       NOT NULL,  -- snapshot đơn giá lúc chốt
    amount          BIGINT       NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    CONSTRAINT fk_invoice_lines_teacher_center
        FOREIGN KEY (teacher_id, center_id)    REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE,
    CONSTRAINT fk_invoice_lines_invoice_center
        FOREIGN KEY (invoice_id, center_id)    REFERENCES invoices(id, center_id)    ON DELETE CASCADE,
    CONSTRAINT fk_invoice_lines_enrollment_center
        FOREIGN KEY (enrollment_id, center_id) REFERENCES enrollments(id, center_id) ON DELETE RESTRICT,
    CONSTRAINT uq_invoice_line UNIQUE (invoice_id, enrollment_id),
    CHECK (amount = billable_count * unit_price)
);
CREATE INDEX idx_invoice_lines_invoice ON invoice_lines(invoice_id);
CREATE INDEX idx_invoice_lines_center ON invoice_lines(center_id);

-- Điều chỉnh tay. R4 yêu cầu sửa được từng dòng KÈM LÝ DO.
-- Cũng là nơi ghi nhận hệ quả của việc sửa điểm danh sau khi kỳ đã đóng (Q5).
CREATE TABLE invoice_adjustments (
    id                  UUID PRIMARY KEY,
    teacher_id          UUID        NOT NULL,
    center_id           UUID        NOT NULL,
    invoice_id          UUID        NOT NULL,
    amount              BIGINT      NOT NULL,   -- âm = giảm trừ
    reason              TEXT        NOT NULL,
    -- Nếu điều chỉnh sinh ra do sửa điểm danh của kỳ đã khoá, trỏ về buổi gốc.
    source_session_id   UUID,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Huỷ một điều chỉnh: tạo điều chỉnh ngược dấu, không xoá.
    -- deleted_at chỉ cho trường hợp nhập nhầm và chưa gửi thông báo.
    deleted_at          TIMESTAMPTZ,
    CONSTRAINT fk_invoice_adjustments_teacher_center
        FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE,
    CONSTRAINT fk_invoice_adjustments_invoice_center
        FOREIGN KEY (invoice_id, center_id) REFERENCES invoices(id, center_id) ON DELETE CASCADE,
    CHECK (length(btrim(reason)) > 0)
);
CREATE INDEX idx_invoice_adjustments_invoice ON invoice_adjustments(invoice_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_invoice_adjustments_center ON invoice_adjustments(center_id) WHERE deleted_at IS NULL;

-- =============================================================
-- 7. THU TIỀN — trục NGƯỜI LIÊN HỆ
-- KHÔNG có deleted_at ở cả hai bảng. Xem ghi chú (i).
-- =============================================================

CREATE TABLE payments (
    id                  UUID PRIMARY KEY,
    teacher_id          UUID        NOT NULL,
    center_id           UUID        NOT NULL,
    contact_id          UUID        NOT NULL,
    amount              BIGINT      NOT NULL CHECK (amount > 0),
    method              VARCHAR(16) NOT NULL DEFAULT 'transfer'
                            CHECK (method IN ('cash', 'transfer', 'other')),
    received_on         DATE        NOT NULL,
    -- Nội dung chuyển khoản, phục vụ đối soát bán tự động ở P1.
    reference_code      VARCHAR(50),
    note                TEXT,
    -- Ghi nhầm một khoản thu thì đảo bút toán, không xoá:
    -- tạo bản ghi mới trỏ reverses_payment_id về bản ghi gốc.
    reverses_payment_id UUID        REFERENCES payments(id),
    reversed_at         TIMESTAMPTZ,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_payments_teacher_center
        FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE,
    CONSTRAINT fk_payments_contact_center
        FOREIGN KEY (contact_id, center_id) REFERENCES contacts(id, center_id) ON DELETE RESTRICT,
    CONSTRAINT uq_payments_cid UNIQUE (id, center_id)
);
CREATE INDEX idx_payments_contact ON payments(contact_id, received_on DESC);
CREATE INDEX idx_payments_center ON payments(center_id);

-- Cầu nối hai trục: một khoản thu của phụ huynh phân bổ vào nhiều invoices
-- của nhiều con. Hiện thực hoá quy tắc ở Q8 của PRD.
CREATE TABLE payment_allocations (
    id              UUID PRIMARY KEY,
    teacher_id      UUID        NOT NULL,
    center_id       UUID        NOT NULL,
    payment_id      UUID        NOT NULL,
    invoice_id      UUID        NOT NULL,
    amount          BIGINT      NOT NULL CHECK (amount > 0),
    -- 'auto' = phân bổ theo quy tắc mặc định; 'manual' = giáo viên override.
    -- Tỉ lệ 'manual' cao nghĩa là quy tắc mặc định sai với thực tế (dữ liệu Q8).
    allocated_by    VARCHAR(16) NOT NULL DEFAULT 'auto'
                        CHECK (allocated_by IN ('auto', 'manual')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT fk_payment_allocations_teacher_center
        FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE,
    CONSTRAINT fk_payment_allocations_payment_center
        FOREIGN KEY (payment_id, center_id) REFERENCES payments(id, center_id) ON DELETE CASCADE,
    CONSTRAINT fk_payment_allocations_invoice_center
        FOREIGN KEY (invoice_id, center_id) REFERENCES invoices(id, center_id) ON DELETE RESTRICT,
    CONSTRAINT uq_payment_allocations UNIQUE (payment_id, invoice_id)
);
CREATE INDEX idx_payment_allocations_invoice ON payment_allocations(invoice_id);
CREATE INDEX idx_payment_allocations_center ON payment_allocations(center_id);

-- =============================================================
-- 8. BÁO CÁO GỬI PHỤ HUYNH
-- Đơn vị là CONTACT + PERIOD, không phải students (R5).
-- =============================================================

CREATE TABLE statements (
    id              UUID PRIMARY KEY,
    teacher_id      UUID        NOT NULL,
    center_id       UUID        NOT NULL,
    contact_id      UUID        NOT NULL,
    period_id       UUID        NOT NULL,
    -- KHÔNG lưu token thô. Lưu SHA-256; server hash token trong URL rồi tra cứu.
    -- Nếu dump DB bị lộ thì kẻ tấn công vẫn không mở được link nào.
    token_hash      BYTEA       NOT NULL,
    expires_at      TIMESTAMPTZ NOT NULL,
    total_due       BIGINT      NOT NULL,   -- snapshot lúc phát hành
    -- Metric leading: tỉ lệ phụ huynh mở link.
    first_viewed_at TIMESTAMPTZ,
    last_viewed_at  TIMESTAMPTZ,
    view_count      INT         NOT NULL DEFAULT 0,
    revoked_at      TIMESTAMPTZ,            -- thu hồi link, khác với xoá
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    CONSTRAINT fk_statements_teacher_center
        FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE,
    CONSTRAINT fk_statements_contact_center
        FOREIGN KEY (contact_id, center_id) REFERENCES contacts(id, center_id)        ON DELETE CASCADE,
    CONSTRAINT fk_statements_period_center
        FOREIGN KEY (period_id, center_id)  REFERENCES billing_periods(id, center_id) ON DELETE CASCADE,
    CONSTRAINT uq_statements_cid UNIQUE (id, center_id)
);
CREATE UNIQUE INDEX uq_statements
    ON statements(contact_id, period_id) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX uq_statements_token ON statements(token_hash);
CREATE INDEX idx_statements_center ON statements(center_id) WHERE deleted_at IS NULL;

-- Một batch gửi zalo_personal có nhịp giãn cách (paced). Counters
-- (total/sent/failed) luôn derive bằng COUNT trên notifications theo run_id —
-- lưu counter ở đây là nguồn sự thật thứ hai sẽ drift. Ngữ nghĩa status:
-- 'interrupted' = process chết giữa run (rows còn queued, giáo viên resume thủ
-- công); 'expired' = phiên Zalo chết giữa run (rows còn lại chuyển failed).
CREATE TABLE notification_runs (
    id                 UUID        PRIMARY KEY,
    teacher_id         UUID        NOT NULL,
    center_id          UUID        NOT NULL,
    billing_period_id  UUID        NOT NULL,
    purpose            VARCHAR(20) NOT NULL DEFAULT 'statements'
                           CHECK (purpose IN ('statements', 'reminder')),
    status             VARCHAR(20) NOT NULL DEFAULT 'running'
                           CHECK (status IN ('running', 'completed', 'interrupted', 'expired')),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at        TIMESTAMPTZ,
    CONSTRAINT fk_notification_runs_teacher_center
        FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE,
    CONSTRAINT fk_notification_runs_period_center
        FOREIGN KEY (billing_period_id, center_id)
        REFERENCES billing_periods(id, center_id) ON DELETE CASCADE,
    CONSTRAINT uq_notification_runs_cid UNIQUE (id, center_id)
);
CREATE INDEX idx_notification_runs_teacher ON notification_runs(teacher_id);
CREATE INDEX idx_notification_runs_center ON notification_runs(center_id);
-- Guard trong DB vì in-process guard không thấy instance API thứ hai
-- (deploy chồng lấn, scale-out nhầm): hai pass chạy song song sẽ DM cùng
-- phụ huynh hai lần từ tài khoản Zalo cá nhân. GIỮ per-teacher: mỗi giáo
-- viên gửi từ tài khoản Zalo riêng nên nhịp gửi là của từng người.
CREATE UNIQUE INDEX uq_notification_runs_one_active
    ON notification_runs(teacher_id)
    WHERE status = 'running';

CREATE TABLE notifications (
    id                  UUID PRIMARY KEY,
    teacher_id          UUID        NOT NULL,
    center_id           UUID        NOT NULL,
    statement_id        UUID        NOT NULL,
    channel             VARCHAR(20) NOT NULL
                            CHECK (channel IN ('zalo_zns', 'zalo_manual', 'sms', 'zalo_personal')),
    purpose             VARCHAR(20) NOT NULL DEFAULT 'statements'
                            CHECK (purpose IN ('statements', 'reminder')),
    status              VARCHAR(20) NOT NULL DEFAULT 'queued'
                            CHECK (status IN ('queued','sent','delivered','failed')),
    provider_msg_id     VARCHAR(100),
    error_message       TEXT,
    sent_at             TIMESTAMPTZ,
    -- Thuộc về một run gửi hàng loạt (kênh zalo_personal); NULL với gửi lẻ.
    -- SET NULL chứ không CASCADE: notification là bản ghi audit của một tin đã
    -- gửi (hoặc gửi hỏng) tới phụ huynh, sống lâu hơn batch của nó.
    run_id              UUID,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at          TIMESTAMPTZ,
    CONSTRAINT fk_notifications_teacher_center
        FOREIGN KEY (teacher_id, center_id)   REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE,
    CONSTRAINT fk_notifications_statement_center
        FOREIGN KEY (statement_id, center_id) REFERENCES statements(id, center_id) ON DELETE CASCADE,
    -- FK composite: DB tự chặn notification trỏ vào run của trung tâm khác —
    -- tiến độ run derive bằng COUNT trên các row này nên link chéo tenant sẽ
    -- trộn số liệu. SET NULL kèm column list (PG >= 15) giữ nguyên center_id
    -- khi run bị xoá.
    CONSTRAINT fk_notifications_run_center
        FOREIGN KEY (run_id, center_id)
        REFERENCES notification_runs(id, center_id) ON DELETE SET NULL (run_id)
);
CREATE INDEX idx_notifications_statement ON notifications(statement_id);
CREATE INDEX idx_notifications_retry ON notifications(status)
    WHERE status IN ('queued','failed') AND deleted_at IS NULL;
CREATE INDEX idx_notifications_run ON notifications(run_id) WHERE run_id IS NOT NULL;
CREATE INDEX idx_notifications_center ON notifications(center_id) WHERE deleted_at IS NULL;

-- =============================================================
-- 9. VIEW HỖ TRỢ
-- =============================================================

-- Bảng thu tiền chế độ "xem theo người liên hệ" (R7, mặc định).
-- center_id cho scope owner; teacher_id giữ lại cho drill-down theo giáo viên.
CREATE VIEW v_contact_balance AS
SELECT
    i.teacher_id,
    i.center_id,
    i.period_id,
    i.contact_id,
    count(*)                            AS student_count,
    sum(i.total_due)                    AS total_due,
    sum(i.paid_amount)                  AS total_paid,
    sum(i.total_due - i.paid_amount)    AS outstanding
FROM invoices i
WHERE i.status <> 'void'
GROUP BY i.teacher_id, i.center_id, i.period_id, i.contact_id;

-- Nền tảng cho bảng "tiền đang thất thoát" (P1):
-- buổi đã dạy, đã điểm danh, nhưng chưa nằm trong invoice_lines nào.
CREATE VIEW v_unbilled_attendance AS
SELECT
    a.teacher_id,
    a.center_id,
    a.enrollment_id,
    a.student_id,
    cs.session_date,
    e.unit_price
FROM attendance_records a
JOIN class_sessions cs ON cs.id = a.session_id
JOIN enrollments    e  ON e.id = a.enrollment_id
WHERE a.billable = true
  AND a.deleted_at IS NULL
  AND cs.deleted_at IS NULL
  AND cs.status = 'held'
  AND cs.attendance_confirmed_at IS NOT NULL
  AND NOT EXISTS (
      SELECT 1
      FROM invoice_lines il
      JOIN invoices inv       ON inv.id = il.invoice_id
      JOIN billing_periods bp ON bp.id = inv.period_id
      WHERE il.enrollment_id = a.enrollment_id
        AND cs.session_date BETWEEN bp.period_start AND bp.period_end
        AND inv.status <> 'void'
  );

-- =============================================================
-- 10. XOÁ DỮ LIỆU CÁ NHÂN (Nghị định 13/2023)
--
-- Không đặt logic trong DB. Việc này do job định kỳ ở backend (Go) thực hiện,
-- nhất quán với nguyên tắc "không nhét nghiệp vụ vào DB" ở ghi chú (l).
--
-- Job cần làm gì, và ràng buộc phải tuân theo — xem ghi chú (q) cuối file.
-- =============================================================

-- =============================================================
-- GHI CHÚ THIẾT KẾ
-- =============================================================
--
-- (i) VÌ SAO 4 BẢNG TÀI CHÍNH KHÔNG CÓ deleted_at
--     invoices, invoice_lines, payments, payment_allocations.
--     Không phải vì quên, mà vì soft delete ở đây gây sai số tiền:
--       - invoice_lines bị xoá mềm -> tổng invoices không khớp chi tiết,
--         phụ huynh hỏi "sao ra số này" thì không giải thích được.
--       - payments bị xoá mềm -> sổ thu không đối chiếu được với sao kê
--         ngân hàng. Kế toán không bao giờ xoá bút toán, họ đảo bút toán.
--       - chỉ cần MỘT query quên "AND deleted_at IS NULL" là gửi nhầm số
--         tiền cho phụ huynh. Toàn bộ PRD xoay quanh niềm tin vào con số này.
--     Thay thế: invoices.status='void' + voided_at + void_reason;
--     payments.reverses_payment_id (bút toán đảo);
--     invoice_adjustments ngược dấu.
--     Kết quả tương đương soft delete nhưng có vết và có lý do.
--
-- (j) MỌI UNIQUE ĐÃ CHUYỂN THÀNH PARTIAL INDEX
--     Trên bảng có deleted_at, UNIQUE thường sẽ chặn việc xoá rồi tạo lại
--     (vd xoá học sinh rồi nhận lại vào cùng lớp). Tất cả đã đổi sang
--     CREATE UNIQUE INDEX ... WHERE deleted_at IS NULL.
--     Hệ quả cần nhớ khi viết Go: mọi truy vấn đọc PHẢI có deleted_at IS NULL.
--     Khuyến nghị dùng repository layer hoặc RLS để ép, không tin vào kỷ luật.
--
-- (k) MÂU THUẪN CHƯA GIẢI TRONG PRD
--     R5 yêu cầu "phụ huynh mở link cũ thấy số liệu đã cập nhật", nhưng
--     invoices là snapshot bất biến sau khi kỳ đóng. Hai cách, phải chọn một:
--       - Kỳ CHƯA thanh toán: tính lại invoices tại chỗ, gửi lại thông báo.
--       - Kỳ ĐÃ thanh toán: sinh invoice_adjustments sang kỳ sau.
--     Schema hỗ trợ cả hai. Quyết định thuộc Q5.
--
-- (l) VÌ SAO KHÔNG DÙNG TRIGGER TÍNH invoices.total_due
--     CHECK constraint đã giữ nhất quán số học. Phép tính đặt ở Go để test
--     được và log được từng bước. Trigger tính tiền là thứ khó debug nhất
--     khi khách hàng báo số sai.
--
-- (m) ROW LEVEL SECURITY
--     Nên bật trên mọi bảng có center_id trước khi có người dùng thật.
--     Policy theo tenant mới: USING (center_id = current_setting('app.center_id')::uuid
--                                    AND deleted_at IS NULL);
--     role teacher thêm điều kiện teacher_id ở query layer (owner thì không).
--     Gộp luôn điều kiện soft delete vào policy là cách rẻ nhất để không
--     phụ thuộc vào việc lập trình viên nhớ.
--
-- (n) VỀ MÔ HÌNH ROLE
--     user_accounts có sẵn ba role nhưng V1 CHỈ tạo role='teachers'.
--     role chỉ quyết định mở giao diện nào; phân quyền dữ liệu luôn suy ra
--     từ teachers.id và contacts.user_id. Một người vừa dạy thêm vừa có con
--     học thầy khác sẽ có cả hai vai — đừng tin vào cột role đơn.
--
-- (o) TENANT = CENTER (migration 000007)
--     Hướng nâng cấp trợ giảng/quản lý cũ đã hiện thực hoá bằng centers:
--       - Mỗi teacher hiện có được backfill một center cá nhân (owner = chính
--         họ) nên hành vi cũ không đổi; teachers.center_id NOT NULL,
--         1 teacher = 1 center.
--       - Ranh giới toàn vẹn DB là center: FK con (x_id, center_id) +
--         FK guard (teacher_id, center_id) → center_members. Guard neo vào
--         LỊCH SỬ membership, không phải teachers: giáo viên rời center thì
--         dữ liệu Ở LẠI center cũ (row membership đóng bằng left_at giữ chân
--         FK), còn membership sống hay không do query layer kiểm khi ghi mới.
--         Cross-teacher TRONG CÙNG center hợp lệ ở DB (chủ đích — owner
--         đọc/ghi thay giáo viên); isolation teacher-với-teacher trong center
--         enforce ở query layer.
--       - Unique NGHIỆP VỤ giữ per-teacher (uq_contacts_phone,
--         uq_billing_periods, uq_contacts_zalo_user): billing và mapping Zalo
--         hiện hành theo từng giáo viên, đổi sang center là đổi ngữ nghĩa.
--       - Đánh đổi đã chấp nhận: V1 không có created_by/confirmed_by nên dữ
--         liệu cũ không truy vết được ai thao tác.
--     Ngoại lệ không re-key: user_accounts, refresh_tokens (identity);
--     zalo_accounts (tài khoản Zalo cá nhân, đi theo người).
--
-- (q) JOB XOÁ DỮ LIỆU CÁ NHÂN (chạy ở backend, không đặt trong DB)
--
--     Ba việc khác nhau, đừng gộp làm một:
--       1. deleted_at  — giáo viên ẩn một bản ghi khỏi danh sách. Dữ liệu còn
--                        nguyên, phục hồi được. Không liên quan pháp lý.
--       2. anonymized_at — xoá PII nhưng giữ khoá ngoại. Dùng khi bản ghi
--                        còn bị invoices tham chiếu.
--       3. hard delete — xoá hẳn dòng. Chỉ làm được sau khi hết nghĩa vụ lưu
--                        trữ và không còn tham chiếu.
--
--     RÀNG BUỘC PHẢI BIẾT TRƯỚC KHI VIẾT JOB:
--     invoices.student_id và invoices.contact_id đang là ON DELETE RESTRICT.
--     Nghĩa là DELETE thẳng một students còn phiếu thu sẽ bị DB chặn.
--     Đây là chủ ý — chặn còn hơn để sổ sách mất dòng.
--     Muốn xoá cứng thật thì phải theo đúng thứ tự:
--       a. invoices.student_name / contact_name đã snapshot sẵn (xem mục 6),
--          nên sổ sách vẫn đọc được sau khi xoá.
--       b. Xoá theo thứ tự: attendance_records -> enrollments -> students -> contacts.
--          invoices và invoice_lines KHÔNG xoá.
--       c. Trước khi xoá, đổi FK invoices.student_id / contact_id sang
--          nullable + ON DELETE SET NULL. Nếu không muốn đổi FK thì chỉ
--          làm được bước ẩn danh (mục 2), không làm được hard delete.
--
--     CHÍNH SÁCH LƯU TRỮ — CHƯA QUYẾT, cần trả lời trước khi bật job:
--       - Giữ dữ liệu bao lâu sau khi học sinh nghỉ? Sổ sách kế toán thường
--         có nghĩa vụ lưu trữ dài hơn nhiều so với dữ liệu cá nhân.
--       - Xoá tự động theo thời hạn, hay chỉ xoá khi có yêu cầu của phụ huynh?
--       - Job xoá phải ghi log (ai/khi nào/bao nhiêu dòng) để chứng minh
--         đã thực hiện. Log này không được chứa chính PII vừa xoá.
--     Đây là quyết định pháp lý, không phải kỹ thuật. Thuộc Q2 của PRD.
--
-- (r) CHƯA CÓ TRONG V1 (thiết kế để không chặn đường)
--     - excused / makeup session: đã chừa CHECK 'excused' và cột billable
--     - giảm giá anh chị em: ghi đè enrollments.unit_price
--     - liên hệ phụ (n:n): thêm bảng contact_student, chuyển
--       students.contact_id thành primary_contact_id
--     - xác nhận đã đọc: thêm acknowledged_at vào statements
