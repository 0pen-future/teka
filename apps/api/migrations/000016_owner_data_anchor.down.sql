-- Đảo 000016 theo dấu vết owner_anchor_backfill. Best-effort có chủ đích:
--   - Con (students/invoices/payments/statements) ĐÃ repoint về survivor thì
--     GIỮ theo survivor — không tách ngược được vì không biết dòng nào từng
--     thuộc loser sau khi dữ liệu mới đè lên.
--   - Statement của loser bị xoá mềm vì trùng kỳ với survivor thì giữ xoá mềm.
--   - Index per-teacher tạo lại được là nhờ anchor đã hồi về teacher cũ; dữ
--     liệu MỚI tạo sau up (neo owner) có thể đâm unique per-teacher với row
--     hồi sinh — trường hợp đó phải dọn tay trước khi down.

-- Hồi anchor về teacher cũ (gồm cả loser đã xoá mềm — row merge giữ
-- old_teacher gốc).
UPDATE contacts c SET teacher_id = b.old_teacher
FROM owner_anchor_backfill b
WHERE b.table_name = 'contacts' AND b.row_id = c.id;

UPDATE students s SET teacher_id = b.old_teacher
FROM owner_anchor_backfill b
WHERE b.table_name = 'students' AND b.row_id = s.id;

-- Re-key unique về per-teacher TRƯỚC khi hồi mapping zalo: chừng nào index
-- còn per-center, hồi mapping cho row thứ hai cùng zalo friend trong một
-- center sẽ đâm unique với row keeper đang sống.
-- (Tên index giữ nguyên như trước 000016.)
DROP INDEX uq_contacts_phone;
CREATE UNIQUE INDEX uq_contacts_phone
    ON contacts(teacher_id, phone) WHERE deleted_at IS NULL;

DROP INDEX uq_contacts_zalo_user;
CREATE UNIQUE INDEX uq_contacts_zalo_user
    ON contacts(teacher_id, zalo_user_id)
    WHERE zalo_user_id IS NOT NULL AND deleted_at IS NULL;

-- Hồi mapping zalo bị gỡ ở bước khử trùng per-center (per-teacher đã cho
-- phép hai giáo viên cùng dính một zalo friend).
UPDATE contacts c
SET zalo_user_id = b.old_zalo_user_id, zalo_name = b.old_zalo_name
FROM owner_anchor_backfill b
WHERE b.table_name = 'contacts_zalo' AND c.id = b.row_id;

-- Hồi sinh loser bị gộp (per-teacher unique đã hợp lệ trở lại vì anchor
-- từng row đã về teacher gốc).
UPDATE contacts c SET deleted_at = NULL, updated_at = now()
FROM owner_anchor_backfill b
WHERE b.table_name = 'contacts' AND b.merged_into IS NOT NULL
  AND c.id = b.row_id;

DROP TABLE owner_anchor_backfill;
