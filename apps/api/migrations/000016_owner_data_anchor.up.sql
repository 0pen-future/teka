-- Danh bạ là dữ liệu TRUNG TÂM, không phải của từng giáo viên: một phụ huynh
-- trong một trung tâm là MỘT bản ghi contacts, neo về owner. Ba bước:
--
--   1. MERGE các contact sống trùng (center_id, phone) về một survivor —
--      hai giáo viên cùng lưu một phụ huynh là kịch bản được hỗ trợ từ trước,
--      nên UPDATE neo thẳng sẽ đâm unique per-teacher nếu không gộp trước.
--   2. RE-KEY hai unique index per-teacher sang per-center.
--   3. ANCHOR contacts + students về owner của center.
--
-- Bảng owner_anchor_backfill giữ đủ dấu vết để down khôi phục anchor, mapping
-- zalo bị gỡ, và hồi sinh các contact bị gộp (best-effort — xem down).

CREATE TABLE owner_anchor_backfill (
    table_name       TEXT NOT NULL,
    row_id           UUID NOT NULL,
    old_teacher      UUID NOT NULL,
    -- NOT NULL khi row là contact bị gộp vào survivor này.
    merged_into      UUID,
    -- Mapping zalo bị gỡ ở bước khử trùng (center_id, zalo_user_id).
    old_zalo_user_id VARCHAR(32),
    old_zalo_name    VARCHAR(100),
    PRIMARY KEY (table_name, row_id)
);

-- =============================================================
-- Bước 1 — MERGE contact sống trùng (center_id, phone).
-- Survivor: created_at sớm nhất; hoà thì nhiều students sống hơn; hoà nữa
-- thì id nhỏ hơn (chỉ để tất định).
-- =============================================================

WITH live AS (
    SELECT c.id, c.center_id, c.phone, c.teacher_id, c.created_at,
           (SELECT count(*) FROM students s
             WHERE s.contact_id = c.id AND s.deleted_at IS NULL) AS n_students
    FROM contacts c
    WHERE c.deleted_at IS NULL
),
ranked AS (
    SELECT id, teacher_id,
           first_value(id) OVER (
               PARTITION BY center_id, phone
               ORDER BY created_at ASC, n_students DESC, id ASC
               ROWS BETWEEN UNBOUNDED PRECEDING AND UNBOUNDED FOLLOWING
           ) AS survivor_id
    FROM live
)
INSERT INTO owner_anchor_backfill (table_name, row_id, old_teacher, merged_into)
SELECT 'contacts', id, teacher_id, survivor_id
FROM ranked
WHERE id <> survivor_id;

-- Repoint con của loser về survivor. uq_invoices (period_id, student_id)
-- không dính contact_id nên invoices/payments/students repoint không thể
-- đâm unique; statements thì có uq (contact_id, period_id) WHERE deleted_at
-- IS NULL — xử lý riêng ngay dưới.
UPDATE students s SET contact_id = b.merged_into
FROM owner_anchor_backfill b
WHERE b.table_name = 'contacts' AND b.merged_into IS NOT NULL
  AND s.contact_id = b.row_id;

UPDATE invoices i SET contact_id = b.merged_into
FROM owner_anchor_backfill b
WHERE b.table_name = 'contacts' AND b.merged_into IS NOT NULL
  AND i.contact_id = b.row_id;

UPDATE payments p SET contact_id = b.merged_into
FROM owner_anchor_backfill b
WHERE b.table_name = 'contacts' AND b.merged_into IS NOT NULL
  AND p.contact_id = b.row_id;

-- Statement trùng kỳ trong NHÓM gộp (survivor + mọi loser của nó): giữ đúng
-- một bản chuẩn — ưu tiên bản của survivor, rồi bản created_at sớm nhất —
-- xoá mềm phần còn lại TRƯỚC khi repoint. Chỉ so với survivor là chưa đủ:
-- hai loser cùng giữ statement một kỳ mà survivor chưa có sẽ đâm
-- uq_statements ngay lúc repoint. Statement là artifact tái sinh được; bản
-- không trùng kỳ repoint giữ nguyên id + token — link phụ huynh đang cầm
-- vẫn mở được. (Statement ngoài nhóm gộp tạo partition đơn lẻ nhờ
-- uq_statements nên luôn tự giữ mình — không bị đụng.)
WITH grouped AS (
    SELECT st.id,
           first_value(st.id) OVER (
               PARTITION BY COALESCE(b.merged_into, st.contact_id), st.period_id
               ORDER BY (b.row_id IS NULL) DESC, st.created_at ASC, st.id ASC
               ROWS BETWEEN UNBOUNDED PRECEDING AND UNBOUNDED FOLLOWING
           ) AS keeper_id
    FROM statements st
    LEFT JOIN owner_anchor_backfill b
           ON b.table_name = 'contacts' AND b.merged_into IS NOT NULL
          AND b.row_id = st.contact_id
    WHERE st.deleted_at IS NULL
)
UPDATE statements st SET deleted_at = now(), updated_at = now()
FROM grouped g
WHERE st.id = g.id AND st.id <> g.keeper_id;

