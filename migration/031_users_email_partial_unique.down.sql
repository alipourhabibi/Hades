-- Restore the blanket UNIQUE constraint on users.email.
--
-- This will fail if more than one row currently has an empty email, which is
-- the normal state once a second organisation exists. Delete or backfill those
-- rows before rolling back.

DROP INDEX IF EXISTS users_email_unique_not_empty;

-- Dropped first: ADD CONSTRAINT has no IF NOT EXISTS, so a rollback replayed
-- over a database that still carries the constraint would fail here.
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_key;
ALTER TABLE users ADD CONSTRAINT users_email_key UNIQUE (email);
