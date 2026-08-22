-- =============================================================
-- 000007 — Center Tenancy: tenant chuyển từ teacher sang center.
--
-- Tạo bảng centers, backfill một center cá nhân cho mỗi teacher hiện có
-- (chính họ là owner — không ai mất quyền, hành vi không đổi), rồi re-key
-- toàn bộ bảng nghiệp vụ sang center_id:
--   (a) thêm cột center_id + backfill từ center của teacher sở hữu;
--   (b) UNIQUE (id, center_id) trên bảng cha + FK guard
--       (teacher_id, center_id) → center_members — DB tự chặn row gán teacher
--       chưa từng là thành viên của center đó; neo vào lịch sử membership
--       (không phải teachers) để giáo viên rời center mà dữ liệu Ở LẠI;
--   (c) FK con đổi vế tenant: (x_id, teacher_id) → (x_id, center_id).
--       Cross-teacher TRONG CÙNG center giờ hợp lệ ở DB (chủ đích — owner
--       sửa/xoá thay teacher); isolation teacher-với-teacher trong center
--       chỉ còn enforce ở query layer;
--   (d) drop FK/UNIQUE (id, teacher_id) cũ.
-- teacher_id GIỮ LẠI trên mọi bảng làm attribution (ai dạy/ai quản) và
-- scope phụ cho role teacher.
--
-- Các unique NGHIỆP VỤ theo teacher giữ nguyên ngữ nghĩa per-teacher
-- (quyết định 260811): uq_contacts_phone, uq_contacts_zalo_user,
-- uq_billing_periods, uq_notification_runs_one_active,
-- idx_class_sessions_pending — không đổi sang center.
--
-- Ngoại lệ không re-key: user_accounts, refresh_tokens (identity, không có
-- tenant key); zalo_accounts (tài khoản Zalo cá nhân, đi theo người).
-- =============================================================

-- Tenant mới của hệ thống. owner là teacher có toàn quyền đọc/ghi trong
-- center. Bất biến owner.center_id = centers.id là app-enforced (FK vòng
-- centers.owner_id ↔ teachers.center_id không khai báo được sạch).
-- owner_id NO ACTION DEFERRABLE (RESTRICT không hoãn được trong PG): center
-- và owner đầu tiên sinh ra trong cùng một transaction (đăng ký teacher mới),
-- và chiều ngược — xoá cứng teacher kèm center cá nhân — cũng phải đi trọn
-- một transaction; mọi kiểm tra dồn về commit, không thể bỏ rơi center mồ côi.
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
ALTER TABLE teachers ADD COLUMN center_id UUID REFERENCES centers(id);

-- Backfill: mỗi teacher hiện có một center cá nhân mang tên mình.
INSERT INTO centers (id, name, owner_id)
SELECT gen_random_uuid(), t.full_name, t.id FROM teachers t;
UPDATE teachers t SET center_id = c.id FROM centers c WHERE c.owner_id = t.id;

ALTER TABLE teachers ALTER COLUMN center_id SET NOT NULL;
CREATE INDEX idx_teachers_center ON teachers(center_id);

-- Lịch sử membership — anchor cho FK guard ở mọi bảng nghiệp vụ. Row sống
-- (left_at IS NULL) là membership hiện tại; row đã đóng giữ chân dữ liệu cũ:
-- giáo viên rời center thì dữ liệu Ở LẠI center cũ và vẫn ghi công họ.
-- Rời center = UPDATE left_at, KHÔNG BAO GIỜ DELETE row membership khi còn
-- dữ liệu — guard FK bên dưới sẽ CASCADE xoá toàn bộ dữ liệu của cặp
-- (teacher, center); DELETE chỉ dành cho đường xoá cứng tài khoản.
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

INSERT INTO center_members (teacher_id, center_id)
SELECT t.id, t.center_id FROM teachers t;

