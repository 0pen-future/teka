-- 000009 down — gỡ các bảng giảng dạy theo thứ tự ngược. Chỉ là bảng mới,
-- không đụng schema có sẵn nên rollback sạch, mất dữ liệu giảng dạy là
-- chấp nhận được (không dính tiền).
DROP TABLE session_marks;
DROP TABLE session_notes;
DROP TABLE lesson_plans;
DROP TABLE class_curricula;
