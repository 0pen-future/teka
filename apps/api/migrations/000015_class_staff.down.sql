-- 000015 down — chỉ là bảng mới, không đụng schema có sẵn nên rollback sạch.
-- classes.teacher_id vẫn là nguồn chân lý giáo viên chính trong cửa sổ
-- dual-write, nên bỏ bảng không mất dữ liệu gốc.
DROP TABLE class_staff;
