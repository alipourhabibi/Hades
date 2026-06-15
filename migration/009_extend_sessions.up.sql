ALTER TABLE sessions
  ADD COLUMN IF NOT EXISTS token_hash           VARCHAR(64) UNIQUE,
  ADD COLUMN IF NOT EXISTS ip_address           VARCHAR(45),
  ADD COLUMN IF NOT EXISTS user_agent           TEXT,
  ADD COLUMN IF NOT EXISTS last_activity_at     TIMESTAMP,
  ADD COLUMN IF NOT EXISTS absolute_expires_at  TIMESTAMP,
  ADD COLUMN IF NOT EXISTS revoked_at           TIMESTAMP,
  ADD COLUMN IF NOT EXISTS totp_verified        BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS old_token_hash       VARCHAR(64),
  ADD COLUMN IF NOT EXISTS old_token_expires_at TIMESTAMP;

CREATE INDEX IF NOT EXISTS idx_sessions_token_hash ON sessions(token_hash);