-- Center hiện tại của teacher phải có row membership tương ứng (sống hay đã
-- đóng là việc của query layer). DEFERRABLE: đăng ký teacher mới chèn
-- teachers trước, center_members ngay sau trong cùng transaction.
ALTER TABLE teachers ADD CONSTRAINT fk_teachers_membership
    FOREIGN KEY (id, center_id) REFERENCES center_members(teacher_id, center_id)
    DEFERRABLE INITIALLY DEFERRED;

-- =============================================================
-- (a) Thêm center_id + backfill từ center của teacher sở hữu — 16 bảng.
-- =============================================================

ALTER TABLE contacts ADD COLUMN center_id UUID;
UPDATE contacts x SET center_id = t.center_id FROM teachers t WHERE x.teacher_id = t.id;
ALTER TABLE contacts ALTER COLUMN center_id SET NOT NULL;

ALTER TABLE students ADD COLUMN center_id UUID;
UPDATE students x SET center_id = t.center_id FROM teachers t WHERE x.teacher_id = t.id;
ALTER TABLE students ALTER COLUMN center_id SET NOT NULL;

ALTER TABLE classes ADD COLUMN center_id UUID;
UPDATE classes x SET center_id = t.center_id FROM teachers t WHERE x.teacher_id = t.id;
ALTER TABLE classes ALTER COLUMN center_id SET NOT NULL;

ALTER TABLE class_schedules ADD COLUMN center_id UUID;
UPDATE class_schedules x SET center_id = t.center_id FROM teachers t WHERE x.teacher_id = t.id;
ALTER TABLE class_schedules ALTER COLUMN center_id SET NOT NULL;

ALTER TABLE enrollments ADD COLUMN center_id UUID;
UPDATE enrollments x SET center_id = t.center_id FROM teachers t WHERE x.teacher_id = t.id;
ALTER TABLE enrollments ALTER COLUMN center_id SET NOT NULL;

ALTER TABLE class_sessions ADD COLUMN center_id UUID;
UPDATE class_sessions x SET center_id = t.center_id FROM teachers t WHERE x.teacher_id = t.id;
ALTER TABLE class_sessions ALTER COLUMN center_id SET NOT NULL;

ALTER TABLE attendance_records ADD COLUMN center_id UUID;
UPDATE attendance_records x SET center_id = t.center_id FROM teachers t WHERE x.teacher_id = t.id;
ALTER TABLE attendance_records ALTER COLUMN center_id SET NOT NULL;

ALTER TABLE billing_periods ADD COLUMN center_id UUID;
UPDATE billing_periods x SET center_id = t.center_id FROM teachers t WHERE x.teacher_id = t.id;
ALTER TABLE billing_periods ALTER COLUMN center_id SET NOT NULL;

ALTER TABLE invoices ADD COLUMN center_id UUID;
UPDATE invoices x SET center_id = t.center_id FROM teachers t WHERE x.teacher_id = t.id;
ALTER TABLE invoices ALTER COLUMN center_id SET NOT NULL;

ALTER TABLE invoice_lines ADD COLUMN center_id UUID;
UPDATE invoice_lines x SET center_id = t.center_id FROM teachers t WHERE x.teacher_id = t.id;
ALTER TABLE invoice_lines ALTER COLUMN center_id SET NOT NULL;

ALTER TABLE invoice_adjustments ADD COLUMN center_id UUID;
UPDATE invoice_adjustments x SET center_id = t.center_id FROM teachers t WHERE x.teacher_id = t.id;
ALTER TABLE invoice_adjustments ALTER COLUMN center_id SET NOT NULL;

ALTER TABLE payments ADD COLUMN center_id UUID;
UPDATE payments x SET center_id = t.center_id FROM teachers t WHERE x.teacher_id = t.id;
ALTER TABLE payments ALTER COLUMN center_id SET NOT NULL;

ALTER TABLE payment_allocations ADD COLUMN center_id UUID;
UPDATE payment_allocations x SET center_id = t.center_id FROM teachers t WHERE x.teacher_id = t.id;
ALTER TABLE payment_allocations ALTER COLUMN center_id SET NOT NULL;

