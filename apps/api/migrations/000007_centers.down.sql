-- =============================================================
-- 000007 down — khôi phục tenant theo teacher như trước migration.
-- Thứ tự ngược với up: dựng lại UNIQUE (id, teacher_id) trước để FK con cũ
-- có chỗ tham chiếu, gắn lại FK cũ, rồi mới gỡ ranh giới center và cột
-- center_id. Không mất dữ liệu: center_id chỉ là cột suy ra, bỏ đi là xong.
--
-- CỬA SỔ ROLLBACK CÓ HẠN: các FK (x_id, teacher_id) gắn lại được chỉ khi mọi
-- chuỗi cha-con vẫn cùng một teacher. Khi đã có dữ liệu cross-teacher trong
-- một center (owner ghi thay giáo viên), lệnh gắn FK bên dưới sẽ fail và cả
-- migration rollback — fail to hơn là im lặng; lúc đó chỉ còn đường sửa tiến.
-- =============================================================

DROP VIEW v_contact_balance;
DROP VIEW v_unbilled_attendance;

-- Anchor cũ cho FK con (id, teacher_id).
ALTER TABLE contacts          ADD CONSTRAINT uq_contacts_tid          UNIQUE (id, teacher_id);
ALTER TABLE students          ADD CONSTRAINT uq_students_tid          UNIQUE (id, teacher_id);
ALTER TABLE classes           ADD CONSTRAINT uq_classes_tid           UNIQUE (id, teacher_id);
ALTER TABLE enrollments       ADD CONSTRAINT uq_enrollments_tid       UNIQUE (id, teacher_id);
ALTER TABLE class_sessions    ADD CONSTRAINT uq_class_sessions_tid    UNIQUE (id, teacher_id);
ALTER TABLE billing_periods   ADD CONSTRAINT uq_billing_periods_tid   UNIQUE (id, teacher_id);
ALTER TABLE invoices          ADD CONSTRAINT uq_invoices_tid          UNIQUE (id, teacher_id);
ALTER TABLE payments          ADD CONSTRAINT uq_payments_tid          UNIQUE (id, teacher_id);
ALTER TABLE statements        ADD CONSTRAINT uq_statements_tid        UNIQUE (id, teacher_id);
ALTER TABLE notification_runs ADD CONSTRAINT uq_notification_runs_tid UNIQUE (id, teacher_id);

-- FK con (x_id, teacher_id) như baseline/000005 — để không tên cho PG tự đặt
-- (<table>_<col1>_<col2>_fkey), khớp tên mà up sẽ drop ở vòng migrate sau.
ALTER TABLE students            ADD FOREIGN KEY (contact_id, teacher_id)    REFERENCES contacts(id, teacher_id)          ON DELETE RESTRICT;
ALTER TABLE class_schedules     ADD FOREIGN KEY (class_id, teacher_id)      REFERENCES classes(id, teacher_id)           ON DELETE CASCADE;
ALTER TABLE enrollments         ADD FOREIGN KEY (student_id, teacher_id)    REFERENCES students(id, teacher_id)          ON DELETE CASCADE;
ALTER TABLE enrollments         ADD FOREIGN KEY (class_id, teacher_id)      REFERENCES classes(id, teacher_id)           ON DELETE CASCADE;
ALTER TABLE class_sessions      ADD FOREIGN KEY (class_id, teacher_id)      REFERENCES classes(id, teacher_id)           ON DELETE CASCADE;
ALTER TABLE attendance_records  ADD FOREIGN KEY (session_id, teacher_id)    REFERENCES class_sessions(id, teacher_id)    ON DELETE CASCADE;
ALTER TABLE attendance_records  ADD FOREIGN KEY (student_id, teacher_id)    REFERENCES students(id, teacher_id)          ON DELETE CASCADE;
ALTER TABLE attendance_records  ADD FOREIGN KEY (enrollment_id, teacher_id) REFERENCES enrollments(id, teacher_id)       ON DELETE CASCADE;
ALTER TABLE invoices            ADD FOREIGN KEY (period_id, teacher_id)     REFERENCES billing_periods(id, teacher_id)   ON DELETE CASCADE;
ALTER TABLE invoices            ADD FOREIGN KEY (student_id, teacher_id)    REFERENCES students(id, teacher_id)          ON DELETE RESTRICT;
ALTER TABLE invoices            ADD FOREIGN KEY (contact_id, teacher_id)    REFERENCES contacts(id, teacher_id)          ON DELETE RESTRICT;
ALTER TABLE invoice_lines       ADD FOREIGN KEY (invoice_id, teacher_id)    REFERENCES invoices(id, teacher_id)          ON DELETE CASCADE;
ALTER TABLE invoice_lines       ADD FOREIGN KEY (enrollment_id, teacher_id) REFERENCES enrollments(id, teacher_id)       ON DELETE RESTRICT;
ALTER TABLE invoice_adjustments ADD FOREIGN KEY (invoice_id, teacher_id)    REFERENCES invoices(id, teacher_id)          ON DELETE CASCADE;
ALTER TABLE payments            ADD FOREIGN KEY (contact_id, teacher_id)    REFERENCES contacts(id, teacher_id)          ON DELETE RESTRICT;
ALTER TABLE payment_allocations ADD FOREIGN KEY (payment_id, teacher_id)    REFERENCES payments(id, teacher_id)          ON DELETE CASCADE;
ALTER TABLE payment_allocations ADD FOREIGN KEY (invoice_id, teacher_id)    REFERENCES invoices(id, teacher_id)          ON DELETE RESTRICT;
ALTER TABLE statements          ADD FOREIGN KEY (contact_id, teacher_id)    REFERENCES contacts(id, teacher_id)          ON DELETE CASCADE;
ALTER TABLE statements          ADD FOREIGN KEY (period_id, teacher_id)     REFERENCES billing_periods(id, teacher_id)   ON DELETE CASCADE;
ALTER TABLE notification_runs   ADD FOREIGN KEY (billing_period_id, teacher_id) REFERENCES billing_periods(id, teacher_id) ON DELETE CASCADE;
ALTER TABLE notifications       ADD FOREIGN KEY (statement_id, teacher_id)  REFERENCES statements(id, teacher_id)        ON DELETE CASCADE;
ALTER TABLE notifications       ADD FOREIGN KEY (run_id, teacher_id)        REFERENCES notification_runs(id, teacher_id) ON DELETE SET NULL (run_id);

