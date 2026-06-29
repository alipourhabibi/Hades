-- SQLite schema for Hades metadata backend.
-- Uses TEXT UUIDs (SQLite has no native UUID type), datetime() for timestamps,
-- and INTEGER for booleans (0/1). JSON stored as TEXT.

PRAGMA journal_mode=WAL;
PRAGMA foreign_keys=ON;

CREATE TABLE IF NOT EXISTS users (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(4))) || '-' || lower(hex(randomblob(2))) || '-4' || substr(lower(hex(randomblob(2))),2) || '-' || substr('89ab',abs(random()) % 4 + 1, 1) || substr(lower(hex(randomblob(2))),2) || '-' || lower(hex(randomblob(6)))),
    create_time DATETIME NOT NULL DEFAULT (datetime('now')),
    update_time DATETIME NOT NULL DEFAULT (datetime('now')),
    username    TEXT UNIQUE NOT NULL,
    email       TEXT UNIQUE NOT NULL,
    password    TEXT NOT NULL DEFAULT '',
    type        INTEGER NOT NULL DEFAULT 0,
    state       INTEGER NOT NULL DEFAULT 1,
    description TEXT,
    url         TEXT,
    failed_login_count  INTEGER NOT NULL DEFAULT 0,
    locked_until        DATETIME,
    email_verified_at   DATETIME
);

CREATE TABLE IF NOT EXISTS sessions (
    id                   TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    create_time          DATETIME NOT NULL DEFAULT (datetime('now')),
    user_id              TEXT NOT NULL REFERENCES users(id),
    auth_module          TEXT NOT NULL DEFAULT '',
    expires_at           DATETIME NOT NULL,
    token_hash           TEXT,
    old_token_hash       TEXT,
    old_token_expires_at DATETIME,
    ip_address           TEXT,
    user_agent           TEXT,
    last_activity_at     DATETIME,
    absolute_expires_at  DATETIME,
    revoked_at           DATETIME,
    totp_verified        INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_sessions_token_hash ON sessions(token_hash);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);

CREATE TABLE IF NOT EXISTS modules (
    id                TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    create_time       DATETIME NOT NULL DEFAULT (datetime('now')),
    update_time       DATETIME NOT NULL DEFAULT (datetime('now')),
    name              TEXT UNIQUE NOT NULL,
    owner_id          TEXT NOT NULL REFERENCES users(id),
    visibility        INTEGER NOT NULL DEFAULT 0,
    state             INTEGER NOT NULL DEFAULT 0,
    description       TEXT,
    url               TEXT,
    default_label_name TEXT,
    default_branch    TEXT NOT NULL DEFAULT 'main'
);

CREATE TABLE IF NOT EXISTS commits (
    id                TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    create_time       DATETIME NOT NULL DEFAULT (datetime('now')),
    update_time       DATETIME NOT NULL DEFAULT (datetime('now')),
    commit_hash       TEXT NOT NULL,
    owner_id          TEXT NOT NULL REFERENCES users(id),
    module_id         TEXT NOT NULL REFERENCES modules(id),
    digest_type       INTEGER NOT NULL DEFAULT 0,
    digest_value      TEXT,
    created_by_user_id TEXT,
    source_control_url TEXT
);

