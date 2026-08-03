-- Runs once on first cluster init (empty postgres-data volume) as superuser.
-- Extensions only: the application schema is owned by the migrations, which
-- also re-assert these idempotently for non-compose databases.
CREATE EXTENSION IF NOT EXISTS citext;
CREATE EXTENSION IF NOT EXISTS pgcrypto;
