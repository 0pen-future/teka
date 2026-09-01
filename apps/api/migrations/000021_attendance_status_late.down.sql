-- Rollback chấp nhận mất phân biệt "muộn": late gập về present trước khi
-- khôi phục CHECK cũ. An toàn về tiền vì late luôn billable = true — về mặt
-- billing, muộn và đúng giờ là một.
UPDATE attendance_records SET status = 'present' WHERE status = 'late';
ALTER TABLE attendance_records
    DROP CONSTRAINT attendance_records_status_check;
ALTER TABLE attendance_records
    ADD CONSTRAINT attendance_records_status_check
    CHECK (status IN ('present', 'absent', 'excused'));