ALTER TABLE statements ADD COLUMN center_id UUID;
UPDATE statements x SET center_id = t.center_id FROM teachers t WHERE x.teacher_id = t.id;
ALTER TABLE statements ALTER COLUMN center_id SET NOT NULL;

ALTER TABLE notification_runs ADD COLUMN center_id UUID;
UPDATE notification_runs x SET center_id = t.center_id FROM teachers t WHERE x.teacher_id = t.id;
ALTER TABLE notification_runs ALTER COLUMN center_id SET NOT NULL;

ALTER TABLE notifications ADD COLUMN center_id UUID;
UPDATE notifications x SET center_id = t.center_id FROM teachers t WHERE x.teacher_id = t.id;
ALTER TABLE notifications ALTER COLUMN center_id SET NOT NULL;

-- =============================================================
-- (b) Ranh giới toàn vẹn mới = center.
-- UNIQUE (id, center_id) trên bảng cha làm target cho FK con; FK guard
-- (teacher_id, center_id) → center_members giữ bất biến "teacher trên row
-- đã/đang là thành viên center của row" — thay cho mọi CHECK thủ công.
-- Membership còn sống hay không do query layer kiểm khi ghi mới.
-- =============================================================

ALTER TABLE contacts            ADD CONSTRAINT uq_contacts_cid          UNIQUE (id, center_id);
ALTER TABLE students            ADD CONSTRAINT uq_students_cid          UNIQUE (id, center_id);
ALTER TABLE classes             ADD CONSTRAINT uq_classes_cid           UNIQUE (id, center_id);
ALTER TABLE enrollments         ADD CONSTRAINT uq_enrollments_cid       UNIQUE (id, center_id);
ALTER TABLE class_sessions      ADD CONSTRAINT uq_class_sessions_cid    UNIQUE (id, center_id);
ALTER TABLE billing_periods     ADD CONSTRAINT uq_billing_periods_cid   UNIQUE (id, center_id);
ALTER TABLE invoices            ADD CONSTRAINT uq_invoices_cid          UNIQUE (id, center_id);
ALTER TABLE payments            ADD CONSTRAINT uq_payments_cid          UNIQUE (id, center_id);
ALTER TABLE statements          ADD CONSTRAINT uq_statements_cid        UNIQUE (id, center_id);
ALTER TABLE notification_runs   ADD CONSTRAINT uq_notification_runs_cid UNIQUE (id, center_id);

ALTER TABLE contacts            ADD CONSTRAINT fk_contacts_teacher_center            FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE;
ALTER TABLE students            ADD CONSTRAINT fk_students_teacher_center            FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE;
ALTER TABLE classes             ADD CONSTRAINT fk_classes_teacher_center             FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE;
ALTER TABLE class_schedules     ADD CONSTRAINT fk_class_schedules_teacher_center     FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE;
ALTER TABLE enrollments         ADD CONSTRAINT fk_enrollments_teacher_center         FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE;
ALTER TABLE class_sessions      ADD CONSTRAINT fk_class_sessions_teacher_center      FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE;
ALTER TABLE attendance_records  ADD CONSTRAINT fk_attendance_records_teacher_center  FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE;
ALTER TABLE billing_periods     ADD CONSTRAINT fk_billing_periods_teacher_center     FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE;
ALTER TABLE invoices            ADD CONSTRAINT fk_invoices_teacher_center            FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE;
ALTER TABLE invoice_lines       ADD CONSTRAINT fk_invoice_lines_teacher_center       FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE;
ALTER TABLE invoice_adjustments ADD CONSTRAINT fk_invoice_adjustments_teacher_center FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE;
ALTER TABLE payments            ADD CONSTRAINT fk_payments_teacher_center            FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE;
ALTER TABLE payment_allocations ADD CONSTRAINT fk_payment_allocations_teacher_center FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE;
ALTER TABLE statements          ADD CONSTRAINT fk_statements_teacher_center          FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE;
ALTER TABLE notification_runs   ADD CONSTRAINT fk_notification_runs_teacher_center   FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE;
ALTER TABLE notifications       ADD CONSTRAINT fk_notifications_teacher_center       FOREIGN KEY (teacher_id, center_id) REFERENCES center_members(teacher_id, center_id) ON DELETE CASCADE;

