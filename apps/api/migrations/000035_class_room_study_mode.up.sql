-- =============================================================
-- 000035 — classes: phòng học và hình thức học cho wizard "Sửa lớp học".
--   room       tên/mã phòng, chuỗi tự do, rỗng nghĩa là chưa gán phòng.
--   study_mode "scheduled" (học theo lịch tuần, mặc định) hoặc "self_paced"
--              (tự học, không có lịch); ràng buộc CHECK giữ đúng hai giá trị.
-- =============================================================

ALTER TABLE classes
  ADD COLUMN room       VARCHAR(50) NOT NULL DEFAULT '',
  ADD COLUMN study_mode VARCHAR(12) NOT NULL DEFAULT 'scheduled'
    CHECK (study_mode IN ('scheduled', 'self_paced'));
