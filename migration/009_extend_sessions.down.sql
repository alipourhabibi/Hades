DROP INDEX IF EXISTS idx_sessions_token_hash;

ALTER TABLE sessions
  DROP COLUMN IF EXISTS token_hash,
  DROP COLUMN IF EXISTS ip_address,
  DROP COLUMN IF EXISTS user_agent,
  DROP COLUMN IF EXISTS last_activity_at,
  DROP COLUMN IF EXISTS absolute_expires_at,
  DROP COLUMN IF EXISTS revoked_at,
  DROP COLUMN IF EXISTS totp_verified,
  DROP COLUMN IF EXISTS old_token_hash,
  DROP COLUMN IF EXISTS old_token_expires_at;
