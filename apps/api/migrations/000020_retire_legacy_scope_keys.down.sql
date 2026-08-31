-- No-op có chủ đích: không dựng lại các dòng đã xóa.
--  * data.view_center_wide: bản backfill 000018 đã materialize các key
--    view_all tương đương, nên quyền hiệu lực của mọi holder vẫn nguyên vẹn
--    khi rollback code — binary cũ đọc các key canonical như thường.
--  * scores.view_all / teaching.view_all: không có điểm enforcement, dòng
--    gán không mang hành vi nào để khôi phục.
-- Nếu cần bằng chứng trạng thái trước cleanup, dùng snapshot kiểm kê trong
-- báo cáo phase (plans/reports/) — không tái tạo dữ liệu từ migration.
SELECT 1;
