-- =============================================================
-- 000025 — classes: trường danh mục cho màn "Danh sách lớp học".
--   code       mã lớp hiển thị/tìm kiếm, duy nhất theo trung tâm trong số
--              lớp chưa xoá mềm (partial unique index).
--   tags       thẻ tự do dạng JSONB mảng chuỗi, thay cả mảng mỗi lần ghi.
--   recruiting cờ "cần tuyển sinh" cho chip đếm và index lọc.
--   note       ghi chú vận hành, văn bản thuần.
-- Backfill code cho lớp cũ bằng ROW_NUMBER() theo trung tâm, ổn định theo
-- created_at rồi id — không dựa vào UUID (6 hex đầu của UUIDv7 là timestamp
-- nên hai lớp tạo cùng mili-giây sẽ trùng). Mã sinh trong code có dạng
-- "L" + 6 ký tự base32 nên không đụng dạng "L0001" của backfill.
-- =============================================================

ALTER TABLE classes
  ADD COLUMN code       VARCHAR(20) NOT NULL DEFAULT '',
  ADD COLUMN tags       JSONB       NOT NULL DEFAULT '[]'::jsonb,
  ADD COLUMN recruiting BOOLEAN     NOT NULL DEFAULT false,
  ADD COLUMN note       TEXT;

WITH numbered AS (
  SELECT id, ROW_NUMBER() OVER (PARTITION BY center_id ORDER BY created_at, id) AS rn
  FROM classes
)
UPDATE classes c
SET code = 'L' || lpad(n.rn::text, 4, '0')
FROM numbered n
WHERE c.id = n.id AND c.code = '';

ALTER TABLE classes ALTER COLUMN code DROP DEFAULT;

CREATE UNIQUE INDEX uq_classes_center_code
  ON classes (center_id, code)
  WHERE deleted_at IS NULL;

CREATE INDEX idx_classes_center_recruiting
  ON classes (center_id)
  WHERE recruiting AND deleted_at IS NULL;
