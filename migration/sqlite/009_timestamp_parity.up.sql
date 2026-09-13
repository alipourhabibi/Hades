-- Three columns existed on PostgreSQL and not on SQLite:
-- sessions.update_time, email_verifications.create_time and
-- password_resets.create_time.
--
-- Nothing reads them yet, so no query was broken. They are added because the
-- two schemas have to match. A drift that is harmless today is how a later
-- query ends up working on one backend and not the other, which is the whole
-- class of bug the schema parity test exists to catch.
--
-- The tables are rebuilt rather than altered. SQLite refuses ADD COLUMN with a
-- non-constant default, and datetime('now') is one. Adding the column nullable
-- instead would leave new rows NULL on SQLite where PostgreSQL writes a time,
-- which is the same drift in a different place.
--
-- The runner disables foreign key enforcement around a migration and runs
-- PRAGMA foreign_key_check before committing, so these rebuilds are safe.
-- See internal/hades/storage/db/sqlitemigrate.go.

-- ---------------------------------------------------------------------------
-- sessions
-- ---------------------------------------------------------------------------

CREATE TABLE sessions_new (
    id                   TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    create_time          DATETIME NOT NULL DEFAULT (datetime('now')),
    update_time          DATETIME NOT NULL DEFAULT (datetime('now')),
    user_id              TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    auth_module          TEXT NOT NULL DEFAULT '',
    expires_at           DATETIME NOT NULL,
    token_hash           TEXT,
    ip_address           TEXT,
    user_agent           TEXT,
    last_activity_at     DATETIME,
    absolute_expires_at  DATETIME,
    revoked_at           DATETIME,
    totp_verified        INTEGER NOT NULL DEFAULT 0
);

INSERT INTO sessions_new (
    id, create_time, update_time, user_id, auth_module, expires_at,
    token_hash, ip_address, user_agent, last_activity_at,
    absolute_expires_at, revoked_at, totp_verified
)
SELECT
    id, create_time, create_time, user_id, auth_module, expires_at,
    token_hash, ip_address, user_agent, last_activity_at,
    absolute_expires_at, revoked_at, totp_verified
FROM sessions;

DROP TABLE sessions;
ALTER TABLE sessions_new RENAME TO sessions;

CREATE INDEX idx_sessions_token_hash ON sessions(token_hash);
CREATE INDEX idx_sessions_user_id    ON sessions(user_id);

-- ---------------------------------------------------------------------------
-- email_verifications
-- ---------------------------------------------------------------------------

CREATE TABLE email_verifications_new (
    id           TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    create_time  DATETIME NOT NULL DEFAULT (datetime('now')),
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   TEXT UNIQUE NOT NULL,
    expires_at   DATETIME NOT NULL,
    used_at      DATETIME
);

INSERT INTO email_verifications_new (id, user_id, token_hash, expires_at, used_at)
SELECT id, user_id, token_hash, expires_at, used_at FROM email_verifications;

DROP TABLE email_verifications;
ALTER TABLE email_verifications_new RENAME TO email_verifications;

-- ---------------------------------------------------------------------------
-- password_resets
-- ---------------------------------------------------------------------------

CREATE TABLE password_resets_new (
    id           TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    create_time  DATETIME NOT NULL DEFAULT (datetime('now')),
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash   TEXT UNIQUE NOT NULL,
    expires_at   DATETIME NOT NULL,
    used_at      DATETIME
);

INSERT INTO password_resets_new (id, user_id, token_hash, expires_at, used_at)
SELECT id, user_id, token_hash, expires_at, used_at FROM password_resets;

DROP TABLE password_resets;
ALTER TABLE password_resets_new RENAME TO password_resets;