-- FK đơn teacher_id trên các bảng gốc (baseline + 000005 đều CASCADE).
ALTER TABLE contacts          ADD FOREIGN KEY (teacher_id) REFERENCES teachers(id) ON DELETE CASCADE;
ALTER TABLE students          ADD FOREIGN KEY (teacher_id) REFERENCES teachers(id) ON DELETE CASCADE;
ALTER TABLE classes           ADD FOREIGN KEY (teacher_id) REFERENCES teachers(id) ON DELETE CASCADE;
ALTER TABLE billing_periods   ADD FOREIGN KEY (teacher_id) REFERENCES teachers(id) ON DELETE CASCADE;
ALTER TABLE notification_runs ADD FOREIGN KEY (teacher_id) REFERENCES teachers(id) ON DELETE CASCADE;

-- Gỡ FK con theo center.
ALTER TABLE students            DROP CONSTRAINT fk_students_contact_center;
ALTER TABLE class_schedules     DROP CONSTRAINT fk_class_schedules_class_center;
ALTER TABLE enrollments         DROP CONSTRAINT fk_enrollments_student_center;
ALTER TABLE enrollments         DROP CONSTRAINT fk_enrollments_class_center;
ALTER TABLE class_sessions      DROP CONSTRAINT fk_class_sessions_class_center;
ALTER TABLE attendance_records  DROP CONSTRAINT fk_attendance_records_session_center;
ALTER TABLE attendance_records  DROP CONSTRAINT fk_attendance_records_student_center;
ALTER TABLE attendance_records  DROP CONSTRAINT fk_attendance_records_enrollment_center;
ALTER TABLE invoices            DROP CONSTRAINT fk_invoices_period_center;
ALTER TABLE invoices            DROP CONSTRAINT fk_invoices_student_center;
ALTER TABLE invoices            DROP CONSTRAINT fk_invoices_contact_center;
ALTER TABLE invoice_lines       DROP CONSTRAINT fk_invoice_lines_invoice_center;
ALTER TABLE invoice_lines       DROP CONSTRAINT fk_invoice_lines_enrollment_center;
ALTER TABLE invoice_adjustments DROP CONSTRAINT fk_invoice_adjustments_invoice_center;
ALTER TABLE payments            DROP CONSTRAINT fk_payments_contact_center;
ALTER TABLE payment_allocations DROP CONSTRAINT fk_payment_allocations_payment_center;
ALTER TABLE payment_allocations DROP CONSTRAINT fk_payment_allocations_invoice_center;
ALTER TABLE statements          DROP CONSTRAINT fk_statements_contact_center;
ALTER TABLE statements          DROP CONSTRAINT fk_statements_period_center;
ALTER TABLE notification_runs   DROP CONSTRAINT fk_notification_runs_period_center;
ALTER TABLE notifications       DROP CONSTRAINT fk_notifications_statement_center;
ALTER TABLE notifications       DROP CONSTRAINT fk_notifications_run_center;