-- =============================================================
-- (c) FK con đổi vế tenant sang center, giữ nguyên hành vi ON DELETE cũ.
-- =============================================================

ALTER TABLE students            ADD CONSTRAINT fk_students_contact_center            FOREIGN KEY (contact_id, center_id)    REFERENCES contacts(id, center_id)          ON DELETE RESTRICT;
ALTER TABLE class_schedules     ADD CONSTRAINT fk_class_schedules_class_center       FOREIGN KEY (class_id, center_id)      REFERENCES classes(id, center_id)           ON DELETE CASCADE;
ALTER TABLE enrollments         ADD CONSTRAINT fk_enrollments_student_center         FOREIGN KEY (student_id, center_id)    REFERENCES students(id, center_id)          ON DELETE CASCADE;
ALTER TABLE enrollments         ADD CONSTRAINT fk_enrollments_class_center           FOREIGN KEY (class_id, center_id)      REFERENCES classes(id, center_id)           ON DELETE CASCADE;
ALTER TABLE class_sessions      ADD CONSTRAINT fk_class_sessions_class_center        FOREIGN KEY (class_id, center_id)      REFERENCES classes(id, center_id)           ON DELETE CASCADE;
ALTER TABLE attendance_records  ADD CONSTRAINT fk_attendance_records_session_center  FOREIGN KEY (session_id, center_id)    REFERENCES class_sessions(id, center_id)    ON DELETE CASCADE;
ALTER TABLE attendance_records  ADD CONSTRAINT fk_attendance_records_student_center  FOREIGN KEY (student_id, center_id)    REFERENCES students(id, center_id)          ON DELETE CASCADE;
ALTER TABLE attendance_records  ADD CONSTRAINT fk_attendance_records_enrollment_center FOREIGN KEY (enrollment_id, center_id) REFERENCES enrollments(id, center_id)     ON DELETE CASCADE;
ALTER TABLE invoices            ADD CONSTRAINT fk_invoices_period_center             FOREIGN KEY (period_id, center_id)     REFERENCES billing_periods(id, center_id)   ON DELETE CASCADE;
ALTER TABLE invoices            ADD CONSTRAINT fk_invoices_student_center            FOREIGN KEY (student_id, center_id)    REFERENCES students(id, center_id)          ON DELETE RESTRICT;
ALTER TABLE invoices            ADD CONSTRAINT fk_invoices_contact_center            FOREIGN KEY (contact_id, center_id)    REFERENCES contacts(id, center_id)          ON DELETE RESTRICT;
ALTER TABLE invoice_lines       ADD CONSTRAINT fk_invoice_lines_invoice_center       FOREIGN KEY (invoice_id, center_id)    REFERENCES invoices(id, center_id)          ON DELETE CASCADE;
ALTER TABLE invoice_lines       ADD CONSTRAINT fk_invoice_lines_enrollment_center    FOREIGN KEY (enrollment_id, center_id) REFERENCES enrollments(id, center_id)       ON DELETE RESTRICT;
ALTER TABLE invoice_adjustments ADD CONSTRAINT fk_invoice_adjustments_invoice_center FOREIGN KEY (invoice_id, center_id)    REFERENCES invoices(id, center_id)          ON DELETE CASCADE;
ALTER TABLE payments            ADD CONSTRAINT fk_payments_contact_center            FOREIGN KEY (contact_id, center_id)    REFERENCES contacts(id, center_id)          ON DELETE RESTRICT;
ALTER TABLE payment_allocations ADD CONSTRAINT fk_payment_allocations_payment_center FOREIGN KEY (payment_id, center_id)    REFERENCES payments(id, center_id)          ON DELETE CASCADE;
ALTER TABLE payment_allocations ADD CONSTRAINT fk_payment_allocations_invoice_center FOREIGN KEY (invoice_id, center_id)    REFERENCES invoices(id, center_id)          ON DELETE RESTRICT;
ALTER TABLE statements          ADD CONSTRAINT fk_statements_contact_center          FOREIGN KEY (contact_id, center_id)    REFERENCES contacts(id, center_id)          ON DELETE CASCADE;
ALTER TABLE statements          ADD CONSTRAINT fk_statements_period_center           FOREIGN KEY (period_id, center_id)     REFERENCES billing_periods(id, center_id)   ON DELETE CASCADE;
ALTER TABLE notification_runs   ADD CONSTRAINT fk_notification_runs_period_center    FOREIGN KEY (billing_period_id, center_id) REFERENCES billing_periods(id, center_id) ON DELETE CASCADE;
ALTER TABLE notifications       ADD CONSTRAINT fk_notifications_statement_center     FOREIGN KEY (statement_id, center_id)  REFERENCES statements(id, center_id)        ON DELETE CASCADE;
-- SET NULL kèm column list (PG >= 15, như 000005): run bị xoá thì notification
-- chỉ mất run_id, giữ nguyên center_id — bản ghi audit sống lâu hơn batch.
ALTER TABLE notifications       ADD CONSTRAINT fk_notifications_run_center           FOREIGN KEY (run_id, center_id)        REFERENCES notification_runs(id, center_id) ON DELETE SET NULL (run_id);

