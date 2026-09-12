-- Replace the blanket UNIQUE constraint on users.email with a partial unique
-- index that ignores empty values.
--
-- Organisations are stored as rows in `users` with an empty email. A plain
-- UNIQUE constraint treats '' as an ordinary value, so the second organisation
-- ever created violated it: only one org could exist.
--
-- The partial index keeps the guarantee that matters (no two accounts share a
-- real email address, which registration and the OAuth email link both rely on)
-- while allowing any number of rows with no email at all.

ALTER TABLE users DROP CONSTRAINT IF EXISTS users_email_key;

CREATE UNIQUE INDEX IF NOT EXISTS users_email_unique_not_empty
    ON users (email)
    WHERE email <> '';
