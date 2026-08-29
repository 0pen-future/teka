-- 000014 down — gỡ các bảng bộ điểm theo thứ tự ngược (con trước cha). Chỉ là
-- bảng mới, không đụng schema có sẵn nên rollback sạch.
DROP TABLE student_scores;
DROP TABLE class_score_components;
DROP TABLE score_set_components;
DROP TABLE score_sets;
