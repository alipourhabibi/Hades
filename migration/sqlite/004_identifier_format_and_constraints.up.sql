-- One identifier format, and the constraints PostgreSQL has had since 003.
--
-- Two problems are fixed together because the second requires rebuilding the
-- same tables as the first.
--
-- 1. users.id was created with a hyphenated 36-character default while every
--    other table used lower(hex(randomblob(16))), which is 32 characters with
--    no hyphens. uuid.UUID.String() is hyphenated, so a query that normalised
--    matched and one that did not silently matched nothing: RevokeSession
--    reported success while revoking no session, DeleteByIds deleted zero rows,
--    and device_grants.api_token_id could never join api_tokens.id. The
--    workaround in the read paths, REPLACE(id,'-',''), also made the primary
--    key index unusable on the two hottest lookups in the system.
--
--    Dashless is the documented convention and the one 20 of 21 tables already
--    used, so users.id moves to it and every column that references an id is
--    normalised with it.
--
-- 2. The SQLite schema was one file against 31 PostgreSQL migrations and had
--    drifted: no indexes or uniqueness on commits, no index or UNIQUE on
--    api_tokens.token_hash (a full table scan on every authenticated request,
--    and duplicate hashes storable), no UNIQUE on the device, verification and
--    reset token hashes, no ON DELETE actions anywhere, org_memberships with no
--    id or created_at, and modules defaulting visibility and state to 0
--    (UNSPECIFIED) where PostgreSQL defaults both to 1.
--
-- The runner disables foreign key enforcement around a migration and runs
-- PRAGMA foreign_key_check before committing, which is what makes the table
-- rebuilds below safe. See internal/hades/storage/db/sqlitemigrate.go.

-- ---------------------------------------------------------------------------
-- Step 1: normalise every stored identifier to the dashless form.
-- ---------------------------------------------------------------------------

UPDATE users               SET id                 = replace(id, '-', '');
UPDATE sessions            SET user_id            = replace(user_id, '-', '');
UPDATE modules             SET owner_id           = replace(owner_id, '-', '');
UPDATE commits             SET id                 = replace(id, '-', ''),
                               owner_id           = replace(owner_id, '-', ''),
                               module_id          = replace(module_id, '-', ''),
                               created_by_user_id = replace(created_by_user_id, '-', '');
UPDATE sdk_jobs            SET commit_id          = replace(commit_id, '-', ''),
                               module_id          = replace(module_id, '-', '');
UPDATE email_verifications SET user_id            = replace(user_id, '-', '');
UPDATE password_resets     SET user_id            = replace(user_id, '-', '');
UPDATE oauth_identities    SET user_id            = replace(user_id, '-', '');
UPDATE api_tokens          SET user_id            = replace(user_id, '-', '');
UPDATE device_grants       SET user_id            = replace(user_id, '-', ''),
                               api_token_id       = replace(api_token_id, '-', '');
UPDATE totp_secrets        SET user_id            = replace(user_id, '-', '');
UPDATE totp_backup_codes   SET user_id            = replace(user_id, '-', '');
UPDATE audit_log           SET user_id            = replace(user_id, '-', '');
UPDATE org_memberships     SET org_id             = replace(org_id, '-', ''),
                               member_id          = replace(member_id, '-', '');
UPDATE ci_runs             SET module_id          = replace(module_id, '-', '');
UPDATE notifications       SET user_id            = replace(user_id, '-', '');
UPDATE resources           SET id                 = replace(id, '-', '');
UPDATE labels              SET module_id          = replace(module_id, '-', ''),
                               commit_id          = replace(commit_id, '-', '');

-- opa_role_bindings.subject holds a username, not an id, and is left alone.
-- notifications.resource_id holds an sdk_jobs.id, which was already dashless.

-- ---------------------------------------------------------------------------
-- Step 2: rebuild users with the dashless default.
-- ---------------------------------------------------------------------------

