-- task_columns.color lets the web board tint a column by stage (none = the
-- default cream, sky/sun/mint reserved for in-progress/pending-review/done
-- style columns). The backfill only recognizes the default starter board's
-- own names and is_done flag: a column a center renamed or added by hand
-- keeps the 'none' default and is colored by hand afterward.
ALTER TABLE task_columns ADD COLUMN color VARCHAR(16) NOT NULL DEFAULT 'none';
ALTER TABLE task_columns ADD CONSTRAINT ck_task_columns_color
  CHECK (color IN ('none', 'sky', 'sun', 'mint'));

UPDATE task_columns SET color = 'mint' WHERE is_done;
UPDATE task_columns SET color = 'sky' WHERE NOT is_done AND name = 'Đang làm';
