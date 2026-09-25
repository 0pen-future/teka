DROP INDEX IF EXISTS idx_classes_center_recruiting;
DROP INDEX IF EXISTS uq_classes_center_code;

ALTER TABLE classes
  DROP COLUMN note,
  DROP COLUMN recruiting,
  DROP COLUMN tags,
  DROP COLUMN code;