-- =============================================================
-- (d) Drop ranh giới teacher cũ: FK con (x_id, teacher_id), FK đơn
-- teacher_id → teachers (FK guard ở (b) đã thay thế), rồi UNIQUE
-- (id, teacher_id) sau khi không còn FK nào tham chiếu.
-- =============================================================

ALTER TABLE students            DROP CONSTRAINT students_contact_id_teacher_id_fkey;
ALTER TABLE class_schedules     DROP CONSTRAINT class_schedules_class_id_teacher_id_fkey;
ALTER TABLE enrollments         DROP CONSTRAINT enrollments_student_id_teacher_id_fkey;
ALTER TABLE enrollments         DROP CONSTRAINT enrollments_class_id_teacher_id_fkey;
ALTER TABLE class_sessions      DROP CONSTRAINT class_sessions_class_id_teacher_id_fkey;
ALTER TABLE attendance_records  DROP CONSTRAINT attendance_records_session_id_teacher_id_fkey;
ALTER TABLE attendance_records  DROP CONSTRAINT attendance_records_student_id_teacher_id_fkey;
ALTER TABLE attendance_records  DROP CONSTRAINT attendance_records_enrollment_id_teacher_id_fkey;
ALTER TABLE invoices            DROP CONSTRAINT invoices_period_id_teacher_id_fkey;
ALTER TABLE invoices            DROP CONSTRAINT invoices_student_id_teacher_id_fkey;
ALTER TABLE invoices            DROP CONSTRAINT invoices_contact_id_teacher_id_fkey;
ALTER TABLE invoice_lines       DROP CONSTRAINT invoice_lines_invoice_id_teacher_id_fkey;
ALTER TABLE invoice_lines       DROP CONSTRAINT invoice_lines_enrollment_id_teacher_id_fkey;
ALTER TABLE invoice_adjustments DROP CONSTRAINT invoice_adjustments_invoice_id_teacher_id_fkey;
ALTER TABLE payments            DROP CONSTRAINT payments_contact_id_teacher_id_fkey;
ALTER TABLE payment_allocations DROP CONSTRAINT payment_allocations_payment_id_teacher_id_fkey;
ALTER TABLE payment_allocations DROP CONSTRAINT payment_allocations_invoice_id_teacher_id_fkey;
ALTER TABLE statements          DROP CONSTRAINT statements_contact_id_teacher_id_fkey;
ALTER TABLE statements          DROP CONSTRAINT statements_period_id_teacher_id_fkey;
ALTER TABLE notification_runs   DROP CONSTRAINT notification_runs_billing_period_id_teacher_id_fkey;
ALTER TABLE notifications       DROP CONSTRAINT notifications_statement_id_teacher_id_fkey;
ALTER TABLE notifications       DROP CONSTRAINT notifications_run_id_teacher_id_fkey;

