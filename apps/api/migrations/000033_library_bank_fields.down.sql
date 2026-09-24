DROP INDEX uq_library_exercises_center_code;
DROP INDEX idx_library_exercises_center_active;
DROP INDEX idx_library_materials_center_active;
ALTER TABLE library_exercises DROP COLUMN active, DROP COLUMN level, DROP COLUMN skill, DROP COLUMN code;
-- Không có chỗ chứa 4 loại mới ở CHECK cũ nên gộp về "other" trước khi thu hẹp.
UPDATE library_materials SET kind = 'other' WHERE kind IN ('audio', 'image', 'note', 'live');
ALTER TABLE library_materials DROP COLUMN active;
ALTER TABLE library_materials DROP CONSTRAINT library_materials_kind_check;
ALTER TABLE library_materials ADD CONSTRAINT library_materials_kind_check
    CHECK (kind IN ('link', 'doc', 'video', 'other'));
