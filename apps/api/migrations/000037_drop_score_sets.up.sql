-- Gỡ danh mục bộ điểm cấp trung tâm mà 000014 đã thêm: bộ điểm giờ lấy từ
-- mẫu chương trình và được chép vào lớp khi áp dụng, nên hai bảng
-- score_sets, score_set_components và cột truy vết
-- class_score_components.source_set_id không còn được dùng. Snapshot
-- class_score_components và điểm student_scores giữ nguyên.
--
-- Xóa cột trước để gỡ FK về score_sets, rồi bảng con, cuối cùng bảng cha
-- (index score_sets_center_name_live đi theo bảng).
ALTER TABLE class_score_components DROP COLUMN IF EXISTS source_set_id;
DROP TABLE IF EXISTS score_set_components;
DROP TABLE IF EXISTS score_sets;