ALTER TABLE contacts            DROP CONSTRAINT contacts_teacher_id_fkey;
ALTER TABLE students            DROP CONSTRAINT students_teacher_id_fkey;
ALTER TABLE classes             DROP CONSTRAINT classes_teacher_id_fkey;
ALTER TABLE billing_periods     DROP CONSTRAINT billing_periods_teacher_id_fkey;
ALTER TABLE notification_runs   DROP CONSTRAINT notification_runs_teacher_id_fkey;

ALTER TABLE contacts            DROP CONSTRAINT uq_contacts_tid;
ALTER TABLE students            DROP CONSTRAINT uq_students_tid;
ALTER TABLE classes             DROP CONSTRAINT uq_classes_tid;
ALTER TABLE enrollments         DROP CONSTRAINT uq_enrollments_tid;
ALTER TABLE class_sessions      DROP CONSTRAINT uq_class_sessions_tid;
ALTER TABLE billing_periods     DROP CONSTRAINT uq_billing_periods_tid;
ALTER TABLE invoices            DROP CONSTRAINT uq_invoices_tid;
ALTER TABLE payments            DROP CONSTRAINT uq_payments_tid;
ALTER TABLE statements          DROP CONSTRAINT uq_statements_tid;
ALTER TABLE notification_runs   DROP CONSTRAINT uq_notification_runs_tid;

-- =============================================================
-- Index đọc theo center — partial deleted_at IS NULL ở bảng có soft delete
-- (house rule: query đọc luôn lọc deleted_at); index thường ở 4 bảng tài
-- chính không có deleted_at. Index teacher_id hiện có GIỮ NGUYÊN: read path
-- của role teacher vẫn lọc teacher_id.
-- =============================================================

CREATE INDEX idx_contacts_center            ON contacts(center_id)            WHERE deleted_at IS NULL;
CREATE INDEX idx_students_center            ON students(center_id)            WHERE deleted_at IS NULL;
CREATE INDEX idx_classes_center             ON classes(center_id)             WHERE deleted_at IS NULL;
CREATE INDEX idx_class_schedules_center     ON class_schedules(center_id)     WHERE deleted_at IS NULL;
CREATE INDEX idx_enrollments_center         ON enrollments(center_id)         WHERE deleted_at IS NULL;
CREATE INDEX idx_class_sessions_center      ON class_sessions(center_id)      WHERE deleted_at IS NULL;
CREATE INDEX idx_attendance_records_center  ON attendance_records(center_id)  WHERE deleted_at IS NULL;
CREATE INDEX idx_billing_periods_center     ON billing_periods(center_id)     WHERE deleted_at IS NULL;
CREATE INDEX idx_invoices_center            ON invoices(center_id);
CREATE INDEX idx_invoice_lines_center       ON invoice_lines(center_id);
CREATE INDEX idx_invoice_adjustments_center ON invoice_adjustments(center_id) WHERE deleted_at IS NULL;
CREATE INDEX idx_payments_center            ON payments(center_id);
CREATE INDEX idx_payment_allocations_center ON payment_allocations(center_id);
CREATE INDEX idx_statements_center          ON statements(center_id)          WHERE deleted_at IS NULL;
CREATE INDEX idx_notification_runs_center   ON notification_runs(center_id);
CREATE INDEX idx_notifications_center       ON notifications(center_id)       WHERE deleted_at IS NULL;

-- =============================================================
-- Views tạo lại kèm center_id (giữ teacher_id cho drill-down theo GV).
-- =============================================================

DROP VIEW v_contact_balance;
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

DROP VIEW v_unbilled_attendance;
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