-- Gỡ FK guard teacher-thuộc-center.
ALTER TABLE contacts            DROP CONSTRAINT fk_contacts_teacher_center;
ALTER TABLE students            DROP CONSTRAINT fk_students_teacher_center;
ALTER TABLE classes             DROP CONSTRAINT fk_classes_teacher_center;
ALTER TABLE class_schedules     DROP CONSTRAINT fk_class_schedules_teacher_center;
ALTER TABLE enrollments         DROP CONSTRAINT fk_enrollments_teacher_center;
ALTER TABLE class_sessions      DROP CONSTRAINT fk_class_sessions_teacher_center;
ALTER TABLE attendance_records  DROP CONSTRAINT fk_attendance_records_teacher_center;
ALTER TABLE billing_periods     DROP CONSTRAINT fk_billing_periods_teacher_center;
ALTER TABLE invoices            DROP CONSTRAINT fk_invoices_teacher_center;
ALTER TABLE invoice_lines       DROP CONSTRAINT fk_invoice_lines_teacher_center;
ALTER TABLE invoice_adjustments DROP CONSTRAINT fk_invoice_adjustments_teacher_center;
ALTER TABLE payments            DROP CONSTRAINT fk_payments_teacher_center;
ALTER TABLE payment_allocations DROP CONSTRAINT fk_payment_allocations_teacher_center;
ALTER TABLE statements          DROP CONSTRAINT fk_statements_teacher_center;
ALTER TABLE notification_runs   DROP CONSTRAINT fk_notification_runs_teacher_center;
ALTER TABLE notifications       DROP CONSTRAINT fk_notifications_teacher_center;

-- Gỡ UNIQUE (id, center_id) rồi cột center_id (index idx_*_center rơi theo cột).
ALTER TABLE contacts            DROP CONSTRAINT uq_contacts_cid;
ALTER TABLE students            DROP CONSTRAINT uq_students_cid;
ALTER TABLE classes             DROP CONSTRAINT uq_classes_cid;
ALTER TABLE enrollments         DROP CONSTRAINT uq_enrollments_cid;
ALTER TABLE class_sessions      DROP CONSTRAINT uq_class_sessions_cid;
ALTER TABLE billing_periods     DROP CONSTRAINT uq_billing_periods_cid;
ALTER TABLE invoices            DROP CONSTRAINT uq_invoices_cid;
ALTER TABLE payments            DROP CONSTRAINT uq_payments_cid;
ALTER TABLE statements          DROP CONSTRAINT uq_statements_cid;
ALTER TABLE notification_runs   DROP CONSTRAINT uq_notification_runs_cid;

ALTER TABLE contacts            DROP COLUMN center_id;
ALTER TABLE students            DROP COLUMN center_id;
ALTER TABLE classes             DROP COLUMN center_id;
ALTER TABLE class_schedules     DROP COLUMN center_id;
ALTER TABLE enrollments         DROP COLUMN center_id;
ALTER TABLE class_sessions      DROP COLUMN center_id;
ALTER TABLE attendance_records  DROP COLUMN center_id;
ALTER TABLE billing_periods     DROP COLUMN center_id;
ALTER TABLE invoices            DROP COLUMN center_id;
ALTER TABLE invoice_lines       DROP COLUMN center_id;
ALTER TABLE invoice_adjustments DROP COLUMN center_id;
ALTER TABLE payments            DROP COLUMN center_id;
ALTER TABLE payment_allocations DROP COLUMN center_id;
ALTER TABLE statements          DROP COLUMN center_id;
ALTER TABLE notification_runs   DROP COLUMN center_id;
ALTER TABLE notifications       DROP COLUMN center_id;

-- Lịch sử membership đi cùng ranh giới center — không còn FK nào tham chiếu
-- sau khi guard đã gỡ ở trên.
ALTER TABLE teachers DROP CONSTRAINT fk_teachers_membership;
DROP TABLE center_members;

ALTER TABLE teachers DROP COLUMN center_id;

DROP TABLE centers;

-- Views bản cũ (baseline) — không có center_id.
CREATE VIEW v_contact_balance AS
SELECT
    i.teacher_id,
    i.period_id,
    i.contact_id,
    count(*)                            AS student_count,
    sum(i.total_due)                    AS total_due,
    sum(i.paid_amount)                  AS total_paid,
    sum(i.total_due - i.paid_amount)    AS outstanding
FROM invoices i
WHERE i.status <> 'void'
GROUP BY i.teacher_id, i.period_id, i.contact_id;

CREATE VIEW v_unbilled_attendance AS
SELECT
    a.teacher_id,
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
