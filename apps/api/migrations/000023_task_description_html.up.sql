-- Task descriptions become a sanitized HTML subset written by the API. Every
-- description stored before this point is plain text, so each non-empty one
-- (soft-deleted rows included, to keep the column uniform) is wrapped in a
-- single paragraph: text is HTML-escaped first, then line breaks become <br>.
-- There is deliberately no content guard such as NOT LIKE '<p>%': a legacy
-- description that literally starts with "<p>" is still plain text and must be
-- escaped like any other. Running twice is prevented by schema_migrations, not
-- by this statement.
UPDATE tasks
SET description = '<p>' ||
  replace(replace(
    replace(replace(replace(description, '&', '&amp;'), '<', '&lt;'), '>', '&gt;'),
    E'\r\n', '<br>'), E'\n', '<br>') ||
  '</p>'
WHERE description <> '';