UPDATE statements st SET contact_id = b.merged_into
FROM owner_anchor_backfill b
WHERE b.table_name = 'contacts' AND b.merged_into IS NOT NULL
  AND st.contact_id = b.row_id;

-- Loser xoá mềm, GIỮ nguyên zalo mapping trên row đã xoá (index partial bỏ
-- qua row xoá mềm; down hồi sinh là mapping còn nguyên).
UPDATE contacts c SET deleted_at = now(), updated_at = now()
FROM owner_anchor_backfill b
WHERE b.table_name = 'contacts' AND b.merged_into IS NOT NULL
  AND c.id = b.row_id;

-- Khử trùng (center_id, zalo_user_id) còn lại giữa các contact SỐNG khác
-- phone (không bị merge ở trên): giữ mapping của bản created_at sớm nhất,
-- gỡ của các bản sau — không bao giờ để hai gia đình dính một zalo friend.
WITH ranked AS (
    SELECT id, teacher_id, zalo_user_id, zalo_name,
           first_value(id) OVER (
               PARTITION BY center_id, zalo_user_id
               ORDER BY created_at ASC, id ASC
               ROWS BETWEEN UNBOUNDED PRECEDING AND UNBOUNDED FOLLOWING
           ) AS keeper_id
    FROM contacts
    WHERE deleted_at IS NULL AND zalo_user_id IS NOT NULL
)
INSERT INTO owner_anchor_backfill
    (table_name, row_id, old_teacher, old_zalo_user_id, old_zalo_name)
SELECT 'contacts_zalo', id, teacher_id, zalo_user_id, zalo_name
FROM ranked
WHERE id <> keeper_id;

UPDATE contacts c SET zalo_user_id = NULL, zalo_name = NULL, updated_at = now()
FROM owner_anchor_backfill b
WHERE b.table_name = 'contacts_zalo' AND c.id = b.row_id;

-- =============================================================
-- Bước 2 — RE-KEY unique per-teacher → per-center (giữ nguyên tên index:
-- mapping lỗi 23505 ở repo layer bám theo tên).
-- =============================================================

DROP INDEX uq_contacts_phone;
CREATE UNIQUE INDEX uq_contacts_phone
    ON contacts(center_id, phone) WHERE deleted_at IS NULL;

DROP INDEX uq_contacts_zalo_user;
CREATE UNIQUE INDEX uq_contacts_zalo_user
    ON contacts(center_id, zalo_user_id)
    WHERE zalo_user_id IS NOT NULL AND deleted_at IS NULL;

-- =============================================================
-- Bước 3 — ANCHOR contacts + students về owner. ON CONFLICT DO NOTHING:
-- loser đã có row backfill từ bước merge (old_teacher đã đúng ở đó).
-- =============================================================

INSERT INTO owner_anchor_backfill (table_name, row_id, old_teacher)
SELECT 'contacts', c.id, c.teacher_id
FROM contacts c
JOIN centers ce ON ce.id = c.center_id
WHERE c.teacher_id <> ce.owner_id
UNION ALL
SELECT 'students', s.id, s.teacher_id
FROM students s
JOIN centers ce ON ce.id = s.center_id
WHERE s.teacher_id <> ce.owner_id
ON CONFLICT (table_name, row_id) DO NOTHING;

UPDATE contacts c SET teacher_id = ce.owner_id
FROM centers ce
WHERE ce.id = c.center_id AND c.teacher_id <> ce.owner_id;

UPDATE students s SET teacher_id = ce.owner_id
FROM centers ce
WHERE ce.id = s.center_id AND s.teacher_id <> ce.owner_id;
