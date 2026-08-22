-- Additive tables only; drop them cleanly. No centers/teachers columns were
-- altered by the up migration, so nothing else to reverse.
DROP TABLE IF EXISTS password_reset_tokens;
DROP TABLE IF EXISTS invitations;
