-- Restore the blanket UNIQUE constraint on users.email.
--
-- This will fail if more than one row currently has an empty email, which is
-- the normal state once a second organisation exists. Delete or backfill those
-- rows before rolling back.

DROP INDEX IF EXISTS users_email_unique_not_empty;

ALTER TABLE users ADD CONSTRAINT users_email_key UNIQUE (email);
