-- ===== 000033 — Bổ sung trường ngân hàng học liệu & ngân hàng bài tập =====
-- Học liệu (library_materials): mở rộng "kind" từ 4 lên 8 giá trị để khớp
-- bảng v5 (video/audio/image/doc/note/live/link, giữ "other" cho dữ liệu cũ)
-- và thêm cờ active để ẩn/hiện khỏi ngân hàng mà không xoá.
-- Bài tập (library_exercises): thêm mã bài tập duy nhất theo trung tâm (dùng
-- để tra cứu/hiển thị ngắn gọn, tách biệt với difficulty đã có), kỹ năng và
-- cấp độ tự do (text), cùng cờ active.
ALTER TABLE library_materials DROP CONSTRAINT library_materials_kind_check;
ALTER TABLE library_materials
    ADD CONSTRAINT library_materials_kind_check
        CHECK (kind IN ('video', 'audio', 'image', 'doc', 'note', 'live', 'link', 'other')),
    ADD COLUMN active BOOLEAN NOT NULL DEFAULT true;

ALTER TABLE library_exercises
    ADD COLUMN code   VARCHAR(20),
    ADD COLUMN skill  VARCHAR(50),
    ADD COLUMN level  VARCHAR(50),
    ADD COLUMN active BOOLEAN NOT NULL DEFAULT true;

-- Backfill mã theo thứ tự tạo trong từng trung tâm: BT-0001, BT-0002, …
WITH numbered AS (
    SELECT id, ROW_NUMBER() OVER (PARTITION BY center_id ORDER BY created_at, id) AS n
    FROM library_exercises
)
UPDATE library_exercises e SET code = 'BT-' || LPAD(n.n::text, 4, '0')
FROM numbered n WHERE n.id = e.id;

ALTER TABLE library_exercises ALTER COLUMN code SET NOT NULL;
-- Partial: chỉ ràng buộc duy nhất trên dòng còn sống, xoá mềm không giữ mã.
CREATE UNIQUE INDEX uq_library_exercises_center_code
    ON library_exercises (center_id, code) WHERE deleted_at IS NULL;
CREATE INDEX idx_library_materials_center_active ON library_materials (center_id, active);
CREATE INDEX idx_library_exercises_center_active ON library_exercises (center_id, active);