CREATE TABLE users_new (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
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
INSERT INTO users_new SELECT
    id, create_time, update_time, username, email, password,
    type, state, description, url,
    failed_login_count, locked_until, email_verified_at
FROM users;
DROP TABLE users;
ALTER TABLE users_new RENAME TO users;
CREATE UNIQUE INDEX IF NOT EXISTS users_email_unique_not_empty
    ON users (email) WHERE email <> '';

-- ---------------------------------------------------------------------------
-- Step 3: rebuild every table that needs a constraint, default or ON DELETE
-- action it did not have. SQLite cannot alter these in place.
-- ---------------------------------------------------------------------------

CREATE TABLE sessions_new (
    id                   TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    create_time          DATETIME NOT NULL DEFAULT (datetime('now')),
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
INSERT INTO sessions_new SELECT
    id, create_time, user_id, auth_module, expires_at, token_hash,
    ip_address, user_agent, last_activity_at, absolute_expires_at,
    revoked_at, totp_verified
FROM sessions;
DROP TABLE sessions;
ALTER TABLE sessions_new RENAME TO sessions;
CREATE INDEX IF NOT EXISTS idx_sessions_token_hash ON sessions(token_hash);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id    ON sessions(user_id);

CREATE TABLE modules_new (
    id                TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    create_time       DATETIME NOT NULL DEFAULT (datetime('now')),
    update_time       DATETIME NOT NULL DEFAULT (datetime('now')),
    name              TEXT UNIQUE NOT NULL,
    owner_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- 1, matching PostgreSQL. A row inserted without an explicit visibility was
    -- UNSPECIFIED here and public there, so the same insert produced a module
    -- that was invisible on one backend and world-readable on the other.
    visibility        INTEGER NOT NULL DEFAULT 1,
    state             INTEGER NOT NULL DEFAULT 1,
    description       TEXT,
    url               TEXT,
    default_label_name TEXT,
    default_branch    TEXT NOT NULL DEFAULT 'main',
    lint_preset       INTEGER NOT NULL DEFAULT 1,
    breaking_enabled  INTEGER NOT NULL DEFAULT 1
);
INSERT INTO modules_new (
    id, create_time, update_time, name, owner_id, visibility, state,
    description, url, default_label_name, default_branch,
    lint_preset, breaking_enabled
)
SELECT
    id, create_time, update_time, name, owner_id, visibility, state,
    description, url, default_label_name, default_branch,
    lint_preset, breaking_enabled
FROM modules;
DROP TABLE modules;
ALTER TABLE modules_new RENAME TO modules;
CREATE INDEX IF NOT EXISTS idx_modules_owner_id ON modules(owner_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_modules_url_unique_non_empty
    ON modules(url) WHERE url IS NOT NULL AND url <> '';

CREATE TABLE commits_new (
    id                TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    create_time       DATETIME NOT NULL DEFAULT (datetime('now')),
    update_time       DATETIME NOT NULL DEFAULT (datetime('now')),
    commit_hash       TEXT NOT NULL,
    owner_id          TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    module_id         TEXT NOT NULL REFERENCES modules(id) ON DELETE CASCADE,
    digest_type       INTEGER NOT NULL DEFAULT 0,
    digest_value      TEXT,
    created_by_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    source_control_url TEXT
);
INSERT INTO commits_new SELECT
    id, create_time, update_time, commit_hash, owner_id, module_id,
    digest_type, digest_value, created_by_user_id, source_control_url
FROM commits;
DROP TABLE commits;
ALTER TABLE commits_new RENAME TO commits;
CREATE INDEX IF NOT EXISTS idx_commits_owner_id            ON commits(owner_id);
CREATE INDEX IF NOT EXISTS idx_commits_module_id           ON commits(module_id);
CREATE INDEX IF NOT EXISTS idx_commits_created_by_user_id  ON commits(created_by_user_id);
CREATE INDEX IF NOT EXISTS idx_commits_commit_hash         ON commits(commit_hash);
CREATE INDEX IF NOT EXISTS idx_commits_module_id_create_time
    ON commits(module_id, create_time DESC);
-- Matches PostgreSQL 023 and 027: unique per module, not globally, because two
-- modules holding identical files legitimately share a digest.
CREATE UNIQUE INDEX IF NOT EXISTS idx_commits_module_commit_digest_unique
    ON commits(module_id, commit_hash, digest_value);
CREATE UNIQUE INDEX IF NOT EXISTS idx_commits_module_digest_unique
    ON commits(module_id, digest_value);

CREATE TABLE sdk_jobs_new (
    id              TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    commit_id       TEXT NOT NULL REFERENCES commits(id) ON DELETE CASCADE,
    module_id       TEXT NOT NULL REFERENCES modules(id) ON DELETE CASCADE,
    status          TEXT NOT NULL DEFAULT 'pending',
    language        TEXT NOT NULL,
    plugin          TEXT NOT NULL,
    plugin_options  TEXT NOT NULL DEFAULT '',
    output_location TEXT,
    error_message   TEXT,
    attempts        INTEGER NOT NULL DEFAULT 0,
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    started_at      DATETIME,
    finished_at     DATETIME
);
INSERT INTO sdk_jobs_new SELECT
    id, commit_id, module_id, status, language, plugin, plugin_options,
    output_location, error_message, attempts, created_at, started_at, finished_at
FROM sdk_jobs;
DROP TABLE sdk_jobs;
ALTER TABLE sdk_jobs_new RENAME TO sdk_jobs;
CREATE INDEX IF NOT EXISTS idx_sdk_jobs_status    ON sdk_jobs(status);
CREATE INDEX IF NOT EXISTS idx_sdk_jobs_commit_id ON sdk_jobs(commit_id);
CREATE INDEX IF NOT EXISTS idx_sdk_jobs_module_id ON sdk_jobs(module_id);

CREATE TABLE email_verifications_new (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT UNIQUE NOT NULL,
    expires_at  DATETIME NOT NULL,
    used_at     DATETIME
);
INSERT INTO email_verifications_new
SELECT id, user_id, token_hash, expires_at, used_at
FROM email_verifications
WHERE token_hash IN (SELECT token_hash FROM email_verifications GROUP BY token_hash HAVING count(*) = 1);
DROP TABLE email_verifications;
ALTER TABLE email_verifications_new RENAME TO email_verifications;

CREATE TABLE password_resets_new (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  TEXT UNIQUE NOT NULL,
    expires_at  DATETIME NOT NULL,
    used_at     DATETIME
);
INSERT INTO password_resets_new
SELECT id, user_id, token_hash, expires_at, used_at
FROM password_resets
WHERE token_hash IN (SELECT token_hash FROM password_resets GROUP BY token_hash HAVING count(*) = 1);
DROP TABLE password_resets;
ALTER TABLE password_resets_new RENAME TO password_resets;

CREATE TABLE oauth_identities_new (
    id           TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    create_time  DATETIME NOT NULL DEFAULT (datetime('now')),
    user_id      TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider     TEXT NOT NULL,
    provider_uid TEXT NOT NULL,
    email        TEXT,
    UNIQUE(provider, provider_uid)
);
INSERT INTO oauth_identities_new SELECT
    id, create_time, user_id, provider, provider_uid, email
FROM oauth_identities;
DROP TABLE oauth_identities;
ALTER TABLE oauth_identities_new RENAME TO oauth_identities;
CREATE INDEX IF NOT EXISTS idx_oauth_identities_user_id ON oauth_identities(user_id);

-- token_hash gains an index and a UNIQUE. GetByTokenHash runs on every
-- authenticated request and was a full table scan on the default backend, and
-- without the UNIQUE two tokens could hash to the same stored value.
CREATE TABLE api_tokens_new (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    create_time DATETIME NOT NULL DEFAULT (datetime('now')),
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name        TEXT NOT NULL,
    prefix      TEXT NOT NULL,
    token_hash  TEXT UNIQUE NOT NULL,
    scopes      TEXT,
    expires_at  DATETIME,
    last_used_at DATETIME,
    revoked_at  DATETIME
);
INSERT INTO api_tokens_new
SELECT id, create_time, user_id, name, prefix, token_hash, scopes,
       expires_at, last_used_at, revoked_at
FROM api_tokens
WHERE token_hash IN (SELECT token_hash FROM api_tokens GROUP BY token_hash HAVING count(*) = 1);
DROP TABLE api_tokens;
ALTER TABLE api_tokens_new RENAME TO api_tokens;
CREATE INDEX IF NOT EXISTS idx_api_tokens_token_hash ON api_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_api_tokens_user_id    ON api_tokens(user_id);

-- device_code_hash and user_code gain UNIQUE. Without it GetByUserCode could
-- match an arbitrary row among duplicates, which is a lookup that decides which
-- account a device-flow token is minted for.
CREATE TABLE device_grants_new (
    id               TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    create_time      DATETIME NOT NULL DEFAULT (datetime('now')),
    device_code_hash TEXT UNIQUE NOT NULL,
    user_code        TEXT UNIQUE NOT NULL,
    user_id          TEXT REFERENCES users(id) ON DELETE CASCADE,
    api_token_id     TEXT REFERENCES api_tokens(id) ON DELETE SET NULL,
    approved_at      DATETIME,
    expires_at       DATETIME NOT NULL
);
INSERT INTO device_grants_new
SELECT id, create_time, device_code_hash, user_code, user_id, api_token_id,
       approved_at, expires_at
FROM device_grants
WHERE device_code_hash IN (SELECT device_code_hash FROM device_grants GROUP BY device_code_hash HAVING count(*) = 1)
  AND user_code        IN (SELECT user_code        FROM device_grants GROUP BY user_code        HAVING count(*) = 1);
DROP TABLE device_grants;
ALTER TABLE device_grants_new RENAME TO device_grants;
CREATE INDEX IF NOT EXISTS idx_device_grants_user_code ON device_grants(user_code);

CREATE TABLE totp_secrets_new (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    create_time DATETIME NOT NULL DEFAULT (datetime('now')),
    user_id     TEXT NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    secret_enc  TEXT NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 0,
    enrolled_at DATETIME
);
INSERT INTO totp_secrets_new SELECT
    id, create_time, user_id, secret_enc, enabled, enrolled_at
FROM totp_secrets;
DROP TABLE totp_secrets;
ALTER TABLE totp_secrets_new RENAME TO totp_secrets;

CREATE TABLE totp_backup_codes_new (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    create_time DATETIME NOT NULL DEFAULT (datetime('now')),
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    code_hash   TEXT NOT NULL,
    used_at     DATETIME
);
INSERT INTO totp_backup_codes_new SELECT
    id, create_time, user_id, code_hash, used_at
FROM totp_backup_codes;
DROP TABLE totp_backup_codes;
ALTER TABLE totp_backup_codes_new RENAME TO totp_backup_codes;
CREATE INDEX IF NOT EXISTS idx_totp_backup_codes_user_id ON totp_backup_codes(user_id);

-- SET NULL rather than CASCADE, matching PostgreSQL 017: deleting a user must
-- not delete the record of what they did.
CREATE TABLE audit_log_new (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    create_time DATETIME NOT NULL DEFAULT (datetime('now')),
    user_id     TEXT REFERENCES users(id) ON DELETE SET NULL,
    event       TEXT NOT NULL,
    ip_address  TEXT,
    user_agent  TEXT,
    metadata    TEXT
);
INSERT INTO audit_log_new SELECT
    id, create_time, user_id, event, ip_address, user_agent, metadata
FROM audit_log;
DROP TABLE audit_log;
ALTER TABLE audit_log_new RENAME TO audit_log;
CREATE INDEX IF NOT EXISTS idx_audit_log_user_id     ON audit_log(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_create_time ON audit_log(create_time DESC);

-- Gains the id and created_at columns PostgreSQL 019 has.
CREATE TABLE org_memberships_new (
    id         TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    org_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    member_id  TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role       TEXT NOT NULL DEFAULT 'member',
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE (org_id, member_id)
);
INSERT INTO org_memberships_new (org_id, member_id, role)
SELECT org_id, member_id, role FROM org_memberships;
DROP TABLE org_memberships;
ALTER TABLE org_memberships_new RENAME TO org_memberships;
CREATE INDEX IF NOT EXISTS org_memberships_org_id_idx    ON org_memberships(org_id);
CREATE INDEX IF NOT EXISTS org_memberships_member_id_idx ON org_memberships(member_id);

CREATE TABLE ci_runs_new (
    id              TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    module_id       TEXT NOT NULL REFERENCES modules(id) ON DELETE CASCADE,
    commit_hash     TEXT NOT NULL,
    lint_passed     INTEGER NOT NULL DEFAULT 0,
    breaking_passed INTEGER NOT NULL DEFAULT 0,
    lint_errors     TEXT NOT NULL DEFAULT '[]',
    breaking_errors TEXT NOT NULL DEFAULT '[]',
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(module_id, commit_hash)
);
INSERT INTO ci_runs_new SELECT
    id, module_id, commit_hash, lint_passed, breaking_passed,
    lint_errors, breaking_errors, created_at
FROM ci_runs;
DROP TABLE ci_runs;
ALTER TABLE ci_runs_new RENAME TO ci_runs;

CREATE TABLE notifications_new (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    user_id     TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type        TEXT NOT NULL,
    title       TEXT NOT NULL,
    body        TEXT,
    resource_id TEXT,
    read_at     DATETIME,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
INSERT INTO notifications_new SELECT
    id, user_id, type, title, body, resource_id, read_at, created_at
FROM notifications;
DROP TABLE notifications;
ALTER TABLE notifications_new RENAME TO notifications;
CREATE INDEX IF NOT EXISTS notifications_user_id_idx ON notifications(user_id);

CREATE INDEX IF NOT EXISTS idx_opa_role_bindings_subject ON opa_role_bindings(subject);
