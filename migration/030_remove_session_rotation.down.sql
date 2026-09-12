ALTER TABLE sessions ADD COLUMN IF NOT EXISTS old_token_hash       VARCHAR(64);
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS old_token_expires_at TIMESTAMP;
