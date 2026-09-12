-- Replace the blanket UNIQUE constraint on users.email with a partial unique
-- index that ignores empty values.
--
-- Organisations are stored as rows in `users` with an empty email. A column
-- level UNIQUE treats '' as an ordinary value, so the second organisation ever
-- created violated it: only one org could exist on this backend.
--
-- SQLite cannot drop a column constraint in place, so the table is rebuilt.
-- The partial index preserves the guarantee that matters (no two accounts share
-- a real email address) while allowing any number of rows with no email.
--
-- Foreign key enforcement is not touched here. The migration runner disables it
-- around every migration and verifies the result with PRAGMA foreign_key_check
-- before committing, because dropping a table that sessions, modules and the
-- rest reference is a violation while enforcement is on.

CREATE TABLE users_new (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    create_time DATETIME NOT NULL DEFAULT (datetime('now')),
    update_time DATETIME NOT NULL DEFAULT (datetime('now')),
    username    TEXT UNIQUE NOT NULL,
    email       TEXT NOT NULL,
    password    TEXT NOT NULL DEFAULT '',
    type        INTEGER NOT NULL DEFAULT 0,
    state       INTEGER NOT NULL DEFAULT 1,
    description TEXT,
    url         TEXT,
    failed_login_count  INTEGER NOT NULL DEFAULT 0,
    locked_until        DATETIME,
    email_verified_at   DATETIME
);

INSERT INTO users_new (
    id, create_time, update_time, username, email, password,
    type, state, description, url,
    failed_login_count, locked_until, email_verified_at
)
SELECT
    id, create_time, update_time, username, email, password,
    type, state, description, url,
    failed_login_count, locked_until, email_verified_at
FROM users;

DROP TABLE users;
ALTER TABLE users_new RENAME TO users;

CREATE UNIQUE INDEX IF NOT EXISTS users_email_unique_not_empty
    ON users (email)
    WHERE email <> '';
