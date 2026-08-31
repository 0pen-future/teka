-- Điểm danh 4 trạng thái: thêm 'late' (đi muộn) vào từ vựng trạng thái.
-- Ngữ nghĩa: present = đúng giờ, late = muộn (vẫn coi là có mặt),
-- absent = vắng, excused = vắng có lý do. Mọi trạng thái đều billable = true
-- ở V1 — 'late' không đổi bất kỳ hành vi tính tiền nào.
ALTER TABLE attendance_records
    DROP CONSTRAINT attendance_records_status_check;
ALTER TABLE attendance_records
    ADD CONSTRAINT attendance_records_status_check
    CHECK (status IN ('present', 'late', 'absent', 'excused'));
