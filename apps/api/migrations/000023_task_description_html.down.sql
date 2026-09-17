-- Lossy: turns the HTML subset back into plain text. Formatting (bold, lists,
-- links) is dropped; only the text and its line breaks survive. Paragraph and
-- list-item ends become line breaks so text from separate blocks does not run
-- together, then every remaining tag is stripped and the entities the API
-- writes are decoded — &amp; last, so an escaped "&lt;" is not decoded twice.
UPDATE tasks
SET description = rtrim(
  replace(replace(replace(replace(replace(
    regexp_replace(
      regexp_replace(
        regexp_replace(description, '<br\s*/?>', E'\n', 'gi'),
        '</(p|li)>', E'\n', 'gi'),
      '<[^>]+>', '', 'g'),
    '&lt;', '<'), '&gt;', '>'), '&#34;', '"'), '&#39;', ''''), '&amp;', '&'),
  E'\n')
WHERE description <> '';
