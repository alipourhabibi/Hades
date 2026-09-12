ALTER TABLE sessions DROP COLUMN IF EXISTS old_token_hash;
ALTER TABLE sessions DROP COLUMN IF EXISTS old_token_expires_at;