CREATE TABLE IF NOT EXISTS sdk_jobs (
    id              TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    commit_id       TEXT NOT NULL REFERENCES commits(id),
    module_id       TEXT NOT NULL REFERENCES modules(id),
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

CREATE TABLE IF NOT EXISTS casbin_rule (
    id    TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    ptype TEXT,
    v0    TEXT,
    v1    TEXT,
    v2    TEXT,
    v3    TEXT,
    v4    TEXT,
    v5    TEXT
);

CREATE TABLE IF NOT EXISTS opa_role_bindings (
    id         TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    subject    TEXT NOT NULL,
    role       TEXT NOT NULL,
    domain     TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(subject, role, domain)
);

CREATE TABLE IF NOT EXISTS email_verifications (
    id         TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    user_id    TEXT NOT NULL REFERENCES users(id),
    token_hash TEXT NOT NULL,
    expires_at DATETIME NOT NULL,
    used_at    DATETIME
);

CREATE TABLE IF NOT EXISTS password_resets (
    id         TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    user_id    TEXT NOT NULL REFERENCES users(id),
    token_hash TEXT NOT NULL,
    expires_at DATETIME NOT NULL,
    used_at    DATETIME
);

CREATE TABLE IF NOT EXISTS oauth_identities (
    id           TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    create_time  DATETIME NOT NULL DEFAULT (datetime('now')),
    user_id      TEXT NOT NULL REFERENCES users(id),
    provider     TEXT NOT NULL,
    provider_uid TEXT NOT NULL,
    email        TEXT,
    UNIQUE(provider, provider_uid)
);

CREATE TABLE IF NOT EXISTS api_tokens (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    create_time DATETIME NOT NULL DEFAULT (datetime('now')),
    user_id     TEXT NOT NULL REFERENCES users(id),
    name        TEXT NOT NULL,
    prefix      TEXT NOT NULL,
    token_hash  TEXT NOT NULL,
    scopes      TEXT,
    expires_at  DATETIME,
    last_used_at DATETIME,
    revoked_at  DATETIME
);

CREATE TABLE IF NOT EXISTS device_grants (
    id               TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    create_time      DATETIME NOT NULL DEFAULT (datetime('now')),
    device_code_hash TEXT NOT NULL,
    user_code        TEXT NOT NULL,
    user_id          TEXT REFERENCES users(id),
    api_token_id     TEXT REFERENCES api_tokens(id),
    approved_at      DATETIME,
    expires_at       DATETIME NOT NULL
);

CREATE TABLE IF NOT EXISTS totp_secrets (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    create_time DATETIME NOT NULL DEFAULT (datetime('now')),
    user_id     TEXT NOT NULL REFERENCES users(id) UNIQUE,
    secret_enc  TEXT NOT NULL,
    enabled     INTEGER NOT NULL DEFAULT 0,
    enrolled_at DATETIME
);

CREATE TABLE IF NOT EXISTS totp_backup_codes (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    create_time DATETIME NOT NULL DEFAULT (datetime('now')),
    user_id     TEXT NOT NULL REFERENCES users(id),
    code_hash   TEXT NOT NULL,
    used_at     DATETIME
);

CREATE TABLE IF NOT EXISTS audit_log (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    create_time DATETIME NOT NULL DEFAULT (datetime('now')),
    user_id     TEXT REFERENCES users(id),
    event       TEXT NOT NULL,
    ip_address  TEXT,
    user_agent  TEXT,
    metadata    TEXT
);

CREATE TABLE IF NOT EXISTS org_memberships (
    org_id    TEXT NOT NULL REFERENCES users(id),
    member_id TEXT NOT NULL REFERENCES users(id),
    role      TEXT NOT NULL DEFAULT 'member',
    PRIMARY KEY (org_id, member_id)
);

CREATE TABLE IF NOT EXISTS ci_runs (
    id              TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    module_id       TEXT NOT NULL REFERENCES modules(id),
    commit_hash     TEXT NOT NULL,
    lint_passed     INTEGER NOT NULL DEFAULT 0,
    breaking_passed INTEGER NOT NULL DEFAULT 0,
    lint_errors     TEXT NOT NULL DEFAULT '[]',
    breaking_errors TEXT NOT NULL DEFAULT '[]',
    created_at      DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(module_id, commit_hash)
);

CREATE TABLE IF NOT EXISTS notifications (
    id          TEXT PRIMARY KEY DEFAULT (lower(hex(randomblob(16)))),
    user_id     TEXT NOT NULL REFERENCES users(id),
    type        TEXT NOT NULL,
    title       TEXT NOT NULL,
    body        TEXT,
    resource_id TEXT,
    read_at     DATETIME,
    created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
